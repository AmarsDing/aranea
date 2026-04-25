<template>
  <q-card flat bordered class="col column chat-mid-card" style="min-height: 0; border-radius: 16px">
    <q-card-section class="chat-messages col q-pa-sm q-pa-md-md scroll">
      <div v-if="!messages.length" class="chat-empty-state column items-center justify-center">
        <q-icon name="forum" size="34px" color="primary" />
        <div class="text-cream-text text-weight-medium q-mt-sm">{{ t("chat.emptyMessages") }}</div>
        <div class="text-cream-muted text-caption q-mt-xs">{{ t("chat.inputLabel") }}</div>
      </div>
      <q-chat-message
        v-for="message in messages"
        :key="message.id"
        :sent="message.role === 'user'"
        :bg-color="message.role === 'user' ? 'primary' : isDark ? 'blue-grey-7' : 'light-blue-1'"
        :text-color="message.role === 'user' ? 'white' : 'grey-9'"
      >
        <div
          class="chat-message-content"
          :class="{ 'chat-message-content--sent': message.role === 'user' }"
          v-html="renderMarkdown(message.content_markdown)"
        />
        <template #name>
          <span
            :class="message.role === 'user' ? 'text-white' : isDark ? 'text-white' : 'text-cream-text'"
          >
            {{ message.role === "user" ? t("chat.me") : t("chat.assistant") }}
          </span>
        </template>
        <template #stamp>
          <span
            :class="message.role === 'user' ? 'text-white' : isDark ? 'text-grey-4' : 'text-cream-muted'"
          >
            {{ formatStamp(message.created_at) }}
          </span>
        </template>
      </q-chat-message>
    </q-card-section>

    <q-separator class="cream-sep" />
    <q-card-section class="chat-composer q-pa-sm q-pa-md-sm">
      <div v-if="attachments.length" class="chat-attachments row q-gutter-xs q-mb-sm">
        <div
          v-for="file in attachments"
          :key="file.id"
          class="chat-file-tile row items-center"
        >
          <q-circular-progress
            v-if="file.progress < 1"
            :value="file.progress * 100"
            size="28px"
            :thickness="0.2"
            color="primary"
            class="q-mr-xs"
          />
          <q-icon v-else name="insert_drive_file" size="20px" class="q-mr-xs" color="primary" />
          <span class="ellipsis text-caption" style="max-width: 56px">{{ file.name }}</span>
          <q-btn
            icon="close"
            class="chat-file-tile__close"
            size="sm"
            round
            dense
            flat
            @click="$emit('remove-attachment', file.id)"
          />
        </div>
      </div>

      <q-input
        :model-value="modelValue"
        filled
        class="chat-input"
        :label="t('chat.inputLabel')"
        type="textarea"
        autogrow
        :input-style="{ minHeight: '100px' }"
        :dark="isDark"
        :disable="sending"
        @update:model-value="$emit('update:modelValue', String($event ?? ''))"
      />

      <div class="chat-toolbar row items-center q-col-gutter-sm q-mt-sm">
        <q-select
          :model-value="dialogMode"
          dense
          options-dense
          outlined
          :options="modeOptions"
          emit-value
          map-options
          :label="t('chat.dialogMode')"
          class="chat-toolbar-field col-12 col-md-4"
          :dark="isDark"
          @update:model-value="$emit('update:dialogMode', String($event ?? ''))"
        />
        <q-select
          :model-value="modelProvider"
          dense
          options-dense
          outlined
          :options="providerOptions"
          emit-value
          map-options
          :label="t('chat.modelProvider')"
          class="chat-toolbar-field col-12 col-md-4"
          :dark="isDark"
          @update:model-value="$emit('update:modelProvider', String($event ?? ''))"
        >
          <template #option="scope">
            <q-item v-bind="scope.itemProps">
              <q-item-section>
                <q-item-label>{{ scope.opt.label }}</q-item-label>
                <q-item-label v-if="scope.opt.caption" caption>{{ scope.opt.caption }}</q-item-label>
              </q-item-section>
            </q-item>
          </template>
        </q-select>
        <div class="chat-toolbar-actions col-12 col-md-4 row items-center no-wrap q-gutter-sm">
          <div class="chat-context-pill row items-center no-wrap">
            <span class="text-caption text-no-wrap q-mr-sm">
              {{ t("chat.contextUse") }}
            </span>
            <q-circular-progress
              :value="contextRatio * 100"
              show-value
              size="34px"
              :thickness="0.2"
              color="primary"
            >
              <span class="text-caption">{{ Math.round(contextRatio * 100) }}%</span>
            </q-circular-progress>
          </div>
          <q-space class="gt-sm" />
          <q-btn
            round
            dense
            unelevated
            color="primary"
            :aria-label="t('chat.fileImport')"
            class="chat-icon-btn q-ml-sm"
            @click="$emit('pick-file')"
          >
            <q-icon name="attach_file" />
          </q-btn>
          <q-btn
            round
            dense
            unelevated
            outline
            color="primary"
            :aria-label="t('chat.voiceInput')"
            class="chat-icon-btn"
            @click="$emit('voice')"
          >
            <q-icon name="mic" />
          </q-btn>
          <q-btn
            v-if="sending"
            round
            dense
            unelevated
            color="negative"
            :aria-label="t('chat.stop')"
            class="chat-icon-btn chat-send-btn"
            @click="$emit('stop')"
          >
            <q-icon name="stop" />
          </q-btn>
          <q-btn
            v-else
            round
            dense
            unelevated
            color="primary"
            :aria-label="t('chat.send')"
            class="chat-icon-btn chat-send-btn"
            @click="$emit('send')"
          >
            <q-icon name="send" />
          </q-btn>
        </div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import DOMPurify from "dompurify";
