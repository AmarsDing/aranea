<template>
  <q-page :class="['agents-page', { 'is-dark': isDark }]">
    <section class="agents-hero">
      <div>
        <div class="agents-kicker">Agent workspace</div>
        <h1 class="agents-title">Agent</h1>
        <p class="agents-subtitle">管理您的 AI Agent，按模型、业务分类、进化状态快速筛选与维护。</p>
      </div>
      <div class="agents-hero__actions">
        <q-btn outline rounded color="primary" icon="sync_alt" label="Agent 迁移" class="agent-action-btn" @click="migrationOpen = true" />
        <q-btn outline rounded color="primary" icon="account_tree" label="管理分类" class="agent-action-btn" to="/settings/agent-categories" />
        <q-btn color="primary" unelevated rounded icon="add" label="创建 Agent" class="agent-primary-btn" @click="openCreate" />
      </div>
    </section>

    <q-card flat bordered class="agents-filter-card">
      <q-card-section class="agents-filter-card__body row q-col-gutter-sm items-center">
        <div class="col-12 col-md-4">
          <q-input v-model="keyword" dense outlined clearable debounce="350" placeholder="搜索 Agent..." class="agent-control">
            <template #prepend><q-icon name="search" /></template>
          </q-input>
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-select v-model="selectedStatus" dense outlined clearable emit-value map-options label="All Types" :options="statusOptions" class="agent-control" />
        </div>
        <div class="col-12 col-sm-6 col-md-3">
          <q-select
            v-model="selectedCategory"
            dense
            outlined
            clearable
            emit-value
            map-options
            use-input
            label="业务分类"
            :options="categoryPositionOptions"
            class="agent-control"
          />
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-select v-model="selectedProvider" dense outlined clearable emit-value map-options label="Provider" :options="providerOptions" class="agent-control" />
        </div>
        <div class="col-12 col-sm-6 col-md-1 row justify-end">
          <q-btn-toggle
            v-model="viewMode"
            dense
            rounded
            unelevated
            class="view-toggle"
            toggle-color="primary"
            :options="[
              { value: 'grid', slot: 'grid' },
              { value: 'list', slot: 'list' }
            ]"
          >
            <template #grid><q-icon name="grid_view" /></template>
            <template #list><q-icon name="view_list" /></template>
          </q-btn-toggle>
        </div>
      </q-card-section>
    </q-card>

    <div v-if="loading" class="row q-col-gutter-md q-mt-md">
      <div v-for="i in rowsPerPage" :key="i" class="col-12 col-sm-6 col-lg-4">
        <q-card flat bordered class="agent-card">
          <q-card-section>
            <q-skeleton type="QAvatar" size="52px" />
            <q-skeleton class="q-mt-md" type="text" />
            <q-skeleton type="text" width="70%" />
            <q-skeleton class="q-mt-md" height="72px" />
          </q-card-section>
        </q-card>
      </div>
    </div>

    <q-card v-else-if="agents.length === 0" flat bordered class="empty-agent-card q-mt-md">
      <q-card-section class="empty-agent-state column items-center text-center">
        <div class="empty-agent-visual">
          <q-avatar size="72px" color="primary" text-color="white" icon="smart_toy" />
        </div>
        <div class="text-h6 q-mt-md">{{ keyword ? "未找到匹配的 Agent" : "暂无 Agent" }}</div>
        <div class="text-body2 text-grey-7 q-mt-sm">调整筛选条件，或创建一个新的 Agent 开始配置。</div>
        <q-btn class="q-mt-md" color="primary" rounded unelevated icon="add" label="创建 Agent" @click="openCreate" />
      </q-card-section>
    </q-card>

    <section v-else class="q-mt-md">
      <div v-if="viewMode === 'grid'" class="row q-col-gutter-md">
        <div v-for="agent in agents" :key="agent.id" class="col-12 col-sm-6 col-lg-4">
          <agent-card
            :agent="agent"
            :favorite="isFavorite(agent.id)"
            :category-label="categoryLabel(agent.category_position_id)"
            :context-label="formatContext(agent.context_window)"
            :evolving="selfEvolveEnabled(agent)"
            @toggle-favorite="toggleFavorite"
            @copy-key="copyKey"
            @delete="confirmDelete"
          />
        </div>
      </div>

      <q-table
        v-else
        class="agents-table"
        flat
        bordered
        :rows="agents"
        :columns="tableColumns"
        row-key="id"
        hide-pagination
      >
        <template #body-cell-name="props">
          <q-td :props="props">
            <div class="row items-center no-wrap q-gutter-sm">
              <q-btn flat dense round size="sm" :color="isFavorite(props.row.id) ? 'amber-8' : 'grey-5'" :icon="isFavorite(props.row.id) ? 'star' : 'star_border'" @click="toggleFavorite(props.row.id)" />
              <q-avatar rounded color="primary" text-color="white" size="36px" :icon="avatarIcon(props.row.icon)">
                <img v-if="avatarSrc(props.row.icon)" :src="avatarSrc(props.row.icon)" :alt="props.row.display_name" />
              </q-avatar>
              <div>
                <div class="text-weight-medium">{{ props.row.display_name }}</div>
                <button class="agent-handle" @click="copyKey(props.row.agent_key)">{{ props.row.agent_key }}</button>
              </div>
            </div>
          </q-td>
        </template>
        <template #body-cell-status="props">
          <q-td :props="props">
            <q-badge rounded :color="props.row.status === 'active' ? 'positive' : 'grey'">{{ props.row.status }}</q-badge>
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props" class="q-gutter-xs">
            <q-btn flat dense rounded color="primary" label="设置" :to="`/agents/${props.row.id}/settings`" />
            <q-btn flat dense round color="negative" icon="delete" @click="confirmDelete(props.row)" />
          </q-td>
        </template>
      </q-table>
    </section>

    <footer class="agents-pagination q-mt-md">
      <div class="text-caption text-grey-7">{{ total }} 条</div>
      <div class="row items-center q-gutter-sm">
        <q-select v-model="rowsPerPage" dense outlined emit-value map-options label="行" :options="[10, 20, 50].map((v) => ({ label: String(v), value: v }))" class="rows-select" />
        <span class="text-caption">第 {{ page }} / {{ pageMax }} 页</span>
        <q-btn round dense flat icon="chevron_left" :disable="page <= 1" @click="page--" />
        <q-btn round dense flat icon="chevron_right" :disable="page >= pageMax" @click="page++" />
      </div>
    </footer>

    <agent-create-dialog
      v-model="createOpen"
      v-model:self-evolve="selfEvolve"
      v-model:category-industry="categoryIndustry"
      v-model:category-department="categoryDepartment"
      :form="form"
      :industry-options="industryOptions"
      :department-options="departmentOptions"
      :position-options="positionOptions"
      :provider-options="providerOptions"
      :model-options="modelOptions"
      :selected-template-key="selectedTemplateKey"
      :agent-key-error="agentKeyError"
      :can-create="canCreate"
      :creating="creating"
      :checking-model="checkingModel"
      @apply-template="applyTemplate"
      @check-model="checkModel"
      @create="onCreate"
    />

    <q-dialog v-model="deleteOpen">
      <q-card style="width: 420px; max-width: 92vw">
        <q-card-section>
          <div class="text-h6">删除 Agent</div>
          <div class="text-body2 text-grey-7 q-mt-sm">确认删除「{{ deleteTarget?.display_name }}」？此操作会软删除，列表中不再显示。</div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat rounded label="取消" v-close-popup />
          <q-btn color="negative" rounded unelevated label="删除" @click="deleteAgentTarget" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="migrationOpen">
      <q-card style="width: 520px; max-width: 92vw">
        <q-card-section>
          <div class="text-h6">Agent 迁移</div>
          <div class="text-body2 text-grey-7 q-mt-sm">导入、导出、批量映射与冲突处理将在单独流程中实现；当前先保留入口。</div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn color="primary" flat rounded label="知道了" v-close-popup />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { copyToClipboard, useQuasar, type QTableColumn } from "quasar";
