CREATE TABLE IF NOT EXISTS agents (
  id TEXT PRIMARY KEY,
  agent_key TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  is_default INTEGER NOT NULL DEFAULT 0,
  is_favorite INTEGER NOT NULL DEFAULT 0,
  icon TEXT NOT NULL DEFAULT '',
  agent_description TEXT NOT NULL DEFAULT '',
  category_position_id TEXT NOT NULL DEFAULT '',
  system_prompt_mode TEXT NOT NULL DEFAULT '',
  context_window INTEGER NOT NULL DEFAULT 0,
  budget_monthly_cents INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS agent_runtime_settings (
  agent_id TEXT PRIMARY KEY,
  self_evolve INTEGER NOT NULL DEFAULT 1,
  subagents_enabled INTEGER NOT NULL DEFAULT 1,
  subagents_max_concurrency INTEGER NOT NULL DEFAULT 20,
  subagents_max_generation_depth INTEGER NOT NULL DEFAULT 1,
  subagents_max_children_per_agent INTEGER NOT NULL DEFAULT 5,
  subagents_archive_after_minutes INTEGER NOT NULL DEFAULT 60,
  subagents_max_retries INTEGER NOT NULL DEFAULT 2,
  subagents_model_override TEXT NOT NULL DEFAULT '',
  tools_enabled INTEGER NOT NULL DEFAULT 1,
  tools_profile TEXT NOT NULL DEFAULT 'full',
  tools_tool_call_prefix TEXT NOT NULL DEFAULT '',
  tools_allow_json TEXT NOT NULL DEFAULT '[]',
  tools_deny_json TEXT NOT NULL DEFAULT '[]',
  tools_concurrent_allow_json TEXT NOT NULL DEFAULT '[]',
  memory_enabled INTEGER NOT NULL DEFAULT 1,
  memory_max_chunk_length INTEGER NOT NULL DEFAULT 1000,
  memory_max_results INTEGER NOT NULL DEFAULT 6,
  memory_min_score REAL NOT NULL DEFAULT 0.35,
  heartbeat_enabled INTEGER NOT NULL DEFAULT 0,
  heartbeat_interval_minutes INTEGER NOT NULL DEFAULT 30,
  evolution_self_evolve INTEGER NOT NULL DEFAULT 1,
  evolution_skill_evolve INTEGER NOT NULL DEFAULT 1,
  evolution_metrics_enabled INTEGER NOT NULL DEFAULT 1,
  evolution_suggestions_enabled INTEGER NOT NULL DEFAULT 1,
  guardrail_max_change_per_period REAL NOT NULL DEFAULT 0.1,
  guardrail_min_data_points INTEGER NOT NULL DEFAULT 100,
  guardrail_rollback_on_decline_percent INTEGER NOT NULL DEFAULT 20,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_prompt_files (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  file_name TEXT NOT NULL,
  body TEXT NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(agent_id, file_name)
);

CREATE TABLE IF NOT EXISTS teams (
  id TEXT PRIMARY KEY,
  team_key TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  is_default INTEGER NOT NULL DEFAULT 0,
  definition_json TEXT NOT NULL DEFAULT '',
  adk_app_name TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS chat_entities_order (
  id TEXT PRIMARY KEY,
  owner_scope TEXT NOT NULL,
  entity_type TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(owner_scope, entity_type, entity_id)
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  owner_type TEXT NOT NULL DEFAULT 'agent',
  agent_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL,
  context_used_ratio REAL NOT NULL DEFAULT 0,
  dialog_mode TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  last_message_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  parent_message_id TEXT NOT NULL DEFAULT '',
  turn_index INTEGER NOT NULL DEFAULT 0,
  role TEXT NOT NULL,
  content_markdown TEXT NOT NULL,
  model_name TEXT NOT NULL DEFAULT '',
  token_in INTEGER NOT NULL DEFAULT 0,
  token_out INTEGER NOT NULL DEFAULT 0,
  latency_ms INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'ok',
  attachments_count INTEGER NOT NULL DEFAULT 0,
  options_json TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS model_token_usage_events (
  id TEXT PRIMARY KEY,
  occurred_at TEXT NOT NULL,
  date_key TEXT NOT NULL,
  hour_key TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  agent_key TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  message_id TEXT NOT NULL DEFAULT '',
  request_id TEXT NOT NULL DEFAULT '',
  provider_code TEXT NOT NULL DEFAULT '',
  provider_type TEXT NOT NULL DEFAULT '',
  provider_display_name TEXT NOT NULL DEFAULT '',
  model_api_id TEXT NOT NULL DEFAULT '',
  model_display_name TEXT NOT NULL DEFAULT '',
  model_category_json TEXT NOT NULL DEFAULT '[]',
  usage_kind TEXT NOT NULL DEFAULT 'chat',
  call_count INTEGER NOT NULL DEFAULT 1,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  reasoning_tokens INTEGER NOT NULL DEFAULT 0,
  embedding_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  input_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  output_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  cached_input_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  reasoning_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  embedding_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  input_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  output_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  cached_input_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  reasoning_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  embedding_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  latency_ms INTEGER NOT NULL DEFAULT 0,
  time_to_first_token_ms INTEGER NOT NULL DEFAULT 0,
  tokens_per_second REAL NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'success',
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  retry_count INTEGER NOT NULL DEFAULT 0,
  prompt_mode TEXT NOT NULL DEFAULT '',
  max_output_tokens INTEGER NOT NULL DEFAULT 0,
  context_window_k INTEGER NOT NULL DEFAULT 0,
  stream_enabled INTEGER NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS model_pricing_rules (
  id TEXT PRIMARY KEY,
  provider_code TEXT NOT NULL,
  model_api_id TEXT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'USD',
  input_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  output_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  cached_input_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  reasoning_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  embedding_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  effective_from TEXT NOT NULL,
  effective_to TEXT NOT NULL DEFAULT '',
  is_active INTEGER NOT NULL DEFAULT 1,
  source TEXT NOT NULL DEFAULT 'manual',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(provider_code, model_api_id, effective_from)
);

CREATE TABLE IF NOT EXISTS model_token_usage_daily (
  id TEXT PRIMARY KEY,
  date_key TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  agent_key TEXT NOT NULL DEFAULT '',
  provider_code TEXT NOT NULL DEFAULT '',
  model_api_id TEXT NOT NULL DEFAULT '',
  usage_kind TEXT NOT NULL DEFAULT 'chat',
  call_count INTEGER NOT NULL DEFAULT 0,
  request_count INTEGER NOT NULL DEFAULT 0,
  success_count INTEGER NOT NULL DEFAULT 0,
  failed_count INTEGER NOT NULL DEFAULT 0,
  cancelled_count INTEGER NOT NULL DEFAULT 0,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  reasoning_tokens INTEGER NOT NULL DEFAULT 0,
  embedding_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  avg_latency_ms REAL NOT NULL DEFAULT 0,
  avg_tokens_per_second REAL NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(date_key, workspace_id, agent_id, provider_code, model_api_id, usage_kind)
);

CREATE TABLE IF NOT EXISTS chat_attachments (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  message_id TEXT NOT NULL DEFAULT '',
  file_name TEXT NOT NULL,
  mime_type TEXT NOT NULL DEFAULT '',
  size_bytes INTEGER NOT NULL DEFAULT 0,
  storage_key TEXT NOT NULL DEFAULT '',
  checksum TEXT NOT NULL DEFAULT '',
  upload_status TEXT NOT NULL DEFAULT 'pending',
  created_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS chat_options (
  type TEXT NOT NULL,
  key TEXT NOT NULL,
  label TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '',
  PRIMARY KEY(type, key)
);

CREATE TABLE IF NOT EXISTS session_summaries (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  summary_markdown TEXT NOT NULL,
  from_turn INTEGER NOT NULL DEFAULT 0,
  to_turn INTEGER NOT NULL DEFAULT 0,
  token_estimate INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS memory_items (
  id TEXT PRIMARY KEY,
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL,
  content TEXT NOT NULL,
  source_session_id TEXT NOT NULL DEFAULT '',
  source_message_id TEXT NOT NULL DEFAULT '',
  importance REAL NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id TEXT PRIMARY KEY,
  action TEXT NOT NULL,
  resource TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  request_id TEXT NOT NULL,
  detail TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS avatar_assets (
  id TEXT PRIMARY KEY,
  asset_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  image_data BLOB NOT NULL DEFAULT X'',
  thumbnail_data BLOB,
  mime_type TEXT NOT NULL DEFAULT 'image/png',
  workspace_id TEXT NOT NULL DEFAULT '',
  owner_user_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'system',
  is_system INTEGER NOT NULL DEFAULT 0,
  file_size_bytes INTEGER NOT NULL DEFAULT 0,
  width_px INTEGER NOT NULL DEFAULT 0,
  height_px INTEGER NOT NULL DEFAULT 0,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS agent_category_nodes (
  id TEXT PRIMARY KEY,
  category_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  parent_id TEXT NOT NULL DEFAULT '',
  level TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  owner_user_id TEXT NOT NULL DEFAULT '',
  is_system INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS llm_provider_models (
  id TEXT PRIMARY KEY,
  model_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS hooks (
  id TEXT PRIMARY KEY,
  hook_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS hook_agents (
  id TEXT PRIMARY KEY,
  hook_id TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  config_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT '',
  UNIQUE(hook_id, agent_id)
);

CREATE TABLE IF NOT EXISTS channel (
  id TEXT PRIMARY KEY,
  channel_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS channel_credential (
  id TEXT PRIMARY KEY,
  channel_id TEXT NOT NULL,
  credential_key TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  secret_ref TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT '',
  UNIQUE(channel_id, credential_key)
);

CREATE TABLE IF NOT EXISTS channel_delivery (
  id TEXT PRIMARY KEY,
  channel_id TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  payload_json TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS mcp_server (
  id TEXT PRIMARY KEY,
  server_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS skill (
  id TEXT PRIMARY KEY,
  skill_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS skill_version (
  id TEXT PRIMARY KEY,
  skill_id TEXT NOT NULL,
  version TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  content_markdown TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(skill_id, version)
);

CREATE TABLE IF NOT EXISTS skill_invocation (
  id TEXT PRIMARY KEY,
  skill_id TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  input_json TEXT NOT NULL DEFAULT '',
  output_json TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS cron_task (
  id TEXT PRIMARY KEY,
  task_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  agent_id TEXT NOT NULL DEFAULT '',
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS cron_task_run (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  started_at TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  output_json TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS monitor_events (
  id TEXT PRIMARY KEY,
  event_key TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'ok',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS monitor_traces (
  id TEXT PRIMARY KEY,
  trace_key TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'ok',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_sessions_agent ON sessions(agent_id, deleted_at, updated_at);
CREATE INDEX IF NOT EXISTS idx_sessions_team ON sessions(team_id, deleted_at, updated_at);
CREATE INDEX IF NOT EXISTS idx_sessions_last_message ON sessions(last_message_at);
CREATE INDEX IF NOT EXISTS idx_messages_session_turn ON messages(session_id, turn_index);
CREATE INDEX IF NOT EXISTS idx_usage_events_time ON model_token_usage_events(occurred_at);
CREATE INDEX IF NOT EXISTS idx_usage_events_date_model ON model_token_usage_events(date_key, provider_code, model_api_id);
CREATE INDEX IF NOT EXISTS idx_usage_events_agent_time ON model_token_usage_events(agent_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_usage_events_session ON model_token_usage_events(session_id);
CREATE INDEX IF NOT EXISTS idx_usage_events_status ON model_token_usage_events(status, occurred_at);
CREATE INDEX IF NOT EXISTS idx_usage_daily_date_model ON model_token_usage_daily(date_key, provider_code, model_api_id);
CREATE INDEX IF NOT EXISTS idx_pricing_rules_model_active ON model_pricing_rules(provider_code, model_api_id, is_active, effective_from);
CREATE INDEX IF NOT EXISTS idx_attachments_session ON chat_attachments(session_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_memory_scope ON memory_items(scope_type, scope_id);
CREATE INDEX IF NOT EXISTS idx_agent_prompt_files_agent ON agent_prompt_files(agent_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_avatar_assets_system ON avatar_assets(is_system, sort_order);
CREATE INDEX IF NOT EXISTS idx_avatar_assets_workspace_owner ON avatar_assets(workspace_id, owner_user_id);
CREATE INDEX IF NOT EXISTS idx_agent_category_parent ON agent_category_nodes(parent_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_agent_category_level ON agent_category_nodes(level, sort_order);
CREATE INDEX IF NOT EXISTS idx_provider_models_provider ON llm_provider_models(provider, enabled, sort_order);
CREATE INDEX IF NOT EXISTS idx_hook_agents_agent ON hook_agents(agent_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_channel_delivery_channel ON channel_delivery(channel_id, created_at);
CREATE INDEX IF NOT EXISTS idx_skill_invocation_skill ON skill_invocation(skill_id, created_at);
CREATE INDEX IF NOT EXISTS idx_cron_task_agent ON cron_task(agent_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_cron_run_task ON cron_task_run(task_id, created_at);
CREATE INDEX IF NOT EXISTS idx_monitor_events_created ON monitor_events(created_at);
CREATE INDEX IF NOT EXISTS idx_monitor_traces_created ON monitor_traces(created_at);
