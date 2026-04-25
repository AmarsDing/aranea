// Package service – MemoryL2Service is the L2 episodic-memory façade
// described in `aranea/docs/14 memory-L2-episodic.md`. Phase 1 ships
// archival of L1 tasks into `memory_episodes`, the unified event view
// (UNION ALL across messages / tools / skills / model usage / team_run_steps),
// episode CRUD, and event Marks. Phase 2 adds BM25 indexing and Recall.
//
// The service intentionally has no goroutines of its own — index builds
// and consolidation are scheduled by the caller (cmd/server) so we can
// keep tests deterministic. Phase 1 tests can simply call BuildIndexFor
// inline.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// MemoryL2Service mediates between L1 tasks, episode storage, the unified
// event view, marks, and (Phase 2) Recall. The L1 dependency is narrow on
// purpose: the Snapshot-only contract (mirrors L1PromptSource) keeps the
// import graph acyclic and lets tests inject a stub.
type MemoryL2Service struct {
	repo     repository.Store
	memoryL1 L1SnapshotSource
	now      func() string
}

// L1SnapshotSource is the slim view of MemoryL1Service that L2 needs when
// archiving an L1 task into an episode. Implemented by *MemoryL1Service.
type L1SnapshotSource interface {
	SnapshotForEpisode(ctx context.Context, taskID string) (domain.L1Episode, error)
}

// NewMemoryL2Service builds the service over a repository and (optionally)
// an L1 source. Callers wire the L1 source via SetL1Source so tests can
// instantiate an L2 service without bringing the full L1 plumbing.
func NewMemoryL2Service(repo repository.Store) *MemoryL2Service {
	return &MemoryL2Service{repo: repo, now: nowUTC}
}

// SetL1Source attaches an L1 snapshot provider used during ArchiveL1Task.
// Nil disables L1-derived archival but keeps the milestone path live.
func (s *MemoryL2Service) SetL1Source(src L1SnapshotSource) { s.memoryL1 = src }

// SetClock overrides the clock for tests.
func (s *MemoryL2Service) SetClock(now func() string) {
	if now != nil {
		s.now = now
	}
}

// --- Inputs / outputs --------------------------------------------------------

// CreateEpisodeInput is the parameter object accepted by both the
// ArchiveL1Task path (after L1 snapshot extraction) and the manual
// CreateMilestoneEpisode path. The HTTP layer also reuses it for §6.3 POST.
type CreateEpisodeInput struct {
	SessionID      string                 `json:"session_id"`
	RunID          string                 `json:"run_id,omitempty"`
	TeamID         string                 `json:"team_id,omitempty"`
	AgentID        string                 `json:"agent_id,omitempty"`
	L1TaskID       string                 `json:"l1_task_id,omitempty"`
	Kind           domain.EpisodeKind     `json:"episode_kind,omitempty"`
	Title          string                 `json:"title"`
	Goal           string                 `json:"goal,omitempty"`
	Outcome        string                 `json:"outcome,omitempty"`
	OutcomeSummary string                 `json:"outcome_summary,omitempty"`
	ResultPreview  string                 `json:"result_preview,omitempty"`
	FailureReason  string                 `json:"failure_reason,omitempty"`
	Importance     float64                `json:"importance,omitempty"`
	Confidence     float64                `json:"confidence,omitempty"`
	UserFeedback   string                 `json:"user_feedback,omitempty"`
	CriticScore    float64                `json:"critic_score,omitempty"`
	KeyDecisions   []domain.L2KeyDecision `json:"key_decisions,omitempty"`
	KeyArtifacts   []domain.L2KeyArtifact `json:"key_artifacts,omitempty"`
	Metadata       map[string]any         `json:"metadata,omitempty"`
	StartedAt      string                 `json:"started_at,omitempty"`
	EndedAt        string                 `json:"ended_at,omitempty"`
}

// EpisodeListResult is the wire shape of GET §6.3.
type EpisodeListResult struct {
	Items  []domain.MemoryEpisode `json:"items"`
	Total  int                    `json:"total"`
	Limit  int                    `json:"limit"`
	Offset int                    `json:"offset"`
}

// EpisodeDetail is the wire shape of GET §6.3 (single episode). Marks come
// from `memory_event_marks`; recent events come from a window of `ListL2Events`
// scoped to the same session and bounded by `started_at` / `ended_at`.
type EpisodeDetail struct {
	Episode domain.MemoryEpisode    `json:"episode"`
	Events  []domain.MemoryL2Event  `json:"events,omitempty"`
	Marks   []domain.MemoryEventMark `json:"marks,omitempty"`
	Summary string                  `json:"summary,omitempty"`
}