import { deleteAgent, listAgentsPaged, updateAgent, type Agent } from "../features/agents/api";
import AgentCard from "../components/agents/AgentCard.vue";
import AgentCreateDialog from "../components/agents/AgentCreateDialog.vue";
import {
  descriptionTemplates,
  avatarThumbnailUrl,
  flattenCategoryPositions,
  formatContext,
  isAvatarAssetRef,
  selfEvolveEnabled,
  statusOptions
} from "../components/agents/agentUi";
import {
  listPlatformResources,
  listPlatformResourceTree,
  validateModel,
  type PlatformResource,
  type PlatformResourceTreeNode
} from "../features/platform/api";
import { useAppStore } from "../stores/app";

type ViewMode = "grid" | "list";

const LS_VIEW = "agents.viewMode";

const $q = useQuasar();
const store = useAppStore();
const isDark = computed(() => $q.dark.isActive);
const agents = ref<Agent[]>([]);
const keyword = ref("");
const selectedStatus = ref<string | null>(null);
const selectedProvider = ref<string | null>(null);
const selectedCategory = ref<string | null>(null);
const page = ref(1);
const rowsPerPage = ref(20);
const total = ref(0);
const loading = ref(false);
const createOpen = ref(false);
const migrationOpen = ref(false);
const deleteOpen = ref(false);
const deleteTarget = ref<Agent | null>(null);
const creating = ref(false);
const checkingModel = ref(false);
const modelCheckPassed = ref(false);
const selfEvolve = ref(true);
const viewMode = ref<ViewMode>((localStorage.getItem(LS_VIEW) as ViewMode) || "grid");
const categoryTree = ref<PlatformResourceTreeNode[]>([]);
const providerModels = ref<PlatformResource[]>([]);
const avatars = ref<PlatformResource[]>([]);
const categoryIndustry = ref<string | null>(null);
const categoryDepartment = ref<string | null>(null);

