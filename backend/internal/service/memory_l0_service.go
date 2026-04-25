package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// MemoryL0Service is the assembly layer described in `12 memory-L0-sensory.md`.
// It does NOT own any data of its own: history is in `messages`, summaries are
// in `session_summaries`, agent prompts come from `agent_prompt_files`. The
// service only orchestrates ordering, token budgeting, truncation, and
// snapshot writing so ChatService / TeamRuntime can hand the LLM a clean
// `messages` slice on every call.
type MemoryL0Service struct {
	repo     repository.Store
	memoryL1 L1PromptSource
	memoryL2 L2RecallSource
}

// L1PromptSource is the narrow contract MemoryL0Service uses to render the
// optional L1 working-memory segment. The full MemoryL1Service (in
// memory_l1_service.go) implements it; the indirection avoids a circular
// import while still allowing dependency injection from ChatService.
type L1PromptSource interface {
	RenderActiveTaskForPrompt(ctx context.Context, sessionID, agentID string) (domain.L1PromptBlock, bool, error)
}

// L2RecallSource is the narrow contract MemoryL0Service uses to render the
// optional L2 episodic recall segment described in
// `aranea/docs/14 memory-L2-episodic.md` §5.3 / §5.4. Implemented by
// *MemoryL2Service. The seam keeps the L0 happy path branch-free when the
// recall feature is disabled (default in agent_runtime_settings).
type L2RecallSource interface {
	RecallSegmentForL0(ctx context.Context, sessionID, agentID, query string) (domain.L0Segment, bool)
}

func NewMemoryL0Service(repo repository.Store) *MemoryL0Service {
	return &MemoryL0Service{repo: repo}
}

// SetL1Source wires the MemoryL1Service into the L0 assembly pipeline. It is
// optional: when nil the L0 layer simply omits the L1 segment.
func (s *MemoryL0Service) SetL1Source(src L1PromptSource) {
	s.memoryL1 = src
}

// SetL2Source wires the MemoryL2Service into the L0 assembly pipeline. It is
// optional: when nil the L0 layer simply omits the L2 recall segment. The
// segment is also gated by `l2_recall_enabled` on agent_runtime_settings.
func (s *MemoryL0Service) SetL2Source(src L2RecallSource) {
	s.memoryL2 = src
}

// l0DefaultSafetyMargin reserves a few hundred tokens out of the model context
// window for output framing, function-call schemas, and SSE overhead. The
// budget formula is: max(0, context_window - reserved_for_output - safety).
const l0DefaultSafetyMargin = 256

// l0PreviewLimit caps the rune count of `Preview` written into snapshots and
// returned by /l0/preview. It is intentionally short — the full content lives
// in `messages` / `session_summaries` already.
const l0PreviewLimit = 200

// l0HardMessageCap bounds the messages-per-window scan even when token sums
// don't reach the budget (paranoia against runaway sessions).
const l0HardMessageCap = 200

// Assemble builds a model-ready prompt according to the request. The returned
// SnapshotID is empty when snapshot_mode = off and no warning was raised.
func (s *MemoryL0Service) Assemble(ctx context.Context, req domain.L0AssemblyRequest) (domain.L0AssemblyResult, error) {
	return s.assemble(ctx, req, false)
}

// Preview is identical to Assemble but never persists a snapshot, never marks
// session.context_status, and redacts every segment to its preview slice. It
// powers the `/l0/preview` debug API and the in-app prompt debugger.
func (s *MemoryL0Service) Preview(ctx context.Context, req domain.L0AssemblyRequest) (domain.L0AssemblyResult, error) {
	return s.assemble(ctx, req, true)
}

