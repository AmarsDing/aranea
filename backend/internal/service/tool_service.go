package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

type toolStore interface {
	SearchTools(query domain.ToolListQuery) (domain.ToolListResult, error)
	GetToolByID(id string) (domain.Tool, error)
	UpdateToolEnabled(id string, enabled bool) (domain.Tool, error)
	SearchToolInvocations(query domain.ToolRunQuery) (domain.ToolRunResult, error)
	GetAgentRuntimeSettings(agentID string) (domain.AgentRuntimeSettings, error)
	UpsertAgentRuntimeSettings(settings domain.AgentRuntimeSettings) (domain.AgentRuntimeSettings, error)
}

// EvolutionToolPolicySource 是 ToolService 用于将 Agent 自演化工具黑名单与偏好分数
// 合并进 EffectiveForAgent 视图的窄接口。由 *AgentEvolutionService 实现。
//
// 该接缝使工具面与 L4 解耦：未注入来源时，EffectiveForAgent 仅回退到静态 profile / allow / deny 计算。
type EvolutionToolPolicySource interface {
	ToolPolicyForAgent(ctx context.Context, agentID string) (blacklist []string, preference map[string]float64, err error)
}

type ToolService struct {
	store     toolStore
	evolution EvolutionToolPolicySource
}

func NewToolService(store toolStore) *ToolService {
	return &ToolService{store: store}
}

// SetEvolutionPolicySource 注入 EffectiveForAgent 使用的可选自演化策略来源。
// 调用方（如 server 启动）应在两个服务都构造完成后传入 AgentEvolutionService 实例。
func (s *ToolService) SetEvolutionPolicySource(src EvolutionToolPolicySource) {
	s.evolution = src
}

func (s *ToolService) Search(query domain.ToolListQuery) (domain.ToolListResult, error) {
	query.Limit, query.Offset = normalizeLimitOffset(query.Limit, query.Offset, 20)
	query.Enabled = strings.TrimSpace(query.Enabled)
	if query.Enabled != "" && query.Enabled != "true" && query.Enabled != "false" {
		return domain.ToolListResult{}, validationError("enabled must be true or false")
	}
	return s.store.SearchTools(query)
}

func (s *ToolService) Get(id string) (domain.Tool, error) {
	if strings.TrimSpace(id) == "" {
		return domain.Tool{}, validationError("tool id is required")
	}
	return s.store.GetToolByID(id)
}

func (s *ToolService) ToggleEnabled(id string, enabled bool) (domain.Tool, error) {
	tool, err := s.Get(id)
	if err != nil {
		return domain.Tool{}, err
	}
	if enabled && (tool.RiskLevel == "high" || tool.RiskLevel == "critical") {
		// 前端会要求确认；后端仍通过仅在显式调用后返回更新后的工具来保留风险可见性。
	}
	return s.store.UpdateToolEnabled(id, enabled)
}

func (s *ToolService) SearchRuns(query domain.ToolRunQuery) (domain.ToolRunResult, error) {
	query.Limit, query.Offset = normalizeLimitOffset(query.Limit, query.Offset, 20)
	return s.store.SearchToolInvocations(query)
}

func (s *ToolService) EffectiveForAgent(agentID string) (domain.AgentEffectiveTools, error) {
	if strings.TrimSpace(agentID) == "" {
		return domain.AgentEffectiveTools{}, validationError("agent id is required")
	}
	settings, err := s.store.GetAgentRuntimeSettings(agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			settings = defaultRuntimeSettings()
			settings.AgentID = agentID
		} else {
			return domain.AgentEffectiveTools{}, err
		}
	}
	all, err := s.store.SearchTools(domain.ToolListQuery{Limit: 1000})
	if err != nil {
		return domain.AgentEffectiveTools{}, err
	}
	allow := jsonList(settings.ToolsAllowJSON)
	deny := jsonList(settings.ToolsDenyJSON)
	allowedSet := s.profileAllowSet(settings.ToolsProfile)
	for _, key := range allow {
		if strings.HasPrefix(key, "group:") {
			for _, member := range toolGroups[strings.TrimPrefix(key, "group:")] {
				allowedSet[member] = true
			}
			continue
		}
		allowedSet[key] = true
	}
	denySet := map[string]bool{}
	for _, key := range deny {
		if strings.HasPrefix(key, "group:") {
			for _, member := range toolGroups[strings.TrimPrefix(key, "group:")] {
				denySet[member] = true
			}
			continue
		}
		denySet[key] = true
	}
	evoBlacklist, evoPreference := s.resolveEvolutionPolicy(agentID)
	evoBlacklistSet := map[string]bool{}
	for _, k := range evoBlacklist {
		evoBlacklistSet[k] = true
	}

	items := make([]domain.EffectiveAgentTool, 0, len(all.Items))
	for _, tool := range all.Items {
		state := "denied"
		reason := "global_disabled"
		enabled := settings.ToolsEnabled && tool.Enabled
		if enabled && (settings.ToolsProfile == "" || settings.ToolsProfile == "full" || allowedSet[tool.Key]) {
			state = "allowed"
			reason = "profile:" + settings.ToolsProfile
		}
		if denySet[tool.Key] {
			state = "denied"
			reason = "agent_deny"
		}
		if evoBlacklistSet[tool.Key] {
			state = "denied"
			reason = "evolution_blacklist"
		}
		if !settings.ToolsEnabled {
			reason = "agent_tools_disabled"
		}
		items = append(items, domain.EffectiveAgentTool{
			ToolKey:        tool.Key,
			DisplayName:    tool.DisplayName,
			Category:       tool.Category,
			Source:         tool.Source,
			Enabled:        enabled && state == "allowed",
			EffectiveState: state,
			Reason:         reason,
		})
	}

	if len(evoPreference) > 0 {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].EffectiveState != items[j].EffectiveState {
				// 允许的项始终排在拒绝项之前，以便 Agent 提示词优先展示可执行子集。
				return items[i].EffectiveState == "allowed"
			}
			return evoPreference[items[i].ToolKey] > evoPreference[items[j].ToolKey]
		})
	}

	return domain.AgentEffectiveTools{
		ToolsEnabled: settings.ToolsEnabled,
		Profile:      canonicalToolProfile(settings.ToolsProfile),
		Allow:        allow,
		Deny:         deny,
		Items:        items,
	}, nil
}

