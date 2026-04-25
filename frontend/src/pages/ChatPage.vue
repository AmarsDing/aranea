<template>
  <q-page class="app-page-cream chat-page fit column no-wrap q-pa-sm" style="min-height: 0">
    <div class="row no-wrap col chat-page__row" style="min-height: 0">
      <ChatEntitySidebar
        v-model:search="search"
        v-model:agents="displayAgents"
        v-model:teams="displayTeams"
        :open="leftOpen"
        :selected-kind="selectedEntityKind"
        :selected-agent-id="store.selectedAgent?.id"
        :selected-team-id="selectedTeamId"
        :category-tree="categoryTree"
        :is-dark="isDark"
        @agent-reorder-end="onEndAgent"
        @team-reorder-end="onEndTeam"
        @select-agent="selectAgent"
        @select-team="selectTeam"
        @settings="openSettings"
        @delete="openDelete"
      />

      <ChatSideToggle
        :open="leftOpen"
        :icon="leftOpen ? 'chevron_left' : 'chevron_right'"
        :aria-label="t('chat.collapseList')"
        @toggle="leftOpen = !leftOpen"
      />

      <div class="col column no-wrap" style="min-width: 0; min-height: 0">
        <ChatMessagePanel
          v-model="inputText"
          v-model:dialog-mode="dialogMode"
          v-model:model-provider="modelProvider"
          :messages="displayMessages"
          :attachments="attachments"
          :mode-options="modeOpts"
          :provider-options="provOpts"
          :context-ratio="selectedSessionForUi?.context_used_ratio ?? 0"
          :is-dark="isDark"
          :sending="sending"
          @update:dialog-mode="onModeChange"
          @update:model-provider="onProviderChange"
          @remove-attachment="removeAttachment"
          @pick-file="pickFile"
          @voice="onVoiceClick"
          @send="onSend"
          @stop="stopStreaming"
        />
        <input ref="fileRef" type="file" hidden multiple @change="onFileChange" />
      </div>

      <ChatSideToggle
        :open="rightOpen"
        :icon="rightOpen ? 'chevron_right' : 'chevron_left'"
        :aria-label="t('chat.collapseSession')"
        @toggle="rightOpen = !rightOpen"
      />

      <ChatSessionSidebar
        :open="rightOpen"
        :sessions="displaySessions"
        :selected-session-id="selectedSessionForUi?.id"
        :is-dark="isDark"
        @select="onSelectSession"
        @new-session="onNewSession"
        @delete="openDelete"
      />
    </div>

    <ChatSettingsDialog
      v-model="settingsOpen"
      v-model:name="editName"
      v-model:provider="editProvider"
      v-model:model="editModel"
      :title="settingsTitle"
      :mode="settingsMode"
      :agent-key="editKey"
      :saving="settingsSaving"
      @save="onSaveSettings"
    />

    <ChatDeleteDialog
      v-model="deleteOpen"
      v-model:name-input="deleteNameInput"
      :title="deleteTitleText"
      :kind="deleteKind"
      :expected-name="expectedDeleteName"
      :blocked-busy="deleteBlockBusy"
      :can-confirm="canConfirmDelete"
      :has-name-error="Boolean(deleteNameError && deleteNameInput)"
      :deleting="deleting"
      @confirm="onConfirmDelete"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import { useRouter } from "vue-router";
import ChatDeleteDialog from "../components/chat/ChatDeleteDialog.vue";
import ChatEntitySidebar from "../components/chat/ChatEntitySidebar.vue";
import ChatMessagePanel from "../components/chat/ChatMessagePanel.vue";
import ChatSessionSidebar from "../components/chat/ChatSessionSidebar.vue";
import ChatSideToggle from "../components/chat/ChatSideToggle.vue";
import { listChatOptions, listTeams, updateAgent } from "../api/client";
import {
  listPlatformResources,
  listPlatformResourceTree,
  type PlatformResource,
  type PlatformResourceTreeNode
} from "../features/platform/api";
import type {
  Agent,
  ChatAttachment,
  ChatEntityKind,
  DeleteKind,
  Message,
  Session,
  SessionView,
  TeamRow
} from "../components/chat/types";
import {
  CHAT_MODE_OPTIONS,
  loadDialogModeFromStorage,
  loadModelFromStorage,
  saveDialogModeToStorage,
  saveModelToStorage
} from "../config/chatOptions";
import { useAppStore } from "../stores/app";

