// Package domain – agent self-evolution domain types described in
// `aranea/docs/16 memory-L4-persistent.md` §3.2 and §4.2. These structures
// represent the agent's stable identity, its strategy / preference
// profile, the immutable log of evolution events, the queue of pending
// proposals, and per-tool skill statistics.
package domain

// Agent identity phases persisted in `agent_identity.current_phase`.
const (
	AgentPhaseColdStart   = "cold-start"
	AgentPhaseWarming     = "warming"
	AgentPhaseMature      = "mature"
	AgentPhaseSpecialized = "specialized"
)

// Tone values persisted in `agent_identity.tone`.
const (
	AgentToneFormal   = "formal"
	AgentToneCasual   = "casual"
	AgentTonePlayful  = "playful"
	AgentToneStrict   = "strict"
	AgentToneAcademic = "academic"
)

// Trigger kinds and event kinds persisted in `agent_evolution_events`.
const (
	EvoTriggerAuto     = "auto"
	EvoTriggerProposal = "proposal"
	EvoTriggerUser     = "user"
	EvoTriggerCritic   = "critic"
	EvoTriggerPlugin   = "plugin"
	EvoTriggerRollback = "rollback"

	EvoKindIdentityUpdate      = "identity_update"
	EvoKindPersonaUpdate       = "persona_update"
	EvoKindToneChange          = "tone_change"
	EvoKindSystemPromptAppend  = "system_prompt_append"
	EvoKindSystemPromptReplace = "system_prompt_replace"
	EvoKindToolEnable          = "tool_enable"
	EvoKindToolDisable         = "tool_disable"
	EvoKindToolPrefUpdate      = "tool_pref_update"
	EvoKindProviderPrefUpdate  = "provider_pref_update"
	EvoKindModelPrefUpdate     = "model_pref_update"
	EvoKindStrategyParamUpdate = "strategy_param_update"
	EvoKindDomainAdded         = "domain_added"
	EvoKindPhaseChange         = "phase_change"
	EvoKindRollback            = "rollback"
	EvoKindRestore             = "restore"
)

// Proposal lifecycle statuses persisted in
// `agent_evolution_proposals.status`.
const (
	EvoProposalPending    = "pending"
	EvoProposalApproved   = "approved"
	EvoProposalRejected   = "rejected"
	EvoProposalApplied    = "applied"
	EvoProposalSuperseded = "superseded"
	EvoProposalExpired    = "expired"
)

// Risk levels persisted in `agent_evolution_proposals.risk_level`.
const (
	EvoRiskLow    = "low"
	EvoRiskMedium = "medium"
	EvoRiskHigh   = "high"
)

// Proposal source values persisted in
// `agent_evolution_proposals.source`.
const (
	EvoSourceConsolidator  = "consolidator"
	EvoSourceCritic        = "critic"
	EvoSourceRuntimeSignal = "runtime_signal"
	EvoSourceUser          = "user"
)

