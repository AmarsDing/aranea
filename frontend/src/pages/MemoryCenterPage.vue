<template>
  <q-page class="memory-page q-pa-md">
    <memory-hero v-model:selected-agent-id="selectedAgentId" :agent-options="agentOptions" :loading="loading" @refresh="loadAll" />

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadAll" />
      </template>
    </q-banner>

    <memory-metric-cards :cards="overviewCards" />

    <q-card flat class="memory-tabs-card">
      <q-tabs v-model="tab" align="left" active-color="primary" indicator-color="primary" no-caps outside-arrows mobile-arrows>
        <q-tab name="overview" icon="hub" label="总览" />
        <q-tab name="knowledge" icon="psychology" label="知识库" />
        <q-tab name="sessions" icon="account_tree" label="会话记忆" />
        <q-tab name="evolution" icon="auto_awesome" label="图谱与进化" />
        <q-tab name="settings" icon="tune" label="设置" />
      </q-tabs>
    </q-card>

    <q-tab-panels v-model="tab" animated class="memory-panels">
      <q-tab-panel name="overview">
        <memory-overview-panel :memory-layers="memoryLayers" :action-items="actionItems" />
      </q-tab-panel>

      <q-tab-panel name="knowledge">
        <memory-knowledge-panel
          v-model:fact-keyword="factKeyword"
          v-model:fact-scope="factScope"
          v-model:fact-status="factStatus"
          :facts-endpoint-ready="factsEndpointReady"
          :scope-options="scopeOptions"
          :fact-status-options="factStatusOptions"
          :fact-rows="factRows"
          :fact-columns="factColumns"
          :loading-facts="loadingFacts"
          @reset="resetFactFilters"
          @search="loadFacts"
          @open-fact="openFact"
        />
      </q-tab-panel>

      <q-tab-panel name="sessions">
        <memory-sessions-panel
          v-model:selected-session-id="selectedSessionId"
          :session-rows="sessionRows"
          :loading-sessions="loadingSessions"
          :snapshot-rows="snapshotRows"
          :snapshot-columns="snapshotColumns"
          :loading-snapshots="loadingSnapshots"
          :task-rows="taskRows"
          @refresh-sessions="loadSessions"
          @refresh-memory="loadSessionMemory"
          @open-snapshot="openSnapshot"
        />
      </q-tab-panel>

      <q-tab-panel name="evolution">
        <memory-evolution-panel :panels="evolutionPanels" />
      </q-tab-panel>

      <q-tab-panel name="settings">
        <memory-settings-status-panel :items="settingChecklist" />
      </q-tab-panel>
    </q-tab-panels>

    <memory-snapshot-drawer v-model="snapshotDrawer" :snapshot="selectedSnapshot" />
    <memory-fact-drawer v-model="factDrawer" :fact="selectedFact" />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import MemoryEvolutionPanel from "../features/memory/MemoryEvolutionPanel.vue";
import MemoryFactDrawer from "../features/memory/MemoryFactDrawer.vue";
import MemoryHero from "../features/memory/MemoryHero.vue";
import MemoryKnowledgePanel from "../features/memory/MemoryKnowledgePanel.vue";
import MemoryMetricCards from "../features/memory/MemoryMetricCards.vue";
import MemoryOverviewPanel from "../features/memory/MemoryOverviewPanel.vue";
import MemorySessionsPanel from "../features/memory/MemorySessionsPanel.vue";
import MemorySettingsStatusPanel from "../features/memory/MemorySettingsStatusPanel.vue";
import MemorySnapshotDrawer from "../features/memory/MemorySnapshotDrawer.vue";
import {
  listAgents,
  listL0Snapshots,
  listL1Tasks,
  listMemoryFacts,
  searchSessions,
  type Agent,
  type L0AssemblySnapshot,
  type L1Task,
  type MemoryFact,
  type Session
} from "../api/client";

const tab = ref("overview");
const agents = ref<Agent[]>([]);
const sessions = ref<Session[]>([]);
const facts = ref<MemoryFact[]>([]);
const snapshots = ref<L0AssemblySnapshot[]>([]);
const tasks = ref<L1Task[]>([]);
const selectedAgentId = ref<string | null>(null);
const selectedSessionId = ref<string | null>(null);
const selectedSnapshot = ref<L0AssemblySnapshot | null>(null);
const selectedFact = ref<MemoryFact | null>(null);
const factKeyword = ref("");
const factScope = ref<string | null>(null);
const factStatus = ref<string | null>("active");
const loadingAgents = ref(false);
const loadingSessions = ref(false);
const loadingFacts = ref(false);
const loadingSnapshots = ref(false);
const loadingTasks = ref(false);
const factsEndpointReady = ref(true);
const snapshotDrawer = ref(false);
const factDrawer = ref(false);
const error = ref("");

