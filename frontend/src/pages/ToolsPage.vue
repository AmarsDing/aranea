<template>
  <q-page class="app-page-cream tools-page">
    <section class="tools-hero">
      <div>
        <div class="tools-kicker">Tool registry</div>
        <h1 class="tools-title">Tools 管理</h1>
        <p class="tools-subtitle">管理 Agent 可调用工具、风险级别、启用状态与调用质量。</p>
      </div>
      <q-btn outline rounded color="primary" icon="history" label="调用记录" :to="{ name: 'tool-runs' }" />
    </section>

    <div class="row q-col-gutter-md q-mb-md">
      <div v-for="card in summaryCards" :key="card.label" class="col-12 col-sm-6 col-lg-3">
        <q-card flat bordered class="tools-summary-card">
          <q-card-section>
            <div class="text-caption text-grey-7">{{ card.label }}</div>
            <div class="text-h5 text-weight-bold q-mt-xs">{{ card.value }}</div>
            <div class="text-caption text-grey-7 q-mt-xs">{{ card.hint }}</div>
          </q-card-section>
        </q-card>
      </div>
    </div>

    <q-card flat bordered class="tools-filter-card q-mb-md">
      <q-card-section class="row q-col-gutter-sm items-center">
        <div class="col-12 col-md-4">
          <q-input v-model="search" dense outlined clearable debounce="350" placeholder="搜索 Tool 名称、Key、描述...">
            <template #prepend><q-icon name="search" /></template>
          </q-input>
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-select v-model="category" dense outlined clearable emit-value map-options label="分类" :options="categoryOptions" />
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-select v-model="riskLevel" dense outlined clearable emit-value map-options label="风险" :options="riskOptions" />
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-select v-model="enabled" dense outlined clearable emit-value map-options label="启用状态" :options="enabledOptions" />
        </div>
        <div class="col-12 col-md-2 row justify-end q-gutter-sm">
          <q-btn flat rounded icon="restart_alt" label="重置" @click="resetFilters" />
          <q-btn flat rounded icon="refresh" label="刷新" :loading="loading" @click="loadRows" />
        </div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <q-table flat bordered class="tools-table" row-key="id" :rows="rows" :columns="columns" :loading="loading" :pagination="tablePagination" hide-pagination>
      <template #body-cell-name="props">
        <q-td :props="props">
          <div class="text-weight-medium">{{ props.row.display_name }}</div>
          <div class="text-caption text-grey-7">{{ props.row.key }}</div>
        </q-td>
      </template>

      <template #body-cell-category="props">
        <q-td :props="props">
          <q-chip dense color="primary" text-color="white">{{ props.row.category }}</q-chip>
          <q-chip dense outline color="grey" class="q-ml-xs">{{ props.row.source }}</q-chip>
        </q-td>
      </template>

      <template #body-cell-risk="props">
        <q-td :props="props">
          <q-badge rounded :color="riskColor(props.row.risk_level)">{{ riskLabel(props.row.risk_level) }}</q-badge>
          <div v-if="props.row.requires_confirmation" class="text-caption text-orange-8 q-mt-xs">需确认</div>
        </q-td>
      </template>

      <template #body-cell-enabled="props">
        <q-td :props="props">
          <q-toggle
            dense
            color="primary"
            :model-value="props.row.enabled"
            :disable="togglingId === props.row.id"
            :aria-label="`${props.row.display_name} 启用状态`"
            @update:model-value="requestToggleEnabled(props.row as Tool, Boolean($event))"
          />
        </q-td>
      </template>

      <template #body-cell-stats="props">
        <q-td :props="props">
          <div class="text-weight-medium">{{ props.row.invoke_count }} 次</div>
          <div class="text-caption text-grey-7">24h {{ props.row.invoke_count_24h }} · 失败 {{ props.row.failure_count }}</div>
        </q-td>
      </template>

      <template #body-cell-latency="props">
        <q-td :props="props">
          {{ props.row.avg_duration_ms == null ? "-" : `${Math.round(props.row.avg_duration_ms)}ms` }}
        </q-td>
      </template>

      <template #body-cell-last="props">
        <q-td :props="props">
          <div>{{ statusLabel(props.row.last_status) }}</div>
          <div class="text-caption text-grey-7">{{ formatDate(props.row.last_invoked_at) }}</div>
        </q-td>
      </template>

      <template #body-cell-actions="props">
        <q-td :props="props">
          <q-btn flat dense round color="primary" icon="visibility" :loading="detailLoading && detailTarget?.id === props.row.id" :aria-label="`查看 ${props.row.display_name}`" @click="openDetail(props.row as Tool)">
            <q-tooltip>查看详情</q-tooltip>
          </q-btn>
        </q-td>
      </template>
    </q-table>

    <skill-pagination v-model:page="page" v-model:page-size="pageSize" :page-max="pageMax" :total="total" :loading="loading" label="个 Tool" />

    <q-dialog v-model="detailOpen">
      <q-card class="tool-detail-card">
        <q-card-section class="row items-start justify-between q-gutter-md">
          <div>
            <div class="text-h6">{{ detailTarget?.display_name }}</div>
            <div class="text-caption text-grey-7">{{ detailTarget?.key }}</div>
          </div>
          <q-btn flat dense round icon="close" aria-label="关闭详情" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section v-if="detailTarget" class="q-gutter-md">
          <q-banner rounded class="bg-grey-2 text-grey-9">{{ detailTarget.description }}</q-banner>
          <div class="row q-col-gutter-sm">
            <div class="col-6"><b>分类：</b>{{ detailTarget.category }}</div>
            <div class="col-6"><b>来源：</b>{{ detailTarget.source }}</div>
            <div class="col-6"><b>风险：</b>{{ riskLabel(detailTarget.risk_level) }}</div>
            <div class="col-6"><b>Agent 覆盖：</b>{{ detailTarget.agent_override_count }}</div>
            <div class="col-6"><b>调用次数：</b>{{ detailTarget.invoke_count }}</div>
            <div class="col-6"><b>成功 / 失败：</b>{{ detailTarget.success_count }} / {{ detailTarget.failure_count }}</div>
          </div>
          <q-expansion-item dense-toggle default-open label="参数 Schema">
            <pre class="tool-schema">{{ prettyJSON(detailTarget.parameters_schema_json, "暂无参数 Schema") }}</pre>
          </q-expansion-item>
          <q-expansion-item dense-toggle label="返回 Schema">
            <pre class="tool-schema">{{ prettyJSON(detailTarget.result_schema_json, "暂无返回 Schema") }}</pre>
          </q-expansion-item>
          <q-expansion-item dense-toggle label="配置 JSON">
            <pre class="tool-schema">{{ prettyJSON(detailTarget.config_json, "暂无配置") }}</pre>
          </q-expansion-item>
          <q-expansion-item dense-toggle label="默认配置">
            <pre class="tool-schema">{{ prettyJSON(detailTarget.default_config_json, "暂无默认配置") }}</pre>
          </q-expansion-item>
          <q-expansion-item dense-toggle label="元数据">
            <pre class="tool-schema">{{ prettyJSON(detailTarget.metadata_json, "暂无元数据") }}</pre>
          </q-expansion-item>
        </q-card-section>
      </q-card>
    </q-dialog>

    <q-dialog v-model="riskConfirmOpen" persistent>
      <q-card class="tool-risk-card">
        <q-card-section class="row items-start q-gutter-md">
          <q-avatar color="negative" text-color="white" icon="warning" />
          <div class="col">
            <div class="text-h6">确认启用严重风险 Tool</div>
            <div class="text-body2 text-grey-7 q-mt-xs">
              {{ pendingToggleTool?.display_name }}（{{ pendingToggleTool?.key }}）风险级别为
              {{ riskLabel(pendingToggleTool?.risk_level || "") }}。
            </div>
          </div>
        </q-card-section>
        <q-card-section class="q-pt-none">
          <q-banner rounded class="bg-red-1 text-red-10">
            启用后，Agent 可能在被授权时调用该能力。严重风险工具通常涉及本地命令、文件写入、外部发送或高影响操作；本次只是开启全局可用状态，真正执行时仍应保留工具策略和确认保护。
          </q-banner>
          <div class="text-body2 q-mt-md">
            请确认你理解该 Tool 的能力边界，并已在 Agent 绑定和运行环境中做好限制。
          </div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="取消" :disable="riskConfirmLoading" v-close-popup />
          <q-btn color="negative" label="确认启用" :loading="riskConfirmLoading" @click="confirmRiskToggle" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useQuasar, type QTableColumn } from "quasar";
