package repository

import (
	"errors"

	"arenea/backend/internal/domain"
)

// InsertL0AssemblySnapshot persists one assembly run. Callers (MemoryL0Service)
// own the ID generation so the snapshot can be referenced from the model-call
// span before the row hits SQLite.
func (r *SQLiteRepository) InsertL0AssemblySnapshot(snap domain.L0AssemblySnapshot) error {
	if snap.ID == "" || snap.SessionID == "" {
		return errors.New("snapshot id and session_id are required")
	}
	if snap.CreatedAt == "" {
		snap.CreatedAt = nowISO()
	}
	if snap.SegmentsJSON == "" {
		snap.SegmentsJSON = "[]"
	}
	if snap.WarningCodesJSON == "" {
		snap.WarningCodesJSON = "[]"
	}
	if snap.MetadataJSON == "" {
		snap.MetadataJSON = "{}"
	}
	_, err := r.db.Exec(
		`INSERT INTO memory_l0_assembly_snapshots(
		   id, session_id, run_id, turn_id, span_id, agent_id, team_id,
		   provider, model, context_window_tokens, budget_tokens,
		   recent_window_turns, recent_window_tokens, summary_token_estimate,
		   l1_field_count, l1_token_estimate,
		   l3_chunk_count, l3_token_estimate,
		   l4_path_count, l4_token_estimate,
		   prompt_token_estimate, prompt_token_actual, used_ratio,
		   truncate_strategy, truncated_message_count,
		   summarized_turn_from, summarized_turn_to,
		   segments_json, warning_codes_json, metadata_json, created_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.ID, snap.SessionID, snap.RunID, snap.TurnID, snap.SpanID, snap.AgentID, snap.TeamID,
		snap.Provider, snap.Model, snap.ContextWindowTokens, snap.BudgetTokens,
		snap.RecentWindowTurns, snap.RecentWindowTokens, snap.SummaryTokenEstimate,
		snap.L1FieldCount, snap.L1TokenEstimate,
		snap.L3ChunkCount, snap.L3TokenEstimate,
		snap.L4PathCount, snap.L4TokenEstimate,
		snap.PromptTokenEstimate, snap.PromptTokenActual, snap.UsedRatio,
		snap.TruncateStrategy, snap.TruncatedMessageCount,
		snap.SummarizedTurnFrom, snap.SummarizedTurnTo,
		snap.SegmentsJSON, snap.WarningCodesJSON, snap.MetadataJSON, snap.CreatedAt,
	)
	return err
}

// UpdateL0AssemblySnapshotActualTokens is called after the model call returns
// usage. Storing the real prompt tokens lets the analytics pipeline measure
// estimator drift and feed the agent-evolution loop.
func (r *SQLiteRepository) UpdateL0AssemblySnapshotActualTokens(snapshotID string, actualPromptTokens int, usedRatio float64) error {
	if snapshotID == "" {
		return errors.New("snapshot id is required")
	}
	if usedRatio < 0 {
		usedRatio = 0
	}
	if usedRatio > 1 {
		usedRatio = 1
	}
	_, err := r.db.Exec(
		`UPDATE memory_l0_assembly_snapshots SET prompt_token_actual = ?, used_ratio = ? WHERE id = ?`,
		actualPromptTokens, usedRatio, snapshotID,
	)
	return err
}

func (r *SQLiteRepository) GetL0AssemblySnapshotByID(id string) (domain.L0AssemblySnapshot, error) {
	row := r.db.QueryRow(memoryL0SelectSQL()+` WHERE id = ?`, id)
	return scanL0Snapshot(row)
}

func (r *SQLiteRepository) ListL0AssemblySnapshotsBySession(sessionID string, limit int) ([]domain.L0AssemblySnapshot, error) {
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.db.Query(memoryL0SelectSQL()+` WHERE session_id = ? ORDER BY created_at DESC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanL0Snapshots(rows)
}

func (r *SQLiteRepository) ListL0AssemblySnapshotsBySpan(spanID string) ([]domain.L0AssemblySnapshot, error) {
	if spanID == "" {
		return nil, errors.New("span id is required")
	}
	rows, err := r.db.Query(memoryL0SelectSQL()+` WHERE span_id = ? ORDER BY created_at DESC`, spanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanL0Snapshots(rows)
}

// ListSessionSummaries returns the existing condensed segments stored on
// `session_summaries` newest-first. The L0 service only needs a small window
// (typically the last 8) to seed the prompt header.
func (r *SQLiteRepository) ListSessionSummaries(sessionID string, limit int) ([]domain.SessionSummary, error) {
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 8
	}
	rows, err := r.db.Query(
		`SELECT id, session_id, summary_markdown, from_turn, to_turn, token_estimate, created_at
		 FROM session_summaries WHERE session_id = ? ORDER BY to_turn DESC, created_at DESC LIMIT ?`,
		sessionID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.SessionSummary, 0, limit)
	for rows.Next() {
		var v domain.SessionSummary
		if err = rows.Scan(&v.ID, &v.SessionID, &v.SummaryMarkdown, &v.FromTurn, &v.ToTurn, &v.TokenEstimate, &v.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// AddSessionSummary inserts a summary row produced by SummaryService. The
// caller fills `ID` / `CreatedAt` to keep this file dependency-free.
func (r *SQLiteRepository) AddSessionSummary(summary domain.SessionSummary) (domain.SessionSummary, error) {
	if summary.ID == "" || summary.SessionID == "" {
		return domain.SessionSummary{}, errors.New("summary id and session_id are required")
	}
	if summary.CreatedAt == "" {
		summary.CreatedAt = nowISO()
	}
	_, err := r.db.Exec(
		`INSERT INTO session_summaries(id, session_id, summary_markdown, from_turn, to_turn, token_estimate, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		summary.ID, summary.SessionID, summary.SummaryMarkdown, summary.FromTurn, summary.ToTurn, summary.TokenEstimate, summary.CreatedAt,
	)
	if err != nil {
		return domain.SessionSummary{}, err
	}
	return summary, nil
}

func memoryL0SelectSQL() string {
	return `SELECT id, session_id, run_id, turn_id, span_id, agent_id, team_id, provider, model,
	 context_window_tokens, budget_tokens, recent_window_turns, recent_window_tokens, summary_token_estimate,
	 l1_field_count, l1_token_estimate, l3_chunk_count, l3_token_estimate, l4_path_count, l4_token_estimate,
	 prompt_token_estimate, prompt_token_actual, used_ratio, truncate_strategy, truncated_message_count,
	 summarized_turn_from, summarized_turn_to, segments_json, warning_codes_json, metadata_json, created_at
	 FROM memory_l0_assembly_snapshots`
}

func scanL0Snapshot(row scanner) (domain.L0AssemblySnapshot, error) {
	var v domain.L0AssemblySnapshot
	err := row.Scan(
		&v.ID, &v.SessionID, &v.RunID, &v.TurnID, &v.SpanID, &v.AgentID, &v.TeamID, &v.Provider, &v.Model,
		&v.ContextWindowTokens, &v.BudgetTokens, &v.RecentWindowTurns, &v.RecentWindowTokens, &v.SummaryTokenEstimate,
		&v.L1FieldCount, &v.L1TokenEstimate, &v.L3ChunkCount, &v.L3TokenEstimate, &v.L4PathCount, &v.L4TokenEstimate,
		&v.PromptTokenEstimate, &v.PromptTokenActual, &v.UsedRatio, &v.TruncateStrategy, &v.TruncatedMessageCount,
		&v.SummarizedTurnFrom, &v.SummarizedTurnTo, &v.SegmentsJSON, &v.WarningCodesJSON, &v.MetadataJSON, &v.CreatedAt,
	)
	return v, err
}

func scanL0Snapshots(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]domain.L0AssemblySnapshot, error) {
	result := make([]domain.L0AssemblySnapshot, 0, 16)
	for rows.Next() {
		v, err := scanL0Snapshot(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
