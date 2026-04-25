package domain

// L1TaskStatus enumerates the lifecycle states of a working-memory task as
// described in `aranea/docs/13 memory-L1-working.md` §3.1. The string values
// match what is persisted in `memory_l1_tasks.status` and surfaced through the
// HTTP API, so they double as the source of truth for the front-end.
type L1TaskStatus string

const (
	L1TaskActive    L1TaskStatus = "active"
	L1TaskPaused    L1TaskStatus = "paused"
	L1TaskCompleted L1TaskStatus = "completed"
	L1TaskFailed    L1TaskStatus = "failed"
	L1TaskCancelled L1TaskStatus = "cancelled"
	L1TaskTimeout   L1TaskStatus = "timeout"
	L1TaskArchived  L1TaskStatus = "archived"
)

// IsTerminal reports whether the status means "no more writes" so the L1
// service can refuse mutations and the L0 renderer can stop injecting it.
func (s L1TaskStatus) IsTerminal() bool {
	switch s {
	case L1TaskCompleted, L1TaskFailed, L1TaskCancelled, L1TaskTimeout, L1TaskArchived:
		return true
	}
	return false
}

// L1FieldShare encodes per-field cross-agent visibility. Stored as JSON inside
// `memory_l1_tasks.shared_with_json`. ReadBy contains agent IDs (or `team:*`
// wildcards) that may read the listed field even if it is otherwise private.
type L1FieldShare struct {
	Field  string   `json:"field"`
	ReadBy []string `json:"read_by"`
}

// MemoryL1Task is the in-memory shape of one row in `memory_l1_tasks`. The
// container ties a working-memory snapshot to a session / agent / run so the
// L0 renderer and ChatService can find the right state for each turn.
type MemoryL1Task struct {
	ID            string         `json:"id"`
	SessionID     string         `json:"session_id"`
	RunID         string         `json:"run_id,omitempty"`
	TeamID        string         `json:"team_id,omitempty"`
	AgentID       string         `json:"agent_id,omitempty"`
	TaskKey       string         `json:"task_key"`
	TaskTitle     string         `json:"task_title"`
	TaskGoal      string         `json:"task_goal"`
	Status        L1TaskStatus   `json:"status"`
	SchemaVersion int            `json:"schema_version"`
	BudgetTokens  int            `json:"budget_tokens"`
	UsedTokens    int            `json:"used_tokens"`
	ParentTaskID  string         `json:"parent_task_id,omitempty"`
	SharedWith    []L1FieldShare `json:"shared_with,omitempty"`
	StartedAt     string         `json:"started_at"`
	EndedAt       string         `json:"ended_at,omitempty"`
	ArchivedAt    string         `json:"archived_at,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
}

// MemoryL1Field is the in-memory shape of one row in `memory_l1_fields`. The
// value is split across text / json / ref columns so callers can store either
// a primitive payload, a structured payload, or just a reference id.
type MemoryL1Field struct {
	ID            string         `json:"id"`
	TaskID        string         `json:"task_id"`
	SessionID     string         `json:"session_id"`
	AgentID       string         `json:"agent_id,omitempty"`
	FieldPath     string         `json:"field_path"`
	FieldKind     string         `json:"field_kind"`
	Visibility    string         `json:"visibility"`
	PinToPrompt   bool           `json:"pin_to_prompt"`
	IsRequired    bool           `json:"is_required"`
	ValueText     string         `json:"value_text,omitempty"`
	ValueJSON     string         `json:"value_json,omitempty"`
	ValueRef      string         `json:"value_ref,omitempty"`
	Preview       string         `json:"preview,omitempty"`
	TokenEstimate int            `json:"token_estimate"`
	Source        string         `json:"source,omitempty"`
	SourceRef     string         `json:"source_ref,omitempty"`
	TTLSeconds    int            `json:"ttl_seconds,omitempty"`
	ExpiresAt     string         `json:"expires_at,omitempty"`
	Revision      int            `json:"revision"`
	LastReadAt    string         `json:"last_read_at,omitempty"`
	ReadCount     int            `json:"read_count,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
}

