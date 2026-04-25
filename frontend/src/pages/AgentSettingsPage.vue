<template>
  <q-page class="agent-settings">
    <q-card flat bordered class="settings-shell">
      <agent-settings-header
        :agent="form"
        :self-evolve="config.self_evolve"
        :favorite="form.is_favorite"
        :saving="saving"
        @back="router.back()"
        @change-avatar="avatarPickerOpen = true"
        @open-prompt="promptDialog = true"
        @toggle-favorite="toggleFavorite"
        @save="saveAgent"
      />
      <q-separator />
      <q-tabs v-model="tab" dense align="left" active-color="primary" indicator-color="primary" class="settings-tabs">
        <q-tab name="agent" label="Agent" />
        <q-tab name="files" label="文件" />
        <q-tab name="permissions" label="权限" />
        <q-tab name="evolution" label="进化" />
        <q-tab name="hooks" label="钩子" />
        <q-tab name="instances" label="用户实例" />
      </q-tabs>
      <q-separator />

      <q-tab-panels v-model="tab" animated class="settings-panels">
        <q-tab-panel name="agent">
          <div class="settings-grid">
            <section class="settings-section">
              <div class="section-heading">
                <div>
                  <div class="text-subtitle1 text-weight-bold">系统提示模式</div>
                  <div class="text-caption text-grey-7">控制运行时注入的提示块体量与人格强度。</div>
                </div>
              </div>
              <div class="row q-col-gutter-md">
                <div v-for="mode in promptModes" :key="mode.value" class="col-12 col-md-3">
                  <q-card
                    flat
                    bordered
                    class="prompt-mode-card cursor-pointer"
                    :class="{ 'is-active': form.system_prompt_mode === mode.value }"
                    @click="form.system_prompt_mode = mode.value"
                  >
                    <q-card-section>
                      <div class="text-subtitle2 text-weight-bold">{{ mode.label }}</div>
                      <div class="text-caption text-grey-7 q-mt-xs">{{ mode.caption }}</div>
                      <q-chip dense square class="prompt-mode-card__token q-mt-sm">{{ mode.tokens }}</q-chip>
                    </q-card-section>
                  </q-card>
                </div>
              </div>
            </section>

            <section class="settings-section">
              <div class="section-heading">
                <div>
                  <div class="text-subtitle1 text-weight-bold">Agent 个性</div>
                  <div class="text-caption text-grey-7">身份、状态、分类与对外描述。</div>
                </div>
              </div>
              <div class="row q-col-gutter-md">
                <q-input v-model="form.display_name" class="col-12 col-md-6" dense outlined label="显示名称" />
                <q-input v-model="form.agent_key" class="col-12 col-md-6" dense outlined readonly label="Agent 标识">
                  <template #append><q-btn flat round dense icon="content_copy" @click="copyKey" /></template>
                </q-input>
                <q-select v-model="form.status" class="col-12 col-md-6" dense outlined emit-value map-options label="状态" :options="statusOptions" />
                <q-toggle v-model="form.is_default" class="col-12 col-md-6" color="primary" label="默认 Agent" />
                <q-input v-model="form.agent_description" class="col-12" outlined autogrow type="textarea" label="专业摘要 / 能力描述" />
              </div>
            </section>

            <section class="settings-section">
              <div class="section-heading">
                <div>
                  <div class="text-subtitle1 text-weight-bold">模型与预算</div>
                  <div class="text-caption text-grey-7">选择数据库已录入的模型；上下文大小在 Provider 管理中维护。</div>
                </div>
              </div>
              <div class="row q-col-gutter-md">
                <q-select
                  :model-value="selectedProviderModelID"
                  class="col-12 col-md-6"
                  dense
                  outlined
                  emit-value
                  map-options
                  use-input
                  input-debounce="0"
                  label="模型"
                  hint="仅可选择 Provider 管理中已录入且启用的模型。"
                  :options="filteredProviderModelOptions"
                  :loading="loadingProviderModels"
                  :disable="loadingProviderModels"
                  @filter="filterProviderModels"
                  @update:model-value="selectProviderModel"
                >
                  <template #option="scope">
                    <q-item v-bind="scope.itemProps">
                      <q-item-section>
                        <q-item-label>{{ scope.opt.label }}</q-item-label>
                        <q-item-label caption>{{ scope.opt.caption }}</q-item-label>
                      </q-item-section>
                    </q-item>
                  </template>
                </q-select>
                <q-input v-model.number="budgetUSD" class="col-12 col-md-6" dense outlined type="number" prefix="$" label="月度预算" />
              </div>
            </section>

            <section class="settings-section">
              <div class="section-heading">
                <div>
                  <div class="text-subtitle1 text-weight-bold">能力</div>
                  <div class="text-caption text-grey-7">子 Agent 与工具策略。冲突工具会在保存前提示。</div>
                </div>
              </div>
              <div class="row q-col-gutter-md">
                <div class="col-12 col-lg-6">
                  <q-card flat bordered class="capability-card">
                    <q-card-section class="row items-center justify-between">
                      <div>
                        <div class="text-subtitle2">子 Agent</div>
                        <div class="text-caption text-grey-7">控制生成限制和归档策略。</div>
                      </div>
                      <q-toggle v-model="config.subagents.enabled" color="primary" />
                    </q-card-section>
                    <q-separator />
                    <q-card-section v-if="config.subagents.enabled" class="row q-col-gutter-sm">
                      <q-input v-model.number="config.subagents.max_concurrency" class="col-6" dense outlined type="number" label="最大并发数" />
                      <q-input v-model.number="config.subagents.max_generation_depth" class="col-6" dense outlined type="number" label="最大生成深度" />
                      <q-input v-model.number="config.subagents.max_children_per_agent" class="col-6" dense outlined type="number" label="每 Agent 最大子数" />
                      <q-input v-model.number="config.subagents.archive_after_minutes" class="col-6" dense outlined type="number" label="归档时间 (分钟)" />
                      <q-input v-model.number="config.subagents.max_retries" class="col-6" dense outlined type="number" label="最大重试次数" />
                      <q-input v-model="config.subagents.model_override" class="col-6" dense outlined label="模型覆盖" placeholder="继承自 Agent" />
                    </q-card-section>
                  </q-card>
                </div>
                <div class="col-12 col-lg-6">
                  <q-card flat bordered class="capability-card">
                    <q-card-section class="row items-center justify-between">
                      <div>
                        <div class="text-subtitle2">工具策略</div>
                        <div class="text-caption text-grey-7">控制可调用工具、黑名单与并行白名单。</div>
                      </div>
                      <q-toggle v-model="config.tools.enabled" color="primary" />
                    </q-card-section>
                    <q-separator />
                    <q-card-section v-if="config.tools.enabled" class="q-gutter-sm">
                      <q-select v-model="config.tools.profile" dense outlined label="配置文件" :options="['full', 'safe', 'minimal']" />
                      <q-input v-model="config.tools.tool_call_prefix" dense outlined label="工具调用前缀" hint="如 proxy_，解析前会从工具名中剥离。" />
                      <q-select v-model="config.tools.allow" dense outlined multiple use-chips label="允许" :options="toolOptions" />
                      <q-select v-model="config.tools.deny" dense outlined multiple use-chips label="拒绝" :options="toolOptions" />
                      <q-select v-model="config.tools.concurrent_allow" dense outlined multiple use-chips label="同时允许" :options="toolOptions" />
                      <q-banner v-if="toolConflicts.length" rounded class="settings-warning-banner">
                        以下工具同时出现在允许与拒绝中，运行时按拒绝优先：{{ toolConflicts.join(", ") }}
                      </q-banner>
                    </q-card-section>
                  </q-card>
                </div>
              </div>
            </section>

            <section class="settings-section">
              <div class="section-heading">
                <div>
                  <div class="text-subtitle1 text-weight-bold">记忆与心跳</div>
                  <div class="text-caption text-grey-7">语义检索、Dreaming 与 HEARTBEAT.MD 注入。</div>
                </div>
              </div>
              <div class="row q-col-gutter-md">
                <q-toggle v-model="config.memory.enabled" class="col-12 col-md-3" color="primary" label="记忆启用" />
                <q-input v-model.number="config.memory.max_chunk_length" class="col-12 col-md-3" dense outlined type="number" label="最大块长度" />
                <q-input v-model.number="config.memory.max_results" class="col-12 col-md-3" dense outlined type="number" label="最大结果数" />
                <q-input v-model.number="config.memory.min_score" class="col-12 col-md-3" dense outlined type="number" step="0.01" label="最低分数" />
                <q-toggle v-model="config.heartbeat.enabled" class="col-12 col-md-3" color="negative" label="心跳启用" />
                <q-input v-model.number="config.heartbeat.interval_minutes" class="col-12 col-md-3" dense outlined type="number" suffix="min" label="间隔" />
                <q-input v-model="heartbeatFile.body" class="col-12 col-md-6" dense outlined autogrow type="textarea" label="检查清单 (HEARTBEAT.MD)" />
              </div>
            </section>
          </div>
        </q-tab-panel>

        <q-tab-panel name="files">
          <agent-files-panel
            v-model:active-file="activeFile"
            v-model:splitter="fileSplitter"
            :files="files"
            :dirty="fileDirty"
            @update-file-body="updateFileBody"
            @reload="reloadActiveFile"
            @ai-edit="aiEditOpen = true"
            @save="saveAgent"
          />
        </q-tab-panel>

        <q-tab-panel name="permissions">
          <q-banner rounded class="settings-placeholder-banner">权限与用户可见范围将按独立 PRD 接入。当前保留入口。</q-banner>
        </q-tab-panel>

        <q-tab-panel name="evolution">
          <agent-evolution-panel v-model:range="evolutionRange" :evolution="config.evolution" :guardrails="config.evolution_guardrails" />
        </q-tab-panel>

        <q-tab-panel name="hooks">
          <q-banner rounded class="settings-placeholder-banner">Hook 绑定入口已保留。全局 Hook 管理见左侧 Tools / Hook 页面。</q-banner>
        </q-tab-panel>

        <q-tab-panel name="instances">
          <q-banner rounded class="settings-placeholder-banner">用户实例用于按用户覆盖 USER.md、权限与默认上下文；当前保留入口。</q-banner>
        </q-tab-panel>
      </q-tab-panels>
    </q-card>

    <q-dialog v-model="promptDialog">
      <q-card class="prompt-dialog">
        <q-card-section class="row items-center justify-between">
          <div>
            <div class="text-h6">系统提示词</div>
            <div class="text-caption text-grey-7">{{ tokenEstimateFor(promptPreview) }} tokens</div>
          </div>
          <q-btn flat round icon="close" v-close-popup />
        </q-card-section>
        <q-tabs v-model="previewMode" dense active-color="primary" indicator-color="primary">
          <q-tab v-for="mode in promptModes" :key="mode.value" :name="mode.value" :label="mode.label" />
        </q-tabs>
        <q-separator />
        <q-card-section>
          <pre class="agent-prompt-preview">{{ promptPreview }}</pre>
        </q-card-section>
      </q-card>
    </q-dialog>

    <q-dialog v-model="aiEditOpen">
      <q-card style="width: 560px; max-width: 92vw">
        <q-card-section class="row items-center justify-between">
          <div>
            <div class="text-h6">AI 编辑</div>
            <div class="text-caption text-grey-7">描述您想要更改的内容。AI 将读取当前文件并相应更新。</div>
          </div>
          <q-btn flat round icon="close" v-close-popup />
        </q-card-section>
        <q-card-section>
          <q-input v-model="aiInstruction" outlined type="textarea" rows="6" label="编辑指令" placeholder="例如：使 Agent 更正式、添加中文支持..." />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat rounded label="取消" v-close-popup />
          <q-btn color="primary" rounded unelevated label="重新生成" @click="applyAiEditPlaceholder" />
        </q-card-actions>
      </q-card>
    </q-dialog>
    <agent-avatar-picker v-model="form.icon" v-model:open="avatarPickerOpen" />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { copyToClipboard, useQuasar } from "quasar";
