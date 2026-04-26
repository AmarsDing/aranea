// Package repository – SQLite-backed implementation of the L3 semantic
// memory described in `aranea/docs/15 memory-L3-semantic.md`. The file
// follows the same conventions as sqlite_memory_l2.go: write paths fill
// in defaults, JSON columns are normalised via helpers, and the FTS index
// is kept consistent through a delete-then-insert pattern.
package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"arenea/backend/internal/domain"
)

// CreateFact inserts a new memory_facts row. Callers are responsible for
// computing the fingerprint and embedding columns; the repository fills
// timestamps and JSON defaults so the row is consistent regardless of
// which path produced it.
func (r *SQLiteRepository) CreateFact(f domain.MemoryFact) (domain.MemoryFact, error) {
	if f.ID == "" {
		return domain.MemoryFact{}, errors.New("fact id is required")
	}
	if f.ScopeType == "" {
		return domain.MemoryFact{}, errors.New("fact scope_type is required")
	}
	if strings.TrimSpace(f.Statement) == "" {
		return domain.MemoryFact{}, errors.New("fact statement is required")
	}
	now := nowISO()
	if f.CreatedAt == "" {
		f.CreatedAt = now
	}
	f.UpdatedAt = now
	if f.Kind == "" {
		f.Kind = domain.FactGeneric
	}
	if f.Status == "" {
		f.Status = domain.FactStatusActive
	}
	if f.SourceKind == "" {
		f.SourceKind = "user"
	}
	if f.EmbeddingStatus == "" {
		f.EmbeddingStatus = "pending"
	}
	if f.DecayFactor == 0 {
		f.DecayFactor = 0.98
	}
	if f.Version == 0 {
		f.Version = 1
	}
	if f.Confidence == 0 {
		f.Confidence = 0.7
	}
	if f.Importance == 0 {
		f.Importance = 0.5
	}
	tagsJSON := encodeStringList(f.Tags)
	metaJSON := f.MetadataJSON
	if metaJSON == "" {
		metaJSON = "{}"
	}
	pii := 0
	if f.PIIFlag {
		pii = 1
	}
	_, err := r.db.Exec(
		`INSERT INTO memory_facts(
			id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
			statement, statement_normalized, fingerprint, details_markdown,
			fact_kind, tags_json,
			confidence, importance, use_count, hit_count,
			positive_feedback_count, negative_feedback_count, conflict_count,
			source_kind, source_episode_id, source_session_id, source_message_id, source_external,
			version, status, superseded_by,
			embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
			pii_flag, redacted_statement,
			ttl_days, decay_factor, next_decay_at, last_used_at, expires_at,
			metadata_json, created_at, updated_at, archived_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, string(f.ScopeType), f.ScopeID, f.WorkspaceID, f.UserID, f.TeamID, f.AgentID,
		f.Statement, f.StatementNormalized, f.Fingerprint, f.DetailsMarkdown,
		string(f.Kind), tagsJSON,
		f.Confidence, f.Importance, f.UseCount, f.HitCount,
		f.PositiveFeedbackCount, f.NegativeFeedbackCount, f.ConflictCount,
		f.SourceKind, f.SourceEpisodeID, f.SourceSessionID, f.SourceMessageID, f.SourceExternal,
		f.Version, f.Status, f.SupersededBy,
		f.EmbeddingStatus, f.EmbeddingModel, f.EmbeddingDim, f.EmbeddingBlob, f.EmbeddingNorm,
		pii, f.RedactedStatement,
		f.TTLDays, f.DecayFactor, f.NextDecayAt, f.LastUsedAt, f.ExpiresAt,
		metaJSON, f.CreatedAt, f.UpdatedAt, f.ArchivedAt, f.DeletedAt,
	)
	if err != nil {
		return domain.MemoryFact{}, err
	}
	return r.GetFact(f.ID)
}

// UpdateFact rewrites the mutable columns. Embedding columns are excluded
// — they have a dedicated UpsertFactEmbedding entry-point.
func (r *SQLiteRepository) UpdateFact(f domain.MemoryFact) error {
	if f.ID == "" {
		return errors.New("fact id is required")
	}
	now := nowISO()
	f.UpdatedAt = now
	tagsJSON := encodeStringList(f.Tags)
	metaJSON := f.MetadataJSON
	if metaJSON == "" {
		metaJSON = "{}"
	}
	pii := 0
	if f.PIIFlag {
		pii = 1
	}
	_, err := r.db.Exec(
		`UPDATE memory_facts SET
			statement = ?, statement_normalized = ?, fingerprint = ?, details_markdown = ?,
			fact_kind = ?, tags_json = ?,
			confidence = ?, importance = ?,
			source_kind = ?, source_episode_id = ?, source_session_id = ?, source_message_id = ?, source_external = ?,
			version = ?, status = ?, superseded_by = ?,
			pii_flag = ?, redacted_statement = ?,
			ttl_days = ?, decay_factor = ?, next_decay_at = ?, last_used_at = ?, expires_at = ?,
			metadata_json = ?, updated_at = ?, archived_at = ?, deleted_at = ?
		 WHERE id = ?`,
		f.Statement, f.StatementNormalized, f.Fingerprint, f.DetailsMarkdown,
		string(f.Kind), tagsJSON,
		f.Confidence, f.Importance,
		f.SourceKind, f.SourceEpisodeID, f.SourceSessionID, f.SourceMessageID, f.SourceExternal,
		f.Version, f.Status, f.SupersededBy,
		pii, f.RedactedStatement,
		f.TTLDays, f.DecayFactor, f.NextDecayAt, f.LastUsedAt, f.ExpiresAt,
		metaJSON, now, f.ArchivedAt, f.DeletedAt,
		f.ID,
	)
	return err
}

// GetFact returns a single fact by ID. Soft-deleted rows are included so
// the audit / version paths can still hydrate them; service-level filters
// strip them when serving Recall / List endpoints.
func (r *SQLiteRepository) GetFact(id string) (domain.MemoryFact, error) {
	row := r.db.QueryRow(memoryFactSelectSQL()+` WHERE id = ?`, id)
	return scanMemoryFact(row)
}

// GetFactByFingerprint is the dedup probe used by UpsertFact. It returns
// sql.ErrNoRows when nothing matches so callers can branch on
// errors.Is(err, sql.ErrNoRows).
func (r *SQLiteRepository) GetFactByFingerprint(scopeType domain.ScopeType, scopeID, fp string) (domain.MemoryFact, error) {
	row := r.db.QueryRow(
		memoryFactSelectSQL()+` WHERE scope_type = ? AND scope_id = ? AND fingerprint = ?`,
		string(scopeType), scopeID, fp,
	)
	return scanMemoryFact(row)
}

// ListFacts returns paginated, filtered facts for §6.2 GET endpoints.
// Soft-deleted rows are excluded.
func (r *SQLiteRepository) ListFacts(q FactListQuery) ([]domain.MemoryFact, int, error) {
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.Limit > 200 {
		q.Limit = 200
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	where := []string{"deleted_at = ''"}
	args := []any{}
	if q.ScopeType != "" {
		where = append(where, "scope_type = ?")
		args = append(args, string(q.ScopeType))
	}
	if q.ScopeID != "" {
		where = append(where, "scope_id = ?")
		args = append(args, q.ScopeID)
	}
	if q.WorkspaceID != "" {
		where = append(where, "workspace_id = ?")
		args = append(args, q.WorkspaceID)
	}
	if q.UserID != "" {
		where = append(where, "user_id = ?")
		args = append(args, q.UserID)
	}
	if q.TeamID != "" {
		where = append(where, "team_id = ?")
		args = append(args, q.TeamID)
	}
	if q.AgentID != "" {
		where = append(where, "agent_id = ?")
		args = append(args, q.AgentID)
	}
	if q.Status != "" {
		where = append(where, "status = ?")
		args = append(args, q.Status)
	}
	if q.Kind != "" {
		where = append(where, "fact_kind = ?")
		args = append(args, string(q.Kind))
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		where = append(where, "(LOWER(statement) LIKE ? OR LOWER(details_markdown) LIKE ?)")
		like := "%" + strings.ToLower(kw) + "%"
		args = append(args, like, like)
	}
	for _, tag := range q.Tags {
		t := strings.TrimSpace(tag)
		if t == "" {
			continue
		}
		where = append(where, "LOWER(tags_json) LIKE ?")
		args = append(args, "%\""+strings.ToLower(t)+"\"%")
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM memory_facts WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, q.Limit, q.Offset)
	rows, err := r.db.Query(
		memoryFactSelectSQL()+` WHERE `+whereSQL+` ORDER BY importance DESC, updated_at DESC LIMIT ? OFFSET ?`,
		listArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []domain.MemoryFact{}
	for rows.Next() {
		v, scanErr := scanMemoryFact(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

// UpdateFactConfidence applies a feedback-driven update in a single
// statement so concurrent feedback writes can't race on the value. The
// caller passes the *target* confidence (clamped 0..1) plus the counter
// increments — the repo only persists.
func (r *SQLiteRepository) UpdateFactConfidence(id string, newConfidence float64, hitInc, posInc, negInc int) error {
	if id == "" {
		return errors.New("fact id is required")
	}
	if newConfidence < 0 {
		newConfidence = 0
	}
	if newConfidence > 1 {
		newConfidence = 1
	}
	_, err := r.db.Exec(
		`UPDATE memory_facts SET
			confidence = ?,
			hit_count = hit_count + ?,
			positive_feedback_count = positive_feedback_count + ?,
			negative_feedback_count = negative_feedback_count + ?,
			updated_at = ?
		 WHERE id = ?`,
		newConfidence, hitInc, posInc, negInc, nowISO(), id,
	)
	return err
}

// UpdateFactStatus mutates the lifecycle column. Pass archivedAt to
// timestamp the archive transition; empty values keep the existing value
// untouched.
func (r *SQLiteRepository) UpdateFactStatus(id, status, supersededBy, archivedAt string) error {
	if id == "" {
		return errors.New("fact id is required")
	}
	if status == "" {
		return errors.New("status is required")
	}
	if archivedAt == "" && status == domain.FactStatusArchived {
		archivedAt = nowISO()
	}
	_, err := r.db.Exec(
		`UPDATE memory_facts SET status = ?, superseded_by = ?, archived_at = COALESCE(NULLIF(?, ''), archived_at), updated_at = ? WHERE id = ?`,
		status, supersededBy, archivedAt, nowISO(), id,
	)
	return err
}

// BumpFactUseStat is called from the recall path after a fact has been
// rendered into the prompt. The caller passes hit=true when the fact was
// actually injected; hit=false is used for "retrieved but not surfaced"
// recordkeeping.
func (r *SQLiteRepository) BumpFactUseStat(id string, hit bool, atISO string) error {
	if id == "" {
		return errors.New("fact id is required")
	}
	if atISO == "" {
		atISO = nowISO()
	}
	hitInc := 0
	if hit {
		hitInc = 1
	}
	_, err := r.db.Exec(
		`UPDATE memory_facts SET use_count = use_count + 1, hit_count = hit_count + ?, last_used_at = ?, updated_at = ? WHERE id = ?`,
		hitInc, atISO, nowISO(), id,
	)
	return err
}

// InsertFactVersion records a snapshot in `memory_fact_versions` for
// rollback / audit. The (fact_id, version) UNIQUE constraint guards
// against double-writes.
func (r *SQLiteRepository) InsertFactVersion(fv domain.FactVersion) error {
	if fv.ID == "" {
		return errors.New("version id is required")
	}
	if fv.FactID == "" {
		return errors.New("version fact_id is required")
	}
	if fv.Version <= 0 {
		return errors.New("version number must be positive")
	}
	if fv.CreatedAt == "" {
		fv.CreatedAt = nowISO()
	}
	tagsJSON := encodeStringList(fv.Tags)
	if fv.DiffJSON == "" {
		fv.DiffJSON = "{}"
	}
	_, err := r.db.Exec(
		`INSERT INTO memory_fact_versions(
			id, fact_id, version, statement, details_markdown, tags_json,
			confidence, status, changed_by, change_reason, diff_json, metadata_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fv.ID, fv.FactID, fv.Version, fv.Statement, fv.Details, tagsJSON,
		fv.Confidence, fv.Status, fv.ChangedBy, fv.ChangeReason, fv.DiffJSON, "{}", fv.CreatedAt,
	)
	return err
}

// ListFactVersions returns the version history sorted descending so the
// latest revision appears first. Limit caps the result to keep the API
// response small.
func (r *SQLiteRepository) ListFactVersions(factID string, limit int) ([]domain.FactVersion, error) {
	if factID == "" {
		return nil, errors.New("fact id is required")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := r.db.Query(
		`SELECT id, fact_id, version, statement, details_markdown, tags_json, confidence, status, changed_by, change_reason, diff_json, created_at
		 FROM memory_fact_versions WHERE fact_id = ? ORDER BY version DESC LIMIT ?`,
		factID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.FactVersion{}
	for rows.Next() {
		v, scanErr := scanFactVersion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetFactVersion looks up a specific (fact_id, version) tuple — used by
// the rollback path so the service can replay the snapshot.
func (r *SQLiteRepository) GetFactVersion(factID string, version int) (domain.FactVersion, error) {
	if factID == "" {
		return domain.FactVersion{}, errors.New("fact id is required")
	}
	row := r.db.QueryRow(
		`SELECT id, fact_id, version, statement, details_markdown, tags_json, confidence, status, changed_by, change_reason, diff_json, created_at
		 FROM memory_fact_versions WHERE fact_id = ? AND version = ?`,
		factID, version,
	)
	return scanFactVersion(row)
}

// InsertFactFeedback appends a row in `memory_fact_feedback`. The service
// layer is responsible for translating the feedback into a confidence /
// importance update via UpdateFactConfidence.
func (r *SQLiteRepository) InsertFactFeedback(fb domain.FactFeedback) (domain.FactFeedback, error) {
	if fb.ID == "" {
		return domain.FactFeedback{}, errors.New("feedback id is required")
	}
	if fb.FactID == "" {
		return domain.FactFeedback{}, errors.New("feedback fact_id is required")
	}
	if fb.Type == "" {
		return domain.FactFeedback{}, errors.New("feedback type is required")
	}
	if fb.Source == "" {
		return domain.FactFeedback{}, errors.New("feedback source is required")
	}
	if fb.CreatedAt == "" {
		fb.CreatedAt = nowISO()
	}
	if fb.Weight == 0 {
		fb.Weight = 1.0
	}
	metaJSON := encodeMetaJSONMap(fb.Metadata)
	_, err := r.db.Exec(
		`INSERT INTO memory_fact_feedback(
			id, fact_id, session_id, agent_id, feedback_type, source, weight, comment, metadata_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fb.ID, fb.FactID, fb.SessionID, fb.AgentID, fb.Type, fb.Source, fb.Weight, fb.Comment, metaJSON, fb.CreatedAt,
	)
	if err != nil {
		return domain.FactFeedback{}, err
	}
	return fb, nil
}

// ListFactFeedback returns the most-recent feedback entries for a fact.
func (r *SQLiteRepository) ListFactFeedback(factID string, limit int) ([]domain.FactFeedback, error) {
	if factID == "" {
		return nil, errors.New("fact id is required")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := r.db.Query(
		`SELECT id, fact_id, session_id, agent_id, feedback_type, source, weight, comment, metadata_json, created_at
		 FROM memory_fact_feedback WHERE fact_id = ? ORDER BY created_at DESC LIMIT ?`,
		factID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.FactFeedback{}
	for rows.Next() {
		var v domain.FactFeedback
		var meta string
		if err = rows.Scan(&v.ID, &v.FactID, &v.SessionID, &v.AgentID, &v.Type, &v.Source, &v.Weight, &v.Comment, &meta, &v.CreatedAt); err != nil {
			return nil, err
		}
		if meta != "" && meta != "{}" {
			_ = json.Unmarshal([]byte(meta), &v.Metadata)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// CountRecentFactFeedback counts the most recent N feedback rows of a
// given type — used by §5.4 step 5 ("3 consecutive rejects auto-create a
// conflict"). limit bounds the lookback window.
func (r *SQLiteRepository) CountRecentFactFeedback(factID, feedbackType string, limit int) (int, error) {
	if factID == "" {
		return 0, errors.New("fact id is required")
	}
	if limit <= 0 {
		limit = 3
	}
	rows, err := r.db.Query(
		`SELECT feedback_type FROM memory_fact_feedback WHERE fact_id = ? ORDER BY created_at DESC LIMIT ?`,
		factID, limit,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var t string
		if err = rows.Scan(&t); err != nil {
			return 0, err
		}
		if t == feedbackType {
			count++
		} else {
			break
		}
	}
	return count, rows.Err()
}

// CountAgentFactFeedbackSince returns how many `memory_fact_feedback`
// rows attributed to `agentID` were created at-or-after `since` and
// whose `feedback_type` is in `feedbackTypes`. Used by the
// EvolutionScanner to evaluate the §5.5 negative-feedback trigger
// without dragging the full feedback history into memory.
func (r *SQLiteRepository) CountAgentFactFeedbackSince(agentID string, feedbackTypes []string, since string) (int, error) {
	if agentID == "" {
		return 0, errors.New("agent id is required")
	}
	if since == "" {
		return 0, errors.New("since is required")
	}
	args := []any{agentID, since}
	q := `SELECT COUNT(1) FROM memory_fact_feedback
	      WHERE agent_id = ? AND created_at >= ?`
	if len(feedbackTypes) > 0 {
		placeholders := strings.Repeat("?,", len(feedbackTypes))
		placeholders = placeholders[:len(placeholders)-1]
		q += " AND feedback_type IN (" + placeholders + ")"
		for _, t := range feedbackTypes {
			args = append(args, t)
		}
	}
	var n int
	if err := r.db.QueryRow(q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// UpsertFactConflict inserts or updates the conflict row keyed on the
// (fact_a_id, fact_b_id) tuple. The service layer normalises the IDs so
// the same pair can't appear twice.
func (r *SQLiteRepository) UpsertFactConflict(c domain.FactConflict) (domain.FactConflict, error) {
	if c.FactAID == "" || c.FactBID == "" {
		return domain.FactConflict{}, errors.New("conflict fact ids are required")
	}
	if c.ID == "" {
		c.ID = c.FactAID + ":" + c.FactBID
	}
	if c.Kind == "" {
		c.Kind = domain.FactConflictContradiction
	}
	if c.Status == "" {
		c.Status = domain.FactConflictStatusOpen
	}
	now := nowISO()
	if c.CreatedAt == "" {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	_, err := r.db.Exec(
		`INSERT INTO memory_fact_conflicts(
			id, fact_a_id, fact_b_id, scope_type, scope_id, conflict_kind, similarity, status,
			detected_by, resolution, resolved_by, resolved_at, metadata_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(fact_a_id, fact_b_id) DO UPDATE SET
			conflict_kind = excluded.conflict_kind,
			similarity = excluded.similarity,
			status = excluded.status,
			detected_by = excluded.detected_by,
			updated_at = excluded.updated_at`,
		c.ID, c.FactAID, c.FactBID, string(c.ScopeType), c.ScopeID, c.Kind, c.Similarity, c.Status,
		c.DetectedBy, c.Resolution, c.ResolvedBy, c.ResolvedAt, "{}", c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return domain.FactConflict{}, err
	}
	return r.GetFactConflict(c.ID)
}

// GetFactConflict returns a conflict by id.
func (r *SQLiteRepository) GetFactConflict(id string) (domain.FactConflict, error) {
	row := r.db.QueryRow(
		`SELECT id, fact_a_id, fact_b_id, scope_type, scope_id, conflict_kind, similarity, status,
			detected_by, resolution, resolved_by, resolved_at, created_at, updated_at
		 FROM memory_fact_conflicts WHERE id = ?`,
		id,
	)
	return scanFactConflict(row)
}

// ListOpenFactConflicts returns conflicts whose status is "open" within
// the given scope. Empty scope params disable the corresponding filter.
func (r *SQLiteRepository) ListOpenFactConflicts(scope domain.ScopeType, scopeID string, limit int) ([]domain.FactConflict, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	where := []string{"status = ?"}
	args := []any{domain.FactConflictStatusOpen}
	if scope != "" {
		where = append(where, "scope_type = ?")
		args = append(args, string(scope))
	}
	if scopeID != "" {
		where = append(where, "scope_id = ?")
		args = append(args, scopeID)
	}
	args = append(args, limit)
	rows, err := r.db.Query(
		`SELECT id, fact_a_id, fact_b_id, scope_type, scope_id, conflict_kind, similarity, status,
			detected_by, resolution, resolved_by, resolved_at, created_at, updated_at
		 FROM memory_fact_conflicts WHERE `+strings.Join(where, " AND ")+` ORDER BY created_at DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.FactConflict{}
	for rows.Next() {
		v, scanErr := scanFactConflict(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// UpdateFactConflictResolution marks a conflict as resolved (or ignored).
func (r *SQLiteRepository) UpdateFactConflictResolution(id, status, resolution, by, resolvedAt string) error {
	if id == "" {
		return errors.New("conflict id is required")
	}
	if status == "" {
		status = domain.FactConflictStatusResolved
	}
	if resolvedAt == "" {
		resolvedAt = nowISO()
	}
	_, err := r.db.Exec(
		`UPDATE memory_fact_conflicts SET status = ?, resolution = ?, resolved_by = ?, resolved_at = ?, updated_at = ? WHERE id = ?`,
		status, resolution, by, resolvedAt, nowISO(), id,
	)
	return err
}

// UpsertFactEmbedding writes both the `memory_facts` embedding columns
// and the `memory_fact_index` mirror so vector search has everything it
// needs in one table.
func (r *SQLiteRepository) UpsertFactEmbedding(id, model string, dim int, blob []byte, norm float64) error {
	if id == "" {
		return errors.New("fact id is required")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(
		`UPDATE memory_facts SET embedding_status = 'ready', embedding_model = ?, embedding_dim = ?, embedding_blob = ?, embedding_norm = ?, updated_at = ? WHERE id = ?`,
		model, dim, blob, norm, nowISO(), id,
	); err != nil {
		return err
	}
	row := tx.QueryRow(`SELECT scope_type, scope_id, importance, confidence FROM memory_facts WHERE id = ?`, id)
	var scopeType, scopeID string
	var importance, confidence float64
	if err = row.Scan(&scopeType, &scopeID, &importance, &confidence); err != nil {
		return err
	}
	if _, err = tx.Exec(
		`INSERT INTO memory_fact_index(fact_id, scope_type, scope_id, embedding_model, embedding_dim, embedding_blob, embedding_norm, importance, confidence, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(fact_id) DO UPDATE SET
			embedding_model = excluded.embedding_model,
			embedding_dim = excluded.embedding_dim,
			embedding_blob = excluded.embedding_blob,
			embedding_norm = excluded.embedding_norm,
			importance = excluded.importance,
			confidence = excluded.confidence,
			updated_at = excluded.updated_at`,
		id, scopeType, scopeID, model, dim, blob, norm, importance, confidence, nowISO(),
	); err != nil {
		return err
	}
	return tx.Commit()
}

// UpsertFactsFTS keeps the BM25 index in sync. FTS5 has no upsert so we
// delete-then-insert. Empty text removes the row entirely.
func (r *SQLiteRepository) UpsertFactsFTS(factID string, scopeType domain.ScopeType, scopeID, kind, text string) error {
	if factID == "" {
		return errors.New("fact id is required")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM memory_facts_fts WHERE fact_id = ?`, factID); err != nil {
		return err
	}
	if strings.TrimSpace(text) != "" {
		if _, err = tx.Exec(
			`INSERT INTO memory_facts_fts(fact_id, scope_type, scope_id, fact_kind, text) VALUES (?, ?, ?, ?, ?)`,
			factID, string(scopeType), scopeID, kind, text,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteFactIndex removes the BM25 + vector index entries for a fact.
// The fact row itself is preserved (soft-delete is handled by
// UpdateFactStatus).
func (r *SQLiteRepository) DeleteFactIndex(factID string) error {
	if factID == "" {
		return errors.New("fact id is required")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM memory_facts_fts WHERE fact_id = ?`, factID); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM memory_fact_index WHERE fact_id = ?`, factID); err != nil {
		return err
	}
	return tx.Commit()
}

// SearchFactsBM25 runs an FTS5 MATCH against the facts index and returns
// matching facts joined with their meta row. Negative bm25 values come
// from FTS5 (lower is better) so we flip the sign for the service layer.
func (r *SQLiteRepository) SearchFactsBM25(scopes []domain.ScopeType, scopeIDs []string, query string, limit int) ([]domain.FactRecallHit, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	where := []string{"memory_facts_fts MATCH ?", "f.deleted_at = ''", "f.status = ?"}
	args := []any{q, domain.FactStatusActive}
	if scopeFilter, scopeArgs := buildScopeFilter("memory_facts_fts.scope_type", "memory_facts_fts.scope_id", scopes, scopeIDs); scopeFilter != "" {
		where = append(where, scopeFilter)
		args = append(args, scopeArgs...)
	}
	args = append(args, limit)
	rows, err := r.db.Query(
		`SELECT f.id, -bm25(memory_facts_fts) AS score
		 FROM memory_facts_fts
		 JOIN memory_facts f ON f.id = memory_facts_fts.fact_id
		 WHERE `+strings.Join(where, " AND ")+`
		 ORDER BY score DESC
		 LIMIT ?`,
		args...,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "syntax error") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	type row struct {
		id    string
		score float64
	}
	var hits []row
	for rows.Next() {
		var h row
		if err = rows.Scan(&h.id, &h.score); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := make([]domain.FactRecallHit, 0, len(hits))
	for _, h := range hits {
		f, gErr := r.GetFact(h.id)
		if gErr != nil {
			continue
		}
		out = append(out, domain.FactRecallHit{
			Fact:      f,
			BM25Score: h.score,
			Reason:    "bm25",
		})
	}
	return out, nil
}

// SearchFactsVector computes cosine similarity in Go because SQLite has
// no native vector type. The scope filter is pushed down so we never load
// embeddings outside the agent's permitted scopes. Phase 1 stub: when no
// embeddings exist (typical first-run state) the function returns an
// empty slice without erroring.
func (r *SQLiteRepository) SearchFactsVector(scopes []domain.ScopeType, scopeIDs []string, q []float32, limit int) ([]domain.FactRecallHit, error) {
	if len(q) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	where := []string{"f.deleted_at = ''", "f.status = ?", "f.embedding_status = 'ready'", "i.embedding_dim = ?"}
	args := []any{domain.FactStatusActive, len(q)}
	if scopeFilter, scopeArgs := buildScopeFilter("i.scope_type", "i.scope_id", scopes, scopeIDs); scopeFilter != "" {
		where = append(where, scopeFilter)
		args = append(args, scopeArgs...)
	}
	rows, err := r.db.Query(
		`SELECT f.id, i.embedding_blob, i.embedding_norm
		 FROM memory_fact_index i
		 JOIN memory_facts f ON f.id = i.fact_id
		 WHERE `+strings.Join(where, " AND "),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type cand struct {
		id    string
		score float64
	}
	qNorm := vectorNorm(q)
	if qNorm == 0 {
		return nil, nil
	}
	var cands []cand
	for rows.Next() {
		var id string
		var blob []byte
		var norm float64
		if err = rows.Scan(&id, &blob, &norm); err != nil {
			return nil, err
		}
		vec, decErr := decodeFloat32Blob(blob)
		if decErr != nil || len(vec) != len(q) {
			continue
		}
		denom := norm * qNorm
		if denom == 0 {
			continue
		}
		cands = append(cands, cand{id: id, score: dotProduct(vec, q) / denom})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	if len(cands) > limit {
		cands = cands[:limit]
	}
	out := make([]domain.FactRecallHit, 0, len(cands))
	for _, c := range cands {
		f, gErr := r.GetFact(c.id)
		if gErr != nil {
			continue
		}
		out = append(out, domain.FactRecallHit{
			Fact:        f,
			VectorScore: c.score,
			Reason:      "vector",
		})
	}
	return out, nil
}

// ListFactsDueForDecay returns active facts whose next_decay_at is in the
// past so the decay worker can iterate them in batches.
func (r *SQLiteRepository) ListFactsDueForDecay(before string, limit int) ([]domain.MemoryFact, error) {
	if before == "" {
		before = nowISO()
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := r.db.Query(
		memoryFactSelectSQL()+` WHERE deleted_at = '' AND status = ? AND (next_decay_at = '' OR next_decay_at <= ?) ORDER BY confidence ASC LIMIT ?`,
		domain.FactStatusActive, before, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.MemoryFact{}
	for rows.Next() {
		v, scanErr := scanMemoryFact(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ApplyFactDecay multiplies confidence by `factor` and bumps next_decay_at.
// The clamp at zero is defensive; the service layer should never request
// a negative factor.
func (r *SQLiteRepository) ApplyFactDecay(factID string, factor float64, nextAt string) error {
	if factID == "" {
		return errors.New("fact id is required")
	}
	if factor <= 0 {
		factor = 0.98
	}
	if nextAt == "" {
		nextAt = nowISO()
	}
	_, err := r.db.Exec(
		`UPDATE memory_facts SET confidence = MAX(0, MIN(1, confidence * ?)), next_decay_at = ?, updated_at = ? WHERE id = ?`,
		factor, nextAt, nowISO(), factID,
	)
	return err
}

// ArchiveFactsBelowConfidence marks active facts whose confidence has
// fallen below the threshold as "archived" in one batch. Returns the
// number of rows affected.
func (r *SQLiteRepository) ArchiveFactsBelowConfidence(threshold float64, limit int) (int, error) {
	if threshold <= 0 {
		threshold = 0.2
	}
	if limit <= 0 {
		limit = 500
	}
	if limit > 5000 {
		limit = 5000
	}
	now := nowISO()
	res, err := r.db.Exec(
		`UPDATE memory_facts SET status = ?, archived_at = ?, updated_at = ?
		 WHERE id IN (
		   SELECT id FROM memory_facts WHERE deleted_at = '' AND status = ? AND confidence < ? ORDER BY confidence ASC LIMIT ?
		 )`,
		domain.FactStatusArchived, now, now, domain.FactStatusActive, threshold, limit,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// CountFactsByStatus aggregates row counts by status for the admin stats
// dashboard. Empty scope params disable the corresponding filter.
func (r *SQLiteRepository) CountFactsByStatus(scope domain.ScopeType, scopeID string) (map[string]int, error) {
	where := []string{"deleted_at = ''"}
	args := []any{}
	if scope != "" {
		where = append(where, "scope_type = ?")
		args = append(args, string(scope))
	}
	if scopeID != "" {
		where = append(where, "scope_id = ?")
		args = append(args, scopeID)
	}
	rows, err := r.db.Query(
		`SELECT status, COUNT(1) FROM memory_facts WHERE `+strings.Join(where, " AND ")+` GROUP BY status`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var s string
		var c int
		if err = rows.Scan(&s, &c); err != nil {
			return nil, err
		}
		out[s] = c
	}
	return out, rows.Err()
}

// --- Helpers ----------------------------------------------------------------

func memoryFactSelectSQL() string {
	return `SELECT id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
		statement, statement_normalized, fingerprint, details_markdown,
		fact_kind, tags_json,
		confidence, importance, use_count, hit_count,
		positive_feedback_count, negative_feedback_count, conflict_count,
		source_kind, source_episode_id, source_session_id, source_message_id, source_external,
		version, status, superseded_by,
		embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
		pii_flag, redacted_statement,
		ttl_days, decay_factor, next_decay_at, last_used_at, expires_at,
		metadata_json, created_at, updated_at, archived_at, deleted_at
	 FROM memory_facts`
}

func scanMemoryFact(row scanner) (domain.MemoryFact, error) {
	var v domain.MemoryFact
	var scope, kind, tagsJSON string
	var pii int
	var blob []byte
	if err := row.Scan(
		&v.ID, &scope, &v.ScopeID, &v.WorkspaceID, &v.UserID, &v.TeamID, &v.AgentID,
		&v.Statement, &v.StatementNormalized, &v.Fingerprint, &v.DetailsMarkdown,
		&kind, &tagsJSON,
		&v.Confidence, &v.Importance, &v.UseCount, &v.HitCount,
		&v.PositiveFeedbackCount, &v.NegativeFeedbackCount, &v.ConflictCount,
		&v.SourceKind, &v.SourceEpisodeID, &v.SourceSessionID, &v.SourceMessageID, &v.SourceExternal,
		&v.Version, &v.Status, &v.SupersededBy,
		&v.EmbeddingStatus, &v.EmbeddingModel, &v.EmbeddingDim, &blob, &v.EmbeddingNorm,
		&pii, &v.RedactedStatement,
		&v.TTLDays, &v.DecayFactor, &v.NextDecayAt, &v.LastUsedAt, &v.ExpiresAt,
		&v.MetadataJSON, &v.CreatedAt, &v.UpdatedAt, &v.ArchivedAt, &v.DeletedAt,
	); err != nil {
		return domain.MemoryFact{}, err
	}
	v.ScopeType = domain.ScopeType(scope)
	v.Kind = domain.FactKind(kind)
	v.Tags = decodeStringList(tagsJSON)
	v.PIIFlag = pii != 0
	v.EmbeddingBlob = blob
	return v, nil
}

func scanFactVersion(row scanner) (domain.FactVersion, error) {
	var v domain.FactVersion
	var tagsJSON string
	if err := row.Scan(&v.ID, &v.FactID, &v.Version, &v.Statement, &v.Details, &tagsJSON, &v.Confidence, &v.Status, &v.ChangedBy, &v.ChangeReason, &v.DiffJSON, &v.CreatedAt); err != nil {
		return domain.FactVersion{}, err
	}
	v.Tags = decodeStringList(tagsJSON)
	return v, nil
}

func scanFactConflict(row scanner) (domain.FactConflict, error) {
	var v domain.FactConflict
	var scope string
	if err := row.Scan(&v.ID, &v.FactAID, &v.FactBID, &scope, &v.ScopeID, &v.Kind, &v.Similarity, &v.Status, &v.DetectedBy, &v.Resolution, &v.ResolvedBy, &v.ResolvedAt, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return domain.FactConflict{}, err
	}
	v.ScopeType = domain.ScopeType(scope)
	return v, nil
}

// buildScopeFilter compiles the IN-list clause for scope-aware queries
// against either the FTS view or the meta index. We allow callers to pass
// scopeIDs that pair positionally with scopes (length match) or, for a
// short generic list, provide just the scope types.
func buildScopeFilter(scopeCol, scopeIDCol string, scopes []domain.ScopeType, scopeIDs []string) (string, []any) {
	if len(scopes) == 0 {
		return "", nil
	}
	// When scopeIDs is the same length as scopes, treat each entry as a
	// (scope, scope_id) pair so we never leak across users / teams.
	if len(scopeIDs) == len(scopes) {
		var clauses []string
		var args []any
		for i, sc := range scopes {
			clauses = append(clauses, fmt.Sprintf("(%s = ? AND %s = ?)", scopeCol, scopeIDCol))
			args = append(args, string(sc), scopeIDs[i])
		}
		return "(" + strings.Join(clauses, " OR ") + ")", args
	}
	// Otherwise filter by scope type only. Used for "global" scope or as
	// a fallback when caller doesn't know the scope id.
	placeholders := make([]string, 0, len(scopes))
	args := make([]any, 0, len(scopes))
	for _, sc := range scopes {
		placeholders = append(placeholders, "?")
		args = append(args, string(sc))
	}
	return fmt.Sprintf("%s IN (%s)", scopeCol, strings.Join(placeholders, ",")), args
}

func encodeStringList(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func decodeStringList(raw string) []string {
	if strings.TrimSpace(raw) == "" || raw == "[]" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func encodeMetaJSONMap(m map[string]any) string {
	if len(m) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(raw)
}
