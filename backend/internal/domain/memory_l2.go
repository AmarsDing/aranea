// Package domain – memory L2 episodic memory types described in
// `aranea/docs/14 memory-L2-episodic.md`. L2 records the unified event
// stream for a session (messages + tool/skill/MCP calls + L1 task
// snapshots) and exposes per-session episodes that feed L3 / L4
// consolidation.
package domain

// EpisodeKind enumerates the high-level reason an episode was created.
// The string values are persisted in `memory_episodes.episode_kind` so
// changing them requires a migration.
type EpisodeKind string

const (
	EpisodeKindTask         EpisodeKind = "task"
	EpisodeKindMilestone    EpisodeKind = "milestone"
	EpisodeKindFailurePM    EpisodeKind = "failure_postmortem"
	EpisodeKindUserMarked   EpisodeKind = "user_marked"
	EpisodeKindCriticPass   EpisodeKind = "critic_pass"
)

// IsValid returns true when the value matches one of the kinds above.
// Empty input is treated as "task" by callers; that translation happens
// at the service layer so the domain stays free of defaults.
func (k EpisodeKind) IsValid() bool {
	switch k {
	case EpisodeKindTask, EpisodeKindMilestone, EpisodeKindFailurePM, EpisodeKindUserMarked, EpisodeKindCriticPass:
		return true
	}
	return false
}

// MemoryEpisode is the persisted row in `memory_episodes`. Field naming
// mirrors the SQL column names so JSON tags double as the wire format
// for the HTTP API in §6.3.
type MemoryEpisode struct {
	ID                  string  `json:"id"`
	SessionID           string  `json:"session_id"`
	RunID               string  `json:"run_id,omitempty"`
	TeamID              string  `json:"team_id,omitempty"`
	AgentID             string  `json:"agent_id,omitempty"`
	L1TaskID            string  `json:"l1_task_id,omitempty"`
	Kind                EpisodeKind `json:"episode_kind"`
	Title               string  `json:"title"`
	Goal                string  `json:"goal,omitempty"`
	Outcome             string  `json:"outcome,omitempty"`
	OutcomeSummary      string  `json:"outcome_summary,omitempty"`
	ResultPreview       string  `json:"result_preview,omitempty"`
	FailureReason       string  `json:"failure_reason,omitempty"`
	Importance          float64 `json:"importance"`
	Confidence          float64 `json:"confidence"`
	UserFeedback        string  `json:"user_feedback,omitempty"`
	CriticScore         float64 `json:"critic_score"`
	SpanCount           int     `json:"span_count"`
	MessageCount        int     `json:"message_count"`
	ToolCallCount       int     `json:"tool_call_count"`
	SkillCallCount      int     `json:"skill_call_count"`
	MCPCallCount        int     `json:"mcp_call_count"`
	TotalTokens         int     `json:"total_tokens"`
	TotalCostMicroUSD   int64   `json:"total_cost_micro_usd"`
	DurationMS          int     `json:"duration_ms"`
	L1SnapshotJSON      string  `json:"l1_snapshot_json,omitempty"`
	KeyDecisionsJSON    string  `json:"key_decisions_json,omitempty"`
	KeyArtifactsJSON    string  `json:"key_artifacts_json,omitempty"`
	EmbeddingStatus     string  `json:"embedding_status,omitempty"`
	EmbeddingModel      string  `json:"embedding_model,omitempty"`
	EmbeddingDim        int     `json:"embedding_dim,omitempty"`
	EmbeddingNorm       float64 `json:"embedding_norm,omitempty"`
	ConsolidationStatus string  `json:"consolidation_status"`
	ConsolidatedAt      string  `json:"consolidated_at,omitempty"`
	ConsolidatedL3Count int     `json:"consolidated_l3_count"`
	ConsolidatedL4Count int     `json:"consolidated_l4_count"`
	StartedAt           string  `json:"started_at,omitempty"`
	EndedAt             string  `json:"ended_at,omitempty"`
	MetadataJSON        string  `json:"metadata_json,omitempty"`
	CreatedAt           string  `json:"created_at,omitempty"`
	UpdatedAt           string  `json:"updated_at,omitempty"`
	ArchivedAt          string  `json:"archived_at,omitempty"`
	DeletedAt           string  `json:"deleted_at,omitempty"`
}

