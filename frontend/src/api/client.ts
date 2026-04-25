export { api, syncApiBaseURL } from "./http";
import { api } from "./http";
import { getBackendBaseURL } from "../config/runtime";

export type Agent = {
  id: string;
  agent_key: string;
  display_name: string;
  provider: string;
  model: string;
  status: string;
  is_default: boolean;
  is_favorite: boolean;
  icon: string;
  agent_description: string;
  category_position_id: string;
  system_prompt_mode: string;
  context_window: number;
  budget_monthly_cents: number;
  config_json: string;
  created_at: string;
  updated_at: string;
  deleted_at: string;
  settings?: AgentRuntimeSettings;
  files?: AgentPromptFile[];
};

export type AgentRuntimeSettings = {
  agent_id?: string;
  self_evolve: boolean;
  subagents_enabled: boolean;
  subagents_max_concurrency: number;
  subagents_max_generation_depth: number;
  subagents_max_children_per_agent: number;
  subagents_archive_after_minutes: number;
  subagents_max_retries: number;
  subagents_model_override: string;
  tools_enabled: boolean;
  tools_profile: string;
  tools_tool_call_prefix: string;
  tools_allow_json: string;
  tools_deny_json: string;
  tools_concurrent_allow_json: string;
  memory_enabled: boolean;
  memory_max_chunk_length: number;
  memory_max_results: number;
  memory_min_score: number;
  heartbeat_enabled: boolean;
  heartbeat_interval_minutes: number;
  evolution_self_evolve: boolean;
  evolution_skill_evolve: boolean;
  evolution_metrics_enabled: boolean;
  evolution_suggestions_enabled: boolean;
  guardrail_max_change_per_period: number;
  guardrail_min_data_points: number;
  guardrail_rollback_on_decline_percent: number;
  created_at?: string;
  updated_at?: string;
};

export type AgentPromptFile = {
  id?: string;
  agent_id?: string;
  name: string;
  body: string;
  sort_order: number;
  created_at?: string;
  updated_at?: string;
};

export type Session = {
  id: string;
  owner_type: string;
  agent_id: string;
  team_id: string;
  title: string;
  summary: string;
  context_used_ratio: number;
  max_context_used_ratio: number;
  context_status: string;
  dialog_mode: string;
  provider: string;
  model: string;
  status: string;
  message_count: number;
  run_count: number;
  model_call_count: number;
  tool_call_count: number;
  skill_call_count: number;
  mcp_call_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost_micro_usd: number;
  last_message_at: string;
  created_at: string;
  updated_at: string;
  archived_at: string;
  deleted_at: string;
};

export type SessionSearchQuery = {
  owner_type?: string;
  agent_id?: string;
  team_id?: string;
  status?: string;
  context_status?: string;
  keyword?: string;
  limit?: number;
  offset?: number;
  page?: number;
  page_size?: number;
};

export type SessionListResult = {
  items: Session[];
  total: number;
  limit: number;
  offset: number;
};

export type Team = {
  id: string;
  team_key: string;
  display_name: string;
  status: string;
  is_default: boolean;
  definition_json: string;
  adk_app_name: string;
  created_at: string;
  updated_at: string;
  deleted_at: string;
};

export type TeamDefinitionMember = {
  agent_id: string;
  role: "coordinator" | "worker" | "synthesizer" | "critic" | "generator" | string;
  name: string;
  enabled: boolean;
  sort_order: number;
};

export type TeamDefinitionGraphNode = {
  id: string;
  type: "start" | "agent" | "join" | "end" | string;
  label: string;
  agent_id?: string;
  role?: string;
  x?: number;
  y?: number;
};

export type TeamDefinitionGraphEdge = {
  id: string;
  source: string;
  target: string;
  label?: string;
  condition?: string;
};

export type TeamDefinition = {
  version: number;
  description?: string;
  mode: "sequential" | "parallel" | "coordinator" | "critic_loop" | "adaptive" | string;
  max_concurrency?: number;
  timeout_seconds?: number;
  members: TeamDefinitionMember[];
  a2a?: {
    enabled?: boolean;
    envelope_version?: string;
    message_format?: "markdown_json" | "plain" | string;
    include_trace?: boolean;
    max_payload_chars?: number;
  };
  graph?: {
    version?: number;
    layout?: "linear" | "parallel" | "loop" | "coordinator" | string;
    nodes: TeamDefinitionGraphNode[];
    edges: TeamDefinitionGraphEdge[];
  };
  synthesizer_agent_id?: string;
  critic_loop?: {
    max_iterations?: number;
    score_threshold?: number;
  };
};

