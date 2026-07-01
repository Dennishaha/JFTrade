<script setup lang="ts">
import type { ADKWorkflowNodeRun } from "@/contracts";
import type { WorkflowFormModel } from "@/features/adkWorkflowForms";
import {
  inputTypeOptions,
  workflowEditStatusOptions,
} from "@/features/adkWorkflowStudio";
import ADKWorkflowNodeRunPreview from "./ADKWorkflowNodeRunPreview.vue";

defineProps<{
  workflowForm: WorkflowFormModel;
  selectedNodeRun: ADKWorkflowNodeRun | null;
  preservedInputCount: number;
}>();

defineEmits<{
  refreshNodeData: [];
  addInputRow: [];
  removeInputRow: [index: number];
}>();
</script>

<template>
  <section class="adk-inspector-section">
    <h3>基础信息</h3>
    <v-text-field
      v-model="workflowForm.name"
      label="名称"
      density="comfortable"
      @update:model-value="$emit('refreshNodeData')"
    />
    <v-text-field v-model="workflowForm.description" label="描述" density="comfortable" />
    <div class="adk-inspector-grid">
      <v-select
        v-model="workflowForm.status"
        :items="workflowEditStatusOptions"
        label="状态"
        density="comfortable"
        @update:model-value="$emit('refreshNodeData')"
      />
      <v-text-field v-model="workflowForm.tagsText" label="标签" density="comfortable" />
    </div>
    <div class="adk-inspector-heading">
      <h3>输入项</h3>
      <v-btn size="small" variant="outlined" @click="$emit('addInputRow')">添加</v-btn>
    </div>
    <div class="adk-workflow-input-list">
      <div
        v-for="(row, index) in workflowForm.inputRows"
        :key="index"
        class="adk-workflow-input-row"
      >
        <v-text-field
          v-model="row.key"
          label="参数名"
          density="compact"
          hide-details
          class="adk-workflow-input-item"
          @update:model-value="$emit('refreshNodeData')"
        />
        <v-select
          v-model="row.type"
          :items="inputTypeOptions"
          label="类型"
          density="compact"
          hide-details
          class="adk-workflow-input-item"
        />
        <v-switch
          v-if="row.type === 'boolean'"
          v-model="row.booleanValue"
          label="默认开启"
          color="primary"
          hide-details
          class="adk-workflow-input-item"
        />
        <v-text-field
          v-else
          v-model="row.value"
          :label="row.type === 'number' ? '默认数字' : '默认文本'"
          density="compact"
          hide-details
        />
        <v-btn
          icon="fa-solid fa-xmark"
          size="small"
          variant="text"
          color="error"
          @click="$emit('removeInputRow', index)"
        />
      </div>
      <div v-if="workflowForm.inputRows.length === 0" class="adk-workflow-muted">
        暂无输入项
      </div>
      <span v-if="preservedInputCount > 0" class="adk-workflow-preserved">
        已保留 {{ preservedInputCount }} 个复杂输入字段
      </span>
    </div>
    <ADKWorkflowNodeRunPreview :run="selectedNodeRun" />
  </section>
</template>