import SkillPagination from "../features/skills/components/SkillPagination.vue";
import { getTool, listTools, toggleToolEnabled } from "../features/tools/api";
import type { Tool, ToolSummary } from "../features/tools/types";

const $q = useQuasar();
const search = ref("");
const category = ref("");
const riskLevel = ref("");
const enabled = ref<boolean | null>(null);
const page = ref(1);
const pageSize = ref(20);
const rows = ref<Tool[]>([]);
const total = ref(0);
const summary = ref<ToolSummary>({ total_tools: 0, enabled_tools: 0, high_risk_enabled: 0, calls_24h: 0, failure_rate_24h: 0 });
const loading = ref(false);
const error = ref("");
const togglingId = ref("");
const detailOpen = ref(false);
const detailLoading = ref(false);
const detailTarget = ref<Tool | null>(null);
const riskConfirmOpen = ref(false);
const riskConfirmLoading = ref(false);
const pendingToggleTool = ref<Tool | null>(null);
const pendingToggleEnabled = ref(false);

const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
const summaryCards = computed(() => [
  { label: "总工具", value: summary.value.total_tools, hint: "已注册 Tool" },
  { label: "已启用", value: summary.value.enabled_tools, hint: "全局启用" },
  { label: "高风险启用", value: summary.value.high_risk_enabled, hint: "high / critical" },
  { label: "24h 失败率", value: `${Math.round(summary.value.failure_rate_24h * 100)}%`, hint: `${summary.value.calls_24h} 次调用` }
]);

