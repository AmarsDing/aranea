package runtime

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

type runnerRuntimeBackend struct {
	adapter *ADKRuntimeAdapter
}

func newRunnerRuntimeBackend(adapter *ADKRuntimeAdapter) runtimeBackend {
	return &runnerRuntimeBackend{adapter: adapter}
}

func (b *runnerRuntimeBackend) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	return b.run(ctx, req, nil)
}

func (b *runnerRuntimeBackend) StreamGenerate(ctx context.Context, req GenerateRequest, onDelta DeltaFunc) (GenerateResult, error) {
	return b.run(ctx, req, onDelta)
}

func (b *runnerRuntimeBackend) run(ctx context.Context, req GenerateRequest, onDelta DeltaFunc) (GenerateResult, error) {
	if strings.TrimSpace(req.Input) == "" {
		return GenerateResult{}, fmt.Errorf("empty input")
	}
	started := time.Now()
	rootAgent, err := b.buildAgent(req)
	if err != nil {
		return GenerateResult{}, err
	}
	plugins, err := b.adapter.builtinPlugins(ctx)
	if err != nil {
		return GenerateResult{}, err
	}
	r, err := runner.New(runner.Config{
		AppName:           "aranea",
		Agent:             rootAgent,
		SessionService:    session.InMemoryService(),
		PluginConfig:      runner.PluginConfig{Plugins: plugins},
		AutoCreateSession: true,
	})
	if err != nil {
		return GenerateResult{}, err
	}

	var finalText string
	emittedPartial := false
	for event, runErr := range r.Run(ctx, "aranea-user", runnerSessionID(req), genai.NewContentFromText(req.Input, genai.RoleUser), agent.RunConfig{}) {
		if runErr != nil {
			return GenerateResult{}, runErr
		}
		if event == nil {
			continue
		}
		text := llmResponseText(&event.LLMResponse)
		if text == "" {
			continue
		}
		finalText = text
		if onDelta != nil && event.LLMResponse.Partial {
			if err = onDelta(text); err != nil {
				return GenerateResult{}, err
			}
			emittedPartial = true
		}
	}
	if strings.TrimSpace(finalText) == "" {
		return GenerateResult{}, fmt.Errorf("adk runner returned empty response")
	}
	if onDelta != nil && !emittedPartial {
		if err = onDelta(finalText); err != nil {
			return GenerateResult{}, err
		}
	}
	cfg, _ := parseProviderConfig(req.ProviderModel.ConfigJSON)
	return GenerateResult{
		Content:          finalText,
		ModelName:        firstNonEmpty(req.ProviderModel.Model, req.ProviderModel.Name),
		PromptTokens:     estimatePromptTokens(req, cfg),
		CompletionTokens: estimateTokens(finalText),
		LatencyMS:        int(time.Since(started).Milliseconds()),
	}, nil
}

func (b *runnerRuntimeBackend) buildAgent(req GenerateRequest) (agent.Agent, error) {
	tools, err := adkFilesystemTools()
	if err != nil {
		return nil, err
	}
	beforeTool, afterTool := runnerToolCallbacks(req)
	return llmagent.New(llmagent.Config{
		Name:                adkAgentName(req),
		Description:         strings.TrimSpace(req.Agent.AgentDescription),
		Instruction:         buildSystemPrompt(req.Agent),
		Model:               newProviderModelLLM(b.adapter, req.Agent, req.ProviderModel),
		Tools:               tools,
		BeforeToolCallbacks: []llmagent.BeforeToolCallback{beforeTool},
		AfterToolCallbacks:  []llmagent.AfterToolCallback{afterTool},
	})
}

