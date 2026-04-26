package repository

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/0001_init.sql
var migrations embed.FS

// SQLiteRepository is the single concrete implementation of Store backed by an
// embedded SQLite database. The implementation is deliberately split into many
// per-aggregate files (sqlite_agents.go, sqlite_sessions.go, ...) so each file
// stays focused and small. Cross-cutting helpers live in sqlite_helpers.go and
// the migration / seeding logic is anchored here.
type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create db directory failed: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	return &SQLiteRepository{db: db}, nil
}

func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

// Migrate runs the embedded schema in two passes: tables-only first so the
// legacy column upgrade can ALTER TABLE, then the full schema so indexes are
// created last. After the schema is up to date all bootstrap seed data is
// installed.
func (r *SQLiteRepository) Migrate() error {
	schema, err := migrations.ReadFile("migrations/0001_init.sql")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	schemaText := string(schema)
	tableSchema := schemaText
	if idx := strings.Index(schemaText, "CREATE INDEX"); idx >= 0 {
		tableSchema = schemaText[:idx]
	}
	if _, err = r.db.Exec(tableSchema); err != nil {
		return err
	}
	if err = r.ensureLegacyColumns(); err != nil {
		return err
	}
	if _, err = r.db.Exec(schemaText); err != nil {
		return err
	}
	if err = r.seedChatOptions(); err != nil {
		return err
	}
	if err = r.seedPlatformDefaults(); err != nil {
		return err
	}
	if err = r.seedBuiltinTools(); err != nil {
		return err
	}
	if err = r.seedSystemAdminAgent(); err != nil {
		return err
	}
	return r.seedAvatarAssets()
}