// AgentIdentity is the persisted row in `agent_identity` describing an
// agent's stable persona, values, tone, and lifecycle phase.
type AgentIdentity struct {
	AgentID          string         `json:"agent_id"`
	Persona          string         `json:"persona"`
	Values           []string       `json:"values"`
	Tone             string         `json:"tone,omitempty"`
	Domains          []string       `json:"domains"`
	UserExpectations string         `json:"user_expectations,omitempty"`
	CurrentPhase     string         `json:"current_phase"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	Version          int            `json:"version"`
	CreatedAt        string         `json:"created_at,omitempty"`
	UpdatedAt        string         `json:"updated_at,omitempty"`
}

// AgentStrategyProfile is the persisted row in `agent_strategy_profile`
// describing the agent's decision style and tool / model / provider
// preferences. All scalar fields live in [0,1].
type AgentStrategyProfile struct {
	AgentID            string             `json:"agent_id"`
	Exploration        float64            `json:"exploration"`
	Conciseness        float64            `json:"conciseness"`
	Caution            float64            `json:"caution"`
	Delegation         float64            `json:"delegation"`
	ToolPreference     map[string]float64 `json:"tool_preference,omitempty"`
	ToolBlacklist      []string           `json:"tool_blacklist,omitempty"`
	ProviderPreference map[string]float64 `json:"provider_preference,omitempty"`
	ModelPreference    map[string]float64 `json:"model_preference,omitempty"`
	Stats              map[string]any     `json:"stats,omitempty"`
	Metadata           map[string]any     `json:"metadata,omitempty"`
	Version            int                `json:"version"`
	CreatedAt          string             `json:"created_at,omitempty"`
	UpdatedAt          string             `json:"updated_at,omitempty"`
}

// EvolutionEvent is the immutable history row in
// `agent_evolution_events`. `BeforeJSON` is preserved for revert.
type EvolutionEvent struct {
	ID                string         `json:"id"`
	AgentID           string         `json:"agent_id"`
	WorkspaceID       string         `json:"workspace_id,omitempty"`
	Kind              string         `json:"event_kind"`
	TargetField       string         `json:"target_field,omitempty"`
	BeforeJSON        string         `json:"before_json,omitempty"`
	AfterJSON         string         `json:"after_json,omitempty"`
	DiffJSON          string         `json:"diff_json,omitempty"`
	TriggerKind       string         `json:"trigger_kind"`
	TriggerSource     string         `json:"trigger_source,omitempty"`
	Evidence          []EvidenceRef  `json:"evidence,omitempty"`
	Reason            string         `json:"reason,omitempty"`
	Applied           bool           `json:"applied"`
	Reverted          bool           `json:"reverted"`
	RevertedByEventID string         `json:"reverted_by_event_id,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	CreatedAt         string         `json:"created_at,omitempty"`
	AppliedAt         string         `json:"applied_at,omitempty"`
	RevertedAt        string         `json:"reverted_at,omitempty"`
}

// EvolutionProposal is the candidate change row in
// `agent_evolution_proposals`. Approved proposals link to the resulting
// EvolutionEvent via `AppliedEventID`.
type EvolutionProposal struct {
	ID                string         `json:"id"`
	AgentID           string         `json:"agent_id"`
	WorkspaceID       string         `json:"workspace_id,omitempty"`
	Kind              string         `json:"proposal_kind"`
	TargetField       string         `json:"target_field,omitempty"`
	ProposedValueJSON string         `json:"proposed_value_json,omitempty"`
	CurrentValueJSON  string         `json:"current_value_json,omitempty"`
	DiffJSON          string         `json:"diff_json,omitempty"`
	Rationale         string         `json:"rationale,omitempty"`
	Evidence          []EvidenceRef  `json:"evidence,omitempty"`
	ExpectedImpact    string         `json:"expected_impact,omitempty"`
	RiskLevel         string         `json:"risk_level"`
	ApprovalRequired  bool           `json:"approval_required"`
	Status            string         `json:"status"`
	ReviewedBy        string         `json:"reviewed_by,omitempty"`
	ReviewedAt        string         `json:"reviewed_at,omitempty"`
	AppliedEventID    string         `json:"applied_event_id,omitempty"`
	ExpiresAt         string         `json:"expires_at,omitempty"`
	Source            string         `json:"source"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	CreatedAt         string         `json:"created_at,omitempty"`
	UpdatedAt         string         `json:"updated_at,omitempty"`
}

// AgentSkillStat is the per-tool skill telemetry row in
// `agent_skill_stats`. Used by the EvolutionWorker to derive tool
// preference / blacklist proposals.
type AgentSkillStat struct {
	AgentID         string         `json:"agent_id"`
	Scope           string         `json:"scope"`
	ScopeValue      string         `json:"scope_value,omitempty"`
	ToolKey         string         `json:"tool_key"`
	Invocations     int            `json:"invocations"`
	Successes       int            `json:"successes"`
	Failures        int            `json:"failures"`
	UserOverrides   int            `json:"user_overrides"`
	AvgLatencyMS    float64        `json:"avg_latency_ms"`
	AvgTokens       float64        `json:"avg_tokens"`
	PreferenceScore float64        `json:"preference_score"`
	LastUsedAt      string         `json:"last_used_at,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	UpdatedAt       string         `json:"updated_at,omitempty"`
}