// RecordActual is called once the model usage is known so the snapshot can
// store the real prompt token count and the session can update its ratio /
// status. It is safe to pass an empty snapshotID when no snapshot was written.
func (s *MemoryL0Service) RecordActual(ctx context.Context, sessionID string, snapshotID string, actualPromptTokens int, contextWindow int) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	ratio := 0.0
	if contextWindow > 0 && actualPromptTokens > 0 {
		ratio = float64(actualPromptTokens) / float64(contextWindow)
		if math.IsInf(ratio, 0) || math.IsNaN(ratio) {
			ratio = 0
		}
		if ratio > 1 {
			ratio = 1
		}
	}
	if err := s.repo.UpdateSessionL0Context(sessionID, actualPromptTokens, contextWindow, ratio); err != nil {
		return err
	}
	if snapshotID == "" {
		return nil
	}
	return s.repo.UpdateL0AssemblySnapshotActualTokens(snapshotID, actualPromptTokens, ratio)
}

func (s *MemoryL0Service) ListSnapshots(ctx context.Context, sessionID string, limit int) ([]domain.L0AssemblySnapshot, error) {
	return s.repo.ListL0AssemblySnapshotsBySession(sessionID, limit)
}

func (s *MemoryL0Service) GetSnapshot(ctx context.Context, id string) (domain.L0AssemblySnapshot, error) {
	return s.repo.GetL0AssemblySnapshotByID(id)
}

// assemble is the shared core for Assemble / Preview. The `previewMode` flag
// strips the full `Content` from the returned segments and skips persistence.
func (s *MemoryL0Service) assemble(ctx context.Context, req domain.L0AssemblyRequest, previewMode bool) (domain.L0AssemblyResult, error) {
	if req.SessionID == "" {
		return domain.L0AssemblyResult{}, errors.New("session_id is required")
	}

	settings := s.resolveL0Settings(req.AgentID)

	contextWindow := req.ContextWindow
	if contextWindow <= 0 {
		contextWindow = 32_000
	}
	reservedOutput := req.ReservedForOutput
	if reservedOutput < 0 {
		reservedOutput = 0
	}
	budget := contextWindow - reservedOutput - l0DefaultSafetyMargin
	if budget <= 0 {
		budget = contextWindow / 2
	}

	segments := make([]domain.L0Segment, 0, 16)

	systemSegments, err := s.buildSystemSegments(req.AgentID, req.ExtraSystemBlocks)
	if err != nil {
		return domain.L0AssemblyResult{}, err
	}
	segments = append(segments, systemSegments...)

	if settings.InjectL1 {
		if seg, ok := s.buildL1Segment(ctx, req.SessionID, req.AgentID); ok {
			segments = append(segments, seg)
		}
	}
	if seg, ok := s.buildL2Segment(ctx, req.SessionID, req.AgentID, req.UserMessage); ok {
		segments = append(segments, seg)
	}
	if settings.InjectL3 {
		segments = append(segments, s.buildL3Segments(req.UserMessage, settings.L3MaxChunks)...)
	}
	if settings.InjectL4 {
		segments = append(segments, s.buildL4Segments(req.UserMessage, settings.L4MaxPaths)...)
	}

	summaries, err := s.repo.ListSessionSummaries(req.SessionID, 8)
	if err != nil {
		return domain.L0AssemblyResult{}, err
	}
	summarizedFrom, summarizedTo := 0, 0
	if seg, from, to, ok := s.buildSummarySegment(summaries); ok {
		segments = append(segments, seg)
		summarizedFrom, summarizedTo = from, to
	}

	tokenWindow := s.resolveTokenWindow(settings.RecentWindowTokens, budget)
	hardCap := s.resolveTurnCap(settings.RecentWindowTurns)

	history, err := s.repo.ListLatestMessagesByTokens(req.SessionID, tokenWindow, hardCap)
	if err != nil {
		return domain.L0AssemblyResult{}, err
	}

	historySegments, recentTurnsCount, recentTokensCount := s.buildHistorySegments(history)
	segments = append(segments, historySegments...)

	userInput := strings.TrimSpace(req.UserMessage)
	if userInput != "" {
		segments = append(segments, domain.L0Segment{
			Section: "user.input",
			Role:    "user",
			Source:  "messages[current]",
			Tokens:  estimateTokensApprox(userInput),
			Content: userInput,
			Preview: previewText(userInput, l0PreviewLimit),
		})
	}

	totalTokens := sumSegmentTokens(segments)
	warningCodes := []string{}
	truncatedCount := 0
	truncateStrategy := settings.TruncateStrategy
	if totalTokens > budget {
		segments, truncatedCount, warningCodes, truncateStrategy = applyTruncation(segments, budget, settings.TruncateStrategy)
		totalTokens = sumSegmentTokens(segments)
	}

	usedRatio := 0.0
	if contextWindow > 0 {
		usedRatio = float64(totalTokens) / float64(contextWindow)
		if math.IsInf(usedRatio, 0) || math.IsNaN(usedRatio) {
			usedRatio = 0
		}
		if usedRatio > 1 {
			warningCodes = appendUnique(warningCodes, "exceeded")
			usedRatio = 1
		} else if usedRatio >= 0.95 {
			warningCodes = appendUnique(warningCodes, "near_limit")
		}
	}

	if settings.InjectL3 && len(s.buildL3Segments(req.UserMessage, settings.L3MaxChunks)) == 0 {
		// best effort, no warning
	}

	promptMessages := assembleChatMessages(segments)

	result := domain.L0AssemblyResult{
		Segments:              redactSegments(segments, previewMode),
		PromptMessages:        promptMessages,
		BudgetTokens:          budget,
		PromptTokenEstimate:   totalTokens,
		UsedRatioEstimate:     usedRatio,
		RecentWindowTurns:     recentTurnsCount,
		RecentWindowTokens:    recentTokensCount,
		SummarizedTurnFrom:    summarizedFrom,
		SummarizedTurnTo:      summarizedTo,
		TruncateStrategy:      truncateStrategy,
		TruncatedMessageCount: truncatedCount,
		WarningCodes:          warningCodes,
	}

	if previewMode {
		result.PromptMessages = redactPromptMessages(result.PromptMessages)
		return result, nil
	}

	if shouldWriteSnapshot(settings.SnapshotMode, usedRatio, len(warningCodes) > 0) {
		snap := buildSnapshot(req, settings, result, segments, contextWindow, summarizedFrom, summarizedTo)
		if err := s.repo.InsertL0AssemblySnapshot(snap); err == nil {
			result.SnapshotID = snap.ID
		}
	}

	return result, nil
}

