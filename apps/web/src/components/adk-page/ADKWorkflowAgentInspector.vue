<script setup lang="ts">
import type { ADKWorkflowNodeRun } from "@/contracts";
import type { WorkflowFormModel } from "@/features/adkWorkflowForms";
import {
  permissionOptions,
  workModeOptions,
} from "@/features/adkWorkflowStudio";
import ADKWorkflowNodeRunPreview from "./ADKWorkflowNodeRunPreview.vue";

defineProps<{
  workflowForm: WorkflowFormModel;
  selectedNodeRun: ADKWorkflowNodeRun | null;
  agentOptions: Array<{ title: string; value: string }>;
  providerOptions: Array<{ title: string; value: string }>;
  inputVariableOptions: Array<{ title: string; value: string }>;
  providerName: (providerId: string) => string;
}>();

defineEmits<{
  refreshNodeData: [];
  insertPromptVariable: [value: string];
}>();
</script>

<template>
  <section class="adk-inspector-section">
    <h3>执行配置</h3>
    <v-select
      v-model="workflowForm.agentId"
      :items="agentOptions"
      label="智能体"
      density="comfortable"
      @update:model-value="$emit('refreshNodeData')"
    />
    <div class="adk-inspector-grid">
      <v-select
        v-model="workflowForm.workMode"
        :items="workModeOptions"
        label="工作模式"
        density="comfortable"
        @update:model-value="$emit('refreshNodeData')"
      />
      <v-select
        v-model="workflowForm.permissionMode"
        :items="permissionOptions"
        label="审批等级"
        density="comfortable"
      />
    </div>
    <v-select
      v-model="workflowForm.providerId"
      :items="providerOptions"
      label="模型服务覆盖"
      density="comfortable"
      clearable
    />
    <v-text-field v-model="workflowForm.model" label="模型覆盖" density="comfortable" />
    <v-textarea
      v-model="workflowForm.objectiveTemplate"
      label="目标模板"
      :rows="2"
      density="comfortable"
    />

    <div class="adk-inspector-heading">
      <h3>运行指令</h3>
    </div>
    <div class="adk-variable-palette">
      <v-btn
        v-for="variable in inputVariableOptions"
        :key="variable.value"
        size="x-small"
        variant="outlined"
        @click="$emit('insertPromptVariable', variable.value)"
      >
        {{ variable.title }}
      </v-btn>
    </div>
    <v-textarea
      v-model="workflowForm.promptTemplate"
      label="运行指令模板"
      :rows="9"
      density="comfortable"
    />
    <div class="adk-workflow-model-meta">
      {{ providerName(workflowForm.providerId) }}
    </div>
    <ADKWorkflowNodeRunPreview :run="selectedNodeRun" />
  </section>
</template>
