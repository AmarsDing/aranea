package domain

type Agent struct {
	ID                 string                `json:"id"`
	AgentKey           string                `json:"agent_key"`
	DisplayName        string                `json:"display_name"`
	Provider           string                `json:"provider"`
	Model              string                `json:"model"`
	Status             string                `json:"status"`
	IsDefault          bool                  `json:"is_default"`
	IsFavorite         bool                  `json:"is_favorite"`
	Icon               string                `json:"icon"`
	AgentDescription   string                `json:"agent_description"`
	CategoryPositionID string                `json:"category_position_id"`
	SystemPromptMode   string                `json:"system_prompt_mode"`
	ContextWindow      int                   `json:"context_window"`
	BudgetMonthlyCents int                   `json:"budget_monthly_cents"`
	ConfigJSON         string                `json:"config_json"`
	CreatedAt          string                `json:"created_at"`
	UpdatedAt          string                `json:"updated_at"`
	DeletedAt          string                `json:"deleted_at"`
	Settings           *AgentRuntimeSettings `json:"settings,omitempty"`
	Files              []AgentPromptFile     `json:"files,omitempty"`
}

type AgentRuntimeSettings struct {
	AgentID                           string  `json:"agent_id,omitempty"`
	SelfEvolve                        bool    `json:"self_evolve"`
	SubagentsEnabled                  bool    `json:"subagents_enabled"`
	SubagentsMaxConcurrency           int     `json:"subagents_max_concurrency"`
	SubagentsMaxGenerationDepth       int     `json:"subagents_max_generation_depth"`
	SubagentsMaxChildrenPerAgent      int     `json:"subagents_max_children_per_agent"`
	SubagentsArchiveAfterMinutes      int     `json:"subagents_archive_after_minutes"`
	SubagentsMaxRetries               int     `json:"subagents_max_retries"`
	SubagentsModelOverride            string  `json:"subagents_model_override"`
	ToolsEnabled                      bool    `json:"tools_enabled"`
	ToolsProfile                      string  `json:"tools_profile"`
	ToolsToolCallPrefix               string  `json:"tools_tool_call_prefix"`
	ToolsAllowJSON                    string  `json:"tools_allow_json"`
	ToolsDenyJSON                     string  `json:"tools_deny_json"`
	ToolsConcurrentAllowJSON          string  `json:"tools_concurrent_allow_json"`
	MemoryEnabled                     bool    `json:"memory_enabled"`
	MemoryMaxChunkLength              int     `json:"memory_max_chunk_length"`
	MemoryMaxResults                  int     `json:"memory_max_results"`
	MemoryMinScore                    float64 `json:"memory_min_score"`
	HeartbeatEnabled                  bool    `json:"heartbeat_enabled"`
	HeartbeatIntervalMinutes          int     `json:"heartbeat_interval_minutes"`
	EvolutionSelfEvolve               bool    `json:"evolution_self_evolve"`
	EvolutionSkillEvolve              bool    `json:"evolution_skill_evolve"`
	EvolutionMetricsEnabled           bool    `json:"evolution_metrics_enabled"`
	EvolutionSuggestionsEnabled       bool    `json:"evolution_suggestions_enabled"`
	GuardrailMaxChangePerPeriod       float64 `json:"guardrail_max_change_per_period"`
	GuardrailMinDataPoints            int     `json:"guardrail_min_data_points"`
	GuardrailRollbackOnDeclinePercent int     `json:"guardrail_rollback_on_decline_percent"`
	CreatedAt                         string  `json:"created_at,omitempty"`
	UpdatedAt                         string  `json:"updated_at,omitempty"`
}

