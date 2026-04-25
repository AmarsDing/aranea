package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"arenea/backend/internal/domain"
)

// CreateL1Task inserts a new memory_l1_tasks row. Defaults are applied so
// callers that only fill in the identity columns still get a sensible record.
// The unique key (session_id, task_key, agent_id) is enforced by the schema;
// duplicate inserts surface a constraint error and are mapped to ErrConflict
// at the service layer (spec §5.5).
func (r *SQLiteRepository) CreateL1Task(t domain.MemoryL1Task) (domain.MemoryL1Task, error) {
	if t.ID == "" {
		return domain.MemoryL1Task{}, errors.New("l1 task id is required")
	}
	if t.SessionID == "" {
		return domain.MemoryL1Task{}, errors.New("l1 task session_id is required")
	}
	now := nowISO()
	if t.StartedAt == "" {
		t.StartedAt = now
	}
	if t.CreatedAt == "" {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = domain.L1TaskActive
	}
	if t.SchemaVersion == 0 {
		t.SchemaVersion = 1
	}
	if t.BudgetTokens <= 0 {
		t.BudgetTokens = 8192
	}
	if t.TaskKey == "" {
		t.TaskKey = "default"
	}
	sharedJSON, err := marshalShared(t.SharedWith)
	if err != nil {
		return domain.MemoryL1Task{}, err
	}
	metaJSON, err := marshalAnyMap(t.Metadata)
	if err != nil {
		return domain.MemoryL1Task{}, err
	}
	_, err = r.db.Exec(
		`INSERT INTO memory_l1_tasks(
			id, session_id, run_id, team_id, agent_id,
			task_key, task_title, task_goal, status,
			schema_version, budget_tokens, used_tokens,
			parent_task_id, shared_with_json,
			started_at, ended_at, archived_at,
			metadata_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.SessionID, t.RunID, t.TeamID, t.AgentID,
		t.TaskKey, t.TaskTitle, t.TaskGoal, string(t.Status),
		t.SchemaVersion, t.BudgetTokens, t.UsedTokens,
		t.ParentTaskID, sharedJSON,
		t.StartedAt, t.EndedAt, t.ArchivedAt,
		metaJSON, t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		return domain.MemoryL1Task{}, err
	}
	return r.GetL1TaskByID(t.ID)
}

// UpdateL1TaskStatus mutates the lifecycle column. ChatService /
// TeamRuntime drive this through MemoryL1Service.EndTask. The endedAt /
// archivedAt arguments may be empty, in which case the existing column value
// is preserved.
func (r *SQLiteRepository) UpdateL1TaskStatus(taskID string, status domain.L1TaskStatus, endedAt string, archivedAt string) error {
	if taskID == "" {
		return errors.New("task id is required")
	}
	now := nowISO()
	_, err := r.db.Exec(
		`UPDATE memory_l1_tasks
		 SET status = ?,
		     ended_at = CASE WHEN ? = '' THEN ended_at ELSE ? END,
		     archived_at = CASE WHEN ? = '' THEN archived_at ELSE ? END,
		     updated_at = ?
		 WHERE id = ?`,
		string(status), endedAt, endedAt, archivedAt, archivedAt, now, taskID,
	)
	return err
}

func (r *SQLiteRepository) UpdateL1TaskUsedTokens(taskID string, usedTokens int) error {
	if taskID == "" {
		return errors.New("task id is required")
	}
	if usedTokens < 0 {
		usedTokens = 0
	}
	_, err := r.db.Exec(
		`UPDATE memory_l1_tasks SET used_tokens = ?, updated_at = ? WHERE id = ?`,
		usedTokens, nowISO(), taskID,
	)
	return err
}

func (r *SQLiteRepository) UpdateL1TaskShared(taskID string, shared []domain.L1FieldShare) error {
	if taskID == "" {
		return errors.New("task id is required")
	}
	encoded, err := marshalShared(shared)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(
		`UPDATE memory_l1_tasks SET shared_with_json = ?, updated_at = ? WHERE id = ?`,
		encoded, nowISO(), taskID,
	)
	return err
}

func (r *SQLiteRepository) UpdateL1TaskBudget(taskID string, budgetTokens int) error {
	if taskID == "" {
		return errors.New("task id is required")
	}
	if budgetTokens <= 0 {
		return errors.New("budget tokens must be positive")
	}
	_, err := r.db.Exec(
		`UPDATE memory_l1_tasks SET budget_tokens = ?, updated_at = ? WHERE id = ?`,
		budgetTokens, nowISO(), taskID,
	)
	return err
}

func (r *SQLiteRepository) GetL1TaskByID(taskID string) (domain.MemoryL1Task, error) {
	row := r.db.QueryRow(l1TaskSelectSQL()+` WHERE id = ?`, taskID)
	return scanL1Task(row)
}

func (r *SQLiteRepository) GetL1TaskByKey(sessionID, taskKey, agentID string) (domain.MemoryL1Task, error) {
	row := r.db.QueryRow(
		l1TaskSelectSQL()+` WHERE session_id = ? AND task_key = ? AND agent_id = ?`,
		sessionID, taskKey, agentID,
	)
	return scanL1Task(row)
}

func (r *SQLiteRepository) ListL1TasksBySession(query domain.L1TaskListQuery) ([]domain.MemoryL1Task, error) {
	clauses := []string{}
	args := []any{}
	if query.SessionID != "" {
		clauses = append(clauses, "session_id = ?")
		args = append(args, query.SessionID)
	}
	if query.AgentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, query.AgentID)
	}
	if query.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, query.Status)
	} else if !query.IncludeEnded {
		clauses = append(clauses, "status IN ('active','paused')")
	}
	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}
	rows, err := r.db.Query(l1TaskSelectSQL()+where+` ORDER BY updated_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.MemoryL1Task
	for rows.Next() {
		v, scanErr := scanL1Task(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// ArchiveIdleL1Tasks flips active tasks whose updated_at is older than the
// before cutoff to status=archived. Returns how many rows were touched so
// the cron caller can emit a metric.
func (r *SQLiteRepository) ArchiveIdleL1Tasks(before string) (int, error) {
	if before == "" {
		return 0, errors.New("before is required")
	}
	now := nowISO()
	res, err := r.db.Exec(
		`UPDATE memory_l1_tasks
		 SET status = 'archived', archived_at = ?, ended_at = COALESCE(NULLIF(ended_at, ''), ?), updated_at = ?
		 WHERE status IN ('active','paused') AND updated_at < ?`,
		now, now, now, before,
	)
	if err != nil {
		return 0, err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// UpsertL1Field writes the field row, appends a history entry, recomputes the
// owner task's used_tokens, and trims excess history rows. Everything happens
// inside a single transaction so partial failures don't leave the budget
// counter in a wrong state. Returns the field row as it lives in the database
// after the write.
func (r *SQLiteRepository) UpsertL1Field(f domain.MemoryL1Field, history domain.MemoryL1FieldHistory, keepRevisions int) (domain.MemoryL1Field, error) {
	if f.ID == "" {
		return domain.MemoryL1Field{}, errors.New("field id is required")
	}
	if f.TaskID == "" || f.FieldPath == "" {
		return domain.MemoryL1Field{}, errors.New("field task_id and field_path are required")
	}
	now := nowISO()
	if f.CreatedAt == "" {
		f.CreatedAt = now
	}
	f.UpdatedAt = now
	if f.FieldKind == "" {
		f.FieldKind = "string"
	}
	if f.Visibility == "" {
		f.Visibility = "prompt"
	}
	metaJSON, err := marshalAnyMap(f.Metadata)
	if err != nil {
		return domain.MemoryL1Field{}, err
	}

	tx, err := r.db.Begin()
	if err != nil {
		return domain.MemoryL1Field{}, err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO memory_l1_fields(
			id, task_id, session_id, agent_id,
			field_path, field_kind, visibility, pin_to_prompt, is_required,
			value_text, value_json, value_ref, preview, token_estimate,
			source, source_ref, ttl_seconds, expires_at,
			revision, last_read_at, read_count,
			metadata_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, field_path) DO UPDATE SET
			session_id = excluded.session_id,
			agent_id = excluded.agent_id,
			field_kind = excluded.field_kind,
			visibility = excluded.visibility,
			pin_to_prompt = excluded.pin_to_prompt,
			is_required = excluded.is_required,
			value_text = excluded.value_text,
			value_json = excluded.value_json,
			value_ref = excluded.value_ref,
			preview = excluded.preview,
			token_estimate = excluded.token_estimate,
			source = excluded.source,
			source_ref = excluded.source_ref,
			ttl_seconds = excluded.ttl_seconds,
			expires_at = excluded.expires_at,
			revision = excluded.revision,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at`,
		f.ID, f.TaskID, f.SessionID, f.AgentID,
		f.FieldPath, f.FieldKind, f.Visibility, f.PinToPrompt, f.IsRequired,
		f.ValueText, f.ValueJSON, f.ValueRef, f.Preview, f.TokenEstimate,
		f.Source, f.SourceRef, f.TTLSeconds, f.ExpiresAt,
		f.Revision, f.LastReadAt, f.ReadCount,
		metaJSON, f.CreatedAt, f.UpdatedAt,
	)
	if err != nil {
		return domain.MemoryL1Field{}, fmt.Errorf("upsert l1 field: %w", err)
	}

	if history.ID != "" {
		if history.CreatedAt == "" {
			history.CreatedAt = now
		}
		if history.MetadataJSON == "" {
			history.MetadataJSON = "{}"
		}
		if history.DiffJSON == "" {
			history.DiffJSON = "{}"
		}
		_, err = tx.Exec(
			`INSERT INTO memory_l1_field_history(
				id, field_id, task_id, revision,
				value_text, value_json, value_ref, preview, token_estimate,
				changed_by, change_reason, diff_json, metadata_json, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			history.ID, f.ID, f.TaskID, history.Revision,
			history.ValueText, history.ValueJSON, history.ValueRef, history.Preview, history.TokenEstimate,
			history.ChangedBy, history.ChangeReason, history.DiffJSON, history.MetadataJSON, history.CreatedAt,
		)
		if err != nil {
			return domain.MemoryL1Field{}, fmt.Errorf("insert l1 field history: %w", err)
		}
	}

	if keepRevisions > 0 {
		if _, err = tx.Exec(
			`DELETE FROM memory_l1_field_history
			 WHERE field_id = ? AND revision <= (
			   SELECT MAX(revision) - ? FROM memory_l1_field_history WHERE field_id = ?
			 )`,
			f.ID, keepRevisions, f.ID,
		); err != nil {
			return domain.MemoryL1Field{}, fmt.Errorf("trim l1 field history: %w", err)
		}
	}

	var sumTokens sql.NullInt64
	if err = tx.QueryRow(
		`SELECT COALESCE(SUM(token_estimate), 0) FROM memory_l1_fields WHERE task_id = ?`,
		f.TaskID,
	).Scan(&sumTokens); err != nil {
		return domain.MemoryL1Field{}, fmt.Errorf("recompute l1 used tokens: %w", err)
	}
	if _, err = tx.Exec(
		`UPDATE memory_l1_tasks SET used_tokens = ?, updated_at = ? WHERE id = ?`,
		int(sumTokens.Int64), now, f.TaskID,
	); err != nil {
		return domain.MemoryL1Field{}, fmt.Errorf("update l1 task used_tokens: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return domain.MemoryL1Field{}, err
	}
	return r.GetL1FieldByID(f.ID)
}

func (r *SQLiteRepository) GetL1Field(taskID, fieldPath string) (domain.MemoryL1Field, error) {
	row := r.db.QueryRow(l1FieldSelectSQL()+` WHERE task_id = ? AND field_path = ?`, taskID, fieldPath)
	return scanL1Field(row)
}

func (r *SQLiteRepository) GetL1FieldByID(fieldID string) (domain.MemoryL1Field, error) {
	row := r.db.QueryRow(l1FieldSelectSQL()+` WHERE id = ?`, fieldID)
	return scanL1Field(row)
}

func (r *SQLiteRepository) ListL1FieldsByTask(taskID string, includeInternal bool) ([]domain.MemoryL1Field, error) {
	q := l1FieldSelectSQL() + ` WHERE task_id = ?`
	if !includeInternal {
		q += ` AND visibility != 'internal'`
	}
	q += ` ORDER BY field_path ASC`
	rows, err := r.db.Query(q, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.MemoryL1Field
	for rows.Next() {
		v, scanErr := scanL1Field(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// DeleteL1Field removes the row, refreshes the parent task's used_tokens
// counter, and leaves the history rows in place so rollback still works
// (callers that want a hard purge can also delete from history separately).
func (r *SQLiteRepository) DeleteL1Field(fieldID string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var taskID string
	if err = tx.QueryRow(`SELECT task_id FROM memory_l1_fields WHERE id = ?`, fieldID).Scan(&taskID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	if _, err = tx.Exec(`DELETE FROM memory_l1_fields WHERE id = ?`, fieldID); err != nil {
		return err
	}
	var sumTokens sql.NullInt64
	if err = tx.QueryRow(
		`SELECT COALESCE(SUM(token_estimate), 0) FROM memory_l1_fields WHERE task_id = ?`,
		taskID,
	).Scan(&sumTokens); err != nil {
		return err
	}
	if _, err = tx.Exec(
		`UPDATE memory_l1_tasks SET used_tokens = ?, updated_at = ? WHERE id = ?`,
		int(sumTokens.Int64), nowISO(), taskID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SQLiteRepository) BumpL1FieldRead(fieldID string, atISO string) error {
	if atISO == "" {
		atISO = nowISO()
	}
	_, err := r.db.Exec(
		`UPDATE memory_l1_fields SET read_count = read_count + 1, last_read_at = ? WHERE id = ?`,
		atISO, fieldID,
	)
	return err
}

func (r *SQLiteRepository) ListL1FieldHistory(fieldID string, limit int) ([]domain.MemoryL1FieldHistory, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Query(
		`SELECT id, field_id, task_id, revision, value_text, value_json, value_ref, preview, token_estimate,
		        changed_by, change_reason, diff_json, metadata_json, created_at
		 FROM memory_l1_field_history
		 WHERE field_id = ?
		 ORDER BY revision DESC
		 LIMIT ?`,
		fieldID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.MemoryL1FieldHistory
	for rows.Next() {
		var v domain.MemoryL1FieldHistory
		if err = rows.Scan(&v.ID, &v.FieldID, &v.TaskID, &v.Revision, &v.ValueText, &v.ValueJSON, &v.ValueRef, &v.Preview, &v.TokenEstimate, &v.ChangedBy, &v.ChangeReason, &v.DiffJSON, &v.MetadataJSON, &v.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) GetL1FieldHistory(fieldID string, revision int) (domain.MemoryL1FieldHistory, error) {
	row := r.db.QueryRow(
		`SELECT id, field_id, task_id, revision, value_text, value_json, value_ref, preview, token_estimate,
		        changed_by, change_reason, diff_json, metadata_json, created_at
		 FROM memory_l1_field_history WHERE field_id = ? AND revision = ?`,
		fieldID, revision,
	)
	var v domain.MemoryL1FieldHistory
	if err := row.Scan(&v.ID, &v.FieldID, &v.TaskID, &v.Revision, &v.ValueText, &v.ValueJSON, &v.ValueRef, &v.Preview, &v.TokenEstimate, &v.ChangedBy, &v.ChangeReason, &v.DiffJSON, &v.MetadataJSON, &v.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.MemoryL1FieldHistory{}, domain.ErrNotFound
		}
		return domain.MemoryL1FieldHistory{}, err
	}
	return v, nil
}

func (r *SQLiteRepository) UpsertL1Schema(s domain.MemoryL1Schema) (domain.MemoryL1Schema, error) {
	if s.ID == "" {
		return domain.MemoryL1Schema{}, errors.New("schema id is required")
	}
	if s.SchemaKey == "" || s.ScopeType == "" {
		return domain.MemoryL1Schema{}, errors.New("schema scope_type and schema_key are required")
	}
	if s.SchemaJSON == "" {
		s.SchemaJSON = "{}"
	}
	if s.SchemaVersion == 0 {
		s.SchemaVersion = 1
	}
	now := nowISO()
	if s.CreatedAt == "" {
		s.CreatedAt = now
	}
	s.UpdatedAt = now
	metaJSON, err := marshalAnyMap(s.Metadata)
	if err != nil {
		return domain.MemoryL1Schema{}, err
	}
	_, err = r.db.Exec(
		`INSERT INTO memory_l1_schemas(
			id, scope_type, scope_id, schema_key, schema_version, schema_json,
			description, enabled, metadata_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope_type, scope_id, schema_key, schema_version) DO UPDATE SET
			schema_json = excluded.schema_json,
			description = excluded.description,
			enabled = excluded.enabled,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at`,
		s.ID, s.ScopeType, s.ScopeID, s.SchemaKey, s.SchemaVersion, s.SchemaJSON,
		s.Description, s.Enabled, metaJSON, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return domain.MemoryL1Schema{}, err
	}
	return r.GetL1SchemaByID(s.ID)
}

func (r *SQLiteRepository) ListL1Schemas(scopeType, scopeID string) ([]domain.MemoryL1Schema, error) {
	clauses := []string{}
	args := []any{}
	if scopeType != "" {
		clauses = append(clauses, "scope_type = ?")
		args = append(args, scopeType)
	}
	if scopeID != "" {
		clauses = append(clauses, "scope_id = ?")
		args = append(args, scopeID)
	}
	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}
	rows, err := r.db.Query(l1SchemaSelectSQL()+where+` ORDER BY scope_type ASC, schema_key ASC, schema_version DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.MemoryL1Schema
	for rows.Next() {
		v, scanErr := scanL1Schema(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) GetL1SchemaByID(id string) (domain.MemoryL1Schema, error) {
	row := r.db.QueryRow(l1SchemaSelectSQL()+` WHERE id = ?`, id)
	return scanL1Schema(row)
}

func (r *SQLiteRepository) DeleteL1Schema(id string) error {
	_, err := r.db.Exec(`DELETE FROM memory_l1_schemas WHERE id = ?`, id)
	return err
}

// --- internal helpers ---------------------------------------------------------

func l1TaskSelectSQL() string {
	return `SELECT id, session_id, run_id, team_id, agent_id,
		task_key, task_title, task_goal, status,
		schema_version, budget_tokens, used_tokens,
		parent_task_id, shared_with_json,
		started_at, ended_at, archived_at,
		metadata_json, created_at, updated_at
	 FROM memory_l1_tasks`
}

func scanL1Task(row scanner) (domain.MemoryL1Task, error) {
	var v domain.MemoryL1Task
	var status, sharedJSON, metaJSON string
	if err := row.Scan(
		&v.ID, &v.SessionID, &v.RunID, &v.TeamID, &v.AgentID,
		&v.TaskKey, &v.TaskTitle, &v.TaskGoal, &status,
		&v.SchemaVersion, &v.BudgetTokens, &v.UsedTokens,
		&v.ParentTaskID, &sharedJSON,
		&v.StartedAt, &v.EndedAt, &v.ArchivedAt,
		&metaJSON, &v.CreatedAt, &v.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return v, domain.ErrNotFound
		}
		return v, err
	}
	v.Status = domain.L1TaskStatus(status)
	if sharedJSON != "" {
		_ = json.Unmarshal([]byte(sharedJSON), &v.SharedWith)
	}
	if metaJSON != "" && metaJSON != "{}" {
		_ = json.Unmarshal([]byte(metaJSON), &v.Metadata)
	}
	return v, nil
}

func l1FieldSelectSQL() string {
	return `SELECT id, task_id, session_id, agent_id,
		field_path, field_kind, visibility, pin_to_prompt, is_required,
		value_text, value_json, value_ref, preview, token_estimate,
		source, source_ref, ttl_seconds, expires_at,
		revision, last_read_at, read_count,
		metadata_json, created_at, updated_at
	 FROM memory_l1_fields`
}

func scanL1Field(row scanner) (domain.MemoryL1Field, error) {
	var v domain.MemoryL1Field
	var metaJSON string
	if err := row.Scan(
		&v.ID, &v.TaskID, &v.SessionID, &v.AgentID,
		&v.FieldPath, &v.FieldKind, &v.Visibility, &v.PinToPrompt, &v.IsRequired,
		&v.ValueText, &v.ValueJSON, &v.ValueRef, &v.Preview, &v.TokenEstimate,
		&v.Source, &v.SourceRef, &v.TTLSeconds, &v.ExpiresAt,
		&v.Revision, &v.LastReadAt, &v.ReadCount,
		&metaJSON, &v.CreatedAt, &v.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return v, domain.ErrNotFound
		}
		return v, err
	}
	if metaJSON != "" && metaJSON != "{}" {
		_ = json.Unmarshal([]byte(metaJSON), &v.Metadata)
	}
	return v, nil
}

func l1SchemaSelectSQL() string {
	return `SELECT id, scope_type, scope_id, schema_key, schema_version, schema_json,
		description, enabled, metadata_json, created_at, updated_at
	 FROM memory_l1_schemas`
}

func scanL1Schema(row scanner) (domain.MemoryL1Schema, error) {
	var v domain.MemoryL1Schema
	var metaJSON string
	if err := row.Scan(
		&v.ID, &v.ScopeType, &v.ScopeID, &v.SchemaKey, &v.SchemaVersion, &v.SchemaJSON,
		&v.Description, &v.Enabled, &metaJSON, &v.CreatedAt, &v.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return v, domain.ErrNotFound
		}
		return v, err
	}
	if metaJSON != "" && metaJSON != "{}" {
		_ = json.Unmarshal([]byte(metaJSON), &v.Metadata)
	}
	return v, nil
}

func marshalShared(values []domain.L1FieldShare) (string, error) {
	if len(values) == 0 {
		return "[]", nil
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func marshalAnyMap(value map[string]any) (string, error) {
	if len(value) == 0 {
		return "{}", nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