export type TeamRun = {
  id: string;
  team_id: string;
  session_id: string;
  message_id: string;
  mode: string;
  status: string;
  input_preview: string;
  output_preview: string;
  token_in: number;
  token_out: number;
  cost_micro_usd: number;
  duration_ms: number;
  error_message: string;
  topology_json: string;
  started_at: string;
  finished_at: string;
  created_at: string;
  updated_at: string;
};

export type TeamRunStep = {
  id: string;
  run_id: string;
  team_id: string;
  agent_id: string;
  agent_key: string;
  agent_name: string;
  role: string;
  sort_order: number;
  status: string;
  input_preview: string;
  output_preview: string;
  token_in: number;
  token_out: number;
  cost_micro_usd: number;
  duration_ms: number;
  error_message: string;
  started_at: string;
  finished_at: string;
  created_at: string;
};

export type TeamRunEvent = {
  type: string;
  team_id: string;
  run_id: string;
  run?: TeamRun;
  step?: TeamRunStep;
};

export type Message = {
  id: string;
  session_id: string;
  parent_message_id: string;
  turn_index: number;
  role: string;
  content_markdown: string;
  model_name: string;
  token_in: number;
  token_out: number;
  latency_ms: number;
  status: string;
  attachments_count: number;
  options_json: string;
  error_message: string;
  created_at: string;
};

export type ModelUsageQuery = {
  range?: string;
  start_date?: string;
  end_date?: string;
  provider_code?: string;
  model_api_id?: string;
  agent_id?: string;
  status?: string;
  limit?: number;
};

export type ModelUsageSummary = {
  call_count: number;
  request_count: number;
  success_count: number;
  failed_count: number;
  cancelled_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost_micro_usd: number;
  avg_latency_ms: number;
  avg_tokens_per_second: number;
  success_rate: number;
};

export type ModelUsageTrendPoint = {
  date_key: string;
  call_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost_micro_usd: number;
  success_count: number;
  failed_count: number;
  cancelled_count: number;
  avg_latency_ms: number;
  avg_tokens_per_second: number;
};

export type ModelUsageBreakdownRow = {
  provider_code: string;
  model_api_id: string;
  model_display_name: string;
  agent_id: string;
  agent_key: string;
  call_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost_micro_usd: number;
  avg_latency_ms: number;
  avg_tokens_per_second: number;
  success_rate: number;
};

export type ModelTokenUsageEvent = {
  id: string;
  occurred_at: string;
  agent_id: string;
  agent_key: string;
  session_id: string;
  message_id: string;
  provider_code: string;
  provider_type: string;
  provider_display_name: string;
  model_api_id: string;
  model_display_name: string;
  call_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost_micro_usd: number;
  latency_ms: number;
  tokens_per_second: number;
  status: string;
  error_message: string;
  prompt_mode: string;
  max_output_tokens: number;
  context_window_k: number;
  stream_enabled: boolean;
};

export type ModelUsageOverview = {
  today: ModelUsageSummary;
  yesterday: ModelUsageSummary;
  month: ModelUsageSummary;
  range: ModelUsageSummary;
  trends: ModelUsageTrendPoint[];
  top_models: ModelUsageBreakdownRow[];
  top_agents: ModelUsageBreakdownRow[];
  anomalies: ModelTokenUsageEvent[];
};

export type ChatOption = {
  type: string;
  key: string;
  label: string;
  enabled: boolean;
  sort_order: number;
  metadata_json: string;
};

export type SendMessageOptions = {
  dialog_mode?: string;
  provider?: string;
  model?: string;
  attachments?: Array<{ id: string }>;
};

export type SendMessageResult = {
  user_message: Message;
  agent_message: Message;
};

export type AgentListQuery = {
  keyword?: string;
  status?: string;
  provider?: string;
  category_id?: string;
  limit?: number;
  offset?: number;
};

export type AgentListResult = {
  items: Agent[];
  total: number;
  limit: number;
  offset: number;
};

