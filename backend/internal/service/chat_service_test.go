package service

import (
	"context"
	"path/filepath"
	"testing"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
	"arenea/backend/internal/runtime"
)

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