function mockTeamSession(
  id: string,
  teamID: string,
  title: string,
  contextUsedRatio: number,
  at: string
): Session & { at: string } {
  const now = new Date().toISOString();
  return {
    id,
    owner_type: "team",
    agent_id: "",
    team_id: teamID,
    title,
    context_used_ratio: contextUsedRatio,
    dialog_mode: "",
    provider: "",
    model: "",
    status: "active",
    last_message_at: now,
    created_at: now,
    updated_at: now,
    deleted_at: "",
    at
  };
}

function mockMessage(id: string, sessionID: string, role: string, content: string): Message {
  return {
    id,
    session_id: sessionID,
    parent_message_id: "",
    turn_index: 1,
    role,
    content_markdown: content,
    model_name: "mock",
    token_in: 0,
    token_out: 0,
    latency_ms: 0,
    status: "ok",
    attachments_count: 0,
    options_json: "",
    error_message: "",
    created_at: new Date().toISOString()
  };
}

const { t } = useI18n();
const $q = useQuasar();
const router = useRouter();
const store = useAppStore();

const LS_AG_ORDER = "chat:order:agents";
const LS_TM_ORDER = "chat:order:teams";

const isDark = computed(() => $q.dark.isActive);
const leftOpen = ref(true);
const rightOpen = ref(true);
const search = ref("");
const selectedEntityKind = ref<ChatEntityKind>("agent");
const selectedTeamId = ref<string | null>(null);
const teamSelectedSessionId = ref<string | null>(null);
const defaultAgentId = ref<string | null>(null);
const defaultTeamId = ref("team-default-1");
const fileRef = ref<HTMLInputElement | null>(null);

const displayAgents = ref<Agent[]>([]);
const categoryTree = ref<PlatformResourceTreeNode[]>([]);
const displayTeams = ref<TeamRow[]>([
  { id: "team-default-1", display_name: "Default Team", isDefault: true, isWorking: false },
  { id: "team-ops-1", display_name: "Ops", isDefault: false, isWorking: true }
]);

const teamSessions = ref<Record<string, Array<Session & { at: string }>>>({
  "team-default-1": [
    mockTeamSession("mock-ts-1", "team-default-1", "Sprint", 0.22, "14:00")
  ],
  "team-ops-1": []
});

const teamMessages = ref<Record<string, Message[]>>({
  "mock-ts-1": [
    mockMessage("m1", "mock-ts-1", "user", "排期同步一下？")
  ]
});

const inputText = ref("");
const dialogMode = ref(loadDialogModeFromStorage("default"));
const modelProvider = ref(loadModelFromStorage(""));
const sending = ref(false);
const streamAbortController = ref<AbortController | null>(null);
const modeOpts = ref(CHAT_MODE_OPTIONS.map((o) => ({ label: o.label, value: o.value })));
const providerModels = ref<PlatformResource[]>([]);
const provOpts = ref<Array<{ label: string; value: string; caption?: string }>>([]);
const attachments = ref<ChatAttachment[]>([]);

const settingsOpen = ref(false);
const settingsMode = ref<ChatEntityKind | null>(null);
const settingsId = ref<string | null>(null);
const editName = ref("");
const editKey = ref("");
const editProvider = ref("");
const editModel = ref("");
const settingsSaving = ref(false);

const deleteOpen = ref(false);
const deleteKind = ref<DeleteKind>("agent");
const deleteTargetId = ref<string | null>(null);
const deleteNameInput = ref("");
const deleteBlockBusy = ref(false);
const deleteBlockDefault = ref(false);
const deleting = ref(false);

const settingsTitle = computed(() => {
  if (settingsMode.value === "agent") return t("chat.settingsTitleAgent");
  if (settingsMode.value === "team") return t("chat.settingsTitleTeam");
  return t("chat.settings");
});
const selectedProviderModel = computed(() =>
  providerModels.value.find((row) => getProviderModelValue(row) === modelProvider.value)
);

const displaySessions = computed((): SessionView[] => {
  if (selectedEntityKind.value === "team" && selectedTeamId.value) {
    return (teamSessions.value[selectedTeamId.value] ?? []).map((session) => ({
      id: session.id,
      title: session.title,
      context_used_ratio: session.context_used_ratio,
      at: session.at,
      agent_id: session.agent_id
    }));
  }

  if (selectedEntityKind.value === "agent" && store.selectedAgent) {
    return store.sessions.map((session) => ({
      id: session.id,
      title: session.title,
      context_used_ratio: session.context_used_ratio,
      at: formatSessionTime(session.last_message_at || session.updated_at || session.created_at)
    }));
  }

  return [];
});

