package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"arenea/backend/internal/domain"
)

// ADKRuntimeAdapter 是对 adk-go 的适配边界。
// 当前实现支持 OpenAI Compatible / Anthropic 直连，未配置连接信息时保留可运行 stub。
type ADKRuntimeAdapter struct {
	client        *http.Client
	backend       runtimeBackend
	direct        *directRuntimeBackend
	runner        runtimeBackend
	pluginSource  PluginSource
	channelSource ChannelSource
	channelMu     sync.RWMutex
	channels      []domain.ChannelRuntimeConfig
}

type PluginSource interface {
	EnabledPluginKeys(context.Context) ([]string, error)
}

type ChannelSource interface {
	EnabledChannelConfigs(context.Context) ([]domain.ChannelRuntimeConfig, error)
}

type runtimeBackend interface {
	Generate(context.Context, GenerateRequest) (GenerateResult, error)
	StreamGenerate(context.Context, GenerateRequest, DeltaFunc) (GenerateResult, error)
}

type directRuntimeBackend struct {
	adapter *ADKRuntimeAdapter
}

type GenerateRequest struct {
	Agent         domain.Agent
	ProviderModel domain.PlatformResource
	Messages      []ChatMessage
	Input         string
}

type ChatMessage struct {
	Role    string
	Content string
}

type GenerateResult struct {
	Content          string
	ModelName        string
	PromptTokens     int
	CompletionTokens int
	LatencyMS        int
}

type DeltaFunc func(delta string) error

type providerConfig struct {
	ProviderType    string `json:"provider_type"`
	APIBaseURL      string `json:"api_base_url"`
	APIKey          string `json:"api_key"`
	APIKeySet       bool   `json:"api_key_set"`
	ContextWindowK  int    `json:"context_window_k"`
	MaxOutputTokens int    `json:"max_output_tokens"`
}

func NewADKRuntimeAdapter() *ADKRuntimeAdapter {
	adapter := &ADKRuntimeAdapter{
		client: &http.Client{Timeout: 60 * time.Second},
	}
	adapter.direct = &directRuntimeBackend{adapter: adapter}
	adapter.backend = adapter.direct
	adapter.runner = newRunnerRuntimeBackend(adapter)
	return adapter
}

func (a *ADKRuntimeAdapter) SetPluginSource(source PluginSource) {
	a.pluginSource = source
}

func (a *ADKRuntimeAdapter) SetChannelSource(source ChannelSource) {
	a.channelSource = source
}

func (a *ADKRuntimeAdapter) ReloadChannels(ctx context.Context) error {
	if a.channelSource == nil {
		a.channelMu.Lock()
		a.channels = nil
		a.channelMu.Unlock()
		return nil
	}
	channels, err := a.channelSource.EnabledChannelConfigs(ctx)
	if err != nil {
		return err
	}
	a.channelMu.Lock()
	a.channels = append([]domain.ChannelRuntimeConfig{}, channels...)
	a.channelMu.Unlock()
	return nil
}

func (a *ADKRuntimeAdapter) ChannelConfigs() []domain.ChannelRuntimeConfig {
	a.channelMu.RLock()
	defer a.channelMu.RUnlock()
	return append([]domain.ChannelRuntimeConfig{}, a.channels...)
}

func (a *ADKRuntimeAdapter) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	return a.activeBackend().Generate(ctx, req)
}

func (a *ADKRuntimeAdapter) StreamGenerate(ctx context.Context, req GenerateRequest, onDelta DeltaFunc) (GenerateResult, error) {
	return a.activeBackend().StreamGenerate(ctx, req, onDelta)
}

func (a *ADKRuntimeAdapter) activeBackend() runtimeBackend {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("RUNTIME_BACKEND")), "adk_runner") && a.runner != nil {
		return a.runner
	}
	if a.backend != nil {
		return a.backend
	}
	return a.direct
}

func (b *directRuntimeBackend) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	return b.adapter.generateDirect(ctx, req)
}

func (b *directRuntimeBackend) StreamGenerate(ctx context.Context, req GenerateRequest, onDelta DeltaFunc) (GenerateResult, error) {
	return b.adapter.streamDirect(ctx, req, onDelta)
}

