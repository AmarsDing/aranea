package domain

// L0Settings captures the agent-level configuration that drives the L0
// (sensory / context-window) memory layer described in `12 memory-L0-sensory.md`.
// All knobs are persisted on agent_runtime_settings so they live alongside the
// existing memory_* and tools_* knobs.
type L0Settings struct {
	RecentWindowTurns  int     `json:"recent_window_turns"`
	RecentWindowTokens int     `json:"recent_window_tokens"`
	SummaryThreshold   float64 `json:"summary_threshold"`
	SummaryKeepTurns   int     `json:"summary_keep_turns"`
	TruncateStrategy   string  `json:"truncate_strategy"`
	InjectL1           bool    `json:"inject_l1"`
	InjectL3           bool    `json:"inject_l3"`
	InjectL4           bool    `json:"inject_l4"`
	L3MaxChunks        int     `json:"l3_max_chunks"`
	L4MaxPaths         int     `json:"l4_max_paths"`
	SnapshotMode       string  `json:"snapshot_mode"`
}

// L0Segment is one block in the assembled prompt. The content is what is fed
// into the model; preview is what is persisted for debugging / audit.
type L0Segment struct {
	Section string `json:"section"`
	Role    string `json:"role"`
	Source  string `json:"source"`
	Tokens  int    `json:"tokens"`
	Content string `json:"content,omitempty"`
	Preview string `json:"preview"`
}

// L0AssemblyRequest is the input from ChatService / TeamRuntime when building
// a model prompt. ContextWindow / ReservedForOutput are taken from the chosen
// provider model so the budget is computed per call.
type L0AssemblyRequest struct {
	SessionID         string      `json:"session_id"`
	RunID             string      `json:"run_id,omitempty"`
	TurnID            string      `json:"turn_id,omitempty"`
	SpanID            string      `json:"span_id,omitempty"`
	AgentID           string      `json:"agent_id,omitempty"`
	TeamID            string      `json:"team_id,omitempty"`
	UserID            string      `json:"user_id,omitempty"`
	WorkspaceID       string      `json:"workspace_id,omitempty"`
	Provider          string      `json:"provider,omitempty"`
	Model             string      `json:"model,omitempty"`
	ContextWindow     int         `json:"context_window,omitempty"`
	ReservedForOutput int         `json:"reserved_for_output,omitempty"`
	UserMessage       string      `json:"user_message"`
	UserMessageID     string      `json:"user_message_id,omitempty"`
	ExtraSystemBlocks []L0Segment `json:"extra_system_blocks,omitempty"`
}

// L0MemoryScopeContext carries the caller's visible memory scopes from L0
// into L3 / L4 recall. Older call sites only supplied session_id + agent_id,
// which meant user/team/workspace recall scopes in runtime settings could not
// participate in the main chat assembly path.
type L0MemoryScopeContext struct {
	SessionID   string `json:"session_id,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	TeamID      string `json:"team_id,omitempty"`
	UserID      string `json:"user_id,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Query       string `json:"query,omitempty"`
}

// L0ChatMessage is the role / content tuple given back to ChatService so it
// can be passed straight to the runtime adapter without leaking layout details.
type L0ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// L0AssemblyResult is what MemoryL0Service.Assemble returns. PromptMessages is
// the final array fed to the model; the rest is metadata used by snapshots,
// trace spans and the prompt debugger UI.
type L0AssemblyResult struct {
	Segments              []L0Segment     `json:"segments"`
	PromptMessages        []L0ChatMessage `json:"prompt_messages"`
	BudgetTokens          int             `json:"budget_tokens"`
	PromptTokenEstimate   int             `json:"prompt_token_estimate"`
	UsedRatioEstimate     float64         `json:"used_ratio_estimate"`
	RecentWindowTurns     int             `json:"recent_window_turns"`
	RecentWindowTokens    int             `json:"recent_window_tokens"`
	SummarizedTurnFrom    int             `json:"summarized_turn_from"`
	SummarizedTurnTo      int             `json:"summarized_turn_to"`
	TruncateStrategy      string          `json:"truncate_strategy"`
	TruncatedMessageCount int             `json:"truncated_message_count"`
	WarningCodes          []string        `json:"warning_codes"`
	SnapshotID            string          `json:"snapshot_id,omitempty"`
}

// L0AssemblySnapshot mirrors a row in memory_l0_assembly_snapshots. It is the
// auditable record of one L0 assembly so operators / agent-evolution analytics
// can replay how a prompt was constructed.
type L0AssemblySnapshot struct {
	ID                    string  `json:"id"`
	SessionID             string  `json:"session_id"`
	RunID                 string  `json:"run_id"`
	TurnID                string  `json:"turn_id"`
	SpanID                string  `json:"span_id"`
	AgentID               string  `json:"agent_id"`
	TeamID                string  `json:"team_id"`
	Provider              string  `json:"provider"`
	Model                 string  `json:"model"`
	ContextWindowTokens   int     `json:"context_window_tokens"`
	BudgetTokens          int     `json:"budget_tokens"`
	RecentWindowTurns     int     `json:"recent_window_turns"`
	RecentWindowTokens    int     `json:"recent_window_tokens"`
	SummaryTokenEstimate  int     `json:"summary_token_estimate"`
	L1FieldCount          int     `json:"l1_field_count"`
	L1TokenEstimate       int     `json:"l1_token_estimate"`
	L3ChunkCount          int     `json:"l3_chunk_count"`
	L3TokenEstimate       int     `json:"l3_token_estimate"`
	L4PathCount           int     `json:"l4_path_count"`
	L4TokenEstimate       int     `json:"l4_token_estimate"`
	PromptTokenEstimate   int     `json:"prompt_token_estimate"`
	PromptTokenActual     int     `json:"prompt_token_actual"`
	UsedRatio             float64 `json:"used_ratio"`
	TruncateStrategy      string  `json:"truncate_strategy"`
	TruncatedMessageCount int     `json:"truncated_message_count"`
	SummarizedTurnFrom    int     `json:"summarized_turn_from"`
	SummarizedTurnTo      int     `json:"summarized_turn_to"`
	SegmentsJSON          string  `json:"segments_json"`
	WarningCodesJSON      string  `json:"warning_codes_json"`
	MetadataJSON          string  `json:"metadata_json"`
	CreatedAt             string  `json:"created_at"`
}

// SessionSummary is a condensed transcript of `from_turn..to_turn` written by
// SummaryService and consumed by L0 to keep older history within budget.
type SessionSummary struct {
	ID              string `json:"id"`
	SessionID       string `json:"session_id"`
	SummaryMarkdown string `json:"summary_markdown"`
	FromTurn        int    `json:"from_turn"`
	ToTurn          int    `json:"to_turn"`
	TokenEstimate   int    `json:"token_estimate"`
	CreatedAt       string `json:"created_at"`
}
