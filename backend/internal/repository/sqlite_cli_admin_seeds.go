package repository

import "arenea/backend/internal/domain"

// cliAdminToolSeeds enumerates the `cli_admin_*` toolkit that backs the
// system administrator agent (see aranea/docs/25 cli.md §6.2). Each
// entry is a thin wrapper around an existing /api/v1 endpoint so tool
// invocations route through the same service layer (and thus the same
// audit / permission / plugin chain) as the Web console. The metadata
// JSON pins the HTTP binding, the risk class and the cli_admin group so
// downstream executors can dispatch without hard-coding a Go switch.
var cliAdminToolSeeds = []domain.Tool{
	// ---- Skill ------------------------------------------------------
	{
		Key: "cli_admin_skill_list", DisplayName: "Skill 列表", Description: "搜索 / 分页查询 Skill。",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"},"enabled":{"type":"string","enum":["true","false"]},"limit":{"type":"integer","default":20},"offset":{"type":"integer","default":0}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/skills"),
	},
	{
		Key: "cli_admin_skill_install_from_url", DisplayName: "Skill 远程安装",
		Description:          "从 GitHub / GitLab / 远程 zip URL 拉取 Skill，本地预校验后调用 import + apply 入库。",
		Category:             "system",
		RiskLevel:            "high",
		Enabled:              true,
		Readonly:             true,
		RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","required":["url"],"properties":{"url":{"type":"string","format":"uri"},"ref":{"type":"string"},"subpath":{"type":"string"},"name":{"type":"string"},"enable":{"type":"boolean","default":false},"publish":{"type":"boolean","default":false},"decision":{"type":"string","enum":["ask","skip","keep","refine"],"default":"ask"},"refine_provider":{"type":"string"},"refine_model":{"type":"string"},"refine_instructions":{"type":"string"},"dry_run":{"type":"boolean","default":false},"idempotency_key":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("high", "POST", "/api/v1/skills/import"),
	},
	{
		Key: "cli_admin_skill_install_from_path", DisplayName: "Skill 本地 zip 安装",
		Description:          "上传本地 zip 包到 /api/v1/skills/import 并完成同样的轮询 + apply 流程。",
		Category:             "system",
		RiskLevel:            "high",
		Enabled:              true,
		Readonly:             true,
		RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","required":["path"],"properties":{"path":{"type":"string"},"name":{"type":"string"},"enable":{"type":"boolean","default":false},"publish":{"type":"boolean","default":false},"decision":{"type":"string","enum":["ask","skip","keep","refine"],"default":"ask"},"dry_run":{"type":"boolean","default":false},"idempotency_key":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("high", "POST", "/api/v1/skills/import"),
	},
	{
		Key: "cli_admin_skill_import_status", DisplayName: "Skill 导入状态",
		Description:          "查询导入任务的最新状态、候选与冲突组。",
		Category:             "system",
		RiskLevel:            "low",
		Enabled:              true,
		Readonly:             true,
		ParametersSchemaJSON: `{"type":"object","required":["job_id"],"properties":{"job_id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/skills/import/{job_id}"),
	},
	{
		Key: "cli_admin_skill_import_apply", DisplayName: "Skill 导入应用",
		Description:          "把决策（create / skip / keep_existing / keep_incoming / merge）发给 /apply 入库。",
		Category:             "system",
		RiskLevel:            "high",
		Enabled:              true,
		Readonly:             true,
		RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","required":["job_id","decisions"],"properties":{"job_id":{"type":"string"},"decisions":{"type":"array","items":{"type":"object"}}}}`,
		MetadataJSON:         cliAdminMeta("high", "POST", "/api/v1/skills/import/{job_id}/apply"),
	},
	{
		Key: "cli_admin_skill_refine_conflict", DisplayName: "Skill 冲突 AI 炼化",
		Description:          "用 AI 把一个冲突组中的多份 Skill 合并成一个新的 candidate。",
		Category:             "system",
		RiskLevel:            "medium",
		Enabled:              true,
		Readonly:             true,
		ParametersSchemaJSON: `{"type":"object","required":["job_id","group_id"],"properties":{"job_id":{"type":"string"},"group_id":{"type":"string"},"instructions":{"type":"string"},"provider":{"type":"string"},"model":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "POST", "/api/v1/skills/import/{job_id}/conflict-groups/{group_id}/refine"),
	},
	{
		Key: "cli_admin_skill_enable", DisplayName: "Skill 启用",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/skills/{id}/enabled"),
	},
	{
		Key: "cli_admin_skill_disable", DisplayName: "Skill 停用",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/skills/{id}/enabled"),
	},
	{
		Key: "cli_admin_skill_delete", DisplayName: "Skill 删除",
		Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("high", "DELETE", "/api/v1/skills/{id}"),
	},

	// ---- Agent ------------------------------------------------------
	{
		Key: "cli_admin_agent_list", DisplayName: "Agent 列表",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"},"status":{"type":"string"},"provider":{"type":"string"},"limit":{"type":"integer"},"offset":{"type":"integer"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/agents"),
	},
	{
		Key: "cli_admin_agent_get", DisplayName: "Agent 详情",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string","description":"agent id 或 agent_key"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/agents/{id}"),
	},
	{
		Key: "cli_admin_agent_create", DisplayName: "Agent 创建",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["agent_key","display_name"],"properties":{"agent_key":{"type":"string"},"display_name":{"type":"string"},"provider":{"type":"string"},"model":{"type":"string"},"agent_description":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "POST", "/api/v1/agents"),
	},
	{
		Key: "cli_admin_agent_update", DisplayName: "Agent 更新",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"patch":{"type":"object"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/agents/{id}"),
	},
	{
		Key: "cli_admin_agent_delete", DisplayName: "Agent 删除",
		Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("high", "DELETE", "/api/v1/agents/{id}"),
	},
	{
		Key: "cli_admin_agent_tools_get", DisplayName: "Agent 工具策略读取",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["agent_id"],"properties":{"agent_id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/agents/{id}/tools/effective"),
	},
	{
		Key: "cli_admin_agent_tools_set", DisplayName: "Agent 工具策略修改",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["agent_id"],"properties":{"agent_id":{"type":"string"},"tools_enabled":{"type":"boolean"},"tools_profile":{"type":"string"},"tools_allow":{"type":"array","items":{"type":"string"}},"tools_deny":{"type":"array","items":{"type":"string"}},"tools_concurrent_allow":{"type":"array","items":{"type":"string"}},"dry_run":{"type":"boolean","default":false}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/agents/{id}/tools/policy"),
	},

	// ---- Team -------------------------------------------------------
	{
		Key: "cli_admin_team_list", DisplayName: "Team 列表",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"},"limit":{"type":"integer"},"offset":{"type":"integer"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/teams"),
	},
	{
		Key: "cli_admin_team_create", DisplayName: "Team 创建",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["team_key","display_name"],"properties":{"team_key":{"type":"string"},"display_name":{"type":"string"},"agents":{"type":"array","items":{"type":"string"}}}}`,
		MetadataJSON:         cliAdminMeta("medium", "POST", "/api/v1/teams"),
	},
	{
		Key: "cli_admin_team_update", DisplayName: "Team 更新",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"patch":{"type":"object"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/teams/{id}"),
	},
	{
		Key: "cli_admin_team_delete", DisplayName: "Team 删除",
		Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("high", "DELETE", "/api/v1/teams/{id}"),
	},
	{
		Key: "cli_admin_team_run", DisplayName: "Team 触发执行",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["team_id","message"],"properties":{"team_id":{"type":"string"},"message":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "POST", "/api/v1/team-runs"),
	},

	// ---- Tool -------------------------------------------------------
	{
		Key: "cli_admin_tool_list", DisplayName: "Tool 列表",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"},"category":{"type":"string"},"enabled":{"type":"string","enum":["true","false"]},"limit":{"type":"integer"},"offset":{"type":"integer"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/tools"),
	},
	{
		Key: "cli_admin_tool_enable", DisplayName: "Tool 启用",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/tools/{id}/enabled"),
	},
	{
		Key: "cli_admin_tool_disable", DisplayName: "Tool 停用",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/tools/{id}/enabled"),
	},
	{
		Key: "cli_admin_tool_config_set", DisplayName: "Tool 配置写入",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id","config"],"properties":{"id":{"type":"string"},"config":{"type":"object"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/tools/{id}/config"),
	},

	// ---- Plugin -----------------------------------------------------
	{
		Key: "cli_admin_plugin_list", DisplayName: "Plugin 列表",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/plugins"),
	},
	{
		Key: "cli_admin_plugin_enable", DisplayName: "Plugin 启用",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/plugins/{id}"),
	},
	{
		Key: "cli_admin_plugin_disable", DisplayName: "Plugin 停用",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/plugins/{id}"),
	},
	{
		Key: "cli_admin_plugin_order_set", DisplayName: "Plugin 排序",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["entries"],"properties":{"entries":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"sort_order":{"type":"integer"}},"required":["id","sort_order"]}}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/plugins"),
	},
	{
		Key: "cli_admin_plugin_config_set", DisplayName: "Plugin 配置写入",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id","config"],"properties":{"id":{"type":"string"},"config":{"type":"object"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/plugins/{id}"),
	},

	// ---- MCP --------------------------------------------------------
	{
		Key: "cli_admin_mcp_list", DisplayName: "MCP Server 列表",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/mcp-servers"),
	},
	{
		Key: "cli_admin_mcp_add", DisplayName: "MCP Server 新增",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["name","transport"],"properties":{"name":{"type":"string"},"transport":{"type":"string","enum":["streamable_http","stdio","sse"]},"url":{"type":"string"},"headers":{"type":"object"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "POST", "/api/v1/mcp-servers"),
	},
	{
		Key: "cli_admin_mcp_update", DisplayName: "MCP Server 更新",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"patch":{"type":"object"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/mcp-servers/{id}"),
	},
	{
		Key: "cli_admin_mcp_delete", DisplayName: "MCP Server 删除",
		Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("high", "DELETE", "/api/v1/mcp-servers/{id}"),
	},
	{
		Key: "cli_admin_mcp_test", DisplayName: "MCP Server 连通性测试",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("low", "POST", "/api/v1/mcp-servers/{id}/test"),
	},

	// ---- Cron -------------------------------------------------------
	{
		Key: "cli_admin_cron_list", DisplayName: "Cron 任务列表",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/cron-tasks"),
	},
	{
		Key: "cli_admin_cron_add", DisplayName: "Cron 任务新增",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["name","schedule_expr"],"properties":{"name":{"type":"string"},"schedule_expr":{"type":"string"},"agent_key":{"type":"string"},"message":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "POST", "/api/v1/cron-tasks"),
	},
	{
		Key: "cli_admin_cron_update", DisplayName: "Cron 任务更新",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"patch":{"type":"object"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/cron-tasks/{id}"),
	},
	{
		Key: "cli_admin_cron_delete", DisplayName: "Cron 任务删除",
		Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("high", "DELETE", "/api/v1/cron-tasks/{id}"),
	},
	{
		Key: "cli_admin_cron_pause", DisplayName: "Cron 任务暂停",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/cron-tasks/{id}"),
	},
	{
		Key: "cli_admin_cron_resume", DisplayName: "Cron 任务恢复",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/cron-tasks/{id}"),
	},
	{
		Key: "cli_admin_cron_trigger", DisplayName: "Cron 任务立即触发",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "POST", "/api/v1/cron-tasks/{id}/trigger"),
	},

	// ---- Channel ----------------------------------------------------
	{
		Key: "cli_admin_channel_list", DisplayName: "Channel 列表",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/channels"),
	},
	{
		Key: "cli_admin_channel_add", DisplayName: "Channel 新增",
		Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","required":["name","kind"],"properties":{"name":{"type":"string"},"kind":{"type":"string"},"config":{"type":"object"}}}`,
		MetadataJSON:         cliAdminMeta("high", "POST", "/api/v1/channels"),
	},
	{
		Key: "cli_admin_channel_update", DisplayName: "Channel 更新",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"patch":{"type":"object"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/channels/{id}"),
	},
	{
		Key: "cli_admin_channel_delete", DisplayName: "Channel 删除",
		Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("high", "DELETE", "/api/v1/channels/{id}"),
	},
	{
		Key: "cli_admin_channel_test", DisplayName: "Channel 连通性测试",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("low", "POST", "/api/v1/channels/{id}/test"),
	},
	{
		Key: "cli_admin_channel_send", DisplayName: "Channel 推送消息",
		Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","required":["id","text"],"properties":{"id":{"type":"string"},"text":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("high", "POST", "/api/v1/channels/{id}/send"),
	},

	// ---- Provider ---------------------------------------------------
	{
		Key: "cli_admin_provider_list", DisplayName: "LLM Provider 列表",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/llm-provider-models"),
	},
	{
		Key: "cli_admin_provider_add", DisplayName: "LLM Provider 新增",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["provider","model"],"properties":{"provider":{"type":"string"},"model":{"type":"string"},"name":{"type":"string"},"config":{"type":"object"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "POST", "/api/v1/llm-provider-models"),
	},
	{
		Key: "cli_admin_provider_update", DisplayName: "LLM Provider 更新",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"patch":{"type":"object"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "PATCH", "/api/v1/llm-provider-models/{id}"),
	},
	{
		Key: "cli_admin_provider_delete", DisplayName: "LLM Provider 删除",
		Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("high", "DELETE", "/api/v1/llm-provider-models/{id}"),
	},
	{
		Key: "cli_admin_provider_inspect", DisplayName: "LLM Provider 自检",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("low", "POST", "/api/v1/llm-provider-models/inspect"),
	},

	// ---- Session ----------------------------------------------------
	{
		Key: "cli_admin_session_list", DisplayName: "Session 列表",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"agent_id":{"type":"string"},"limit":{"type":"integer"},"offset":{"type":"integer"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/sessions"),
	},
	{
		Key: "cli_admin_session_get", DisplayName: "Session 详情",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/sessions/{id}"),
	},
	{
		Key: "cli_admin_session_send", DisplayName: "Session 发送消息",
		Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["agent_key","content"],"properties":{"session_id":{"type":"string"},"agent_key":{"type":"string"},"content":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "POST", "/api/v1/chat/messages"),
	},

	// ---- Monitor ----------------------------------------------------
	{
		Key: "cli_admin_monitor_audit", DisplayName: "审计日志查询",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"kind":{"type":"string"},"actor":{"type":"string"},"from":{"type":"string"},"to":{"type":"string"},"limit":{"type":"integer"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/monitor/audit"),
	},
	{
		Key: "cli_admin_monitor_events", DisplayName: "运行事件查询",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"agent_id":{"type":"string"},"from":{"type":"string"},"to":{"type":"string"},"limit":{"type":"integer"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/monitor/events"),
	},
	{
		Key: "cli_admin_monitor_traces", DisplayName: "Trace 列表",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"agent_id":{"type":"string"},"from":{"type":"string"},"to":{"type":"string"},"limit":{"type":"integer"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/monitor/traces"),
	},
	{
		Key: "cli_admin_monitor_usage_overview", DisplayName: "Token 用量概览",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"from":{"type":"string"},"to":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/model-usage/overview"),
	},

	// ---- System -----------------------------------------------------
	{
		Key: "cli_admin_system_health", DisplayName: "后端健康检查",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","properties":{}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/healthz"),
	},
	{
		Key: "cli_admin_system_backup", DisplayName: "系统备份",
		Category: "system", RiskLevel: "high", Enabled: false, Readonly: true, RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"include_skills":{"type":"boolean","default":true}}}`,
		MetadataJSON:         cliAdminMeta("high", "POST", "/api/v1/system/backup"),
	},
	{
		Key: "cli_admin_system_restore", DisplayName: "系统恢复",
		Category: "system", RiskLevel: "high", Enabled: false, Readonly: true, RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","required":["snapshot_id"],"properties":{"snapshot_id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("high", "POST", "/api/v1/system/restore"),
	},
}

// cliAdminMeta produces the standard metadata JSON for a CLI admin tool.
// Keeping the function next to the seed table makes it obvious which
// fields downstream executors can rely on (group, risk, http binding).
func cliAdminMeta(risk, method, path string) string {
	return `{"group":"cli_admin","cli_admin":true,"risk":"` + risk + `","http":{"method":"` + method + `","path":"` + path + `"}}`
}

// CLIAdminToolKeys returns the deterministic list of cli_admin_* tool
// keys used to populate the system_admin agent's tools_allow_json and
// to seed the `cli_admin` tool group consumed by the policy resolver.
func CLIAdminToolKeys() []string {
	keys := make([]string, len(cliAdminToolSeeds))
	for i, t := range cliAdminToolSeeds {
		keys[i] = t.Key
	}
	return keys
}
