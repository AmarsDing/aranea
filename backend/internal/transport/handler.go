package transport

import (
	"net/http"

	"arenea/backend/internal/service"
)

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

func NewHTTPHandler(agentSvc *service.AgentService, teamSvc *service.TeamService, sessionSvc *service.SessionService, chatSvc *service.ChatService, auditSvc *service.AuditService, platformSvc *service.PlatformService, usageSvc *service.UsageService, skillSvc *service.SkillService, toolSvc *service.ToolService, pluginSvc *service.PluginService, channelSvc *service.ChannelService) http.Handler {
	h := &HTTPHandler{
		agentSvc:    agentSvc,
		teamSvc:     teamSvc,
		sessionSvc:  sessionSvc,
		chatSvc:     chatSvc,
		auditSvc:    auditSvc,
		platformSvc: platformSvc,
		usageSvc:    usageSvc,
		skillSvc:    skillSvc,
		toolSvc:     toolSvc,
		pluginSvc:   pluginSvc,
		channelSvc:  channelSvc,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc("/api/v1/agents/validate-model", h.handleValidateModel)
	mux.HandleFunc("/api/v1/agents/", h.handleAgentByID)
	mux.HandleFunc("/api/v1/agents", h.handleAgents)
	mux.HandleFunc("/api/v1/teams/", h.handleTeamByID)
	mux.HandleFunc("/api/v1/teams", h.handleTeams)
	mux.HandleFunc("/api/v1/team-runs/", h.handleTeamRunByID)
	mux.HandleFunc("/api/v1/team-runs", h.handleTeamRuns)
	mux.HandleFunc("/api/v1/team-run-events", h.handleTeamRunEvents)
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
	mux.HandleFunc("/api/v1/channels/catalog", h.handleChannelCatalog)
	mux.HandleFunc("/api/v1/channels", h.handleChannels)
	mux.HandleFunc("/api/v1/channels/", h.handleChannelByID)
	mux.HandleFunc("/api/v1/mcp-servers", h.handlePlatformCollection("mcp-servers"))
	mux.HandleFunc("/api/v1/mcp-servers/", h.handlePlatformItem("mcp-servers", "/api/v1/mcp-servers/"))
	mux.HandleFunc("/api/v1/skills/import", h.handleSkillImport)
	mux.HandleFunc("/api/v1/skills/import/", h.handleSkillImportByID)
	mux.HandleFunc("/api/v1/skills", h.handleSkills)
	mux.HandleFunc("/api/v1/skills/", h.handleSkillByID)
	mux.HandleFunc("/api/v1/skill-runs", h.handleSkillRuns)
	mux.HandleFunc("/api/v1/tools/runs", h.handleToolRuns)
	mux.HandleFunc("/api/v1/tools", h.handleTools)
	mux.HandleFunc("/api/v1/tools/", h.handleToolByID)
	mux.HandleFunc("/api/v1/plugins", h.handlePlugins)
	mux.HandleFunc("/api/v1/plugins/", h.handlePluginByID)
	mux.HandleFunc("/api/v1/cron-tasks", h.handlePlatformCollection("cron-tasks"))
	mux.HandleFunc("/api/v1/cron-tasks/", h.handlePlatformItem("cron-tasks", "/api/v1/cron-tasks/"))
	mux.HandleFunc("/api/v1/sessions", h.handleSessions)
	mux.HandleFunc("/api/v1/sessions/", h.handleSessionByID)
	mux.HandleFunc("/api/v1/chat/messages/stream", h.handleChatMessagesStream)
	mux.HandleFunc("/api/v1/chat/messages", h.handleChatMessages)
	mux.HandleFunc("/api/v1/chat/options", h.handleChatOptions)
	mux.HandleFunc("/api/v1/model-usage/overview", h.handleModelUsageOverview)
	mux.HandleFunc("/api/v1/model-usage/trends", h.handleModelUsageTrends)
	mux.HandleFunc("/api/v1/model-usage/top-models", h.handleModelUsageTopModels)
	mux.HandleFunc("/api/v1/model-usage/top-agents", h.handleModelUsageTopAgents)
	mux.HandleFunc("/api/v1/model-usage/events", h.handleModelUsageEvents)
	mux.HandleFunc("/api/v1/monitor/events", h.handlePlatformCollection("monitor-events"))
	mux.HandleFunc("/api/v1/monitor/events/", h.handlePlatformItem("monitor-events", "/api/v1/monitor/events/"))
	mux.HandleFunc("/api/v1/monitor/traces", h.handlePlatformCollection("monitor-traces"))
	mux.HandleFunc("/api/v1/monitor/traces/", h.handlePlatformItem("monitor-traces", "/api/v1/monitor/traces/"))
	mux.HandleFunc("/api/v1/monitor/audit", h.handleAuditLogs)
	return mux
}

func (h *HTTPHandler) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
