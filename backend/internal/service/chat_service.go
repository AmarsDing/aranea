package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
	"arenea/backend/internal/runtime"
)

type ChatService struct {
	repo          repository.Store
	runtime       *runtime.ADKRuntimeAdapter
	teamRunEvents *TeamRunEventBroker
}

type SendMessageInput struct {
	SessionID string             `json:"session_id"`
	AgentKey  string             `json:"agent_key"`
	TeamID    string             `json:"team_id"`
	Content   string             `json:"content"`
	Options   SendMessageOptions `json:"options"`
}

type SendMessageOptions struct {
	DialogMode  string               `json:"dialog_mode"`
	Provider    string               `json:"provider"`
	Model       string               `json:"model"`
	Attachments []AttachmentRefInput `json:"attachments"`
}

type AttachmentRefInput struct {
	ID string `json:"id"`
}

type SendMessageResult struct {
	UserMessage  domain.Message `json:"user_message"`
	AgentMessage domain.Message `json:"agent_message"`
}

type SendStreamCallbacks struct {
	OnUserMessage  func(domain.Message) error
	OnDelta        func(string) error
	OnAgentMessage func(domain.Message) error
}

func NewChatService(repo repository.Store, runtimeAdapter *runtime.ADKRuntimeAdapter) *ChatService {
	return &ChatService{repo: repo, runtime: runtimeAdapter, teamRunEvents: NewTeamRunEventBroker()}
}

func (s *ChatService) Send(ctx context.Context, in SendMessageInput) (SendMessageResult, error) {
	if in.SessionID == "" || in.Content == "" {
		return SendMessageResult{}, validationError("session_id and content are required")
	}
	session, err := s.repo.GetSessionByID(in.SessionID)
	if err != nil {
		return SendMessageResult{}, err
	}
	if session.OwnerType == "team" {
		return s.sendTeam(ctx, in, session, nil)
	}
	if in.AgentKey == "" {
		return SendMessageResult{}, validationError("agent_key is required")
	}
	agent, err := s.repo.GetAgentByKey(in.AgentKey)
	if err != nil {
		return SendMessageResult{}, err
	}
	if session.OwnerType != "" && session.OwnerType != "agent" {
		return SendMessageResult{}, validationError("chat send currently requires an agent-owned session")
	}
	if session.AgentID != agent.ID {
		return SendMessageResult{}, conflictError("session does not belong to agent")
	}
	provider, model := resolveProviderModel(in.Options, session, agent)
	providerModel, err := s.repo.GetProviderModel(provider, model)
	if err != nil {
		return SendMessageResult{}, err
	}
	history, err := s.repo.ListMessages(in.SessionID)
	if err != nil {
		return SendMessageResult{}, err
	}
	optionsJSON := ""
	if in.Options.DialogMode != "" || in.Options.Provider != "" || in.Options.Model != "" || len(in.Options.Attachments) > 0 {
		raw, err := json.Marshal(in.Options)
		if err != nil {
			return SendMessageResult{}, err
		}
		optionsJSON = string(raw)
	}

	userMsg := domain.Message{
		ID:               newID(),
		SessionID:        in.SessionID,
		Role:             "user",
		Content:          in.Content,
		ModelName:        "",
		Status:           "ok",
		AttachmentsCount: len(in.Options.Attachments),
		OptionsJSON:      optionsJSON,
	}
	userMsg, err = s.repo.AddMessage(userMsg)
	if err != nil {
		return SendMessageResult{}, err
	}

	modelMessages := make([]runtime.ChatMessage, 0, len(history)+1)
	for _, item := range history {
		if item.Role != "user" && item.Role != "assistant" {
			continue
		}
		modelMessages = append(modelMessages, runtime.ChatMessage{Role: item.Role, Content: item.Content})
	}
	modelMessages = append(modelMessages, runtime.ChatMessage{Role: "user", Content: in.Content})

	generated, err := s.runtime.Generate(ctx, runtime.GenerateRequest{
		Agent:         agent,
		ProviderModel: providerModel,
		Messages:      modelMessages,
		Input:         in.Content,
	})
	if err != nil {
		_ = s.recordModelTokenUsage(agent, session, providerModel, in.Options, runtime.GenerateResult{}, domain.Message{}, false, "failed", err)
		return SendMessageResult{}, err
	}

	agentMsg := domain.Message{
		ID:        newID(),
		SessionID: in.SessionID,
		Role:      "assistant",
		Content:   generated.Content,
		ModelName: generated.ModelName,
		TokenIn:   generated.PromptTokens,
		TokenOut:  generated.CompletionTokens,
		LatencyMS: generated.LatencyMS,
		Status:    "ok",
	}
	agentMsg, err = s.repo.AddMessage(agentMsg)
	if err != nil {
		return SendMessageResult{}, err
	}
	_ = s.recordModelTokenUsage(agent, session, providerModel, in.Options, generated, agentMsg, false, "success", nil)
	_ = s.updateSessionContextRatio(in.SessionID, agent, providerModel, generated)
	_ = s.recordProviderModelTPS(providerModel, generated)

	return SendMessageResult{
		UserMessage:  userMsg,
		AgentMessage: agentMsg,
	}, nil
}

