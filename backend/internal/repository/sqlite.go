package repository

import (
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"arenea/backend/internal/domain"
	_ "modernc.org/sqlite"
)

//go:embed migrations/0001_init.sql
var migrations embed.FS

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
	return r.seedAvatarAssets()
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

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
			"owner_type":             "TEXT NOT NULL DEFAULT 'agent'",
			"team_id":                "TEXT NOT NULL DEFAULT ''",
			"summary":                "TEXT NOT NULL DEFAULT ''",
			"context_used_ratio":     "REAL NOT NULL DEFAULT 0",
			"max_context_used_ratio": "REAL NOT NULL DEFAULT 0",
			"context_status":         "TEXT NOT NULL DEFAULT 'normal'",
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
			"message_id":    "TEXT NOT NULL DEFAULT ''",
			"topology_json": "TEXT NOT NULL DEFAULT '{}'",
		},
		"team_run_steps": {
			"agent_name": "TEXT NOT NULL DEFAULT ''",
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
			"created_at":                            "TEXT NOT NULL DEFAULT ''",
			"updated_at":                            "TEXT NOT NULL DEFAULT ''",
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

func (r *SQLiteRepository) seedChatOptions() error {
	rows := []domain.ChatOption{
		{Type: "dialog_mode", Key: "default", Label: "标准对话", Enabled: true, SortOrder: 10},
		{Type: "dialog_mode", Key: "plan", Label: "深思考", Enabled: true, SortOrder: 20},
		{Type: "dialog_mode", Key: "code", Label: "仅代码", Enabled: true, SortOrder: 30},
		{Type: "model_provider", Key: "openai", Label: "OpenAI 兼容", Enabled: true, SortOrder: 10},
		{Type: "model_provider", Key: "anthropic", Label: "Anthropic", Enabled: true, SortOrder: 20},
		{Type: "model_provider", Key: "self", Label: "自托管", Enabled: true, SortOrder: 30},
	}
	for _, row := range rows {
		_, err := r.db.Exec(
			`INSERT OR IGNORE INTO chat_options(type, key, label, enabled, sort_order, metadata_json) VALUES (?, ?, ?, ?, ?, ?)`,
			row.Type, row.Key, row.Label, row.Enabled, row.SortOrder, row.MetadataJSON,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLiteRepository) seedPlatformDefaults() error {
	defaults := []domain.PlatformResource{
		{ID: "model_openrouter_gpt41mini", Resource: "llm-provider-models", Key: "openrouter:gpt-4.1-mini", Name: "GPT 4.1 Mini", Provider: "openrouter", Model: "gpt-4.1-mini", Description: "默认 OpenRouter 兼容模型", Enabled: true, SortOrder: 10},
		{ID: "model_anthropic_sonnet", Resource: "llm-provider-models", Key: "anthropic:claude-sonnet", Name: "Claude Sonnet", Provider: "anthropic", Model: "claude-sonnet", Description: "Anthropic 兼容模型", Enabled: true, SortOrder: 20},
	}
	defaults = append(defaults, agentCategorySeeds...)
	for _, row := range defaults {
		if _, err := r.CreatePlatformResource(row); err != nil && !strings.Contains(err.Error(), "constraint failed") {
			return err
		}
	}
	return nil
}

func (r *SQLiteRepository) seedBuiltinTools() error {
	now := nowISO()
	for _, row := range builtinToolSeeds {
		if row.ID == "" {
			row.ID = "tool_" + strings.ReplaceAll(row.Key, "-", "_")
		}
		if row.Source == "" {
			row.Source = "builtin"
		}
		if row.RiskLevel == "" {
			row.RiskLevel = "low"
		}
		if row.ParametersSchemaJSON == "" {
			row.ParametersSchemaJSON = "{}"
		}
		if row.ResultSchemaJSON == "" {
			row.ResultSchemaJSON = "{}"
		}
		if row.ConfigSchemaJSON == "" {
			row.ConfigSchemaJSON = "{}"
		}
		if row.ConfigJSON == "" {
			row.ConfigJSON = "{}"
		}
		if row.DefaultConfigJSON == "" {
			row.DefaultConfigJSON = row.ConfigJSON
		}
		if row.MetadataJSON == "" {
			row.MetadataJSON = "{}"
		}
		_, err := r.db.Exec(
			`INSERT INTO tools(
			 id, tool_key, display_name, description, category, source, risk_level, enabled, readonly, requires_confirmation,
			 supports_streaming, supports_concurrency, parameters_schema_json, result_schema_json, config_schema_json, config_json,
			 default_config_json, metadata_json, created_at, updated_at, deleted_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '')
			ON CONFLICT(tool_key) DO UPDATE SET
			 display_name = excluded.display_name,
			 description = excluded.description,
			 category = excluded.category,
			 source = excluded.source,
			 risk_level = excluded.risk_level,
			 readonly = excluded.readonly,
			 requires_confirmation = excluded.requires_confirmation,
			 supports_streaming = excluded.supports_streaming,
			 supports_concurrency = excluded.supports_concurrency,
			 parameters_schema_json = excluded.parameters_schema_json,
			 result_schema_json = excluded.result_schema_json,
			 config_schema_json = excluded.config_schema_json,
			 default_config_json = excluded.default_config_json,
			 metadata_json = excluded.metadata_json,
			 updated_at = excluded.updated_at,
			 deleted_at = ''`,
			row.ID, row.Key, row.DisplayName, row.Description, row.Category, row.Source, row.RiskLevel, row.Enabled, row.Readonly, row.RequiresConfirmation,
			row.SupportsStreaming, row.SupportsConcurrency, row.ParametersSchemaJSON, row.ResultSchemaJSON, row.ConfigSchemaJSON, row.ConfigJSON,
			row.DefaultConfigJSON, row.MetadataJSON, now, now,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

var builtinToolSeeds = []domain.Tool{
	{ID: "tool_datetime", Key: "datetime", DisplayName: "当前时间", Description: "返回当前时间、日期和时区信息。", Category: "system", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{}}`},
	{ID: "tool_web_search", Key: "web_search", DisplayName: "Web 搜索", Description: "搜索实时网络信息，返回标题、链接和摘要。", Category: "web", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"},"limit":{"type":"number","description":"返回结果数量"}},"required":["query"]}`},
	{ID: "tool_web_fetch", Key: "web_fetch", DisplayName: "Web 抓取", Description: "抓取 URL 并提取页面文本或 Markdown。", Category: "web", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"url":{"type":"string"},"extract_mode":{"type":"string","enum":["markdown","text","json"]}},"required":["url"]}`},
	{ID: "tool_read_file", Key: "read_file", DisplayName: "读取文件", Description: "读取工作区允许路径内的文件内容。", Category: "filesystem", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`},
	{ID: "tool_write_file", Key: "write_file", DisplayName: "写入文件", Description: "创建或覆盖工作区文件。", Category: "filesystem", RiskLevel: "medium", Enabled: true, ParametersSchemaJSON: `{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"},"deliver":{"type":"boolean"}},"required":["path","content"]}`},
	{ID: "tool_list_files", Key: "list_files", DisplayName: "文件列表", Description: "列出工作区目录内容。", Category: "filesystem", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"path":{"type":"string"}}}`},
	{ID: "tool_edit_file", Key: "edit_file", DisplayName: "编辑文件", Description: "按精确匹配修改已有文件片段。", Category: "filesystem", RiskLevel: "medium", Enabled: true, ParametersSchemaJSON: `{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"}},"required":["path","old_string","new_string"]}`},
	{ID: "tool_skill_search", Key: "skill_search", DisplayName: "Skill 搜索", Description: "搜索当前系统可用 Skill。", Category: "skill", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`},
	{ID: "tool_use_skill", Key: "use_skill", DisplayName: "使用 Skill", Description: "标记本次运行使用某个 Skill，用于追踪。", Category: "skill", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`},
	{ID: "tool_memory_search", Key: "memory_search", DisplayName: "Memory 搜索", Description: "搜索 Agent 长期记忆。", Category: "memory", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`},
	{ID: "tool_memory_get", Key: "memory_get", DisplayName: "Memory 读取", Description: "读取指定 memory 内容。", Category: "memory", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`},
	{ID: "tool_read_image", Key: "read_image", DisplayName: "图片理解", Description: "分析图片内容。", Category: "media", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`},
	{ID: "tool_read_document", Key: "read_document", DisplayName: "文档理解", Description: "分析 PDF、Office、CSV 等文档。", Category: "media", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`},
	{ID: "tool_create_image", Key: "create_image", DisplayName: "图片生成", Description: "根据文本提示生成图片。", Category: "media", RiskLevel: "medium", Enabled: false, ParametersSchemaJSON: `{"type":"object","properties":{"prompt":{"type":"string"},"size":{"type":"string"}},"required":["prompt"]}`},
	{ID: "tool_tts", Key: "tts", DisplayName: "文本转语音", Description: "将文本转换成语音文件。", Category: "media", RiskLevel: "medium", Enabled: false, ParametersSchemaJSON: `{"type":"object","properties":{"text":{"type":"string"},"voice":{"type":"string"}},"required":["text"]}`},
	{ID: "tool_shell_exec", Key: "shell_exec", DisplayName: "Shell 命令", Description: "执行本地 shell 命令。", Category: "runtime", RiskLevel: "critical", Enabled: false, RequiresConfirmation: true, ParametersSchemaJSON: `{"type":"object","properties":{"command":{"type":"string"},"working_dir":{"type":"string"}},"required":["command"]}`},
}

type platformTable struct {
	name      string
	keyColumn string
}

var platformTables = map[string]platformTable{
	"avatar-assets":       {name: "avatar_assets", keyColumn: "asset_key"},
	"agent-categories":    {name: "agent_category_nodes", keyColumn: "category_key"},
	"llm-provider-models": {name: "llm_provider_models", keyColumn: "model_key"},
	"hooks":               {name: "hooks", keyColumn: "hook_key"},
	"channels":            {name: "channel", keyColumn: "channel_key"},
	"mcp-servers":         {name: "mcp_server", keyColumn: "server_key"},
	"skills":              {name: "skill", keyColumn: "skill_key"},
	"cron-tasks":          {name: "cron_task", keyColumn: "task_key"},
	"monitor-events":      {name: "monitor_events", keyColumn: "event_key"},
	"monitor-traces":      {name: "monitor_traces", keyColumn: "trace_key"},
}

func platformTableFor(resource string) (platformTable, error) {
	table, ok := platformTables[resource]
	if !ok {
		return platformTable{}, fmt.Errorf("unsupported resource: %s", resource)
	}
	return table, nil
}

var agentCategorySeeds = []domain.PlatformResource{
	{ID: "cat_it", Resource: "agent-categories", Key: "it-industry", Name: "IT行业", Description: "系统预置行业：研发、平台、游戏与 AI 工程", Enabled: true, SortOrder: 10, Level: "industry", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_it_game", Resource: "agent-categories", Key: "it-game-dev", Name: "游戏开发部", Description: "游戏研发、引擎、场景与玩法", Enabled: true, SortOrder: 10, ParentID: "cat_it", Level: "department", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_it_game_ue5", Resource: "agent-categories", Key: "it-game-ue5-scene-designer", Name: "UE5场景设计师", Description: "负责 UE5 场景、光照、材质与关卡协作", Enabled: true, SortOrder: 10, ParentID: "cat_it_game", Level: "position", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_it_system", Resource: "agent-categories", Key: "it-system-dev", Name: "系统开发部", Description: "后端、平台工程、工具链与基础设施", Enabled: true, SortOrder: 20, ParentID: "cat_it", Level: "department", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_it_system_go", Resource: "agent-categories", Key: "it-system-golang-senior", Name: "golang后端高级工程师", Description: "负责 Go 服务、接口、数据库与可靠性", Enabled: true, SortOrder: 10, ParentID: "cat_it_system", Level: "position", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_ai", Resource: "agent-categories", Key: "ai-industry", Name: "AI行业", Description: "系统预置行业：智能体、模型应用与数据工程", Enabled: true, SortOrder: 20, Level: "industry", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_ai_agent", Resource: "agent-categories", Key: "ai-agent-platform", Name: "Agent平台部", Description: "Agent 编排、工具、记忆与工作流", Enabled: true, SortOrder: 10, ParentID: "cat_ai", Level: "department", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_ai_agent_architect", Resource: "agent-categories", Key: "ai-agent-architect", Name: "Agent架构师", Description: "设计 Agent 能力、提示词、工具策略与运行闭环", Enabled: true, SortOrder: 10, ParentID: "cat_ai_agent", Level: "position", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_design", Resource: "agent-categories", Key: "design-industry", Name: "创意设计行业", Description: "系统预置行业：品牌、界面、内容与视觉生产", Enabled: true, SortOrder: 30, Level: "industry", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_design_ui", Resource: "agent-categories", Key: "design-uiux", Name: "UIUX设计部", Description: "用户体验、界面系统与视觉规范", Enabled: true, SortOrder: 10, ParentID: "cat_design", Level: "department", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_design_ui_senior", Resource: "agent-categories", Key: "design-uiux-senior", Name: "高级UIUX设计师", Description: "负责设计系统、交互流程与高保真界面", Enabled: true, SortOrder: 10, ParentID: "cat_design_ui", Level: "position", MetadataJSON: `{"is_system":true}`},
}

func (r *SQLiteRepository) ListPlatformResources(resource string) ([]domain.PlatformResource, error) {
	table, err := platformTableFor(resource)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`SELECT id, %s, name, description, status, enabled, sort_order, %s, %s, %s, %s, %s, config_json, metadata_json, created_at, updated_at, deleted_at FROM %s WHERE deleted_at = '' ORDER BY sort_order ASC, created_at DESC`,
		table.keyColumn, optionalColumn(table.name, "parent_id"), optionalColumn(table.name, "level"), optionalColumn(table.name, "agent_id"), optionalColumn(table.name, "provider"), optionalColumn(table.name, "model"), table.name)
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPlatformRows(resource, rows)
}

func (r *SQLiteRepository) GetPlatformResource(resource string, id string) (domain.PlatformResource, error) {
	table, err := platformTableFor(resource)
	if err != nil {
		return domain.PlatformResource{}, err
	}
	query := fmt.Sprintf(`SELECT id, %s, name, description, status, enabled, sort_order, %s, %s, %s, %s, %s, config_json, metadata_json, created_at, updated_at, deleted_at FROM %s WHERE id = ? AND deleted_at = ''`,
		table.keyColumn, optionalColumn(table.name, "parent_id"), optionalColumn(table.name, "level"), optionalColumn(table.name, "agent_id"), optionalColumn(table.name, "provider"), optionalColumn(table.name, "model"), table.name)
	row := r.db.QueryRow(query, id)
	return scanPlatformResource(resource, row)
}

func (r *SQLiteRepository) CreatePlatformResource(v domain.PlatformResource) (domain.PlatformResource, error) {
	table, err := platformTableFor(v.Resource)
	if err != nil {
		return domain.PlatformResource{}, err
	}
	if v.ID == "" || v.Key == "" || v.Name == "" {
		return domain.PlatformResource{}, errors.New("id, key and name are required")
	}
	now := nowISO()
	if v.CreatedAt == "" {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	if v.Status == "" {
		v.Status = "active"
	}
	query := fmt.Sprintf(`INSERT INTO %s(id, %s, name, description, status, enabled, sort_order, %s, %s, %s, %s, %s, config_json, metadata_json, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		table.name, table.keyColumn, optionalColumn(table.name, "parent_id"), optionalColumn(table.name, "level"), optionalColumn(table.name, "agent_id"), optionalColumn(table.name, "provider"), optionalColumn(table.name, "model"))
	_, err = r.db.Exec(query, v.ID, v.Key, v.Name, v.Description, v.Status, v.Enabled, v.SortOrder, v.ParentID, v.Level, v.AgentID, v.Provider, v.Model, v.ConfigJSON, v.MetadataJSON, v.CreatedAt, v.UpdatedAt, v.DeletedAt)
	return v, err
}

func (r *SQLiteRepository) UpdatePlatformResource(v domain.PlatformResource) (domain.PlatformResource, error) {
	table, err := platformTableFor(v.Resource)
	if err != nil {
		return domain.PlatformResource{}, err
	}
	current, err := r.GetPlatformResource(v.Resource, v.ID)
	if err != nil {
		return domain.PlatformResource{}, err
	}
	if v.Key == "" {
		v.Key = current.Key
	}
	if v.Name == "" {
		v.Name = current.Name
	}
	if v.Status == "" {
		v.Status = current.Status
	}
	v.CreatedAt = current.CreatedAt
	v.UpdatedAt = nowISO()
	query := fmt.Sprintf(`UPDATE %s SET %s = ?, name = ?, description = ?, status = ?, enabled = ?, sort_order = ?, %s = ?, %s = ?, %s = ?, %s = ?, %s = ?, config_json = ?, metadata_json = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`,
		table.name, table.keyColumn, optionalColumn(table.name, "parent_id"), optionalColumn(table.name, "level"), optionalColumn(table.name, "agent_id"), optionalColumn(table.name, "provider"), optionalColumn(table.name, "model"))
	_, err = r.db.Exec(query, v.Key, v.Name, v.Description, v.Status, v.Enabled, v.SortOrder, v.ParentID, v.Level, v.AgentID, v.Provider, v.Model, v.ConfigJSON, v.MetadataJSON, v.UpdatedAt, v.ID)
	return v, err
}

func (r *SQLiteRepository) DeletePlatformResource(resource string, id string) error {
	table, err := platformTableFor(resource)
	if err != nil {
		return err
	}
	if resource == "agent-categories" {
		if err = r.ensureCategoryCanDelete(id); err != nil {
			return err
		}
	}
	_, err = r.db.Exec(fmt.Sprintf(`UPDATE %s SET deleted_at = ?, status = 'deleted', updated_at = ? WHERE id = ? AND deleted_at = ''`, table.name), nowISO(), nowISO(), id)
	return err
}

func (r *SQLiteRepository) SearchPlugins(query domain.PluginListQuery) (domain.PluginListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	where := []string{"deleted_at = ''"}
	args := []any{}
	if search := strings.TrimSpace(query.Search); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = append(where, "(LOWER(plugin_key) LIKE ? OR LOWER(name) LIKE ? OR LOWER(description) LIKE ?)")
		args = append(args, like, like, like)
	}
	if query.Category != "" {
		where = append(where, "category = ?")
		args = append(args, query.Category)
	}
	if query.Enabled == "true" {
		where = append(where, "enabled = 1")
	}
	if query.Enabled == "false" {
		where = append(where, "enabled = 0")
	}
	if query.CallbackPoint != "" {
		where = append(where, "callback_points_json LIKE ?")
		args = append(args, "%"+query.CallbackPoint+"%")
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM plugins WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return domain.PluginListResult{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.Limit, query.Offset)
	rows, err := r.db.Query(pluginSelectSQL()+` WHERE `+whereSQL+` ORDER BY sort_order ASC, created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return domain.PluginListResult{}, err
	}
	defer rows.Close()
	items, err := scanPlugins(rows)
	if err != nil {
		return domain.PluginListResult{}, err
	}
	return domain.PluginListResult{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

func (r *SQLiteRepository) UpsertPlugin(plugin domain.Plugin) (domain.Plugin, error) {
	now := nowISO()
	if plugin.ID == "" {
		plugin.ID = "plugin_" + plugin.Key
	}
	if plugin.Scope == "" {
		plugin.Scope = "global"
	}
	if plugin.CreatedAt == "" {
		plugin.CreatedAt = now
	}
	plugin.UpdatedAt = now
	callbacks, _ := json.Marshal(plugin.CallbackPoints)
	_, err := r.db.Exec(`
		INSERT INTO plugins(id, plugin_key, name, description, category, risk_level, status, enabled, scope, callback_points_json, sort_order, config_schema_json, config_json, default_config_json, created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, 'active', ?, ?, ?, ?, ?, ?, ?, ?, ?, '')
		ON CONFLICT(plugin_key) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			category = excluded.category,
			risk_level = excluded.risk_level,
			scope = excluded.scope,
			callback_points_json = excluded.callback_points_json,
			sort_order = excluded.sort_order,
			config_schema_json = excluded.config_schema_json,
			default_config_json = excluded.default_config_json,
			updated_at = excluded.updated_at,
			deleted_at = ''`,
		plugin.ID, plugin.Key, plugin.Name, plugin.Description, plugin.Category, plugin.RiskLevel, plugin.Enabled, plugin.Scope, string(callbacks), plugin.SortOrder, plugin.ConfigSchemaJSON, plugin.ConfigJSON, plugin.DefaultConfigJSON, plugin.CreatedAt, plugin.UpdatedAt,
	)
	if err != nil {
		return domain.Plugin{}, err
	}
	rows, err := r.db.Query(pluginSelectSQL()+` WHERE plugin_key = ? AND deleted_at = '' LIMIT 1`, plugin.Key)
	if err != nil {
		return domain.Plugin{}, err
	}
	defer rows.Close()
	items, err := scanPlugins(rows)
	if err != nil {
		return domain.Plugin{}, err
	}
	if len(items) == 0 {
		return domain.Plugin{}, sql.ErrNoRows
	}
	return items[0], nil
}

func (r *SQLiteRepository) UpdatePluginEnabled(id string, enabled bool) (domain.Plugin, error) {
	_, err := r.db.Exec(`UPDATE plugins SET enabled = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`, enabled, nowISO(), id)
	if err != nil {
		return domain.Plugin{}, err
	}
	return r.getPluginByID(id)
}

func (r *SQLiteRepository) UpdatePluginConfig(id string, configJSON string) (domain.Plugin, error) {
	if strings.TrimSpace(configJSON) == "" {
		configJSON = "{}"
	}
	if !json.Valid([]byte(configJSON)) {
		return domain.Plugin{}, errors.New("plugin config_json must be valid JSON")
	}
	_, err := r.db.Exec(`UPDATE plugins SET config_json = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`, configJSON, nowISO(), id)
	if err != nil {
		return domain.Plugin{}, err
	}
	return r.getPluginByID(id)
}

func (r *SQLiteRepository) ListEnabledPluginKeys() ([]string, error) {
	rows, err := r.db.Query(`SELECT plugin_key FROM plugins WHERE enabled = 1 AND deleted_at = '' ORDER BY sort_order ASC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (r *SQLiteRepository) getPluginByID(id string) (domain.Plugin, error) {
	rows, err := r.db.Query(pluginSelectSQL()+` WHERE id = ? AND deleted_at = '' LIMIT 1`, id)
	if err != nil {
		return domain.Plugin{}, err
	}
	defer rows.Close()
	items, err := scanPlugins(rows)
	if err != nil {
		return domain.Plugin{}, err
	}
	if len(items) == 0 {
		return domain.Plugin{}, sql.ErrNoRows
	}
	return items[0], nil
}

func (r *SQLiteRepository) SearchSkills(query domain.SkillListQuery) (domain.SkillListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}

	where, args := skillWhereClause(query)
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM skill s WHERE `+where, args...).Scan(&total); err != nil {
		return domain.SkillListResult{}, err
	}

	listArgs := append([]any{time.Now().UTC().AddDate(0, 0, -7).Format(time.RFC3339)}, args...)
	listArgs = append(listArgs, query.Limit, query.Offset)
	rows, err := r.db.Query(skillSelectSQL()+` WHERE `+where+` ORDER BY s.updated_at DESC, s.created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return domain.SkillListResult{}, err
	}
	defer rows.Close()

	items, err := scanSkills(rows)
	if err != nil {
		return domain.SkillListResult{}, err
	}
	return domain.SkillListResult{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

func (r *SQLiteRepository) GetSkillByID(id string) (domain.Skill, error) {
	rows, err := r.db.Query(skillSelectSQL()+` WHERE s.id = ? AND s.deleted_at = '' LIMIT 1`, time.Now().UTC().AddDate(0, 0, -7).Format(time.RFC3339), id)
	if err != nil {
		return domain.Skill{}, err
	}
	defer rows.Close()
	items, err := scanSkills(rows)
	if err != nil {
		return domain.Skill{}, err
	}
	if len(items) == 0 {
		return domain.Skill{}, sql.ErrNoRows
	}
	return items[0], nil
}

func (r *SQLiteRepository) UpdateSkillEnabled(id string, enabled bool) (domain.Skill, error) {
	if id == "" {
		return domain.Skill{}, errors.New("skill id is required")
	}
	if _, err := r.db.Exec(`UPDATE skill SET enabled = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`, enabled, nowISO(), id); err != nil {
		return domain.Skill{}, err
	}
	return r.GetSkillByID(id)
}

func (r *SQLiteRepository) DuplicateSkill(id string) (domain.Skill, error) {
	current, err := r.GetSkillByID(id)
	if err != nil {
		return domain.Skill{}, err
	}
	newID := fmt.Sprintf("skill_%d", time.Now().UTC().UnixNano())
	newKey := fmt.Sprintf("%s-copy-%d", current.Slug, time.Now().UTC().Unix())
	if strings.TrimSpace(current.Slug) == "" {
		newKey = newID
	}
	now := nowISO()
	var configJSON, metadataJSON string
	if err = r.db.QueryRow(`SELECT config_json, metadata_json FROM skill WHERE id = ? AND deleted_at = ''`, id).Scan(&configJSON, &metadataJSON); err != nil {
		return domain.Skill{}, err
	}
	_, err = r.db.Exec(
		`INSERT INTO skill(id, skill_key, name, description, status, enabled, sort_order, config_json, metadata_json, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, 'draft', 0, 0, ?, ?, ?, ?, '')`,
		newID, newKey, current.Name+" Copy", current.Description, configJSON, metadataJSON, now, now,
	)
	if err != nil {
		return domain.Skill{}, err
	}
	return r.GetSkillByID(newID)
}

func (r *SQLiteRepository) DeleteSkill(id string) error {
	return r.DeletePlatformResource("skills", id)
}

func (r *SQLiteRepository) SearchTools(query domain.ToolListQuery) (domain.ToolListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	where, args := toolWhereClause(query)
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM tools t WHERE `+where, args...).Scan(&total); err != nil {
		return domain.ToolListResult{}, err
	}
	listArgs := append([]any{time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)}, args...)
	listArgs = append(listArgs, query.Limit, query.Offset)
	rows, err := r.db.Query(toolSelectSQL()+` WHERE `+where+` ORDER BY t.category ASC, t.display_name ASC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return domain.ToolListResult{}, err
	}
	defer rows.Close()
	items, err := scanTools(rows)
	if err != nil {
		return domain.ToolListResult{}, err
	}
	summary, err := r.toolSummary(query)
	if err != nil {
		return domain.ToolListResult{}, err
	}
	return domain.ToolListResult{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset, Summary: summary}, nil
}

func (r *SQLiteRepository) GetToolByID(id string) (domain.Tool, error) {
	rows, err := r.db.Query(toolSelectSQL()+` WHERE (t.id = ? OR t.tool_key = ?) AND t.deleted_at = '' LIMIT 1`, time.Now().UTC().Add(-24*time.Hour).Format(time.RFC3339), id, id)
	if err != nil {
		return domain.Tool{}, err
	}
	defer rows.Close()
	items, err := scanTools(rows)
	if err != nil {
		return domain.Tool{}, err
	}
	if len(items) == 0 {
		return domain.Tool{}, sql.ErrNoRows
	}
	return items[0], nil
}

func (r *SQLiteRepository) UpdateToolEnabled(id string, enabled bool) (domain.Tool, error) {
	if strings.TrimSpace(id) == "" {
		return domain.Tool{}, errors.New("tool id is required")
	}
	result, err := r.db.Exec(`UPDATE tools SET enabled = ?, updated_at = ? WHERE (id = ? OR tool_key = ?) AND deleted_at = ''`, enabled, nowISO(), id, id)
	if err != nil {
		return domain.Tool{}, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return domain.Tool{}, sql.ErrNoRows
	}
	return r.GetToolByID(id)
}

func (r *SQLiteRepository) SearchToolInvocations(query domain.ToolRunQuery) (domain.ToolRunResult, error) {
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	where := []string{"1 = 1"}
	args := []any{}
	if query.ToolKey != "" {
		where = append(where, "ti.tool_key = ?")
		args = append(args, query.ToolKey)
	}
	if query.AgentID != "" {
		where = append(where, "ti.agent_id = ?")
		args = append(args, query.AgentID)
	}
	if query.SessionID != "" {
		where = append(where, "ti.session_id = ?")
		args = append(args, query.SessionID)
	}
	if query.Status != "" {
		where = append(where, "ti.status = ?")
		args = append(args, query.Status)
	}
	if query.From != "" {
		where = append(where, "ti.started_at >= ?")
		args = append(args, query.From)
	}
	if query.To != "" {
		where = append(where, "ti.started_at <= ?")
		args = append(args, query.To)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM tool_invocations ti WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return domain.ToolRunResult{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.Limit, query.Offset)
	rows, err := r.db.Query(`
		SELECT ti.id, ti.request_id, ti.invocation_id, ti.tool_id, ti.tool_key, COALESCE(t.display_name, ti.tool_key),
		       ti.agent_id, ti.agent_key, COALESCE(a.display_name, ''), ti.session_id, ti.message_id, ti.user_id,
		       ti.source, ti.status, ti.started_at, ti.ended_at, ti.duration_ms,
		       ti.input_preview, ti.input_hash, ti.output_preview, ti.output_hash,
		       ti.error_code, ti.error_message, ti.redaction_applied, ti.metadata_json, ti.created_at
		FROM tool_invocations ti
		LEFT JOIN tools t ON t.tool_key = ti.tool_key
		LEFT JOIN agents a ON a.id = ti.agent_id
		WHERE `+whereSQL+`
		ORDER BY ti.started_at DESC, ti.created_at DESC
		LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return domain.ToolRunResult{}, err
	}
	defer rows.Close()
	items := []domain.ToolInvocation{}
	for rows.Next() {
		var item domain.ToolInvocation
		if err = rows.Scan(
			&item.ID, &item.RequestID, &item.InvocationID, &item.ToolID, &item.ToolKey, &item.ToolDisplayName,
			&item.AgentID, &item.AgentKey, &item.AgentDisplayName, &item.SessionID, &item.MessageID, &item.UserID,
			&item.Source, &item.Status, &item.StartedAt, &item.EndedAt, &item.DurationMS,
			&item.InputPreview, &item.InputHash, &item.OutputPreview, &item.OutputHash,
			&item.ErrorCode, &item.ErrorMessage, &item.RedactionApplied, &item.MetadataJSON, &item.CreatedAt,
		); err != nil {
			return domain.ToolRunResult{}, err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return domain.ToolRunResult{}, err
	}
	return domain.ToolRunResult{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

func (r *SQLiteRepository) toolSummary(query domain.ToolListQuery) (domain.ToolSummary, error) {
	cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	var summary domain.ToolSummary
	where, args := toolWhereClause(domain.ToolListQuery{
		Search:    query.Search,
		Category:  query.Category,
		Source:    query.Source,
		RiskLevel: query.RiskLevel,
		Enabled:   query.Enabled,
	})
	if err := r.db.QueryRow(`
		SELECT
		  COALESCE(COUNT(1), 0),
		  COALESCE(SUM(CASE WHEN enabled = 1 THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN enabled = 1 AND risk_level IN ('high', 'critical') THEN 1 ELSE 0 END), 0)
		FROM tools t WHERE `+where,
		args...,
	).Scan(&summary.TotalTools, &summary.EnabledTools, &summary.HighRiskEnabled); err != nil {
		return domain.ToolSummary{}, err
	}
	var success24h, failed24h, blocked24h int
	if err := r.db.QueryRow(`
		SELECT
		  COALESCE(COUNT(1), 0),
		  COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END), 0)
		FROM tool_invocations WHERE started_at >= ?`,
		cutoff,
	).Scan(&summary.Calls24h, &success24h, &failed24h, &blocked24h); err != nil {
		return domain.ToolSummary{}, err
	}
	if summary.Calls24h > 0 {
		summary.FailureRate24h = float64(failed24h+blocked24h) / float64(summary.Calls24h)
	}
	_ = success24h
	return summary, nil
}

func toolWhereClause(query domain.ToolListQuery) (string, []any) {
	where := []string{"t.deleted_at = ''"}
	args := []any{}
	if search := strings.TrimSpace(query.Search); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = append(where, "(LOWER(t.tool_key) LIKE ? OR LOWER(t.display_name) LIKE ? OR LOWER(t.description) LIKE ?)")
		args = append(args, like, like, like)
	}
	if query.Category != "" {
		where = append(where, "t.category = ?")
		args = append(args, query.Category)
	}
	if query.Source != "" {
		where = append(where, "t.source = ?")
		args = append(args, query.Source)
	}
	if query.RiskLevel != "" {
		where = append(where, "t.risk_level = ?")
		args = append(args, query.RiskLevel)
	}
	if query.Enabled == "true" || query.Enabled == "false" {
		where = append(where, "t.enabled = ?")
		args = append(args, query.Enabled == "true")
	}
	return strings.Join(where, " AND "), args
}

func toolSelectSQL() string {
	return `
		SELECT t.id, t.tool_key, t.display_name, t.description, t.category, t.source, t.risk_level,
		       t.enabled, t.readonly, t.requires_confirmation, t.supports_streaming, t.supports_concurrency,
		       t.parameters_schema_json, t.result_schema_json, t.config_schema_json, t.config_json, t.default_config_json, t.metadata_json,
		       COALESCE(stats.invoke_count, 0), COALESCE(stats.invoke_count_24h, 0), COALESCE(stats.success_count, 0),
		       COALESCE(stats.failure_count, 0), COALESCE(stats.blocked_count, 0), COALESCE(overrides.agent_override_count, 0),
		       stats.avg_duration_ms, COALESCE(last.started_at, ''), COALESCE(last.status, ''),
		       t.created_at, t.updated_at, t.deleted_at
		FROM tools t
		LEFT JOIN (
			SELECT tool_key,
			       COUNT(1) AS invoke_count,
			       SUM(CASE WHEN started_at >= ? THEN 1 ELSE 0 END) AS invoke_count_24h,
			       SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) AS success_count,
			       SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) AS failure_count,
			       SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END) AS blocked_count,
			       AVG(duration_ms) AS avg_duration_ms
			FROM tool_invocations
			GROUP BY tool_key
		) stats ON stats.tool_key = t.tool_key
		LEFT JOIN (
			SELECT tool_key, COUNT(1) AS agent_override_count
			FROM tool_agent_overrides
			WHERE deleted_at = ''
			GROUP BY tool_key
		) overrides ON overrides.tool_key = t.tool_key
		LEFT JOIN (
			SELECT ti.tool_key, ti.started_at, ti.status
			FROM tool_invocations ti
			INNER JOIN (
				SELECT tool_key, MAX(started_at) AS max_started_at
				FROM tool_invocations
				GROUP BY tool_key
			) latest ON latest.tool_key = ti.tool_key AND latest.max_started_at = ti.started_at
		) last ON last.tool_key = t.tool_key`
}

func scanTools(rows *sql.Rows) ([]domain.Tool, error) {
	items := []domain.Tool{}
	for rows.Next() {
		var item domain.Tool
		if err := rows.Scan(
			&item.ID, &item.Key, &item.DisplayName, &item.Description, &item.Category, &item.Source, &item.RiskLevel,
			&item.Enabled, &item.Readonly, &item.RequiresConfirmation, &item.SupportsStreaming, &item.SupportsConcurrency,
			&item.ParametersSchemaJSON, &item.ResultSchemaJSON, &item.ConfigSchemaJSON, &item.ConfigJSON, &item.DefaultConfigJSON, &item.MetadataJSON,
			&item.InvokeCount, &item.InvokeCount24h, &item.SuccessCount, &item.FailureCount, &item.BlockedCount, &item.AgentOverrideCount,
			&item.AvgDurationMS, &item.LastInvokedAt, &item.LastStatus,
			&item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
		); err != nil {
			return nil, err
		}
		item.Permissions = domain.ToolPermissions{CanManage: true}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SQLiteRepository) SearchSkillInvocations(query domain.SkillRunQuery) (domain.SkillRunResult, error) {
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	where := []string{"1 = 1"}
	args := []any{}
	if query.SkillID != "" {
		where = append(where, "si.skill_id = ?")
		args = append(args, query.SkillID)
	}
	if query.AgentID != "" {
		where = append(where, "si.agent_id = ?")
		args = append(args, query.AgentID)
	}
	if query.Status != "" {
		where = append(where, "si.status = ?")
		args = append(args, query.Status)
	}
	if query.From != "" {
		where = append(where, "COALESCE(NULLIF(si.started_at, ''), si.created_at) >= ?")
		args = append(args, query.From)
	}
	if query.To != "" {
		where = append(where, "COALESCE(NULLIF(si.started_at, ''), si.created_at) <= ?")
		args = append(args, query.To)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM skill_invocation si WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return domain.SkillRunResult{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.Limit, query.Offset)
	rows, err := r.db.Query(`
		SELECT si.id, si.skill_id, COALESCE(s.name, ''), si.skill_version, si.agent_id, COALESCE(a.display_name, ''),
		       si.user_id, si.session_id, si.status, si.duration_ms,
		       COALESCE(NULLIF(si.started_at, ''), si.created_at), si.ended_at,
		       si.input_preview, si.input_hash, si.output_preview, si.error_code, si.error_message
		FROM skill_invocation si
		LEFT JOIN skill s ON s.id = si.skill_id
		LEFT JOIN agents a ON a.id = si.agent_id
		WHERE `+whereSQL+`
		ORDER BY COALESCE(NULLIF(si.started_at, ''), si.created_at) DESC
		LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return domain.SkillRunResult{}, err
	}
	defer rows.Close()
	items := []domain.SkillInvocation{}
	for rows.Next() {
		var item domain.SkillInvocation
		if err = rows.Scan(
			&item.ID, &item.SkillID, &item.SkillName, &item.SkillVersion, &item.AgentID, &item.AgentDisplayName,
			&item.UserID, &item.SessionID, &item.Status, &item.DurationMS, &item.StartedAt, &item.EndedAt,
			&item.InputPreview, &item.InputHash, &item.OutputPreview, &item.ErrorCode, &item.ErrorMessage,
		); err != nil {
			return domain.SkillRunResult{}, err
		}
		item.Permissions = domain.SkillInvocationPermissions{CanViewDetail: true}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return domain.SkillRunResult{}, err
	}
	return domain.SkillRunResult{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

func (r *SQLiteRepository) ListSkillSimilaritySources() ([]domain.SkillSimilaritySource, error) {
	rows, err := r.db.Query(`
		SELECT s.id, s.name, s.skill_key, s.description,
		       COALESCE((SELECT sv.version FROM skill_version sv WHERE sv.skill_id = s.id ORDER BY sv.created_at DESC LIMIT 1), ''),
		       COALESCE((SELECT sv.content_markdown FROM skill_version sv WHERE sv.skill_id = s.id ORDER BY sv.created_at DESC LIMIT 1), '')
		FROM skill s
		WHERE s.deleted_at = ''
		ORDER BY s.updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.SkillSimilaritySource{}
	for rows.Next() {
		var item domain.SkillSimilaritySource
		if err = rows.Scan(&item.ID, &item.Name, &item.Slug, &item.Description, &item.Version, &item.Body); err != nil {
			return nil, err
		}
		item.BodyPreview = previewText(item.Body, 240)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SQLiteRepository) CreateSkillWithVersion(input domain.SkillCreateInput) (domain.Skill, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = strings.TrimSpace(input.Slug)
	input.Description = strings.TrimSpace(input.Description)
	input.Body = strings.TrimSpace(input.Body)
	if input.Name == "" || input.Slug == "" || input.Body == "" {
		return domain.Skill{}, errors.New("skill name, slug and body are required")
	}
	now := nowISO()
	skillID := fmt.Sprintf("skill_%d", time.Now().UTC().UnixNano())
	versionID := fmt.Sprintf("skillver_%d", time.Now().UTC().UnixNano())
	metadata := struct {
		Tags       []domain.SkillTag `json:"tags"`
		StorageDir string            `json:"storage_dir"`
	}{Tags: input.Tags, StorageDir: input.StorageDir}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return domain.Skill{}, err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return domain.Skill{}, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(
		`INSERT INTO skill(id, skill_key, name, description, status, enabled, sort_order, config_json, metadata_json, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, 'draft', 0, 0, '', ?, ?, ?, '')`,
		skillID, input.Slug, input.Name, input.Description, string(metadataJSON), now, now,
	); err != nil {
		return domain.Skill{}, err
	}
	if _, err = tx.Exec(
		`INSERT INTO skill_version(id, skill_id, version, status, content_markdown, metadata_json, created_at, updated_at) VALUES (?, ?, '1.0.0', 'pass', ?, ?, ?, ?)`,
		versionID, skillID, input.Body, string(metadataJSON), now, now,
	); err != nil {
		return domain.Skill{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Skill{}, err
	}
	return r.GetSkillByID(skillID)
}

func (r *SQLiteRepository) GetSkillStorageDir(id string) (string, error) {
	var raw string
	if err := r.db.QueryRow(`SELECT metadata_json FROM skill WHERE id = ? AND deleted_at = ''`, id).Scan(&raw); err != nil {
		return "", err
	}
	var metadata struct {
		StorageDir string `json:"storage_dir"`
	}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return "", err
	}
	if strings.TrimSpace(metadata.StorageDir) == "" {
		return "", errors.New("skill storage directory is not configured")
	}
	return metadata.StorageDir, nil
}

func (r *SQLiteRepository) ensureCategoryCanDelete(id string) error {
	var children int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM agent_category_nodes WHERE parent_id = ? AND deleted_at = ''`, id).Scan(&children); err != nil {
		return err
	}
	if children > 0 {
		return fmt.Errorf("category has %d child nodes", children)
	}
	var agents int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM agents WHERE category_position_id = ? AND deleted_at = ''`, id).Scan(&agents); err != nil {
		return err
	}
	if agents > 0 {
		return fmt.Errorf("category is used by %d agents", agents)
	}
	return nil
}

func (r *SQLiteRepository) ValidateProviderModel(provider string, model string) (bool, error) {
	if provider == "" || model == "" {
		return false, nil
	}
	var count int
	err := r.db.QueryRow(`SELECT COUNT(1) FROM llm_provider_models WHERE provider = ? AND model = ? AND enabled = 1 AND deleted_at = ''`, provider, model).Scan(&count)
	return count > 0, err
}

func (r *SQLiteRepository) GetProviderModel(provider string, model string) (domain.PlatformResource, error) {
	if provider == "" || model == "" {
		return domain.PlatformResource{}, errors.New("provider and model are required")
	}
	row := r.db.QueryRow(
		`SELECT id, model_key, name, description, status, enabled, sort_order, parent_id, level, agent_id, provider, model, config_json, metadata_json, created_at, updated_at, deleted_at
		 FROM llm_provider_models
		 WHERE provider = ? AND model = ? AND enabled = 1 AND deleted_at = ''
		 LIMIT 1`,
		provider,
		model,
	)
	return scanPlatformResource("llm-provider-models", row)
}

func optionalColumn(table string, column string) string {
	return column
}

func skillSelectSQL() string {
	return `
		SELECT s.id, s.skill_key, s.name, s.description, s.status, s.enabled, s.config_json, s.metadata_json, s.created_at, s.updated_at,
		       (SELECT sv.id FROM skill_version sv WHERE sv.skill_id = s.id ORDER BY sv.created_at DESC LIMIT 1),
		       (SELECT sv.version FROM skill_version sv WHERE sv.skill_id = s.id ORDER BY sv.created_at DESC LIMIT 1),
		       (SELECT sv.status FROM skill_version sv WHERE sv.skill_id = s.id ORDER BY sv.created_at DESC LIMIT 1),
		       (SELECT sv.created_at FROM skill_version sv WHERE sv.skill_id = s.id ORDER BY sv.created_at DESC LIMIT 1),
		       (SELECT COUNT(1) FROM skill_invocation si WHERE si.skill_id = s.id),
		       (SELECT COALESCE(SUM(CASE WHEN si.status = 'success' THEN 1 ELSE 0 END), 0) FROM skill_invocation si WHERE si.skill_id = s.id),
		       (SELECT COALESCE(SUM(CASE WHEN si.status = 'failure' THEN 1 ELSE 0 END), 0) FROM skill_invocation si WHERE si.skill_id = s.id),
		       (SELECT COUNT(1) FROM skill_invocation si WHERE si.skill_id = s.id AND COALESCE(NULLIF(si.started_at, ''), si.created_at) >= ?),
		       (SELECT AVG(NULLIF(si.duration_ms, 0)) FROM skill_invocation si WHERE si.skill_id = s.id),
		       (SELECT si.agent_id FROM skill_invocation si WHERE si.skill_id = s.id ORDER BY COALESCE(NULLIF(si.started_at, ''), si.created_at) DESC LIMIT 1),
		       (SELECT COALESCE(a.display_name, '') FROM skill_invocation si LEFT JOIN agents a ON a.id = si.agent_id WHERE si.skill_id = s.id ORDER BY COALESCE(NULLIF(si.started_at, ''), si.created_at) DESC LIMIT 1),
		       (SELECT COALESCE(NULLIF(si.started_at, ''), si.created_at) FROM skill_invocation si WHERE si.skill_id = s.id ORDER BY COALESCE(NULLIF(si.started_at, ''), si.created_at) DESC LIMIT 1),
		       (SELECT si.duration_ms FROM skill_invocation si WHERE si.skill_id = s.id ORDER BY COALESCE(NULLIF(si.started_at, ''), si.created_at) DESC LIMIT 1)
		FROM skill s`
}

func skillWhereClause(query domain.SkillListQuery) (string, []any) {
	where := []string{"s.deleted_at = ''"}
	args := []any{}
	if q := strings.TrimSpace(query.Search); q != "" {
		where = append(where, "(LOWER(s.skill_key) LIKE ? OR LOWER(s.name) LIKE ? OR LOWER(s.description) LIKE ?)")
		like := "%" + strings.ToLower(q) + "%"
		args = append(args, like, like, like)
	}
	if query.Tags != "" {
		for _, tag := range strings.Split(query.Tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			where = append(where, "LOWER(s.metadata_json) LIKE ?")
			args = append(args, "%"+strings.ToLower(tag)+"%")
		}
	}
	if query.Enabled == "true" {
		where = append(where, "s.enabled = 1")
	}
	if query.Enabled == "false" {
		where = append(where, "s.enabled = 0")
	}
	if query.Status != "" {
		if query.Status == "published" {
			where = append(where, "s.status IN ('published', 'active')")
		} else {
			where = append(where, "s.status = ?")
			args = append(args, query.Status)
		}
	}
	return strings.Join(where, " AND "), args
}

func scanSkills(rows *sql.Rows) ([]domain.Skill, error) {
	items := []domain.Skill{}
	for rows.Next() {
		var item domain.Skill
		var enabled bool
		var configJSON string
		var metadataJSON string
		var versionID, version, validationStatus, publishedAt sql.NullString
		var avgDuration sql.NullFloat64
		var lastAgentID, lastAgentName, lastInvokedAt sql.NullString
		var lastDuration sql.NullInt64
		if err := rows.Scan(
			&item.ID, &item.Slug, &item.Name, &item.Description, &item.Status, &enabled, &configJSON, &metadataJSON, &item.CreatedAt, &item.UpdatedAt,
			&versionID, &version, &validationStatus, &publishedAt,
			&item.InvokeCount, &item.SuccessCount, &item.FailureCount, &item.UsageCount7d, &avgDuration,
			&lastAgentID, &lastAgentName, &lastInvokedAt, &lastDuration,
		); err != nil {
			return nil, err
		}
		item.Status = normalizeSkillStatus(item.Status)
		item.Enabled = enabled
		item.Tags = parseSkillTags(metadataJSON)
		if len(item.Tags) == 0 {
			item.Tags = parseSkillTags(configJSON)
		}
		if versionID.Valid {
			status := validationStatus.String
			if status == "" || status == "active" {
				status = "pass"
			}
			item.CurrentVersion = &domain.SkillVersionSummary{
				ID:               versionID.String,
				Version:          version.String,
				ValidationStatus: status,
				PublishedAt:      publishedAt.String,
			}
		}
		if avgDuration.Valid {
			v := avgDuration.Float64
			item.AvgDurationMS = &v
		}
		if lastDuration.Valid {
			v := int(lastDuration.Int64)
			item.LastDurationMS = &v
		}
		item.LastAgentID = lastAgentID.String
		item.LastAgentDisplayName = lastAgentName.String
		item.LastInvokedAt = lastInvokedAt.String
		item.Permissions = domain.SkillPermissions{
			CanEdit:          true,
			CanDelete:        true,
			CanToggleEnabled: true,
			CanDuplicate:     true,
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func normalizeSkillStatus(status string) string {
	switch status {
	case "", "active", "published":
		return "published"
	case "inactive":
		return "archived"
	default:
		return status
	}
}

func parseSkillTags(raw string) []domain.SkillTag {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []domain.SkillTag{}
	}
	var envelope struct {
		Tags []domain.SkillTag `json:"tags"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err == nil && len(envelope.Tags) > 0 {
		return normalizeSkillTags(envelope.Tags)
	}
	var tags []domain.SkillTag
	if err := json.Unmarshal([]byte(raw), &tags); err == nil {
		return normalizeSkillTags(tags)
	}
	return []domain.SkillTag{}
}

func normalizeSkillTags(tags []domain.SkillTag) []domain.SkillTag {
	result := []domain.SkillTag{}
	seen := map[string]bool{}
	for _, tag := range tags {
		tag.Name = strings.TrimSpace(tag.Name)
		if tag.Name == "" || seen[strings.ToLower(tag.Name)] {
			continue
		}
		if tag.Source == "" {
			tag.Source = "user"
		}
		seen[strings.ToLower(tag.Name)] = true
		result = append(result, tag)
	}
	return result
}

func previewText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "..."
}

func (r *SQLiteRepository) ListAgents() ([]domain.Agent, error) {
	rows, err := r.db.Query(`SELECT id, agent_key, display_name, provider, model, status, is_default, is_favorite, icon, agent_description, category_position_id, system_prompt_mode, context_window, budget_monthly_cents, config_json, created_at, updated_at, deleted_at FROM agents WHERE deleted_at = '' ORDER BY is_default DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgents(rows)
}

func (r *SQLiteRepository) SearchAgents(query domain.AgentListQuery) (domain.AgentListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 24
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}

	where := []string{"deleted_at = ''"}
	args := []any{}
	if q := strings.TrimSpace(query.Keyword); q != "" {
		where = append(where, "(LOWER(agent_key) LIKE ? OR LOWER(display_name) LIKE ? OR LOWER(provider) LIKE ? OR LOWER(model) LIKE ? OR LOWER(agent_description) LIKE ?)")
		like := "%" + strings.ToLower(q) + "%"
		args = append(args, like, like, like, like, like)
	}
	if query.Status != "" {
		where = append(where, "status = ?")
		args = append(args, query.Status)
	}
	if query.Provider != "" {
		where = append(where, "provider = ?")
		args = append(args, query.Provider)
	}
	if query.CategoryID != "" {
		where = append(where, "category_position_id = ?")
		args = append(args, query.CategoryID)
	}

	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM agents WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return domain.AgentListResult{}, err
	}

	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.Limit, query.Offset)
	rows, err := r.db.Query(`SELECT id, agent_key, display_name, provider, model, status, is_default, is_favorite, icon, agent_description, category_position_id, system_prompt_mode, context_window, budget_monthly_cents, config_json, created_at, updated_at, deleted_at FROM agents WHERE `+whereSQL+` ORDER BY is_default DESC, updated_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return domain.AgentListResult{}, err
	}
	defer rows.Close()

	items, err := scanAgents(rows)
	if err != nil {
		return domain.AgentListResult{}, err
	}
	return domain.AgentListResult{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

func (r *SQLiteRepository) GetAgentByID(id string) (domain.Agent, error) {
	row := r.db.QueryRow(`SELECT id, agent_key, display_name, provider, model, status, is_default, is_favorite, icon, agent_description, category_position_id, system_prompt_mode, context_window, budget_monthly_cents, config_json, created_at, updated_at, deleted_at FROM agents WHERE id = ? AND deleted_at = ''`, id)
	return scanAgent(row)
}

func (r *SQLiteRepository) GetAgentByKey(key string) (domain.Agent, error) {
	row := r.db.QueryRow(`SELECT id, agent_key, display_name, provider, model, status, is_default, is_favorite, icon, agent_description, category_position_id, system_prompt_mode, context_window, budget_monthly_cents, config_json, created_at, updated_at, deleted_at FROM agents WHERE agent_key = ? AND deleted_at = ''`, key)
	return scanAgent(row)
}

func (r *SQLiteRepository) CreateAgent(a domain.Agent) (domain.Agent, error) {
	if a.ID == "" || a.AgentKey == "" || a.DisplayName == "" || a.Provider == "" || a.Model == "" {
		return domain.Agent{}, errors.New("missing required fields")
	}
	now := nowISO()
	a.CreatedAt = now
	a.UpdatedAt = now
	if a.Status == "" {
		a.Status = "active"
	}
	_, err := r.db.Exec(
		`INSERT INTO agents(id, agent_key, display_name, provider, model, status, is_default, is_favorite, icon, agent_description, category_position_id, system_prompt_mode, context_window, budget_monthly_cents, config_json, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.AgentKey, a.DisplayName, a.Provider, a.Model, a.Status, a.IsDefault, a.IsFavorite, a.Icon, a.AgentDescription, a.CategoryPositionID, a.SystemPromptMode, a.ContextWindow, a.BudgetMonthlyCents, a.ConfigJSON, a.CreatedAt, a.UpdatedAt, a.DeletedAt,
	)
	return a, err
}

func (r *SQLiteRepository) UpdateAgent(a domain.Agent) (domain.Agent, error) {
	if a.ID == "" {
		return domain.Agent{}, errors.New("id is required")
	}
	current, err := r.GetAgentByID(a.ID)
	if err != nil {
		return domain.Agent{}, err
	}
	if a.AgentKey == "" {
		a.AgentKey = current.AgentKey
	}
	if a.DisplayName == "" {
		a.DisplayName = current.DisplayName
	}
	if a.Provider == "" {
		a.Provider = current.Provider
	}
	if a.Model == "" {
		a.Model = current.Model
	}
	if a.Status == "" {
		a.Status = current.Status
	}
	a.CreatedAt = current.CreatedAt
	a.UpdatedAt = nowISO()
	_, err = r.db.Exec(
		`UPDATE agents SET display_name = ?, provider = ?, model = ?, status = ?, is_default = ?, is_favorite = ?, icon = ?, agent_description = ?, category_position_id = ?, system_prompt_mode = ?, context_window = ?, budget_monthly_cents = ?, config_json = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`,
		a.DisplayName, a.Provider, a.Model, a.Status, a.IsDefault, a.IsFavorite, a.Icon, a.AgentDescription, a.CategoryPositionID, a.SystemPromptMode, a.ContextWindow, a.BudgetMonthlyCents, a.ConfigJSON, a.UpdatedAt, a.ID,
	)
	return a, err
}

func (r *SQLiteRepository) GetAgentRuntimeSettings(agentID string) (domain.AgentRuntimeSettings, error) {
	row := r.db.QueryRow(`SELECT agent_id, self_evolve, subagents_enabled, subagents_max_concurrency, subagents_max_generation_depth, subagents_max_children_per_agent, subagents_archive_after_minutes, subagents_max_retries, subagents_model_override, tools_enabled, tools_profile, tools_tool_call_prefix, tools_allow_json, tools_deny_json, tools_concurrent_allow_json, memory_enabled, memory_max_chunk_length, memory_max_results, memory_min_score, heartbeat_enabled, heartbeat_interval_minutes, evolution_self_evolve, evolution_skill_evolve, evolution_metrics_enabled, evolution_suggestions_enabled, guardrail_max_change_per_period, guardrail_min_data_points, guardrail_rollback_on_decline_percent, created_at, updated_at FROM agent_runtime_settings WHERE agent_id = ?`, agentID)
	var v domain.AgentRuntimeSettings
	err := row.Scan(
		&v.AgentID, &v.SelfEvolve, &v.SubagentsEnabled, &v.SubagentsMaxConcurrency, &v.SubagentsMaxGenerationDepth, &v.SubagentsMaxChildrenPerAgent, &v.SubagentsArchiveAfterMinutes, &v.SubagentsMaxRetries, &v.SubagentsModelOverride,
		&v.ToolsEnabled, &v.ToolsProfile, &v.ToolsToolCallPrefix, &v.ToolsAllowJSON, &v.ToolsDenyJSON, &v.ToolsConcurrentAllowJSON,
		&v.MemoryEnabled, &v.MemoryMaxChunkLength, &v.MemoryMaxResults, &v.MemoryMinScore,
		&v.HeartbeatEnabled, &v.HeartbeatIntervalMinutes,
		&v.EvolutionSelfEvolve, &v.EvolutionSkillEvolve, &v.EvolutionMetricsEnabled, &v.EvolutionSuggestionsEnabled,
		&v.GuardrailMaxChangePerPeriod, &v.GuardrailMinDataPoints, &v.GuardrailRollbackOnDeclinePercent, &v.CreatedAt, &v.UpdatedAt,
	)
	return v, err
}

func (r *SQLiteRepository) UpsertAgentRuntimeSettings(v domain.AgentRuntimeSettings) (domain.AgentRuntimeSettings, error) {
	if v.AgentID == "" {
		return domain.AgentRuntimeSettings{}, errors.New("agent id is required")
	}
	now := nowISO()
	if v.CreatedAt == "" {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	_, err := r.db.Exec(
		`INSERT INTO agent_runtime_settings(agent_id, self_evolve, subagents_enabled, subagents_max_concurrency, subagents_max_generation_depth, subagents_max_children_per_agent, subagents_archive_after_minutes, subagents_max_retries, subagents_model_override, tools_enabled, tools_profile, tools_tool_call_prefix, tools_allow_json, tools_deny_json, tools_concurrent_allow_json, memory_enabled, memory_max_chunk_length, memory_max_results, memory_min_score, heartbeat_enabled, heartbeat_interval_minutes, evolution_self_evolve, evolution_skill_evolve, evolution_metrics_enabled, evolution_suggestions_enabled, guardrail_max_change_per_period, guardrail_min_data_points, guardrail_rollback_on_decline_percent, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(agent_id) DO UPDATE SET self_evolve = excluded.self_evolve, subagents_enabled = excluded.subagents_enabled, subagents_max_concurrency = excluded.subagents_max_concurrency, subagents_max_generation_depth = excluded.subagents_max_generation_depth, subagents_max_children_per_agent = excluded.subagents_max_children_per_agent, subagents_archive_after_minutes = excluded.subagents_archive_after_minutes, subagents_max_retries = excluded.subagents_max_retries, subagents_model_override = excluded.subagents_model_override, tools_enabled = excluded.tools_enabled, tools_profile = excluded.tools_profile, tools_tool_call_prefix = excluded.tools_tool_call_prefix, tools_allow_json = excluded.tools_allow_json, tools_deny_json = excluded.tools_deny_json, tools_concurrent_allow_json = excluded.tools_concurrent_allow_json, memory_enabled = excluded.memory_enabled, memory_max_chunk_length = excluded.memory_max_chunk_length, memory_max_results = excluded.memory_max_results, memory_min_score = excluded.memory_min_score, heartbeat_enabled = excluded.heartbeat_enabled, heartbeat_interval_minutes = excluded.heartbeat_interval_minutes, evolution_self_evolve = excluded.evolution_self_evolve, evolution_skill_evolve = excluded.evolution_skill_evolve, evolution_metrics_enabled = excluded.evolution_metrics_enabled, evolution_suggestions_enabled = excluded.evolution_suggestions_enabled, guardrail_max_change_per_period = excluded.guardrail_max_change_per_period, guardrail_min_data_points = excluded.guardrail_min_data_points, guardrail_rollback_on_decline_percent = excluded.guardrail_rollback_on_decline_percent, updated_at = excluded.updated_at`,
		v.AgentID, v.SelfEvolve, v.SubagentsEnabled, v.SubagentsMaxConcurrency, v.SubagentsMaxGenerationDepth, v.SubagentsMaxChildrenPerAgent, v.SubagentsArchiveAfterMinutes, v.SubagentsMaxRetries, v.SubagentsModelOverride,
		v.ToolsEnabled, v.ToolsProfile, v.ToolsToolCallPrefix, normalizeJSONList(v.ToolsAllowJSON), normalizeJSONList(v.ToolsDenyJSON), normalizeJSONList(v.ToolsConcurrentAllowJSON),
		v.MemoryEnabled, v.MemoryMaxChunkLength, v.MemoryMaxResults, v.MemoryMinScore,
		v.HeartbeatEnabled, v.HeartbeatIntervalMinutes,
		v.EvolutionSelfEvolve, v.EvolutionSkillEvolve, v.EvolutionMetricsEnabled, v.EvolutionSuggestionsEnabled,
		v.GuardrailMaxChangePerPeriod, v.GuardrailMinDataPoints, v.GuardrailRollbackOnDeclinePercent, v.CreatedAt, v.UpdatedAt,
	)
	if err != nil {
		return domain.AgentRuntimeSettings{}, err
	}
	return r.GetAgentRuntimeSettings(v.AgentID)
}

func (r *SQLiteRepository) ListAgentPromptFiles(agentID string) ([]domain.AgentPromptFile, error) {
	rows, err := r.db.Query(`SELECT id, agent_id, file_name, body, sort_order, created_at, updated_at FROM agent_prompt_files WHERE agent_id = ? ORDER BY sort_order ASC, file_name ASC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := []domain.AgentPromptFile{}
	for rows.Next() {
		var v domain.AgentPromptFile
		if err = rows.Scan(&v.ID, &v.AgentID, &v.Name, &v.Body, &v.SortOrder, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		files = append(files, v)
	}
	return files, rows.Err()
}

func (r *SQLiteRepository) ReplaceAgentPromptFiles(agentID string, files []domain.AgentPromptFile) ([]domain.AgentPromptFile, error) {
	if agentID == "" {
		return nil, errors.New("agent id is required")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err = tx.Exec(`DELETE FROM agent_prompt_files WHERE agent_id = ?`, agentID); err != nil {
		return nil, err
	}
	now := nowISO()
	for i, file := range files {
		if strings.TrimSpace(file.Name) == "" {
			continue
		}
		if file.ID == "" {
			file.ID = fmt.Sprintf("%s_%s", agentID, sanitizePromptFileID(file.Name))
		}
		if file.SortOrder == 0 {
			file.SortOrder = (i + 1) * 10
		}
		if _, err = tx.Exec(`INSERT INTO agent_prompt_files(id, agent_id, file_name, body, sort_order, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			file.ID, agentID, strings.TrimSpace(file.Name), file.Body, file.SortOrder, now, now); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.ListAgentPromptFiles(agentID)
}

func (r *SQLiteRepository) DeleteAgent(id string) error {
	if id == "" {
		return errors.New("id is required")
	}
	_, err := r.db.Exec(`UPDATE agents SET deleted_at = ?, status = 'deleted', updated_at = ? WHERE id = ? AND deleted_at = ''`, nowISO(), nowISO(), id)
	return err
}

func (r *SQLiteRepository) ListTeams() ([]domain.Team, error) {
	rows, err := r.db.Query(`SELECT id, team_key, display_name, status, is_default, definition_json, adk_app_name, created_at, updated_at, deleted_at FROM teams WHERE deleted_at = '' ORDER BY is_default DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.Team
	for rows.Next() {
		var v domain.Team
		if err = rows.Scan(&v.ID, &v.TeamKey, &v.DisplayName, &v.Status, &v.IsDefault, &v.DefinitionJSON, &v.ADKAppName, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) GetTeamByID(id string) (domain.Team, error) {
	row := r.db.QueryRow(`SELECT id, team_key, display_name, status, is_default, definition_json, adk_app_name, created_at, updated_at, deleted_at FROM teams WHERE id = ? AND deleted_at = ''`, id)
	var v domain.Team
	if err := row.Scan(&v.ID, &v.TeamKey, &v.DisplayName, &v.Status, &v.IsDefault, &v.DefinitionJSON, &v.ADKAppName, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt); err != nil {
		return domain.Team{}, err
	}
	return v, nil
}

func (r *SQLiteRepository) CreateTeam(t domain.Team) (domain.Team, error) {
	if t.ID == "" || t.TeamKey == "" || t.DisplayName == "" {
		return domain.Team{}, errors.New("missing required fields")
	}
	now := nowISO()
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = "active"
	}
	_, err := r.db.Exec(`INSERT INTO teams(id, team_key, display_name, status, is_default, definition_json, adk_app_name, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.TeamKey, t.DisplayName, t.Status, t.IsDefault, t.DefinitionJSON, t.ADKAppName, t.CreatedAt, t.UpdatedAt, t.DeletedAt)
	return t, err
}

func (r *SQLiteRepository) UpdateTeam(t domain.Team) (domain.Team, error) {
	if t.ID == "" || t.TeamKey == "" || t.DisplayName == "" {
		return domain.Team{}, errors.New("missing required fields")
	}
	t.UpdatedAt = nowISO()
	if t.Status == "" {
		t.Status = "active"
	}
	_, err := r.db.Exec(`UPDATE teams SET team_key = ?, display_name = ?, status = ?, is_default = ?, definition_json = ?, adk_app_name = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`,
		t.TeamKey, t.DisplayName, t.Status, t.IsDefault, t.DefinitionJSON, t.ADKAppName, t.UpdatedAt, t.ID)
	if err != nil {
		return domain.Team{}, err
	}
	return r.GetTeamByID(t.ID)
}

func (r *SQLiteRepository) DeleteTeam(id string) error {
	if id == "" {
		return errors.New("id is required")
	}
	_, err := r.db.Exec(`UPDATE teams SET deleted_at = ?, status = 'deleted', updated_at = ? WHERE id = ? AND deleted_at = '' AND is_default = 0`, nowISO(), nowISO(), id)
	return err
}

func (r *SQLiteRepository) AddTeamRun(run domain.TeamRun) (domain.TeamRun, error) {
	now := nowISO()
	if run.ID == "" || run.TeamID == "" {
		return domain.TeamRun{}, errors.New("team run id and team_id are required")
	}
	if run.CreatedAt == "" {
		run.CreatedAt = now
	}
	if run.UpdatedAt == "" {
		run.UpdatedAt = now
	}
	if run.StartedAt == "" {
		run.StartedAt = now
	}
	if run.Status == "" {
		run.Status = "running"
	}
	if run.TopologyJSON == "" {
		run.TopologyJSON = "{}"
	}
	_, err := r.db.Exec(`INSERT INTO team_runs(id, team_id, session_id, message_id, mode, status, input_preview, output_preview, token_in, token_out, duration_ms, error_message, topology_json, started_at, finished_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.TeamID, run.SessionID, run.MessageID, run.Mode, run.Status, run.InputPreview, run.OutputPreview, run.TokenIn, run.TokenOut, run.DurationMS, run.ErrorMessage, run.TopologyJSON, run.StartedAt, run.FinishedAt, run.CreatedAt, run.UpdatedAt)
	return run, err
}

func (r *SQLiteRepository) UpdateTeamRun(run domain.TeamRun) (domain.TeamRun, error) {
	if run.ID == "" {
		return domain.TeamRun{}, errors.New("team run id is required")
	}
	run.UpdatedAt = nowISO()
	_, err := r.db.Exec(`UPDATE team_runs SET message_id = ?, status = ?, output_preview = ?, token_in = ?, token_out = ?, duration_ms = ?, error_message = ?, topology_json = ?, finished_at = ?, updated_at = ? WHERE id = ?`,
		run.MessageID, run.Status, run.OutputPreview, run.TokenIn, run.TokenOut, run.DurationMS, run.ErrorMessage, run.TopologyJSON, run.FinishedAt, run.UpdatedAt, run.ID)
	if err != nil {
		return domain.TeamRun{}, err
	}
	items, err := r.ListTeamRuns(run.TeamID, 100)
	if err != nil {
		return domain.TeamRun{}, err
	}
	for _, item := range items {
		if item.ID == run.ID {
			return item, nil
		}
	}
	return run, nil
}

func (r *SQLiteRepository) AddTeamRunStep(step domain.TeamRunStep) (domain.TeamRunStep, error) {
	now := nowISO()
	if step.ID == "" || step.RunID == "" || step.TeamID == "" {
		return domain.TeamRunStep{}, errors.New("team run step id, run_id and team_id are required")
	}
	if step.CreatedAt == "" {
		step.CreatedAt = now
	}
	if step.StartedAt == "" {
		step.StartedAt = now
	}
	if step.FinishedAt == "" {
		step.FinishedAt = now
	}
	if step.Status == "" {
		step.Status = "success"
	}
	_, err := r.db.Exec(`INSERT INTO team_run_steps(id, run_id, team_id, agent_id, agent_key, agent_name, role, sort_order, status, input_preview, output_preview, token_in, token_out, duration_ms, error_message, started_at, finished_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		step.ID, step.RunID, step.TeamID, step.AgentID, step.AgentKey, step.AgentName, step.Role, step.SortOrder, step.Status, step.InputPreview, step.OutputPreview, step.TokenIn, step.TokenOut, step.DurationMS, step.ErrorMessage, step.StartedAt, step.FinishedAt, step.CreatedAt)
	return step, err
}

func (r *SQLiteRepository) ListTeamRuns(teamID string, limit int) ([]domain.TeamRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	where := "1=1"
	args := []any{}
	if teamID != "" {
		where = "team_id = ?"
		args = append(args, teamID)
	}
	args = append(args, limit)
	rows, err := r.db.Query(`SELECT id, team_id, session_id, message_id, mode, status, input_preview, output_preview, token_in, token_out, duration_ms, error_message, topology_json, started_at, finished_at, created_at, updated_at FROM team_runs WHERE `+where+` ORDER BY created_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.TeamRun{}
	for rows.Next() {
		var item domain.TeamRun
		if err = rows.Scan(&item.ID, &item.TeamID, &item.SessionID, &item.MessageID, &item.Mode, &item.Status, &item.InputPreview, &item.OutputPreview, &item.TokenIn, &item.TokenOut, &item.DurationMS, &item.ErrorMessage, &item.TopologyJSON, &item.StartedAt, &item.FinishedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SQLiteRepository) ListTeamRunSteps(runID string) ([]domain.TeamRunStep, error) {
	rows, err := r.db.Query(`SELECT id, run_id, team_id, agent_id, agent_key, agent_name, role, sort_order, status, input_preview, output_preview, token_in, token_out, duration_ms, error_message, started_at, finished_at, created_at FROM team_run_steps WHERE run_id = ? ORDER BY sort_order ASC, created_at ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.TeamRunStep{}
	for rows.Next() {
		var item domain.TeamRunStep
		if err = rows.Scan(&item.ID, &item.RunID, &item.TeamID, &item.AgentID, &item.AgentKey, &item.AgentName, &item.Role, &item.SortOrder, &item.Status, &item.InputPreview, &item.OutputPreview, &item.TokenIn, &item.TokenOut, &item.DurationMS, &item.ErrorMessage, &item.StartedAt, &item.FinishedAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

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
		 id, owner_type, agent_id, team_id, title, summary, context_used_ratio, max_context_used_ratio, context_status,
		 dialog_mode, provider, model, status, message_count, run_count, model_call_count, tool_call_count, skill_call_count,
		 mcp_call_count, input_tokens, output_tokens, total_tokens, total_cost_micro_usd, last_message_at, created_at, updated_at, archived_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.OwnerType, s.AgentID, s.TeamID, s.Title, s.Summary, s.ContextUsedRatio, s.MaxContextUsedRatio, s.ContextStatus,
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
	_, err := r.db.Exec(
		`INSERT INTO messages(id, session_id, parent_message_id, turn_index, role, content_markdown, model_name, token_in, token_out, latency_ms, status, attachments_count, options_json, error_message, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.SessionID, m.ParentMessageID, m.TurnIndex, m.Role, m.Content, m.ModelName, m.TokenIn, m.TokenOut, m.LatencyMS, m.Status, m.AttachmentsCount, m.OptionsJSON, m.ErrorMessage, m.CreatedAt,
	)
	if err != nil {
		return domain.Message{}, err
	}
	_, _ = r.db.Exec(`UPDATE sessions SET message_count = message_count + 1, last_message_at = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`, m.CreatedAt, m.CreatedAt, m.SessionID)
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

func (r *SQLiteRepository) GetActiveModelPricingRule(provider string, model string, at string) (domain.ModelPricingRule, error) {
	if at == "" {
		at = nowISO()
	}
	row := r.db.QueryRow(
		`SELECT id, provider_code, model_api_id, currency, input_price_micro_usd_per_1k, output_price_micro_usd_per_1k,
		 cached_input_price_micro_usd_per_1k, reasoning_price_micro_usd_per_1k, embedding_price_micro_usd_per_1k,
		 effective_from, effective_to, is_active, source, metadata_json, created_at, updated_at
		 FROM model_pricing_rules
		 WHERE provider_code = ? AND model_api_id = ? AND is_active = 1
		   AND effective_from <= ? AND (effective_to = '' OR effective_to > ?)
		 ORDER BY effective_from DESC LIMIT 1`,
		provider, model, at, at,
	)
	rule, err := scanModelPricingRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ModelPricingRule{ProviderCode: provider, ModelAPIID: model, Currency: "USD"}, nil
	}
	return rule, err
}

func (r *SQLiteRepository) UpsertModelPricingRule(rule domain.ModelPricingRule) (domain.ModelPricingRule, error) {
	if rule.ProviderCode == "" || rule.ModelAPIID == "" {
		return domain.ModelPricingRule{}, errors.New("provider_code and model_api_id are required")
	}
	now := nowISO()
	if rule.Currency == "" {
		rule.Currency = "USD"
	}
	if rule.EffectiveFrom == "" {
		rule.EffectiveFrom = now
	}
	if rule.Source == "" {
		rule.Source = "manual"
	}
	if rule.MetadataJSON == "" {
		rule.MetadataJSON = "{}"
	}
	result, err := r.db.Exec(
		`UPDATE model_pricing_rules SET currency = ?, input_price_micro_usd_per_1k = ?, output_price_micro_usd_per_1k = ?,
		 cached_input_price_micro_usd_per_1k = ?, reasoning_price_micro_usd_per_1k = ?, embedding_price_micro_usd_per_1k = ?,
		 source = ?, metadata_json = ?, updated_at = ?
		 WHERE provider_code = ? AND model_api_id = ? AND is_active = 1 AND effective_to = ''`,
		rule.Currency, rule.InputPriceMicroUSDPer1K, rule.OutputPriceMicroUSDPer1K, rule.CachedInputPriceMicroUSDPer1K, rule.ReasoningPriceMicroUSDPer1K, rule.EmbeddingPriceMicroUSDPer1K,
		rule.Source, rule.MetadataJSON, now, rule.ProviderCode, rule.ModelAPIID,
	)
	if err != nil {
		return domain.ModelPricingRule{}, err
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		return r.GetActiveModelPricingRule(rule.ProviderCode, rule.ModelAPIID, now)
	}
	if rule.ID == "" {
		rule.ID = fmt.Sprintf("pricing:%s:%s:%d", rule.ProviderCode, strings.ReplaceAll(rule.ModelAPIID, "/", "_"), time.Now().UTC().UnixNano())
	}
	rule.IsActive = true
	rule.CreatedAt = now
	rule.UpdatedAt = now
	_, err = r.db.Exec(
		`INSERT INTO model_pricing_rules(id, provider_code, model_api_id, currency, input_price_micro_usd_per_1k, output_price_micro_usd_per_1k,
		 cached_input_price_micro_usd_per_1k, reasoning_price_micro_usd_per_1k, embedding_price_micro_usd_per_1k,
		 effective_from, effective_to, is_active, source, metadata_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ID, rule.ProviderCode, rule.ModelAPIID, rule.Currency, rule.InputPriceMicroUSDPer1K, rule.OutputPriceMicroUSDPer1K,
		rule.CachedInputPriceMicroUSDPer1K, rule.ReasoningPriceMicroUSDPer1K, rule.EmbeddingPriceMicroUSDPer1K,
		rule.EffectiveFrom, rule.EffectiveTo, rule.IsActive, rule.Source, rule.MetadataJSON, rule.CreatedAt, rule.UpdatedAt,
	)
	if err != nil {
		return domain.ModelPricingRule{}, err
	}
	return rule, nil
}

func (r *SQLiteRepository) AddModelTokenUsageEvent(event domain.ModelTokenUsageEvent) (domain.ModelTokenUsageEvent, error) {
	if event.ID == "" {
		return domain.ModelTokenUsageEvent{}, errors.New("id is required")
	}
	if event.OccurredAt == "" {
		event.OccurredAt = nowISO()
	}
	if event.CreatedAt == "" {
		event.CreatedAt = event.OccurredAt
	}
	if event.DateKey == "" {
		event.DateKey = event.OccurredAt[:10]
	}
	if event.HourKey == "" {
		event.HourKey = event.OccurredAt[:13] + ":00"
	}
	if event.UsageKind == "" {
		event.UsageKind = "chat"
	}
	if event.CallCount <= 0 {
		event.CallCount = 1
	}
	if event.Status == "" {
		event.Status = "success"
	}
	if event.ModelCategoryJSON == "" {
		event.ModelCategoryJSON = "[]"
	}
	if event.MetadataJSON == "" {
		event.MetadataJSON = "{}"
	}
	streamEnabled := 0
	if event.StreamEnabled {
		streamEnabled = 1
	}
	_, err := r.db.Exec(
		`INSERT INTO model_token_usage_events(
		 id, occurred_at, date_key, hour_key, workspace_id, user_id, team_id, agent_id, agent_key, session_id, message_id, request_id,
		 provider_code, provider_type, provider_display_name, model_api_id, model_display_name, model_category_json, usage_kind, call_count,
		 input_tokens, output_tokens, cached_input_tokens, reasoning_tokens, embedding_tokens, total_tokens,
		 input_price_micro_usd_per_1k, output_price_micro_usd_per_1k, cached_input_price_micro_usd_per_1k, reasoning_price_micro_usd_per_1k, embedding_price_micro_usd_per_1k,
		 input_cost_micro_usd, output_cost_micro_usd, cached_input_cost_micro_usd, reasoning_cost_micro_usd, embedding_cost_micro_usd, total_cost_micro_usd,
		 latency_ms, time_to_first_token_ms, tokens_per_second, status, error_code, error_message, retry_count,
		 prompt_mode, max_output_tokens, context_window_k, stream_enabled, metadata_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.OccurredAt, event.DateKey, event.HourKey, event.WorkspaceID, event.UserID, event.TeamID, event.AgentID, event.AgentKey, event.SessionID, event.MessageID, event.RequestID,
		event.ProviderCode, event.ProviderType, event.ProviderDisplayName, event.ModelAPIID, event.ModelDisplayName, event.ModelCategoryJSON, event.UsageKind, event.CallCount,
		event.InputTokens, event.OutputTokens, event.CachedInputTokens, event.ReasoningTokens, event.EmbeddingTokens, event.TotalTokens,
		event.InputPriceMicroUSDPer1K, event.OutputPriceMicroUSDPer1K, event.CachedInputPriceMicroUSDPer1K, event.ReasoningPriceMicroUSDPer1K, event.EmbeddingPriceMicroUSDPer1K,
		event.InputCostMicroUSD, event.OutputCostMicroUSD, event.CachedInputCostMicroUSD, event.ReasoningCostMicroUSD, event.EmbeddingCostMicroUSD, event.TotalCostMicroUSD,
		event.LatencyMS, event.TimeToFirstTokenMS, event.TokensPerSecond, event.Status, event.ErrorCode, event.ErrorMessage, event.RetryCount,
		event.PromptMode, event.MaxOutputTokens, event.ContextWindowK, streamEnabled, event.MetadataJSON, event.CreatedAt,
	)
	if err != nil {
		return domain.ModelTokenUsageEvent{}, err
	}
	if event.SessionID != "" {
		_, _ = r.db.Exec(
			`UPDATE sessions
			 SET model_call_count = model_call_count + ?,
			     input_tokens = input_tokens + ?,
			     output_tokens = output_tokens + ?,
			     total_tokens = total_tokens + ?,
			     total_cost_micro_usd = total_cost_micro_usd + ?,
			     provider = ?,
			     model = ?,
			     updated_at = ?
			 WHERE id = ? AND deleted_at = ''`,
			event.CallCount, event.InputTokens, event.OutputTokens, event.TotalTokens, event.TotalCostMicroUSD,
			event.ProviderCode, event.ModelAPIID, nowISO(), event.SessionID,
		)
	}
	return event, nil
}

func (r *SQLiteRepository) UpsertModelTokenUsageDaily(event domain.ModelTokenUsageEvent) error {
	if event.DateKey == "" {
		return nil
	}
	successCount := 0
	failedCount := 0
	cancelledCount := 0
	switch event.Status {
	case "success":
		successCount = 1
	case "cancelled":
		cancelledCount = 1
	default:
		failedCount = 1
	}
	id := strings.Join([]string{event.DateKey, event.WorkspaceID, event.AgentID, event.ProviderCode, event.ModelAPIID, event.UsageKind}, ":")
	now := nowISO()
	_, err := r.db.Exec(
		`INSERT INTO model_token_usage_daily(
		 id, date_key, workspace_id, agent_id, agent_key, provider_code, model_api_id, usage_kind,
		 call_count, request_count, success_count, failed_count, cancelled_count,
		 input_tokens, output_tokens, cached_input_tokens, reasoning_tokens, embedding_tokens, total_tokens,
		 total_cost_micro_usd, avg_latency_ms, avg_tokens_per_second, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(date_key, workspace_id, agent_id, provider_code, model_api_id, usage_kind) DO UPDATE SET
		 call_count = call_count + excluded.call_count,
		 request_count = request_count + excluded.request_count,
		 success_count = success_count + excluded.success_count,
		 failed_count = failed_count + excluded.failed_count,
		 cancelled_count = cancelled_count + excluded.cancelled_count,
		 input_tokens = input_tokens + excluded.input_tokens,
		 output_tokens = output_tokens + excluded.output_tokens,
		 cached_input_tokens = cached_input_tokens + excluded.cached_input_tokens,
		 reasoning_tokens = reasoning_tokens + excluded.reasoning_tokens,
		 embedding_tokens = embedding_tokens + excluded.embedding_tokens,
		 total_tokens = total_tokens + excluded.total_tokens,
		 total_cost_micro_usd = total_cost_micro_usd + excluded.total_cost_micro_usd,
		 avg_latency_ms = ((avg_latency_ms * request_count) + excluded.avg_latency_ms) / (request_count + excluded.request_count),
		 avg_tokens_per_second = ((avg_tokens_per_second * request_count) + excluded.avg_tokens_per_second) / (request_count + excluded.request_count),
		 updated_at = excluded.updated_at`,
		id, event.DateKey, event.WorkspaceID, event.AgentID, event.AgentKey, event.ProviderCode, event.ModelAPIID, event.UsageKind,
		event.CallCount, successCount, failedCount, cancelledCount,
		event.InputTokens, event.OutputTokens, event.CachedInputTokens, event.ReasoningTokens, event.EmbeddingTokens, event.TotalTokens,
		event.TotalCostMicroUSD, float64(event.LatencyMS), event.TokensPerSecond, now, now,
	)
	return err
}

func (r *SQLiteRepository) GetModelUsageSummary(query domain.ModelUsageQuery) (domain.ModelUsageSummary, error) {
	where, args := usageWhere(query)
	row := r.db.QueryRow(
		`SELECT
		 COALESCE(SUM(call_count), 0), COUNT(*),
		 COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0),
		 COALESCE(SUM(CASE WHEN status = 'failed' OR status = 'timeout' THEN 1 ELSE 0 END), 0),
		 COALESCE(SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END), 0),
		 COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0), COALESCE(AVG(latency_ms), 0), COALESCE(AVG(tokens_per_second), 0)
		 FROM model_token_usage_events`+where,
		args...,
	)
	return scanModelUsageSummary(row)
}

func (r *SQLiteRepository) ListModelUsageTrends(query domain.ModelUsageQuery) ([]domain.ModelUsageTrendPoint, error) {
	where, args := usageWhere(query)
	rows, err := r.db.Query(
		`SELECT date_key,
		 COALESCE(SUM(call_count), 0),
		 COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0),
		 COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0),
		 COALESCE(SUM(CASE WHEN status = 'failed' OR status = 'timeout' THEN 1 ELSE 0 END), 0),
		 COALESCE(SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END), 0),
		 COALESCE(AVG(latency_ms), 0), COALESCE(AVG(tokens_per_second), 0)
		 FROM model_token_usage_events`+where+` GROUP BY date_key ORDER BY date_key ASC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ModelUsageTrendPoint{}
	for rows.Next() {
		var point domain.ModelUsageTrendPoint
		if err = rows.Scan(&point.DateKey, &point.CallCount, &point.InputTokens, &point.OutputTokens, &point.TotalTokens, &point.TotalCostMicroUSD, &point.SuccessCount, &point.FailedCount, &point.CancelledCount, &point.AvgLatencyMS, &point.AvgTokensPerSecond); err != nil {
			return nil, err
		}
		result = append(result, point)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) ListTopModelUsage(query domain.ModelUsageQuery) ([]domain.ModelUsageBreakdownRow, error) {
	where, args := usageWhere(query)
	args = append(args, usageLimit(query.Limit))
	rows, err := r.db.Query(
		`SELECT provider_code, model_api_id, MAX(model_display_name),
		 COALESCE(SUM(call_count), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0), COALESCE(AVG(latency_ms), 0), COALESCE(AVG(tokens_per_second), 0),
		 COALESCE(1.0 * SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 0)
		 FROM model_token_usage_events`+where+` GROUP BY provider_code, model_api_id ORDER BY total_cost_micro_usd DESC, call_count DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ModelUsageBreakdownRow{}
	for rows.Next() {
		var item domain.ModelUsageBreakdownRow
		if err = rows.Scan(&item.ProviderCode, &item.ModelAPIID, &item.ModelDisplayName, &item.CallCount, &item.InputTokens, &item.OutputTokens, &item.TotalTokens, &item.TotalCostMicroUSD, &item.AvgLatencyMS, &item.AvgTokensPerSecond, &item.SuccessRate); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) ListTopAgentUsage(query domain.ModelUsageQuery) ([]domain.ModelUsageBreakdownRow, error) {
	where, args := usageWhere(query)
	args = append(args, usageLimit(query.Limit))
	rows, err := r.db.Query(
		`SELECT agent_id, agent_key,
		 COALESCE(SUM(call_count), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0), COALESCE(AVG(latency_ms), 0), COALESCE(AVG(tokens_per_second), 0),
		 COALESCE(1.0 * SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 0)
		 FROM model_token_usage_events`+where+` GROUP BY agent_id, agent_key ORDER BY total_cost_micro_usd DESC, call_count DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ModelUsageBreakdownRow{}
	for rows.Next() {
		var item domain.ModelUsageBreakdownRow
		if err = rows.Scan(&item.AgentID, &item.AgentKey, &item.CallCount, &item.InputTokens, &item.OutputTokens, &item.TotalTokens, &item.TotalCostMicroUSD, &item.AvgLatencyMS, &item.AvgTokensPerSecond, &item.SuccessRate); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) ListModelUsageEvents(query domain.ModelUsageQuery) ([]domain.ModelTokenUsageEvent, error) {
	where, args := usageWhere(query)
	args = append(args, usageLimit(query.Limit))
	rows, err := r.db.Query(
		`SELECT id, occurred_at, date_key, hour_key, workspace_id, user_id, team_id, agent_id, agent_key, session_id, message_id, request_id,
		 provider_code, provider_type, provider_display_name, model_api_id, model_display_name, model_category_json, usage_kind, call_count,
		 input_tokens, output_tokens, cached_input_tokens, reasoning_tokens, embedding_tokens, total_tokens,
		 input_price_micro_usd_per_1k, output_price_micro_usd_per_1k, cached_input_price_micro_usd_per_1k, reasoning_price_micro_usd_per_1k, embedding_price_micro_usd_per_1k,
		 input_cost_micro_usd, output_cost_micro_usd, cached_input_cost_micro_usd, reasoning_cost_micro_usd, embedding_cost_micro_usd, total_cost_micro_usd,
		 latency_ms, time_to_first_token_ms, tokens_per_second, status, error_code, error_message, retry_count,
		 prompt_mode, max_output_tokens, context_window_k, stream_enabled, metadata_json, created_at
		 FROM model_token_usage_events`+where+` ORDER BY occurred_at DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ModelTokenUsageEvent{}
	for rows.Next() {
		event, err := scanModelTokenUsageEvent(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) ListChatOptions(optionType string) ([]domain.ChatOption, error) {
	query := `SELECT type, key, label, enabled, sort_order, metadata_json FROM chat_options WHERE enabled = 1`
	args := []any{}
	if strings.TrimSpace(optionType) != "" {
		query += ` AND type = ?`
		args = append(args, optionType)
	}
	query += ` ORDER BY type ASC, sort_order ASC`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.ChatOption
	for rows.Next() {
		var v domain.ChatOption
		if err = rows.Scan(&v.Type, &v.Key, &v.Label, &v.Enabled, &v.SortOrder, &v.MetadataJSON); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) AddAuditLog(l domain.AuditLog) error {
	if l.ID == "" {
		return errors.New("id is required")
	}
	if l.CreatedAt == "" {
		l.CreatedAt = nowISO()
	}
	_, err := r.db.Exec(
		`INSERT INTO audit_logs(id, action, resource, resource_id, request_id, detail, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		l.ID, l.Action, l.Resource, l.ResourceID, l.RequestID, l.Detail, l.CreatedAt,
	)
	return err
}

func (r *SQLiteRepository) ListAuditLogs(limit int) ([]domain.AuditLog, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.Query(
		`SELECT id, action, resource, resource_id, request_id, detail, created_at
		 FROM audit_logs ORDER BY created_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.AuditLog
	for rows.Next() {
		var v domain.AuditLog
		if err = rows.Scan(&v.ID, &v.Action, &v.Resource, &v.ResourceID, &v.RequestID, &v.Detail, &v.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAgent(row scanner) (domain.Agent, error) {
	var v domain.Agent
	err := row.Scan(&v.ID, &v.AgentKey, &v.DisplayName, &v.Provider, &v.Model, &v.Status, &v.IsDefault, &v.IsFavorite, &v.Icon, &v.AgentDescription, &v.CategoryPositionID, &v.SystemPromptMode, &v.ContextWindow, &v.BudgetMonthlyCents, &v.ConfigJSON, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt)
	return v, err
}

func scanAgents(rows *sql.Rows) ([]domain.Agent, error) {
	var result []domain.Agent
	for rows.Next() {
		v, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func sessionSelectSQL() string {
	return `SELECT id, owner_type, agent_id, team_id, title, summary, context_used_ratio, max_context_used_ratio, context_status,
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
	err := row.Scan(&v.ID, &v.OwnerType, &v.AgentID, &v.TeamID, &v.Title, &v.Summary, &v.ContextUsedRatio, &v.MaxContextUsedRatio, &v.ContextStatus, &v.DialogMode, &v.Provider, &v.Model, &v.Status, &v.MessageCount, &v.RunCount, &v.ModelCallCount, &v.ToolCallCount, &v.SkillCallCount, &v.MCPCallCount, &v.InputTokens, &v.OutputTokens, &v.TotalTokens, &v.TotalCostMicroUSD, &v.LastMessageAt, &v.CreatedAt, &v.UpdatedAt, &v.ArchivedAt, &v.DeletedAt)
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

func scanModelPricingRule(row scanner) (domain.ModelPricingRule, error) {
	var v domain.ModelPricingRule
	err := row.Scan(&v.ID, &v.ProviderCode, &v.ModelAPIID, &v.Currency, &v.InputPriceMicroUSDPer1K, &v.OutputPriceMicroUSDPer1K, &v.CachedInputPriceMicroUSDPer1K, &v.ReasoningPriceMicroUSDPer1K, &v.EmbeddingPriceMicroUSDPer1K, &v.EffectiveFrom, &v.EffectiveTo, &v.IsActive, &v.Source, &v.MetadataJSON, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

func scanModelUsageSummary(row scanner) (domain.ModelUsageSummary, error) {
	var v domain.ModelUsageSummary
	err := row.Scan(&v.CallCount, &v.RequestCount, &v.SuccessCount, &v.FailedCount, &v.CancelledCount, &v.InputTokens, &v.OutputTokens, &v.TotalTokens, &v.TotalCostMicroUSD, &v.AvgLatencyMS, &v.AvgTokensPerSecond)
	if v.RequestCount > 0 {
		v.SuccessRate = float64(v.SuccessCount) / float64(v.RequestCount)
	}
	return v, err
}

func scanModelTokenUsageEvent(row scanner) (domain.ModelTokenUsageEvent, error) {
	var v domain.ModelTokenUsageEvent
	var streamEnabled int
	err := row.Scan(
		&v.ID, &v.OccurredAt, &v.DateKey, &v.HourKey, &v.WorkspaceID, &v.UserID, &v.TeamID, &v.AgentID, &v.AgentKey, &v.SessionID, &v.MessageID, &v.RequestID,
		&v.ProviderCode, &v.ProviderType, &v.ProviderDisplayName, &v.ModelAPIID, &v.ModelDisplayName, &v.ModelCategoryJSON, &v.UsageKind, &v.CallCount,
		&v.InputTokens, &v.OutputTokens, &v.CachedInputTokens, &v.ReasoningTokens, &v.EmbeddingTokens, &v.TotalTokens,
		&v.InputPriceMicroUSDPer1K, &v.OutputPriceMicroUSDPer1K, &v.CachedInputPriceMicroUSDPer1K, &v.ReasoningPriceMicroUSDPer1K, &v.EmbeddingPriceMicroUSDPer1K,
		&v.InputCostMicroUSD, &v.OutputCostMicroUSD, &v.CachedInputCostMicroUSD, &v.ReasoningCostMicroUSD, &v.EmbeddingCostMicroUSD, &v.TotalCostMicroUSD,
		&v.LatencyMS, &v.TimeToFirstTokenMS, &v.TokensPerSecond, &v.Status, &v.ErrorCode, &v.ErrorMessage, &v.RetryCount,
		&v.PromptMode, &v.MaxOutputTokens, &v.ContextWindowK, &streamEnabled, &v.MetadataJSON, &v.CreatedAt,
	)
	v.StreamEnabled = streamEnabled != 0
	return v, err
}

func usageWhere(query domain.ModelUsageQuery) (string, []any) {
	parts := []string{}
	args := []any{}
	if query.StartDate != "" {
		parts = append(parts, "date_key >= ?")
		args = append(args, query.StartDate)
	}
	if query.EndDate != "" {
		parts = append(parts, "date_key <= ?")
		args = append(args, query.EndDate)
	}
	if query.ProviderCode != "" {
		parts = append(parts, "provider_code = ?")
		args = append(args, query.ProviderCode)
	}
	if query.ModelAPIID != "" {
		parts = append(parts, "model_api_id = ?")
		args = append(args, query.ModelAPIID)
	}
	if query.AgentID != "" {
		parts = append(parts, "agent_id = ?")
		args = append(args, query.AgentID)
	}
	if query.Status != "" {
		if query.Status == "abnormal" {
			parts = append(parts, "status <> 'success'")
		} else {
			parts = append(parts, "status = ?")
			args = append(args, query.Status)
		}
	}
	if len(parts) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

func usageLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func scanPlatformResource(resource string, row scanner) (domain.PlatformResource, error) {
	var v domain.PlatformResource
	v.Resource = resource
	err := row.Scan(&v.ID, &v.Key, &v.Name, &v.Description, &v.Status, &v.Enabled, &v.SortOrder, &v.ParentID, &v.Level, &v.AgentID, &v.Provider, &v.Model, &v.ConfigJSON, &v.MetadataJSON, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt)
	return v, err
}

func scanPlatformRows(resource string, rows *sql.Rows) ([]domain.PlatformResource, error) {
	var result []domain.PlatformResource
	for rows.Next() {
		v, err := scanPlatformResource(resource, rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func pluginSelectSQL() string {
	return `SELECT id, plugin_key, name, description, category, risk_level, enabled, scope, callback_points_json, sort_order, config_schema_json, config_json, default_config_json, invoke_count, block_count, error_count, last_invoked_at, last_status, created_at, updated_at FROM plugins`
}

func scanPlugins(rows *sql.Rows) ([]domain.Plugin, error) {
	items := []domain.Plugin{}
	for rows.Next() {
		var item domain.Plugin
		var callbackJSON string
		if err := rows.Scan(
			&item.ID, &item.Key, &item.Name, &item.Description, &item.Category, &item.RiskLevel, &item.Enabled, &item.Scope,
			&callbackJSON, &item.SortOrder, &item.ConfigSchemaJSON, &item.ConfigJSON, &item.DefaultConfigJSON,
			&item.InvokeCount, &item.BlockCount, &item.ErrorCount, &item.LastInvokedAt, &item.LastStatus, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(callbackJSON), &item.CallbackPoints)
		item.Permissions = domain.PluginPermissions{CanView: true, CanToggle: true, CanEditConfig: true, CanViewLogs: true}
		items = append(items, item)
	}
	return items, rows.Err()
}

func normalizeJSONList(value string) string {
	if strings.TrimSpace(value) == "" {
		return "[]"
	}
	if json.Valid([]byte(value)) {
		return value
	}
	encoded, err := json.Marshal([]string{value})
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func sanitizePromptFileID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, value)
	return strings.Trim(value, "_")
}
