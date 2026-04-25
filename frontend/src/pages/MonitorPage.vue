<template>
  <q-page class="q-pa-md">
    <q-card flat bordered>
      <q-card-section class="row items-center justify-between">
        <div>
          <div class="text-h6">运行监控</div>
          <div class="text-caption text-grey-7">查看审计、平台事件和 Trace 摘要。</div>
        </div>
        <q-btn dense flat icon="refresh" @click="load" />
      </q-card-section>
      <q-separator />
      <q-tabs v-model="tab" dense align="left" active-color="primary" indicator-color="primary">
        <q-tab name="audit" label="Audit" />
        <q-tab name="events" label="Events" />
        <q-tab name="traces" label="Traces" />
      </q-tabs>
      <q-separator />
      <q-card-section class="panel-scroll">
        <q-tab-panels v-model="tab" animated>
          <q-tab-panel name="audit">
            <q-list bordered separator>
              <q-item v-for="line in auditLines" :key="line.id">
                <q-item-section>
                  <q-item-label>{{ line.created_at }} | {{ line.action }} {{ line.resource }}</q-item-label>
                  <q-item-label caption>{{ line.request_id }} | {{ line.detail }}</q-item-label>
                </q-item-section>
              </q-item>
            </q-list>
          </q-tab-panel>
          <q-tab-panel name="events">
            <q-list bordered separator>
              <q-item v-for="event in events" :key="event.id">
                <q-item-section>
                  <q-item-label>{{ event.name }}</q-item-label>
                  <q-item-label caption>{{ event.created_at }} | {{ event.status }} | {{ event.description }}</q-item-label>
                </q-item-section>
                <q-item-section side>
                  <q-badge :color="event.enabled ? 'positive' : 'grey'">{{ event.key || "event" }}</q-badge>
                </q-item-section>
              </q-item>
            </q-list>
          </q-tab-panel>
          <q-tab-panel name="traces">
            <q-list bordered separator>
              <q-item v-for="trace in traces" :key="trace.id">
                <q-item-section>
                  <q-item-label>{{ trace.name }}</q-item-label>
                  <q-item-label caption>{{ trace.created_at }} | {{ trace.status }} | {{ trace.description }}</q-item-label>
                </q-item-section>
                <q-item-section side>
                  <q-badge color="primary">{{ trace.key || "trace" }}</q-badge>
                </q-item-section>
              </q-item>
            </q-list>
          </q-tab-panel>
        </q-tab-panels>
      </q-card-section>
    </q-card>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { listAuditLogs, type AuditLog } from "../api/client";
import { listPlatformResources, type PlatformResource } from "../features/platform/api";

const tab = ref("audit");
const auditLines = ref<AuditLog[]>([]);
const events = ref<PlatformResource[]>([]);
const traces = ref<PlatformResource[]>([]);

onMounted(load);

async function load() {
  const [auditRows, eventRows, traceRows] = await Promise.all([
    listAuditLogs(),
    listPlatformResources("monitor-events"),
    listPlatformResources("monitor-traces")
  ]);
  auditLines.value = auditRows;
  events.value = eventRows;
  traces.value = traceRows;
}
</script>