const columns: QTableColumn<Tool>[] = [
  { name: "name", label: "名称", field: "display_name", align: "left" },
  { name: "description", label: "描述", field: "description", align: "left", style: "max-width: 300px; white-space: normal;" },
  { name: "category", label: "分类 / 来源", field: "category", align: "left" },
  { name: "risk", label: "风险", field: "risk_level", align: "left" },
  { name: "enabled", label: "启用", field: "enabled", align: "center" },
  { name: "stats", label: "调用统计", field: "invoke_count", align: "left" },
  { name: "latency", label: "平均耗时", field: "avg_duration_ms", align: "left" },
  { name: "last", label: "最近调用", field: "last_invoked_at", align: "left" },
  { name: "actions", label: "操作", field: "id", align: "right" }
];

const categoryOptions = ["filesystem", "runtime", "web", "memory", "skill", "media", "session", "messaging", "mcp", "system"].map((value) => ({ label: value, value }));
const riskOptions = ["low", "medium", "high", "critical"].map((value) => ({ label: riskLabel(value), value }));
const enabledOptions = [
  { label: "仅启用", value: true },
  { label: "仅停用", value: false }
];
const tablePagination = { rowsPerPage: 0 };

async function loadRows() {
  loading.value = true;
  error.value = "";
  try {
    const data = await listTools({
      search: search.value,
      category: category.value,
      risk_level: riskLevel.value,
      enabled: enabled.value,
      page: page.value,
      page_size: pageSize.value
    });
    rows.value = data.items;
    total.value = data.total;
    summary.value = data.summary;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载 Tools 失败";
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  search.value = "";
  category.value = "";
  riskLevel.value = "";
  enabled.value = null;
  page.value = 1;
  void loadRows();
}

function requestToggleEnabled(tool: Tool, next: boolean) {
  if (next && ["high", "critical"].includes(tool.risk_level)) {
    pendingToggleTool.value = tool;
    pendingToggleEnabled.value = next;
    riskConfirmOpen.value = true;
    return;
  }
  void performToggleEnabled(tool, next);
}

async function confirmRiskToggle() {
  if (!pendingToggleTool.value) return;
  riskConfirmLoading.value = true;
  try {
    await performToggleEnabled(pendingToggleTool.value, pendingToggleEnabled.value);
    riskConfirmOpen.value = false;
    pendingToggleTool.value = null;
  } finally {
    riskConfirmLoading.value = false;
  }
}

async function performToggleEnabled(tool: Tool, next: boolean) {
  togglingId.value = tool.id;
  try {
    const updated = await toggleToolEnabled(tool.id, next);
    rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
    $q.notify({ type: "positive", message: next ? "Tool 已启用" : "Tool 已停用" });
    await loadRows();
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "更新 Tool 失败" });
  } finally {
    togglingId.value = "";
  }
}

