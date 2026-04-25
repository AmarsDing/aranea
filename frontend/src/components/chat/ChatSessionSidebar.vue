<template>
  <transition name="chat-side">
    <aside v-show="open" class="chat-side chat-side--right column no-wrap">
      <div class="chat-session-header row items-center justify-between">
        <div class="text-caption text-cream-muted text-uppercase">
          Session
        </div>
        <q-badge rounded color="primary" :label="sessions.length" />
      </div>
      <q-scroll-area class="col">
        <q-list class="chat-session-list" dense>
          <q-item
            v-for="session in sessions"
            :key="session.id"
            clickable
            :active="selectedSessionId === session.id"
            :active-class="isDark ? 'bg-primary' : 'cream-menu-item--active'"
            class="chat-session-item rounded-borders q-mb-sm q-px-sm"
            @click="$emit('select', session.id)"
          >
            <q-item-section side>
              <div class="column items-center" style="gap: 2px">
                <q-circular-progress
                  :value="session.context_used_ratio * 100"
                  show-value
                  size="32px"
                  :thickness="0.22"
                  color="primary"
                >
                  <span class="text-caption" style="font-size: 0.6rem">
                    {{ Math.round(session.context_used_ratio * 100) }}%
                  </span>
                </q-circular-progress>
                <q-btn
                  dense
                  round
                  flat
                  size="sm"
                  icon="close"
                  color="negative"
                  class="chat-action-btn chat-danger-btn"
                  :aria-label="t('chat.remove')"
                  @click.stop="$emit('delete', 'session', session.id)"
                />
              </div>
            </q-item-section>
            <q-item-section class="ellipsis" style="max-width: 1px">
              <q-item-label class="ellipsis" lines="2" style="text-align: right">
                {{ session.title }}
              </q-item-label>
              <q-item-label caption class="text-cream-muted ellipsis" style="text-align: right">
                {{ session.at }}
              </q-item-label>
            </q-item-section>
          </q-item>
        </q-list>
      </q-scroll-area>
      <q-separator class="cream-sep" />
      <div class="chat-session-actions row no-wrap q-gutter-sm">
        <q-btn
          unelevated
          dense
          class="chat-primary-btn col"
          color="primary"
          no-caps
          :label="t('chat.newSession')"
          @click="$emit('new-session')"
        />
        <q-btn
          outline
          dense
          class="chat-outline-danger-btn col"
          color="negative"
          no-caps
          :label="t('chat.clearAllSession')"
          @click="$emit('delete', 'all', '')"
        />
      </div>
    </aside>
  </transition>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";
import type { DeleteKind, SessionView } from "./types";

defineProps<{
  open: boolean;
  sessions: SessionView[];
  selectedSessionId?: string | null;
  isDark: boolean;
}>();

defineEmits<{
  select: [id: string];
  "new-session": [];
  delete: [kind: DeleteKind, id: string];
}>();

const { t } = useI18n();
</script>

<style scoped>
.chat-side--right {
  width: 300px;
  min-width: 280px;
  flex: 0 0 300px;
}
</style>
