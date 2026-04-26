// agent_evolution_scanner.go implements the §5.5 EvolutionWorker pipeline
// in heuristic mode (no LLM calls). It aggregates per-tool telemetry from
// `tool_invocations` into `agent_skill_stats`, then turns clear win/loss
// signals into ProposalInputs that flow through the existing
// Propose/Approve/Apply pipeline.
//
// Phase 5 may swap the deterministic heuristics for an LLM JSON
// reflection prompt; this file isolates that boundary so the worker loop
// in `cmd/server` does not need to change.
package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// scanWindow is the default look-back used when an agent has no
// recorded `last_scan_at` yet. Picked to be long enough to gather
// statistically meaningful telemetry but short enough that fading
// failures eventually drop out.
const scanWindow = 30 * 24 * time.Hour

// scanMinInvocations is the per-tool floor that gates *any* heuristic
// proposal — below this we have too few samples to act on.
const scanMinInvocations = 5

// scanFailureThreshold and scanSuccessThreshold are the heuristic
// rates that trigger blacklist / preference-boost proposals. Both are
// intentionally conservative so the worker prefers silence to noise.
const (
	scanFailureThreshold = 0.30
	scanSuccessThreshold = 0.85
)

// rollback alarm tuning per §13:
//
//   "回滚率 > 20% 后，evo_auto_apply 自动转为 0 并发出告警"
//
// We additionally require a minimum sample size so a single flaky
// proposal cannot trip the brake.
const (
	rollbackAlarmThreshold   = 0.20
	rollbackAlarmMinEvents   = 5
	rollbackAlarmWindowHours = 24 * 30
)

// statsLastScanAtKey is the AgentStrategyProfile.Stats key the scanner
// uses to remember when it last completed for an agent. It is *not* a
// formal evolution event — updating it is bookkeeping only.
const statsLastScanAtKey = "last_scan_at"

// statsLastScanReportKey snapshots the most recent ScanReport for the
// agent, primarily for UI inspection / debugging.
const statsLastScanReportKey = "last_scan_report"

// negativeFeedbackTypes lists the FactFeedback values the scanner
// counts as "the user pushed back". Confirms / used / not_used are
// excluded — only signals that imply the agent was wrong.
var negativeFeedbackTypes = []string{
	domain.FactFeedbackReject,
	domain.FactFeedbackRefine,
}

// AggregateSkillStats pulls tool_invocations for `agentID` since `since`,
// groups by tool_key, and upserts one `agent_skill_stats` row per tool.
// Returns the upserted slice so callers (the scanner) can immediately
// consume them without a re-read. Empty `since` defaults to `now -
// scanWindow`.
func (s *AgentEvolutionService) AggregateSkillStats(ctx context.Context, agentID, since string) ([]domain.AgentSkillStat, error) {
	if agentID == "" {
		return nil, validationError("agent id is required")
	}
	if strings.TrimSpace(since) == "" {
		since = time.Now().UTC().Add(-scanWindow).Format(time.RFC3339)
	}

	runs, err := s.repo.SearchToolInvocations(domain.ToolRunQuery{
		AgentID: agentID,
		From:    since,
		Limit:   1000,
	})
	if err != nil {
		return nil, err
	}

	type acc struct {
		invocations  int
		successes    int
		failures     int
		userOverride int
		latencyTotal float64
		tokenTotal   float64
		lastUsedAt   string
	}
	bucket := map[string]*acc{}
	for _, r := range runs.Items {
		if r.ToolKey == "" {
			continue
		}
		a, ok := bucket[r.ToolKey]
		if !ok {
			a = &acc{}
			bucket[r.ToolKey] = a
		}
		a.invocations++
		switch r.Status {
		case "success":
			a.successes++
		case "error", "failure":
			a.failures++
		case "blocked":
			a.userOverride++
		}
		a.latencyTotal += float64(r.DurationMS)
		// Approximate token cost using output preview length / 4 — good
		// enough for sorting; precise accounting belongs in the chat
		// usage pipeline, not the scanner.
		a.tokenTotal += float64(len(r.OutputPreview)) / 4
		if r.StartedAt > a.lastUsedAt {
			a.lastUsedAt = r.StartedAt
		}
	}

	out := make([]domain.AgentSkillStat, 0, len(bucket))
	for toolKey, a := range bucket {
		stat := domain.AgentSkillStat{
			AgentID:         agentID,
			Scope:           "overall",
			ScopeValue:      "",
			ToolKey:         toolKey,
			Invocations:     a.invocations,
			Successes:       a.successes,
			Failures:        a.failures,
			UserOverrides:   a.userOverride,
			AvgLatencyMS:    safeAvg(a.latencyTotal, a.invocations),
			AvgTokens:       safeAvg(a.tokenTotal, a.invocations),
			PreferenceScore: skillPreferenceScore(a.successes, a.failures, a.invocations),
			LastUsedAt:      a.lastUsedAt,
			Metadata: map[string]any{
				"window_since": since,
				"computed_at":  s.now(),
			},
		}
		stored, err := s.repo.UpsertAgentSkillStat(stat)
		if err != nil {
			return out, err
		}
		out = append(out, stored)
	}
	return out, nil
}

