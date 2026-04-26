// Package console implements the Aranea ADK SubLauncher that drives the
// interactive REPL. It mirrors the structure of
// google.golang.org/adk/cmd/launcher/console but talks to the Aranea
// backend over HTTP instead of running an in-process ADK runner. That
// way every action triggered by the system administrator agent goes
// through the audited /api/v1/* layer just like the web UI.
package console

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"

	adklauncher "google.golang.org/adk/cmd/launcher"

	"arenea/backend/cmd/internal/agent"
	"arenea/backend/cmd/internal/apiclient"
	araneal "arenea/backend/cmd/internal/launcher"
	"arenea/backend/cmd/internal/session"
	"arenea/backend/internal/domain"
	"arenea/backend/internal/service"
)

// NewLauncher constructs the console SubLauncher. The Aranea Config is
// captured here and consumed from Run() so we never need to attach
// pointers to non-ADK types onto adklauncher.Config.
func NewLauncher(cfg *araneal.Config) adklauncher.SubLauncher {
	flags := flag.NewFlagSet("console", flag.ContinueOnError)
	c := &consoleConfig{}
	flags.StringVar(&c.AgentKey, "agent", "__system_admin__", "agent_key to chat with")
	flags.StringVar(&c.SessionID, "session", "", "Resume an existing session id")
	flags.StringVar(&c.Mode, "mode", "default", "Dialog mode (default|plan|code|...)")
	flags.BoolVar(&c.AutoYes, "yes", false, "Auto-confirm any /confirm prompts")
	flags.BoolVar(&c.NoStream, "no-stream", false, "Disable SSE streaming and use POST /chat/messages instead")
	return &consoleLauncher{flags: flags, config: c, arn: cfg}
}

// consoleConfig holds the parsed CLI flags for the console launcher.
type consoleConfig struct {
	AgentKey  string
	SessionID string
	Mode      string
	AutoYes   bool
	NoStream  bool
}

// consoleLauncher implements adklauncher.SubLauncher.
type consoleLauncher struct {
	flags  *flag.FlagSet
	config *consoleConfig
	arn    *araneal.Config
}

// Keyword implements adklauncher.SubLauncher.
func (l *consoleLauncher) Keyword() string { return "console" }

// SimpleDescription implements adklauncher.SubLauncher.
func (l *consoleLauncher) SimpleDescription() string {
	return "interactive REPL chatting with the system administrator agent"
}

// CommandLineSyntax implements adklauncher.SubLauncher. We render the
// flag set ourselves to avoid depending on adk's internal cli/util pkg.
func (l *consoleLauncher) CommandLineSyntax() string {
	var buf bytes.Buffer
	buf.WriteString("Flags:\n")
	l.flags.SetOutput(&buf)
	l.flags.PrintDefaults()
	return buf.String()
}

// Parse implements adklauncher.SubLauncher.
func (l *consoleLauncher) Parse(args []string) ([]string, error) {
	if err := l.flags.Parse(args); err != nil {
		return nil, fmt.Errorf("console: %w", err)
	}
	return l.flags.Args(), nil
}

// Run implements adklauncher.SubLauncher. It is the heart of the
// interactive console: resolve the target agent, ensure a session
// exists, then loop on user input forwarding each line to the chat
// stream API and rendering the SSE events back to the terminal.
func (l *consoleLauncher) Run(ctx context.Context, _ *adklauncher.Config) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	g := l.arn.Client
	if g == nil {
		return errors.New("console launcher: no API client configured")
	}

	agentRef, err := agent.Resolve(ctx, g, l.config.AgentKey)
	if err != nil {
		// Best effort fallback: pick the first listed agent so a brand new
		// install (where __system_admin__ has not been seeded yet) still
		// gives the user a working REPL.
		fallback, fbErr := pickFallbackAgent(ctx, g)
		if fbErr != nil {
			return fmt.Errorf("agent %q not found and no fallback available: %w", l.config.AgentKey, err)
		}
		fmt.Fprintf(os.Stderr, "warning: agent %q not found, falling back to %q\n", l.config.AgentKey, fallback.AgentKey)
		agentRef = fallback
	}

	sess, err := session.EnsureSession(ctx, g, l.config.SessionID, agentRef.ID, "CLI console with "+agentRef.DisplayName)
	if err != nil {
		return err
	}

	printBanner(agentRef, sess, l.config.Mode)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)

	for {
		fmt.Print("\x1b[36mYou\x1b[0m > ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			fmt.Println()
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if handled, exit := l.handleSlash(ctx, g, &sess, line); handled {
			if exit {
				return nil
			}
			continue
		}
		if err := l.send(ctx, g, sess.ID, agentRef.AgentKey, line); err != nil {
			fmt.Fprintf(os.Stderr, "\x1b[31merror\x1b[0m %v\n", err)
		}
	}
}

