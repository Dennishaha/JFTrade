<script setup lang="ts">
import { computed } from "vue";
import type { ADKWorkflowNodeRun } from "@/contracts";
import type { WorkflowFormModel } from "@/features/adkWorkflowForms";
import type { FlowNodeData } from "@/features/adkWorkflowStudio";
import {
  permissionOptions,
} from "@/features/adkWorkflowStudio";
import ADKWorkflowNodeRunPreview from "./ADKWorkflowNodeRunPreview.vue";

const props = defineProps<{
  workflowForm: WorkflowFormModel;
  selectedNodeRun: ADKWorkflowNodeRun | null;
  selectedNodeId: string;
  selectedAgentNodeData: FlowNodeData;
  agentOptions: Array<{ title: string; value: string }>;
  providerOptions: Array<{ title: string; value: string }>;
  inputVariableOptions: Array<{ title: string; value: string }>;
  providerName: (providerId: string) => string;
}>();

const emit = defineEmits<{
  refreshNodeData: [];
  insertPromptVariable: [value: string];
  updateAgentNodeData: [payload: { key: string; value: unknown }];
}>();

function nodeStringField(key: string, fallback = "") {
  return computed({
    get: () => String(props.selectedAgentNodeData[key] ?? fallback),
    set: (value: string) => emit("updateAgentNodeData", { key, value }),
  });
}

const nodeTitle = nodeStringField("title", "智能体");
const nodeAgentId = nodeStringField("agentId", props.workflowForm.agentId);
const nodeProviderId = nodeStringField("providerId", "");
const nodeModel = nodeStringField("model", "");
const nodePermissionMode = nodeStringField("permissionMode", "");
const nodeObjectiveTemplate = nodeStringField("objectiveTemplate", "");
const nodePromptTemplate = nodeStringField("promptTemplate", "");

function insertNodePromptVariable(value: string): void {
  const prefix = nodePromptTemplate.value.endsWith("\n") || nodePromptTemplate.value === "" ? "" : "\n";
  nodePromptTemplate.value += `${prefix}${value}`;
}
</script>

<template>
  <section class="adk-inspector-section">
    <h3>执行配置</h3>
    <v-text-field
      v-model="nodeTitle"
      label="节点标题"
      density="comfortable"
      @update:model-value="$emit('refreshNodeData')"
    />
    <v-select
      v-model="nodeAgentId"
      :items="agentOptions"
      label="智能体"
      density="comfortable"
      @update:model-value="$emit('refreshNodeData')"
    />
    <div class="adk-inspector-grid">
      <v-select
        v-model="nodePermissionMode"
        :items="permissionOptions"
        label="审批等级"
        density="comfortable"
        clearable
      />
    </div>
    <v-select
      v-model="nodeProviderId"
      :items="providerOptions"
      label="模型服务覆盖"
      density="comfortable"
      clearable
    />
    <v-text-field v-model="nodeModel" label="模型覆盖" density="comfortable" />
    <v-textarea
      v-model="nodeObjectiveTemplate"
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
        @click="insertNodePromptVariable(variable.value)"
      >
        {{ variable.title }}
      </v-btn>
    </div>
    <v-textarea
      v-model="nodePromptTemplate"
      label="运行指令模板"
      :rows="9"
      density="comfortable"
    />
    <div class="adk-workflow-model-meta">
      {{ providerName(nodeProviderId) }}
    </div>
    <ADKWorkflowNodeRunPreview :run="selectedNodeRun" />
  </section>
</template>