// resolveL0Settings reads the agent-level settings and falls back to the
// service defaults when the agent has not been configured yet (or has no
// row).
func (s *MemoryL0Service) resolveL0Settings(agentID string) domain.L0Settings {
	defaults := domain.L0Settings{
		RecentWindowTurns:  12,
		RecentWindowTokens: 0,
		SummaryThreshold:   0.6,
		SummaryKeepTurns:   4,
		TruncateStrategy:   "summary",
		InjectL1:           true,
		InjectL3:           true,
		InjectL4:           false,
		L3MaxChunks:        5,
		L4MaxPaths:         3,
		SnapshotMode:       "on_warning",
	}
	if agentID == "" {
		return defaults
	}
	settings, err := s.repo.GetAgentRuntimeSettings(agentID)
	if err != nil {
		return defaults
	}
	out := defaults
	if settings.L0RecentWindowTurns > 0 {
		out.RecentWindowTurns = settings.L0RecentWindowTurns
	}
	if settings.L0RecentWindowTokens > 0 {
		out.RecentWindowTokens = settings.L0RecentWindowTokens
	}
	if settings.L0SummaryThreshold > 0 {
		out.SummaryThreshold = settings.L0SummaryThreshold
	}
	if settings.L0SummaryKeepTurns > 0 {
		out.SummaryKeepTurns = settings.L0SummaryKeepTurns
	}
	if strings.TrimSpace(settings.L0TruncateStrategy) != "" {
		out.TruncateStrategy = settings.L0TruncateStrategy
	}
	out.InjectL1 = settings.L0InjectL1
	out.InjectL3 = settings.L0InjectL3
	out.InjectL4 = settings.L0InjectL4
	if settings.L0L3MaxChunks > 0 {
		out.L3MaxChunks = settings.L0L3MaxChunks
	}
	if settings.L0L4MaxPaths > 0 {
		out.L4MaxPaths = settings.L0L4MaxPaths
	}
	if strings.TrimSpace(settings.L0SnapshotMode) != "" {
		out.SnapshotMode = settings.L0SnapshotMode
	}
	return out
}

