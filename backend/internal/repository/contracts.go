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
	UpdateSessionL0Context(sessionID string, promptTokens int, contextWindow int, ratio float64) error
	ArchiveSession(id string) error
	DeleteSession(id string) error
	DeleteSessionsByAgentID(agentID string) error
	AddMessage(m domain.Message) (domain.Message, error)
	ListMessages(sessionID string) ([]domain.Message, error)
	ListLatestMessagesByTokens(sessionID string, maxTokens int, hardCap int) ([]domain.Message, error)
	ListSessionSummaries(sessionID string, limit int) ([]domain.SessionSummary, error)
	AddSessionSummary(summary domain.SessionSummary) (domain.SessionSummary, error)
	InsertL0AssemblySnapshot(snap domain.L0AssemblySnapshot) error
	UpdateL0AssemblySnapshotActualTokens(snapshotID string, actualPromptTokens int, usedRatio float64) error
	GetL0AssemblySnapshotByID(id string) (domain.L0AssemblySnapshot, error)
	ListL0AssemblySnapshotsBySession(sessionID string, limit int) ([]domain.L0AssemblySnapshot, error)
	ListL0AssemblySnapshotsBySpan(spanID string) ([]domain.L0AssemblySnapshot, error)
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

	// L1 working memory (aranea/docs/13 memory-L1-working.md §4.2). All methods
	// are synchronous; the service wraps them into the higher-level lifecycle
	// hooks consumed by ChatService and the HTTP layer.
	CreateL1Task(t domain.MemoryL1Task) (domain.MemoryL1Task, error)
	UpdateL1TaskStatus(taskID string, status domain.L1TaskStatus, endedAt string, archivedAt string) error
	UpdateL1TaskUsedTokens(taskID string, usedTokens int) error
	UpdateL1TaskShared(taskID string, shared []domain.L1FieldShare) error
	UpdateL1TaskBudget(taskID string, budgetTokens int) error
	GetL1TaskByID(taskID string) (domain.MemoryL1Task, error)
	GetL1TaskByKey(sessionID, taskKey, agentID string) (domain.MemoryL1Task, error)
	ListL1TasksBySession(query domain.L1TaskListQuery) ([]domain.MemoryL1Task, error)
	ArchiveIdleL1Tasks(before string) (int, error)

	UpsertL1Field(f domain.MemoryL1Field, history domain.MemoryL1FieldHistory, keepRevisions int) (domain.MemoryL1Field, error)
	GetL1Field(taskID, fieldPath string) (domain.MemoryL1Field, error)
	GetL1FieldByID(fieldID string) (domain.MemoryL1Field, error)
	ListL1FieldsByTask(taskID string, includeInternal bool) ([]domain.MemoryL1Field, error)
	DeleteL1Field(fieldID string) error
	BumpL1FieldRead(fieldID string, atISO string) error

	ListL1FieldHistory(fieldID string, limit int) ([]domain.MemoryL1FieldHistory, error)
	GetL1FieldHistory(fieldID string, revision int) (domain.MemoryL1FieldHistory, error)

	UpsertL1Schema(s domain.MemoryL1Schema) (domain.MemoryL1Schema, error)
	ListL1Schemas(scopeType, scopeID string) ([]domain.MemoryL1Schema, error)
	GetL1SchemaByID(id string) (domain.MemoryL1Schema, error)
	DeleteL1Schema(id string) error

	// L2 episodic memory (aranea/docs/14 memory-L2-episodic.md §4.2).
	CreateEpisode(e domain.MemoryEpisode) (domain.MemoryEpisode, error)
	UpdateEpisode(e domain.MemoryEpisode) error
	GetEpisode(id string) (domain.MemoryEpisode, error)
	ListEpisodes(sessionID, kind string, limit, offset int) ([]domain.MemoryEpisode, int, error)
	ListPendingConsolidation(minImportance float64, limit int) ([]domain.MemoryEpisode, error)
	UpdateEpisodeConsolidationStatus(id, status string, l3Count, l4Count int) error
	UpdateEpisodeEmbedding(id, status, model string, dim int, norm float64) error
	SoftDeleteEpisode(id string) error
	UpsertL2Index(entry domain.MemoryL2IndexEntry, text string) error
	DeleteL2Index(episodeID string) error
	SearchL2BM25(sessionID, query string, minImportance float64, limit int) ([]domain.MemoryL2RecallResult, error)
	UpsertEventMark(m domain.MemoryEventMark) (domain.MemoryEventMark, error)
	SoftDeleteEventMark(id string) error
	ListEventMarks(sessionID, markType string, limit int) ([]domain.MemoryEventMark, error)
	ListMarksForEpisode(episodeID string) ([]domain.MemoryEventMark, error)
	ListL2Events(q domain.MemoryL2EventQuery) ([]domain.MemoryL2Event, int, error)
	ArchiveEpisodesBeforeDate(sessionID, before string) (int, error)
	DeleteArchivedEpisodesBefore(before string) (int, error)

	// L3 semantic memory (aranea/docs/15 memory-L3-semantic.md §4.2).
	CreateFact(f domain.MemoryFact) (domain.MemoryFact, error)
	UpdateFact(f domain.MemoryFact) error
	GetFact(id string) (domain.MemoryFact, error)
	GetFactByFingerprint(scopeType domain.ScopeType, scopeID, fp string) (domain.MemoryFact, error)
	ListFacts(q FactListQuery) ([]domain.MemoryFact, int, error)
	UpdateFactConfidence(id string, newConfidence float64, hitInc, posInc, negInc int) error
	UpdateFactStatus(id, status, supersededBy, archivedAt string) error
	BumpFactUseStat(id string, hit bool, atISO string) error

	InsertFactVersion(fv domain.FactVersion) error
	ListFactVersions(factID string, limit int) ([]domain.FactVersion, error)
	GetFactVersion(factID string, version int) (domain.FactVersion, error)

	InsertFactFeedback(fb domain.FactFeedback) (domain.FactFeedback, error)
	ListFactFeedback(factID string, limit int) ([]domain.FactFeedback, error)
	CountRecentFactFeedback(factID, feedbackType string, limit int) (int, error)

	UpsertFactConflict(c domain.FactConflict) (domain.FactConflict, error)
	GetFactConflict(id string) (domain.FactConflict, error)
	ListOpenFactConflicts(scope domain.ScopeType, scopeID string, limit int) ([]domain.FactConflict, error)
	UpdateFactConflictResolution(id, status, resolution, by, resolvedAt string) error

	UpsertFactEmbedding(id, model string, dim int, blob []byte, norm float64) error
	UpsertFactsFTS(factID string, scopeType domain.ScopeType, scopeID, kind, text string) error
	DeleteFactIndex(factID string) error
	SearchFactsBM25(scopes []domain.ScopeType, scopeIDs []string, query string, limit int) ([]domain.FactRecallHit, error)
	SearchFactsVector(scopes []domain.ScopeType, scopeIDs []string, q []float32, limit int) ([]domain.FactRecallHit, error)

	ListFactsDueForDecay(before string, limit int) ([]domain.MemoryFact, error)
	ApplyFactDecay(factID string, factor float64, nextAt string) error
	ArchiveFactsBelowConfidence(threshold float64, limit int) (int, error)
	CountFactsByStatus(scope domain.ScopeType, scopeID string) (map[string]int, error)

	// L4 persistent / evolutionary memory
	// (aranea/docs/16 memory-L4-persistent.md §5.1).
	UpsertEntity(e domain.MemoryEntity) (domain.MemoryEntity, error)
	GetEntity(id string) (domain.MemoryEntity, error)
	GetEntityByName(scope domain.ScopeType, scopeID string, t domain.EntityType, normalized string) (domain.MemoryEntity, error)
	ListEntities(q EntityListQuery) ([]domain.MemoryEntity, int, error)
	UpdateEntityStatus(id, status, mergedInto, archivedAt, deletedAt string) error
	UpdateEntityName(id, name, normalized string) error
	UpsertEntityFact(entityID, factID string, weight float64) error
	ListFactsForEntity(entityID string, limit int) ([]domain.MemoryEntityFactLink, error)
	InsertEntityVersion(v domain.MemoryEntityVersion) error
	ListEntityVersions(entityID string, limit int) ([]domain.MemoryEntityVersion, error)
	BumpEntityUseCount(id string, atISO string) error

	UpsertRelation(r domain.MemoryRelation) (domain.MemoryRelation, error)
	GetRelation(id string) (domain.MemoryRelation, error)
	ListRelationsForNode(nodeID string, limit int) ([]domain.MemoryRelation, error)
	UpdateRelationStatus(id, status, archivedAt, deletedAt string) error
	BumpRelationUseCount(id string, atISO string) error

	GetNeighborhood(centerID string, hops, maxNodes int) (domain.GraphNeighborhood, error)

	// Agent evolution (§5.3).
	GetAgentIdentity(agentID string) (domain.AgentIdentity, error)
	UpsertAgentIdentity(id domain.AgentIdentity) (domain.AgentIdentity, error)
	GetAgentStrategyProfile(agentID string) (domain.AgentStrategyProfile, error)
	UpsertAgentStrategyProfile(p domain.AgentStrategyProfile) (domain.AgentStrategyProfile, error)

	InsertEvolutionEvent(e domain.EvolutionEvent) (domain.EvolutionEvent, error)
	GetEvolutionEvent(id string) (domain.EvolutionEvent, error)
	ListEvolutionEvents(q EvolutionEventQuery) ([]domain.EvolutionEvent, int, error)
	MarkEvolutionEventReverted(id, byEventID, atISO string) error

	InsertEvolutionProposal(p domain.EvolutionProposal) (domain.EvolutionProposal, error)
	GetEvolutionProposal(id string) (domain.EvolutionProposal, error)
	ListEvolutionProposals(q EvolutionProposalQuery) ([]domain.EvolutionProposal, int, error)
	UpdateEvolutionProposalStatus(id, status, by, eventID, atISO string) error
	SupersedeProposalsByTarget(agentID, targetField, sinceISO string) (int, error)

	UpsertAgentSkillStat(s domain.AgentSkillStat) (domain.AgentSkillStat, error)
	GetAgentSkillStat(agentID, scope, scopeValue, toolKey string) (domain.AgentSkillStat, error)
	ListAgentSkillStats(agentID string, limit int) ([]domain.AgentSkillStat, error)
}

