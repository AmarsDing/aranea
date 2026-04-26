// agent_evolution_service_test.go covers the §13 acceptance criteria for
// the L4 Agent self-evolution service: identity / strategy CRUD with
// PII rejection, propose → approve → apply (evolution events recorded),
// revert (round-trip restore), throttle (second proposal in window
// becomes superseded), tool-blacklist enforcement via
// ResolveToolWhitelist, model-routing reorder via ResolveModelRouting,
// and BuildSelfPromptAppend gating on l4_enabled / l4_identity_inject /
// l4_strategy_inject.
package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// newTestEvolutionService spins up an in-memory L4 stack (repo + service)
// for the agent-evolution tests. The repo is shared so tests can poke
// at low-level state when an invariant isn't exposed via the service
// surface (e.g. assert that a row exists in agent_evolution_events).
func newTestEvolutionService(t *testing.T) (*AgentEvolutionService, repository.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "evo.db")
	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("new repo failed: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	return NewAgentEvolutionService(repo), repo
}

func strPtr(s string) *string { return &s }

// §13 – first GetIdentity / GetStrategy creates blank rows at version 1.
func TestEvoGetIdentityCreatesBlankRow(t *testing.T) {
	svc, _ := newTestEvolutionService(t)
	id, err := svc.GetIdentity(context.Background(), "agent-cold")
	if err != nil {
		t.Fatalf("get identity: %v", err)
	}
	if id.AgentID != "agent-cold" || id.Version != 1 || id.CurrentPhase != domain.AgentPhaseColdStart {
		t.Fatalf("expected cold-start v1 row, got %#v", id)
	}
	prof, err := svc.GetStrategy(context.Background(), "agent-cold")
	if err != nil {
		t.Fatalf("get strategy: %v", err)
	}
	if prof.AgentID != "agent-cold" || prof.Version != 1 || prof.Exploration != 0.5 {
		t.Fatalf("expected default profile, got %#v", prof)
	}
}

// §13 – PII (e.g. email) in persona is rejected with a validation error.
func TestEvoUpdateIdentityRejectsPersonaPII(t *testing.T) {
	svc, _ := newTestEvolutionService(t)
	persona := "Reach me at jane@example.com if confused."
	_, err := svc.UpdateIdentity(context.Background(), "agent-pii", IdentityPatch{Persona: &persona})
	if err == nil {
		t.Fatalf("expected error for PII persona, got nil")
	}
	if !strings.Contains(err.Error(), "PII") {
		t.Fatalf("expected error to mention PII, got %v", err)
	}
}

// §13 – UpdateIdentity records one EvolutionEvent per changed field and
// bumps the row version.
func TestEvoUpdateIdentityRecordsEvents(t *testing.T) {
	svc, _ := newTestEvolutionService(t)
	persona := "I am a senior backend engineer focused on Go and distributed systems."
	tone := domain.AgentToneAcademic
	domains := []string{"backend", "distributed-systems"}
	id, err := svc.UpdateIdentity(context.Background(), "agent-up", IdentityPatch{
		Persona: &persona, Tone: &tone, Domains: &domains, By: "tester",
	})
	if err != nil {
		t.Fatalf("update identity: %v", err)
	}
	if id.Version != 2 {
		t.Fatalf("expected version=2 after first update, got %d", id.Version)
	}
	events, err := svc.ListEvents(context.Background(), "agent-up", "", 50, 0)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if events.Total < 3 {
		t.Fatalf("expected at least 3 events (persona/tone/domains), got %d", events.Total)
	}
}