func (s *MemoryL0Service) buildSystemSegments(agentID string, extra []domain.L0Segment) ([]domain.L0Segment, error) {
	out := make([]domain.L0Segment, 0, 4+len(extra))
	if agentID != "" {
		agent, err := s.repo.GetAgentByID(agentID)
		if err == nil {
			if desc := strings.TrimSpace(agent.AgentDescription); desc != "" {
				out = append(out, domain.L0Segment{
					Section: "system.prompt",
					Role:    "system",
					Source:  "agent.description",
					Tokens:  estimateTokensApprox(desc),
					Content: desc,
					Preview: previewText(desc, l0PreviewLimit),
				})
			}
			files, err := s.repo.ListAgentPromptFiles(agentID)
			if err == nil {
				for _, file := range files {
					body := strings.TrimSpace(file.Body)
					if body == "" {
						continue
					}
					out = append(out, domain.L0Segment{
						Section: "system.prompt_file",
						Role:    "system",
						Source:  "agent_prompt_files:" + file.Name,
						Tokens:  estimateTokensApprox(body),
						Content: body,
						Preview: previewText(body, l0PreviewLimit),
					})
				}
			}
		}
	}
	for _, seg := range extra {
		if strings.TrimSpace(seg.Section) == "" {
			seg.Section = "system.extra"
		}
		if strings.TrimSpace(seg.Role) == "" {
			seg.Role = "system"
		}
		if seg.Tokens <= 0 {
			seg.Tokens = estimateTokensApprox(seg.Content)
		}
		if strings.TrimSpace(seg.Preview) == "" {
			seg.Preview = previewText(seg.Content, l0PreviewLimit)
		}
		out = append(out, seg)
	}
	return out, nil
}

// buildL1Segment delegates to the configured L1PromptSource. When the agent
// has no active L1 task or the source is missing the function returns false so
// the segment list stays clean.
func (s *MemoryL0Service) buildL1Segment(ctx context.Context, sessionID, agentID string) (domain.L0Segment, bool) {
	if s.memoryL1 == nil || sessionID == "" {
		return domain.L0Segment{}, false
	}
	block, ok, err := s.memoryL1.RenderActiveTaskForPrompt(ctx, sessionID, agentID)
	if err != nil || !ok {
		return domain.L0Segment{}, false
	}
	body := strings.TrimSpace(block.Content)
	if body == "" {
		return domain.L0Segment{}, false
	}
	tokens := block.Tokens
	if tokens <= 0 {
		tokens = estimateTokensApprox(body)
	}
	source := strings.TrimSpace(block.Source)
	if source == "" {
		source = "memory.l1"
	}
	return domain.L0Segment{
		Section: "memory.l1",
		Role:    "system",
		Source:  source,
		Tokens:  tokens,
		Content: body,
		Preview: previewText(body, l0PreviewLimit),
	}, true
}

// buildL2Segment delegates to the configured L2RecallSource. The MemoryL2
// service itself enforces the `l2_recall_enabled` flag and the per-agent
// recall_max so this method only has to translate "no source" / "no hits"
// into ok=false.
func (s *MemoryL0Service) buildL2Segment(ctx context.Context, sessionID, agentID, query string) (domain.L0Segment, bool) {
	if s.memoryL2 == nil || sessionID == "" {
		return domain.L0Segment{}, false
	}
	return s.memoryL2.RecallSegmentForL0(ctx, sessionID, agentID, query)
}