// EventListResult is the wire shape of GET §6.2.
type EventListResult struct {
	Items  []domain.MemoryL2Event `json:"items"`
	Total  int                    `json:"total"`
	Limit  int                    `json:"limit"`
	Offset int                    `json:"offset"`
}

// MarkInput is the wire shape of POST §6.4.
type MarkInput struct {
	EpisodeID string         `json:"episode_id,omitempty"`
	RefKind   string         `json:"ref_kind"`
	RefID     string         `json:"ref_id"`
	MarkType  string         `json:"mark_type"`
	MarkedBy  string         `json:"marked_by,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	Weight    float64        `json:"weight,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// RetentionReport is returned by ApplyRetention so the cron caller can emit
// metrics / audit log lines.
type RetentionReport struct {
	ArchivedEpisodes int `json:"archived_episodes"`
	DeletedEpisodes  int `json:"deleted_episodes"`
}

// --- Episode lifecycle -------------------------------------------------------

// ArchiveL1Task implements §5.2: pull the L1 snapshot, aggregate counters,
// derive importance, and create the episode (status=pending consolidation).
// It is idempotent on (session_id, l1_task_id) – when an active episode
// already exists for the task we return it unchanged.
func (s *MemoryL2Service) ArchiveL1Task(ctx context.Context, l1TaskID string) (domain.MemoryEpisode, error) {
	if l1TaskID == "" {
		return domain.MemoryEpisode{}, validationError("l1_task_id is required")
	}
	if s.memoryL1 == nil {
		return domain.MemoryEpisode{}, errors.New("L1 source not configured")
	}
	snap, err := s.memoryL1.SnapshotForEpisode(ctx, l1TaskID)
	if err != nil {
		return domain.MemoryEpisode{}, err
	}
	if snap.SessionID == "" {
		return domain.MemoryEpisode{}, validationError("snapshot session_id is empty")
	}
	settings := s.resolveSettings(snap.AgentID)
	if !settings.EpisodeEnabled {
		return domain.MemoryEpisode{}, nil
	}
	if existing, ok := s.findExistingEpisodeForTask(snap.SessionID, l1TaskID); ok {
		return existing, nil
	}

	stats := s.collectSessionStats(snap.SessionID, snap.AgentID, snap.StartedAt, snap.EndedAt)
	keyDecisions, keyArtifacts := extractKeyDecisionsArtifacts(snap)
	importance := computeImportance(snap, stats)
	if importance < 0 {
		importance = 0
	} else if importance > 1 {
		importance = 1
	}

	now := s.now()
	endedAt := snap.EndedAt
	if endedAt == "" {
		endedAt = now
	}
	startedAt := snap.StartedAt
	if startedAt == "" {
		startedAt = endedAt
	}
	durationMS := computeDurationMS(startedAt, endedAt)

	title := snap.TaskTitle
	if title == "" {
		title = snap.TaskKey
	}
	if title == "" {
		title = "L1 task"
	}

	snapJSON, _ := json.Marshal(snap)
	metadataJSON := encodeMetadataJSON(map[string]any{
		"l1_used_tokens":   snap.UsedTokens,
		"l1_budget_tokens": snap.BudgetTokens,
		"l1_status":        string(snap.Status),
	})

	episode := domain.MemoryEpisode{
		ID:                  newID(),
		SessionID:           snap.SessionID,
		AgentID:             snap.AgentID,
		L1TaskID:            l1TaskID,
		Kind:                domain.EpisodeKindTask,
		Title:               title,
		Goal:                snap.TaskGoal,
		Outcome:             outcomeForStatus(snap.Status),
		OutcomeSummary:      "",
		ResultPreview:       previewText(snap.TaskGoal, l0PreviewLimit),
		Importance:          importance,
		Confidence:          0.7,
		CriticScore:         -1,
		MessageCount:        stats.MessageCount,
		ToolCallCount:       stats.ToolCallCount,
		SkillCallCount:      stats.SkillCallCount,
		MCPCallCount:        stats.MCPCallCount,
		TotalTokens:         stats.TotalTokens,
		TotalCostMicroUSD:   stats.TotalCostMicroUSD,
		DurationMS:          durationMS,
		L1SnapshotJSON:      string(snapJSON),
		KeyDecisionsJSON:    encodeKeyDecisions(keyDecisions),
		KeyArtifactsJSON:    encodeKeyArtifacts(keyArtifacts),
		ConsolidationStatus: "pending",
		EmbeddingStatus:     "pending",
		StartedAt:           startedAt,
		EndedAt:             endedAt,
		MetadataJSON:        metadataJSON,
	}
	created, err := s.repo.CreateEpisode(episode)
	if err != nil {
		return domain.MemoryEpisode{}, err
	}
	_ = s.audit("l2.archive_task", "memory_episodes", created.ID, map[string]any{
		"session":    created.SessionID,
		"agent":      created.AgentID,
		"l1_task":    l1TaskID,
		"importance": created.Importance,
	})
	if settings.IndexEnabled {
		// Best-effort: index failures must never block archival.
		_ = s.BuildIndexFor(ctx, created.ID)
	}
	return created, nil
}

// CreateMilestoneEpisode is the §5.4 user / Critic / Plugin entry point.
// Unlike ArchiveL1Task it does not require an L1 snapshot — callers supply
// the title / goal / outcome directly. Importance defaults to 0.6 (above
// the typical consolidation floor) so the consolidation worker picks it up
// quickly.
func (s *MemoryL2Service) CreateMilestoneEpisode(ctx context.Context, in CreateEpisodeInput) (domain.MemoryEpisode, error) {
	_ = ctx
	if in.SessionID == "" {
		return domain.MemoryEpisode{}, validationError("session_id is required")
	}
	if in.Title == "" {
		return domain.MemoryEpisode{}, validationError("title is required")
	}
	kind := in.Kind
	if kind == "" {
		kind = domain.EpisodeKindMilestone
	}
	if !kind.IsValid() {
		return domain.MemoryEpisode{}, validationError("invalid episode_kind: %q", string(kind))
	}
	importance := in.Importance
	if importance == 0 {
		importance = 0.6
	}
	confidence := in.Confidence
	if confidence == 0 {
		confidence = 0.7
	}
	criticScore := in.CriticScore
	if criticScore == 0 {
		criticScore = -1
	}
	now := s.now()
	endedAt := in.EndedAt
	if endedAt == "" {
		endedAt = now
	}
	startedAt := in.StartedAt
	if startedAt == "" {
		startedAt = endedAt
	}
	episode := domain.MemoryEpisode{
		ID:                  newID(),
		SessionID:           in.SessionID,
		RunID:               in.RunID,
		TeamID:              in.TeamID,
		AgentID:             in.AgentID,
		L1TaskID:            in.L1TaskID,
		Kind:                kind,
		Title:               in.Title,
		Goal:                in.Goal,
		Outcome:             firstNonEmptyString(in.Outcome, "success"),
		OutcomeSummary:      in.OutcomeSummary,
		ResultPreview:       previewText(firstNonEmptyString(in.ResultPreview, in.OutcomeSummary, in.Goal), l0PreviewLimit),
		FailureReason:       in.FailureReason,
		Importance:          importance,
		Confidence:          confidence,
		UserFeedback:        in.UserFeedback,
		CriticScore:         criticScore,
		KeyDecisionsJSON:    encodeKeyDecisions(in.KeyDecisions),
		KeyArtifactsJSON:    encodeKeyArtifacts(in.KeyArtifacts),
		ConsolidationStatus: "pending",
		EmbeddingStatus:     "pending",
		StartedAt:           startedAt,
		EndedAt:             endedAt,
		MetadataJSON:        encodeMetadataJSON(in.Metadata),
		DurationMS:          computeDurationMS(startedAt, endedAt),
	}
	created, err := s.repo.CreateEpisode(episode)
	if err != nil {
		return domain.MemoryEpisode{}, err
	}
	_ = s.audit("l2.create_milestone", "memory_episodes", created.ID, map[string]any{
		"session": created.SessionID,
		"agent":   created.AgentID,
		"kind":    string(created.Kind),
	})
	settings := s.resolveSettings(in.AgentID)
	if settings.IndexEnabled {
		_ = s.BuildIndexFor(ctx, created.ID)
	}
	return created, nil
}

// UpdateEpisode mutates the editable fields of an episode. Callers must pass
// the full row (typical pattern: GET → mutate → PATCH). The repository
// preserves embedding / consolidation status so the worker can keep going.
func (s *MemoryL2Service) UpdateEpisode(ctx context.Context, ep domain.MemoryEpisode) (domain.MemoryEpisode, error) {
	_ = ctx
	if ep.ID == "" {
		return domain.MemoryEpisode{}, validationError("id is required")
	}
	if err := s.repo.UpdateEpisode(ep); err != nil {
		return domain.MemoryEpisode{}, err
	}
	updated, err := s.repo.GetEpisode(ep.ID)
	if err != nil {
		return domain.MemoryEpisode{}, err
	}
	_ = s.audit("l2.update_episode", "memory_episodes", updated.ID, map[string]any{
		"session": updated.SessionID,
	})
	return updated, nil
}

// DeleteEpisode is a soft delete. The underlying events stay queryable.
func (s *MemoryL2Service) DeleteEpisode(ctx context.Context, id string) error {
	_ = ctx
	if id == "" {
		return validationError("id is required")
	}
	if err := s.repo.SoftDeleteEpisode(id); err != nil {
		return err
	}
	_ = s.audit("l2.delete_episode", "memory_episodes", id, nil)
	return nil
}

// GetEpisode loads an episode plus its marks and a recent slice of events.
// The events window uses the episode timestamps when available so callers
// see only related rows.
func (s *MemoryL2Service) GetEpisode(ctx context.Context, id string) (EpisodeDetail, error) {
	_ = ctx
	if id == "" {
		return EpisodeDetail{}, validationError("id is required")
	}
	ep, err := s.repo.GetEpisode(id)
	if err != nil {
		return EpisodeDetail{}, err
	}
	marks, _ := s.repo.ListMarksForEpisode(ep.ID)
	events, _, _ := s.repo.ListL2Events(domain.MemoryL2EventQuery{
		SessionID:    ep.SessionID,
		StartTimeUTC: ep.StartedAt,
		EndTimeUTC:   ep.EndedAt,
		Limit:        100,
	})
	return EpisodeDetail{
		Episode: ep,
		Events:  events,
		Marks:   marks,
		Summary: ep.OutcomeSummary,
	}, nil
}

// ListEpisodes returns paginated episodes for a session.
func (s *MemoryL2Service) ListEpisodes(ctx context.Context, sessionID, kind string, limit, offset int) (EpisodeListResult, error) {
	_ = ctx
	if sessionID == "" {
		return EpisodeListResult{}, validationError("session_id is required")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	items, total, err := s.repo.ListEpisodes(sessionID, kind, limit, offset)
	if err != nil {
		return EpisodeListResult{}, err
	}
	if items == nil {
		items = []domain.MemoryEpisode{}
	}
	return EpisodeListResult{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

// ListEvents proxies to the repository's UNION ALL query.
func (s *MemoryL2Service) ListEvents(ctx context.Context, q domain.MemoryL2EventQuery) (EventListResult, error) {
	_ = ctx
	if q.SessionID == "" {
		return EventListResult{}, validationError("session_id is required")
	}
	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	q.Limit = limit
	q.Offset = offset
	items, total, err := s.repo.ListL2Events(q)
	if err != nil {
		return EventListResult{}, err
	}
	if items == nil {
		items = []domain.MemoryL2Event{}
	}
	return EventListResult{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

// --- Marks -------------------------------------------------------------------

// Mark applies a §3.4 mark and bumps the linked episode's importance per
// §5.4 (star ⇒ +0.2; consolidate ⇒ +0.15 etc., capped at 1.0). When
// EpisodeID is empty and the ref is itself an episode, we use ref_id.
func (s *MemoryL2Service) Mark(ctx context.Context, in MarkInput) (domain.MemoryEventMark, error) {
	_ = ctx
	if in.RefKind == "" || in.RefID == "" || in.MarkType == "" {
		return domain.MemoryEventMark{}, validationError("ref_kind, ref_id and mark_type are required")
	}
	mark := domain.MemoryEventMark{
		EpisodeID: in.EpisodeID,
		RefKind:   in.RefKind,
		RefID:     in.RefID,
		MarkType:  in.MarkType,
		MarkedBy:  in.MarkedBy,
		Reason:    in.Reason,
		Weight:    in.Weight,
		Metadata:  in.Metadata,
	}
	if mark.EpisodeID == "" && in.RefKind == "episode" {
		mark.EpisodeID = in.RefID
	}
	// session_id resolution: prefer the linked episode; fall back to ""
	// (the repository will reject ""). For non-episode refs the caller
	// must pass episode_id explicitly so we can resolve the session.
	sessionID, err := s.resolveSessionForMark(mark)
	if err != nil {
		return domain.MemoryEventMark{}, err
	}
	mark.SessionID = sessionID

	stored, err := s.repo.UpsertEventMark(mark)
	if err != nil {
		return domain.MemoryEventMark{}, err
	}
	if stored.EpisodeID != "" {
		s.adjustImportanceForMark(stored.EpisodeID, in.MarkType)
	}
	_ = s.audit("l2.mark", "memory_event_marks", stored.ID, map[string]any{
		"ref_kind":  stored.RefKind,
		"ref_id":    stored.RefID,
		"mark_type": stored.MarkType,
		"episode":   stored.EpisodeID,
	})
	return stored, nil
}

// UnMark soft-deletes a mark by ID. Importance is left as-is so a
// subsequent re-mark doesn't double-count.
func (s *MemoryL2Service) UnMark(ctx context.Context, id string) error {
	_ = ctx
	if id == "" {
		return validationError("id is required")
	}
	if err := s.repo.SoftDeleteEventMark(id); err != nil {
		return err
	}
	_ = s.audit("l2.unmark", "memory_event_marks", id, nil)
	return nil
}

// ListMarks returns recent marks for a session (optionally filtered by type).
func (s *MemoryL2Service) ListMarks(ctx context.Context, sessionID, markType string, limit int) ([]domain.MemoryEventMark, error) {
	_ = ctx
	if sessionID == "" {
		return nil, validationError("session_id is required")
	}
	return s.repo.ListEventMarks(sessionID, markType, limit)
}

// --- Recall (Phase 2 surface, BM25-only) ------------------------------------

// RecallByQuery executes the §5.3 fusion against the BM25 index. Vector
// recall lands once embeddings are wired (Phase 3); the function shape is
// stable so callers don't need to migrate later.
func (s *MemoryL2Service) RecallByQuery(ctx context.Context, q domain.MemoryL2RecallQuery) ([]domain.MemoryL2RecallResult, error) {
	_ = ctx
	if q.SessionID == "" {
		return nil, validationError("session_id is required")
	}
	settings := s.resolveSettings(q.AgentID)
	if !settings.RecallEnabled && q.AgentID != "" {
		return nil, nil
	}
	topK := q.TopK
	if topK <= 0 {
		topK = settings.RecallMax
	}
	if topK <= 0 {
		topK = 5
	}
	min := q.MinImportance
	if min <= 0 {
		min = 0
	}
	results, err := s.repo.SearchL2BM25(q.SessionID, q.Query, min, topK*2)
	if err != nil {
		return nil, err
	}
	results = applyKindFilter(results, q.IncludeKinds)
	results = fuseRecallScores(results)
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

// RecallSegmentForL0 returns a prompt-ready segment when the agent has L2
// recall enabled and there are matching episodes. The L0 service injects it
// alongside L3 / L4 segments. The function NEVER errors – missing data
// silently returns ok=false so the L0 happy path stays branch-free.
func (s *MemoryL2Service) RecallSegmentForL0(ctx context.Context, sessionID, agentID, query string) (domain.L0Segment, bool) {
	if sessionID == "" {
		return domain.L0Segment{}, false
	}
	settings := s.resolveSettings(agentID)
	if !settings.RecallEnabled {
		return domain.L0Segment{}, false
	}
	results, err := s.RecallByQuery(ctx, domain.MemoryL2RecallQuery{
		SessionID: sessionID,
		AgentID:   agentID,
		Query:     query,
		TopK:      settings.RecallMax,
	})
	if err != nil || len(results) == 0 {
		return domain.L0Segment{}, false
	}
	body := renderRecallMarkdown(results)
	if strings.TrimSpace(body) == "" {
		return domain.L0Segment{}, false
	}
	return domain.L0Segment{
		Section: "memory.l2",
		Role:    "system",
		Source:  fmt.Sprintf("memory.l2:recall(%d)", len(results)),
		Tokens:  estimateTokensApprox(body),
		Content: body,
		Preview: previewText(body, l0PreviewLimit),
	}, true
}

// --- Indexing ---------------------------------------------------------------

// BuildIndexFor renders the FTS5 row for an episode. Phase 1 picks a small
// concatenation of (title, goal, outcome_summary, result_preview, key
// decisions) — enough for the BM25 ranker; Phase 3 will bolt on embeddings.
func (s *MemoryL2Service) BuildIndexFor(ctx context.Context, episodeID string) error {
	_ = ctx
	if episodeID == "" {
		return validationError("episode_id is required")
	}
	ep, err := s.repo.GetEpisode(episodeID)
	if err != nil {
		return err
	}
	text := buildIndexText(ep)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	entry := domain.MemoryL2IndexEntry{
		EpisodeID:     ep.ID,
		SessionID:     ep.SessionID,
		AgentID:       ep.AgentID,
		TextKind:      "episode",
		TextPreview:   previewText(text, l0PreviewLimit),
		TokenEstimate: estimateTokensApprox(text),
		Importance:    ep.Importance,
	}
	if err := s.repo.UpsertL2Index(entry, text); err != nil {
		return err
	}
	if err := s.repo.UpdateEpisodeEmbedding(ep.ID, "skipped", "", 0, 0); err != nil {
		return err
	}
	return nil
}

// --- Retention --------------------------------------------------------------

// ApplyRetention archives episodes older than `archive_after_days` and
// hard-deletes archived/deleted rows older than `retention_days`. Both
// thresholds are read from the agent's runtime settings; missing rows
// fall back to spec defaults (90 days retention / 30 days archival).
func (s *MemoryL2Service) ApplyRetention(ctx context.Context) (RetentionReport, error) {
	_ = ctx
	settings := s.resolveSettings("")
	now := time.Now().UTC()
	archiveBefore := now.Add(-time.Duration(settings.ArchiveAfterDays) * 24 * time.Hour).Format(time.RFC3339)
	retentionBefore := now.Add(-time.Duration(settings.RetentionDays) * 24 * time.Hour).Format(time.RFC3339)
	archived, err := s.repo.ArchiveEpisodesBeforeDate("", archiveBefore)
	if err != nil {
		return RetentionReport{}, err
	}
	deleted, err := s.repo.DeleteArchivedEpisodesBefore(retentionBefore)
	if err != nil {
		return RetentionReport{}, err
	}
	_ = s.audit("l2.retention", "memory_episodes", "", map[string]any{
		"archived": archived,
		"deleted":  deleted,
	})
	return RetentionReport{ArchivedEpisodes: archived, DeletedEpisodes: deleted}, nil
}

// --- internals ---------------------------------------------------------------

type l2Settings struct {
	EpisodeEnabled       bool
	EpisodeMinImportance float64
	IndexEnabled         bool
	IndexEmbeddingModel  string
	RecallEnabled        bool
	RecallMax            int
	RetentionDays        int
	ArchiveAfterDays     int
}

func (s *MemoryL2Service) resolveSettings(agentID string) l2Settings {
	out := l2Settings{
		EpisodeEnabled:       true,
		EpisodeMinImportance: 0.3,
		IndexEnabled:         true,
		RecallEnabled:        false,
		RecallMax:            3,
		RetentionDays:        90,
		ArchiveAfterDays:     30,
	}
	if agentID == "" {
		return out
	}
	row, err := s.repo.GetAgentRuntimeSettings(agentID)
	if err != nil {
		return out
	}
	out.EpisodeEnabled = row.L2EpisodeEnabled
	if row.L2EpisodeMinImportance > 0 {
		out.EpisodeMinImportance = row.L2EpisodeMinImportance
	}
	out.IndexEnabled = row.L2IndexEnabled
	out.IndexEmbeddingModel = row.L2IndexEmbeddingModel
	out.RecallEnabled = row.L2RecallEnabled
	if row.L2RecallMax > 0 {
		out.RecallMax = row.L2RecallMax
	}
	if row.L2RetentionDays > 0 {
		out.RetentionDays = row.L2RetentionDays
	}
	if row.L2ArchiveAfterDays > 0 {
		out.ArchiveAfterDays = row.L2ArchiveAfterDays
	}
	return out
}

func (s *MemoryL2Service) findExistingEpisodeForTask(sessionID, l1TaskID string) (domain.MemoryEpisode, bool) {
	episodes, _, err := s.repo.ListEpisodes(sessionID, "", 50, 0)
	if err != nil {
		return domain.MemoryEpisode{}, false
	}
	for _, ep := range episodes {
		if ep.L1TaskID == l1TaskID && ep.DeletedAt == "" {
			return ep, true
		}
	}
	return domain.MemoryEpisode{}, false
}

// resolveSessionForMark resolves the session_id for a new mark. Episode-
// scoped marks read from the linked episode; everything else defers to
// the caller (front-end) so we never have to reverse-engineer the table
// of origin.
func (s *MemoryL2Service) resolveSessionForMark(m domain.MemoryEventMark) (string, error) {
	if m.EpisodeID != "" {
		ep, err := s.repo.GetEpisode(m.EpisodeID)
		if err == nil && ep.SessionID != "" {
			return ep.SessionID, nil
		}
	}
	if m.SessionID != "" {
		return m.SessionID, nil
	}
	return "", validationError("session_id (or episode_id) is required for mark")
}

// adjustImportanceForMark bumps the importance per §5.4. Best-effort —
// failures are silent so the mark itself still lands.
func (s *MemoryL2Service) adjustImportanceForMark(episodeID, markType string) {
	delta := 0.0
	switch strings.ToLower(strings.TrimSpace(markType)) {
	case "star", "pin", "good_example":
		delta = 0.2
	case "consolidate", "critic_pass":
		delta = 0.15
	case "postmortem", "bad_example":
		delta = 0.1
	case "forget":
		delta = -0.3
	}
	if delta == 0 {
		return
	}
	ep, err := s.repo.GetEpisode(episodeID)
	if err != nil {
		return
	}
	next := ep.Importance + delta
	if next > 1 {
		next = 1
	} else if next < 0 {
		next = 0
	}
	if math.Abs(next-ep.Importance) < 1e-6 {
		return
	}
	ep.Importance = next
	_ = s.repo.UpdateEpisode(ep)
}

func (s *MemoryL2Service) audit(action, resource, resourceID string, detail map[string]any) error {
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

// l2SessionStats aggregates quick counters for an episode header.
type l2SessionStats struct {
	MessageCount      int
	ToolCallCount     int
	SkillCallCount    int
	MCPCallCount      int
	TotalTokens       int
	TotalCostMicroUSD int64
}

// collectSessionStats walks the unified event view to fill the counters
// stored on the episode. The window respects (started_at, ended_at) when
// both are set; otherwise we use the entire session.
func (s *MemoryL2Service) collectSessionStats(sessionID, agentID, startedAt, endedAt string) l2SessionStats {
	var stats l2SessionStats
	events, _, err := s.repo.ListL2Events(domain.MemoryL2EventQuery{
		SessionID:    sessionID,
		StartTimeUTC: startedAt,
		EndTimeUTC:   endedAt,
		Limit:        500,
	})
	if err != nil {
		return stats
	}
	for _, ev := range events {
		if agentID != "" && ev.ActorID != "" && ev.ActorID != agentID {
			continue
		}
		switch ev.Kind {
		case "message":
			stats.MessageCount++
		case "tool_call":
			stats.ToolCallCount++
		case "skill_call":
			stats.SkillCallCount++
		case "mcp_call":
			stats.MCPCallCount++
		case "model_call":
			stats.TotalTokens += ev.TokensIn + ev.TokensOut
			stats.TotalCostMicroUSD += ev.CostMicro
		}
	}
	return stats
}

// computeImportance is the §5.2 step-5 scoring formula. It is intentionally
// linear and bounded so the front-end can show the contributing factors.
func computeImportance(snap domain.L1Episode, stats l2SessionStats) float64 {
	imp := 0.3
	if snap.Status == domain.L1TaskCompleted {
		imp += 0.2
	}
	if stats.MessageCount > 0 {
		imp += 0.1
	}
	if stats.ToolCallCount > 0 || stats.SkillCallCount > 0 {
		imp += 0.1
	}
	used := snap.UsedTokens
	budget := snap.BudgetTokens
	if budget > 0 {
		ratio := float64(used) / float64(budget)
		if ratio > 0.5 {
			imp += 0.1
		}
		if ratio > 0.8 {
			imp += 0.1
		}
	}
	if imp > 1 {
		imp = 1
	}
	return imp
}

// extractKeyDecisionsArtifacts walks the L1 snapshot picking out fields
// whose path starts with "decisions." or "artifacts." Phase 1 keeps the
// extractor simple — just pull the rendered values; future phases will
// hydrate richer metadata from `session_trace_spans`.
func extractKeyDecisionsArtifacts(snap domain.L1Episode) ([]domain.L2KeyDecision, []domain.L2KeyArtifact) {
	var decisions []domain.L2KeyDecision
	var artifacts []domain.L2KeyArtifact
	for path, raw := range snap.Snapshot {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		value := fmt.Sprintf("%v", entry["value"])
		switch {
		case strings.HasPrefix(path, "decisions."):
			decisions = append(decisions, domain.L2KeyDecision{
				Decision: strings.TrimPrefix(path, "decisions."),
				Rationale: value,
				At:        fmt.Sprintf("%v", entry["updated_at"]),
			})
		case strings.HasPrefix(path, "artifacts."):
			artifacts = append(artifacts, domain.L2KeyArtifact{
				Kind:    strings.TrimPrefix(path, "artifacts."),
				Ref:     value,
				Preview: previewText(value, 120),
			})
		}
	}
	sort.Slice(decisions, func(i, j int) bool { return decisions[i].Decision < decisions[j].Decision })
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Kind < artifacts[j].Kind })
	return decisions, artifacts
}

func encodeKeyDecisions(values []domain.L2KeyDecision) string {
	if len(values) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func encodeKeyArtifacts(values []domain.L2KeyArtifact) string {
	if len(values) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func encodeMetadataJSON(meta map[string]any) string {
	if len(meta) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func outcomeForStatus(status domain.L1TaskStatus) string {
	switch status {
	case domain.L1TaskCompleted:
		return "success"
	case domain.L1TaskFailed:
		return "failed"
	case domain.L1TaskCancelled:
		return "cancelled"
	case domain.L1TaskArchived:
		return "archived"
	}
	return "partial"
}

func computeDurationMS(startedAt, endedAt string) int {
	if startedAt == "" || endedAt == "" {
		return 0
	}
	start, err1 := time.Parse(time.RFC3339, startedAt)
	end, err2 := time.Parse(time.RFC3339, endedAt)
	if err1 != nil || err2 != nil {
		return 0
	}
	d := end.Sub(start)
	if d < 0 {
		return 0
	}
	return int(d.Milliseconds())
}

func applyKindFilter(results []domain.MemoryL2RecallResult, kinds []domain.EpisodeKind) []domain.MemoryL2RecallResult {
	if len(kinds) == 0 {
		return results
	}
	allowed := make(map[domain.EpisodeKind]bool, len(kinds))
	for _, k := range kinds {
		allowed[k] = true
	}
	out := results[:0]
	for _, r := range results {
		if allowed[r.Episode.Kind] {
			out = append(out, r)
		}
	}
	return out
}

// fuseRecallScores normalises BM25 (already flipped to "higher = better")
// into [0,1] and computes the §5.3 weighted final rank. With BM25 being
// the only signal in Phase 1, the fused score is dominated by it; the
// importance term keeps high-value episodes from being buried by very
// recent low-score hits.
func fuseRecallScores(results []domain.MemoryL2RecallResult) []domain.MemoryL2RecallResult {
	if len(results) == 0 {
		return results
	}
	max := results[0].BM25Score
	for _, r := range results {
		if r.BM25Score > max {
			max = r.BM25Score
		}
	}
	if max <= 0 {
		max = 1
	}
	for i := range results {
		bm := results[i].BM25Score / max
		final := 0.7*bm + 0.3*results[i].Episode.Importance
		results[i].FinalRank = final
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].FinalRank > results[j].FinalRank })
	return results
}

// renderRecallMarkdown formats recall results as a compact bullet list so
// the LLM can absorb them with minimal prompt overhead. Only summary
// fields are rendered (per §9: "Recall 只返回摘要").
func renderRecallMarkdown(results []domain.MemoryL2RecallResult) string {
	var b strings.Builder
	b.WriteString("## Episodic Memory (recall)\n")
	for _, r := range results {
		ep := r.Episode
		title := ep.Title
		if title == "" {
			title = ep.Goal
		}
		summary := ep.OutcomeSummary
		if summary == "" {
			summary = ep.ResultPreview
		}
		fmt.Fprintf(&b, "- **%s** (%s, importance=%.2f): %s\n",
			title, ep.Outcome, ep.Importance, previewText(summary, 160))
	}
	return strings.TrimRight(b.String(), "\n")
}

// buildIndexText concatenates the searchable surface of an episode. We
// repeat the title to give it more weight in BM25 (FTS5 doesn't support
// per-field boosts in Phase 1).
func buildIndexText(ep domain.MemoryEpisode) string {
	var parts []string
	if ep.Title != "" {
		parts = append(parts, ep.Title, ep.Title)
	}
	if ep.Goal != "" {
		parts = append(parts, ep.Goal)
	}
	if ep.OutcomeSummary != "" {
		parts = append(parts, ep.OutcomeSummary)
	}
	if ep.ResultPreview != "" {
		parts = append(parts, ep.ResultPreview)
	}
	if ep.FailureReason != "" {
		parts = append(parts, ep.FailureReason)
	}
	if ep.KeyDecisionsJSON != "" && ep.KeyDecisionsJSON != "[]" {
		parts = append(parts, ep.KeyDecisionsJSON)
	}
	return strings.Join(parts, "\n")
}