// §13 – Approving a proposal applies the change and writes a linked
// EvolutionEvent; the proposal is marked applied.
func TestEvoProposeApproveAppliesChange(t *testing.T) {
	svc, repo := newTestEvolutionService(t)
	prop, err := svc.Propose(context.Background(), ProposalInput{
		AgentID:       "agent-prop",
		Kind:          domain.EvoKindStrategyParamUpdate,
		TargetField:   "strategy.exploration",
		ProposedValue: 0.85,
		CurrentValue:  0.5,
		Source:        domain.EvoSourceUser,
		RiskLevel:     domain.EvoRiskLow,
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if prop.Status != domain.EvoProposalPending {
		t.Fatalf("expected pending status, got %q", prop.Status)
	}
	event, err := svc.Approve(context.Background(), prop.ID, "tester")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if event.TargetField != "strategy.exploration" || !event.Applied {
		t.Fatalf("expected applied event for exploration, got %#v", event)
	}
	updated, err := repo.GetEvolutionProposal(prop.ID)
	if err != nil {
		t.Fatalf("get proposal: %v", err)
	}
	if updated.Status != domain.EvoProposalApplied || updated.AppliedEventID != event.ID {
		t.Fatalf("expected proposal applied with event link, got %#v", updated)
	}
	prof, err := svc.GetStrategy(context.Background(), "agent-prop")
	if err != nil {
		t.Fatalf("get strategy: %v", err)
	}
	if prof.Exploration < 0.84 || prof.Exploration > 0.86 {
		t.Fatalf("expected exploration ≈ 0.85, got %.3f", prof.Exploration)
	}
}

// §13 – Revert restores the original value and marks the original event
// reverted.
func TestEvoRevertRestoresPreviousValue(t *testing.T) {
	svc, repo := newTestEvolutionService(t)
	event, err := svc.Apply(context.Background(), ApplyInput{
		AgentID:     "agent-revert",
		Kind:        domain.EvoKindToneChange,
		TargetField: "identity.tone",
		BeforeValue: domain.AgentToneCasual,
		AfterValue:  domain.AgentToneStrict,
		By:          "tester",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	id, err := svc.GetIdentity(context.Background(), "agent-revert")
	if err != nil {
		t.Fatalf("get identity: %v", err)
	}
	if id.Tone != domain.AgentToneStrict {
		t.Fatalf("expected tone=strict after apply, got %q", id.Tone)
	}
	if _, err := svc.Revert(context.Background(), event.ID, "tester", "user changed mind"); err != nil {
		t.Fatalf("revert: %v", err)
	}
	id, err = svc.GetIdentity(context.Background(), "agent-revert")
	if err != nil {
		t.Fatalf("get identity post-revert: %v", err)
	}
	if id.Tone != domain.AgentToneCasual {
		t.Fatalf("expected tone reverted to casual, got %q", id.Tone)
	}
	original, err := repo.GetEvolutionEvent(event.ID)
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	if !original.Reverted || original.RevertedByEventID == "" {
		t.Fatalf("expected original event marked reverted with link, got %#v", original)
	}
}

// §13 – the second proposal for the same target_field within the
// throttle window is marked superseded; the first one stays pending.
func TestEvoProposeThrottlesDuplicateWithinWindow(t *testing.T) {
	svc, _ := newTestEvolutionService(t)
	first, err := svc.Propose(context.Background(), ProposalInput{
		AgentID:       "agent-throttle",
		Kind:          domain.EvoKindStrategyParamUpdate,
		TargetField:   "strategy.caution",
		ProposedValue: 0.3,
		CurrentValue:  0.5,
	})
	if err != nil {
		t.Fatalf("first propose: %v", err)
	}
	if first.Status != domain.EvoProposalPending {
		t.Fatalf("expected first pending, got %q", first.Status)
	}
	second, err := svc.Propose(context.Background(), ProposalInput{
		AgentID:       "agent-throttle",
		Kind:          domain.EvoKindStrategyParamUpdate,
		TargetField:   "strategy.caution",
		ProposedValue: 0.7,
		CurrentValue:  0.5,
	})
	if err != nil {
		t.Fatalf("second propose: %v", err)
	}
	if second.Status != domain.EvoProposalSuperseded {
		t.Fatalf("expected second superseded by throttle, got %q", second.Status)
	}
}

// §13 – Apply rejects target_fields outside the §11 whitelist.
func TestEvoApplyRejectsNonWhitelistedTarget(t *testing.T) {
	svc, _ := newTestEvolutionService(t)
	_, err := svc.Apply(context.Background(), ApplyInput{
		AgentID:     "agent-wl",
		Kind:        domain.EvoKindIdentityUpdate,
		TargetField: "internal.secret_key",
		AfterValue:  "leak",
	})
	if err == nil {
		t.Fatalf("expected error for non-whitelisted target")
	}
	if !strings.Contains(err.Error(), "whitelist") {
		t.Fatalf("expected whitelist error, got %v", err)
	}
}

// §13 – tools in `strategy.tool_blacklist` are removed by
// ResolveToolWhitelist; survivors are reordered by tool_preference desc.
func TestEvoResolveToolWhitelistFiltersAndReorders(t *testing.T) {
	svc, _ := newTestEvolutionService(t)
	blacklist := []string{"shell"}
	prefs := map[string]float64{"git": 0.9, "edit": 0.4, "search": 0.7}
	if _, err := svc.UpdateStrategy(context.Background(), "agent-tools", StrategyPatch{
		ToolBlacklist:  &blacklist,
		ToolPreference: prefs,
	}); err != nil {
		t.Fatalf("update strategy: %v", err)
	}
	got, err := svc.ResolveToolWhitelist(context.Background(), "agent-tools",
		[]string{"git", "shell", "edit", "search"})
	if err != nil {
		t.Fatalf("resolve whitelist: %v", err)
	}
	for _, t0 := range got {
		if t0 == "shell" {
			t.Fatalf("expected shell to be filtered, got %v", got)
		}
	}
	if len(got) != 3 || got[0] != "git" || got[1] != "search" || got[2] != "edit" {
		t.Fatalf("expected ordering [git, search, edit], got %v", got)
	}
}

// §13 – ResolveModelRouting reorders by base_score * (0.5 + preference).
func TestEvoResolveModelRoutingReordersByPreference(t *testing.T) {
	svc, _ := newTestEvolutionService(t)
	modelPrefs := map[string]float64{"openai/gpt-4": 0.9, "anthropic/claude-3": 0.2}
	if _, err := svc.UpdateStrategy(context.Background(), "agent-models", StrategyPatch{
		ModelPreference: modelPrefs,
	}); err != nil {
		t.Fatalf("update strategy: %v", err)
	}
	candidates := []ModelCandidate{
		{ProviderKey: "anthropic", Model: "claude-3", BaseScore: 1.0},
		{ProviderKey: "openai", Model: "gpt-4", BaseScore: 1.0},
	}
	got, err := svc.ResolveModelRouting(context.Background(), "agent-models", candidates)
	if err != nil {
		t.Fatalf("resolve routing: %v", err)
	}
	if got[0].Model != "gpt-4" {
		t.Fatalf("expected gpt-4 first by preference, got %#v", got)
	}
}

// §13 – BuildSelfPromptAppend honours `l4_enabled`. When the master
// switch is off the call returns an empty string regardless of identity.
func TestEvoBuildSelfPromptAppendGatedByL4Enabled(t *testing.T) {
	svc, repo := newTestEvolutionService(t)
	persona := "Concise systems engineer."
	if _, err := svc.UpdateIdentity(context.Background(), "agent-self", IdentityPatch{
		Persona: &persona,
		Tone:    strPtr(domain.AgentToneAcademic),
	}); err != nil {
		t.Fatalf("update identity: %v", err)
	}
	if _, err := repo.UpsertAgentRuntimeSettings(domain.AgentRuntimeSettings{
		AgentID:           "agent-self",
		L4Enabled:         false,
		L4IdentityInject:  true,
		EvoPersonaMaxChars: 200,
	}); err != nil {
		t.Fatalf("upsert settings: %v", err)
	}
	body, err := svc.BuildSelfPromptAppend(context.Background(), "agent-self")
	if err != nil {
		t.Fatalf("build prompt: %v", err)
	}
	if body != "" {
		t.Fatalf("expected empty body when l4_enabled=false, got %q", body)
	}
	if _, err := repo.UpsertAgentRuntimeSettings(domain.AgentRuntimeSettings{
		AgentID:            "agent-self",
		L4Enabled:          true,
		L4IdentityInject:   true,
		L4StrategyInject:   true,
		EvoPersonaMaxChars: 200,
	}); err != nil {
		t.Fatalf("upsert settings: %v", err)
	}
	body, err = svc.BuildSelfPromptAppend(context.Background(), "agent-self")
	if err != nil {
		t.Fatalf("build prompt: %v", err)
	}
	if !strings.Contains(body, "<self_evolution>") {
		t.Fatalf("expected wrapped prompt, got %q", body)
	}
	if !strings.Contains(body, persona) {
		t.Fatalf("expected persona in prompt, got %q", body)
	}
	if !strings.Contains(body, "Tone: academic") {
		t.Fatalf("expected tone in prompt, got %q", body)
	}
	if !strings.Contains(body, "Strategy hints") {
		t.Fatalf("expected strategy hints in prompt, got %q", body)
	}
}

// §13 – Persona is truncated to evo_persona_max_chars when long.
func TestEvoBuildSelfPromptAppendTruncatesPersona(t *testing.T) {
	svc, repo := newTestEvolutionService(t)
	long := strings.Repeat("a", 500)
	if _, err := svc.UpdateIdentity(context.Background(), "agent-trunc", IdentityPatch{
		Persona: &long,
	}); err != nil {
		t.Fatalf("update identity: %v", err)
	}
	if _, err := repo.UpsertAgentRuntimeSettings(domain.AgentRuntimeSettings{
		AgentID:           "agent-trunc",
		L4Enabled:         true,
		L4IdentityInject:  true,
		EvoPersonaMaxChars: 100,
	}); err != nil {
		t.Fatalf("upsert settings: %v", err)
	}
	body, err := svc.BuildSelfPromptAppend(context.Background(), "agent-trunc")
	if err != nil {
		t.Fatalf("build prompt: %v", err)
	}
	if !strings.Contains(body, strings.Repeat("a", 100)+"...") {
		t.Fatalf("expected truncated persona with ellipsis, got %q", body)
	}
}

// §13 – ToolPolicyForAgent surfaces the strategy.tool_blacklist plus
// strategy.tool_preference scores so ToolService.EffectiveForAgent can
// fold them into the per-agent view.
func TestEvoToolPolicyForAgentReturnsBlacklistAndPreferences(t *testing.T) {
	svc, _ := newTestEvolutionService(t)
	bl := []string{"shell_exec"}
	if _, err := svc.UpdateStrategy(context.Background(), "agent-tp", StrategyPatch{
		ToolBlacklist:  &bl,
		ToolPreference: map[string]float64{"web_search": 0.9, "datetime": 0.8},
	}); err != nil {
		t.Fatalf("update strategy: %v", err)
	}
	got, prefs, err := svc.ToolPolicyForAgent(context.Background(), "agent-tp")
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	if len(got) != 1 || got[0] != "shell_exec" {
		t.Fatalf("expected shell_exec blacklist, got %v", got)
	}
	if prefs["web_search"] != 0.9 {
		t.Fatalf("expected web_search preference, got %v", prefs)
	}
}

// §13 – ToolPolicyForAgent on a cold-start agent must not error: it
// returns empty values so ToolService can degrade gracefully.
func TestEvoToolPolicyForAgentColdStart(t *testing.T) {
	svc, _ := newTestEvolutionService(t)
	bl, prefs, err := svc.ToolPolicyForAgent(context.Background(), "agent-fresh")
	if err != nil {
		t.Fatalf("cold-start policy: %v", err)
	}
	if len(bl) != 0 || len(prefs) != 0 {
		t.Fatalf("expected empty policy on cold start, got bl=%v prefs=%v", bl, prefs)
	}
}

// Sanity check – ApplyInput's BeforeValue / AfterValue can be passed
// as JSON-decoded `any` (matching how Approve unmarshals proposals) and
// still propagate to the typed identity / strategy stores.
func TestEvoApplyAcceptsJSONDecodedValues(t *testing.T) {
	svc, _ := newTestEvolutionService(t)
	var after any
	if err := json.Unmarshal([]byte(`0.42`), &after); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	_, err := svc.Apply(context.Background(), ApplyInput{
		AgentID:     "agent-json",
		Kind:        domain.EvoKindStrategyParamUpdate,
		TargetField: "strategy.exploration",
		AfterValue:  after,
		By:          "tester",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	prof, err := svc.GetStrategy(context.Background(), "agent-json")
	if err != nil {
		t.Fatalf("get strategy: %v", err)
	}
	if prof.Exploration < 0.41 || prof.Exploration > 0.43 {
		t.Fatalf("expected exploration ≈ 0.42, got %.3f", prof.Exploration)
	}
}
