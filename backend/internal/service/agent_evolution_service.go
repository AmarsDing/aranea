// Package service – AgentEvolutionService is the L4 self-evolution
// façade described in `aranea/docs/16 memory-L4-persistent.md`. Phase 3
// ships identity / strategy CRUD, proposal & event lifecycle (propose →
// approve → apply / reject / revert), the BuildSelfPromptAppend and
// ResolveToolWhitelist / ResolveModelRouting helpers used by ChatService,
// and a stubbed RunEvolutionScan that returns an empty report so the
// HTTP handler can be wired now.
//
// The service is intentionally synchronous: the auto-scan worker is
// scheduled by the caller (cmd/server). All mutating operations write
// an audit log entry plus an EvolutionEvent so administrators can
// reconstruct who changed what.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// AgentEvolutionService owns identity / strategy CRUD plus the proposal
// / event lifecycle. The L2 episode source and L3 fact source are
// optional dependencies wired in by ChatService; the Phase 1 scan stub
// works without them.
type AgentEvolutionService struct {
	repo repository.Store
	pii  *PIIFilter
	now  func() string
}

// NewAgentEvolutionService builds a service over a repository. Callers
// may inject a PII filter via SetPIIFilter; otherwise the default regex
// filter is used.
func NewAgentEvolutionService(repo repository.Store) *AgentEvolutionService {
	return &AgentEvolutionService{repo: repo, pii: NewPIIFilter(), now: nowUTC}
}

// SetClock overrides the clock for tests.
func (s *AgentEvolutionService) SetClock(now func() string) {
	if now != nil {
		s.now = now
	}
}

// SetPIIFilter replaces the persona / values PII filter.
func (s *AgentEvolutionService) SetPIIFilter(p *PIIFilter) {
	if p != nil {
		s.pii = p
	}
}

// --- Inputs / outputs --------------------------------------------------------

// IdentityPatch is the partial update payload for §6.4 PATCH identity.
// Pointers signal "field present"; nil leaves the existing value
// untouched.
type IdentityPatch struct {
	Persona          *string   `json:"persona,omitempty"`
	Values           *[]string `json:"values,omitempty"`
	Tone             *string   `json:"tone,omitempty"`
	Domains          *[]string `json:"domains,omitempty"`
	UserExpectations *string   `json:"user_expectations,omitempty"`
	Phase            *string   `json:"current_phase,omitempty"`
	By               string    `json:"by,omitempty"`
	Reason           string    `json:"reason,omitempty"`
}

// StrategyPatch is the partial update payload for §6.4 PATCH strategy.
type StrategyPatch struct {
	Exploration        *float64           `json:"exploration,omitempty"`
	Conciseness        *float64           `json:"conciseness,omitempty"`
	Caution            *float64           `json:"caution,omitempty"`
	Delegation         *float64           `json:"delegation,omitempty"`
	ToolPreference     map[string]float64 `json:"tool_preference,omitempty"`
	ToolBlacklist      *[]string          `json:"tool_blacklist,omitempty"`
	ProviderPreference map[string]float64 `json:"provider_preference,omitempty"`
	ModelPreference    map[string]float64 `json:"model_preference,omitempty"`
	By                 string             `json:"by,omitempty"`
	Reason             string             `json:"reason,omitempty"`
}

// ProposalInput is the parameter object for §6.4 POST proposals.
type ProposalInput struct {
	AgentID          string               `json:"agent_id"`
	WorkspaceID      string               `json:"workspace_id,omitempty"`
	Kind             string               `json:"proposal_kind"`
	TargetField      string               `json:"target_field"`
	ProposedValue    any                  `json:"proposed_value,omitempty"`
	CurrentValue     any                  `json:"current_value,omitempty"`
	Rationale        string               `json:"rationale,omitempty"`
	Evidence         []domain.EvidenceRef `json:"evidence,omitempty"`
	ExpectedImpact   string               `json:"expected_impact,omitempty"`
	RiskLevel        string               `json:"risk_level,omitempty"`
	ApprovalRequired bool                 `json:"approval_required,omitempty"`
	Source           string               `json:"source,omitempty"`
	TTLDays          int                  `json:"ttl_days,omitempty"`
}

// ApplyInput is the parameter object for §5.6 Apply.
type ApplyInput struct {
	AgentID       string               `json:"agent_id"`
	Kind          string               `json:"event_kind"`
	TargetField   string               `json:"target_field,omitempty"`
	BeforeValue   any                  `json:"before_value,omitempty"`
	AfterValue    any                  `json:"after_value,omitempty"`
	TriggerKind   string               `json:"trigger_kind,omitempty"`
	TriggerSource string               `json:"trigger_source,omitempty"`
	Evidence      []domain.EvidenceRef `json:"evidence,omitempty"`
	Reason        string               `json:"reason,omitempty"`
	By            string               `json:"by,omitempty"`
}