// resolveEvolutionPolicy 通过可选来源查询 Agent 自演化工具黑名单与偏好分数。
// 未注入来源或查询失败时返回 nil；调用方须容忍空结果，使工具视图能优雅降级。
func (s *ToolService) resolveEvolutionPolicy(agentID string) ([]string, map[string]float64) {
	if s.evolution == nil {
		return nil, nil
	}
	bl, pref, err := s.evolution.ToolPolicyForAgent(context.Background(), agentID)
	if err != nil {
		return nil, nil
	}
	return bl, pref
}

func (s *ToolService) UpdateAgentPolicy(agentID string, input domain.AgentEffectiveTools) (domain.AgentEffectiveTools, error) {
	if strings.TrimSpace(agentID) == "" {
		return domain.AgentEffectiveTools{}, validationError("agent id is required")
	}
	settings, err := s.store.GetAgentRuntimeSettings(agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			settings = defaultRuntimeSettings()
			settings.AgentID = agentID
		} else {
			return domain.AgentEffectiveTools{}, err
		}
	}
	settings.ToolsEnabled = input.ToolsEnabled
	if strings.TrimSpace(input.Profile) != "" {
		settings.ToolsProfile = strings.TrimSpace(input.Profile)
	}
	allow, _ := json.Marshal(input.Allow)
	deny, _ := json.Marshal(input.Deny)
	settings.ToolsAllowJSON = string(allow)
	settings.ToolsDenyJSON = string(deny)
	if _, err = s.store.UpsertAgentRuntimeSettings(settings); err != nil {
		return domain.AgentEffectiveTools{}, err
	}
	return s.EffectiveForAgent(agentID)
}

func (s *ToolService) profileAllowSet(profile string) map[string]bool {
	result := map[string]bool{}
	for _, key := range toolProfiles[strings.TrimSpace(profile)] {
		if strings.HasPrefix(key, "group:") {
			for _, member := range toolGroups[strings.TrimPrefix(key, "group:")] {
				result[member] = true
			}
			continue
		}
		result[key] = true
	}
	return result
}

var toolGroups = map[string][]string{
	"filesystem": {"read_file", "write_file", "list_files", "edit_file"},
	"web":        {"web_search", "web_fetch"},
	"memory":     {"memory_search", "memory_get"},
	"skill":      {"skill_search", "use_skill"},
	"media":      {"read_image", "read_document", "create_image", "tts"},
	"runtime":    {"shell_exec"},
	// cli_admin 由 cli_admin_* 工具种子惰性填充，新增管理员工具时该组会自动扩展。
	"cli_admin": repository.CLIAdminToolKeys(),
}

// toolProfiles 定义可授予 Agent 的规范、语义化工具面。命名刻意按意图（chat_only / read_only /
// coding / research / full）而非实现划分，便于运维理解能力范围而无需通读工具列表。
//
// 此处保留旧名（"minimal"、"safe"、"system_admin"），使 agents.tools_profile 既有行行为不变。
// 前端应展示新名称；旧值经 canonicalToolProfile 平滑映射到可比较的新 profile。
var toolProfiles = map[string][]string{
	"chat_only": {},
	"read_only": {"datetime", "read_file", "list_files"},
	"coding":    {"group:filesystem", "group:web", "group:skill", "datetime"},
	"research":  {"web_search", "web_fetch", "read_file", "list_files", "skill_search", "memory_search", "datetime"},
	"full":      {"group:filesystem", "group:web", "group:skill", "group:memory", "group:media", "group:runtime", "group:cli_admin", "datetime"},

	// 为兼容已存储的 Agent 设置而保留的旧别名。视为已弃用——新 UI 应从
	// chat_only / read_only / coding / research / full 中选择。
	"minimal":      {},
	"safe":         {"datetime", "read_file", "list_files"},
	"system_admin": {"group:cli_admin", "web_fetch", "datetime"},
}

// canonicalToolProfile 将任意 profile 字符串（含旧名）规范为支持的 canonical profile 之一。
// runtime / API 层在报告 Agent 有效 profile 时使用，以便前端显示一致标签。
func canonicalToolProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "":
		return ""
	case "chat_only", "minimal":
		return "chat_only"
	case "read_only", "safe":
		return "read_only"
	case "coding":
		return "coding"
	case "research":
		return "research"
	case "system_admin", "full":
		return "full"
	default:
		return profile
	}
}