const selectedSessionForUi = computed((): SessionView | null => {
  if (selectedEntityKind.value === "team" && teamSelectedSessionId.value) {
    return displaySessions.value.find((session) => session.id === teamSelectedSessionId.value) ?? null;
  }

  if (!store.selectedSession) return null;

  return (
    displaySessions.value.find((session) => session.id === store.selectedSession!.id) ?? {
      id: store.selectedSession.id,
      title: store.selectedSession.title,
      context_used_ratio: store.selectedSession.context_used_ratio,
      at: ""
    }
  );
});

const displayMessages = computed((): Message[] => {
  if (selectedEntityKind.value === "team" && teamSelectedSessionId.value) {
    return teamMessages.value[teamSelectedSessionId.value] ?? [];
  }
  return store.messages;
});

const expectedDeleteName = computed(() => {
  if (deleteKind.value === "agent" && deleteTargetId.value) {
    return (
      store.agents.find((agent) => agent.id === deleteTargetId.value)?.display_name ??
      displayAgents.value.find((agent) => agent.id === deleteTargetId.value)?.display_name ??
      ""
    );
  }
  if (deleteKind.value === "team") {
    return displayTeams.value.find((team) => team.id === deleteTargetId.value)?.display_name ?? "";
  }
  if (deleteKind.value === "session") {
    return displaySessions.value.find((session) => session.id === deleteTargetId.value)?.title ?? "";
  }
  return "";
});

const deleteNameError = computed(
  () => deleteNameInput.value && deleteNameInput.value !== expectedDeleteName.value
);

const canConfirmDelete = computed(() => {
  if (deleteBlockBusy.value || deleteBlockDefault.value) return false;
  if (deleteKind.value === "all") return true;
  return deleteNameInput.value === expectedDeleteName.value;
});

const deleteTitleText = computed(() => {
  if (deleteKind.value === "agent") return t("chat.deleteTitleAgent");
  if (deleteKind.value === "team") return t("chat.deleteTitleTeam");
  if (deleteKind.value === "session") return t("chat.deleteTitleSession");
  return t("chat.deleteAllTitle");
});

function isAgentWorking(agent: Agent) {
  return /work|run|busy|ing/i.test(agent.status || "");
}

function onEndAgent() {
  if (defaultAgentId.value) {
    const current = displayAgents.value;
    if (current[0] && current[0].id !== defaultAgentId.value) {
      const fixed = current.find((agent) => agent.id === defaultAgentId.value);
      if (fixed) displayAgents.value = [fixed, ...current.filter((agent) => agent.id !== fixed.id)];
    }
  }
  try {
    localStorage.setItem(LS_AG_ORDER, JSON.stringify(displayAgents.value.map((agent) => agent.id)));
  } catch {
    // localStorage can be unavailable in restricted contexts.
  }
}

function onEndTeam() {
  const current = displayTeams.value;
  if (current[0] && current[0].id !== defaultTeamId.value) {
    const fixed = current.find((team) => team.id === defaultTeamId.value);
    if (fixed) displayTeams.value = [fixed, ...current.filter((team) => team.id !== fixed.id)];
  }
  try {
    localStorage.setItem(LS_TM_ORDER, JSON.stringify(displayTeams.value.map((team) => team.id)));
  } catch {
    // localStorage can be unavailable in restricted contexts.
  }
}

function loadAgentOrder(agents: Agent[], defaultId: string | null): Agent[] {
  if (agents.length === 0) return [];
  const defaultResolved = defaultId && agents.some((agent) => agent.id === defaultId) ? defaultId : agents[0]!.id;
  const ordered = applyStoredOrder(agents, LS_AG_ORDER);
  const fixed = ordered.find((agent) => agent.id === defaultResolved) ?? ordered[0]!;
  return [fixed, ...ordered.filter((agent) => agent.id !== fixed.id)];
}