import { useRoute, useRouter } from "vue-router";
import { getAgent, getAgentPromptPreview, updateAgent, type Agent, type AgentPromptFile, type AgentRuntimeSettings } from "../api/client";
import AgentAvatarPicker from "../components/agents/AgentAvatarPicker.vue";
import AgentEvolutionPanel from "../components/agents/AgentEvolutionPanel.vue";
import AgentFilesPanel from "../components/agents/AgentFilesPanel.vue";
import AgentSettingsHeader from "../components/agents/AgentSettingsHeader.vue";
import {
  defaultAgentFiles,
  promptModes,
  statusOptions,
  tokenEstimateFor,
  type AgentFile,
  type EvolutionKey,
  type PromptMode
} from "../components/agents/agentUi";
import { listPlatformResources, type PlatformResource } from "../features/platform/api";
import { useAppStore } from "../stores/app";

const $q = useQuasar();
const route = useRoute();
const router = useRouter();
const store = useAppStore();
const tab = ref("agent");
const saving = ref(false);
const promptDialog = ref(false);
const previewMode = ref<PromptMode>("complete");
const promptPreview = ref("");
const fileSplitter = ref(28);
const activeFile = ref("AGENTS_CORE.md");
const initialFileBodies = ref<Record<string, string>>({});
const aiEditOpen = ref(false);
const avatarPickerOpen = ref(false);
const aiInstruction = ref("");
const evolutionRange = ref("30d");
const providerModels = ref<PlatformResource[]>([]);
const providerModelSearch = ref("");
const loadingProviderModels = ref(false);

