package transport

import (
	"net/http"

	"arenea/backend/internal/service"
)

// Services 聚合 HTTP 层依赖的全部应用服务。使用单一结构体可在新增服务时
// 保持处理器构造函数稳定，并让调用方显式命名字段而非依赖位置参数。
type Services struct {
	Agent    *service.AgentService
	Team     *service.TeamService
	Session  *service.SessionService
	Chat     *service.ChatService
	Audit    *service.AuditService
	Platform *service.PlatformService
	Usage    *service.UsageService
	Skill    *service.SkillService
	Tool     *service.ToolService
	Plugin   *service.PluginService
	Channel  *service.ChannelService
}

type HTTPHandler struct {
	agentSvc    *service.AgentService
	teamSvc     *service.TeamService
	sessionSvc  *service.SessionService
	chatSvc     *service.ChatService
	auditSvc    *service.AuditService
	platformSvc *service.PlatformService
	usageSvc    *service.UsageService
	skillSvc    *service.SkillService
	toolSvc     *service.ToolService
	pluginSvc   *service.PluginService
	channelSvc  *service.ChannelService
}

// NewHTTPHandler 将所有应用路由挂到默认 mux 上。*HTTPHandler 作为 Services 的
// 内部具现，使各处理方法可按名称访问各依赖。
func NewHTTPHandler(svc Services) http.Handler {
	h := &HTTPHandler{
		agentSvc:    svc.Agent,
		teamSvc:     svc.Team,
		sessionSvc:  svc.Session,
		chatSvc:     svc.Chat,
		auditSvc:    svc.Audit,
		platformSvc: svc.Platform,
		usageSvc:    svc.Usage,
		skillSvc:    svc.Skill,
		toolSvc:     svc.Tool,
		pluginSvc:   svc.Plugin,
		channelSvc:  svc.Channel,
	}
	mux := http.NewServeMux()
	h.registerRoutes(mux)
	return mux
}