func (a *ADKRuntimeAdapter) generateDirect(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	trimmed := strings.TrimSpace(req.Input)
	if trimmed == "" {
		return GenerateResult{}, fmt.Errorf("empty input")
	}

	cfg, err := parseProviderConfig(req.ProviderModel.ConfigJSON)
	if err != nil {
		return GenerateResult{}, err
	}

	if strings.TrimSpace(cfg.APIBaseURL) == "" || strings.TrimSpace(cfg.APIKey) == "" {
		content := fmt.Sprintf("[stub/%s] %s", req.Agent.AgentKey, trimmed)
		return GenerateResult{
			Content:          content,
			ModelName:        req.ProviderModel.Model,
			PromptTokens:     estimatePromptTokens(req, cfg),
			CompletionTokens: estimateTokens(content),
			LatencyMS:        20,
		}, nil
	}

	started := time.Now()
	var result GenerateResult
	if isAnthropicProvider(cfg.ProviderType) {
		result, err = a.generateAnthropic(ctx, cfg, req)
	} else {
		result, err = a.generateOpenAICompatible(ctx, cfg, req)
	}
	if err != nil {
		return GenerateResult{}, err
	}
	result.LatencyMS = int(time.Since(started).Milliseconds())
	if result.ModelName == "" {
		result.ModelName = req.ProviderModel.Model
	}
	if result.PromptTokens == 0 {
		result.PromptTokens = estimatePromptTokens(req, cfg)
	}
	if result.CompletionTokens == 0 {
		result.CompletionTokens = estimateTokens(result.Content)
	}
	return result, nil
}

func (a *ADKRuntimeAdapter) streamDirect(ctx context.Context, req GenerateRequest, onDelta DeltaFunc) (GenerateResult, error) {
	trimmed := strings.TrimSpace(req.Input)
	if trimmed == "" {
		return GenerateResult{}, fmt.Errorf("empty input")
	}

	cfg, err := parseProviderConfig(req.ProviderModel.ConfigJSON)
	if err != nil {
		return GenerateResult{}, err
	}

	if strings.TrimSpace(cfg.APIBaseURL) == "" || strings.TrimSpace(cfg.APIKey) == "" {
		content := fmt.Sprintf("[stub/%s] %s", req.Agent.AgentKey, trimmed)
		if onDelta != nil {
			if err = onDelta(content); err != nil {
				return GenerateResult{}, err
			}
		}
		return GenerateResult{
			Content:          content,
			ModelName:        req.ProviderModel.Model,
			PromptTokens:     estimatePromptTokens(req, cfg),
			CompletionTokens: estimateTokens(content),
			LatencyMS:        20,
		}, nil
	}

	started := time.Now()
	var result GenerateResult
	if isAnthropicProvider(cfg.ProviderType) {
		result, err = a.streamAnthropic(ctx, cfg, req, onDelta)
	} else {
		result, err = a.streamOpenAICompatible(ctx, cfg, req, onDelta)
	}
	if err != nil {
		return GenerateResult{}, err
	}
	result.LatencyMS = int(time.Since(started).Milliseconds())
	if result.ModelName == "" {
		result.ModelName = req.ProviderModel.Model
	}
	if result.PromptTokens == 0 {
		result.PromptTokens = estimatePromptTokens(req, cfg)
	}
	if result.CompletionTokens == 0 {
		result.CompletionTokens = estimateTokens(result.Content)
	}
	return result, nil
}

func parseProviderConfig(raw string) (providerConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return providerConfig{}, nil
	}
	var cfg providerConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return providerConfig{}, fmt.Errorf("invalid provider config: %w", err)
	}
	return cfg, nil
}

func (a *ADKRuntimeAdapter) generateOpenAICompatible(ctx context.Context, cfg providerConfig, req GenerateRequest) (GenerateResult, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	payload := struct {
		Model     string    `json:"model"`
		Messages  []message `json:"messages"`
		Stream    bool      `json:"stream"`
		MaxTokens int       `json:"max_tokens,omitempty"`
	}{
		Model:     req.ProviderModel.Model,
		Stream:    false,
		MaxTokens: resolveMaxOutputTokens(cfg),
	}
	if system := buildSystemPrompt(req.Agent); system != "" {
		payload.Messages = append(payload.Messages, message{Role: "system", Content: system})
	}
	for _, item := range trimMessagesByContext(req.Messages, cfg.ContextWindowK) {
		role := normalizeChatRole(item.Role)
		if role == "system" {
			role = "user"
		}
		payload.Messages = append(payload.Messages, message{Role: role, Content: item.Content})
	}

	var out struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := a.postJSON(ctx, chatCompletionsURL(cfg.APIBaseURL), cfg.APIKey, payload, &out, nil); err != nil {
		return GenerateResult{}, err
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return GenerateResult{}, fmt.Errorf("model returned empty response")
	}
	return GenerateResult{
		Content:          out.Choices[0].Message.Content,
		ModelName:        firstNonEmpty(out.Model, req.ProviderModel.Model),
		PromptTokens:     out.Usage.PromptTokens,
		CompletionTokens: out.Usage.CompletionTokens,
	}, nil
}