const form = reactive<Agent>({
  id: "",
  agent_key: "",
  display_name: "",
  provider: "",
  model: "",
  status: "active",
  is_default: false,
  is_favorite: false,
  icon: "",
  agent_description: "",
  category_position_id: "",
  system_prompt_mode: "complete",
  context_window: 0,
  budget_monthly_cents: 0,
  config_json: "",
  created_at: "",
  updated_at: "",
  deleted_at: ""
});

const config = reactive({
  self_evolve: true,
  subagents: {
    enabled: true,
    max_concurrency: 20,
    max_generation_depth: 1,
    max_children_per_agent: 5,
    archive_after_minutes: 60,
    max_retries: 2,
    model_override: ""
  },
  tools: {
    enabled: true,
    profile: "full",
    tool_call_prefix: "",
    allow: [] as string[],
    deny: [] as string[],
    concurrent_allow: [] as string[]
  },
  memory: {
    enabled: true,
    max_chunk_length: 1000,
    max_results: 6,
    min_score: 0.35
  },
  heartbeat: {
    enabled: false,
    interval_minutes: 30
  },
  evolution: {
    self_evolve: true,
    skill_evolve: true,
    evolution_metrics_enabled: true,
    evolution_suggestions_enabled: true
  } as Record<EvolutionKey, boolean>,
  evolution_guardrails: {
    max_change_per_period: 0.1,
    min_data_points: 100,
    rollback_on_decline_percent: 20
  }
});