// EntityListQuery filters knowledge graph nodes in the repository layer.
type EntityListQuery struct {
	ScopeType   domain.ScopeType
	ScopeID     string
	WorkspaceID string
	UserID      string
	EntityType  domain.EntityType
	Status      string
	Keyword     string
	Limit       int
	Offset      int
}

// EvolutionEventQuery filters EvolutionEvent rows for list endpoints.
type EvolutionEventQuery struct {
	AgentID     string
	WorkspaceID string
	Kind        string
	TriggerKind string
	Reverted    *bool
	Limit       int
	Offset      int
}

// EvolutionProposalQuery filters EvolutionProposal rows for list endpoints.
type EvolutionProposalQuery struct {
	AgentID     string
	WorkspaceID string
	Status      string
	RiskLevel   string
	Source      string
	TargetField string
	Limit       int
	Offset      int
}

// FactListQuery filters facts in the repository layer. Empty values are
// ignored so the same struct works for the "show all" admin endpoint and
// the scoped agent UI.
type FactListQuery struct {
	ScopeType   domain.ScopeType
	ScopeID     string
	WorkspaceID string
	UserID      string
	TeamID      string
	AgentID     string
	Status      string
	Kind        domain.FactKind
	Tags        []string
	Keyword     string
	Limit       int
	Offset      int
}