func runnerToolCallbacks(req GenerateRequest) (llmagent.BeforeToolCallback, llmagent.AfterToolCallback) {
	var mu sync.Mutex
	started := map[string]time.Time{}
	before := func(ctx tool.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
		if req.OnToolEvent != nil && t != nil {
			id := toolEventID(ctx, t.Name())
			mu.Lock()
			started[id] = time.Now()
			mu.Unlock()
			if err := req.OnToolEvent(newRunnerToolEvent(req, id, "before", "running", t.Name(), args, nil, nil, 0)); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	after := func(ctx tool.Context, t tool.Tool, args, result map[string]any, toolErr error) (map[string]any, error) {
		if req.OnToolEvent != nil && t != nil {
			id := toolEventID(ctx, t.Name())
			durationMS := 0
			mu.Lock()
			if at, ok := started[id]; ok {
				durationMS = int(time.Since(at).Milliseconds())
				delete(started, id)
			}
			mu.Unlock()
			status := "success"
			if toolErr != nil {
				status = "failed"
			}
			if err := req.OnToolEvent(newRunnerToolEvent(req, id, "after", status, t.Name(), args, result, toolErr, durationMS)); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	return before, after
}

func newRunnerToolEvent(req GenerateRequest, id string, phase string, status string, toolName string, args map[string]any, result map[string]any, toolErr error, durationMS int) ToolEvent {
	event := ToolEvent{
		ID:         id,
		Phase:      phase,
		Status:     status,
		AgentID:    req.Agent.ID,
		AgentKey:   req.Agent.AgentKey,
		AgentName:  firstNonEmpty(req.Agent.DisplayName, req.Agent.AgentKey, req.Agent.ID),
		AgentIcon:  req.Agent.Icon,
		ToolName:   toolName,
		ToolLabel:  toolDisplayLabel(toolName),
		Arguments:  sanitizeToolArgs(args),
		Result:     summarizeToolResult(result),
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		DurationMS: durationMS,
	}
	if toolErr != nil {
		event.Error = toolErr.Error()
	}
	if phase == "before" {
		event.MessageHint = fmt.Sprintf("%s 正在使用 %s", event.AgentName, event.ToolLabel)
	} else if status == "success" {
		event.MessageHint = fmt.Sprintf("%s 已完成 %s", event.AgentName, event.ToolLabel)
	} else {
		event.MessageHint = fmt.Sprintf("%s 使用 %s 失败", event.AgentName, event.ToolLabel)
	}
	return event
}

func toolEventID(ctx tool.Context, fallback string) string {
	if ctx != nil && strings.TrimSpace(ctx.FunctionCallID()) != "" {
		return strings.TrimSpace(ctx.FunctionCallID())
	}
	return strings.TrimSpace(fallback)
}

func toolDisplayLabel(name string) string {
	switch name {
	case "read_file":
		return "读取文件"
	case "write_file":
		return "写入文件"
	case "list_files":
		return "列出文件"
	case "edit_file":
		return "编辑文件"
	default:
		return name
	}
}

func sanitizeToolArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	out := map[string]any{}
	for k, v := range args {
		if strings.EqualFold(k, "content") || strings.EqualFold(k, "new_string") || strings.EqualFold(k, "old_string") {
			text := fmt.Sprint(v)
			if len([]rune(text)) > 80 {
				runes := []rune(text)
				text = string(runes[:80]) + "..."
			}
			out[k] = text
			continue
		}
		out[k] = v
	}
	return out
}

func summarizeToolResult(result map[string]any) map[string]any {
	if len(result) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{"path", "written", "replacements", "size"} {
		if value, ok := result[key]; ok {
			out[key] = value
		}
	}
	if items, ok := result["items"].([]map[string]any); ok {
		out["items_count"] = len(items)
	} else if items, ok := result["items"].([]any); ok {
		out["items_count"] = len(items)
	}
	return out
}

func runnerSessionID(req GenerateRequest) string {
	key := strings.TrimSpace(req.Agent.ID)
	if key == "" {
		key = strings.TrimSpace(req.Agent.AgentKey)
	}
	if key == "" {
		key = "default"
	}
	return "runtime-" + sanitizeIdentifier(key)
}

func adkAgentName(req GenerateRequest) string {
	name := firstNonEmpty(req.Agent.AgentKey, req.Agent.ID, req.Agent.DisplayName, "aranea_agent")
	return sanitizeIdentifier(name)
}

var unsafeIdentifierChars = regexp.MustCompile(`[^A-Za-z0-9_]`)

func sanitizeIdentifier(value string) string {
	value = unsafeIdentifierChars.ReplaceAllString(strings.TrimSpace(value), "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return "aranea_agent"
	}
	if value[0] >= '0' && value[0] <= '9' {
		value = "agent_" + value
	}
	return value
}