// RunEvolutionScan implements §5.5. It refreshes per-tool skill stats
// from telemetry, evaluates the §13 rollback-rate safety brake, gates
// on `evo_enabled` plus the activity-volume / negative-feedback
// triggers, and emits one ProposalInput per clear signal. When
// `evo_auto_apply=true` AND the proposal is `low` risk it is also
// auto-approved + applied.
//
// Throttling is delegated to `Propose`, so re-running the scan within
// the throttle window is safe (the new proposals end up `superseded`).
func (s *AgentEvolutionService) RunEvolutionScan(ctx context.Context, agentID string) (ScanReport, error) {
	if agentID == "" {
		return ScanReport{}, validationError("agent id is required")
	}
	scanStart := time.Now().UTC()
	settings, _ := s.repo.GetAgentRuntimeSettings(agentID)
	if !settings.EvoEnabled {
		return ScanReport{Note: "evo_enabled=false"}, nil
	}

	current, err := s.GetStrategy(ctx, agentID)
	if err != nil {
		return ScanReport{}, err
	}

	// §13 rollback-rate brake — runs *before* generating new proposals
	// so a misbehaving auto-apply pass cannot pile fresh damage onto a
	// quarantined agent.
	if disabled, rate, total := s.evaluateRollbackAlarm(ctx, agentID, settings); disabled {
		// Reload settings so subsequent steps see evo_auto_apply=false
		// even though we won't auto-apply on this pass anyway.
		settings.EvoAutoApply = false
		_ = s.audit("agent.evolution.scanner.rollback_alarm",
			"agent_runtime_settings", agentID, map[string]any{
				"rollback_rate": rate,
				"event_total":   total,
				"threshold":     rollbackAlarmThreshold,
				"action":        "evo_auto_apply=false",
			})
	}

	// §5.5 step 2 — episodes and feedback use the incremental
	// `last_scan_at` window so they only count what happened *since the
	// previous scan*. The `agent_skill_stats` aggregation, in contrast,
	// is always a rolling 30-day window so trends remain stable across
	// successive scans (per spec: "skill_stats = AgentSkillStat 最近聚合").
	triggerSince := s.scanWindowSince(current, scanStart)
	aggregationSince := scanStart.Add(-scanWindow).Format(time.RFC3339)

	episodes, err := s.repo.CountAgentEpisodesSince(agentID, triggerSince)
	if err != nil {
		// Episode counting is informative-only; a failure here must not
		// block the rest of the scan because tool telemetry alone is
		// enough to drive proposals.
		episodes = 0
	}
	negFeedback, _ := s.repo.CountAgentFactFeedbackSince(agentID, negativeFeedbackTypes, triggerSince)

	stats, err := s.AggregateSkillStats(ctx, agentID, aggregationSince)
	if err != nil {
		return ScanReport{Errors: 1, EpisodesScanned: episodes}, err
	}

	minEpisodes := settings.EvoMinEpisodes
	if minEpisodes <= 0 {
		minEpisodes = 20
	}
	minNegFeedback := settings.EvoMinNegativeFeedback
	if minNegFeedback <= 0 {
		minNegFeedback = 3
	}
	hasFailingTool := false
	for _, st := range stats {
		if st.Invocations >= scanMinInvocations && failureRate(st) > scanFailureThreshold {
			hasFailingTool = true
			break
		}
	}
	triggered := episodes >= minEpisodes ||
		hasFailingTool ||
		negFeedback >= minNegFeedback

	if !triggered {
		report := ScanReport{
			EpisodesScanned: episodes,
			Note:            "trigger conditions not met",
		}
		s.persistScanCheckpoint(ctx, agentID, scanStart, report)
		return report, nil
	}

	report := ScanReport{EpisodesScanned: episodes}
	currentBlacklist := map[string]bool{}
	for _, k := range current.ToolBlacklist {
		currentBlacklist[k] = true
	}

	for _, st := range stats {
		if st.Invocations < scanMinInvocations {
			continue
		}
		if failureRate(st) > scanFailureThreshold && !currentBlacklist[st.ToolKey] {
			next := append([]string(nil), current.ToolBlacklist...)
			next = append(next, st.ToolKey)
			prop, err := s.Propose(ctx, ProposalInput{
				AgentID:        agentID,
				Kind:           domain.EvoKindToolDisable,
				TargetField:    "strategy.tool_blacklist",
				ProposedValue:  next,
				CurrentValue:   current.ToolBlacklist,
				Rationale:      fmt.Sprintf("auto-scan: tool %s failure_rate=%.2f over %d invocations", st.ToolKey, failureRate(st), st.Invocations),
				ExpectedImpact: "Reduce repeated failures by removing the tool from the agent's whitelist.",
				RiskLevel:      domain.EvoRiskLow,
				Source:         domain.EvoSourceRuntimeSignal,
			})
			if err != nil {
				report.Errors++
				continue
			}
			s.handleScanProposalLifecycle(ctx, settings, prop, &report)
		}

		if successRate(st) > scanSuccessThreshold {
			currentPref := current.ToolPreference[st.ToolKey]
			if currentPref >= 0.7 {
				continue
			}
			merged := map[string]float64{}
			for k, v := range current.ToolPreference {
				merged[k] = v
			}
			merged[st.ToolKey] = 0.8
			prop, err := s.Propose(ctx, ProposalInput{
				AgentID:        agentID,
				Kind:           domain.EvoKindToolPrefUpdate,
				TargetField:    "strategy.tool_preference",
				ProposedValue:  merged,
				CurrentValue:   current.ToolPreference,
				Rationale:      fmt.Sprintf("auto-scan: tool %s success_rate=%.2f over %d invocations", st.ToolKey, successRate(st), st.Invocations),
				ExpectedImpact: "Promote a reliably-used tool so it ranks higher in the prompt's tool list.",
				RiskLevel:      domain.EvoRiskLow,
				Source:         domain.EvoSourceRuntimeSignal,
			})
			if err != nil {
				report.Errors++
				continue
			}
			s.handleScanProposalLifecycle(ctx, settings, prop, &report)
		}
	}

	s.persistScanCheckpoint(ctx, agentID, scanStart, report)
	return report, nil
}