const files = reactive<AgentFile[]>(defaultAgentFiles.map((file) => ({ ...file })));

const heartbeatFile = computed(() => files.find((file) => file.name === "HEARTBEAT.md") ?? files[0]);
const activeFileMeta = computed(() => files.find((file) => file.name === activeFile.value) ?? files[0]);
const activeFileBody = computed({
  get: () => activeFileMeta.value.body,
  set: (value: string) => {
    activeFileMeta.value.body = value;
  }
});
const fileDirty = computed(() => activeFileBody.value !== (initialFileBodies.value[activeFile.value] ?? ""));
const budgetUSD = computed({
  get: () => Math.round((form.budget_monthly_cents || 0) / 100),
  set: (value: number) => {
    form.budget_monthly_cents = Math.round((Number(value) || 0) * 100);
  }
});
const providerModelOptions = computed(() =>
  providerModels.value
    .filter((row) => row.enabled && row.provider && row.model)
    .map((row) => {
      const contextWindowK = providerContextWindowK(row);
      return {
        label: row.name || row.model,
        value: row.id,
        caption: `${row.provider} / ${row.model}${contextWindowK ? ` · ${contextWindowK}K ctx` : ""}`,
        provider: row.provider,
        model: row.model
      };
    })
);
const filteredProviderModelOptions = computed(() => {
  const keyword = providerModelSearch.value.trim().toLowerCase();
  if (!keyword) return providerModelOptions.value;
  return providerModelOptions.value.filter((option) =>
    [option.label, option.caption, option.provider, option.model].some((value) => value.toLowerCase().includes(keyword))
  );
});
const selectedProviderModelID = computed(() => providerModelOptions.value.find((row) => row.provider === form.provider && row.model === form.model)?.value ?? "");
const toolOptions = ["browser", "edit", "list_files", "read_file", "write_file", "create_image", "create_video", "stt"];
const toolConflicts = computed(() => config.tools.allow.filter((tool) => config.tools.deny.includes(tool)));