function loadTeamOrder(teams: TeamRow[]): TeamRow[] {
  const ordered = applyStoredOrder(teams, LS_TM_ORDER);
  const fixed = ordered.find((team) => team.id === defaultTeamId.value) ?? ordered[0];
  return fixed ? [fixed, ...ordered.filter((team) => team.id !== fixed.id)] : ordered;
}

function applyStoredOrder<T extends { id: string }>(items: T[], key: string): T[] {
  const byId = new Map(items.map((item) => [item.id, item] as const));
  const ordered: T[] = [];

  try {
    const ids = JSON.parse(localStorage.getItem(key) || "[]") as string[];
    for (const id of ids) {
      const item = byId.get(id);
      if (item) ordered.push(item);
    }
  } catch {
    // Ignore malformed stored order and rebuild below.
  }

  for (const item of items) {
    if (!ordered.some((candidate) => candidate.id === item.id)) ordered.push(item);
  }

  return ordered;
}

async function selectAgent(agent: Agent) {
  selectedEntityKind.value = "agent";
  selectedTeamId.value = null;
  teamSelectedSessionId.value = null;
  store.selectedAgent = agent;
  await store.loadSessions();
  await nextTick();
  store.selectedSession = store.sessions[0] ?? null;
  store.messages = [];
  if (store.selectedSession) await store.loadMessages();
}

function selectTeam(team: TeamRow) {
  selectedEntityKind.value = "team";
  selectedTeamId.value = team.id;
  teamSelectedSessionId.value = teamSessions.value[team.id]?.[0]?.id ?? null;
  store.selectedSession = null;
  store.messages = [];
}

async function onSelectSession(sessionId: string) {
  if (selectedEntityKind.value === "team") {
    teamSelectedSessionId.value = sessionId;
    return;
  }

  const session = store.sessions.find((item) => item.id === sessionId) ?? null;
  store.selectedSession = session;
  store.messages = [];
  if (session) await store.loadMessages();
}

async function onNewSession() {
  if (selectedEntityKind.value === "agent" && store.selectedAgent) {
    const selectedModel = selectedProviderModel.value;
    await store.addSession(`S ${new Date().toLocaleString()}`, {
      dialog_mode: dialogMode.value,
      provider: selectedModel?.provider || store.selectedAgent.provider,
      model: selectedModel?.model || store.selectedAgent.model
    });
    if (store.selectedSession) await store.loadMessages();
    return;
  }

  if (selectedEntityKind.value === "team" && selectedTeamId.value) {
    const id = `mo-${Date.now()}`;
    const at = new Date().toLocaleTimeString();
    const row = mockTeamSession(id, selectedTeamId.value, `S ${at}`, 0, at.length > 5 ? at.slice(0, 5) : at);

    teamSessions.value[selectedTeamId.value] = [row, ...(teamSessions.value[selectedTeamId.value] ?? [])];
    teamSelectedSessionId.value = id;
    teamMessages.value[id] = [];
  }
}

async function onSend() {
  const content = inputText.value.trim();
  if (!content || sending.value) return;

  if (selectedEntityKind.value === "agent") {
    sending.value = true;
    streamAbortController.value = new AbortController();
    try {
      if (!store.selectedSession) await onNewSession();
      if (store.selectedSession) {
        const selectedModel = selectedProviderModel.value;
        inputText.value = "";
        await store.sendStream(
          content,
          {
            dialog_mode: dialogMode.value,
            provider: selectedModel?.provider || store.selectedSession.provider || store.selectedAgent?.provider || "",
            model: selectedModel?.model || store.selectedSession.model || store.selectedAgent?.model || "",
            attachments: attachments.value.map((item) => ({ id: item.id }))
          },
          streamAbortController.value.signal
        );
      }
    } catch (error) {
      if (!(error instanceof DOMException && error.name === "AbortError")) {
        $q.notify({
          type: "negative",
          message: error instanceof Error ? error.message : "发送失败，请稍后重试"
        });
      }
    } finally {
      sending.value = false;
      streamAbortController.value = null;
    }
    return;
  }

  if (selectedEntityKind.value === "team" && teamSelectedSessionId.value) {
    const sessionId = teamSelectedSessionId.value;
    inputText.value = "";
    const now = new Date().toISOString();
    teamMessages.value[sessionId] = [
      ...(teamMessages.value[sessionId] ?? []),
      { ...mockMessage(`u-${Date.now()}`, sessionId, "user", content), created_at: now },
      { ...mockMessage(`a-${Date.now()}`, sessionId, "assistant", "（Team 占位回复）"), created_at: now }
    ];
  }
}

