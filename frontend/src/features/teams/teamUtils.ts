import type { Agent, Team, TeamDefinition } from "../../api/client";

export const modeOptions = [
  { label: "顺序 sequential", value: "sequential" },
  { label: "并行 parallel", value: "parallel" },
  { label: "主控 coordinator", value: "coordinator" },
  { label: "生成评审 critic_loop", value: "critic_loop" }
];

export const statusOptions = ["draft", "active", "archived"].map((value) => ({ label: value, value }));

export const roleOptions = ["worker", "coordinator", "synthesizer", "generator", "critic"].map((value) => ({ label: value, value }));

export function defaultDefinition(): TeamDefinition {
  return {
    version: 1,
    description: "",
    mode: "sequential",
    max_concurrency: 2,
    timeout_seconds: 180,
    members: [],
    critic_loop: { max_iterations: 3, score_threshold: 0.8 }
  };
}

export function parseDefinition(team: Team): TeamDefinition {
  try {
    const parsed = JSON.parse(team.definition_json || "{}") as TeamDefinition;
    return {
      version: parsed.version || 1,
      description: parsed.description || "",
      mode: parsed.mode || "sequential",
      max_concurrency: parsed.max_concurrency || 2,
      timeout_seconds: parsed.timeout_seconds || 180,
      members: Array.isArray(parsed.members) ? parsed.members : [],
      synthesizer_agent_id: parsed.synthesizer_agent_id,
      critic_loop: parsed.critic_loop || { max_iterations: 3, score_threshold: 0.8 }
    };
  } catch {
    return defaultDefinition();
  }
}

export function agentName(agents: Agent[], id: string) {
  return agents.find((agent) => agent.id === id)?.display_name || id || "未选择 Agent";
}

export function memberIcon(role: string) {
  return ({ coordinator: "route", synthesizer: "merge_type", generator: "edit_note", critic: "fact_check", worker: "smart_toy" } as Record<string, string>)[role] || "smart_toy";
}

export function topologyNodesFromDefinition(def: TeamDefinition) {
  const mode = def.mode || "sequential";
  if (mode === "parallel") return [{ icon: "call_split", label: "并行分派" }, { icon: "groups", label: "Worker" }, { icon: "merge_type", label: "汇总" }];
  if (mode === "coordinator") return [{ icon: "route", label: "主控拆分" }, { icon: "smart_toy", label: "成员执行" }, { icon: "summarize", label: "总结" }];
  if (mode === "critic_loop") return [{ icon: "edit_note", label: "生成" }, { icon: "fact_check", label: "评审" }, { icon: "loop", label: "迭代" }];
  return [{ icon: "looks_one", label: "顺序 1" }, { icon: "arrow_forward", label: "传递" }, { icon: "flag", label: "最终输出" }];
}

export function topologyNodes(team: Team) {
  return topologyNodesFromDefinition(parseDefinition(team));
}

export function formatDate(value: string) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}
