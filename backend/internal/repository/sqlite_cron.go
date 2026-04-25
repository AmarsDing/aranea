package repository

import (
	"database/sql"
	"strings"

	"arenea/backend/internal/domain"
)

// ListCronTaskRuns returns the recent execution history for cron tasks. The
// task name is enriched via a LEFT JOIN so deleted tasks still surface their
// last known label instead of an empty string.
func (r *SQLiteRepository) ListCronTaskRuns(query domain.CronTaskRunQuery) ([]domain.CronTaskRun, error) {
	conditions := []string{"1=1"}
	args := []any{}
	if id := strings.TrimSpace(query.TaskID); id != "" {
		conditions = append(conditions, "ctr.task_id = ?")
		args = append(args, id)
	}
	if status := strings.TrimSpace(query.Status); status != "" {
		conditions = append(conditions, "ctr.status = ?")
		args = append(args, status)
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	sqlText := `SELECT ctr.id, ctr.task_id, COALESCE(ct.name, ''), ctr.status,
		   ctr.started_at, ctr.finished_at, ctr.output_json, ctr.error_message, ctr.created_at
		FROM cron_task_run ctr
		LEFT JOIN cron_task ct ON ct.id = ctr.task_id
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY ctr.created_at DESC
		LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []domain.CronTaskRun{}
	for rows.Next() {
		run, scanErr := scanCronTaskRun(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, run)
	}
	return items, rows.Err()
}

func scanCronTaskRun(s scanner) (domain.CronTaskRun, error) {
	var run domain.CronTaskRun
	var started, finished sql.NullString
	if err := s.Scan(
		&run.ID, &run.TaskID, &run.TaskName, &run.Status,
		&started, &finished, &run.OutputJSON, &run.ErrorMessage, &run.CreatedAt,
	); err != nil {
		return domain.CronTaskRun{}, err
	}
	run.StartedAt = started.String
	run.FinishedAt = finished.String
	return run, nil
}