// scanWindowSince picks the lower bound of the scan window. Prefers the
// `last_scan_at` checkpoint stored in `strategy.stats`; falls back to
// `now - scanWindow`. A checkpoint older than `scanWindow` is also
// clamped so a long-idle agent does not suddenly aggregate a year of
// noisy data on its first scan after being re-enabled.
func (s *AgentEvolutionService) scanWindowSince(strat domain.AgentStrategyProfile, now time.Time) string {
	fallback := now.Add(-scanWindow)
	if strat.Stats == nil {
		return fallback.Format(time.RFC3339)
	}
	raw, ok := strat.Stats[statsLastScanAtKey].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback.Format(time.RFC3339)
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return fallback.Format(time.RFC3339)
	}
	if parsed.Before(fallback) {
		return fallback.Format(time.RFC3339)
	}
	return parsed.UTC().Format(time.RFC3339)
}

// persistScanCheckpoint stores `last_scan_at` and a tiny snapshot of the
// most recent ScanReport into `strategy.stats`. It re-reads the live
// strategy first so any auto-applied proposals from this same scan
// survive the bookkeeping write (otherwise we'd overwrite the brand-new
// blacklist with the stale snapshot we cached at the top of the scan).
// A failure here is logged-only — the scan itself already succeeded.
func (s *AgentEvolutionService) persistScanCheckpoint(ctx context.Context, agentID string, scanStart time.Time, report ScanReport) {
	live, err := s.repo.GetAgentStrategyProfile(agentID)
	if err != nil {
		return
	}
	if live.Stats == nil {
		live.Stats = map[string]any{}
	}
	live.Stats[statsLastScanAtKey] = scanStart.Format(time.RFC3339)
	live.Stats[statsLastScanReportKey] = map[string]any{
		"episodes_scanned":    report.EpisodesScanned,
		"new_proposals":       report.NewProposals,
		"auto_applied":        report.AutoApplied,
		"throttled_proposals": report.ThrottledProposals,
		"errors":              report.Errors,
		"note":                report.Note,
		"recorded_at":         scanStart.Format(time.RFC3339),
	}
	_, _ = s.repo.UpsertAgentStrategyProfile(live)
}

