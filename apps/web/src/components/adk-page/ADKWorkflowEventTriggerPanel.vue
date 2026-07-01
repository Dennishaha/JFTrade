<script setup lang="ts">
import type { TriggerFormModel } from "@/features/adkWorkflowForms";

defineProps<{
  triggerForm: TriggerFormModel;
}>();
</script>

<template>
  <div class="adk-inspector-section is-inner">
    <h3>事件匹配</h3>
    <v-text-field v-model="triggerForm.event.source" label="来源" density="comfortable" />
    <v-text-field
      v-model="triggerForm.event.eventType"
      label="事件类型"
      density="comfortable"
    />
    <div class="adk-inspector-grid">
      <v-text-field
        v-model="triggerForm.event.entityId"
        label="实体标识"
        density="comfortable"
      />
      <v-text-field
        v-model="triggerForm.event.category"
        label="分类"
        density="comfortable"
      />
    </div>
    <div class="adk-inspector-grid">
      <v-text-field v-model="triggerForm.event.level" label="级别" density="comfortable" />
      <v-text-field
        v-model="triggerForm.event.cooldownSec"
        label="冷却秒数"
        type="number"
        density="comfortable"
      />
    </div>
  </div>
</template>
