package repository

import (
	"database/sql"

	"arenea/backend/internal/domain"
)

func addMessageTx(tx *sql.Tx, m domain.Message) error {
	if _, err := tx.Exec(
		`INSERT INTO messages(id, session_id, parent_message_id, turn_index, role, content_markdown, model_name, token_in, token_out, latency_ms, status, attachments_count, options_json, error_message, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.SessionID, m.ParentMessageID, m.TurnIndex, m.Role, m.Content, m.ModelName, m.TokenIn, m.TokenOut, m.LatencyMS, m.Status, m.AttachmentsCount, m.OptionsJSON, m.ErrorMessage, m.CreatedAt,
	); err != nil {
		return err
	}
	_, err := tx.Exec(`UPDATE sessions SET message_count = message_count + 1, last_message_at = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`, m.CreatedAt, m.CreatedAt, m.SessionID)
	return err
}

// ListLatestMessagesByTokens walks `messages` from newest to oldest and returns
// the longest suffix that fits within (maxTokens, hardCap). It is the primary
// data source for the L0 sliding-window assembly described in
// `12 memory-L0-sensory.md`. The returned slice is in chronological order
// (oldest first) so callers can append it directly to a prompt.
//
// The estimator uses `token_in + token_out` when present, falling back to a
// 4-rune-per-token approximation of `content_markdown`. We always keep at least
// one message so a fresh session never returns an empty window.
func (r *SQLiteRepository) ListLatestMessagesByTokens(sessionID string, maxTokens int, hardCap int) ([]domain.Message, error) {
	if hardCap <= 0 {
		hardCap = 200
	}
	rows, err := r.db.Query(
		`SELECT id, session_id, parent_message_id, turn_index, role, content_markdown, COALESCE(model_name, ''), token_in, token_out, latency_ms, status, attachments_count, options_json, error_message, created_at
		 FROM messages WHERE session_id = ? ORDER BY turn_index DESC, created_at DESC LIMIT ?`,
		sessionID, hardCap,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	collected := make([]domain.Message, 0, hardCap)
	totalTokens := 0
	for rows.Next() {
		var v domain.Message
		if err = rows.Scan(&v.ID, &v.SessionID, &v.ParentMessageID, &v.TurnIndex, &v.Role, &v.Content, &v.ModelName, &v.TokenIn, &v.TokenOut, &v.LatencyMS, &v.Status, &v.AttachmentsCount, &v.OptionsJSON, &v.ErrorMessage, &v.CreatedAt); err != nil {
			return nil, err
		}
		tokens := v.TokenIn + v.TokenOut
		if tokens <= 0 {
			tokens = approxTokensFromText(v.Content)
		}
		if maxTokens > 0 && len(collected) > 0 && totalTokens+tokens > maxTokens {
			break
		}
		totalTokens += tokens
		collected = append(collected, v)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
		collected[i], collected[j] = collected[j], collected[i]
	}
	return collected, nil
}

// approxTokensFromText is the same 4-rune heuristic used by the runtime
// adapter. Keeping it private here so the repository does not import runtime.
func approxTokensFromText(text string) int {
	runes := 0
	for range text {
		runes++
	}
	if runes == 0 {
		return 0
	}
	tokens := runes / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}