const form = reactive({
  agent_key: "",
  display_name: "",
  provider: "openrouter",
  model: "gpt-4.1-mini",
  icon: "smart_toy",
  agent_description: "",
  category_position_id: ""
});

const tableColumns: QTableColumn<Agent>[] = [
  { name: "name", label: "名称", align: "left", field: "display_name" },
  { name: "status", label: "状态", align: "left", field: "status" },
  { name: "model", label: "模型", align: "left", field: (row) => `${row.provider} / ${row.model}` },
  { name: "category", label: "业务分类", align: "left", field: (row) => categoryLabel(row.category_position_id) },
  { name: "ctx", label: "上下文", align: "left", field: (row) => formatContext(row.context_window) },
  { name: "actions", label: "操作", align: "right", field: "id" }
];

const providerOptions = computed(() =>
  Array.from(new Set(providerModels.value.map((row) => row.provider).filter(Boolean))).map((provider) => ({
    label: provider,
    value: provider
  }))
);
const modelOptions = computed(() =>
  providerModels.value
    .filter((row) => row.provider === form.provider)
    .map((row) => ({ label: row.name, value: row.model }))
);
const industryNodes = computed(() => categoryTree.value.filter((row) => row.level === "industry" && row.enabled));
const industryOptions = computed(() => industryNodes.value.map((row) => ({ label: row.name, value: row.id })));
const selectedIndustryNode = computed(() => industryNodes.value.find((row) => row.id === categoryIndustry.value));
const selectedDepartmentNode = computed(() =>
  selectedIndustryNode.value?.children?.find((row) => row.id === categoryDepartment.value && row.level === "department" && row.enabled)
);
const departmentOptions = computed(() =>
  (selectedIndustryNode.value?.children ?? [])
    .filter((row) => row.level === "department" && row.enabled)
    .map((row) => ({ label: row.name, value: row.id }))
);
const positionOptions = computed(() =>
  (selectedDepartmentNode.value?.children ?? [])
    .filter((row) => row.level === "position" && row.enabled)
    .map((row) => ({ label: row.name, value: row.id }))
);
const categoryPositionOptions = computed(() => flattenCategoryPositions(industryNodes.value));
const pageMax = computed(() => Math.max(1, Math.ceil(total.value / rowsPerPage.value)));
const agentKeyError = computed(() => {
  if (!form.agent_key) return "";
  return /^[a-z0-9]+(-[a-z0-9]+)*$/.test(form.agent_key) ? "" : "仅支持小写字母、数字、连字符";
});
const canCreate = computed(
  () => Boolean(form.display_name && form.agent_key && !agentKeyError.value && form.provider && form.model && modelCheckPassed.value)
);