// ScanReport summarises a §5.5 RunEvolutionScan call.
type ScanReport struct {
	EpisodesScanned    int    `json:"episodes_scanned"`
	NewProposals       int    `json:"new_proposals"`
	AutoApplied        int    `json:"auto_applied"`
	ThrottledProposals int    `json:"throttled_proposals"`
	Errors             int    `json:"errors"`
	Note               string `json:"note,omitempty"`
}

// ModelCandidate mirrors the §5.4 type used by ResolveModelRouting.
type ModelCandidate struct {
	ProviderKey string  `json:"provider_key"`
	Model       string  `json:"model"`
	BaseScore   float64 `json:"base_score"`
}

// EvolutionEventListResult is the wire shape of GET §6.4 events.
type EvolutionEventListResult struct {
	Items  []domain.EvolutionEvent `json:"items"`
	Total  int                     `json:"total"`
	Limit  int                     `json:"limit"`
	Offset int                     `json:"offset"`
}

// EvolutionProposalListResult is the wire shape of GET §6.4 proposals.
type EvolutionProposalListResult struct {
	Items  []domain.EvolutionProposal `json:"items"`
	Total  int                        `json:"total"`
	Limit  int                        `json:"limit"`
	Offset int                        `json:"offset"`
}

// allowedTargetFields enumerates the §5.6 / §11 whitelist of fields a
// proposal / apply call may touch. Anything outside this list is
// rejected to prevent self-evolution from escaping into security-critical
// settings (RBAC, mcp credentials, base system prompt, etc).
var allowedTargetFields = map[string]struct{}{
	"identity.persona":            {},
	"identity.tone":               {},
	"identity.values":             {},
	"identity.domains":            {},
	"identity.user_expectations":  {},
	"identity.current_phase":      {},
	"strategy.exploration":        {},
	"strategy.conciseness":        {},
	"strategy.caution":            {},
	"strategy.delegation":         {},
	"strategy.tool_preference":    {},
	"strategy.tool_blacklist":     {},
	"strategy.provider_preference": {},
	"strategy.model_preference":   {},
	"system_prompt_append":        {},
	"tool_whitelist_diff":         {},
}

// --- Identity ---------------------------------------------------------------

