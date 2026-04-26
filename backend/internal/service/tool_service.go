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

// EvolutionToolPolicySource is the narrow contract ToolService uses to
// fold the agent's self-evolution tool blacklist and preference scores
// into the EffectiveForAgent view. Implemented by *AgentEvolutionService.
//
// The seam keeps the tool surface independent of L4: when no source is
// wired, EffectiveForAgent falls back to the static profile / allow /
// deny computation only.
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

// SetEvolutionPolicySource wires the optional self-evolution source used
// by EffectiveForAgent. Callers (e.g. server bootstrap) should pass the
// AgentEvolutionService instance after constructing both services.
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
		// The frontend asks for confirmation; the backend still keeps the risk
		// visible by returning the updated tool only after this explicit call.
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
				// Allowed items always sort before denied ones so the
				// agent prompt renders the actionable subset first.
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

// resolveEvolutionPolicy looks up the agent's self-evolution tool
// blacklist + preference scores via the optional source. Returns nils
// when no source is wired or the lookup fails — callers must tolerate
// the empty case so the tool view degrades gracefully.
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
	// cli_admin is populated lazily from the cli_admin_* tool seeds so
	// the group automatically expands when new admin tools are added.
	"cli_admin": repository.CLIAdminToolKeys(),
}

// toolProfiles defines the canonical, semantically meaningful tool
// surfaces an agent can be granted. The names are deliberately
// intent-driven (chat_only / read_only / coding / research / full)
// instead of implementation-driven so operators can reason about an
// agent's capability scope without reading the tool list.
//
// Legacy names ("minimal", "safe", "system_admin") are preserved here
// so existing rows in agents.tools_profile keep their original
// behaviour. Frontends should expose the new names; legacy values are
// gracefully mapped to a comparable new profile via canonicalToolProfile.
var toolProfiles = map[string][]string{
	"chat_only": {},
	"read_only": {"datetime", "read_file", "list_files"},
	"coding":    {"group:filesystem", "group:web", "group:skill", "datetime"},
	"research":  {"web_search", "web_fetch", "read_file", "list_files", "skill_search", "memory_search", "datetime"},
	"full":      {"group:filesystem", "group:web", "group:skill", "group:memory", "group:media", "group:runtime", "group:cli_admin", "datetime"},

	// Legacy aliases retained for backward compatibility with stored
	// agent settings. Treat them as deprecated — new UI flows should
	// pick from chat_only / read_only / coding / research / full.
	"minimal":      {},
	"safe":         {"datetime", "read_file", "list_files"},
	"system_admin": {"group:cli_admin", "web_fetch", "datetime"},
}

// canonicalToolProfile normalizes any profile string (including legacy
// names) into one of the supported canonical profiles. It is used by
// the runtime / API layer when reporting an agent's effective profile
// so the frontend can render a consistent label.
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