func (a *ADKRuntimeAdapter) generateAnthropic(ctx context.Context, cfg providerConfig, req GenerateRequest) (GenerateResult, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	payload := struct {
		Model     string    `json:"model"`
		System    string    `json:"system,omitempty"`
		MaxTokens int       `json:"max_tokens"`
		Messages  []message `json:"messages"`
	}{
		Model:     req.ProviderModel.Model,
		System:    buildSystemPrompt(req.Agent),
		MaxTokens: resolveMaxOutputTokens(cfg),
	}
	for _, item := range trimMessagesByContext(req.Messages, cfg.ContextWindowK) {
		role := normalizeChatRole(item.Role)
		if role == "system" {
			continue
		}
		payload.Messages = append(payload.Messages, message{Role: role, Content: item.Content})
	}

	var out struct {
		Model   string `json:"model"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	headers := map[string]string{
		"anthropic-version": "2023-06-01",
	}
	if err := a.postJSON(ctx, messagesURL(cfg.APIBaseURL), cfg.APIKey, payload, &out, headers); err != nil {
		return GenerateResult{}, err
	}
	parts := []string{}
	for _, item := range out.Content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			parts = append(parts, item.Text)
		}
	}
	content := strings.TrimSpace(strings.Join(parts, "\n"))
	if content == "" {
		return GenerateResult{}, fmt.Errorf("model returned empty response")
	}
	return GenerateResult{
		Content:          content,
		ModelName:        firstNonEmpty(out.Model, req.ProviderModel.Model),
		PromptTokens:     out.Usage.InputTokens,
		CompletionTokens: out.Usage.OutputTokens,
	}, nil
}

func (a *ADKRuntimeAdapter) streamOpenAICompatible(ctx context.Context, cfg providerConfig, req GenerateRequest, onDelta DeltaFunc) (GenerateResult, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	payload := struct {
		Model     string    `json:"model"`
		Messages  []message `json:"messages"`
		Stream    bool      `json:"stream"`
		MaxTokens int       `json:"max_tokens,omitempty"`
	}{
		Model:     req.ProviderModel.Model,
		Stream:    true,
		MaxTokens: resolveMaxOutputTokens(cfg),
	}
	if system := buildSystemPrompt(req.Agent); system != "" {
		payload.Messages = append(payload.Messages, message{Role: "system", Content: system})
	}
	for _, item := range trimMessagesByContext(req.Messages, cfg.ContextWindowK) {
		role := normalizeChatRole(item.Role)
		if role == "system" {
			role = "user"
		}
		payload.Messages = append(payload.Messages, message{Role: role, Content: item.Content})
	}

	response, err := a.postStream(ctx, chatCompletionsURL(cfg.APIBaseURL), cfg.APIKey, payload, nil)
	if err != nil {
		return GenerateResult{}, err
	}
	defer response.Body.Close()

	var content strings.Builder
	result := GenerateResult{ModelName: req.ProviderModel.Model}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk struct {
			Model   string `json:"model"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err = json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Model != "" {
			result.ModelName = chunk.Model
		}
		if chunk.Usage.PromptTokens > 0 {
			result.PromptTokens = chunk.Usage.PromptTokens
		}
		if chunk.Usage.CompletionTokens > 0 {
			result.CompletionTokens = chunk.Usage.CompletionTokens
		}
		for _, choice := range chunk.Choices {
			delta := choice.Delta.Content
			if delta == "" {
				continue
			}
			content.WriteString(delta)
			if onDelta != nil {
				if err = onDelta(delta); err != nil {
					return GenerateResult{}, err
				}
			}
		}
	}
	if err = scanner.Err(); err != nil {
		return GenerateResult{}, err
	}
	result.Content = content.String()
	if strings.TrimSpace(result.Content) == "" {
		return GenerateResult{}, fmt.Errorf("model returned empty response")
	}
	if result.PromptTokens == 0 {
		result.PromptTokens = estimatePromptTokens(req, cfg)
	}
	if result.CompletionTokens == 0 {
		result.CompletionTokens = estimateTokens(result.Content)
	}
	return result, nil
}