// MemoryL1FieldHistory records one revision of a field. It is appended on
// every write (see spec §5.2) so users can roll back.
type MemoryL1FieldHistory struct {
	ID            string `json:"id"`
	FieldID       string `json:"field_id"`
	TaskID        string `json:"task_id"`
	Revision      int    `json:"revision"`
	ValueText     string `json:"value_text,omitempty"`
	ValueJSON     string `json:"value_json,omitempty"`
	ValueRef      string `json:"value_ref,omitempty"`
	Preview       string `json:"preview,omitempty"`
	TokenEstimate int    `json:"token_estimate"`
	ChangedBy     string `json:"changed_by,omitempty"`
	ChangeReason  string `json:"change_reason,omitempty"`
	DiffJSON      string `json:"diff_json,omitempty"`
	MetadataJSON  string `json:"metadata_json,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// MemoryL1Schema represents one declared expected-fields schema for a scope
// (agent / skill / team / global). The actual JSON Schema text lives in
// SchemaJSON. Validation against this schema happens on writes (Phase 2).
type MemoryL1Schema struct {
	ID            string         `json:"id"`
	ScopeType     string         `json:"scope_type"`
	ScopeID       string         `json:"scope_id,omitempty"`
	SchemaKey     string         `json:"schema_key"`
	SchemaVersion int            `json:"schema_version"`
	SchemaJSON    string         `json:"schema_json"`
	Description   string         `json:"description,omitempty"`
	Enabled       bool           `json:"enabled"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
}

// L1FieldPatch is the input to MemoryL1Service.SetField / PatchFields. The
// service decides which value column to fill based on FieldKind. IfRevision
// implements the optimistic-lock contract from spec §5.2 step 6.
type L1FieldPatch struct {
	FieldPath    string         `json:"field_path"`
	FieldKind    string         `json:"field_kind,omitempty"`
	Value        any            `json:"value,omitempty"`
	ValueRef     string         `json:"value_ref,omitempty"`
	Preview      string         `json:"preview,omitempty"`
	Visibility   string         `json:"visibility,omitempty"`
	PinToPrompt  *bool          `json:"pin_to_prompt,omitempty"`
	IsRequired   *bool          `json:"is_required,omitempty"`
	TTLSeconds   *int           `json:"ttl_seconds,omitempty"`
	Source       string         `json:"source,omitempty"`
	SourceRef    string         `json:"source_ref,omitempty"`
	ChangedBy    string         `json:"changed_by,omitempty"`
	ChangeReason string         `json:"change_reason,omitempty"`
	IfRevision   *int           `json:"if_revision,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// L1TaskListQuery is the filter used by the HTTP layer to list tasks of one
// session.
type L1TaskListQuery struct {
	SessionID    string
	AgentID      string
	Status       string
	IncludeEnded bool
}

// L1PromptBlock is the rendered view of a task's prompt-visible fields. It is
// produced by MemoryL1Service.RenderForPrompt and consumed by MemoryL0Service
// (see spec §5.3). The Content is the markdown / yaml fed into the model;
// MissingFields lists schema-required paths that are still empty so the L0
// layer can append a "please fill in" reminder.
type L1PromptBlock struct {
	Section       string   `json:"section"`
	Role          string   `json:"role"`
	Source        string   `json:"source"`
	Tokens        int      `json:"tokens"`
	Content       string   `json:"content"`
	Preview       string   `json:"preview,omitempty"`
	MissingFields []string `json:"missing_fields,omitempty"`
	TaskID        string   `json:"task_id,omitempty"`
}

// L1Episode is the snapshot delivered to the L2 episode pipeline when a task
// ends. The actual L2 schema lives in `aranea/docs/14`; this is just the
// transport shape.
type L1Episode struct {
	TaskID       string         `json:"task_id"`
	SessionID    string         `json:"session_id"`
	AgentID      string         `json:"agent_id,omitempty"`
	TaskKey      string         `json:"task_key"`
	TaskTitle    string         `json:"task_title"`
	TaskGoal     string         `json:"task_goal"`
	Status       L1TaskStatus   `json:"status"`
	StartedAt    string         `json:"started_at"`
	EndedAt      string         `json:"ended_at"`
	UsedTokens   int            `json:"used_tokens"`
	BudgetTokens int            `json:"budget_tokens"`
	Snapshot     map[string]any `json:"snapshot"`
	Stats        map[string]int `json:"stats"`
}