// buildL3Segments / buildL4Segments are intentionally empty for now: L3 / L4
// services are not yet implemented (`Phase 4` in the spec). They exist so the
// assembly flow has a single, stable seam.
func (s *MemoryL0Service) buildL3Segments(_ string, _ int) []domain.L0Segment { return nil }
func (s *MemoryL0Service) buildL4Segments(_ string, _ int) []domain.L0Segment { return nil }

func (s *MemoryL0Service) buildSummarySegment(summaries []domain.SessionSummary) (domain.L0Segment, int, int, bool) {
	if len(summaries) == 0 {
		return domain.L0Segment{}, 0, 0, false
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].FromTurn == summaries[j].FromTurn {
			return summaries[i].ToTurn < summaries[j].ToTurn
		}
		return summaries[i].FromTurn < summaries[j].FromTurn
	})
	var b strings.Builder
	from, to := summaries[0].FromTurn, summaries[0].ToTurn
	for i, s := range summaries {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "[summary turn %d-%d] %s", s.FromTurn, s.ToTurn, strings.TrimSpace(s.SummaryMarkdown))
		if s.FromTurn < from {
			from = s.FromTurn
		}
		if s.ToTurn > to {
			to = s.ToTurn
		}
	}
	body := b.String()
	return domain.L0Segment{
		Section: "summary",
		Role:    "system",
		Source:  fmt.Sprintf("session_summaries:%d", len(summaries)),
		Tokens:  estimateTokensApprox(body),
		Content: body,
		Preview: previewText(body, l0PreviewLimit),
	}, from, to, true
}

func (s *MemoryL0Service) buildHistorySegments(history []domain.Message) ([]domain.L0Segment, int, int) {
	out := make([]domain.L0Segment, 0, len(history))
	turns := 0
	totalTokens := 0
	for _, msg := range history {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "user" && role != "assistant" && role != "tool" && role != "system" {
			continue
		}
		body := strings.TrimSpace(msg.Content)
		if body == "" {
			continue
		}
		tokens := msg.TokenIn + msg.TokenOut
		if tokens <= 0 {
			tokens = estimateTokensApprox(body)
		}
		out = append(out, domain.L0Segment{
			Section: "history",
			Role:    role,
			Source:  "messages:" + msg.ID,
			Tokens:  tokens,
			Content: body,
			Preview: previewText(body, l0PreviewLimit),
		})
		totalTokens += tokens
		if role == "user" {
			turns++
		}
	}
	return out, turns, totalTokens
}

func (s *MemoryL0Service) resolveTokenWindow(configured int, budget int) int {
	if configured > 0 {
		return configured
	}
	if budget <= 0 {
		return 0
	}
	half := budget / 2
	if half < 1024 {
		return 1024
	}
	return half
}

func (s *MemoryL0Service) resolveTurnCap(turns int) int {
	if turns <= 0 {
		return l0HardMessageCap
	}
	return turns * 2 // user + assistant per turn
}

// applyTruncation trims segments until the prompt fits inside `budget`. It
// always preserves system segments, the summary block, and the user.input
// segment so the model still has the latest task. The strategy controls which
// of `drop_tool_results` / `drop_oldest` / `summary` runs in what order.
func applyTruncation(segments []domain.L0Segment, budget int, strategy string) ([]domain.L0Segment, int, []string, string) {
	warnings := []string{"truncated"}
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "drop_tool_results":
		segs, dropped := dropMatching(segments, func(seg domain.L0Segment) bool {
			return seg.Section == "history" && seg.Role == "tool"
		}, budget)
		return segs, dropped, warnings, "drop_tool_results"
	case "drop_oldest":
		segs, dropped := dropOldestHistory(segments, budget)
		return segs, dropped, warnings, "drop_oldest"
	case "hybrid":
		segs, dropped1 := dropMatching(segments, func(seg domain.L0Segment) bool {
			return seg.Section == "history" && seg.Role == "tool"
		}, budget)
		if sumSegmentTokens(segs) <= budget {
			return segs, dropped1, warnings, "hybrid"
		}
		segs, dropped2 := dropOldestHistory(segs, budget)
		return segs, dropped1 + dropped2, warnings, "hybrid"
	default: // summary fallback (without an active SummaryService) → drop_oldest
		segs, dropped := dropOldestHistory(segments, budget)
		return segs, dropped, append(warnings, "summary_unavailable"), "drop_oldest"
	}
}