export async function listAgents(query: AgentListQuery = {}): Promise<Agent[]> {
  const result = await listAgentsPaged(query);
  return result.items;
}

export async function listAgentsPaged(query: AgentListQuery = {}): Promise<AgentListResult> {
  const { data } = await api.get("/agents", { params: query });
  return {
    items: data.items ?? [],
    total: data.total ?? data.items?.length ?? 0,
    limit: data.limit ?? query.limit ?? 24,
    offset: data.offset ?? query.offset ?? 0
  };
}

export async function createAgent(payload: {
  agent_key: string;
  display_name: string;
  provider: string;
  model: string;
  icon?: string;
  agent_description?: string;
  category_position_id?: string;
  system_prompt_mode?: string;
  context_window?: number;
  budget_monthly_cents?: number;
  config_json?: string;
}): Promise<Agent> {
  const { data } = await api.post("/agents", payload);
  return data;
}

export async function getAgent(id: string): Promise<Agent> {
  const { data } = await api.get(`/agents/${id}`);
  return data;
}

export async function listTeams(): Promise<Team[]> {
  const { data } = await api.get("/teams");
  return data.items ?? [];
}

export async function createTeam(payload: Partial<Team>): Promise<Team> {
  const { data } = await api.post("/teams", payload);
  return data;
}

export async function updateTeam(id: string, payload: Partial<Team>): Promise<Team> {
  const { data } = await api.patch(`/teams/${id}`, payload);
  return data;
}

export async function duplicateTeam(id: string): Promise<Team> {
  const { data } = await api.post(`/teams/${id}/duplicate`);
  return data;
}

export async function deleteTeam(id: string): Promise<void> {
  await api.delete(`/teams/${id}`);
}

export async function listTeamRuns(teamID?: string, limit = 50): Promise<TeamRun[]> {
  const { data } = await api.get("/team-runs", { params: { team_id: teamID, limit } });
  return data.items ?? [];
}

export async function listTeamRunSteps(runID: string): Promise<TeamRunStep[]> {
  const { data } = await api.get(`/team-runs/${runID}/steps`);
  return data.items ?? [];
}

export function subscribeTeamRunEvents(teamID: string, onEvent: (event: TeamRunEvent) => void, onError?: (error: Event) => void): EventSource {
  const query = new URLSearchParams({ team_id: teamID });
  const source = new EventSource(`${getBackendBaseURL()}/team-run-events?${query.toString()}`);
  for (const eventName of ["run_started", "step_finished", "run_finished"]) {
    source.addEventListener(eventName, (event) => {
      onEvent(JSON.parse((event as MessageEvent).data) as TeamRunEvent);
    });
  }
  source.onerror = (event) => {
    onError?.(event);
  };
  return source;
}

export async function updateAgent(id: string, payload: Partial<Agent>): Promise<Agent> {
  const { data } = await api.patch(`/agents/${id}`, payload);
  return data;
}

export async function getAgentPromptPreview(id: string, mode?: string): Promise<string> {
  const { data } = await api.get(`/agents/${id}/system-prompt/preview`, { params: mode ? { mode } : undefined });
  return data.preview ?? "";
}

export async function deleteAgent(id: string): Promise<void> {
  await api.delete(`/agents/${id}`);
}

export async function listSessions(agentID: string): Promise<Session[]> {
  const data = await searchSessions({ agent_id: agentID, limit: 200 });
  return data.items;
}

export async function listTeamSessions(teamID: string): Promise<Session[]> {
  const data = await searchSessions({ team_id: teamID, limit: 200 });
  return data.items;
}

export async function searchSessions(query: SessionSearchQuery = {}): Promise<SessionListResult> {
  const { data } = await api.get("/sessions", { params: query });
  const items = data.items ?? [];
  return {
    items,
    total: data.total ?? items.length,
    limit: data.limit ?? query.limit ?? query.page_size ?? items.length,
    offset: data.offset ?? query.offset ?? 0
  };
}

export async function getSession(id: string): Promise<Session> {
  const { data } = await api.get(`/sessions/${id}`);
  return data;
}

export async function createSession(payload: {
  owner_type?: string;
  agent_id?: string;
  team_id?: string;
  title: string;
  dialog_mode?: string;
  provider?: string;
  model?: string;
}): Promise<Session> {
  const { data } = await api.post("/sessions", payload);
  return data;
}