watch(viewMode, (value) => localStorage.setItem(LS_VIEW, value));
watch(rowsPerPage, () => {
  page.value = 1;
  void loadAgentsPage();
});
watch(page, () => void loadAgentsPage());
watch([keyword, selectedStatus, selectedProvider, selectedCategory], () => {
  page.value = 1;
  void loadAgentsPage();
});
watch(
  () => form.provider,
  () => {
    form.model = modelOptions.value[0]?.value ?? "";
    modelCheckPassed.value = false;
  }
);
watch(
  () => form.model,
  () => {
    modelCheckPassed.value = false;
  }
);
watch(categoryIndustry, () => {
  categoryDepartment.value = null;
  form.category_position_id = "";
});
watch(categoryDepartment, () => {
  form.category_position_id = "";
});

onMounted(async () => {
  await Promise.all([loadAgentsPage(), loadDependencies()]);
});

async function loadAgentsPage() {
  loading.value = true;
  try {
    const result = await listAgentsPaged({
      keyword: keyword.value || undefined,
      status: selectedStatus.value || undefined,
      provider: selectedProvider.value || undefined,
      category_id: selectedCategory.value || undefined,
      limit: rowsPerPage.value,
      offset: (page.value - 1) * rowsPerPage.value
    });
    agents.value = result.items;
    total.value = result.total;
  } catch (error) {
    $q.notify({ type: "negative", message: error instanceof Error ? error.message : "Agent 列表加载失败" });
  } finally {
    loading.value = false;
  }
}

async function loadDependencies() {
  const [treeRows, providerRows, avatarRows] = await Promise.all([
    listPlatformResourceTree("agent-categories"),
    listPlatformResources("llm-provider-models"),
    listPlatformResources("avatar-assets")
  ]);
  categoryTree.value = treeRows;
  providerModels.value = providerRows;
  avatars.value = avatarRows;
  if (!form.icon || form.icon === "smart_toy") {
    form.icon = avatarRows[0]?.id ?? "smart_toy";
  }
}

async function openCreate() {
  modelCheckPassed.value = false;
  await loadDependencies();
  createOpen.value = true;
}

async function checkModel() {
  checkingModel.value = true;
  try {
    const result = await validateModel(form.provider, form.model);
    modelCheckPassed.value = result.ok;
    $q.notify({ type: result.ok ? "positive" : "negative", message: result.message });
  } finally {
    checkingModel.value = false;
  }
}

async function onCreate() {
  if (!canCreate.value) return;
  creating.value = true;
  try {
    await store.addAgent({
      ...form,
      config_json: JSON.stringify({
        self_evolve: selfEvolve.value,
        description_template_key: selectedTemplateKey.value
      })
    });
    keyword.value = "";
    selectedStatus.value = null;
    selectedProvider.value = null;
    selectedCategory.value = null;
    page.value = 1;
    await loadAgentsPage();
    resetForm();
    createOpen.value = false;
    $q.notify({ type: "positive", message: "创建成功" });
  } finally {
    creating.value = false;
  }
}

const selectedTemplateKey = ref("");

function applyTemplate(template: (typeof descriptionTemplates)[number]) {
  selectedTemplateKey.value = template.key;
  if (form.agent_description.trim()) {
    form.agent_description = `${form.agent_description}\n\n${template.text}`;
  } else {
    form.agent_description = template.text;
  }
}

function resetForm() {
  Object.assign(form, {
    agent_key: "",
    display_name: "",
    provider: "openrouter",
    model: "gpt-4.1-mini",
    icon: avatars.value[0]?.id ?? "smart_toy",
    agent_description: "",
    category_position_id: ""
  });
  categoryIndustry.value = null;
  categoryDepartment.value = null;
  selectedTemplateKey.value = "";
  modelCheckPassed.value = false;
  selfEvolve.value = true;
}

function confirmDelete(agent: Agent) {
  deleteTarget.value = agent;
  deleteOpen.value = true;
}