// registerRoutes 注册全部 API 路由。按聚合根分组，使相关端点在视觉上相邻、便于调整顺序。
func (h *HTTPHandler) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.healthz)

	// 智能体与团队。
	mux.HandleFunc("/api/v1/agents/validate-model", h.handleValidateModel)
	mux.HandleFunc("/api/v1/agents/", h.handleAgentByID)
	mux.HandleFunc("/api/v1/agents", h.handleAgents)
	mux.HandleFunc("/api/v1/teams/", h.handleTeamByID)
	mux.HandleFunc("/api/v1/teams", h.handleTeams)
	mux.HandleFunc("/api/v1/team-runs/", h.handleTeamRunByID)
	mux.HandleFunc("/api/v1/team-runs", h.handleTeamRuns)
	mux.HandleFunc("/api/v1/team-run-events", h.handleTeamRunEvents)

	// 平台资源。
	mux.HandleFunc("/api/v1/agent-categories/tree", h.handlePlatformTree("agent-categories"))
	mux.HandleFunc("/api/v1/agent-categories", h.handlePlatformCollection("agent-categories"))
	mux.HandleFunc("/api/v1/agent-categories/", h.handlePlatformItem("agent-categories", "/api/v1/agent-categories/"))
	mux.HandleFunc("/api/v1/llm-provider-models/inspect", h.handleInspectProviderModel)
	mux.HandleFunc("/api/v1/llm-provider-models", h.handlePlatformCollection("llm-provider-models"))
	mux.HandleFunc("/api/v1/llm-provider-models/", h.handlePlatformItem("llm-provider-models", "/api/v1/llm-provider-models/"))
	mux.HandleFunc("/api/v1/avatar-assets", h.handleAvatarAssets)
	mux.HandleFunc("/api/v1/avatar-assets/", h.handleAvatarAssetByID)
	mux.HandleFunc("/api/v1/hooks", h.handlePlatformCollection("hooks"))
	mux.HandleFunc("/api/v1/hooks/", h.handlePlatformItem("hooks", "/api/v1/hooks/"))
	mux.HandleFunc("/api/v1/mcp-servers", h.handlePlatformCollection("mcp-servers"))
	mux.HandleFunc("/api/v1/mcp-servers/", h.handlePlatformItem("mcp-servers", "/api/v1/mcp-servers/"))

	// 频道。
	mux.HandleFunc("/api/v1/channels/catalog", h.handleChannelCatalog)
	mux.HandleFunc("/api/v1/channels", h.handleChannels)
	mux.HandleFunc("/api/v1/channels/", h.handleChannelByID)

	// 技能与工具。
	mux.HandleFunc("/api/v1/skills/import", h.handleSkillImport)
	mux.HandleFunc("/api/v1/skills/import/", h.handleSkillImportByID)
	mux.HandleFunc("/api/v1/skills", h.handleSkills)
	mux.HandleFunc("/api/v1/skills/", h.handleSkillByID)
	mux.HandleFunc("/api/v1/skill-runs", h.handleSkillRuns)
	mux.HandleFunc("/api/v1/tools/runs", h.handleToolRuns)
	mux.HandleFunc("/api/v1/tools", h.handleTools)
	mux.HandleFunc("/api/v1/tools/", h.handleToolByID)

	// 插件与定时任务。
	mux.HandleFunc("/api/v1/plugins", h.handlePlugins)
	mux.HandleFunc("/api/v1/plugins/", h.handlePluginByID)
	mux.HandleFunc("/api/v1/cron-tasks", h.handlePlatformCollection("cron-tasks"))
	mux.HandleFunc("/api/v1/cron-tasks/", h.handlePlatformItem("cron-tasks", "/api/v1/cron-tasks/"))
	mux.HandleFunc("/api/v1/cron-task-runs", h.handleCronTaskRuns)

	// 会话与聊天。
	mux.HandleFunc("/api/v1/sessions", h.handleSessions)
	mux.HandleFunc("/api/v1/sessions/", h.handleSessionByID)
	mux.HandleFunc("/api/v1/chat/messages/stream", h.handleChatMessagesStream)
	mux.HandleFunc("/api/v1/chat/messages", h.handleChatMessages)
	mux.HandleFunc("/api/v1/chat/options", h.handleChatOptions)

	// 记忆 L0（感知 / 上下文窗口）调试接口。
	mux.HandleFunc("/api/v1/l0/preview", h.handleL0Preview)
	mux.HandleFunc("/api/v1/l0/snapshots/", h.handleL0SnapshotByID)

	// 记忆 L1（工作记忆）模式管理。按任务 / 按字段的路由与会话绑定，由 handleSessionByID 分发。
	h.registerMemoryL1Routes(mux)

	// 记忆 L2（情景记忆）管理端路由。按会话的事件 / 片段 / 标记 / 回忆由 handleSessionByID 分发。
	h.registerMemoryL2AdminRoutes(mux)

	// 记忆 L3（语义记忆）路由。事实按工作区 / 用户等作用域，非会话作用域，故挂在 /api/v1/memory/l3/。
	h.registerMemoryL3Routes(mux)

	// 记忆 L4（持久 / 知识图谱）路由与智能体自进化接口。二者均为工作区 / 用户 / 智能体作用域，
	// 分别位于 /api/v1/memory/l4/ 与 /api/v1/agent-evolution/。
	h.registerMemoryL4Routes(mux)
	h.registerAgentEvolutionRoutes(mux)

	// 模型用量分析。
	mux.HandleFunc("/api/v1/model-usage/overview", h.handleModelUsageOverview)
	mux.HandleFunc("/api/v1/model-usage/trends", h.handleModelUsageTrends)
	mux.HandleFunc("/api/v1/model-usage/top-models", h.handleModelUsageTopModels)
	mux.HandleFunc("/api/v1/model-usage/top-agents", h.handleModelUsageTopAgents)
	mux.HandleFunc("/api/v1/model-usage/events", h.handleModelUsageEvents)

	// 监控 / 可观测性。
	mux.HandleFunc("/api/v1/monitor/logs/stream", h.handleMonitorLogStream)
	mux.HandleFunc("/api/v1/monitor/logs", h.handleMonitorLogs)
	mux.HandleFunc("/api/v1/monitor/events", h.handleMonitorEvents)
	mux.HandleFunc("/api/v1/monitor/events/", h.handleMonitorEventByID)
	mux.HandleFunc("/api/v1/monitor/traces", h.handleMonitorTraces)
	mux.HandleFunc("/api/v1/monitor/traces/", h.handleMonitorTraceByID)
	mux.HandleFunc("/api/v1/monitor/audit", h.handleMonitorAudit)
}

func (h *HTTPHandler) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