// GetIdentity returns the agent's current identity, lazily creating an
// empty cold-start row when none exists yet.
func (s *AgentEvolutionService) GetIdentity(ctx context.Context, agentID string) (domain.AgentIdentity, error) {
	if agentID == "" {
		return domain.AgentIdentity{}, validationError("agent id is required")
	}
	id, err := s.repo.GetAgentIdentity(agentID)
	if err == nil {
		return id, nil
	}
	now := s.now()
	id = domain.AgentIdentity{
		AgentID:      agentID,
		CurrentPhase: domain.AgentPhaseColdStart,
		Version:      1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	return s.repo.UpsertAgentIdentity(id)
}

// UpdateIdentity applies a patch and writes one EvolutionEvent per
// changed field so the timeline stays granular.
func (s *AgentEvolutionService) UpdateIdentity(ctx context.Context, agentID string, patch IdentityPatch) (domain.AgentIdentity, error) {
	if agentID == "" {
		return domain.AgentIdentity{}, validationError("agent id is required")
	}
	current, err := s.GetIdentity(ctx, agentID)
	if err != nil {
		return domain.AgentIdentity{}, err
	}
	updated := current
	changes := []apply{}
	if patch.Persona != nil {
		next := strings.TrimSpace(*patch.Persona)
		if s.pii != nil && s.pii.HasPII(next) {
			return domain.AgentIdentity{}, validationError("persona contains PII")
		}
		if next != current.Persona {
			updated.Persona = next
			changes = append(changes, apply{kind: domain.EvoKindPersonaUpdate, target: "identity.persona", before: current.Persona, after: next})
		}
	}
	if patch.Values != nil {
		next := normalizeStringList(*patch.Values)
		if !stringSliceEqual(next, current.Values) {
			updated.Values = next
			changes = append(changes, apply{kind: domain.EvoKindIdentityUpdate, target: "identity.values", before: current.Values, after: next})
		}
	}
	if patch.Tone != nil {
		next := strings.TrimSpace(*patch.Tone)
		if next != current.Tone {
			updated.Tone = next
			changes = append(changes, apply{kind: domain.EvoKindToneChange, target: "identity.tone", before: current.Tone, after: next})
		}
	}
	if patch.Domains != nil {
		next := normalizeStringList(*patch.Domains)
		if !stringSliceEqual(next, current.Domains) {
			updated.Domains = next
			changes = append(changes, apply{kind: domain.EvoKindDomainAdded, target: "identity.domains", before: current.Domains, after: next})
		}
	}
	if patch.UserExpectations != nil {
		next := strings.TrimSpace(*patch.UserExpectations)
		if next != current.UserExpectations {
			updated.UserExpectations = next
			changes = append(changes, apply{kind: domain.EvoKindIdentityUpdate, target: "identity.user_expectations", before: current.UserExpectations, after: next})
		}
	}
	if patch.Phase != nil {
		next := strings.TrimSpace(*patch.Phase)
		if next != current.CurrentPhase {
			updated.CurrentPhase = next
			changes = append(changes, apply{kind: domain.EvoKindPhaseChange, target: "identity.current_phase", before: current.CurrentPhase, after: next})
		}
	}
	if len(changes) == 0 {
		return current, nil
	}
	updated.Version = current.Version + 1
	updated.UpdatedAt = s.now()
	stored, err := s.repo.UpsertAgentIdentity(updated)
	if err != nil {
		return domain.AgentIdentity{}, err
	}
	for _, ch := range changes {
		_ = s.recordEvent(agentID, ch, patch.By, patch.Reason, domain.EvoTriggerUser, "")
	}
	_ = s.audit("agent.identity.update", "agent_identity", agentID, map[string]any{
		"by": patch.By, "reason": patch.Reason, "fields": changeFields(changes),
	})
	return stored, nil
}

// --- Strategy ---------------------------------------------------------------

// GetStrategy returns the agent's current strategy profile, lazily
// creating an empty row at version 1 when none exists.
func (s *AgentEvolutionService) GetStrategy(ctx context.Context, agentID string) (domain.AgentStrategyProfile, error) {
	if agentID == "" {
		return domain.AgentStrategyProfile{}, validationError("agent id is required")
	}
	p, err := s.repo.GetAgentStrategyProfile(agentID)
	if err == nil {
		return p, nil
	}
	now := s.now()
	p = domain.AgentStrategyProfile{
		AgentID:     agentID,
		Exploration: 0.5,
		Conciseness: 0.5,
		Caution:     0.5,
		Delegation:  0.5,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return s.repo.UpsertAgentStrategyProfile(p)
}

// UpdateStrategy applies a patch and writes one EvolutionEvent per
// changed field. Scalar fields are clamped to [0,1].
func (s *AgentEvolutionService) UpdateStrategy(ctx context.Context, agentID string, patch StrategyPatch) (domain.AgentStrategyProfile, error) {
	if agentID == "" {
		return domain.AgentStrategyProfile{}, validationError("agent id is required")
	}
	current, err := s.GetStrategy(ctx, agentID)
	if err != nil {
		return domain.AgentStrategyProfile{}, err
	}
	updated := current
	changes := []apply{}
	maybeFloat := func(name string, dst *float64, in *float64) {
		if in == nil {
			return
		}
		next := clamp01(*in)
		if next != *dst {
			before := *dst
			*dst = next
			changes = append(changes, apply{kind: domain.EvoKindStrategyParamUpdate, target: "strategy." + name, before: before, after: next})
		}
	}
	maybeFloat("exploration", &updated.Exploration, patch.Exploration)
	maybeFloat("conciseness", &updated.Conciseness, patch.Conciseness)
	maybeFloat("caution", &updated.Caution, patch.Caution)
	maybeFloat("delegation", &updated.Delegation, patch.Delegation)
	if patch.ToolPreference != nil {
		updated.ToolPreference = mergeFloatMap(current.ToolPreference, patch.ToolPreference)
		changes = append(changes, apply{kind: domain.EvoKindToolPrefUpdate, target: "strategy.tool_preference", before: current.ToolPreference, after: updated.ToolPreference})
	}
	if patch.ToolBlacklist != nil {
		next := normalizeStringList(*patch.ToolBlacklist)
		if !stringSliceEqual(next, current.ToolBlacklist) {
			updated.ToolBlacklist = next
			kind := domain.EvoKindToolDisable
			if len(next) < len(current.ToolBlacklist) {
				kind = domain.EvoKindToolEnable
			}
			changes = append(changes, apply{kind: kind, target: "strategy.tool_blacklist", before: current.ToolBlacklist, after: next})
		}
	}
	if patch.ProviderPreference != nil {
		updated.ProviderPreference = mergeFloatMap(current.ProviderPreference, patch.ProviderPreference)
		changes = append(changes, apply{kind: domain.EvoKindProviderPrefUpdate, target: "strategy.provider_preference", before: current.ProviderPreference, after: updated.ProviderPreference})
	}
	if patch.ModelPreference != nil {
		updated.ModelPreference = mergeFloatMap(current.ModelPreference, patch.ModelPreference)
		changes = append(changes, apply{kind: domain.EvoKindModelPrefUpdate, target: "strategy.model_preference", before: current.ModelPreference, after: updated.ModelPreference})
	}
	if len(changes) == 0 {
		return current, nil
	}
	updated.Version = current.Version + 1
	updated.UpdatedAt = s.now()
	stored, err := s.repo.UpsertAgentStrategyProfile(updated)
	if err != nil {
		return domain.AgentStrategyProfile{}, err
	}
	for _, ch := range changes {
		_ = s.recordEvent(agentID, ch, patch.By, patch.Reason, domain.EvoTriggerUser, "")
	}
	_ = s.audit("agent.strategy.update", "agent_strategy_profile", agentID, map[string]any{
		"by": patch.By, "reason": patch.Reason, "fields": changeFields(changes),
	})
	return stored, nil
}

// --- Proposals ---------------------------------------------------------------

// Propose stores a new EvolutionProposal. Throttling: pending proposals
// for the same target_field that fall within the agent's
// `evo_throttle_hours` window are marked superseded.
func (s *AgentEvolutionService) Propose(ctx context.Context, in ProposalInput) (domain.EvolutionProposal, error) {
	if in.AgentID == "" {
		return domain.EvolutionProposal{}, validationError("agent id is required")
	}
	if in.Kind == "" {
		return domain.EvolutionProposal{}, validationError("proposal_kind is required")
	}
	if _, ok := allowedTargetFields[in.TargetField]; !ok {
		return domain.EvolutionProposal{}, validationError("target_field %q is not in the evolution whitelist", in.TargetField)
	}
	now := s.now()
	settings, _ := s.repo.GetAgentRuntimeSettings(in.AgentID)
	throttleHours := settings.EvoThrottleHours
	if throttleHours <= 0 {
		throttleHours = 24
	}
	cutoff := time.Now().UTC().Add(-time.Duration(throttleHours) * time.Hour).Format(time.RFC3339)

	// Throttle: §13 says "24h 内同 target_field 第二次 proposal 被标记
	// superseded" — i.e. the *new* proposal arriving within the window
	// is superseded so the earliest one survives.
	status := domain.EvoProposalPending
	recent, _, err := s.repo.ListEvolutionProposals(repository.EvolutionProposalQuery{
		AgentID:     in.AgentID,
		TargetField: in.TargetField,
		Status:      domain.EvoProposalPending,
		Limit:       50,
	})
	if err == nil {
		for _, p := range recent {
			if p.CreatedAt > cutoff {
				status = domain.EvoProposalSuperseded
				break
			}
		}
	}

	ttlDays := in.TTLDays
	if ttlDays <= 0 {
		ttlDays = settings.EvoProposalTTLDays
	}
	if ttlDays <= 0 {
		ttlDays = 14
	}
	expiresAt := time.Now().UTC().Add(time.Duration(ttlDays) * 24 * time.Hour).Format(time.RFC3339)
	proposed, _ := json.Marshal(in.ProposedValue)
	current, _ := json.Marshal(in.CurrentValue)

	prop := domain.EvolutionProposal{
		ID:                newID(),
		AgentID:           in.AgentID,
		WorkspaceID:       in.WorkspaceID,
		Kind:              in.Kind,
		TargetField:       in.TargetField,
		ProposedValueJSON: string(proposed),
		CurrentValueJSON:  string(current),
		Rationale:         in.Rationale,
		Evidence:          in.Evidence,
		ExpectedImpact:    in.ExpectedImpact,
		RiskLevel:         defaultIfEmpty(in.RiskLevel, domain.EvoRiskLow),
		ApprovalRequired:  in.ApprovalRequired,
		Status:            status,
		ExpiresAt:         expiresAt,
		Source:            defaultIfEmpty(in.Source, domain.EvoSourceUser),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	stored, err := s.repo.InsertEvolutionProposal(prop)
	if err != nil {
		return domain.EvolutionProposal{}, err
	}
	_ = s.audit("agent.evolution.proposal.create", "agent_evolution_proposals", stored.ID, map[string]any{
		"agent_id": in.AgentID, "kind": in.Kind, "target": in.TargetField, "source": in.Source,
	})
	return stored, nil
}

// ListProposals returns the proposal queue for an agent.
func (s *AgentEvolutionService) ListProposals(ctx context.Context, agentID, status string, limit, offset int) (EvolutionProposalListResult, error) {
	q := repository.EvolutionProposalQuery{AgentID: agentID, Status: status, Limit: limit, Offset: offset}
	items, total, err := s.repo.ListEvolutionProposals(q)
	if err != nil {
		return EvolutionProposalListResult{}, err
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	return EvolutionProposalListResult{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}

// Approve transitions a pending proposal to applied: it reads the
// proposed value, applies it via the §5.6 path, and marks the proposal
// as applied with a link to the resulting EvolutionEvent.
func (s *AgentEvolutionService) Approve(ctx context.Context, proposalID, by string) (domain.EvolutionEvent, error) {
	if proposalID == "" {
		return domain.EvolutionEvent{}, validationError("proposal id is required")
	}
	prop, err := s.repo.GetEvolutionProposal(proposalID)
	if err != nil {
		return domain.EvolutionEvent{}, err
	}
	if prop.Status != domain.EvoProposalPending {
		return domain.EvolutionEvent{}, validationError("proposal is not pending (status=%s)", prop.Status)
	}
	var proposed any
	if prop.ProposedValueJSON != "" {
		_ = json.Unmarshal([]byte(prop.ProposedValueJSON), &proposed)
	}
	var current any
	if prop.CurrentValueJSON != "" {
		_ = json.Unmarshal([]byte(prop.CurrentValueJSON), &current)
	}
	event, err := s.Apply(ctx, ApplyInput{
		AgentID:       prop.AgentID,
		Kind:          prop.Kind,
		TargetField:   prop.TargetField,
		BeforeValue:   current,
		AfterValue:    proposed,
		TriggerKind:   domain.EvoTriggerProposal,
		TriggerSource: prop.ID,
		Evidence:      prop.Evidence,
		Reason:        prop.Rationale,
		By:            by,
	})
	if err != nil {
		return domain.EvolutionEvent{}, err
	}
	if err := s.repo.UpdateEvolutionProposalStatus(prop.ID, domain.EvoProposalApplied, by, event.ID, s.now()); err != nil {
		return event, err
	}
	_ = s.audit("agent.evolution.proposal.approve", "agent_evolution_proposals", prop.ID, map[string]any{
		"agent_id": prop.AgentID, "by": by, "event_id": event.ID,
	})
	return event, nil
}

// Reject marks a proposal as rejected with an optional reason.
func (s *AgentEvolutionService) Reject(ctx context.Context, proposalID, by, reason string) error {
	if proposalID == "" {
		return validationError("proposal id is required")
	}
	prop, err := s.repo.GetEvolutionProposal(proposalID)
	if err != nil {
		return err
	}
	if prop.Status != domain.EvoProposalPending {
		return validationError("proposal is not pending (status=%s)", prop.Status)
	}
	if err := s.repo.UpdateEvolutionProposalStatus(prop.ID, domain.EvoProposalRejected, by, "", s.now()); err != nil {
		return err
	}
	_ = s.audit("agent.evolution.proposal.reject", "agent_evolution_proposals", prop.ID, map[string]any{
		"agent_id": prop.AgentID, "by": by, "reason": reason,
	})
	return nil
}

// --- Apply / Revert ----------------------------------------------------------

// Apply writes the EvolutionEvent and propagates the change to the
// underlying identity / strategy / runtime_settings store. The §11
// allowedTargetFields whitelist is enforced.
func (s *AgentEvolutionService) Apply(ctx context.Context, in ApplyInput) (domain.EvolutionEvent, error) {
	if in.AgentID == "" {
		return domain.EvolutionEvent{}, validationError("agent id is required")
	}
	if in.Kind == "" {
		return domain.EvolutionEvent{}, validationError("event_kind is required")
	}
	if _, ok := allowedTargetFields[in.TargetField]; !ok {
		return domain.EvolutionEvent{}, validationError("target_field %q is not in the evolution whitelist", in.TargetField)
	}
	if err := s.applyTargetChange(ctx, in.AgentID, in.TargetField, in.AfterValue, in.By, in.Reason); err != nil {
		return domain.EvolutionEvent{}, err
	}
	beforeJSON, _ := json.Marshal(in.BeforeValue)
	afterJSON, _ := json.Marshal(in.AfterValue)
	now := s.now()
	event := domain.EvolutionEvent{
		ID:            newID(),
		AgentID:       in.AgentID,
		Kind:          in.Kind,
		TargetField:   in.TargetField,
		BeforeJSON:    string(beforeJSON),
		AfterJSON:     string(afterJSON),
		TriggerKind:   defaultIfEmpty(in.TriggerKind, domain.EvoTriggerUser),
		TriggerSource: in.TriggerSource,
		Evidence:      in.Evidence,
		Reason:        in.Reason,
		Applied:       true,
		AppliedAt:     now,
		CreatedAt:     now,
	}
	stored, err := s.repo.InsertEvolutionEvent(event)
	if err != nil {
		return domain.EvolutionEvent{}, err
	}
	_ = s.audit("agent.evolution.event.apply", "agent_evolution_events", stored.ID, map[string]any{
		"agent_id": in.AgentID, "kind": in.Kind, "target": in.TargetField, "by": in.By,
	})
	return stored, nil
}

// Revert reverses a previously applied event by writing a new event of
// kind=rollback whose AfterValue is the original BeforeValue, then marks
// the original event reverted.
func (s *AgentEvolutionService) Revert(ctx context.Context, eventID, by, reason string) (domain.EvolutionEvent, error) {
	if eventID == "" {
		return domain.EvolutionEvent{}, validationError("event id is required")
	}
	original, err := s.repo.GetEvolutionEvent(eventID)
	if err != nil {
		return domain.EvolutionEvent{}, err
	}
	if original.Reverted {
		return domain.EvolutionEvent{}, validationError("event already reverted")
	}
	var before any
	if original.BeforeJSON != "" {
		_ = json.Unmarshal([]byte(original.BeforeJSON), &before)
	}
	var after any
	if original.AfterJSON != "" {
		_ = json.Unmarshal([]byte(original.AfterJSON), &after)
	}
	event, err := s.Apply(ctx, ApplyInput{
		AgentID:       original.AgentID,
		Kind:          domain.EvoKindRollback,
		TargetField:   original.TargetField,
		BeforeValue:   after,
		AfterValue:    before,
		TriggerKind:   domain.EvoTriggerRollback,
		TriggerSource: eventID,
		Reason:        defaultIfEmpty(reason, "rollback"),
		By:            by,
	})
	if err != nil {
		return domain.EvolutionEvent{}, err
	}
	if err := s.repo.MarkEvolutionEventReverted(eventID, event.ID, s.now()); err != nil {
		return event, err
	}
	_ = s.audit("agent.evolution.event.revert", "agent_evolution_events", eventID, map[string]any{
		"agent_id": original.AgentID, "by": by, "reason": reason, "rollback_event": event.ID,
	})
	return event, nil
}

// ListEvents returns the EvolutionEvent timeline for an agent.
func (s *AgentEvolutionService) ListEvents(ctx context.Context, agentID, kind string, limit, offset int) (EvolutionEventListResult, error) {
	q := repository.EvolutionEventQuery{AgentID: agentID, Kind: kind, Limit: limit, Offset: offset}
	items, total, err := s.repo.ListEvolutionEvents(q)
	if err != nil {
		return EvolutionEventListResult{}, err
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	return EvolutionEventListResult{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}

// GetEvent returns a single event by ID.
func (s *AgentEvolutionService) GetEvent(ctx context.Context, id string) (domain.EvolutionEvent, error) {
	if id == "" {
		return domain.EvolutionEvent{}, validationError("event id is required")
	}
	return s.repo.GetEvolutionEvent(id)
}

// --- Skill stats -------------------------------------------------------------

// ListSkillStats returns per-tool skill statistics for an agent, sorted
// by preference score descending.
func (s *AgentEvolutionService) ListSkillStats(ctx context.Context, agentID string, limit int) ([]domain.AgentSkillStat, error) {
	return s.repo.ListAgentSkillStats(agentID, limit)
}

// UpsertSkillStat persists one row of telemetry. Used by the chat /
// tool-call pipeline (Phase 4) to drive proposal generation.
func (s *AgentEvolutionService) UpsertSkillStat(ctx context.Context, stat domain.AgentSkillStat) (domain.AgentSkillStat, error) {
	return s.repo.UpsertAgentSkillStat(stat)
}

// --- Worker ------------------------------------------------------------------

// RunEvolutionScan is the §5.5 stub: Phase 4 will implement the LLM
// JSON-mode reflection prompt that produces ProposalInputs. Phase 1
// returns an empty report so the HTTP handler can be wired now.
func (s *AgentEvolutionService) RunEvolutionScan(ctx context.Context, agentID string) (ScanReport, error) {
	if agentID == "" {
		return ScanReport{}, validationError("agent id is required")
	}
	settings, err := s.repo.GetAgentRuntimeSettings(agentID)
	if err == nil && !settings.EvoEnabled {
		return ScanReport{Note: "evo_enabled=false"}, nil
	}
	return ScanReport{Note: "evolution scan not yet implemented (see §5.5 Phase 4)"}, nil
}

// --- Runtime helpers consumed by ChatService / L0 ---------------------------

// BuildSelfPromptAppend returns the markdown self-evolution segment
// appended to the agent's system prompt by L0 assembly. Returns an empty
// string when the feature is disabled or no identity exists.
func (s *AgentEvolutionService) BuildSelfPromptAppend(ctx context.Context, agentID string) (string, error) {
	if agentID == "" {
		return "", nil
	}
	settings, err := s.repo.GetAgentRuntimeSettings(agentID)
	if err == nil {
		if !settings.L4Enabled {
			return "", nil
		}
	}
	identity, err := s.GetIdentity(ctx, agentID)
	if err != nil {
		return "", nil
	}
	maxChars := 1500
	if settings.EvoPersonaMaxChars > 0 {
		maxChars = settings.EvoPersonaMaxChars
	}
	parts := []string{}
	if settings.L4IdentityInject || settings.AgentID == "" {
		persona := strings.TrimSpace(identity.Persona)
		if persona != "" {
			if len(persona) > maxChars {
				persona = persona[:maxChars] + "..."
			}
			parts = append(parts, "# Self\n"+persona)
		}
		if len(identity.Values) > 0 {
			parts = append(parts, "Values: "+strings.Join(identity.Values, ", "))
		}
		if identity.Tone != "" {
			parts = append(parts, "Tone: "+identity.Tone)
		}
		if len(identity.Domains) > 0 {
			parts = append(parts, "Domains: "+strings.Join(identity.Domains, ", "))
		}
	}
	if settings.L4StrategyInject {
		profile, err := s.GetStrategy(ctx, agentID)
		if err == nil {
			parts = append(parts,
				fmt.Sprintf("Strategy hints:\n  - exploration=%.2f\n  - conciseness=%.2f\n  - caution=%.2f\n  - delegation=%.2f",
					profile.Exploration, profile.Conciseness, profile.Caution, profile.Delegation))
		}
	}
	if len(parts) == 0 {
		return "", nil
	}
	body := "<self_evolution>\n" + strings.Join(parts, "\n\n") + "\n</self_evolution>"
	return body, nil
}

// ResolveToolWhitelist filters the base whitelist by removing any tool
// in the agent's strategy.tool_blacklist and reorders the survivors by
// preference score (descending).
func (s *AgentEvolutionService) ResolveToolWhitelist(ctx context.Context, agentID string, base []string) ([]string, error) {
	if agentID == "" || len(base) == 0 {
		return base, nil
	}
	profile, err := s.GetStrategy(ctx, agentID)
	if err != nil {
		return base, nil
	}
	blacklist := map[string]struct{}{}
	for _, t := range profile.ToolBlacklist {
		blacklist[t] = struct{}{}
	}
	out := make([]string, 0, len(base))
	for _, t := range base {
		if _, blocked := blacklist[t]; blocked {
			continue
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return profile.ToolPreference[out[i]] > profile.ToolPreference[out[j]]
	})
	return out, nil
}

// ToolPolicyForAgent returns the agent's strategy.tool_blacklist plus a
// copy of strategy.tool_preference. Used by ToolService.EffectiveForAgent
// to surface evolution-driven denials and reorder allowed tools by the
// agent's preference scores. Returns empty values + nil error when no
// strategy row exists yet (cold start).
func (s *AgentEvolutionService) ToolPolicyForAgent(ctx context.Context, agentID string) ([]string, map[string]float64, error) {
	if agentID == "" {
		return nil, nil, nil
	}
	profile, err := s.GetStrategy(ctx, agentID)
	if err != nil {
		return nil, nil, err
	}
	prefs := map[string]float64{}
	for k, v := range profile.ToolPreference {
		prefs[k] = v
	}
	return append([]string(nil), profile.ToolBlacklist...), prefs, nil
}

// ResolveModelRouting reorders model candidates by `BaseScore *
// (0.5 + preference)` so the highest-scoring model is first. Empty
// preferences default to 0.5.
func (s *AgentEvolutionService) ResolveModelRouting(ctx context.Context, agentID string, candidates []ModelCandidate) ([]ModelCandidate, error) {
	if agentID == "" || len(candidates) == 0 {
		return candidates, nil
	}
	profile, err := s.GetStrategy(ctx, agentID)
	if err != nil {
		return candidates, nil
	}
	scored := make([]ModelCandidate, len(candidates))
	for i, c := range candidates {
		key := c.ProviderKey + "/" + c.Model
		pref, ok := profile.ModelPreference[key]
		if !ok {
			pref = profile.ProviderPreference[c.ProviderKey]
		}
		scored[i] = c
		scored[i].BaseScore = c.BaseScore * (0.5 + pref)
	}
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].BaseScore > scored[j].BaseScore
	})
	return scored, nil
}

// --- Internal apply machinery -----------------------------------------------

type apply struct {
	kind   string
	target string
	before any
	after  any
}

func changeFields(c []apply) []string {
	out := make([]string, 0, len(c))
	for _, ch := range c {
		out = append(out, ch.target)
	}
	return out
}

// recordEvent writes a single EvolutionEvent for one identity / strategy
// field mutation. Used by UpdateIdentity / UpdateStrategy where multiple
// fields can change in a single call.
func (s *AgentEvolutionService) recordEvent(agentID string, ch apply, by, reason, trigger, triggerSource string) error {
	beforeJSON, _ := json.Marshal(ch.before)
	afterJSON, _ := json.Marshal(ch.after)
	now := s.now()
	_, err := s.repo.InsertEvolutionEvent(domain.EvolutionEvent{
		ID:            newID(),
		AgentID:       agentID,
		Kind:          ch.kind,
		TargetField:   ch.target,
		BeforeJSON:    string(beforeJSON),
		AfterJSON:     string(afterJSON),
		TriggerKind:   trigger,
		TriggerSource: triggerSource,
		Reason:        reason,
		Applied:       true,
		AppliedAt:     now,
		CreatedAt:     now,
	})
	return err
}

// applyTargetChange propagates a single field change to the underlying
// identity / strategy / runtime_settings store. Unknown targets are
// already filtered by allowedTargetFields.
func (s *AgentEvolutionService) applyTargetChange(ctx context.Context, agentID, target string, after any, by, reason string) error {
	switch target {
	case "identity.persona", "identity.tone", "identity.values", "identity.domains", "identity.user_expectations", "identity.current_phase":
		patch := IdentityPatch{By: by, Reason: reason}
		switch target {
		case "identity.persona":
			str := toString(after)
			patch.Persona = &str
		case "identity.tone":
			str := toString(after)
			patch.Tone = &str
		case "identity.values":
			vals := toStringSlice(after)
			patch.Values = &vals
		case "identity.domains":
			vals := toStringSlice(after)
			patch.Domains = &vals
		case "identity.user_expectations":
			str := toString(after)
			patch.UserExpectations = &str
		case "identity.current_phase":
			str := toString(after)
			patch.Phase = &str
		}
		// Direct write without recursing through UpdateIdentity (which
		// would generate yet another EvolutionEvent for the same change).
		return s.applyIdentityPatch(ctx, agentID, patch)
	case "strategy.exploration", "strategy.conciseness", "strategy.caution", "strategy.delegation":
		val := toFloat(after)
		return s.applyStrategyScalar(ctx, agentID, strings.TrimPrefix(target, "strategy."), val)
	case "strategy.tool_preference":
		m := toFloatMap(after)
		return s.applyStrategyMap(ctx, agentID, "tool_preference", m)
	case "strategy.tool_blacklist":
		v := toStringSlice(after)
		return s.applyStrategyBlacklist(ctx, agentID, v)
	case "strategy.provider_preference":
		m := toFloatMap(after)
		return s.applyStrategyMap(ctx, agentID, "provider_preference", m)
	case "strategy.model_preference":
		m := toFloatMap(after)
		return s.applyStrategyMap(ctx, agentID, "model_preference", m)
	case "system_prompt_append", "tool_whitelist_diff":
		// These targets are surfaced via BuildSelfPromptAppend /
		// ResolveToolWhitelist at L0 assembly time; no settings row is
		// directly mutated.
		return nil
	}
	return nil
}

func (s *AgentEvolutionService) applyIdentityPatch(ctx context.Context, agentID string, patch IdentityPatch) error {
	current, err := s.GetIdentity(ctx, agentID)
	if err != nil {
		return err
	}
	updated := current
	if patch.Persona != nil {
		updated.Persona = strings.TrimSpace(*patch.Persona)
	}
	if patch.Values != nil {
		updated.Values = normalizeStringList(*patch.Values)
	}
	if patch.Tone != nil {
		updated.Tone = strings.TrimSpace(*patch.Tone)
	}
	if patch.Domains != nil {
		updated.Domains = normalizeStringList(*patch.Domains)
	}
	if patch.UserExpectations != nil {
		updated.UserExpectations = strings.TrimSpace(*patch.UserExpectations)
	}
	if patch.Phase != nil {
		updated.CurrentPhase = strings.TrimSpace(*patch.Phase)
	}
	updated.Version = current.Version + 1
	updated.UpdatedAt = s.now()
	_, err = s.repo.UpsertAgentIdentity(updated)
	return err
}

func (s *AgentEvolutionService) applyStrategyScalar(ctx context.Context, agentID, field string, value float64) error {
	current, err := s.GetStrategy(ctx, agentID)
	if err != nil {
		return err
	}
	updated := current
	switch field {
	case "exploration":
		updated.Exploration = clamp01(value)
	case "conciseness":
		updated.Conciseness = clamp01(value)
	case "caution":
		updated.Caution = clamp01(value)
	case "delegation":
		updated.Delegation = clamp01(value)
	}
	updated.Version = current.Version + 1
	updated.UpdatedAt = s.now()
	_, err = s.repo.UpsertAgentStrategyProfile(updated)
	return err
}

func (s *AgentEvolutionService) applyStrategyMap(ctx context.Context, agentID, field string, value map[string]float64) error {
	current, err := s.GetStrategy(ctx, agentID)
	if err != nil {
		return err
	}
	updated := current
	switch field {
	case "tool_preference":
		updated.ToolPreference = mergeFloatMap(current.ToolPreference, value)
	case "provider_preference":
		updated.ProviderPreference = mergeFloatMap(current.ProviderPreference, value)
	case "model_preference":
		updated.ModelPreference = mergeFloatMap(current.ModelPreference, value)
	}
	updated.Version = current.Version + 1
	updated.UpdatedAt = s.now()
	_, err = s.repo.UpsertAgentStrategyProfile(updated)
	return err
}

func (s *AgentEvolutionService) applyStrategyBlacklist(ctx context.Context, agentID string, value []string) error {
	current, err := s.GetStrategy(ctx, agentID)
	if err != nil {
		return err
	}
	current.ToolBlacklist = normalizeStringList(value)
	current.Version = current.Version + 1
	current.UpdatedAt = s.now()
	_, err = s.repo.UpsertAgentStrategyProfile(current)
	return err
}

// --- Audit helper -----------------------------------------------------------

func (s *AgentEvolutionService) audit(action, resource, resourceID string, detail map[string]any) error {
	body, _ := json.Marshal(detail)
	if len(body) == 0 {
		body = []byte("{}")
	}
	return s.repo.AddAuditLog(domain.AuditLog{
		ID:         newID(),
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Detail:     string(body),
	})
}

// --- Pure helpers -----------------------------------------------------------

func normalizeStringList(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, v)
	}
	return out
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mergeFloatMap(base, patch map[string]float64) map[string]float64 {
	out := map[string]float64{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range patch {
		out[k] = v
	}
	return out
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

func toStringSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, e := range s {
			if str, ok := e.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

func toFloatMap(v any) map[string]float64 {
	switch m := v.(type) {
	case map[string]float64:
		return m
	case map[string]any:
		out := make(map[string]float64, len(m))
		for k, e := range m {
			out[k] = toFloat(e)
		}
		return out
	}
	return nil
}