onMounted(async () => {
  const [agent] = await Promise.all([getAgent(String(route.params.id)), loadProviderModels()]);
  if (!agent) return;
  Object.assign(form, agent);
  hydrateSettings(agent);
  store.upsertAgent(agent);
  snapshotFiles();
  previewMode.value = (form.system_prompt_mode as PromptMode) || "complete";
  await loadPromptPreview();
});

watch(previewMode, () => void loadPromptPreview());
watch(
  () => form.system_prompt_mode,
  (value) => {
    previewMode.value = (value as PromptMode) || "complete";
    config.evolution.self_evolve = config.self_evolve;
  }
);
watch(
  () => config.evolution.self_evolve,
  (value) => {
    config.self_evolve = value;
  }
);

function hydrateConfig(raw: string) {
  try {
    const parsed = JSON.parse(raw || "{}");
    Object.assign(config, {
      ...config,
      ...parsed,
      subagents: { ...config.subagents, ...(parsed.subagents || {}) },
      tools: { ...config.tools, ...(parsed.tools || {}) },
      memory: { ...config.memory, ...(parsed.memory || {}) },
      heartbeat: { ...config.heartbeat, ...(parsed.heartbeat || {}) },
      evolution: { ...config.evolution, ...(parsed.evolution || {}), self_evolve: parsed.self_evolve ?? config.self_evolve },
      evolution_guardrails: { ...config.evolution_guardrails, ...(parsed.evolution_guardrails || {}) }
    });
    if (Array.isArray(parsed.files)) {
      for (const saved of parsed.files) {
        const file = files.find((item) => item.name === saved.name);
        if (file) file.body = saved.body;
      }
    }
  } catch {
    // Legacy config can be plain text; keep defaults.
  }
}

function hydrateSettings(agent: Agent) {
  if (agent.settings) {
    Object.assign(config, {
      ...config,
      self_evolve: agent.settings.self_evolve,
      subagents: {
        enabled: agent.settings.subagents_enabled,
        max_concurrency: agent.settings.subagents_max_concurrency,
        max_generation_depth: agent.settings.subagents_max_generation_depth,
        max_children_per_agent: agent.settings.subagents_max_children_per_agent,
        archive_after_minutes: agent.settings.subagents_archive_after_minutes,
        max_retries: agent.settings.subagents_max_retries,
        model_override: agent.settings.subagents_model_override
      },
      tools: {
        enabled: agent.settings.tools_enabled,
        profile: agent.settings.tools_profile,
        tool_call_prefix: agent.settings.tools_tool_call_prefix,
        allow: parseJSONList(agent.settings.tools_allow_json),
        deny: parseJSONList(agent.settings.tools_deny_json),
        concurrent_allow: parseJSONList(agent.settings.tools_concurrent_allow_json)
      },
      memory: {
        enabled: agent.settings.memory_enabled,
        max_chunk_length: agent.settings.memory_max_chunk_length,
        max_results: agent.settings.memory_max_results,
        min_score: agent.settings.memory_min_score
      },
      heartbeat: {
        enabled: agent.settings.heartbeat_enabled,
        interval_minutes: agent.settings.heartbeat_interval_minutes
      },
      evolution: {
        self_evolve: agent.settings.evolution_self_evolve,
        skill_evolve: agent.settings.evolution_skill_evolve,
        evolution_metrics_enabled: agent.settings.evolution_metrics_enabled,
        evolution_suggestions_enabled: agent.settings.evolution_suggestions_enabled
      },
      evolution_guardrails: {
        max_change_per_period: agent.settings.guardrail_max_change_per_period,
        min_data_points: agent.settings.guardrail_min_data_points,
        rollback_on_decline_percent: agent.settings.guardrail_rollback_on_decline_percent
      }
    });
  } else {
    hydrateConfig(agent.config_json);
  }
  if (agent.files?.length) {
    hydrateFiles(agent.files);
  }
}

