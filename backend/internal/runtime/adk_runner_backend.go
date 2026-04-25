package runtime

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
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
	return llmagent.New(llmagent.Config{
		Name:        adkAgentName(req),
		Description: strings.TrimSpace(req.Agent.AgentDescription),
		Instruction: buildSystemPrompt(req.Agent),
		Model:       newProviderModelLLM(b.adapter, req.Agent, req.ProviderModel),
	})
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
