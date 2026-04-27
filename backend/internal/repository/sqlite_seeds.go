package repository

import (
	"encoding/json"
	"strings"

	"arenea/backend/internal/domain"
)

// seedChatOptions 插入默认对话模式与模型提供方项，供聊天界面在零配置时即可渲染。
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

// seedPlatformDefaults 安装引导用 LLM 模型与系统智能体分类树。约束失败短路，已有行保持不变。
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

// seedBuiltinTools 对系统内置工具集做 upsert，新部署无需手工接入即可有可用工具目录。
// 此处同时追加 cli_admin_* 工具集（aranea/docs/25 cli.md §6），每次启动后 tools 表形态确定，与是否新发布无关。
func (r *SQLiteRepository) seedBuiltinTools() error {
	now := nowISO()
	allSeeds := make([]domain.Tool, 0, len(builtinToolSeeds)+len(cliAdminToolSeeds))
	allSeeds = append(allSeeds, builtinToolSeeds...)
	allSeeds = append(allSeeds, cliAdminToolSeeds...)
	for _, row := range allSeeds {
		applyBuiltinToolDefaults(&row)
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

// applyBuiltinToolDefaults 补全内置工具种子的必填字段，使 upsert 每列均有合理值。
func applyBuiltinToolDefaults(row *domain.Tool) {
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
}

// seedSystemAdminAgent 插入内置 `__system_admin__` 智能体，支撑 Aranea CLI 交互式 REPL（见 25 cli.md §1.2 与 §3）。
// 每次启动 upsert，新列或系统提示词修订自动生效，不干扰用户自定义智能体。
func (r *SQLiteRepository) seedSystemAdminAgent() error {
	const id = "agent_system_admin"
	const key = "__system_admin__"
	now := nowISO()

	systemPrompt := `你是 Aranea 平台的"系统管家" Agent，运行在命令行的交互式控制台中。

职责：
  * 帮助管理员通过自然语言完成 Skill / Agent / Tool / Plugin / MCP /
    定时任务 / 渠道 / 会话 / 监控等系统级操作。
  * 当用户描述"装一个 GitHub 上的 skill" 等需求时，回复一条可直接复制
    执行的 aranea 命令（例如 ` + "`aranea skill install <url>`" + `），
    并解释每一步的影响范围与回滚方式。
  * 涉及高风险动作（删除、提权、禁用核心插件等）时主动提示用户加上
    --yes 或在确认后再执行。

输出风格：先给一行结论，然后用简短的步骤说明，最后附可执行命令。`

	configJSON := `{"system_prompt":` + jsonString(systemPrompt) + `,"is_system":true,"readonly":true,"kind":"system_admin"}`

	_, err := r.db.Exec(
		`INSERT INTO agents(
		   id, agent_key, display_name, provider, model, status, is_default, is_favorite, icon, agent_description,
		   category_position_id, system_prompt_mode, context_window, budget_monthly_cents, config_json,
		   created_at, updated_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, 0, 1, ?, ?, '', 'manual', 16000, 0, ?, ?, ?, '')
		ON CONFLICT(agent_key) DO UPDATE SET
		   display_name      = excluded.display_name,
		   icon              = excluded.icon,
		   agent_description = excluded.agent_description,
		   config_json       = excluded.config_json,
		   updated_at        = excluded.updated_at,
		   deleted_at        = ''`,
		id, key, "系统管家", "openrouter", "gpt-4.1-mini", "active",
		"settings", "Aranea CLI 的内置 Agent，负责把自然语言转成系统级操作指令。", configJSON,
		now, now,
	)
	if err != nil {
		return err
	}
	return r.seedSystemAdminAgentSettings(id)
}

// seedSystemAdminAgentSettings 配置运行时策略，使系统管家可调用全部 cli_admin_* 及少量安全辅助（web_fetch、read_file、datetime）。
// 同时写入 `group:cli_admin` 与显式 key，无组展开时行为仍正确。允许/拒绝列表以 JSON 数组持久化，由 UpsertAgentRuntimeSettings 归一化保持稳定。
func (r *SQLiteRepository) seedSystemAdminAgentSettings(agentID string) error {
	allow := []string{"group:cli_admin", "web_fetch", "read_file", "datetime"}
	deny := []string{"shell_exec", "write_file", "edit_file", "create_image", "tts"}
	allowJSON, err := json.Marshal(allow)
	if err != nil {
		return err
	}
	denyJSON, err := json.Marshal(deny)
	if err != nil {
		return err
	}
	_, err = r.UpsertAgentRuntimeSettings(domain.AgentRuntimeSettings{
		AgentID:                           agentID,
		SubagentsEnabled:                  false,
		SubagentsMaxConcurrency:           4,
		SubagentsMaxGenerationDepth:       1,
		SubagentsMaxChildrenPerAgent:      2,
		SubagentsArchiveAfterMinutes:      60,
		SubagentsMaxRetries:               1,
		ToolsEnabled:                      true,
		ToolsProfile:                      "system_admin",
		ToolsAllowJSON:                    string(allowJSON),
		ToolsDenyJSON:                     string(denyJSON),
		ToolsConcurrentAllowJSON:          "[]",
		MemoryEnabled:                     false,
		MemoryMaxChunkLength:              1000,
		MemoryMaxResults:                  6,
		MemoryMinScore:                    0.35,
		HeartbeatEnabled:                  false,
		HeartbeatIntervalMinutes:          30,
		EvolutionSelfEvolve:               false,
		EvolutionSkillEvolve:              false,
		EvolutionMetricsEnabled:           true,
		EvolutionSuggestionsEnabled:       false,
		GuardrailMaxChangePerPeriod:       0.1,
		GuardrailMinDataPoints:            100,
		GuardrailRollbackOnDeclinePercent: 20,
	})
	return err
}

// jsonString 将 s 编码为带引号与转义的 JSON 字符串字面量，供仓库手工拼接 JSON 时内联。
func jsonString(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '"')
	for _, r := range s {
		switch r {
		case '"':
			out = append(out, '\\', '"')
		case '\\':
			out = append(out, '\\', '\\')
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		case '\t':
			out = append(out, '\\', 't')
		default:
			if r < 0x20 {
				out = append(out, []byte{'\\', 'u', '0', '0', hex(byte(r>>4)), hex(byte(r&0x0f))}...)
				continue
			}
			out = append(out, []byte(string(r))...)
		}
	}
	out = append(out, '"')
	return string(out)
}

func hex(b byte) byte {
	if b < 10 {
		return '0' + b
	}
	return 'a' + (b - 10)
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

	// L1 工作记忆工具（aranea/docs/13 memory-L1-working.md §5.4）。LLM 用其查看/修改当前任务的结构化字段。
	// 操作自动限定于当前 session + agent；调用方不能跨上下文操作任意任务。
	{ID: "tool_working_memory_read", Key: "working_memory.read", DisplayName: "工作记忆读取", Description: "读取当前任务的结构化工作记忆字段。不传 field_path 时返回整张快照。", Category: "memory", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"field_path":{"type":"string","description":"字段路径（可选）"}}}`},
	{ID: "tool_working_memory_list", Key: "working_memory.list", DisplayName: "工作记忆列表", Description: "列出当前任务下所有可见字段（path、preview、token_estimate、revision）。", Category: "memory", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"include_internal":{"type":"boolean","description":"是否包含 visibility=internal 字段"}}}`},
	{ID: "tool_working_memory_write", Key: "working_memory.write", DisplayName: "工作记忆写入", Description: "向当前任务写入或更新一个结构化字段。超过单字段 / 任务上限时返回 OVERFLOW，调用方应先精简或归档。", Category: "memory", Enabled: true, ParametersSchemaJSON: `{"type":"object","properties":{"field_path":{"type":"string"},"value":{"description":"任意 JSON 值"},"field_kind":{"type":"string","enum":["string","number","boolean","json","reference","markdown"]},"visibility":{"type":"string","enum":["prompt","internal","shared"]},"pin_to_prompt":{"type":"boolean"},"reason":{"type":"string"},"if_revision":{"type":"integer","description":"乐观锁，期望的当前 revision"}},"required":["field_path","value"]}`},
	{ID: "tool_working_memory_patch", Key: "working_memory.patch", DisplayName: "工作记忆批量补丁", Description: "一次写入多个字段。任意一项失败将中断后续写入并返回错误。", Category: "memory", Enabled: true, ParametersSchemaJSON: `{"type":"object","properties":{"patches":{"type":"array","items":{"type":"object","properties":{"field_path":{"type":"string"},"value":{},"field_kind":{"type":"string"},"visibility":{"type":"string"},"reason":{"type":"string"},"if_revision":{"type":"integer"}},"required":["field_path","value"]}}},"required":["patches"]}`},
	{ID: "tool_working_memory_delete", Key: "working_memory.delete", DisplayName: "工作记忆删除", Description: "删除当前任务下的一个字段（历史版本仍可回滚）。", Category: "memory", Enabled: true, ParametersSchemaJSON: `{"type":"object","properties":{"field_path":{"type":"string"}},"required":["field_path"]}`},
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