import MarkdownIt from "markdown-it";
import { useI18n } from "vue-i18n";
import type { ChatAttachment, Message } from "./types";

type Option = { label: string; value: string; caption?: string };

defineProps<{
  modelValue: string;
  messages: Message[];
  attachments: ChatAttachment[];
  dialogMode: string;
  modelProvider: string;
  modeOptions: Option[];
  providerOptions: Option[];
  contextRatio: number;
  isDark: boolean;
  sending?: boolean;
}>();

defineEmits<{
  "update:modelValue": [value: string];
  "update:dialogMode": [value: string];
  "update:modelProvider": [value: string];
  "remove-attachment": [id: string];
  "pick-file": [];
  voice: [];
  send: [];
  stop: [];
}>();

const { t } = useI18n();
const markdown = new MarkdownIt({
  breaks: true,
  html: false,
  linkify: true
});
markdown.enable(["table", "strikethrough"]);

function formatStamp(iso: string) {
  if (!iso) return "";
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

function renderMarkdown(content: string) {
  return DOMPurify.sanitize(markdown.render(content || ""));
}
</script>

<style scoped lang="sass">
.chat-file-tile
  position: relative
  min-width: 30px
  min-height: 30px
  max-width: 120px
  padding: 2px 4px
  background: rgba(255, 255, 255, 0.75)
  border: 1px solid rgba(141, 110, 99, 0.2)
  border-radius: 6px

  .chat-file-tile__close
    position: absolute
    top: -4px
    right: -4px
    opacity: 0

  &:hover .chat-file-tile__close
    opacity: 1

.chat-message-content
  max-width: min(760px, 74vw)
  font-size: 14px
  overflow-wrap: anywhere
  white-space: normal
  line-height: 1.65

  :deep(h1),
  :deep(h2),
  :deep(h3),
  :deep(h4),
  :deep(h5),
  :deep(h6)
    margin: 0.9em 0 0.45em
    color: #1f2937
    font-weight: 700
    line-height: 1.28

  :deep(h1)
    font-size: 24px

  :deep(h2)
    font-size: 21px

  :deep(h3)
    font-size: 18px

  :deep(h4),
  :deep(h5),
  :deep(h6)
    font-size: 16px

  :deep(h1:first-child),
  :deep(h2:first-child),
  :deep(h3:first-child)
    margin-top: 0

  :deep(p)
    margin: 0 0 0.65em

  :deep(p:last-child)
    margin-bottom: 0

  :deep(ul),
  :deep(ol)
    margin: 0.35em 0 0.8em
    padding-left: 1.35em

  :deep(table)
    display: block
    width: max-content
    max-width: 100%
    overflow-x: auto
    margin: 0.85em 0 1em
    border-collapse: collapse
    border: 1px solid rgba(100, 116, 139, 0.38)
    border-radius: 10px
    background: rgba(255, 255, 255, 0.72)
    font-size: 13px
    line-height: 1.55

  :deep(thead)
    background: rgba(25, 118, 210, 0.1)

  :deep(th),
  :deep(td)
    min-width: 96px
    padding: 8px 10px
    border: 1px solid rgba(100, 116, 139, 0.32)
    text-align: left
    vertical-align: top

  :deep(th)
    color: #1f2937
    font-weight: 700

  :deep(td)
    color: #374151

  :deep(pre)
    max-width: 100%
    overflow-x: auto
    margin: 0.8em 0
    padding: 12px
    border-radius: 10px
    background: rgba(15, 23, 42, 0.92)
    color: #e5e7eb
    line-height: 1.55

  :deep(code)
    padding: 0.12em 0.35em
    border-radius: 5px
    background: rgba(15, 23, 42, 0.1)
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace

  :deep(pre code)
    padding: 0
    background: transparent

  :deep(blockquote)
    margin: 0.8em 0
    padding-left: 0.9em
    border-left: 3px solid rgba(25, 118, 210, 0.45)
    color: rgba(31, 41, 55, 0.78)

.chat-message-content--sent
  :deep(h1),
  :deep(h2),
  :deep(h3),
  :deep(h4),
  :deep(h5),
  :deep(h6)
    color: #fff

  :deep(a)
    color: #fff

  :deep(code)
    background: rgba(255, 255, 255, 0.18)

  :deep(table)
    background: rgba(255, 255, 255, 0.08)
    border-color: rgba(255, 255, 255, 0.35)

  :deep(thead)
    background: rgba(255, 255, 255, 0.18)

  :deep(th),
  :deep(td)
    color: #fff
    border-color: rgba(255, 255, 255, 0.28)
</style>