const loading = computed(() => loadingAgents.value || loadingSessions.value || loadingFacts.value || loadingSnapshots.value || loadingTasks.value);
const agentOptions = computed(() => agents.value.map((agent) => ({ label: agent.display_name || agent.agent_key, value: agent.id })));
const sessionRows = computed(() => sessions.value);
const factRows = computed(() => facts.value);
const snapshotRows = computed(() => snapshots.value);
const taskRows = computed(() => tasks.value);

const overviewCards = computed(() => {
  const avgContext = sessions.value.length ? sessions.value.reduce((sum, session) => sum + (session.context_used_ratio || 0), 0) / sessions.value.length : 0;
  const riskySessions = sessions.value.filter((session) => ["warning", "critical", "exceeded"].includes(session.context_status)).length;
  const activeTasks = tasks.value.filter((task) => task.status === "active" || task.status === "paused").length;
  return [
    { label: "上下文风险", value: riskySessions, hint: `平均占用 ${formatPercent(avgContext)}`, icon: "speed", color: contextRatioColor(avgContext) },
    { label: "活跃任务", value: activeTasks, hint: "L1 working memory tasks", icon: "assignment", color: "primary" },
    { label: "长期知识", value: facts.value.length, hint: factsEndpointReady.value ? "已加载 L3 facts" : "等待 L3 API 接入", icon: "psychology", color: "deep-purple" },
    { label: "Prompt 快照", value: snapshots.value.length, hint: "最近 L0 assembly snapshots", icon: "preview", color: "blue-grey" }
  ];
});

const actionItems = computed(() => [
  { title: "上下文接近上限", caption: "建议检查摘要阈值和注入片段数量。", count: sessions.value.filter((s) => ["warning", "critical", "exceeded"].includes(s.context_status)).length, icon: "report", color: "warning" },
  { title: "知识冲突待办", caption: "L3 conflict API 接入后展示需要仲裁的 facts。", count: facts.value.reduce((sum, fact) => sum + (fact.conflict_count || 0), 0), icon: "rule", color: "negative" },
  { title: "待审核进化提议", caption: "L4 evolution API 接入后展示 Agent 自我修改建议。", count: 0, icon: "auto_awesome", color: "info" }
]);

const memoryLayers = [
  { key: "l0", title: "上下文窗口 L0", caption: "下一次模型调用实际看到的材料。", icon: "preview", color: "primary", status: "已接入", statusColor: "positive" },
  { key: "l1", title: "工作记忆 L1", caption: "当前任务目标、约束、决策和中间结果。", icon: "assignment", color: "indigo", status: "已接入", statusColor: "positive" },
  { key: "l2", title: "事件记忆 L2", caption: "会话 timeline、episode、marks 与巩固队列。", icon: "timeline", color: "teal", status: "设计中", statusColor: "warning" },
  { key: "l3", title: "知识记忆 L3", caption: "跨会话 facts、偏好、规则、冲突与反馈。", icon: "psychology", color: "deep-purple", status: "待 API", statusColor: "warning" },
  { key: "l4", title: "图谱与进化 L4", caption: "实体关系、Agent identity、strategy 和 proposal。", icon: "auto_awesome", color: "orange", status: "待 API", statusColor: "warning" }
];

const evolutionPanels = [
  { title: "知识图谱", caption: "实体、关系、证据链和邻居召回将汇总在这里。", state: "等待 `/memory/l4/entities` 与 neighborhood API 接入。", icon: "device_hub", color: "teal" },
  { title: "Agent Identity", caption: "persona、values、tone、domains 和用户期望。", state: "等待 `/agents/{id}/identity` API 接入。", icon: "badge", color: "primary" },
  { title: "Strategy Profile", caption: "探索度、简洁度、谨慎度、工具偏好和模型偏好。", state: "等待 `/agents/{id}/strategy` API 接入。", icon: "tune", color: "deep-purple" },
  { title: "Evolution Proposals", caption: "待审核的自我修正建议和回滚日志。", state: "等待 `/agents/{id}/evolution/proposals` API 接入。", icon: "rule", color: "orange" }
];

