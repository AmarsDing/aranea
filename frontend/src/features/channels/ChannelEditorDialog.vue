<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="channel-editor-card">
      <q-card-section class="row items-start justify-between q-gutter-md">
        <div>
          <div class="text-h6">{{ row ? "编辑 Channel" : "新增 Channel" }}</div>
          <div class="text-caption text-grey-7">配置非敏感参数，密钥字段留空表示不修改。</div>
        </div>
        <q-btn flat dense round icon="close" aria-label="关闭" @click="$emit('update:modelValue', false)" />
      </q-card-section>
      <q-separator />

      <q-card-section class="q-gutter-md">
        <div v-if="!row" class="q-gutter-sm">
          <div class="section-label">选择平台</div>
          <ChannelCatalogPicker v-model="selectedType" :catalog="catalog" />
        </div>

        <div class="row q-col-gutter-md">
          <q-input v-model="form.name" class="col-12 col-md-6" dense outlined label="名称 *" />
          <q-input v-model="form.key" class="col-12 col-md-6" dense outlined label="Key *" hint="同平台多实例可用 telegram_support 这类命名" />
          <q-input v-model="form.description" class="col-12" dense outlined autogrow type="textarea" label="描述" />
          <q-select
            v-model="receiveMode"
            class="col-12 col-md-4"
            dense
            outlined
            emit-value
            map-options
            label="接入方式"
            :options="receiveModeOptions"
          />
          <q-input v-model="webhookPath" class="col-12 col-md-8" dense outlined label="Webhook Path" :disable="receiveMode !== 'webhook'" />
          <q-input v-model="defaultAgentId" class="col-12 col-md-6" dense outlined label="默认 Agent" placeholder="main" />
          <q-input v-model="externalId" class="col-12 col-md-6" dense outlined label="外部 ID" />
          <q-input v-model="iconUrl" class="col-12 col-md-6" dense outlined label="自定义图标 URL" />
          <q-toggle v-model="form.enabled" class="col-12 col-md-6" color="primary" label="启用 Channel" />
        </div>

        <q-expansion-item default-open icon="key" label="凭据">
          <div class="row q-col-gutter-md q-pt-sm">
            <q-input
              v-for="key in credentialKeys"
              :key="key"
              v-model="credentialDraft[key]"
              class="col-12 col-md-6"
              dense
              outlined
              :type="showSecrets ? 'text' : 'password'"
              :label="credentialLabel(key)"
              :hint="credentialHint(key)"
            />
            <q-toggle v-model="showSecrets" class="col-12" color="primary" label="显示本次输入的密钥" />
          </div>
        </q-expansion-item>

        <q-expansion-item icon="data_object" label="高级 JSON">
          <div class="row q-col-gutter-md q-pt-sm">
            <q-input v-model="configExtraText" class="col-12" dense outlined autogrow type="textarea" label="config_json.config 额外字段" :error="Boolean(configError)" :error-message="configError" />
            <q-input v-model="metadataExtraText" class="col-12" dense outlined autogrow type="textarea" label="metadata_json 额外字段" :error="Boolean(metadataError)" :error-message="metadataError" />
          </div>
        </q-expansion-item>
      </q-card-section>

      <q-separator />
      <q-card-actions align="right">
        <q-btn flat rounded label="取消" @click="$emit('update:modelValue', false)" />
        <q-btn color="primary" rounded unelevated label="保存" :loading="saving" :disable="!canSave" @click="save" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { useQuasar } from "quasar";
import { createChannel, updateChannel } from "./api";
import ChannelCatalogPicker from "./ChannelCatalogPicker.vue";
import type { ChannelCatalogItem, ChannelConfig, ChannelCredential, ChannelCredentialInput, ChannelMetadata, ChannelRow } from "./types";

const props = defineProps<{
  modelValue: boolean;
  catalog: ChannelCatalogItem[];
  row: ChannelRow | null;
  credentials: ChannelCredential[];
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  saved: [row: ChannelRow];
}>();

const $q = useQuasar();
const saving = ref(false);
const selectedType = ref("");
const receiveMode = ref("webhook");
const webhookPath = ref("");
const defaultAgentId = ref("main");
const externalId = ref("");
const iconUrl = ref("");
const showSecrets = ref(false);
const configExtraText = ref("{}");
const metadataExtraText = ref("{}");
const credentialDraft = reactive<Record<string, string>>({});
const form = reactive({
  key: "",
  name: "",
  description: "",
  enabled: true
});