func dropMatching(segments []domain.L0Segment, match func(domain.L0Segment) bool, budget int) ([]domain.L0Segment, int) {
	if sumSegmentTokens(segments) <= budget {
		return segments, 0
	}
	dropped := 0
	out := make([]domain.L0Segment, 0, len(segments))
	for _, seg := range segments {
		if match(seg) && sumSegmentTokens(out)+remainingTokens(out, segments)-seg.Tokens >= 0 && sumSegmentTokens(append(append([]domain.L0Segment{}, out...), seg)) > budget {
			dropped++
			continue
		}
		out = append(out, seg)
	}
	return out, dropped
}

// remainingTokens is a small helper used by dropMatching to ensure we don't
// trim more than necessary.
func remainingTokens(out []domain.L0Segment, all []domain.L0Segment) int {
	if len(out) >= len(all) {
		return 0
	}
	return sumSegmentTokens(all[len(out):])
}

func dropOldestHistory(segments []domain.L0Segment, budget int) ([]domain.L0Segment, int) {
	if sumSegmentTokens(segments) <= budget {
		return segments, 0
	}
	type indexed struct {
		i   int
		seg domain.L0Segment
	}
	historyIdx := []indexed{}
	for i, seg := range segments {
		if seg.Section == "history" {
			historyIdx = append(historyIdx, indexed{i, seg})
		}
	}
	dropped := 0
	keep := make(map[int]bool, len(segments))
	for i := range segments {
		keep[i] = true
	}
	for _, h := range historyIdx {
		out := materialize(segments, keep)
		if sumSegmentTokens(out) <= budget {
			break
		}
		keep[h.i] = false
		dropped++
	}
	return materialize(segments, keep), dropped
}

func materialize(segments []domain.L0Segment, keep map[int]bool) []domain.L0Segment {
	out := make([]domain.L0Segment, 0, len(segments))
	for i, seg := range segments {
		if keep[i] {
			out = append(out, seg)
		}
	}
	return out
}

func sumSegmentTokens(segments []domain.L0Segment) int {
	total := 0
	for _, seg := range segments {
		total += seg.Tokens
	}
	return total
}

func appendUnique(values []string, value string) []string {
	for _, v := range values {
		if v == value {
			return values
		}
	}
	return append(values, value)
}

// assembleChatMessages flattens segments into a model-ready []L0ChatMessage.
// All system / summary / l1 / l3 / l4 segments collapse into a single system
// message at the top so providers that limit system role count keep working.
// History and user.input segments preserve their roles 1:1.
func assembleChatMessages(segments []domain.L0Segment) []domain.L0ChatMessage {
	var systemBlocks []string
	var rest []domain.L0ChatMessage
	for _, seg := range segments {
		switch seg.Section {
		case "history":
			rest = append(rest, domain.L0ChatMessage{Role: seg.Role, Content: seg.Content})
		case "user.input":
			rest = append(rest, domain.L0ChatMessage{Role: "user", Content: seg.Content})
		default:
			if strings.TrimSpace(seg.Content) == "" {
				continue
			}
			systemBlocks = append(systemBlocks, seg.Content)
		}
	}
	out := make([]domain.L0ChatMessage, 0, len(rest)+1)
	if len(systemBlocks) > 0 {
		out = append(out, domain.L0ChatMessage{
			Role:    "system",
			Content: strings.Join(systemBlocks, "\n\n"),
		})
	}
	return append(out, rest...)
}

func redactSegments(segments []domain.L0Segment, redact bool) []domain.L0Segment {
	if !redact {
		return segments
	}
	out := make([]domain.L0Segment, len(segments))
	for i, seg := range segments {
		seg.Content = ""
		out[i] = seg
	}
	return out
}

