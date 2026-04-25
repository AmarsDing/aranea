<template>
  <q-page class="skills-page">
    <section class="skills-hero">
      <div>
        <div class="skills-kicker">Skill registry</div>
        <h1 class="skills-title">Skill 管理</h1>
        <p class="skills-subtitle">查看 Skill 使用频率、成功失败统计、最近调用 Agent，并维护启用状态。</p>
      </div>
    </section>

    <skill-upload-placeholder class="q-mb-md" @completed="loadRows" />

    <skill-filter-bar
      v-model:search="search"
      v-model:enabled="enabled"
      v-model:status="status"
      :loading="loading"
      class="q-mb-md"
      @reset="resetFilters"
      @refresh="loadRows"
    />

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <q-card v-if="!loading && rows.length === 0" flat bordered class="skills-empty-card">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="psychology" />
        <div class="text-h6 q-mt-md">{{ search ? "没有匹配的 Skill" : "暂无 Skill" }}</div>
        <div class="text-body2 text-grey-7 q-mt-sm">上传能力将在后续版本启用；当前可先查看已有 Skill 与运行统计。</div>
      </q-card-section>
    </q-card>

    <skill-table
      v-else
      :rows="rows"
      :loading="loading"
      :toggling-id="togglingId"
      @toggle-enabled="onToggleEnabled"
      @edit="openEditor"
      @delete="confirmDelete"
    />

    <footer class="skills-pagination q-mt-md">
      <div class="text-caption text-grey-7">{{ total }} 条 Skill</div>
      <div class="row items-center q-gutter-sm">
        <q-select v-model="pageSize" dense outlined emit-value map-options label="行" :options="pageSizeOptions" class="skills-page-size" />
        <span class="text-caption">第 {{ page }} / {{ pageMax }} 页</span>
        <q-btn round dense flat icon="chevron_left" :disable="page <= 1 || loading" @click="page--" />
        <q-btn round dense flat icon="chevron_right" :disable="page >= pageMax || loading" @click="page++" />
      </div>
    </footer>

    <q-dialog v-model="deleteOpen">
      <q-card style="width: 420px; max-width: 92vw">
        <q-card-section>
          <div class="text-h6">删除 Skill</div>
          <div class="text-body2 text-grey-7 q-mt-sm">确认删除「{{ deleteTarget?.name }}」？此操作会软删除，列表中不再显示。</div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat rounded label="取消" v-close-popup />
          <q-btn color="negative" rounded unelevated label="删除" :loading="deleting" @click="deleteTargetSkill" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="editorOpen" maximized>
      <q-card class="skill-editor-card">
        <q-card-section class="row items-center justify-between q-pb-sm">
          <div>
            <div class="text-h6">编辑 Skill 文件</div>
            <div class="text-caption text-grey-7">{{ editorTarget?.name }}</div>
          </div>
          <q-btn flat round dense icon="close" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section class="skill-editor-body">
          <q-card flat bordered class="skill-editor-files">
            <q-list separator>
              <q-item v-if="filesLoading">
                <q-item-section>正在加载文件...</q-item-section>
              </q-item>
              <q-item v-for="file in editorFiles" :key="file.path" clickable :active="file.path === selectedFile?.path" @click="selectFile(file.path)">
                <q-item-section avatar>
                  <q-icon :name="fileIcon(file.language)" />
                </q-item-section>
                <q-item-section>
                  <q-item-label>{{ file.name }}</q-item-label>
                  <q-item-label caption>{{ file.path }}</q-item-label>
                </q-item-section>
              </q-item>
              <q-item v-if="!filesLoading && editorFiles.length === 0">
                <q-item-section>该 Skill 暂无可编辑文件</q-item-section>
              </q-item>
            </q-list>
          </q-card>
          <q-card flat bordered class="skill-editor-pane">
            <q-card-section class="row items-center justify-between">
              <div>
                <div class="text-subtitle1">{{ selectedFile?.path || "请选择文件" }}</div>
                <div class="text-caption text-grey-7">{{ selectedFile?.language || "text" }}</div>
              </div>
              <q-btn color="primary" rounded unelevated icon="save" label="保存" :disable="!selectedFile" :loading="savingFile" @click="saveFile" />
            </q-card-section>
            <q-separator />
            <q-card-section class="q-pa-none">
              <q-input
                v-model="editorContent"
                type="textarea"
                borderless
                autogrow
                class="skill-editor-textarea"
                :disable="!selectedFile || readingFile"
                :placeholder="readingFile ? '正在读取文件...' : '选择左侧文件后编辑内容'"
              />
            </q-card-section>
          </q-card>
        </q-card-section>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useQuasar } from "quasar";
import SkillFilterBar from "../features/skills/components/SkillFilterBar.vue";
import SkillTable from "../features/skills/components/SkillTable.vue";
import SkillUploadPlaceholder from "../features/skills/components/SkillUploadPlaceholder.vue";
import { deleteSkill, listSkillFiles, listSkills, readSkillFile, toggleSkillEnabled, updateSkillFile } from "../features/skills/api";
import type { Skill, SkillFile } from "../features/skills/types";

