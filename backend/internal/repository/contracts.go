package repository

import "arenea/backend/internal/domain"

type Store interface {
	Migrate() error
	Close() error
	ListAgents() ([]domain.Agent, error)
	SearchAgents(query domain.AgentListQuery) (domain.AgentListResult, error)
	GetAgentByID(id string) (domain.Agent, error)
	GetAgentByKey(key string) (domain.Agent, error)
	CreateAgent(a domain.Agent) (domain.Agent, error)
	UpdateAgent(a domain.Agent) (domain.Agent, error)
	GetAgentRuntimeSettings(agentID string) (domain.AgentRuntimeSettings, error)
	UpsertAgentRuntimeSettings(settings domain.AgentRuntimeSettings) (domain.AgentRuntimeSettings, error)
	ListAgentPromptFiles(agentID string) ([]domain.AgentPromptFile, error)
	ReplaceAgentPromptFiles(agentID string, files []domain.AgentPromptFile) ([]domain.AgentPromptFile, error)
	DeleteAgent(id string) error
	ListTeams() ([]domain.Team, error)
	GetTeamByID(id string) (domain.Team, error)
	CreateTeam(t domain.Team) (domain.Team, error)
	UpdateTeam(t domain.Team) (domain.Team, error)
	DeleteTeam(id string) error
	AddTeamRun(run domain.TeamRun) (domain.TeamRun, error)
	UpdateTeamRun(run domain.TeamRun) (domain.TeamRun, error)
	AddTeamRunStep(step domain.TeamRunStep) (domain.TeamRunStep, error)
	ListTeamRuns(teamID string, limit int) ([]domain.TeamRun, error)
	ListTeamRunSteps(runID string) ([]domain.TeamRunStep, error)
	CreateSession(s domain.Session) (domain.Session, error)
	GetSessionByID(id string) (domain.Session, error)
	SearchSessions(query domain.SessionSearchQuery) (domain.SessionListResult, error)
	ListSessions(agentID string) ([]domain.Session, error)
	ListTeamSessions(teamID string) ([]domain.Session, error)
	UpdateSessionTitle(id string, title string) (domain.Session, error)
	UpdateSessionContextUsedRatio(sessionID string, ratio float64) error
	ArchiveSession(id string) error
	DeleteSession(id string) error
	DeleteSessionsByAgentID(agentID string) error
	AddMessage(m domain.Message) (domain.Message, error)
	ListMessages(sessionID string) ([]domain.Message, error)
	GetActiveModelPricingRule(provider string, model string, at string) (domain.ModelPricingRule, error)
	UpsertModelPricingRule(rule domain.ModelPricingRule) (domain.ModelPricingRule, error)
	AddModelTokenUsageEvent(event domain.ModelTokenUsageEvent) (domain.ModelTokenUsageEvent, error)
	UpsertModelTokenUsageDaily(event domain.ModelTokenUsageEvent) error
	GetModelUsageSummary(query domain.ModelUsageQuery) (domain.ModelUsageSummary, error)
	ListModelUsageTrends(query domain.ModelUsageQuery) ([]domain.ModelUsageTrendPoint, error)
	ListTopModelUsage(query domain.ModelUsageQuery) ([]domain.ModelUsageBreakdownRow, error)
	ListTopAgentUsage(query domain.ModelUsageQuery) ([]domain.ModelUsageBreakdownRow, error)
	ListModelUsageEvents(query domain.ModelUsageQuery) ([]domain.ModelTokenUsageEvent, error)
	ListChatOptions(optionType string) ([]domain.ChatOption, error)
	SearchTools(query domain.ToolListQuery) (domain.ToolListResult, error)
	GetToolByID(id string) (domain.Tool, error)
	UpdateToolEnabled(id string, enabled bool) (domain.Tool, error)
	SearchToolInvocations(query domain.ToolRunQuery) (domain.ToolRunResult, error)
	SearchSkills(query domain.SkillListQuery) (domain.SkillListResult, error)
	GetSkillByID(id string) (domain.Skill, error)
	UpdateSkillEnabled(id string, enabled bool) (domain.Skill, error)
	DuplicateSkill(id string) (domain.Skill, error)
	DeleteSkill(id string) error
	SearchSkillInvocations(query domain.SkillRunQuery) (domain.SkillRunResult, error)
	ListSkillSimilaritySources() ([]domain.SkillSimilaritySource, error)
	CreateSkillWithVersion(input domain.SkillCreateInput) (domain.Skill, error)
	GetSkillStorageDir(id string) (string, error)
	ListPlatformResources(resource string) ([]domain.PlatformResource, error)
	GetProviderModel(provider string, model string) (domain.PlatformResource, error)
	GetPlatformResource(resource string, id string) (domain.PlatformResource, error)
	CreatePlatformResource(v domain.PlatformResource) (domain.PlatformResource, error)
	UpdatePlatformResource(v domain.PlatformResource) (domain.PlatformResource, error)
	DeletePlatformResource(resource string, id string) error
	ListCronTaskRuns(query domain.CronTaskRunQuery) ([]domain.CronTaskRun, error)
	AddCronTaskRun(run domain.CronTaskRun) (domain.CronTaskRun, error)
	UpdateCronTaskRun(run domain.CronTaskRun) (domain.CronTaskRun, error)
	ListChannelCredentials(channelID string) ([]domain.ChannelCredential, error)
	UpsertChannelCredential(credential domain.ChannelCredential) (domain.ChannelCredential, error)
	DeleteChannelCredential(channelID string, credentialKey string) error
	AddChannelDelivery(delivery domain.ChannelDelivery) (domain.ChannelDelivery, error)
	ListChannelDeliveries(channelID string, limit int) ([]domain.ChannelDelivery, error)
	ListEnabledChannelRuntimeConfigs() ([]domain.ChannelRuntimeConfig, error)
	SearchPlugins(query domain.PluginListQuery) (domain.PluginListResult, error)
	UpsertPlugin(plugin domain.Plugin) (domain.Plugin, error)
	UpdatePluginEnabled(id string, enabled bool) (domain.Plugin, error)
	UpdatePluginConfig(id string, configJSON string) (domain.Plugin, error)
	ListEnabledPluginKeys() ([]string, error)
	ListAvatarAssets(scope string, workspaceID string, ownerUserID string) ([]domain.AvatarAsset, error)
	GetAvatarImage(id string, thumbnail bool) (domain.AvatarImage, error)
	CreateAvatarAsset(asset domain.AvatarAsset, image []byte, thumbnail []byte) (domain.AvatarAsset, error)
	ValidateProviderModel(provider string, model string) (bool, error)
	AddAuditLog(l domain.AuditLog) error
	ListAuditLogs(limit int) ([]domain.AuditLog, error)
}