func redactPromptMessages(messages []domain.L0ChatMessage) []domain.L0ChatMessage {
	out := make([]domain.L0ChatMessage, len(messages))
	for i, m := range messages {
		m.Content = previewText(m.Content, l0PreviewLimit)
		out[i] = m
	}
	return out
}

func shouldWriteSnapshot(mode string, usedRatio float64, hasWarning bool) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "always":
		return true
	case "off":
		return false
	default: // on_warning
		return hasWarning || usedRatio >= 0.6
	}
}

func buildSnapshot(req domain.L0AssemblyRequest, settings domain.L0Settings, result domain.L0AssemblyResult, segments []domain.L0Segment, contextWindow int, summarizedFrom int, summarizedTo int) domain.L0AssemblySnapshot {
	segmentsJSON := mustMarshalSegments(segments)
	warningJSON := mustMarshalStrings(result.WarningCodes)
	return domain.L0AssemblySnapshot{
		ID:                    newID(),
		SessionID:             req.SessionID,
		RunID:                 req.RunID,
		TurnID:                req.TurnID,
		SpanID:                req.SpanID,
		AgentID:               req.AgentID,
		TeamID:                req.TeamID,
		Provider:              req.Provider,
		Model:                 req.Model,
		ContextWindowTokens:   contextWindow,
		BudgetTokens:          result.BudgetTokens,
		RecentWindowTurns:     result.RecentWindowTurns,
		RecentWindowTokens:    result.RecentWindowTokens,
		SummaryTokenEstimate:  segmentTokensBySection(segments, "summary"),
		L1FieldCount:          countSegments(segments, "memory.l1"),
		L1TokenEstimate:       segmentTokensBySection(segments, "memory.l1"),
		L3ChunkCount:          countSegments(segments, "memory.l3"),
		L3TokenEstimate:       segmentTokensBySection(segments, "memory.l3"),
		L4PathCount:           countSegments(segments, "memory.l4"),
		L4TokenEstimate:       segmentTokensBySection(segments, "memory.l4"),
		PromptTokenEstimate:   result.PromptTokenEstimate,
		PromptTokenActual:     0,
		UsedRatio:             result.UsedRatioEstimate,
		TruncateStrategy:      result.TruncateStrategy,
		TruncatedMessageCount: result.TruncatedMessageCount,
		SummarizedTurnFrom:    summarizedFrom,
		SummarizedTurnTo:      summarizedTo,
		SegmentsJSON:          segmentsJSON,
		WarningCodesJSON:      warningJSON,
		MetadataJSON:          mustMarshalMetadata(settings, req),
		CreatedAt:             nowUTC(),
	}
}

func segmentTokensBySection(segments []domain.L0Segment, section string) int {
	total := 0
	for _, seg := range segments {
		if seg.Section == section {
			total += seg.Tokens
		}
	}
	return total
}

func countSegments(segments []domain.L0Segment, section string) int {
	count := 0
	for _, seg := range segments {
		if seg.Section == section {
			count++
		}
	}
	return count
}

// mustMarshalSegments stores ONLY preview + meta so we never leak full prompt
// bodies into the snapshot table. Full content remains addressable via the
// source `messages.id` references.
func mustMarshalSegments(segments []domain.L0Segment) string {
	out := make([]map[string]any, len(segments))
	for i, seg := range segments {
		out[i] = map[string]any{
			"section": seg.Section,
			"role":    seg.Role,
			"source":  seg.Source,
			"tokens":  seg.Tokens,
			"preview": seg.Preview,
		}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func mustMarshalStrings(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func mustMarshalMetadata(settings domain.L0Settings, req domain.L0AssemblyRequest) string {
	data, err := json.Marshal(map[string]any{
		"settings":            settings,
		"reserved_for_output": req.ReservedForOutput,
	})
	if err != nil {
		return "{}"
	}
	return string(data)
}

func estimateTokensApprox(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	tokens := len([]rune(trimmed)) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}