const selectedCatalog = computed(() => props.catalog.find((item) => item.type === selectedType.value) ?? null);
const receiveModeOptions = computed(() => (selectedCatalog.value?.receive_modes.length ? selectedCatalog.value.receive_modes : ["webhook"]).map((value) => ({ label: value, value })));
const credentialKeys = computed(() => selectedCatalog.value?.credential_schema?.required ?? []);

const configError = computed(() => jsonError(configExtraText.value));
const metadataError = computed(() => jsonError(metadataExtraText.value));
const canSave = computed(() => Boolean(form.key.trim() && form.name.trim() && selectedType.value && !configError.value && !metadataError.value));

watch(
  () => props.modelValue,
  (open) => {
    if (open) resetForm();
  }
);

watch(selectedType, (type) => {
  const item = props.catalog.find((entry) => entry.type === type);
  if (!item) return;
  if (!receiveMode.value || !item.receive_modes.includes(receiveMode.value)) {
    receiveMode.value = item.receive_modes[0] || "webhook";
  }
  if (!props.row && !form.key) form.key = type;
});

function resetForm() {
  const row = props.row;
  const cfg = parseJSON<ChannelConfig>(row?.config_json, {});
  const metadata = parseJSON<ChannelMetadata>(row?.metadata_json, {});
  selectedType.value = cfg.type || props.catalog[0]?.type || "";
  receiveMode.value = cfg.receive_mode || props.catalog.find((item) => item.type === selectedType.value)?.receive_modes[0] || "webhook";
  webhookPath.value = String(cfg.webhook?.path ?? "");
  defaultAgentId.value = String(cfg.routing?.default_agent_id ?? "main");
  externalId.value = metadata.external_id || "";
  iconUrl.value = metadata.icon_url || "";
  form.key = row?.key || selectedType.value;
  form.name = row?.name || props.catalog.find((item) => item.type === selectedType.value)?.label || "";
  form.description = row?.description || "";
  form.enabled = row?.enabled ?? true;
  configExtraText.value = JSON.stringify(cfg.config || {}, null, 2);
  metadataExtraText.value = JSON.stringify({ ...metadata, icon_url: undefined, external_id: undefined }, null, 2);
  Object.keys(credentialDraft).forEach((key) => delete credentialDraft[key]);
  credentialKeys.value.forEach((key) => {
    const existing = props.credentials.find((item) => item.credential_key === key);
    credentialDraft[key] = existing?.configured ? "" : "";
  });
}

async function save() {
  saving.value = true;
  try {
    const extraConfig = parseJSON<Record<string, unknown>>(configExtraText.value, {});
    const extraMetadata = parseJSON<Record<string, unknown>>(metadataExtraText.value, {});
    const config: ChannelConfig = {
      type: selectedType.value,
      receive_mode: receiveMode.value,
      webhook: { path: webhookPath.value },
      routing: { default_agent_id: defaultAgentId.value || "main" },
      config: extraConfig,
      accounts: []
    };
    const metadata: ChannelMetadata = {
      ...extraMetadata,
      icon_url: iconUrl.value,
      external_id: externalId.value,
      catalog_group: selectedCatalog.value?.group,
      schema_version: 1
    };
    const credentials: ChannelCredentialInput[] = Object.entries(credentialDraft)
      .filter(([, secret]) => secret.trim())
      .map(([credential_key, secret]) => ({ credential_key, secret: secret.trim(), metadata_json: "{}" }));
    const payload = {
      key: form.key.trim(),
      name: form.name.trim(),
      description: form.description.trim(),
      enabled: form.enabled,
      config_json: JSON.stringify(config),
      metadata_json: JSON.stringify(metadata),
      credentials
    };
    const saved = props.row ? await updateChannel(props.row.id, payload) : await createChannel(payload);
    emit("saved", saved);
    emit("update:modelValue", false);
    $q.notify({ type: "positive", message: "Channel 已保存" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存失败" });
  } finally {
    saving.value = false;
  }
}

function credentialLabel(key: string) {
  return key.replaceAll("_", " ");
}

function credentialHint(key: string) {
  const existing = props.credentials.find((item) => item.credential_key === key);
  return existing?.configured ? `已配置：${existing.masked_preview || "********"}；留空不修改` : "新建时建议填写";
}

function jsonError(value: string) {
  try {
    JSON.parse(value || "{}");
    return "";
  } catch (err) {
    return err instanceof Error ? err.message : "JSON 格式错误";
  }
}

function parseJSON<T>(value: string | undefined, fallback: T): T {
  if (!value) return fallback;
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
}
</script>

<style scoped>
.channel-editor-card {
  width: 960px;
  max-width: 96vw;
}

.section-label {
  font-weight: 700;
}
</style>
