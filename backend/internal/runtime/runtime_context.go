package runtime

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/genai"
)

// RuntimeContext is the structured runtime payload that gets rendered
// into the system prompt so the model has explicit knowledge of the
// session it's running in, the team it belongs to (if any) and the
// tool surface available for the current turn. Injecting this layer
// is the systemic fix for cases where the model used file system
// tools to answer questions whose answers were already implicit in
// our backend (e.g. team member counts).
type RuntimeContext struct {
	Session  SessionContext
	Team     *TeamContext
	SelfRole string
	Tools    []ToolHint
}

type SessionContext struct {
	SessionID  string
	DialogMode string
	StartedAt  string
}

type TeamContext struct {
	TeamID      string
	DisplayName string
	Mode        string
	Members     []TeamMemberContext
}

type TeamMemberContext struct {
	AgentID string
	Role    string
	Name    string
}

type ToolHint struct {
	Name        string
	Description string
}

// CloneWithRole returns a shallow copy of the context with a different
// SelfRole. Used by team_runtime where the same context is shared
// across members but each member's SelfRole differs.
func (c *RuntimeContext) CloneWithRole(role string) *RuntimeContext {
	if c == nil {
		return &RuntimeContext{SelfRole: role}
	}
	clone := *c
	clone.SelfRole = role
	return &clone
}

// renderRuntimeContextBlock returns the deterministic, fixed-format
// block that is appended to the system prompt. Empty contexts return
// an empty string so callers can safely omit the section.
func renderRuntimeContextBlock(rc *RuntimeContext) string {
	if rc == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## Runtime Context\n")
	if rc.Session.SessionID != "" {
		b.WriteString("- session_id: ")
		b.WriteString(rc.Session.SessionID)
		b.WriteString("\n")
	}
	if rc.Session.DialogMode != "" {
		b.WriteString("- dialog_mode: ")
		b.WriteString(rc.Session.DialogMode)
		b.WriteString("\n")
	}
	if rc.Session.StartedAt != "" {
		b.WriteString("- started_at: ")
		b.WriteString(rc.Session.StartedAt)
		b.WriteString("\n")
	}
	if rc.SelfRole != "" {
		b.WriteString("- self_role: ")
		b.WriteString(rc.SelfRole)
		b.WriteString("\n")
	}
	if rc.Team != nil {
		b.WriteString("\n### Team\n")
		if rc.Team.DisplayName != "" {
			b.WriteString("- name: ")
			b.WriteString(rc.Team.DisplayName)
			b.WriteString("\n")
		}
		if rc.Team.TeamID != "" {
			b.WriteString("- team_id: ")
			b.WriteString(rc.Team.TeamID)
			b.WriteString("\n")
		}
		if rc.Team.Mode != "" {
			b.WriteString("- orchestration_mode: ")
			b.WriteString(rc.Team.Mode)
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("- member_count: %d\n", len(rc.Team.Members)))
		if len(rc.Team.Members) > 0 {
			b.WriteString("- members:\n")
			for i, member := range rc.Team.Members {
				label := firstNonEmpty(member.Name, member.Role, member.AgentID)
				role := firstNonEmpty(member.Role, "member")
				b.WriteString(fmt.Sprintf("  %d. %s (role=%s, agent_id=%s)\n", i+1, label, role, member.AgentID))
			}
		}
	}
	if len(rc.Tools) > 0 {
		b.WriteString("\n### Available Tools\n")
		hints := append([]ToolHint(nil), rc.Tools...)
		sort.SliceStable(hints, func(i, j int) bool { return hints[i].Name < hints[j].Name })
		for _, hint := range hints {
			b.WriteString("- `")
			b.WriteString(hint.Name)
			b.WriteString("`")
			if desc := strings.TrimSpace(hint.Description); desc != "" {
				b.WriteString(": ")
				b.WriteString(escapeInstructionPlaceholders(desc))
			}
			b.WriteString("\n")
		}
	} else if rc.Session.SessionID != "" || rc.Team != nil {
		b.WriteString("\n### Available Tools\n- (none — this turn must be answered from context)\n")
	}
	b.WriteString(renderToolUsagePolicy(rc))
	return b.String()
}

func renderToolUsagePolicy(rc *RuntimeContext) string {
	if rc == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Tool Usage Policy\n")
	b.WriteString("1. Treat the Runtime Context block as authoritative. If the user's question is satisfied by it (e.g. team membership, session metadata, current self role), answer directly without calling any tool.\n")
	b.WriteString("2. Use a tool only when (a) the answer requires data the Runtime Context does not contain and (b) the chosen tool's description matches the data class needed.\n")
	b.WriteString("3. Never call file system tools (read_file, list_files, write_file, edit_file) to ask about agents, teams, members, sessions, providers, models or any in-app metadata. That information lives in the Runtime Context only.\n")
	b.WriteString("4. If the same tool fails twice with similar arguments, stop calling it and explain the limitation to the user instead of retrying further.\n")
	b.WriteString("5. Prefer answering with reasoning. A turn that returns a clear answer without any tool call is preferred over a turn with redundant or speculative tool calls.\n")
	return b.String()
}

// escapeInstructionPlaceholders neutralizes any "{name}" pattern that
// the ADK instruction processor would otherwise interpret as a session
// state placeholder. Without this guard, a tool description that
// contains "{ path }" leaks into the system prompt and triggers
// "state key does not exist" failures at runtime.
func escapeInstructionPlaceholders(text string) string {
	if !strings.ContainsAny(text, "{}") {
		return text
	}
	replacer := strings.NewReplacer("{", "[", "}", "]")
	return replacer.Replace(text)
}

// ToolHintsFromDeclarations converts the function declarations sent
// to the model into structured hints suitable for runtime context
// rendering. Used by call sites that have already filtered tools by
// the agent's runtime settings (see adkRuntimeTools).
func ToolHintsFromDeclarations(declarations []*genai.FunctionDeclaration) []ToolHint {
	if len(declarations) == 0 {
		return nil
	}
	hints := make([]ToolHint, 0, len(declarations))
	for _, declaration := range declarations {
		if declaration == nil || strings.TrimSpace(declaration.Name) == "" {
			continue
		}
		hints = append(hints, ToolHint{Name: declaration.Name, Description: declaration.Description})
	}
	return hints
}