async function deleteAgentTarget() {
  if (!deleteTarget.value) return;
  await deleteAgent(deleteTarget.value.id);
  deleteOpen.value = false;
  deleteTarget.value = null;
  await loadAgentsPage();
  $q.notify({ type: "positive", message: "已删除" });
}

function isFavorite(id: string) {
  return agents.value.find((agent) => agent.id === id)?.is_favorite ?? false;
}

async function toggleFavorite(id: string) {
  const agent = agents.value.find((item) => item.id === id);
  if (!agent) return;
  const previous = agent.is_favorite;
  agent.is_favorite = !previous;
  try {
    const updated = await updateAgent(id, { ...agent, is_favorite: agent.is_favorite });
    agents.value = agents.value.map((item) => (item.id === id ? updated : item));
    store.upsertAgent(updated);
  } catch (error) {
    agent.is_favorite = previous;
    $q.notify({ type: "negative", message: error instanceof Error ? error.message : "收藏保存失败" });
  }
}

function categoryLabel(id: string) {
  if (!id) return "未分类";
  return categoryPositionOptions.value.find((item) => item.value === id)?.label ?? "未分类";
}

function avatarSrc(value: string) {
  return isAvatarAssetRef(value) ? avatarThumbnailUrl(value) : "";
}

function avatarIcon(value: string) {
  return avatarSrc(value) ? undefined : value || "smart_toy";
}

async function copyKey(value: string) {
  await copyToClipboard(value);
  $q.notify({ type: "positive", message: "Agent 标识已复制" });
}
</script>

<style scoped>
.agents-page {
  min-height: 100%;
  padding: 28px;
  background:
    radial-gradient(circle at 82% 0%, rgba(25, 118, 210, 0.12), transparent 28%),
    radial-gradient(circle at 8% 14%, rgba(255, 152, 0, 0.1), transparent 24%),
    linear-gradient(180deg, #fbfcff 0%, #f7f9fc 46%, #ffffff 100%);
}

.agents-hero,
.agents-pagination {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.agents-hero {
  padding: 4px 2px 0;
}

.agents-kicker {
  display: inline-flex;
  align-items: center;
  height: 28px;
  padding: 0 12px;
  border: 1px solid rgba(25, 118, 210, 0.14);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.78);
  color: #155ebc;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  box-shadow: 0 8px 24px rgba(16, 24, 40, 0.04);
}

.agents-hero__actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.agents-title {
  margin: 12px 0 0;
  font-size: clamp(36px, 5vw, 56px);
  line-height: 1;
  font-weight: 800;
  letter-spacing: -0.055em;
  color: #101828;
}

.agents-subtitle {
  margin: 10px 0 0;
  max-width: 620px;
  color: #5f6b7a;
  font-size: 15px;
  line-height: 1.65;
}

.agents-filter-card {
  margin-top: 22px;
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.82);
  box-shadow: 0 18px 48px rgba(16, 24, 40, 0.06);
  backdrop-filter: blur(16px);
}

.agents-filter-card__body {
  padding: 14px 16px;
}

.agent-control :deep(.q-field__control) {
  border-radius: 16px;
  background: #ffffff;
  min-height: 44px;
}

.agent-control :deep(.q-field__control::before) {
  border-color: rgba(15, 23, 42, 0.12);
}

.agent-control :deep(.q-field__control::after) {
  border-width: 1px;
}

.view-toggle {
  padding: 3px;
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 999px;
  background: #f2f5f9;
}

.agent-handle {
  padding: 0;
  border: 0;
  background: transparent;
  color: #667085;
  cursor: pointer;
  font-size: 12px;
}

.empty-agent-card,
.agents-table {
  border-radius: 24px;
  border-color: rgba(15, 23, 42, 0.08);
  box-shadow: 0 18px 48px rgba(16, 24, 40, 0.05);
}

