<template>
  <q-page class="skill-runs-page">
    <section class="skill-runs-hero">
      <div>
        <div class="skill-runs-kicker">Skill observability</div>
        <h1 class="skill-runs-title">Skill 运行记录</h1>
        <p class="skill-runs-subtitle">按 Skill、Agent、结果筛选调用明细，用于追踪使用频率和执行质量。</p>
      </div>
      <q-btn outline rounded color="primary" icon="arrow_back" label="返回 Skill 管理" to="/skills" />
    </section>

    <q-card flat bordered class="skill-runs-filter q-mb-md">
      <q-card-section class="row q-col-gutter-sm items-center">
        <div class="col-12 col-md-3">
          <q-input v-model="skillId" dense outlined clearable debounce="350" label="Skill ID" />
        </div>
        <div class="col-12 col-md-3">
          <q-input v-model="agentId" dense outlined clearable debounce="350" label="Agent ID" />
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-select v-model="status" dense outlined clearable emit-value map-options label="结果" :options="statusOptions" />
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-input v-model="from" dense outlined clearable label="开始时间 ISO" />
        </div>
        <div class="col-12 col-sm-6 col-md-2 row justify-end">
          <q-btn flat rounded icon="restart_alt" label="重置" @click="resetFilters" />
        </div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <skill-runs-table :rows="rows" :loading="loading" />

    <footer class="skill-runs-pagination q-mt-md">
      <div class="text-caption text-grey-7">{{ total }} 条运行记录</div>
      <div class="row items-center q-gutter-sm">
        <q-select v-model="pageSize" dense outlined emit-value map-options label="行" :options="pageSizeOptions" class="skill-runs-page-size" />
        <span class="text-caption">第 {{ page }} / {{ pageMax }} 页</span>
        <q-btn round dense flat icon="chevron_left" :disable="page <= 1 || loading" @click="page--" />
        <q-btn round dense flat icon="chevron_right" :disable="page >= pageMax || loading" @click="page++" />
      </div>
    </footer>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import SkillRunsTable from "../features/skills/components/SkillRunsTable.vue";
import { listSkillRuns } from "../features/skills/api";
import type { SkillInvocation } from "../features/skills/types";

const skillId = ref("");
const agentId = ref("");
const status = ref("");
const from = ref("");
const page = ref(1);
const pageSize = ref(20);
const rows = ref<SkillInvocation[]>([]);
const total = ref(0);
const loading = ref(false);
const error = ref("");

const statusOptions = [
  { label: "成功", value: "success" },
  { label: "失败", value: "failure" }
];
const pageSizeOptions = [10, 20, 50].map((value) => ({ label: String(value), value }));
const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

async function loadRows() {
  loading.value = true;
  error.value = "";
  try {
    const data = await listSkillRuns({
      skill_id: skillId.value,
      agent_id: agentId.value,
      status: status.value,
      from: from.value,
      page: page.value,
      page_size: pageSize.value
    });
    rows.value = data.items;
    total.value = data.total;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载运行记录失败";
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  skillId.value = "";
  agentId.value = "";
  status.value = "";
  from.value = "";
  page.value = 1;
  void loadRows();
}

watch([skillId, agentId, status, from], () => {
  page.value = 1;
  void loadRows();
});
watch([page, pageSize], () => {
  void loadRows();
});

onMounted(loadRows);
</script>

<style scoped lang="sass">
.skill-runs-page
  padding: 24px

.skill-runs-hero
  display: flex
  justify-content: space-between
  gap: 16px
  align-items: flex-start
  margin-bottom: 18px

.skill-runs-kicker
  color: var(--q-primary)
  font-size: 12px
  font-weight: 700
  letter-spacing: .12em
  text-transform: uppercase

.skill-runs-title
  margin: 4px 0
  font-size: 34px
  line-height: 1.15

.skill-runs-subtitle
  margin: 0
  color: var(--q-grey-7)

.skill-runs-filter
  border-radius: 22px

.skill-runs-pagination
  display: flex
  align-items: center
  justify-content: space-between
  gap: 12px

.skill-runs-page-size
  min-width: 96px

@media (max-width: 720px)
  .skill-runs-hero,
  .skill-runs-pagination
    flex-direction: column
    align-items: stretch
</style>