const settingChecklist = [
  { label: "基础 memory_* 设置", caption: "Agent 设置页已有旧版记忆启用、结果数和最低分数。", done: true },
  { label: "L0 上下文策略", caption: "后端字段已出现，前端设置表单待拆分成记忆 Tab。", done: false },
  { label: "L1 工作记忆预算", caption: "L1 task/field API 已接入，Agent 级设置待补。", done: false },
  { label: "L3 语义记忆设置", caption: "等待 facts/recall/conflict API 注册。", done: false },
  { label: "L4 图谱与进化设置", caption: "等待 graph/evolution API 注册。", done: false }
];

const scopeOptions = ["user", "agent", "team", "workspace", "global"].map((value) => ({ label: value, value }));
const factStatusOptions = ["active", "archived", "disputed", "deprecated", "deleted"].map((value) => ({ label: value, value }));

const factColumns = [
  { name: "statement", label: "Statement", field: "statement", align: "left", sortable: false },
  { name: "scope", label: "Scope", field: "scope_type", align: "left", sortable: false },
  { name: "confidence", label: "Confidence", field: "confidence", align: "left", sortable: false },
  { name: "source", label: "Source", field: "source_kind", align: "left", sortable: false },
  { name: "updated", label: "Updated", field: "updated_at", align: "left", sortable: false, format: formatDate },
  { name: "actions", label: "操作", field: "id", align: "right", sortable: false }
] as const;

const snapshotColumns = [
  { name: "created", label: "时间", field: "created_at", align: "left", sortable: false, format: formatDate },
  { name: "model", label: "模型", field: (row: L0AssemblySnapshot) => `${row.provider || "-"} / ${row.model || "-"}`, align: "left", sortable: false },
  { name: "ratio", label: "Used", field: "used_ratio", align: "left", sortable: false },
  { name: "segments", label: "段落", field: "segments_json", align: "left", sortable: false },
  { name: "strategy", label: "裁剪策略", field: "truncate_strategy", align: "left", sortable: false },
  { name: "actions", label: "操作", field: "id", align: "right", sortable: false }
] as const;

onMounted(loadAll);

watch(selectedAgentId, async () => {
  selectedSessionId.value = null;
  await loadSessions();
});

watch(selectedSessionId, () => {
  void loadSessionMemory();
});

watch([factKeyword, factScope, factStatus], () => {
  void loadFacts();
});

async function loadAll() {
  error.value = "";
  try {
    await Promise.all([loadAgents(), loadSessions(), loadFacts()]);
  } catch (err) {
    error.value = err instanceof Error ? err.message : "记忆中心加载失败";
  }
}

async function loadAgents() {
  loadingAgents.value = true;
  try {
    agents.value = await listAgents({ limit: 200 });
  } finally {
    loadingAgents.value = false;
  }
}

async function loadSessions() {
  loadingSessions.value = true;
  try {
    const result = await searchSessions({ agent_id: selectedAgentId.value || undefined, limit: 30 });
    sessions.value = result.items;
    if (!selectedSessionId.value && sessions.value.length) {
      selectedSessionId.value = sessions.value[0].id;
    }
  } finally {
    loadingSessions.value = false;
  }
}

async function loadFacts() {
  loadingFacts.value = true;
  try {
    const result = await listMemoryFacts({
      keyword: factKeyword.value || undefined,
      scope_type: factScope.value || undefined,
      status: factStatus.value || undefined,
      limit: 50
    });
    facts.value = result.items;
    factsEndpointReady.value = true;
  } catch {
    facts.value = [];
    factsEndpointReady.value = false;
  } finally {
    loadingFacts.value = false;
  }
}

async function loadSessionMemory() {
  if (!selectedSessionId.value) {
    snapshots.value = [];
    tasks.value = [];
    return;
  }
  await Promise.all([loadSnapshots(), loadTasks()]);
}

async function loadSnapshots() {
  if (!selectedSessionId.value) return;
  loadingSnapshots.value = true;
  try {
    snapshots.value = await listL0Snapshots(selectedSessionId.value, 20);
  } catch {
    snapshots.value = [];
  } finally {
    loadingSnapshots.value = false;
  }
}