async function saveAgent() {
  if (!selectedProviderModelID.value) {
    $q.notify({ type: "negative", message: "请选择已录入且启用的模型" });
    return;
  }
  saving.value = true;
  try {
    const updated = await updateAgent(form.id, {
      ...form,
      settings: buildSettingsPayload(),
      files: files.map((file, index) => ({ name: file.name, body: file.body, sort_order: (index + 1) * 10 })),
      config_json: JSON.stringify({
        self_evolve: config.self_evolve,
        subagents: config.subagents,
        tools: config.tools,
        memory: config.memory,
        heartbeat: config.heartbeat,
        evolution: config.evolution,
        evolution_guardrails: config.evolution_guardrails,
        files: files.map((file) => ({ name: file.name, body: file.body }))
      })
    });
    Object.assign(form, updated);
    hydrateSettings(updated);
    store.upsertAgent(updated);
    snapshotFiles();
    await loadPromptPreview();
    $q.notify({ type: "positive", message: "已保存" });
  } finally {
    saving.value = false;
  }
}

async function toggleFavorite() {
  const next = !form.is_favorite;
  form.is_favorite = next;
  try {
    const updated = await updateAgent(form.id, { ...form, is_favorite: next, settings: buildSettingsPayload(), files: files.map((file, index) => ({ name: file.name, body: file.body, sort_order: (index + 1) * 10 })) });
    Object.assign(form, updated);
    hydrateSettings(updated);
    store.upsertAgent(updated);
  } catch (error) {
    form.is_favorite = !next;
    $q.notify({ type: "negative", message: error instanceof Error ? error.message : "收藏保存失败" });
  }
}

async function loadPromptPreview() {
  if (!form.id) return;
  promptPreview.value = await getAgentPromptPreview(form.id, previewMode.value);
}

async function loadProviderModels() {
  loadingProviderModels.value = true;
  try {
    providerModels.value = await listPlatformResources("llm-provider-models");
  } finally {
    loadingProviderModels.value = false;
  }
}

function selectProviderModel(value: string | null) {
  const selected = providerModels.value.find((row) => row.id === value);
  if (!selected) {
    form.provider = "";
    form.model = "";
    return;
  }
  form.provider = selected.provider;
  form.model = selected.model;
}

function filterProviderModels(value: string, update: (callback: () => void) => void) {
  update(() => {
    providerModelSearch.value = value;
  });
}

function providerContextWindowK(row: PlatformResource) {
  try {
    const parsed = JSON.parse(row.config_json || "{}") as { context_window_k?: number | string | null };
    const value = Number(parsed.context_window_k);
    return Number.isFinite(value) && value > 0 ? value : null;
  } catch {
    return null;
  }
}

function reloadActiveFile() {
  activeFileBody.value = initialFileBodies.value[activeFile.value] ?? activeFileBody.value;
}

function updateFileBody(name: string, body: string) {
  const file = files.find((item) => item.name === name);
  if (file) file.body = body;
}

function snapshotFiles() {
  initialFileBodies.value = Object.fromEntries(files.map((file) => [file.name, file.body]));
}

function hydrateFiles(savedFiles: AgentPromptFile[]) {
  const byName = new Map(savedFiles.map((file) => [file.name, file]));
  for (const file of files) {
    const saved = byName.get(file.name);
    if (saved) file.body = saved.body;
  }
  for (const saved of savedFiles) {
    if (!files.some((file) => file.name === saved.name)) {
      files.push({ name: saved.name, caption: "自定义 Prompt 文件", body: saved.body });
    }
  }
}