type AgentPromptFile struct {
	ID        string `json:"id,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	Name      string `json:"name"`
	Body      string `json:"body"`
	SortOrder int    `json:"sort_order"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type AgentListQuery struct {
	Keyword    string `json:"keyword"`
	Status     string `json:"status"`
	Provider   string `json:"provider"`
	CategoryID string `json:"category_id"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
}

type AgentListResult struct {
	Items  []Agent `json:"items"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

type Team struct {
	ID             string `json:"id"`
	TeamKey        string `json:"team_key"`
	DisplayName    string `json:"display_name"`
	Status         string `json:"status"`
	IsDefault      bool   `json:"is_default"`
	DefinitionJSON string `json:"definition_json"`
	ADKAppName     string `json:"adk_app_name"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	DeletedAt      string `json:"deleted_at"`
}

type Session struct {
	ID               string  `json:"id"`
	OwnerType        string  `json:"owner_type"`
	AgentID          string  `json:"agent_id"`
	TeamID           string  `json:"team_id"`
	Title            string  `json:"title"`
	ContextUsedRatio float64 `json:"context_used_ratio"`
	DialogMode       string  `json:"dialog_mode"`
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	Status           string  `json:"status"`
	LastMessageAt    string  `json:"last_message_at"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
	DeletedAt        string  `json:"deleted_at"`
}

type Message struct {
	ID               string `json:"id"`
	SessionID        string `json:"session_id"`
	ParentMessageID  string `json:"parent_message_id"`
	TurnIndex        int    `json:"turn_index"`
	Role             string `json:"role"`
	Content          string `json:"content_markdown"`
	ModelName        string `json:"model_name"`
	TokenIn          int    `json:"token_in"`
	TokenOut         int    `json:"token_out"`
	LatencyMS        int    `json:"latency_ms"`
	Status           string `json:"status"`
	AttachmentsCount int    `json:"attachments_count"`
	OptionsJSON      string `json:"options_json"`
	ErrorMessage     string `json:"error_message"`
	CreatedAt        string `json:"created_at"`
}

type ModelTokenUsageEvent struct {
	ID                            string  `json:"id"`
	OccurredAt                    string  `json:"occurred_at"`
	DateKey                       string  `json:"date_key"`
	HourKey                       string  `json:"hour_key"`
	WorkspaceID                   string  `json:"workspace_id"`
	UserID                        string  `json:"user_id"`
	TeamID                        string  `json:"team_id"`
	AgentID                       string  `json:"agent_id"`
	AgentKey                      string  `json:"agent_key"`
	SessionID                     string  `json:"session_id"`
	MessageID                     string  `json:"message_id"`
	RequestID                     string  `json:"request_id"`
	ProviderCode                  string  `json:"provider_code"`
	ProviderType                  string  `json:"provider_type"`
	ProviderDisplayName           string  `json:"provider_display_name"`
	ModelAPIID                    string  `json:"model_api_id"`
	ModelDisplayName              string  `json:"model_display_name"`
	ModelCategoryJSON             string  `json:"model_category_json"`
	UsageKind                     string  `json:"usage_kind"`
	CallCount                     int     `json:"call_count"`
	InputTokens                   int     `json:"input_tokens"`
	OutputTokens                  int     `json:"output_tokens"`
	CachedInputTokens             int     `json:"cached_input_tokens"`
	ReasoningTokens               int     `json:"reasoning_tokens"`
	EmbeddingTokens               int     `json:"embedding_tokens"`
	TotalTokens                   int     `json:"total_tokens"`
	InputPriceMicroUSDPer1K       int64   `json:"input_price_micro_usd_per_1k"`
	OutputPriceMicroUSDPer1K      int64   `json:"output_price_micro_usd_per_1k"`
	CachedInputPriceMicroUSDPer1K int64   `json:"cached_input_price_micro_usd_per_1k"`
	ReasoningPriceMicroUSDPer1K   int64   `json:"reasoning_price_micro_usd_per_1k"`
	EmbeddingPriceMicroUSDPer1K   int64   `json:"embedding_price_micro_usd_per_1k"`
	InputCostMicroUSD             int64   `json:"input_cost_micro_usd"`
	OutputCostMicroUSD            int64   `json:"output_cost_micro_usd"`
	CachedInputCostMicroUSD       int64   `json:"cached_input_cost_micro_usd"`
	ReasoningCostMicroUSD         int64   `json:"reasoning_cost_micro_usd"`
	EmbeddingCostMicroUSD         int64   `json:"embedding_cost_micro_usd"`
	TotalCostMicroUSD             int64   `json:"total_cost_micro_usd"`
	LatencyMS                     int     `json:"latency_ms"`
	TimeToFirstTokenMS            int     `json:"time_to_first_token_ms"`
	TokensPerSecond               float64 `json:"tokens_per_second"`
	Status                        string  `json:"status"`
	ErrorCode                     string  `json:"error_code"`
	ErrorMessage                  string  `json:"error_message"`
	RetryCount                    int     `json:"retry_count"`
	PromptMode                    string  `json:"prompt_mode"`
	MaxOutputTokens               int     `json:"max_output_tokens"`
	ContextWindowK                int     `json:"context_window_k"`
	StreamEnabled                 bool    `json:"stream_enabled"`
	MetadataJSON                  string  `json:"metadata_json"`
	CreatedAt                     string  `json:"created_at"`
}

type ModelPricingRule struct {
	ID                            string `json:"id"`
	ProviderCode                  string `json:"provider_code"`
	ModelAPIID                    string `json:"model_api_id"`
	Currency                      string `json:"currency"`
	InputPriceMicroUSDPer1K       int64  `json:"input_price_micro_usd_per_1k"`
	OutputPriceMicroUSDPer1K      int64  `json:"output_price_micro_usd_per_1k"`
	CachedInputPriceMicroUSDPer1K int64  `json:"cached_input_price_micro_usd_per_1k"`
	ReasoningPriceMicroUSDPer1K   int64  `json:"reasoning_price_micro_usd_per_1k"`
	EmbeddingPriceMicroUSDPer1K   int64  `json:"embedding_price_micro_usd_per_1k"`
	EffectiveFrom                 string `json:"effective_from"`
	EffectiveTo                   string `json:"effective_to"`
	IsActive                      bool   `json:"is_active"`
	Source                        string `json:"source"`
	MetadataJSON                  string `json:"metadata_json"`
	CreatedAt                     string `json:"created_at"`
	UpdatedAt                     string `json:"updated_at"`
}

type ModelTokenUsageDaily struct {
	ID                 string  `json:"id"`
	DateKey            string  `json:"date_key"`
	WorkspaceID        string  `json:"workspace_id"`
	AgentID            string  `json:"agent_id"`
	AgentKey           string  `json:"agent_key"`
	ProviderCode       string  `json:"provider_code"`
	ModelAPIID         string  `json:"model_api_id"`
	UsageKind          string  `json:"usage_kind"`
	CallCount          int     `json:"call_count"`
	RequestCount       int     `json:"request_count"`
	SuccessCount       int     `json:"success_count"`
	FailedCount        int     `json:"failed_count"`
	CancelledCount     int     `json:"cancelled_count"`
	InputTokens        int     `json:"input_tokens"`
	OutputTokens       int     `json:"output_tokens"`
	CachedInputTokens  int     `json:"cached_input_tokens"`
	ReasoningTokens    int     `json:"reasoning_tokens"`
	EmbeddingTokens    int     `json:"embedding_tokens"`
	TotalTokens        int     `json:"total_tokens"`
	TotalCostMicroUSD  int64   `json:"total_cost_micro_usd"`
	AvgLatencyMS       float64 `json:"avg_latency_ms"`
	AvgTokensPerSecond float64 `json:"avg_tokens_per_second"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

type ModelUsageQuery struct {
	Range        string `json:"range"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	ProviderCode string `json:"provider_code"`
	ModelAPIID   string `json:"model_api_id"`
	AgentID      string `json:"agent_id"`
	Status       string `json:"status"`
	Limit        int    `json:"limit"`
}

type ModelUsageSummary struct {
	CallCount          int     `json:"call_count"`
	RequestCount       int     `json:"request_count"`
	SuccessCount       int     `json:"success_count"`
	FailedCount        int     `json:"failed_count"`
	CancelledCount     int     `json:"cancelled_count"`
	InputTokens        int     `json:"input_tokens"`
	OutputTokens       int     `json:"output_tokens"`
	TotalTokens        int     `json:"total_tokens"`
	TotalCostMicroUSD  int64   `json:"total_cost_micro_usd"`
	AvgLatencyMS       float64 `json:"avg_latency_ms"`
	AvgTokensPerSecond float64 `json:"avg_tokens_per_second"`
	SuccessRate        float64 `json:"success_rate"`
}

type ModelUsageTrendPoint struct {
	DateKey            string  `json:"date_key"`
	CallCount          int     `json:"call_count"`
	InputTokens        int     `json:"input_tokens"`
	OutputTokens       int     `json:"output_tokens"`
	TotalTokens        int     `json:"total_tokens"`
	TotalCostMicroUSD  int64   `json:"total_cost_micro_usd"`
	SuccessCount       int     `json:"success_count"`
	FailedCount        int     `json:"failed_count"`
	CancelledCount     int     `json:"cancelled_count"`
	AvgLatencyMS       float64 `json:"avg_latency_ms"`
	AvgTokensPerSecond float64 `json:"avg_tokens_per_second"`
}

type ModelUsageBreakdownRow struct {
	ProviderCode       string  `json:"provider_code"`
	ModelAPIID         string  `json:"model_api_id"`
	ModelDisplayName   string  `json:"model_display_name"`
	AgentID            string  `json:"agent_id"`
	AgentKey           string  `json:"agent_key"`
	CallCount          int     `json:"call_count"`
	InputTokens        int     `json:"input_tokens"`
	OutputTokens       int     `json:"output_tokens"`
	TotalTokens        int     `json:"total_tokens"`
	TotalCostMicroUSD  int64   `json:"total_cost_micro_usd"`
	AvgLatencyMS       float64 `json:"avg_latency_ms"`
	AvgTokensPerSecond float64 `json:"avg_tokens_per_second"`
	SuccessRate        float64 `json:"success_rate"`
}

type ModelUsageOverview struct {
	Today     ModelUsageSummary        `json:"today"`
	Yesterday ModelUsageSummary        `json:"yesterday"`
	Month     ModelUsageSummary        `json:"month"`
	Range     ModelUsageSummary        `json:"range"`
	Trends    []ModelUsageTrendPoint   `json:"trends"`
	TopModels []ModelUsageBreakdownRow `json:"top_models"`
	TopAgents []ModelUsageBreakdownRow `json:"top_agents"`
	Anomalies []ModelTokenUsageEvent   `json:"anomalies"`
}

type ChatAttachment struct {
	ID           string `json:"id"`
	SessionID    string `json:"session_id"`
	MessageID    string `json:"message_id"`
	FileName     string `json:"file_name"`
	MimeType     string `json:"mime_type"`
	SizeBytes    int64  `json:"size_bytes"`
	StorageKey   string `json:"storage_key"`
	Checksum     string `json:"checksum"`
	UploadStatus string `json:"upload_status"`
	CreatedAt    string `json:"created_at"`
	DeletedAt    string `json:"deleted_at"`
}

type ChatOption struct {
	Type         string `json:"type"`
	Key          string `json:"key"`
	Label        string `json:"label"`
	Enabled      bool   `json:"enabled"`
	SortOrder    int    `json:"sort_order"`
	MetadataJSON string `json:"metadata_json"`
}

type PlatformResource struct {
	ID           string `json:"id"`
	Resource     string `json:"resource"`
	Key          string `json:"key"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	Enabled      bool   `json:"enabled"`
	SortOrder    int    `json:"sort_order"`
	ParentID     string `json:"parent_id"`
	Level        string `json:"level"`
	AgentID      string `json:"agent_id"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	ConfigJSON   string `json:"config_json"`
	MetadataJSON string `json:"metadata_json"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	DeletedAt    string `json:"deleted_at"`
}

type PlatformResourceTreeNode struct {
	PlatformResource
	Children []PlatformResourceTreeNode `json:"children"`
}

type SkillTag struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type SkillVersionSummary struct {
	ID               string `json:"id"`
	Version          string `json:"version"`
	ValidationStatus string `json:"validation_status"`
	PublishedAt      string `json:"published_at"`
}

type SkillPermissions struct {
	CanEdit          bool `json:"can_edit"`
	CanDelete        bool `json:"can_delete"`
	CanToggleEnabled bool `json:"can_toggle_enabled"`
	CanDuplicate     bool `json:"can_duplicate"`
}

type Skill struct {
	ID                   string               `json:"id"`
	Name                 string               `json:"name"`
	Slug                 string               `json:"slug"`
	Description          string               `json:"description"`
	Tags                 []SkillTag           `json:"tags"`
	ExtendsSkillID       string               `json:"extends_skill_id,omitempty"`
	Status               string               `json:"status"`
	Enabled              bool                 `json:"enabled"`
	CurrentVersion       *SkillVersionSummary `json:"current_version"`
	InvokeCount          int                  `json:"invoke_count"`
	SuccessCount         int                  `json:"success_count"`
	FailureCount         int                  `json:"failure_count"`
	UsageCount7d         int                  `json:"usage_count_7d"`
	AvgDurationMS        *float64             `json:"avg_duration_ms"`
	LastAgentID          string               `json:"last_agent_id,omitempty"`
	LastAgentDisplayName string               `json:"last_agent_display_name,omitempty"`
	LastInvokedAt        string               `json:"last_invoked_at,omitempty"`
	LastDurationMS       *int                 `json:"last_duration_ms"`
	CreatedAt            string               `json:"created_at"`
	UpdatedAt            string               `json:"updated_at"`
	Permissions          SkillPermissions     `json:"permissions"`
}

type SkillListQuery struct {
	Search  string `json:"search"`
	Tags    string `json:"tags"`
	Enabled string `json:"enabled"`
	Status  string `json:"status"`
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
}

type SkillListResult struct {
	Items  []Skill `json:"items"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

type SkillInvocationPermissions struct {
	CanViewDetail bool `json:"can_view_detail"`
}

type SkillInvocation struct {
	ID               string                     `json:"id"`
	SkillID          string                     `json:"skill_id"`
	SkillName        string                     `json:"skill_name"`
	SkillVersion     string                     `json:"skill_version"`
	AgentID          string                     `json:"agent_id"`
	AgentDisplayName string                     `json:"agent_display_name"`
	UserID           string                     `json:"user_id,omitempty"`
	SessionID        string                     `json:"session_id,omitempty"`
	Status           string                     `json:"status"`
	DurationMS       int                        `json:"duration_ms"`
	StartedAt        string                     `json:"started_at"`
	EndedAt          string                     `json:"ended_at,omitempty"`
	InputPreview     string                     `json:"input_preview,omitempty"`
	InputHash        string                     `json:"input_hash,omitempty"`
	OutputPreview    string                     `json:"output_preview,omitempty"`
	ErrorCode        string                     `json:"error_code,omitempty"`
	ErrorMessage     string                     `json:"error_message,omitempty"`
	Permissions      SkillInvocationPermissions `json:"permissions"`
}

type SkillRunQuery struct {
	SkillID string `json:"skill_id"`
	AgentID string `json:"agent_id"`
	Status  string `json:"status"`
	From    string `json:"from"`
	To      string `json:"to"`
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
}

type SkillRunResult struct {
	Items  []SkillInvocation `json:"items"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

type SkillImportJob struct {
	JobID            string                 `json:"job_id"`
	Status           string                 `json:"status"`
	ValidationStatus string                 `json:"validation_status"`
	StorageRoot      string                 `json:"storage_root"`
	Candidates       []SkillImportCandidate `json:"candidates"`
	ConflictGroups   []SkillConflictGroup   `json:"conflict_groups"`
	Message          string                 `json:"message,omitempty"`
}

type SkillImportCandidate struct {
	CandidateID      string             `json:"candidate_id"`
	Name             string             `json:"name"`
	Slug             string             `json:"slug"`
	Description      string             `json:"description"`
	BodyPreview      string             `json:"body_preview"`
	TargetDir        string             `json:"target_dir"`
	ValidationStatus string             `json:"validation_status"`
	StatusIcon       string             `json:"status_icon"`
	Warnings         []SkillImportIssue `json:"warnings"`
	Blocks           []SkillImportIssue `json:"blocks"`
}

type SkillImportIssue struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type SkillSimilarityMetrics struct {
	SimilarityScore       float64 `json:"similarity_score"`
	NameSimilarity        float64 `json:"name_similarity"`
	DescriptionSimilarity float64 `json:"description_similarity"`
	BodySimilarity        float64 `json:"body_similarity"`
	TriggerSimilarity     float64 `json:"trigger_similarity"`
	ToolSimilarity        float64 `json:"tool_similarity"`
	ConflictRisk          string  `json:"conflict_risk"`
	Recommendation        string  `json:"recommendation"`
	Confidence            float64 `json:"confidence"`
}

type SkillConflictGroup struct {
	GroupID                string                  `json:"group_id"`
	HighestSimilarityScore float64                 `json:"highest_similarity_score"`
	Metrics                SkillSimilarityMetrics  `json:"metrics"`
	Reason                 string                  `json:"reason"`
	Evidence               []string                `json:"evidence"`
	CandidateIDs           []string                `json:"candidate_ids"`
	ExistingSkills         []SkillSimilaritySource `json:"existing_skills"`
	CanRefine              bool                    `json:"can_refine"`
}

type SkillSimilaritySource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Version     string `json:"version"`
	BodyPreview string `json:"body_preview"`
	Body        string `json:"-"`
}

type SkillRefineRequest struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Instructions string `json:"instructions"`
}

type SkillRefineResult struct {
	MergedName             string     `json:"merged_name"`
	MergedDescription      string     `json:"merged_description"`
	MergedBody             string     `json:"merged_body"`
	MergedTags             []SkillTag `json:"merged_tags"`
	SourceCandidateIDs     []string   `json:"source_candidate_ids"`
	SourceExistingSkillIDs []string   `json:"source_existing_skill_ids"`
}

type SkillImportDecision struct {
	CandidateID       string     `json:"candidate_id,omitempty"`
	GroupID           string     `json:"group_id,omitempty"`
	Action            string     `json:"action"`
	MergedName        string     `json:"merged_name,omitempty"`
	MergedDescription string     `json:"merged_description,omitempty"`
	MergedBody        string     `json:"merged_body,omitempty"`
	MergedTags        []SkillTag `json:"merged_tags,omitempty"`
}

type SkillImportApplyRequest struct {
	Decisions []SkillImportDecision `json:"decisions"`
}

type SkillImportApplyResult struct {
	CreatedSkillIDs     []string `json:"created_skill_ids"`
	SkippedCandidateIDs []string `json:"skipped_candidate_ids"`
	Message             string   `json:"message"`
}

type SkillCreateInput struct {
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Description string     `json:"description"`
	Body        string     `json:"body"`
	Tags        []SkillTag `json:"tags"`
	StorageDir  string     `json:"storage_dir"`
}

type SkillFile struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Language  string `json:"language"`
	Size      int64  `json:"size"`
	UpdatedAt string `json:"updated_at"`
}

type SkillFileContent struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Language string `json:"language"`
}

type SkillFileUpdateInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type AvatarAsset struct {
	ID            string `json:"id"`
	Key           string `json:"key"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	MimeType      string `json:"mime_type"`
	WorkspaceID   string `json:"workspace_id"`
	OwnerUserID   string `json:"owner_user_id"`
	Source        string `json:"source"`
	IsSystem      bool   `json:"is_system"`
	FileSizeBytes int    `json:"file_size_bytes"`
	WidthPx       int    `json:"width_px"`
	HeightPx      int    `json:"height_px"`
	SortOrder     int    `json:"sort_order"`
	CreatedAt     string `json:"created_at"`
}

type AvatarImage struct {
	ID       string
	MimeType string
	Data     []byte
}

type ValidateModelInput struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type ValidateModelResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type InspectProviderModelInput struct {
	ResourceID   string `json:"resource_id"`
	ProviderCode string `json:"provider_code"`
	ProviderType string `json:"provider_type"`
	ModelAPIID   string `json:"model_api_id"`
	APIBaseURL   string `json:"api_base_url"`
	APIKey       string `json:"api_key"`
}

type InspectProviderModelResult struct {
	OK                            bool   `json:"ok"`
	Message                       string `json:"message"`
	ProviderCode                  string `json:"provider_code"`
	ProviderType                  string `json:"provider_type"`
	ModelAPIID                    string `json:"model_api_id"`
	ModelDisplayName              string `json:"model_display_name"`
	ModelSizeLabel                string `json:"model_size_label"`
	ContextWindowK                int    `json:"context_window_k"`
	MaxOutputTokens               int    `json:"max_output_tokens"`
	InputPriceMicroUSDPer1K       int64  `json:"input_price_micro_usd_per_1k"`
	OutputPriceMicroUSDPer1K      int64  `json:"output_price_micro_usd_per_1k"`
	CachedInputPriceMicroUSDPer1K int64  `json:"cached_input_price_micro_usd_per_1k"`
	ReasoningPriceMicroUSDPer1K   int64  `json:"reasoning_price_micro_usd_per_1k"`
	EmbeddingPriceMicroUSDPer1K   int64  `json:"embedding_price_micro_usd_per_1k"`
	Source                        string `json:"source"`
	RawMetadataJSON               string `json:"raw_metadata_json"`
}

type AuditLog struct {
	ID         string `json:"id"`
	Action     string `json:"action"`
	Resource   string `json:"resource"`
	ResourceID string `json:"resource_id"`
	RequestID  string `json:"request_id"`
	Detail     string `json:"detail"`
	CreatedAt  string `json:"created_at"`
}