func (s *ChatService) SendStream(ctx context.Context, in SendMessageInput, callbacks SendStreamCallbacks) error {
	if in.SessionID == "" || in.Content == "" {
		return validationError("session_id and content are required")
	}
	session, err := s.repo.GetSessionByID(in.SessionID)
	if err != nil {
		return err
	}
	if session.OwnerType == "team" {
		_, err = s.sendTeam(ctx, in, session, &callbacks)
		return err
	}
	if in.AgentKey == "" {
		return validationError("agent_key is required")
	}
	agent, err := s.repo.GetAgentByKey(in.AgentKey)
	if err != nil {
		return err
	}
	if session.OwnerType != "" && session.OwnerType != "agent" {
		return validationError("chat send currently requires an agent-owned session")
	}
	if session.AgentID != agent.ID {
		return conflictError("session does not belong to agent")
	}
	provider, model := resolveProviderModel(in.Options, session, agent)
	providerModel, err := s.repo.GetProviderModel(provider, model)
	if err != nil {
		return err
	}
	history, err := s.repo.ListMessages(in.SessionID)
	if err != nil {
		return err
	}
	optionsJSON := ""
	if in.Options.DialogMode != "" || in.Options.Provider != "" || in.Options.Model != "" || len(in.Options.Attachments) > 0 {
		raw, err := json.Marshal(in.Options)
		if err != nil {
			return err
		}
		optionsJSON = string(raw)
	}

	userMsg := domain.Message{
		ID:               newID(),
		SessionID:        in.SessionID,
		Role:             "user",
		Content:          in.Content,
		Status:           "ok",
		AttachmentsCount: len(in.Options.Attachments),
		OptionsJSON:      optionsJSON,
	}
	userMsg, err = s.repo.AddMessage(userMsg)
	if err != nil {
		return err
	}
	if callbacks.OnUserMessage != nil {
		if err = callbacks.OnUserMessage(userMsg); err != nil {
			return err
		}
	}

	modelMessages := make([]runtime.ChatMessage, 0, len(history)+1)
	for _, item := range history {
		if item.Role != "user" && item.Role != "assistant" {
			continue
		}
		modelMessages = append(modelMessages, runtime.ChatMessage{Role: item.Role, Content: item.Content})
	}
	modelMessages = append(modelMessages, runtime.ChatMessage{Role: "user", Content: in.Content})

	generated, err := s.runtime.StreamGenerate(ctx, runtime.GenerateRequest{
		Agent:         agent,
		ProviderModel: providerModel,
		Messages:      modelMessages,
		Input:         in.Content,
	}, callbacks.OnDelta)
	if err != nil {
		status := "failed"
		if errors.Is(err, context.Canceled) {
			status = "cancelled"
		}
		_ = s.recordModelTokenUsage(agent, session, providerModel, in.Options, runtime.GenerateResult{}, domain.Message{}, true, status, err)
		return err
	}

	agentMsg := domain.Message{
		ID:        newID(),
		SessionID: in.SessionID,
		Role:      "assistant",
		Content:   generated.Content,
		ModelName: generated.ModelName,
		TokenIn:   generated.PromptTokens,
		TokenOut:  generated.CompletionTokens,
		LatencyMS: generated.LatencyMS,
		Status:    "ok",
	}
	agentMsg, err = s.repo.AddMessage(agentMsg)
	if err != nil {
		return err
	}
	_ = s.recordModelTokenUsage(agent, session, providerModel, in.Options, generated, agentMsg, true, "success", nil)
	_ = s.updateSessionContextRatio(in.SessionID, agent, providerModel, generated)
	_ = s.recordProviderModelTPS(providerModel, generated)
	if callbacks.OnAgentMessage != nil {
		return callbacks.OnAgentMessage(agentMsg)
	}
	return nil
}