.empty-agent-card {
  overflow: hidden;
  background:
    radial-gradient(circle at center 26%, rgba(25, 118, 210, 0.08), transparent 22%),
    linear-gradient(180deg, #ffffff, #fbfcff);
}

.empty-agent-state {
  min-height: 230px;
  padding: 36px 24px;
}

.empty-agent-visual {
  display: grid;
  place-items: center;
  width: 108px;
  height: 108px;
  border: 1px solid rgba(25, 118, 210, 0.12);
  border-radius: 32px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.9), rgba(238, 246, 255, 0.9)),
    radial-gradient(circle at top, rgba(25, 118, 210, 0.16), transparent 55%);
  box-shadow: 0 18px 42px rgba(16, 24, 40, 0.08);
}

.agents-table :deep(th) {
  color: #475467;
  font-size: 12px;
  font-weight: 700;
  background: #f8fafc;
}

.agents-table :deep(td) {
  color: #344054;
}

.agents-pagination {
  padding: 12px 16px;
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.78);
}

.rows-select {
  width: 96px;
}

.agent-action-btn,
.agent-primary-btn {
  min-height: 42px;
  padding: 0 18px;
  font-weight: 700;
}

.agent-primary-btn {
  box-shadow: 0 12px 26px rgba(25, 118, 210, 0.22);
}

.min-width-0 {
  min-width: 0;
}

.agents-page.is-dark {
  background:
    radial-gradient(circle at 82% 0%, rgba(59, 130, 246, 0.16), transparent 30%),
    radial-gradient(circle at 8% 14%, rgba(245, 158, 11, 0.1), transparent 24%),
    linear-gradient(160deg, #0b1220 0%, #111827 48%, #0f172a 100%);
  color: #e5e7eb;
}

.agents-page.is-dark .agents-kicker {
  border-color: rgba(96, 165, 250, 0.22);
  background: rgba(15, 23, 42, 0.74);
  color: #93c5fd;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.28);
}

.agents-page.is-dark .agents-title {
  color: #f8fafc;
}

.agents-page.is-dark .agents-subtitle,
.agents-page.is-dark .agent-handle {
  color: #94a3b8;
}

.agents-page.is-dark .agents-filter-card,
.agents-page.is-dark .empty-agent-card,
.agents-page.is-dark .agents-table,
.agents-page.is-dark .agents-pagination {
  border-color: rgba(148, 163, 184, 0.16);
  background: rgba(17, 24, 39, 0.88);
  box-shadow: 0 14px 38px rgba(0, 0, 0, 0.32);
}

.agents-page.is-dark .agent-control :deep(.q-field__control),
.agents-page.is-dark .rows-select :deep(.q-field__control) {
  background: rgba(30, 41, 59, 0.76);
}

.agents-page.is-dark .agent-control :deep(.q-field__control::before),
.agents-page.is-dark .rows-select :deep(.q-field__control::before) {
  border-color: rgba(148, 163, 184, 0.18);
}

.agents-page.is-dark .view-toggle {
  border-color: rgba(148, 163, 184, 0.16);
  background: rgba(30, 41, 59, 0.72);
}

.agents-page.is-dark .empty-agent-card {
  background:
    radial-gradient(circle at center 26%, rgba(59, 130, 246, 0.12), transparent 22%),
    linear-gradient(180deg, rgba(17, 24, 39, 0.94), rgba(15, 23, 42, 0.92));
}

.agents-page.is-dark .empty-agent-visual {
  border-color: rgba(96, 165, 250, 0.2);
  background:
    linear-gradient(180deg, rgba(30, 41, 59, 0.86), rgba(15, 23, 42, 0.9)),
    radial-gradient(circle at top, rgba(59, 130, 246, 0.18), transparent 55%);
  box-shadow: 0 18px 42px rgba(0, 0, 0, 0.32);
}

.agents-page.is-dark .agents-table :deep(th) {
  background: rgba(15, 23, 42, 0.92);
  color: #cbd5e1;
}

.agents-page.is-dark .agents-table :deep(td) {
  color: #e2e8f0;
}

.agents-page.is-dark .agents-table :deep(tbody tr:hover) {
  background: rgba(51, 65, 85, 0.46);
}

@media (max-width: 599px) {
  .agents-page {
    padding: 18px;
  }

  .agents-hero__actions {
    width: 100%;
  }

  .agent-action-btn,
  .agent-primary-btn {
    flex: 1;
  }
}
</style>