// evaluateRollbackAlarm checks the §13 brake. Returns `disabled=true`
// when the rate is over `rollbackAlarmThreshold` and the agent is
// currently configured for auto-apply. The function flips the runtime
// settings flag in-place so subsequent calls do not re-trigger the
// audit log on the same threshold breach.
func (s *AgentEvolutionService) evaluateRollbackAlarm(ctx context.Context, agentID string, settings domain.AgentRuntimeSettings) (bool, float64, int) {
	if !settings.EvoAutoApply {
		return false, 0, 0
	}
	since := time.Now().UTC().Add(-rollbackAlarmWindowHours * time.Hour).Format(time.RFC3339)
	events, _, err := s.repo.ListEvolutionEvents(repository.EvolutionEventQuery{
		AgentID: agentID,
		Limit:   500,
	})
	if err != nil {
		return false, 0, 0
	}
	total, reverted := 0, 0
	for _, ev := range events {
		if ev.CreatedAt < since {
			continue
		}
		if ev.Kind == domain.EvoKindRollback {
			// A rollback event is the *consequence* of a prior reverted
			// event; counting both would double-charge the rate.
			continue
		}
		total++
		if ev.Reverted {
			reverted++
		}
	}
	if total < rollbackAlarmMinEvents {
		return false, 0, total
	}
	rate := float64(reverted) / float64(total)
	if rate <= rollbackAlarmThreshold {
		return false, rate, total
	}
	settings.EvoAutoApply = false
	if _, err := s.repo.UpsertAgentRuntimeSettings(settings); err != nil {
		return false, rate, total
	}
	return true, rate, total
}

// handleScanProposalLifecycle inspects the proposal status and bumps the
// matching ScanReport counter. When `evo_auto_apply` is enabled and the
// proposal is low-risk pending it auto-approves it.
func (s *AgentEvolutionService) handleScanProposalLifecycle(ctx context.Context, settings domain.AgentRuntimeSettings, prop domain.EvolutionProposal, report *ScanReport) {
	switch prop.Status {
	case domain.EvoProposalSuperseded:
		report.ThrottledProposals++
		return
	case domain.EvoProposalPending:
		report.NewProposals++
	default:
		report.NewProposals++
	}
	if !settings.EvoAutoApply || prop.RiskLevel != domain.EvoRiskLow || prop.Status != domain.EvoProposalPending {
		return
	}
	if _, err := s.Approve(ctx, prop.ID, "auto_scanner"); err != nil {
		report.Errors++
		return
	}
	report.AutoApplied++
}

func failureRate(s domain.AgentSkillStat) float64 {
	if s.Invocations <= 0 {
		return 0
	}
	return float64(s.Failures) / float64(s.Invocations)
}

func successRate(s domain.AgentSkillStat) float64 {
	if s.Invocations <= 0 {
		return 0
	}
	return float64(s.Successes) / float64(s.Invocations)
}

func safeAvg(total float64, n int) float64 {
	if n <= 0 {
		return 0
	}
	return total / float64(n)
}

// skillPreferenceScore maps the {success, failure, total} triple into
// a [0,1] preference score with mild smoothing so a single failure
// against a brand-new tool does not crater its preference. Mirrors the
// "preference_score REAL DEFAULT 0.5" baseline in §3.2.5.
func skillPreferenceScore(successes, failures, total int) float64 {
	if total <= 0 {
		return 0.5
	}
	const prior = 2.0
	num := float64(successes) + 0.5*prior
	den := float64(total) + prior
	score := num/den - 0.3*float64(failures)/(float64(total)+1)
	// Floor at 0.01 instead of 0 so the upsert path (which treats 0 as
	// "unset" and defaults back to 0.5) does not overwrite a genuinely
	// poor score.
	if score < 0.01 {
		return 0.01
	}
	if score > 1 {
		return 1
	}
	return score
}
