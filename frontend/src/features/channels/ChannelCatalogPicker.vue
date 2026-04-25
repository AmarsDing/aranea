<template>
  <div class="row q-col-gutter-sm">
    <div v-for="item in catalog" :key="item.type" class="col-12 col-sm-6 col-md-4">
      <q-card
        flat
        bordered
        :class="['catalog-card cursor-pointer', { selected: item.type === modelValue }]"
        @click="$emit('update:modelValue', item.type)"
      >
        <q-card-section>
          <div class="row items-start no-wrap q-gutter-sm">
            <q-avatar color="primary" text-color="white" size="34px">{{ item.label.slice(0, 1) }}</q-avatar>
            <div>
              <div class="text-weight-bold">{{ item.label }}</div>
              <div class="text-caption text-grey-7">{{ item.group }} · {{ item.receive_modes.join(", ") }}</div>
            </div>
          </div>
          <div class="text-caption text-grey-7 q-mt-sm catalog-desc">{{ item.description }}</div>
        </q-card-section>
      </q-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ChannelCatalogItem } from "./types";

defineProps<{
  catalog: ChannelCatalogItem[];
  modelValue: string;
}>();

defineEmits<{
  "update:modelValue": [value: string];
}>();
</script>

<style scoped>
.catalog-card {
  height: 100%;
  border-radius: 16px;
  transition: border-color 0.16s ease, transform 0.16s ease;
}

.catalog-card:hover,
.catalog-card.selected {
  border-color: var(--q-primary);
  transform: translateY(-1px);
}

.catalog-desc {
  min-height: 36px;
}
</style>
