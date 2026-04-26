package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
	"arenea/backend/internal/runtime"
)

// §13 – ChatService exposes the agent-evolution model-routing accessor
// so future fallback / retry logic can pick the agent's preferred model
// from a candidate set.
func TestChatServiceRouteAgentModelCandidatesUsesPreference(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "chat-route.db")
	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	defer repo.Close()
	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := NewChatService(repo, runtime.NewADKRuntimeAdapter())
	if _, err := svc.AgentEvolution().UpdateStrategy(context.Background(), "agent-route", StrategyPatch{
		ModelPreference: map[string]float64{
			"openai/gpt-4o-mini": 0.95,
			"openai/gpt-3.5":     0.10,
		},
	}); err != nil {
		t.Fatalf("update strategy: %v", err)
	}
	candidates := []ModelCandidate{
		{ProviderKey: "openai", Model: "gpt-3.5", BaseScore: 1.0},
		{ProviderKey: "openai", Model: "gpt-4o-mini", BaseScore: 1.0},
	}
	out, err := svc.RouteAgentModelCandidates(context.Background(), "agent-route", candidates)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if len(out) != 2 || out[0].Model != "gpt-4o-mini" {
		t.Fatalf("expected gpt-4o-mini ranked first, got %#v", out)
	}
}

// §13 – RouteAgentModelCandidates returns the input unchanged when the
// agent has no preferences yet so default fallback ordering survives.
func TestChatServiceRouteAgentModelCandidatesPassthrough(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "chat-route-pass.db")
	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	defer repo.Close()
	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := NewChatService(repo, runtime.NewADKRuntimeAdapter())
	candidates := []ModelCandidate{
		{ProviderKey: "openai", Model: "gpt-4o-mini", BaseScore: 1.0},
		{ProviderKey: "openrouter", Model: "claude-3.5-sonnet", BaseScore: 0.5},
	}
	out, err := svc.RouteAgentModelCandidates(context.Background(), "agent-pass", candidates)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	// Without preferences both candidates collapse to base*0.5; ordering
	// is stable so the original first entry should remain first.
	if len(out) != 2 || out[0].Model != "gpt-4o-mini" {
		t.Fatalf("expected stable order, got %#v", out)
	}
}

func TestChatServiceSend(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("new repo failed: %v", err)
	}
	defer repo.Close()

	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}

	_, err = repo.CreateAgent(domain.Agent{
		ID:          "a1",
		AgentKey:    "default",
		DisplayName: "Default",
		Provider:    "openrouter",
		Model:       "gpt-4.1-mini",
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("create agent failed: %v", err)
	}

	_, err = repo.CreateSession(domain.Session{
		ID:      "s1",
		AgentID: "a1",
		Title:   "session",
	})
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	svc := NewChatService(repo, runtime.NewADKRuntimeAdapter())
	out, err := svc.Send(context.Background(), SendMessageInput{
		SessionID: "s1",
		AgentKey:  "default",
		Content:   "hello",
		Options: SendMessageOptions{
			DialogMode: "default",
			Provider:   "openai",
		},
	})
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if out.AgentMessage.Role != "assistant" {
		t.Fatalf("unexpected role: %s", out.AgentMessage.Role)
	}
	if out.UserMessage.OptionsJSON == "" {
		t.Fatal("expected user message options to be persisted")
	}
	usage, err := repo.GetModelUsageSummary(domain.ModelUsageQuery{})
	if err != nil {
		t.Fatalf("usage summary failed: %v", err)
	}
	if usage.CallCount != 1 || usage.TotalTokens == 0 {
		t.Fatalf("expected usage event to be recorded, got %#v", usage)
	}
}

func TestChatServiceSendRejectsAgentSessionMismatch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("new repo failed: %v", err)
	}
	defer repo.Close()

	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	if _, err = repo.CreateAgent(domain.Agent{ID: "a1", AgentKey: "one", DisplayName: "One", Provider: "openrouter", Model: "m"}); err != nil {
		t.Fatalf("create agent one failed: %v", err)
	}
	if _, err = repo.CreateAgent(domain.Agent{ID: "a2", AgentKey: "two", DisplayName: "Two", Provider: "openrouter", Model: "m"}); err != nil {
		t.Fatalf("create agent two failed: %v", err)
	}
	if _, err = repo.CreateSession(domain.Session{ID: "s1", AgentID: "a1", Title: "session"}); err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	svc := NewChatService(repo, runtime.NewADKRuntimeAdapter())
	_, err = svc.Send(context.Background(), SendMessageInput{
		SessionID: "s1",
		AgentKey:  "two",
		Content:   "hello",
	})
	if err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestChatServiceRunTeamParallelRecordsPartialFailure(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("new repo failed: %v", err)
	}
	defer repo.Close()

	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	if _, err = repo.CreateAgent(domain.Agent{ID: "a1", AgentKey: "one", DisplayName: "One", Provider: "openrouter", Model: "gpt-4.1-mini"}); err != nil {
		t.Fatalf("create agent failed: %v", err)
	}

	svc := NewChatService(repo, runtime.NewADKRuntimeAdapter())
	run := domain.TeamRun{ID: "run1", TeamID: "team1", SessionID: "s1", InputPreview: "hello", TopologyJSON: `{"mode":"parallel"}`}
	members := []teamMember{
		{AgentID: "a1", Role: "writer", Name: "Writer", SortOrder: 1},
		{AgentID: "missing", Role: "reviewer", Name: "Reviewer", SortOrder: 2},
	}

	steps, err := svc.runTeamParallel(context.Background(), run, members, SendMessageInput{SessionID: "s1", Content: "hello"}, domain.Session{ID: "s1"}, nil, 2, nil)
	if err != nil {
		t.Fatalf("expected partial success to return nil error, got %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if !hasSuccessfulTeamSteps(steps) || !hasFailedTeamSteps(steps) {
		t.Fatalf("expected mixed success/failure steps, got %#v", steps)
	}
	recorded, err := repo.ListTeamRunSteps("run1")
	if err != nil {
		t.Fatalf("list team run steps failed: %v", err)
	}
	if len(recorded) != 2 {
		t.Fatalf("expected 2 recorded steps, got %d", len(recorded))
	}
	var failed domain.TeamRunStep
	for _, item := range recorded {
		if item.Status != "success" {
			failed = item
		}
	}
	if failed.AgentID != "missing" || !strings.Contains(failed.ErrorMessage, `team member agent "missing" was not found`) {
		t.Fatalf("expected missing agent failure to be recorded, got %#v", failed)
	}
}
