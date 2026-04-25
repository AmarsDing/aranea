// Package web implements the Aranea ADK SubLauncher that boots the
// backend HTTP server in-process. It mirrors the structure of
// google.golang.org/adk/cmd/launcher/web but delegates the actual
// service wiring to arenea/backend/internal/server so the standalone
// `aranea-server` binary and the embedded launcher always behave the
// same. This is the implementation of 前端/25 cli.md §1.4 / §5 — the
// `aranea web` keyword used to spin up a local playground.
package web

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	adklauncher "google.golang.org/adk/cmd/launcher"

	araneal "arenea/backend/cmd/aranea/internal/launcher"
	"arenea/backend/internal/server"
)

// NewLauncher constructs the web SubLauncher. cfg is captured to read
// the resolved backend config (HTTP address override, custom DB path)
// without mutating the ADK launcher.Config struct.
func NewLauncher(cfg *araneal.Config) adklauncher.SubLauncher {
	flags := flag.NewFlagSet("web", flag.ContinueOnError)
	c := &webConfig{}
	flags.StringVar(&c.Addr, "addr", "", "HTTP listen address (default: HTTP_ADDR env or :8080)")
	flags.StringVar(&c.DBPath, "db", "", "SQLite database file (default: DB_PATH env or data/arenea.db)")
	flags.BoolVar(&c.Quiet, "quiet", false, "Suppress server logs (the ready banner still prints)")
	return &webLauncher{flags: flags, config: c, arn: cfg}
}

// webConfig holds the parsed CLI flags for the web launcher.
type webConfig struct {
	Addr   string
	DBPath string
	Quiet  bool
}

// webLauncher implements adklauncher.SubLauncher.
type webLauncher struct {
	flags  *flag.FlagSet
	config *webConfig
	arn    *araneal.Config
}

// Keyword implements adklauncher.SubLauncher.
func (l *webLauncher) Keyword() string { return "web" }

// SimpleDescription implements adklauncher.SubLauncher.
func (l *webLauncher) SimpleDescription() string {
	return "boot the Aranea backend HTTP server (admin REST + chat SSE) in-process"
}

// CommandLineSyntax implements adklauncher.SubLauncher. We render the
// flag set ourselves to keep the launcher independent from adk's
// internal cli/util package.
func (l *webLauncher) CommandLineSyntax() string {
	var buf bytes.Buffer
	buf.WriteString("Flags:\n")
	l.flags.SetOutput(&buf)
	l.flags.PrintDefaults()
	return buf.String()
}

// Parse implements adklauncher.SubLauncher.
func (l *webLauncher) Parse(args []string) ([]string, error) {
	if err := l.flags.Parse(args); err != nil {
		return nil, fmt.Errorf("web: %w", err)
	}
	return l.flags.Args(), nil
}

// Run implements adklauncher.SubLauncher. It bridges the ADK launcher
// lifecycle with arenea/backend/internal/server.Run, taking care of:
//
//   - resolving the DB path (flag → CLI config → env → default)
//   - silencing the server logger when --quiet is set
//   - publishing the bound listen address through the Ready channel
//     so we can print a single, accurate banner pointing at the actual
//     port (important when --addr=:0 is used).
func (l *webLauncher) Run(ctx context.Context, _ *adklauncher.Config) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	logger := log.Default()
	if l.config.Quiet {
		logger = log.New(quietWriter{}, "", 0)
	}

	ready := make(chan server.ListenInfo, 1)
	go func() {
		select {
		case info := <-ready:
			fmt.Fprintf(os.Stdout, "aranea backend ready on http://%s\n", info.Addr)
		case <-ctx.Done():
		}
	}()

	return server.Run(ctx, server.Options{
		Addr:          l.config.Addr,
		DBPath:        l.config.DBPath,
		SkipTelemetry: false,
		Ready:         ready,
		Logger:        logger,
	})
}

// quietWriter swallows server log output when --quiet is set without
// requiring callers to import io/ioutil.
type quietWriter struct{}

func (quietWriter) Write(p []byte) (int, error) { return len(p), nil }