function onModeChange(value: string) {
  dialogMode.value = value;
  saveDialogModeToStorage(value);
}

function onProviderChange(value: string) {
  modelProvider.value = value;
  saveModelToStorage(value);
}

function stopStreaming() {
  streamAbortController.value?.abort();
}

function formatSessionTime(iso: string) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

async function openSettings(kind: ChatEntityKind, id: string) {
  if (kind === "agent") {
    await router.push(`/agents/${id}/settings`);
    return;
  }

  settingsMode.value = kind;
  settingsId.value = id;

  const team = displayTeams.value.find((item) => item.id === id);
  if (team) editName.value = team.display_name;

  settingsOpen.value = true;
}

async function onSaveSettings() {
  settingsSaving.value = true;
  try {
    if (settingsMode.value === "agent" && settingsId.value) {
      const agent = store.agents.find((item) => item.id === settingsId.value) ?? displayAgents.value.find((item) => item.id === settingsId.value);
      if (agent) {
        const updated = await updateAgent(agent.id, {
          ...agent,
          display_name: editName.value,
          provider: editProvider.value,
          model: editModel.value
        });
        store.upsertAgent(updated);
        displayAgents.value = displayAgents.value.map((item) => (item.id === updated.id ? { ...item, ...updated } : item));
      }
    } else if (settingsMode.value === "team" && settingsId.value) {
      const team = displayTeams.value.find((item) => item.id === settingsId.value);
      if (team) team.display_name = editName.value;
    }

    settingsOpen.value = false;
    $q.notify({ type: "positive", message: t("chat.save") });
  } finally {
    settingsSaving.value = false;
  }
}

function openDelete(kind: DeleteKind, id: string) {
  deleteKind.value = kind;
  deleteTargetId.value = id;
  deleteNameInput.value = "";
  deleteBlockBusy.value = false;
  deleteBlockDefault.value = false;

  if (kind === "agent" && id) {
    const agent = store.agents.find((item) => item.id === id) ?? displayAgents.value.find((item) => item.id === id);
    deleteBlockBusy.value = agent ? isAgentWorking(agent) : false;
  }

  if (kind === "team" && id) {
    const team = displayTeams.value.find((item) => item.id === id);
    if (team?.isDefault) {
      deleteBlockDefault.value = true;
      $q.notify({ type: "warning", message: t("chat.deleteBlockedDefault") });
      return;
    }
    deleteBlockBusy.value = team?.isWorking ?? false;
  }

  deleteOpen.value = true;
}

async function onConfirmDelete() {
  const id = deleteTargetId.value;
  if (deleteBlockBusy.value || deleteBlockDefault.value) return;

  if (deleteKind.value === "agent" && id) {
    deleting.value = true;
    try {
      localStorage.removeItem(LS_AG_ORDER);
      await store.removeAgentFromList(id);
      displayAgents.value = displayAgents.value.filter((agent) => agent.id !== id);
      defaultAgentId.value = store.agents[0]?.id ?? null;
      if (selectedEntityKind.value === "agent") {
        if (store.selectedAgent) {
          await store.loadSessions();
          store.selectedSession = store.sessions[0] ?? null;
          store.messages = [];
          if (store.selectedSession) await store.loadMessages();
        } else if (displayTeams.value[0]) {
          selectTeam(displayTeams.value[0]);
        }
      }
    } finally {
      deleting.value = false;
    }
  } else if (deleteKind.value === "team" && id) {
    localStorage.removeItem(LS_TM_ORDER);
    displayTeams.value = displayTeams.value.filter((team) => team.id !== id);
    if (selectedTeamId.value === id) selectedTeamId.value = null;
  } else if (deleteKind.value === "session" && id) {
    await deleteSession(id);
  } else if (deleteKind.value === "all") {
    await clearSessions();
  }

  deleteOpen.value = false;
  $q.notify({ type: "info", message: t("chat.deleteSuccess") });
}

async function deleteSession(id: string) {
  if (selectedEntityKind.value === "team" && selectedTeamId.value) {
    teamSessions.value[selectedTeamId.value] = (teamSessions.value[selectedTeamId.value] ?? []).filter(
      (session) => session.id !== id
    );
    delete teamMessages.value[id];
    if (teamSelectedSessionId.value === id) {
      teamSelectedSessionId.value = teamSessions.value[selectedTeamId.value]?.[0]?.id ?? null;
    }
    return;
  }

  await store.removeSessionLocal(id);
  if (store.selectedSession) await store.loadMessages();
}