async function loadTasks() {
  if (!selectedSessionId.value) return;
  loadingTasks.value = true;
  try {
    tasks.value = await listL1Tasks(selectedSessionId.value, { include_ended: true });
  } catch {
    tasks.value = [];
  } finally {
    loadingTasks.value = false;
  }
}

function resetFactFilters() {
  factKeyword.value = "";
  factScope.value = null;
  factStatus.value = "active";
}

function openSnapshot(row: L0AssemblySnapshot) {
  selectedSnapshot.value = row;
  snapshotDrawer.value = true;
}

function openFact(row: MemoryFact) {
  selectedFact.value = row;
  factDrawer.value = true;
}

function contextRatioColor(value?: number) {
  const ratio = Math.max(0, Math.min(1, Number(value) || 0));
  if (ratio >= 0.85) return "negative";
  if (ratio >= 0.6) return "warning";
  return "positive";
}

function formatPercent(value?: number) {
  return `${Math.round((Number(value) || 0) * 100)}%`;
}

function formatDate(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" });
}
</script>

<style>
.memory-page {
  min-height: 100%;
  background:
    radial-gradient(circle at 8% 0%, rgba(25, 118, 210, 0.12), transparent 30%),
    radial-gradient(circle at 92% 12%, rgba(156, 39, 176, 0.08), transparent 28%),
    linear-gradient(180deg, #fbfcff 0%, #f7f9fc 48%, #ffffff 100%);
}

.memory-hero {
  align-items: center;
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 4px 20px;
}

.memory-kicker {
  color: #1976d2;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.memory-title {
  color: #1d2939;
  font-size: clamp(30px, 4vw, 46px);
  font-weight: 900;
  letter-spacing: -0.04em;
  line-height: 1.05;
  margin: 4px 0;
}

.memory-subtitle {
  color: #667085;
  margin: 0;
  max-width: 760px;
}

.memory-select {
  min-width: 220px;
}

.memory-card,
.memory-metric-card,
.memory-tabs-card {
  border-radius: 18px;
}

.memory-tabs-card {
  margin-bottom: 12px;
  overflow: hidden;
}

.memory-panels {
  background: transparent;
}

.memory-panels .q-tab-panel {
  padding: 0;
}

.memory-flow {
  display: grid;
  gap: 12px;
}

.memory-flow-node {
  align-items: center;
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 16px;
  display: flex;
  gap: 12px;
  padding: 14px;
}

.memory-active-item {
  background: rgba(25, 118, 210, 0.08);
  color: var(--q-primary);
}

.memory-info-banner {
  background: #eef6ff;
  color: #175cd3;
}

.memory-pre {
  background: #f8fafc;
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 14px;
  color: #1d2939;
  line-height: 1.55;
  margin: 12px 0 0;
  max-height: 320px;
  overflow: auto;
  padding: 12px;
  white-space: pre-wrap;
}

.memory-drawer {
  background: #ffffff;
}

body.body--dark .memory-page {
  background:
    radial-gradient(circle at 8% 0%, rgba(59, 130, 246, 0.18), transparent 32%),
    radial-gradient(circle at 92% 12%, rgba(168, 85, 247, 0.13), transparent 30%),
    linear-gradient(180deg, #0f172a 0%, #111827 48%, #0b1120 100%);
}

body.body--dark .memory-title {
  color: #f8fafc;
}

body.body--dark .memory-subtitle,
body.body--dark .memory-page .text-grey-7 {
  color: #94a3b8 !important;
}

body.body--dark .memory-card,
body.body--dark .memory-metric-card,
body.body--dark .memory-tabs-card {
  background: rgba(17, 24, 39, 0.88);
  border-color: rgba(148, 163, 184, 0.18);
}

body.body--dark .memory-flow-node {
  border-color: rgba(148, 163, 184, 0.18);
  background: rgba(15, 23, 42, 0.5);
}

body.body--dark .memory-info-banner {
  background: rgba(30, 64, 175, 0.24);
  color: #bfdbfe;
}

body.body--dark .memory-pre {
  background: #0f172a;
  border-color: rgba(148, 163, 184, 0.2);
  color: #e2e8f0;
}

body.body--dark .memory-drawer {
  background: #111827;
}

@media (max-width: 800px) {
  .memory-hero {
    align-items: stretch;
    flex-direction: column;
  }

  .memory-select {
    min-width: 100%;
  }
}
</style>
