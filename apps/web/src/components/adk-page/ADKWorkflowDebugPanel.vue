<script setup lang="ts">
import type { WorkflowInputRow } from "@/features/adkWorkflowForms";
import { inputTypeOptions } from "@/features/adkWorkflowStudio";

defineProps<{
  inputRows: WorkflowInputRow[];
  running: boolean;
}>();

defineEmits<{
  addInput: [];
  removeInput: [index: number];
  run: [];
}>();
</script>

<template>
  <section class="adk-debug-panel">
    <div class="adk-inspector-heading">
      <h3>调试输入</h3>
      <div class="adk-inspector-actions">
        <v-btn size="x-small" variant="text" @click="$emit('addInput')">添加输入</v-btn>
        <v-btn size="x-small" color="primary" :loading="running" @click="$emit('run')">
          开始调试
        </v-btn>
      </div>
    </div>
    <div class="adk-workflow-input-list">
      <div
        v-for="(row, index) in inputRows"
        :key="index"
        class="adk-workflow-input-row"
      >
        <v-text-field v-model="row.key" label="参数名" density="compact" hide-details />
        <v-select v-model="row.type" :items="inputTypeOptions" label="类型" density="compact" hide-details />
        <v-switch
          v-if="row.type === 'boolean'"
          v-model="row.booleanValue"
          label="开启"
          color="primary"
          hide-details
        />
        <v-text-field
          v-else
          v-model="row.value"
          :label="row.type === 'number' ? '调试数字' : '调试文本'"
          density="compact"
          hide-details
        />
        <v-btn
          icon="fa-solid fa-xmark"
          size="small"
          variant="text"
          color="error"
          @click="$emit('removeInput', index)"
        />
      </div>
    </div>
  </section>
</template>