// MemoryL2Event is the read-only unified event row exposed by §6.2.
// Its physical source can be `messages`, `session_trace_spans`,
// `tool_invocations`, `skill_invocation`, `team_run_steps`, `monitor_events`
// — `RefTable` / `RefID` lets callers drill back to the canonical row.
type MemoryL2Event struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	SessionID  string         `json:"session_id"`
	RunID      string         `json:"run_id,omitempty"`
	TurnID     string         `json:"turn_id,omitempty"`
	SpanID     string         `json:"span_id,omitempty"`
	ActorType  string         `json:"actor_type,omitempty"`
	ActorID    string         `json:"actor_id,omitempty"`
	ActorName  string         `json:"actor_name,omitempty"`
	Status     string         `json:"status,omitempty"`
	Title      string         `json:"title,omitempty"`
	Preview    string         `json:"preview,omitempty"`
	OccurredAt string         `json:"occurred_at"`
	DurationMS int            `json:"duration_ms,omitempty"`
	TokensIn   int            `json:"tokens_in,omitempty"`
	TokensOut  int            `json:"tokens_out,omitempty"`
	CostMicro  int64          `json:"cost_micro_usd,omitempty"`
	RefTable   string         `json:"ref_table"`
	RefID      string         `json:"ref_id"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// MemoryL2EventQuery captures the filter set accepted by §6.2. Empty
// fields are treated as "match anything"; an empty session_id is rejected
// at the service layer so callers can never accidentally return cross-
// session data.
type MemoryL2EventQuery struct {
	SessionID    string   `json:"session_id"`
	TurnID       string   `json:"turn_id,omitempty"`
	SpanID       string   `json:"span_id,omitempty"`
	Kinds        []string `json:"kinds,omitempty"`
	ActorIDs     []string `json:"actor_ids,omitempty"`
	StatusIn     []string `json:"status_in,omitempty"`
	StartTimeUTC string   `json:"start_time,omitempty"`
	EndTimeUTC   string   `json:"end_time,omitempty"`
	Keyword      string   `json:"keyword,omitempty"`
	Limit        int      `json:"limit,omitempty"`
	Offset       int      `json:"offset,omitempty"`
}

// MemoryL2RecallQuery is the input to RecallByQuery / POST /l2/recall.
// QueryEmbedding is optional: when empty, the service falls back to
// BM25-only ranking (Phase 2).
type MemoryL2RecallQuery struct {
	SessionID      string        `json:"session_id"`
	AgentID        string        `json:"agent_id,omitempty"`
	Query          string        `json:"query"`
	QueryEmbedding []float32     `json:"query_embedding,omitempty"`
	MinImportance  float64       `json:"min_importance,omitempty"`
	TopK           int           `json:"top_k,omitempty"`
	IncludeKinds   []EpisodeKind `json:"include_kinds,omitempty"`
}

// MemoryL2RecallResult is the row shape returned by §6.5. Episode is
// pruned (only summary fields) before serialisation so prompt growth
// stays bounded.
type MemoryL2RecallResult struct {
	Episode   MemoryEpisode `json:"episode"`
	BM25Score float64       `json:"bm25_score,omitempty"`
	VectorSim float64       `json:"vector_sim,omitempty"`
	FinalRank float64       `json:"final_rank"`
}

// MemoryEventMark is the persisted row in `memory_event_marks`. The
// (RefKind, RefID, MarkType, MarkedBy) tuple is unique so re-marking
// the same event by the same actor is idempotent (UPSERT).
type MemoryEventMark struct {
	ID           string         `json:"id"`
	SessionID    string         `json:"session_id"`
	EpisodeID    string         `json:"episode_id,omitempty"`
	RefKind      string         `json:"ref_kind"`
	RefID        string         `json:"ref_id"`
	MarkType     string         `json:"mark_type"`
	MarkedBy     string         `json:"marked_by,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	Weight       float64        `json:"weight"`
	MetadataJSON string         `json:"metadata_json,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	CreatedAt    string         `json:"created_at,omitempty"`
	DeletedAt    string         `json:"deleted_at,omitempty"`
}

// MemoryL2IndexEntry mirrors `memory_l2_index_meta`. Phase 2 writes one
// row per (episode, text_kind) tuple; the FTS5 virtual table holds the
// tokenised text for BM25 ranking.
type MemoryL2IndexEntry struct {
	ID             string  `json:"id"`
	EpisodeID      string  `json:"episode_id"`
	SessionID      string  `json:"session_id"`
	AgentID        string  `json:"agent_id,omitempty"`
	TextKind       string  `json:"text_kind"`
	TextPreview    string  `json:"text_preview,omitempty"`
	TokenEstimate  int     `json:"token_estimate"`
	EmbeddingModel string  `json:"embedding_model,omitempty"`
	EmbeddingDim   int     `json:"embedding_dim,omitempty"`
	EmbeddingNorm  float64 `json:"embedding_norm,omitempty"`
	Importance     float64 `json:"importance"`
	CreatedAt      string  `json:"created_at,omitempty"`
	UpdatedAt      string  `json:"updated_at,omitempty"`
}

// L2KeyDecision and L2KeyArtifact are the structured shapes packed into
// `key_decisions_json` / `key_artifacts_json`. Persisting them as JSON
// strings keeps the column count bounded while still letting the API /
// front-end render them as first-class lists.
type L2KeyDecision struct {
	Decision  string `json:"decision"`
	Rationale string `json:"rationale,omitempty"`
	At        string `json:"at,omitempty"`
	SpanID    string `json:"span_id,omitempty"`
}

type L2KeyArtifact struct {
	Kind    string `json:"kind"`
	Ref     string `json:"ref"`
	Preview string `json:"preview,omitempty"`
}