func pickFallbackAgent(ctx context.Context, g *apiclient.GlobalContext) (domain.Agent, error) {
	var resp domain.AgentListResult
	if err := g.Client().Get(ctx, "/api/v1/agents", nil, &resp); err != nil {
		return domain.Agent{}, err
	}
	if len(resp.Items) == 0 {
		return domain.Agent{}, errors.New("no agents available")
	}
	return resp.Items[0], nil
}

func printBanner(a domain.Agent, s domain.Session, mode string) {
	fmt.Printf("\x1b[1mAranea console\x1b[0m\n")
	fmt.Printf("  agent  : %s (%s)\n", a.DisplayName, a.AgentKey)
	fmt.Printf("  session: %s\n", s.ID)
	fmt.Printf("  mode   : %s\n", mode)
	fmt.Printf("Type /help for commands, /quit to exit.\n\n")
}

// handleSlash implements the lightweight slash-command surface defined
// in 前端/25 cli.md §1.7 (subset). It returns (handled, exit) — exit is
// true only for /quit and /exit so the caller can break out of Run().
func (l *consoleLauncher) handleSlash(ctx context.Context, g *apiclient.GlobalContext, sess *domain.Session, line string) (bool, bool) {
	if !strings.HasPrefix(line, "/") {
		return false, false
	}
	parts := strings.Fields(line)
	switch parts[0] {
	case "/help", "/?":
		fmt.Println("/help                  show this message")
		fmt.Println("/quit, /exit           leave the REPL")
		fmt.Println("/clear                 clear screen")
		fmt.Println("/session new           start a fresh session with the same agent")
		fmt.Println("/agent <key>           switch to another agent (creates a new session)")
		fmt.Println("/run <cli args...>     run an `aranea` sub-command in-process")
	case "/quit", "/exit":
		return true, true
	case "/clear":
		fmt.Print("\x1b[2J\x1b[H")
	case "/session":
		if len(parts) >= 2 && parts[1] == "new" {
			ns, err := session.EnsureSession(ctx, g, "", sess.AgentID, "CLI console (new)")
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
			} else {
				*sess = ns
				fmt.Printf("started session %s\n", ns.ID)
			}
		}
	case "/agent":
		if len(parts) < 2 {
			fmt.Println("usage: /agent <agent_key>")
			break
		}
		a, err := agent.Resolve(ctx, g, parts[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			break
		}
		ns, err := session.EnsureSession(ctx, g, "", a.ID, "CLI console with "+a.DisplayName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			break
		}
		*sess = ns
		fmt.Printf("switched to %s (%s)\n", a.DisplayName, a.AgentKey)
	case "/run":
		fmt.Println("(/run is not implemented in this build — open another terminal)")
	default:
		fmt.Printf("unknown slash command: %s\n", parts[0])
	}
	return true, false
}

// send streams a single user message and prints the agent reply to
// stdout. When NoStream is set we fall back to the synchronous endpoint
// so the console works against backends that have streaming disabled.
func (l *consoleLauncher) send(ctx context.Context, g *apiclient.GlobalContext, sessionID, agentKey, content string) error {
	in := service.SendMessageInput{
		SessionID: sessionID,
		AgentKey:  agentKey,
		Content:   content,
		Options:   service.SendMessageOptions{DialogMode: l.config.Mode},
	}
	if l.config.NoStream {
		var out service.SendMessageResult
		if err := g.Client().Post(ctx, "/api/v1/chat/messages", in, &out); err != nil {
			return err
		}
		fmt.Printf("\x1b[35m%s\x1b[0m > %s\n\n", out.AgentMessage.ModelName, out.AgentMessage.Content)
		return nil
	}
	return l.stream(ctx, g, in)
}

// stream POSTs to /api/v1/chat/messages/stream and renders the SSE
// stream incrementally. The implementation is intentionally minimal —
// we parse `event:` and `data:` lines and dispatch on event type.
func (l *consoleLauncher) stream(ctx context.Context, g *apiclient.GlobalContext, in service.SendMessageInput) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	url := strings.TrimRight(g.Client().BaseURL(), "/") + "/api/v1/chat/messages/stream"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if token := g.Client().Token(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stream %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	fmt.Print("\x1b[35magent\x1b[0m > ")
	reader := bufio.NewReader(resp.Body)
	currentEvent := "message"
	wroteAny := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			currentEvent = "message"
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		switch currentEvent {
		case "delta":
			var d struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal([]byte(data), &d); err == nil {
				fmt.Print(d.Content)
				wroteAny = true
			}
		case "done":
			var d map[string]domain.Message
			if err := json.Unmarshal([]byte(data), &d); err == nil {
				if msg, ok := d["agent_message"]; ok && !wroteAny {
					fmt.Print(msg.Content)
				}
			}
		case "error":
			var d struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal([]byte(data), &d)
			fmt.Fprintf(os.Stderr, "\n\x1b[31merror\x1b[0m %s\n", d.Message)
		}
	}
	fmt.Println()
	return nil
}