func resolveProviderModel(options SendMessageOptions, session domain.Session, agent domain.Agent) (string, string) {
	if options.Provider != "" && options.Model != "" {
		return options.Provider, options.Model
	}
	if session.Provider != "" && session.Model != "" {
		return session.Provider, session.Model
	}
	return agent.Provider, agent.Model
}

func (s *ChatService) updateSessionContextRatio(sessionID string, agent domain.Agent, providerModel domain.PlatformResource, generated runtime.GenerateResult) error {
	contextTokens := providerContextWindowTokens(providerModel, agent)
	if sessionID == "" || contextTokens <= 0 || generated.PromptTokens <= 0 {
		return nil
	}
	ratio := float64(generated.PromptTokens) / float64(contextTokens)
	if ratio < 0 || math.IsInf(ratio, 0) || math.IsNaN(ratio) {
		return nil
	}
	if ratio > 1 {
		ratio = 1
	}
	return s.repo.UpdateSessionContextUsedRatio(sessionID, ratio)
}

func providerContextWindowTokens(providerModel domain.PlatformResource, agent domain.Agent) int {
	type config struct {
		ContextWindowK int `json:"context_window_k"`
	}
	var cfg config
	if providerModel.ConfigJSON != "" {
		_ = json.Unmarshal([]byte(providerModel.ConfigJSON), &cfg)
	}
	if cfg.ContextWindowK > 0 {
		return cfg.ContextWindowK * 1000
	}
	return agent.ContextWindow
}

func (s *ChatService) recordModelTokenUsage(agent domain.Agent, session domain.Session, providerModel domain.PlatformResource, options SendMessageOptions, generated runtime.GenerateResult, message domain.Message, streamEnabled bool, status string, callErr error) error {
	cfg := parseUsageProviderConfig(providerModel.ConfigJSON)
	occurredAt := nowUTC()
	pricing, err := s.repo.GetActiveModelPricingRule(providerModel.Provider, providerModel.Model, occurredAt)
	if err != nil {
		return err
	}

	inputTokens := generated.PromptTokens
	outputTokens := generated.CompletionTokens
	totalTokens := inputTokens + outputTokens
	inputCost := costMicroUSD(inputTokens, pricing.InputPriceMicroUSDPer1K)
	outputCost := costMicroUSD(outputTokens, pricing.OutputPriceMicroUSDPer1K)
	tps := 0.0
	if generated.CompletionTokens > 0 && generated.LatencyMS > 0 {
		tps = math.Round((float64(generated.CompletionTokens)/(float64(generated.LatencyMS)/1000))*100) / 100
	}
	errorMessage := ""
	if callErr != nil {
		errorMessage = callErr.Error()
	}
	if status == "" {
		status = "success"
	}
	if status == "failed" && errors.Is(callErr, context.DeadlineExceeded) {
		status = "timeout"
	}

	event := domain.ModelTokenUsageEvent{
		ID:                            newID(),
		OccurredAt:                    occurredAt,
		DateKey:                       occurredAt[:10],
		HourKey:                       occurredAt[:13] + ":00",
		TeamID:                        session.TeamID,
		AgentID:                       agent.ID,
		AgentKey:                      agent.AgentKey,
		SessionID:                     session.ID,
		MessageID:                     message.ID,
		ProviderCode:                  providerModel.Provider,
		ProviderType:                  cfg.ProviderType,
		ProviderDisplayName:           firstNonEmptyString(cfg.ProviderDisplayName, providerModel.Provider),
		ModelAPIID:                    providerModel.Model,
		ModelDisplayName:              firstNonEmptyString(providerModel.Name, providerModel.Model),
		ModelCategoryJSON:             cfg.ModelCategoryJSON,
		UsageKind:                     "chat",
		CallCount:                     1,
		InputTokens:                   inputTokens,
		OutputTokens:                  outputTokens,
		TotalTokens:                   totalTokens,
		InputPriceMicroUSDPer1K:       pricing.InputPriceMicroUSDPer1K,
		OutputPriceMicroUSDPer1K:      pricing.OutputPriceMicroUSDPer1K,
		InputCostMicroUSD:             inputCost,
		OutputCostMicroUSD:            outputCost,
		TotalCostMicroUSD:             inputCost + outputCost,
		LatencyMS:                     generated.LatencyMS,
		TokensPerSecond:               tps,
		Status:                        status,
		ErrorMessage:                  errorMessage,
		PromptMode:                    options.DialogMode,
		MaxOutputTokens:               cfg.MaxOutputTokens,
		ContextWindowK:                cfg.ContextWindowK,
		StreamEnabled:                 streamEnabled,
		MetadataJSON:                  "{}",
		CreatedAt:                     occurredAt,
		CachedInputPriceMicroUSDPer1K: pricing.CachedInputPriceMicroUSDPer1K,
		ReasoningPriceMicroUSDPer1K:   pricing.ReasoningPriceMicroUSDPer1K,
		EmbeddingPriceMicroUSDPer1K:   pricing.EmbeddingPriceMicroUSDPer1K,
	}
	event, err = s.repo.AddModelTokenUsageEvent(event)
	if err != nil {
		return err
	}
	return s.repo.UpsertModelTokenUsageDaily(event)
}