function buildSettingsPayload(): AgentRuntimeSettings {
  return {
    self_evolve: config.self_evolve,
    subagents_enabled: config.subagents.enabled,
    subagents_max_concurrency: config.subagents.max_concurrency,
    subagents_max_generation_depth: config.subagents.max_generation_depth,
    subagents_max_children_per_agent: config.subagents.max_children_per_agent,
    subagents_archive_after_minutes: config.subagents.archive_after_minutes,
    subagents_max_retries: config.subagents.max_retries,
    subagents_model_override: config.subagents.model_override,
    tools_enabled: config.tools.enabled,
    tools_profile: config.tools.profile,
    tools_tool_call_prefix: config.tools.tool_call_prefix,
    tools_allow_json: JSON.stringify(config.tools.allow),
    tools_deny_json: JSON.stringify(config.tools.deny),
    tools_concurrent_allow_json: JSON.stringify(config.tools.concurrent_allow),
    memory_enabled: config.memory.enabled,
    memory_max_chunk_length: config.memory.max_chunk_length,
    memory_max_results: config.memory.max_results,
    memory_min_score: config.memory.min_score,
    heartbeat_enabled: config.heartbeat.enabled,
    heartbeat_interval_minutes: config.heartbeat.interval_minutes,
    evolution_self_evolve: config.evolution.self_evolve,
    evolution_skill_evolve: config.evolution.skill_evolve,
    evolution_metrics_enabled: config.evolution.evolution_metrics_enabled,
    evolution_suggestions_enabled: config.evolution.evolution_suggestions_enabled,
    guardrail_max_change_per_period: config.evolution_guardrails.max_change_per_period,
    guardrail_min_data_points: config.evolution_guardrails.min_data_points,
    guardrail_rollback_on_decline_percent: config.evolution_guardrails.rollback_on_decline_percent
  };
}

function parseJSONList(raw: string) {
  try {
    const parsed = JSON.parse(raw || "[]");
    return Array.isArray(parsed) ? parsed.map(String) : [];
  } catch {
    return [];
  }
}

function applyAiEditPlaceholder() {
  if (!aiInstruction.value.trim()) return;
  activeFileBody.value = `${activeFileBody.value}\n\n<!-- AI edit instruction: ${aiInstruction.value.trim()} -->`;
  aiInstruction.value = "";
  aiEditOpen.value = false;
}

async function copyKey() {
  await copyToClipboard(form.agent_key);
  $q.notify({ type: "positive", message: "Agent 标识已复制" });
}
</script>

<style scoped>
.agent-settings {
  min-height: 100%;
  padding: 28px;
  background:
    radial-gradient(circle at 12% 0%, rgba(25, 118, 210, 0.1), transparent 30%),
    radial-gradient(circle at 90% 12%, rgba(255, 152, 0, 0.08), transparent 26%),
    linear-gradient(180deg, #fbfcff 0%, #f7f9fc 48%, #ffffff 100%);
}

.settings-shell,
.settings-section {
  border-radius: 24px;
}

.settings-tabs {
  padding: 8px 16px 0;
  background: rgba(255, 255, 255, 0.84);
}

.settings-tabs :deep(.q-tab) {
  min-height: 46px;
  border-radius: 14px 14px 0 0;
  font-weight: 700;
}

.settings-panels {
  background: transparent;
}

.settings-panels :deep(.q-tab-panel) {
  padding: 18px;
}

.settings-shell {
  border: 1px solid rgba(15, 23, 42, 0.08);
  background: rgba(255, 255, 255, 0.82);
  box-shadow: 0 22px 70px rgba(16, 24, 40, 0.08);
  overflow: hidden;
  backdrop-filter: blur(16px);
}

.settings-grid {
  display: grid;
  gap: 18px;
}

.settings-section {
  padding: 20px;
  border: 1px solid rgba(0, 0, 0, 0.08);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(255, 255, 255, 0.92)),
    radial-gradient(circle at top right, rgba(25, 118, 210, 0.05), transparent 30%);
  box-shadow: 0 14px 36px rgba(16, 24, 40, 0.045);
}

.section-heading {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 14px;
}

.prompt-mode-card {
  height: 100%;
  border-color: rgba(15, 23, 42, 0.08);
  border-radius: 18px;
  background: #fbfcff;
  transition:
    transform 180ms ease,
    box-shadow 180ms ease,
    border-color 180ms ease;
}

.prompt-mode-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 16px 34px rgba(16, 24, 40, 0.08);
}

.prompt-mode-card.is-active {
  border-color: var(--q-primary);
  background: #eef6ff;
  box-shadow: 0 0 0 1px rgba(25, 118, 210, 0.22), 0 16px 34px rgba(25, 118, 210, 0.12);
}

.prompt-mode-card__token {
  background: #ffffff;
  color: #475467;
  font-weight: 700;
}

