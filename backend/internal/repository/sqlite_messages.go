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