// ensureLegacyColumns adds columns to existing tables for installations that
// were created before later columns existed. Each entry is keyed by table name
// and lists the column name with its SQLite DDL fragment used by ADD COLUMN.
func (r *SQLiteRepository) ensureLegacyColumns() error {
	columns := map[string]map[string]string{
		"agents": {
			"is_default":           "INTEGER NOT NULL DEFAULT 0",
			"is_favorite":          "INTEGER NOT NULL DEFAULT 0",
			"icon":                 "TEXT NOT NULL DEFAULT ''",
			"agent_description":    "TEXT NOT NULL DEFAULT ''",
			"category_position_id": "TEXT NOT NULL DEFAULT ''",
			"system_prompt_mode":   "TEXT NOT NULL DEFAULT ''",
			"context_window":       "INTEGER NOT NULL DEFAULT 0",
			"budget_monthly_cents": "INTEGER NOT NULL DEFAULT 0",
			"config_json":          "TEXT NOT NULL DEFAULT ''",
			"deleted_at":           "TEXT NOT NULL DEFAULT ''",
		},
		"sessions": {
			"owner_type":                 "TEXT NOT NULL DEFAULT 'agent'",
			"team_id":                    "TEXT NOT NULL DEFAULT ''",
			"summary":                    "TEXT NOT NULL DEFAULT ''",
			"context_used_ratio":         "REAL NOT NULL DEFAULT 0",
			"context_used_tokens":        "INTEGER NOT NULL DEFAULT 0",
			"max_context_used_ratio":     "REAL NOT NULL DEFAULT 0",
			"last_context_window_tokens": "INTEGER NOT NULL DEFAULT 0",
			"context_status":             "TEXT NOT NULL DEFAULT 'normal'",
			"dialog_mode":            "TEXT NOT NULL DEFAULT ''",
			"provider":               "TEXT NOT NULL DEFAULT ''",
			"model":                  "TEXT NOT NULL DEFAULT ''",
			"status":                 "TEXT NOT NULL DEFAULT 'active'",
			"message_count":          "INTEGER NOT NULL DEFAULT 0",
			"run_count":              "INTEGER NOT NULL DEFAULT 0",
			"model_call_count":       "INTEGER NOT NULL DEFAULT 0",
			"tool_call_count":        "INTEGER NOT NULL DEFAULT 0",
			"skill_call_count":       "INTEGER NOT NULL DEFAULT 0",
			"mcp_call_count":         "INTEGER NOT NULL DEFAULT 0",
			"input_tokens":           "INTEGER NOT NULL DEFAULT 0",
			"output_tokens":          "INTEGER NOT NULL DEFAULT 0",
			"total_tokens":           "INTEGER NOT NULL DEFAULT 0",
			"total_cost_micro_usd":   "INTEGER NOT NULL DEFAULT 0",
			"last_message_at":        "TEXT NOT NULL DEFAULT ''",
			"archived_at":            "TEXT NOT NULL DEFAULT ''",
			"deleted_at":             "TEXT NOT NULL DEFAULT ''",
		},
		"messages": {
			"parent_message_id": "TEXT NOT NULL DEFAULT ''",
			"turn_index":        "INTEGER NOT NULL DEFAULT 0",
			"attachments_count": "INTEGER NOT NULL DEFAULT 0",
			"options_json":      "TEXT NOT NULL DEFAULT ''",
			"error_message":     "TEXT NOT NULL DEFAULT ''",
		},
		"team_runs": {
			"message_id":     "TEXT NOT NULL DEFAULT ''",
			"cost_micro_usd": "INTEGER NOT NULL DEFAULT 0",
			"topology_json":  "TEXT NOT NULL DEFAULT '{}'",
		},
		"team_run_steps": {
			"agent_name":     "TEXT NOT NULL DEFAULT ''",
			"cost_micro_usd": "INTEGER NOT NULL DEFAULT 0",
		},
		"avatar_assets": {
			"image_data":      "BLOB NOT NULL DEFAULT X''",
			"thumbnail_data":  "BLOB",
			"mime_type":       "TEXT NOT NULL DEFAULT 'image/png'",
			"workspace_id":    "TEXT NOT NULL DEFAULT ''",
			"owner_user_id":   "TEXT NOT NULL DEFAULT ''",
			"source":          "TEXT NOT NULL DEFAULT 'system'",
			"is_system":       "INTEGER NOT NULL DEFAULT 0",
			"file_size_bytes": "INTEGER NOT NULL DEFAULT 0",
			"width_px":        "INTEGER NOT NULL DEFAULT 0",
			"height_px":       "INTEGER NOT NULL DEFAULT 0",
			"parent_id":       "TEXT NOT NULL DEFAULT ''",
			"level":           "TEXT NOT NULL DEFAULT ''",
			"agent_id":        "TEXT NOT NULL DEFAULT ''",
			"provider":        "TEXT NOT NULL DEFAULT ''",
			"model":           "TEXT NOT NULL DEFAULT ''",
		},
		"agent_category_nodes": {
			"workspace_id":  "TEXT NOT NULL DEFAULT ''",
			"owner_user_id": "TEXT NOT NULL DEFAULT ''",
			"is_system":     "INTEGER NOT NULL DEFAULT 0",
			"agent_id":      "TEXT NOT NULL DEFAULT ''",
			"provider":      "TEXT NOT NULL DEFAULT ''",
			"model":         "TEXT NOT NULL DEFAULT ''",
		},
		"llm_provider_models": {
			"parent_id": "TEXT NOT NULL DEFAULT ''",
			"level":     "TEXT NOT NULL DEFAULT ''",
			"agent_id":  "TEXT NOT NULL DEFAULT ''",
		},
		"hooks": {
			"parent_id": "TEXT NOT NULL DEFAULT ''",
			"level":     "TEXT NOT NULL DEFAULT ''",
			"agent_id":  "TEXT NOT NULL DEFAULT ''",
			"provider":  "TEXT NOT NULL DEFAULT ''",
			"model":     "TEXT NOT NULL DEFAULT ''",
		},
		"plugins": {
			"scope":                "TEXT NOT NULL DEFAULT 'global'",
			"callback_points_json": "TEXT NOT NULL DEFAULT '[]'",
			"config_schema_json":   "TEXT NOT NULL DEFAULT '{}'",
			"default_config_json":  "TEXT NOT NULL DEFAULT '{}'",
			"invoke_count":         "INTEGER NOT NULL DEFAULT 0",
			"block_count":          "INTEGER NOT NULL DEFAULT 0",
			"error_count":          "INTEGER NOT NULL DEFAULT 0",
			"last_invoked_at":      "TEXT NOT NULL DEFAULT ''",
			"last_status":          "TEXT NOT NULL DEFAULT ''",
		},
		"channel": {
			"parent_id": "TEXT NOT NULL DEFAULT ''",
			"level":     "TEXT NOT NULL DEFAULT ''",
			"agent_id":  "TEXT NOT NULL DEFAULT ''",
			"provider":  "TEXT NOT NULL DEFAULT ''",
			"model":     "TEXT NOT NULL DEFAULT ''",
		},
		"mcp_server": {
			"parent_id": "TEXT NOT NULL DEFAULT ''",
			"level":     "TEXT NOT NULL DEFAULT ''",
			"agent_id":  "TEXT NOT NULL DEFAULT ''",
			"provider":  "TEXT NOT NULL DEFAULT ''",
			"model":     "TEXT NOT NULL DEFAULT ''",
		},
		"skill": {
			"parent_id": "TEXT NOT NULL DEFAULT ''",
			"level":     "TEXT NOT NULL DEFAULT ''",
			"agent_id":  "TEXT NOT NULL DEFAULT ''",
			"provider":  "TEXT NOT NULL DEFAULT ''",
			"model":     "TEXT NOT NULL DEFAULT ''",
		},
		"skill_invocation": {
			"skill_version":  "TEXT NOT NULL DEFAULT ''",
			"user_id":        "TEXT NOT NULL DEFAULT ''",
			"session_id":     "TEXT NOT NULL DEFAULT ''",
			"duration_ms":    "INTEGER NOT NULL DEFAULT 0",
			"started_at":     "TEXT NOT NULL DEFAULT ''",
			"ended_at":       "TEXT NOT NULL DEFAULT ''",
			"input_preview":  "TEXT NOT NULL DEFAULT ''",
			"input_hash":     "TEXT NOT NULL DEFAULT ''",
			"output_preview": "TEXT NOT NULL DEFAULT ''",
			"error_code":     "TEXT NOT NULL DEFAULT ''",
		},
		"cron_task": {
			"parent_id": "TEXT NOT NULL DEFAULT ''",
			"level":     "TEXT NOT NULL DEFAULT ''",
			"provider":  "TEXT NOT NULL DEFAULT ''",
			"model":     "TEXT NOT NULL DEFAULT ''",
		},
		"monitor_events": {
			"enabled":     "INTEGER NOT NULL DEFAULT 1",
			"sort_order":  "INTEGER NOT NULL DEFAULT 0",
			"parent_id":   "TEXT NOT NULL DEFAULT ''",
			"level":       "TEXT NOT NULL DEFAULT ''",
			"agent_id":    "TEXT NOT NULL DEFAULT ''",
			"provider":    "TEXT NOT NULL DEFAULT ''",
			"model":       "TEXT NOT NULL DEFAULT ''",
			"config_json": "TEXT NOT NULL DEFAULT ''",
		},
		"monitor_traces": {
			"enabled":     "INTEGER NOT NULL DEFAULT 1",
			"sort_order":  "INTEGER NOT NULL DEFAULT 0",
			"parent_id":   "TEXT NOT NULL DEFAULT ''",
			"level":       "TEXT NOT NULL DEFAULT ''",
			"agent_id":    "TEXT NOT NULL DEFAULT ''",
			"provider":    "TEXT NOT NULL DEFAULT ''",
			"model":       "TEXT NOT NULL DEFAULT ''",
			"config_json": "TEXT NOT NULL DEFAULT ''",
		},
		"hook_agents": {
			"status":      "TEXT NOT NULL DEFAULT 'active'",
			"enabled":     "INTEGER NOT NULL DEFAULT 1",
			"config_json": "TEXT NOT NULL DEFAULT ''",
			"created_at":  "TEXT NOT NULL DEFAULT ''",
			"updated_at":  "TEXT NOT NULL DEFAULT ''",
			"deleted_at":  "TEXT NOT NULL DEFAULT ''",
		},
		"channel_credential": {
			"status":        "TEXT NOT NULL DEFAULT 'active'",
			"secret_ref":    "TEXT NOT NULL DEFAULT ''",
			"metadata_json": "TEXT NOT NULL DEFAULT ''",
			"created_at":    "TEXT NOT NULL DEFAULT ''",
			"updated_at":    "TEXT NOT NULL DEFAULT ''",
			"deleted_at":    "TEXT NOT NULL DEFAULT ''",
		},
		"agent_runtime_settings": {
			"agent_id":                              "TEXT PRIMARY KEY",
			"self_evolve":                           "INTEGER NOT NULL DEFAULT 1",
			"subagents_enabled":                     "INTEGER NOT NULL DEFAULT 1",
			"subagents_max_concurrency":             "INTEGER NOT NULL DEFAULT 20",
			"subagents_max_generation_depth":        "INTEGER NOT NULL DEFAULT 1",
			"subagents_max_children_per_agent":      "INTEGER NOT NULL DEFAULT 5",
			"subagents_archive_after_minutes":       "INTEGER NOT NULL DEFAULT 60",
			"subagents_max_retries":                 "INTEGER NOT NULL DEFAULT 2",
			"subagents_model_override":              "TEXT NOT NULL DEFAULT ''",
			"tools_enabled":                         "INTEGER NOT NULL DEFAULT 1",
			"tools_profile":                         "TEXT NOT NULL DEFAULT 'full'",
			"tools_tool_call_prefix":                "TEXT NOT NULL DEFAULT ''",
			"tools_allow_json":                      "TEXT NOT NULL DEFAULT '[]'",
			"tools_deny_json":                       "TEXT NOT NULL DEFAULT '[]'",
			"tools_concurrent_allow_json":           "TEXT NOT NULL DEFAULT '[]'",
			"memory_enabled":                        "INTEGER NOT NULL DEFAULT 1",
			"memory_max_chunk_length":               "INTEGER NOT NULL DEFAULT 1000",
			"memory_max_results":                    "INTEGER NOT NULL DEFAULT 6",
			"memory_min_score":                      "REAL NOT NULL DEFAULT 0.35",
			"heartbeat_enabled":                     "INTEGER NOT NULL DEFAULT 0",
			"heartbeat_interval_minutes":            "INTEGER NOT NULL DEFAULT 30",
			"evolution_self_evolve":                 "INTEGER NOT NULL DEFAULT 1",
			"evolution_skill_evolve":                "INTEGER NOT NULL DEFAULT 1",
			"evolution_metrics_enabled":             "INTEGER NOT NULL DEFAULT 1",
			"evolution_suggestions_enabled":         "INTEGER NOT NULL DEFAULT 1",
			"guardrail_max_change_per_period":       "REAL NOT NULL DEFAULT 0.1",
			"guardrail_min_data_points":             "INTEGER NOT NULL DEFAULT 100",
			"guardrail_rollback_on_decline_percent": "INTEGER NOT NULL DEFAULT 20",
			"l0_recent_window_turns":                "INTEGER NOT NULL DEFAULT 12",
			"l0_recent_window_tokens":               "INTEGER NOT NULL DEFAULT 0",
			"l0_summary_threshold":                  "REAL NOT NULL DEFAULT 0.6",
			"l0_summary_keep_turns":                 "INTEGER NOT NULL DEFAULT 4",
			"l0_truncate_strategy":                  "TEXT NOT NULL DEFAULT 'summary'",
			"l0_inject_l1":                          "INTEGER NOT NULL DEFAULT 1",
			"l0_inject_l3":                          "INTEGER NOT NULL DEFAULT 1",
			"l0_inject_l4":                          "INTEGER NOT NULL DEFAULT 0",
			"l0_l3_max_chunks":                      "INTEGER NOT NULL DEFAULT 5",
			"l0_l4_max_paths":                       "INTEGER NOT NULL DEFAULT 3",
			"l0_snapshot_mode":                      "TEXT NOT NULL DEFAULT 'on_warning'",
			"l1_enabled":                            "INTEGER NOT NULL DEFAULT 1",
			"l1_budget_tokens":                      "INTEGER NOT NULL DEFAULT 8192",
			"l1_field_max_tokens":                   "INTEGER NOT NULL DEFAULT 2048",
			"l1_history_keep_revisions":             "INTEGER NOT NULL DEFAULT 10",
			"l1_default_schema_id":                  "TEXT NOT NULL DEFAULT ''",
			"l1_archive_on_idle_minutes":            "INTEGER NOT NULL DEFAULT 60",
			"l2_episode_enabled":                    "INTEGER NOT NULL DEFAULT 1",
			"l2_episode_min_importance":             "REAL NOT NULL DEFAULT 0.3",
			"l2_index_enabled":                      "INTEGER NOT NULL DEFAULT 1",
			"l2_index_embedding_model":              "TEXT NOT NULL DEFAULT ''",
			"l2_recall_enabled":                     "INTEGER NOT NULL DEFAULT 0",
			"l2_recall_max":                         "INTEGER NOT NULL DEFAULT 3",
			"l2_retention_days":                     "INTEGER NOT NULL DEFAULT 90",
			"l2_archive_after_days":                 "INTEGER NOT NULL DEFAULT 30",
			"l3_enabled":                            "INTEGER NOT NULL DEFAULT 1",
			"l3_recall_top_k":                       "INTEGER NOT NULL DEFAULT 5",
			"l3_recall_min_score":                   "REAL NOT NULL DEFAULT 0.55",
			"l3_recall_scopes_json":                 "TEXT NOT NULL DEFAULT '[\"agent\",\"user\",\"team\",\"workspace\"]'",
			"l3_embedding_model":                    "TEXT NOT NULL DEFAULT ''",
			"l3_decay_interval_hours":               "INTEGER NOT NULL DEFAULT 24",
			"l3_archive_threshold":                  "REAL NOT NULL DEFAULT 0.2",
			"l3_max_per_recall_chars":               "INTEGER NOT NULL DEFAULT 1500",
			"l4_enabled":                            "INTEGER NOT NULL DEFAULT 1",
			"l4_graph_inject_neighbors":             "INTEGER NOT NULL DEFAULT 1",
			"l4_graph_max_neighbors":                "INTEGER NOT NULL DEFAULT 6",
			"l4_graph_max_hops":                     "INTEGER NOT NULL DEFAULT 2",
			"l4_identity_inject":                    "INTEGER NOT NULL DEFAULT 1",
			"l4_strategy_inject":                    "INTEGER NOT NULL DEFAULT 0",
			"evo_enabled":                           "INTEGER NOT NULL DEFAULT 0",
			"evo_auto_apply":                        "INTEGER NOT NULL DEFAULT 0",
			"evo_min_episodes":                      "INTEGER NOT NULL DEFAULT 20",
			"evo_min_negative_feedback":             "INTEGER NOT NULL DEFAULT 3",
			"evo_throttle_hours":                    "INTEGER NOT NULL DEFAULT 24",
			"evo_proposal_ttl_days":                 "INTEGER NOT NULL DEFAULT 14",
			"evo_persona_max_chars":                 "INTEGER NOT NULL DEFAULT 1500",
			"evo_system_prompt_max_appends":         "INTEGER NOT NULL DEFAULT 5",
			"created_at":                            "TEXT NOT NULL DEFAULT ''",
			"updated_at":                            "TEXT NOT NULL DEFAULT ''",
		},
		"memory_items": {
			"scope_subtype": "TEXT NOT NULL DEFAULT ''",
			"fact_id":       "TEXT NOT NULL DEFAULT ''",
		},
		"agent_prompt_files": {
			"id":         "TEXT PRIMARY KEY",
			"agent_id":   "TEXT NOT NULL DEFAULT ''",
			"file_name":  "TEXT NOT NULL DEFAULT ''",
			"body":       "TEXT NOT NULL DEFAULT ''",
			"sort_order": "INTEGER NOT NULL DEFAULT 0",
			"created_at": "TEXT NOT NULL DEFAULT ''",
			"updated_at": "TEXT NOT NULL DEFAULT ''",
		},
	}
	commonPlatformColumns := map[string]string{
		"description":   "TEXT NOT NULL DEFAULT ''",
		"status":        "TEXT NOT NULL DEFAULT 'active'",
		"enabled":       "INTEGER NOT NULL DEFAULT 1",
		"sort_order":    "INTEGER NOT NULL DEFAULT 0",
		"parent_id":     "TEXT NOT NULL DEFAULT ''",
		"level":         "TEXT NOT NULL DEFAULT ''",
		"agent_id":      "TEXT NOT NULL DEFAULT ''",
		"provider":      "TEXT NOT NULL DEFAULT ''",
		"model":         "TEXT NOT NULL DEFAULT ''",
		"config_json":   "TEXT NOT NULL DEFAULT ''",
		"metadata_json": "TEXT NOT NULL DEFAULT ''",
		"created_at":    "TEXT NOT NULL DEFAULT ''",
		"updated_at":    "TEXT NOT NULL DEFAULT ''",
		"deleted_at":    "TEXT NOT NULL DEFAULT ''",
	}
	for _, table := range platformTables {
		if _, ok := columns[table.name]; !ok {
			columns[table.name] = map[string]string{}
		}
		for name, ddl := range commonPlatformColumns {
			if _, exists := columns[table.name][name]; !exists {
				columns[table.name][name] = ddl
			}
		}
	}
	for table, cols := range columns {
		existing, err := r.tableColumns(table)
		if err != nil {
			return err
		}
		for name, ddl := range cols {
			if existing[name] {
				continue
			}
			if _, err = r.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, name, ddl)); err != nil {
				return fmt.Errorf("add column %s.%s: %w", table, name, err)
			}
		}
	}
	return nil
}

func (r *SQLiteRepository) tableColumns(table string) (map[string]bool, error) {
	rows, err := r.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err = rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		result[name] = true
	}
	return result, rows.Err()
}