export async function deleteSession(id: string): Promise<void> {
  await api.delete(`/sessions/${id}`);
}

export async function archiveSession(id: string): Promise<void> {
  await api.post(`/sessions/${id}/archive`);
}

export async function updateSessionTitle(id: string, title: string): Promise<Session> {
  const { data } = await api.patch(`/sessions/${id}`, { title });
  return data;
}

export async function clearAgentSessions(agentID: string): Promise<void> {
  await api.delete("/sessions", { params: { agent_id: agentID } });
}

export async function listMessages(sessionID: string): Promise<Message[]> {
  const { data } = await api.get("/chat/messages", { params: { session_id: sessionID } });
  return data.items ?? [];
}

export async function sendMessage(payload: {
  session_id: string;
  agent_key?: string;
  team_id?: string;
  content: string;
  options?: SendMessageOptions;
}): Promise<SendMessageResult> {
  const { data } = await api.post("/chat/messages", payload);
  return data;
}

export type SendMessageStreamCallbacks = {
  signal?: AbortSignal;
  onUserMessage?: (message: Message) => void;
  onDelta?: (content: string) => void;
  onDone?: (message: Message) => void;
};

export async function sendMessageStream(
  payload: {
    session_id: string;
    agent_key?: string;
    team_id?: string;
    content: string;
    options?: SendMessageOptions;
  },
  callbacks: SendMessageStreamCallbacks = {}
): Promise<void> {
  const response = await fetch(`${getBackendBaseURL()}/chat/messages/stream`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
    signal: callbacks.signal
  });
  if (!response.ok || !response.body) {
    throw new Error(await response.text() || `stream request failed: ${response.status}`);
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const events = buffer.split(/\n\n/);
    buffer = events.pop() ?? "";
    for (const eventBlock of events) {
      handleStreamEvent(eventBlock, callbacks);
    }
  }
  if (buffer.trim()) {
    handleStreamEvent(buffer, callbacks);
  }
}

function handleStreamEvent(block: string, callbacks: SendMessageStreamCallbacks) {
  const lines = block.split(/\r?\n/);
  const event = lines.find((line) => line.startsWith("event:"))?.replace(/^event:\s*/, "").trim();
  const data = lines
    .filter((line) => line.startsWith("data:"))
    .map((line) => line.replace(/^data:\s*/, ""))
    .join("\n");
  if (!event || !data) return;
  const parsed = JSON.parse(data);
  if (event === "user_message") {
    callbacks.onUserMessage?.(parsed as Message);
  } else if (event === "delta") {
    callbacks.onDelta?.(String(parsed.content ?? ""));
  } else if (event === "done") {
    callbacks.onDone?.(parsed.agent_message as Message);
  } else if (event === "error") {
    throw new Error(String(parsed.message ?? "stream failed"));
  }
}

export async function listChatOptions(type?: string): Promise<ChatOption[]> {
  const { data } = await api.get("/chat/options", { params: type ? { type } : undefined });
  return data.items ?? [];
}

export async function getModelUsageOverview(query: ModelUsageQuery = {}): Promise<ModelUsageOverview> {
  const { data } = await api.get("/model-usage/overview", { params: cleanModelUsageQuery(query) });
  return data;
}

export async function listModelUsageTrends(query: ModelUsageQuery = {}): Promise<ModelUsageTrendPoint[]> {
  const { data } = await api.get("/model-usage/trends", { params: cleanModelUsageQuery(query) });
  return data.items ?? [];
}

export async function listModelUsageEvents(query: ModelUsageQuery = {}): Promise<ModelTokenUsageEvent[]> {
  const { data } = await api.get("/model-usage/events", { params: cleanModelUsageQuery(query) });
  return data.items ?? [];
}

function cleanModelUsageQuery(query: ModelUsageQuery) {
  return Object.fromEntries(
    Object.entries(query).filter(([, value]) => value !== "" && value !== undefined && value !== null)
  );
}

export type AuditLog = {
  id: string;
  action: string;
  resource: string;
  resource_id: string;
  request_id: string;
  detail: string;
  created_at: string;
};

export async function listAuditLogs(): Promise<AuditLog[]> {
  const { data } = await api.get("/monitor/audit");
  return data.items ?? [];
}