func (a *ADKRuntimeAdapter) streamAnthropic(ctx context.Context, cfg providerConfig, req GenerateRequest, onDelta DeltaFunc) (GenerateResult, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	payload := struct {
		Model     string    `json:"model"`
		System    string    `json:"system,omitempty"`
		MaxTokens int       `json:"max_tokens"`
		Stream    bool      `json:"stream"`
		Messages  []message `json:"messages"`
	}{
		Model:     req.ProviderModel.Model,
		System:    buildSystemPrompt(req.Agent),
		MaxTokens: resolveMaxOutputTokens(cfg),
		Stream:    true,
	}
	for _, item := range trimMessagesByContext(req.Messages, cfg.ContextWindowK) {
		role := normalizeChatRole(item.Role)
		if role == "system" {
			continue
		}
		payload.Messages = append(payload.Messages, message{Role: role, Content: item.Content})
	}

	headers := map[string]string{"anthropic-version": "2023-06-01"}
	response, err := a.postStream(ctx, messagesURL(cfg.APIBaseURL), cfg.APIKey, payload, headers)
	if err != nil {
		return GenerateResult{}, err
	}
	defer response.Body.Close()

	var content strings.Builder
	result := GenerateResult{ModelName: req.ProviderModel.Model}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		var chunk struct {
			Type  string `json:"type"`
			Model string `json:"model"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			Message struct {
				Model string `json:"model"`
				Usage struct {
					InputTokens int `json:"input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err = json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Model != "" {
			result.ModelName = chunk.Model
		}
		if chunk.Message.Model != "" {
			result.ModelName = chunk.Message.Model
		}
		if chunk.Message.Usage.InputTokens > 0 {
			result.PromptTokens = chunk.Message.Usage.InputTokens
		}
		if chunk.Usage.OutputTokens > 0 {
			result.CompletionTokens = chunk.Usage.OutputTokens
		}
		if chunk.Delta.Type != "text_delta" || chunk.Delta.Text == "" {
			continue
		}
		content.WriteString(chunk.Delta.Text)
		if onDelta != nil {
			if err = onDelta(chunk.Delta.Text); err != nil {
				return GenerateResult{}, err
			}
		}
	}
	if err = scanner.Err(); err != nil {
		return GenerateResult{}, err
	}
	result.Content = content.String()
	if strings.TrimSpace(result.Content) == "" {
		return GenerateResult{}, fmt.Errorf("model returned empty response")
	}
	if result.PromptTokens == 0 {
		result.PromptTokens = estimatePromptTokens(req, cfg)
	}
	if result.CompletionTokens == 0 {
		result.CompletionTokens = estimateTokens(result.Content)
	}
	return result, nil
}

func (a *ADKRuntimeAdapter) postJSON(ctx context.Context, endpoint string, apiKey string, payload any, out any, headers map[string]string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("x-api-key", apiKey)
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := a.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	data, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("model request failed: status %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	if err = json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("invalid model response: %w", err)
	}
	return nil
}

func (a *ADKRuntimeAdapter) postStream(ctx context.Context, endpoint string, apiKey string, payload any, headers map[string]string) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("x-api-key", apiKey)
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := a.client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return response, nil
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if readErr != nil {
		return nil, readErr
	}
	return nil, fmt.Errorf("model stream request failed: status %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
}

func buildSystemPrompt(agent domain.Agent) string {
	parts := []string{}
	if strings.TrimSpace(agent.DisplayName) != "" {
		parts = append(parts, "You are "+strings.TrimSpace(agent.DisplayName)+".")
	}
	if strings.TrimSpace(agent.AgentDescription) != "" {
		parts = append(parts, strings.TrimSpace(agent.AgentDescription))
	}
	return strings.Join(parts, "\n")
}

func normalizeChatRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant":
		return "assistant"
	case "system":
		return "system"
	default:
		return "user"
	}
}

func chatCompletionsURL(base string) string {
	return appendEndpoint(base, "/chat/completions")
}

func messagesURL(base string) string {
	return appendEndpoint(base, "/messages")
}

func appendEndpoint(base string, endpoint string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if trimmed == "" {
		return endpoint
	}
	if _, err := url.ParseRequestURI(trimmed); err != nil {
		return trimmed + endpoint
	}
	if strings.HasSuffix(trimmed, endpoint) {
		return trimmed
	}
	return trimmed + endpoint
}

func isAnthropicProvider(providerType string) bool {
	return strings.Contains(strings.ToLower(providerType), "anthropic")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func resolveMaxOutputTokens(cfg providerConfig) int {
	if cfg.MaxOutputTokens > 0 {
		if cfg.MaxOutputTokens > 32768 {
			return 32768
		}
		return cfg.MaxOutputTokens
	}
	return 4096
}

func trimMessagesByContext(messages []ChatMessage, contextWindowK int) []ChatMessage {
	if contextWindowK <= 0 {
		return messages
	}
	budget := contextWindowK * 1000
	if budget <= 0 {
		return messages
	}
	total := 0
	result := make([]ChatMessage, 0, len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		tokens := estimateTokens(messages[i].Content)
		if len(result) > 0 && total+tokens > budget {
			break
		}
		total += tokens
		result = append(result, messages[i])
	}
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

func estimatePromptTokens(req GenerateRequest, cfg providerConfig) int {
	total := estimateTokens(buildSystemPrompt(req.Agent))
	for _, message := range trimMessagesByContext(req.Messages, cfg.ContextWindowK) {
		total += estimateTokens(message.Content)
	}
	return total
}

func estimateTokens(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	tokens := len([]rune(trimmed)) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}
