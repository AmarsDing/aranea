package repository

import (
	"database/sql"
	"errors"
	"strings"

	"arenea/backend/internal/domain"
)

func (r *SQLiteRepository) CreateSession(s domain.Session) (domain.Session, error) {
	if s.ID == "" || s.Title == "" {
		return domain.Session{}, errors.New("missing required fields")
	}
	if s.OwnerType == "" {
		s.OwnerType = "agent"
	}
	if s.OwnerType == "agent" && s.AgentID == "" {
		return domain.Session{}, errors.New("agent_id is required")
	}
	if s.OwnerType == "team" && s.TeamID == "" {
		return domain.Session{}, errors.New("team_id is required")
	}
	now := nowISO()
	s.CreatedAt = now
	s.UpdatedAt = now
	if s.Status == "" {
		s.Status = "active"
	}
	if s.ContextStatus == "" {
		s.ContextStatus = contextStatusForRatio(s.ContextUsedRatio)
	}
	_, err := r.db.Exec(
		`INSERT INTO sessions(
		 id, owner_type, agent_id, team_id, title, summary, context_used_ratio, context_used_tokens, max_context_used_ratio, last_context_window_tokens, context_status,
		 dialog_mode, provider, model, status, message_count, run_count, model_call_count, tool_call_count, skill_call_count,
		 mcp_call_count, input_tokens, output_tokens, total_tokens, total_cost_micro_usd, last_message_at, created_at, updated_at, archived_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.OwnerType, s.AgentID, s.TeamID, s.Title, s.Summary, s.ContextUsedRatio, s.ContextUsedTokens, s.MaxContextUsedRatio, s.LastContextWindowTokens, s.ContextStatus,
		s.DialogMode, s.Provider, s.Model, s.Status, s.MessageCount, s.RunCount, s.ModelCallCount, s.ToolCallCount, s.SkillCallCount,
		s.MCPCallCount, s.InputTokens, s.OutputTokens, s.TotalTokens, s.TotalCostMicroUSD, s.LastMessageAt, s.CreatedAt, s.UpdatedAt, s.ArchivedAt, s.DeletedAt,
	)
	return s, err
}

func (r *SQLiteRepository) GetSessionByID(id string) (domain.Session, error) {
	row := r.db.QueryRow(sessionSelectSQL()+` WHERE id = ? AND deleted_at = ''`, id)
	return scanSession(row)
}

func (r *SQLiteRepository) ListSessions(agentID string) ([]domain.Session, error) {
	result, err := r.SearchSessions(domain.SessionSearchQuery{AgentID: agentID, Limit: 200})
	return result.Items, err
}

func (r *SQLiteRepository) ListTeamSessions(teamID string) ([]domain.Session, error) {
	result, err := r.SearchSessions(domain.SessionSearchQuery{TeamID: teamID, Limit: 200})
	return result.Items, err
}

func (r *SQLiteRepository) SearchSessions(query domain.SessionSearchQuery) (domain.SessionListResult, error) {
	if query.Limit <= 0 || query.Limit > 100 {
		query.Limit = 20
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	clauses := []string{"deleted_at = ''"}
	args := []any{}
	if query.OwnerType != "" {
		clauses = append(clauses, "owner_type = ?")
		args = append(args, query.OwnerType)
	}
	if query.AgentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, query.AgentID)
	}
	if query.TeamID != "" {
		clauses = append(clauses, "team_id = ?")
		args = append(args, query.TeamID)
	}
	if query.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, query.Status)
	}
	if query.ContextStatus != "" {
		clauses = append(clauses, "context_status = ?")
		args = append(args, query.ContextStatus)
	}
	if query.Keyword != "" {
		clauses = append(clauses, "(title LIKE ? OR summary LIKE ? OR id LIKE ?)")
		like := "%" + query.Keyword + "%"
		args = append(args, like, like, like)
	}
	where := strings.Join(clauses, " AND ")
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE `+where, args...).Scan(&total); err != nil {
		return domain.SessionListResult{}, err
	}
	listArgs := append(append([]any{}, args...), query.Limit, query.Offset)
	rows, err := r.db.Query(sessionSelectSQL()+` WHERE `+where+` ORDER BY COALESCE(NULLIF(last_message_at, ''), updated_at) DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return domain.SessionListResult{}, err
	}
	defer rows.Close()
	items, err := scanSessions(rows)
	if err != nil {
		return domain.SessionListResult{}, err
	}
	return domain.SessionListResult{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

func (r *SQLiteRepository) UpdateSessionTitle(id string, title string) (domain.Session, error) {
	_, err := r.db.Exec(`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`, title, nowISO(), id)
	if err != nil {
		return domain.Session{}, err
	}
	return r.GetSessionByID(id)
}

func (r *SQLiteRepository) UpdateSessionContextUsedRatio(sessionID string, ratio float64) error {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	_, err := r.db.Exec(`UPDATE sessions SET context_used_ratio = ?, max_context_used_ratio = MAX(max_context_used_ratio, ?), context_status = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`, ratio, ratio, contextStatusForRatio(ratio), nowISO(), sessionID)
	return err
}

// UpdateSessionL0Context records both the prompt-level token usage and the
// effective model context window. It mirrors UpdateSessionContextUsedRatio but
// keeps the L0 metrics in sync so the front-end "context" tab can render the
// real numbers behind the ratio.
func (r *SQLiteRepository) UpdateSessionL0Context(sessionID string, promptTokens int, contextWindow int, ratio float64) error {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	if promptTokens < 0 {
		promptTokens = 0
	}
	if contextWindow < 0 {
		contextWindow = 0
	}
	_, err := r.db.Exec(`UPDATE sessions SET context_used_ratio = ?, context_used_tokens = ?, last_context_window_tokens = ?, max_context_used_ratio = MAX(max_context_used_ratio, ?), context_status = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`,
		ratio, promptTokens, contextWindow, ratio, contextStatusForRatio(ratio), nowISO(), sessionID)
	return err
}

func (r *SQLiteRepository) ArchiveSession(id string) error {
	now := nowISO()
	_, err := r.db.Exec(`UPDATE sessions SET status = 'archived', archived_at = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`, now, now, id)
	return err
}

func (r *SQLiteRepository) DeleteSession(id string) error {
	now := nowISO()
	_, err := r.db.Exec(`UPDATE sessions SET deleted_at = ?, status = 'deleted', updated_at = ? WHERE id = ? AND deleted_at = ''`, now, now, id)
	return err
}

func (r *SQLiteRepository) DeleteSessionsByAgentID(agentID string) error {
	now := nowISO()
	_, err := r.db.Exec(`UPDATE sessions SET deleted_at = ?, status = 'deleted', updated_at = ? WHERE agent_id = ? AND deleted_at = ''`, now, now, agentID)
	return err
}

func (r *SQLiteRepository) AddMessage(m domain.Message) (domain.Message, error) {
	if m.ID == "" || m.SessionID == "" || m.Role == "" || m.Content == "" {
		return domain.Message{}, errors.New("missing required fields")
	}
	if m.Status == "" {
		m.Status = "ok"
	}
	if m.TurnIndex <= 0 {
		next, err := r.nextTurnIndex(m.SessionID)
		if err != nil {
			return domain.Message{}, err
		}
		m.TurnIndex = next
	}
	m.CreatedAt = nowISO()
	tx, err := r.db.Begin()
	if err != nil {
		return domain.Message{}, err
	}
	defer tx.Rollback()
	if err = addMessageTx(tx, m); err != nil {
		return domain.Message{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Message{}, err
	}
	return m, nil
}

func (r *SQLiteRepository) nextTurnIndex(sessionID string) (int, error) {
	var next sql.NullInt64
	err := r.db.QueryRow(`SELECT COALESCE(MAX(turn_index), 0) + 1 FROM messages WHERE session_id = ?`, sessionID).Scan(&next)
	if err != nil {
		return 0, err
	}
	return int(next.Int64), nil
}

func (r *SQLiteRepository) ListMessages(sessionID string) ([]domain.Message, error) {
	rows, err := r.db.Query(
		`SELECT id, session_id, parent_message_id, turn_index, role, content_markdown, COALESCE(model_name, ''), token_in, token_out, latency_ms, status, attachments_count, options_json, error_message, created_at
		 FROM messages WHERE session_id = ? ORDER BY turn_index ASC, created_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.Message
	for rows.Next() {
		var v domain.Message
		if err = rows.Scan(&v.ID, &v.SessionID, &v.ParentMessageID, &v.TurnIndex, &v.Role, &v.Content, &v.ModelName, &v.TokenIn, &v.TokenOut, &v.LatencyMS, &v.Status, &v.AttachmentsCount, &v.OptionsJSON, &v.ErrorMessage, &v.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func sessionSelectSQL() string {
	return `SELECT id, owner_type, agent_id, team_id, title, summary, context_used_ratio, context_used_tokens, max_context_used_ratio, last_context_window_tokens, context_status,
	 dialog_mode, provider, model, status, message_count, run_count, model_call_count, tool_call_count, skill_call_count,
	 mcp_call_count, input_tokens, output_tokens, total_tokens, total_cost_micro_usd, last_message_at, created_at, updated_at, archived_at, deleted_at FROM sessions`
}

func contextStatusForRatio(ratio float64) string {
	switch {
	case ratio >= 0.95:
		return "exceeded"
	case ratio >= 0.8:
		return "critical"
	case ratio >= 0.6:
		return "warning"
	default:
		return "normal"
	}
}

func scanSession(row scanner) (domain.Session, error) {
	var v domain.Session
	err := row.Scan(&v.ID, &v.OwnerType, &v.AgentID, &v.TeamID, &v.Title, &v.Summary, &v.ContextUsedRatio, &v.ContextUsedTokens, &v.MaxContextUsedRatio, &v.LastContextWindowTokens, &v.ContextStatus, &v.DialogMode, &v.Provider, &v.Model, &v.Status, &v.MessageCount, &v.RunCount, &v.ModelCallCount, &v.ToolCallCount, &v.SkillCallCount, &v.MCPCallCount, &v.InputTokens, &v.OutputTokens, &v.TotalTokens, &v.TotalCostMicroUSD, &v.LastMessageAt, &v.CreatedAt, &v.UpdatedAt, &v.ArchivedAt, &v.DeletedAt)
	return v, err
}

func scanSessions(rows *sql.Rows) ([]domain.Session, error) {
	var result []domain.Session
	for rows.Next() {
		v, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