const $q = useQuasar();
const search = ref("");
const enabled = ref<boolean | null>(null);
const status = ref("");
const page = ref(1);
const pageSize = ref(20);
const rows = ref<Skill[]>([]);
const total = ref(0);
const loading = ref(false);
const error = ref("");
const togglingId = ref("");
const deleteOpen = ref(false);
const deleteTarget = ref<Skill | null>(null);
const deleting = ref(false);
const editorOpen = ref(false);
const editorTarget = ref<Skill | null>(null);
const editorFiles = ref<SkillFile[]>([]);
const selectedFile = ref<SkillFile | null>(null);
const editorContent = ref("");
const filesLoading = ref(false);
const readingFile = ref(false);
const savingFile = ref(false);

const pageSizeOptions = [10, 20, 50].map((value) => ({ label: String(value), value }));
const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

async function loadRows() {
  loading.value = true;
  error.value = "";
  try {
    const data = await listSkills({
      search: search.value,
      enabled: enabled.value,
      status: status.value,
      page: page.value,
      page_size: pageSize.value
    });
    rows.value = data.items;
    total.value = data.total;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载 Skill 失败";
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  search.value = "";
  enabled.value = null;
  status.value = "";
  page.value = 1;
  void loadRows();
}

async function onToggleEnabled(skill: Skill, next: boolean) {
  togglingId.value = skill.id;
  try {
    const updated = await toggleSkillEnabled(skill.id, next);
    rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
    $q.notify({ type: "positive", message: next ? "Skill 已启用" : "Skill 已停用" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "更新启用状态失败" });
  } finally {
    togglingId.value = "";
  }
}

async function openEditor(skill: Skill) {
  editorTarget.value = skill;
  editorOpen.value = true;
  editorFiles.value = [];
  selectedFile.value = null;
  editorContent.value = "";
  filesLoading.value = true;
  try {
    editorFiles.value = await listSkillFiles(skill.id);
    const preferred = editorFiles.value.find((file) => file.path.toLowerCase() === "skill.md") ?? editorFiles.value[0];
    if (preferred) {
      await selectFile(preferred.path);
    }
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "加载 Skill 文件失败" });
  } finally {
    filesLoading.value = false;
  }
}

async function selectFile(path: string) {
  if (!editorTarget.value) return;
  const file = editorFiles.value.find((item) => item.path === path);
  if (!file) return;
  selectedFile.value = file;
  readingFile.value = true;
  try {
    const data = await readSkillFile(editorTarget.value.id, path);
    editorContent.value = data.content;
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "读取文件失败" });
  } finally {
    readingFile.value = false;
  }
}

async function saveFile() {
  if (!editorTarget.value || !selectedFile.value) return;
  savingFile.value = true;
  try {
    const data = await updateSkillFile(editorTarget.value.id, selectedFile.value.path, editorContent.value);
    editorContent.value = data.content;
    $q.notify({ type: "positive", message: "文件已保存" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存文件失败" });
  } finally {
    savingFile.value = false;
  }
}

function fileIcon(language: string) {
  return language === "markdown" ? "description" : language === "python" ? "data_object" : language === "javascript" || language === "typescript" ? "code" : "insert_drive_file";
}

function confirmDelete(skill: Skill) {
  deleteTarget.value = skill;
  deleteOpen.value = true;
}

async function deleteTargetSkill() {
  if (!deleteTarget.value) return;
  deleting.value = true;
  try {
    await deleteSkill(deleteTarget.value.id);
    deleteOpen.value = false;
    $q.notify({ type: "positive", message: "Skill 已删除" });
    await loadRows();
    if (rows.value.length === 0 && page.value > 1) {
      page.value -= 1;
      await loadRows();
    }
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "删除失败" });
  } finally {
    deleting.value = false;
  }
}

watch([search, enabled, status], () => {
  page.value = 1;
  void loadRows();
});
watch([page, pageSize], () => {
  void loadRows();
});

onMounted(loadRows);
</script>

<style scoped lang="sass">
.skills-page
  padding: 24px

.skills-hero
  display: flex
  justify-content: space-between
  gap: 16px
  align-items: flex-start
  margin-bottom: 18px

.skills-kicker
  color: var(--q-primary)
  font-size: 12px
  font-weight: 700
  letter-spacing: .12em
  text-transform: uppercase

.skills-title
  margin: 4px 0
  font-size: 34px
  line-height: 1.15

.skills-subtitle
  margin: 0
  color: var(--q-grey-7)

.skills-pagination
  display: flex
  align-items: center
  justify-content: space-between
  gap: 12px

.skills-empty-card
  border-radius: 22px

.skills-page-size
  min-width: 96px

.skill-editor-card
  min-height: 100vh

.skill-editor-body
  display: grid
  grid-template-columns: minmax(260px, 360px) 1fr
  gap: 16px
  height: calc(100vh - 82px)

.skill-editor-files,
.skill-editor-pane
  border-radius: 18px
  overflow: auto

.skill-editor-textarea
  min-height: calc(100vh - 190px)
  padding: 16px
  font-family: Consolas, 'Courier New', monospace
  font-size: 13px
  line-height: 1.6

@media (max-width: 720px)
  .skills-hero,
  .skills-pagination
    flex-direction: column
    align-items: stretch
  .skill-editor-body
    grid-template-columns: 1fr
    height: auto
</style>