.capability-card {
  border-color: rgba(15, 23, 42, 0.08);
  border-radius: 20px;
  overflow: hidden;
  background: #fbfcff;
}

.capability-card :deep(.q-card__section:first-child) {
  background: #ffffff;
}

.settings-section :deep(.q-field__control) {
  border-radius: 14px;
  background: #ffffff;
}

.settings-section :deep(.q-toggle__label) {
  font-weight: 600;
  color: #344054;
}

.settings-warning-banner {
  background: #fff7ed;
  color: #9a3412;
}

.settings-placeholder-banner {
  background: #f2f4f7;
  color: #344054;
}

.prompt-dialog {
  width: 860px;
  max-width: 94vw;
  border-radius: 24px;
  box-shadow: 0 28px 80px rgba(16, 24, 40, 0.18);
}

.agent-prompt-preview {
  max-height: 64vh;
  overflow: auto;
  margin: 0;
  white-space: pre-wrap;
  padding: 16px;
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 16px;
  background: #f8fafc;
  color: #1d2939;
  line-height: 1.6;
  font-family: ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", monospace;
}

.min-width-0 {
  min-width: 0;
}

body.body--dark .agent-settings {
  background:
    radial-gradient(circle at 12% 0%, rgba(25, 118, 210, 0.18), transparent 32%),
    radial-gradient(circle at 90% 12%, rgba(255, 152, 0, 0.12), transparent 28%),
    linear-gradient(180deg, #0f172a 0%, #111827 48%, #0b1120 100%);
}

body.body--dark .settings-shell {
  border-color: rgba(148, 163, 184, 0.18);
  background: rgba(17, 24, 39, 0.86);
  box-shadow: 0 22px 70px rgba(0, 0, 0, 0.32);
}

body.body--dark .settings-tabs {
  background: rgba(17, 24, 39, 0.82);
}

body.body--dark .settings-section {
  border-color: rgba(148, 163, 184, 0.18);
  background:
    linear-gradient(180deg, rgba(30, 41, 59, 0.96), rgba(15, 23, 42, 0.92)),
    radial-gradient(circle at top right, rgba(59, 130, 246, 0.14), transparent 32%);
  box-shadow: 0 14px 36px rgba(0, 0, 0, 0.24);
}

body.body--dark .prompt-mode-card,
body.body--dark .capability-card {
  border-color: rgba(148, 163, 184, 0.18);
  background: rgba(15, 23, 42, 0.9);
}

body.body--dark .prompt-mode-card:hover {
  box-shadow: 0 16px 34px rgba(0, 0, 0, 0.28);
}

body.body--dark .prompt-mode-card.is-active {
  border-color: rgba(96, 165, 250, 0.88);
  background: rgba(30, 64, 175, 0.32);
  box-shadow: 0 0 0 1px rgba(96, 165, 250, 0.28), 0 16px 34px rgba(37, 99, 235, 0.2);
}

body.body--dark .prompt-mode-card__token {
  background: rgba(30, 41, 59, 0.94);
  color: #cbd5e1;
}

body.body--dark .capability-card :deep(.q-card__section:first-child) {
  background: rgba(30, 41, 59, 0.86);
}

body.body--dark .settings-section :deep(.q-field__control) {
  background: rgba(15, 23, 42, 0.72);
}

body.body--dark .settings-section :deep(.q-toggle__label) {
  color: #e2e8f0;
}

body.body--dark .settings-section :deep(.text-grey-7) {
  color: #94a3b8 !important;
}

body.body--dark .settings-warning-banner {
  background: rgba(154, 52, 18, 0.22);
  color: #fed7aa;
}

body.body--dark .settings-placeholder-banner {
  background: rgba(30, 41, 59, 0.82);
  color: #cbd5e1;
}

body.body--dark .prompt-dialog {
  background: #111827;
  box-shadow: 0 28px 80px rgba(0, 0, 0, 0.38);
}

body.body--dark .agent-prompt-preview {
  border-color: rgba(148, 163, 184, 0.2);
  background: #0f172a;
  color: #e2e8f0;
}

@media (max-width: 599px) {
  .agent-settings {
    padding: 18px;
  }

  .settings-panels :deep(.q-tab-panel) {
    padding: 14px;
  }
}
</style>