async function clearSessions() {
  if (selectedEntityKind.value === "agent") {
    await store.clearAllSessions();
    return;
  }

  if (selectedEntityKind.value === "team" && selectedTeamId.value) {
    teamSessions.value[selectedTeamId.value] = [];
    teamSelectedSessionId.value = null;
    for (const id of Object.keys(teamMessages.value)) delete teamMessages.value[id];
  }
}

function pickFile() {
  fileRef.value?.click();
}

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement;
  if (!input.files?.length) return;

  Array.from(input.files).forEach((file, i) => {
    const record: ChatAttachment = { id: `f-${Date.now()}-${i}`, name: file.name, progress: 0 };
    attachments.value.push(record);
    const timer = setInterval(() => {
      const stillExists = attachments.value.some((item) => item.id === record.id);
      if (!stillExists) {
        clearInterval(timer);
        return;
      }
      record.progress = Math.min(1, record.progress + 0.2);
      if (record.progress >= 1) {
        clearInterval(timer);
        delete record.timer;
      }
    }, 200);
    record.timer = timer;
  });

  input.value = "";
}

function removeAttachment(id: string) {
  const target = attachments.value.find((item) => item.id === id);
  if (target?.timer) clearInterval(target.timer);
  attachments.value = attachments.value.filter((item) => item.id !== id);
}

function onVoiceClick() {
  $q.notify({ type: "info", message: t("chat.voicePlaceholder") });
}

onMounted(async () => {
  await Promise.all([loadChatOptions(), loadCategoryTree(), loadTeams()]);
  await store.loadAgents();
  defaultAgentId.value = store.agents[0]?.id ?? null;
  displayAgents.value = loadAgentOrder(store.agents, defaultAgentId.value);
  displayTeams.value = loadTeamOrder([...displayTeams.value]);

  if (store.selectedAgent) {
    await store.loadSessions();
    store.selectedSession = store.sessions[0] ?? null;
    store.messages = [];
    if (store.selectedSession) await store.loadMessages();
  } else if (store.agents[0]) {
    await selectAgent(store.agents[0]);
  } else {
    selectTeam(displayTeams.value[0]!);
  }
});

async function loadCategoryTree() {
  categoryTree.value = await listPlatformResourceTree("agent-categories");
}

async function loadTeams() {
  try {
    const rows = await listTeams();
    if (rows.length) {
      displayTeams.value = rows.map((team) => ({
        id: team.id,
        team_key: team.team_key,
        display_name: team.display_name,
        status: team.status,
        isDefault: team.is_default,
        isWorking: /work|run|busy|ing/i.test(team.status || ""),
        definition_json: team.definition_json
      }));
    }
  } catch {
    // Keep placeholder teams when the backend has not seeded teams yet.
  }
}

async function loadChatOptions() {
  try {
    const [modes, models] = await Promise.all([
      listChatOptions("dialog_mode"),
      listPlatformResources("llm-provider-models")
    ]);
    if (modes.length) {
      modeOpts.value = modes.map((item) => ({ label: item.label, value: item.key }));
    }
    providerModels.value = models.filter((item) => item.enabled);
    if (providerModels.value.length) {
      provOpts.value = providerModels.value.map((item) => ({
        label: item.name || item.model,
        value: getProviderModelValue(item),
        caption: `${item.provider} / ${item.model}`
      }));
      ensureSelectedModel();
    }
  } catch {
    // Keep local fallback options when backend config is unavailable.
  }
}

function ensureSelectedModel() {
  if (!providerModels.value.length) return;
  const stored = providerModels.value.find((item) => getProviderModelValue(item) === modelProvider.value);
  if (stored) return;

  const agentModel = store.selectedAgent
    ? providerModels.value.find(
        (item) => item.provider === store.selectedAgent?.provider && item.model === store.selectedAgent?.model
      )
    : null;
  const nextModel = agentModel ?? providerModels.value[0];
  modelProvider.value = getProviderModelValue(nextModel);
  saveModelToStorage(modelProvider.value);
}

function getProviderModelValue(row: PlatformResource) {
  return row.key || `${row.provider}:${row.model}`;
}
</script>
