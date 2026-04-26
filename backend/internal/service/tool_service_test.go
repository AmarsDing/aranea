// tool_service_test.go covers the agent-evolution → tool-policy wiring
// added in Phase 3 (§5.10): ToolService.EffectiveForAgent must downgrade
// blacklisted tools to denied with reason="evolution_blacklist" and
// reorder allowed items by the agent's strategy.tool_preference scores.
package service

import (
	"context"
	"path/filepath"
	"testing"

	"arenea/backend/internal/repository"
)

func newTestToolService(t *testing.T) (*ToolService, *AgentEvolutionService, repository.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tools.db")
	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tools := NewToolService(repo)
	evo := NewAgentEvolutionService(repo)
	tools.SetEvolutionPolicySource(evo)
	return tools, evo, repo
}

// §13 – evolution blacklist must surface in EffectiveForAgent as
// state=denied / reason="evolution_blacklist" without disturbing the
// rest of the catalog.
func TestToolEffectiveAppliesEvolutionBlacklist(t *testing.T) {
	tools, evo, _ := newTestToolService(t)
	bl := []string{"shell_exec"}
	if _, err := evo.UpdateStrategy(context.Background(), "agent-bl", StrategyPatch{
		ToolBlacklist: &bl,
	}); err != nil {
		t.Fatalf("update strategy: %v", err)
	}
	view, err := tools.EffectiveForAgent("agent-bl")
	if err != nil {
		t.Fatalf("effective: %v", err)
	}
	var found bool
	for _, item := range view.Items {
		if item.ToolKey == "shell_exec" {
			found = true
			if item.EffectiveState != "denied" || item.Reason != "evolution_blacklist" || item.Enabled {
				t.Fatalf("expected shell_exec denied via evolution_blacklist, got %#v", item)
			}
		}
	}
	if !found {
		t.Fatalf("expected shell_exec in seeded tool catalog")
	}
}

// §13 – tool_preference scores must reorder allowed items so the
// highest-scoring tool ranks first in the prompt-rendered view.
func TestToolEffectiveSortsByEvolutionPreference(t *testing.T) {
	tools, evo, _ := newTestToolService(t)
	if _, err := evo.UpdateStrategy(context.Background(), "agent-pref", StrategyPatch{
		ToolPreference: map[string]float64{
			"datetime":   0.95,
			"web_search": 0.10,
		},
	}); err != nil {
		t.Fatalf("update strategy: %v", err)
	}
	view, err := tools.EffectiveForAgent("agent-pref")
	if err != nil {
		t.Fatalf("effective: %v", err)
	}
	var dtIdx, wsIdx = -1, -1
	for i, item := range view.Items {
		if item.EffectiveState != "allowed" {
			continue
		}
		if item.ToolKey == "datetime" {
			dtIdx = i
		}
		if item.ToolKey == "web_search" {
			wsIdx = i
		}
	}
	if dtIdx < 0 || wsIdx < 0 {
		t.Fatalf("expected both datetime and web_search to be allowed, got dt=%d ws=%d", dtIdx, wsIdx)
	}
	if dtIdx >= wsIdx {
		t.Fatalf("expected datetime before web_search after preference reorder, got dt=%d ws=%d", dtIdx, wsIdx)
	}
}

// Without a wired evolution source ToolService must still produce a
// consistent view — guarding against accidental nil dereferences.
func TestToolEffectiveWithoutEvolutionSourceWorks(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tools.db")
	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	view, err := NewToolService(repo).EffectiveForAgent("agent-none")
	if err != nil {
		t.Fatalf("effective: %v", err)
	}
	if len(view.Items) == 0 {
		t.Fatalf("expected seeded tool catalog, got 0 items")
	}
}