type usageProviderConfig struct {
	ProviderType        string `json:"provider_type"`
	ProviderDisplayName string `json:"provider_display_name"`
	ModelCategoryJSON   string
	ContextWindowK      int `json:"context_window_k"`
	MaxOutputTokens     int `json:"max_output_tokens"`
}

func parseUsageProviderConfig(raw string) usageProviderConfig {
	var body map[string]any
	cfg := usageProviderConfig{ModelCategoryJSON: "[]"}
	if raw == "" {
		return cfg
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return cfg
	}
	if value, ok := body["provider_type"].(string); ok {
		cfg.ProviderType = value
	}
	if value, ok := body["provider_display_name"].(string); ok {
		cfg.ProviderDisplayName = value
	}
	if value, ok := body["context_window_k"].(float64); ok {
		cfg.ContextWindowK = int(value)
	}
	if value, ok := body["max_output_tokens"].(float64); ok {
		cfg.MaxOutputTokens = int(value)
	}
	if value, ok := body["model_category"]; ok {
		if rawCategory, err := json.Marshal(value); err == nil {
			cfg.ModelCategoryJSON = string(rawCategory)
		}
	}
	return cfg
}

func costMicroUSD(tokens int, priceMicroUSDPer1K int64) int64 {
	if tokens <= 0 || priceMicroUSDPer1K <= 0 {
		return 0
	}
	return int64(tokens) * priceMicroUSDPer1K / 1000
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func previewText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}

func (s *ChatService) recordProviderModelTPS(providerModel domain.PlatformResource, generated runtime.GenerateResult) error {
	if providerModel.ID == "" || generated.CompletionTokens <= 0 || generated.LatencyMS <= 0 {
		return nil
	}
	tps := float64(generated.CompletionTokens) / (float64(generated.LatencyMS) / 1000)
	if tps <= 0 || math.IsInf(tps, 0) || math.IsNaN(tps) {
		return nil
	}

	config := map[string]any{}
	if providerModel.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(providerModel.ConfigJSON), &config); err != nil {
			return err
		}
	}
	config["tokens_per_second"] = math.Round(tps*100) / 100
	raw, err := json.Marshal(config)
	if err != nil {
		return err
	}
	providerModel.ConfigJSON = string(raw)
	_, err = s.repo.UpdatePlatformResource(providerModel)
	return err
}

func (s *ChatService) ListMessages(sessionID string) ([]domain.Message, error) {
	return s.repo.ListMessages(sessionID)
}

func (s *ChatService) ListOptions(optionType string) ([]domain.ChatOption, error) {
	return s.repo.ListChatOptions(optionType)
}