async function openDetail(tool: Tool) {
  detailTarget.value = tool;
  detailOpen.value = true;
  detailLoading.value = true;
  try {
    detailTarget.value = await getTool(tool.id || tool.key);
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "加载 Tool 详情失败" });
  } finally {
    detailLoading.value = false;
  }
}

function riskLabel(value: string) {
  return ({ low: "低", medium: "中", high: "高", critical: "严重" } as Record<string, string>)[value] ?? value;
}

function riskColor(value: string) {
  return ({ low: "positive", medium: "warning", high: "orange", critical: "negative" } as Record<string, string>)[value] ?? "grey";
}

function statusLabel(value?: string) {
  if (!value) return "未调用";
  return ({ success: "成功", error: "错误", blocked: "阻断", cancelled: "取消" } as Record<string, string>)[value] ?? value;
}

function formatDate(value?: string) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}

function prettyJSON(value: string, emptyLabel = "{}") {
  try {
    const parsed = JSON.parse(value || "{}");
    if (parsed && typeof parsed === "object" && Object.keys(parsed).length === 0) {
      return emptyLabel;
    }
    return JSON.stringify(parsed, null, 2);
  } catch {
    return value || emptyLabel;
  }
}

watch([search, category, riskLevel, enabled], () => {
  page.value = 1;
  void loadRows();
});
watch([page, pageSize], () => {
  void loadRows();
});

onMounted(loadRows);
</script>

<style scoped lang="sass">
.tools-page
  padding: 24px

.tools-hero
  display: flex
  justify-content: space-between
  gap: 16px
  align-items: flex-start
  margin-bottom: 18px

.tools-kicker
  color: var(--q-primary)
  font-size: 12px
  font-weight: 700
  letter-spacing: .12em
  text-transform: uppercase

.tools-title
  margin: 4px 0
  font-size: 34px
  line-height: 1.15

.tools-subtitle
  margin: 0
  color: var(--q-grey-7)

.tools-summary-card,
.tools-filter-card,
.tools-table
  border-radius: 22px
  overflow: hidden

.tool-detail-card
  width: min(760px, 92vw)
  border-radius: 22px

.tool-risk-card
  width: min(560px, 92vw)
  border-radius: 22px

.tool-schema
  max-height: 280px
  overflow: auto
  padding: 12px
  border-radius: 12px
  background: #0f172a
  color: #e2e8f0
  font-size: 12px
  line-height: 1.55
  white-space: pre-wrap

@media (max-width: 720px)
  .tools-hero
    flex-direction: column
    align-items: stretch
</style>
