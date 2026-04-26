package transport

import (
	"net/http"

	"arenea/backend/internal/service"
)

// Services bundles every application service the HTTP layer depends on. Using
// a single struct keeps the handler constructor stable as new services are
// introduced and lets call sites name fields explicitly instead of relying on
// positional arguments.
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

// NewHTTPHandler wires every application route onto the default mux. The
// concrete *HTTPHandler is kept as an internal alias of Services so individual
// handler methods can address each dependency by name.
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

// registerRoutes installs every API route. Routes are grouped by aggregate so
// related endpoints stay visually adjacent and reordering is straightforward.
func (h *HTTPHandler) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.healthz)

	// Agents & teams.
	mux.HandleFunc("/api/v1/agents/validate-model", h.handleValidateModel)
	mux.HandleFunc("/api/v1/agents/", h.handleAgentByID)
	mux.HandleFunc("/api/v1/agents", h.handleAgents)
	mux.HandleFunc("/api/v1/teams/", h.handleTeamByID)
	mux.HandleFunc("/api/v1/teams", h.handleTeams)
	mux.HandleFunc("/api/v1/team-runs/", h.handleTeamRunByID)
	mux.HandleFunc("/api/v1/team-runs", h.handleTeamRuns)
	mux.HandleFunc("/api/v1/team-run-events", h.handleTeamRunEvents)

	// Platform resources.
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

	// Channels.
	mux.HandleFunc("/api/v1/channels/catalog", h.handleChannelCatalog)
	mux.HandleFunc("/api/v1/channels", h.handleChannels)
	mux.HandleFunc("/api/v1/channels/", h.handleChannelByID)

	// Skills & tools.
	mux.HandleFunc("/api/v1/skills/import", h.handleSkillImport)
	mux.HandleFunc("/api/v1/skills/import/", h.handleSkillImportByID)
	mux.HandleFunc("/api/v1/skills", h.handleSkills)
	mux.HandleFunc("/api/v1/skills/", h.handleSkillByID)
	mux.HandleFunc("/api/v1/skill-runs", h.handleSkillRuns)
	mux.HandleFunc("/api/v1/tools/runs", h.handleToolRuns)
	mux.HandleFunc("/api/v1/tools", h.handleTools)
	mux.HandleFunc("/api/v1/tools/", h.handleToolByID)

	// Plugins & cron.
	mux.HandleFunc("/api/v1/plugins", h.handlePlugins)
	mux.HandleFunc("/api/v1/plugins/", h.handlePluginByID)
	mux.HandleFunc("/api/v1/cron-tasks", h.handlePlatformCollection("cron-tasks"))
	mux.HandleFunc("/api/v1/cron-tasks/", h.handlePlatformItem("cron-tasks", "/api/v1/cron-tasks/"))
	mux.HandleFunc("/api/v1/cron-task-runs", h.handleCronTaskRuns)

	// Sessions & chat.
	mux.HandleFunc("/api/v1/sessions", h.handleSessions)
	mux.HandleFunc("/api/v1/sessions/", h.handleSessionByID)
	mux.HandleFunc("/api/v1/chat/messages/stream", h.handleChatMessagesStream)
	mux.HandleFunc("/api/v1/chat/messages", h.handleChatMessages)
	mux.HandleFunc("/api/v1/chat/options", h.handleChatOptions)

	// Memory L0 (sensory / context-window) debug surface.
	mux.HandleFunc("/api/v1/l0/preview", h.handleL0Preview)
	mux.HandleFunc("/api/v1/l0/snapshots/", h.handleL0SnapshotByID)

	// Memory L1 (working memory) schema management. Per-task / per-field
	// routes are session-scoped and dispatched from handleSessionByID.
	h.registerMemoryL1Routes(mux)

	// Memory L2 (episodic memory) admin routes. Per-session events /
	// episodes / marks / recall are dispatched from handleSessionByID.
	h.registerMemoryL2AdminRoutes(mux)

	// Memory L3 (semantic memory) routes. Facts are workspace-/user-
	// scoped, not session-scoped, so they live under /api/v1/memory/l3/.
	h.registerMemoryL3Routes(mux)

	// Memory L4 (persistent / knowledge graph) routes and agent
	// self-evolution surface. Both are workspace- / user- / agent-
	// scoped and live under /api/v1/memory/l4/ and
	// /api/v1/agent-evolution/ respectively.
	h.registerMemoryL4Routes(mux)
	h.registerAgentEvolutionRoutes(mux)

	// Model usage analytics.
	mux.HandleFunc("/api/v1/model-usage/overview", h.handleModelUsageOverview)
	mux.HandleFunc("/api/v1/model-usage/trends", h.handleModelUsageTrends)
	mux.HandleFunc("/api/v1/model-usage/top-models", h.handleModelUsageTopModels)
	mux.HandleFunc("/api/v1/model-usage/top-agents", h.handleModelUsageTopAgents)
	mux.HandleFunc("/api/v1/model-usage/events", h.handleModelUsageEvents)

	// Monitor / observability.
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
