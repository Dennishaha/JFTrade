<script setup lang="ts">
import type { ADKWorkflowDefinition } from "@/contracts";
import type { PageEnvelope } from "@/composables/adkWorkflowsApi";
import type { WorkflowTemplate } from "@/features/adkWorkflowStudio";

defineProps<{
  workflows: ADKWorkflowDefinition[];
  selectedWorkflowId: string;
  templates: Array<{
    value: WorkflowTemplate;
    title: string;
    description: string;
    icon: string;
  }>;
  showTemplatePicker: boolean;
  search: string;
  statusFilter: string;
  statusOptions: Array<{ title: string; value: string }>;
  loading: boolean;
  page: PageEnvelope;
  pageSummary: string;
  agentName: (agentId: string) => string;
  workModeLabel: (mode: string) => string;
  workflowTone: (status: string) => string;
  statusLabel: (status: string) => string;
}>();

defineEmits<{
  "update:showTemplatePicker": [value: boolean];
  "update:search": [value: string];
  "update:statusFilter": [value: string];
  "start-template": [template: WorkflowTemplate];
  "select-workflow": [workflow: ADKWorkflowDefinition];
  previous: [];
  next: [];
}>();
</script>

<template>
  <aside class="adk-workflow-studio__sidebar">
    <div class="adk-workflow-studio__brand">
      <div>
        <span class="adk-workflow-studio__eyebrow">工作流中心</span>
        <h2>工作流</h2>
      </div>
      <v-btn
        icon="fa-solid fa-plus"
        size="small"
        variant="text"
        title="新建工作流"
        @click="$emit('update:showTemplatePicker', !showTemplatePicker)"
      />
    </div>

    <div v-if="showTemplatePicker" class="adk-workflow-template-list">
      <button
        v-for="item in templates"
        :key="item.value"
        type="button"
        class="adk-workflow-template"
        @click="$emit('start-template', item.value)"
      >
        <v-icon size="14">{{ item.icon }}</v-icon>
        <span>
          <strong>{{ item.title }}</strong>
          <small>{{ item.description }}</small>
        </span>
      </button>
    </div>

    <div class="adk-workflow-studio__filters">
      <v-text-field
        :model-value="search"
        placeholder="搜索工作流"
        density="compact"
        hide-details
        @update:model-value="$emit('update:search', String($event ?? ''))"
      />
      <v-select
        :model-value="statusFilter"
        :items="statusOptions"
        density="compact"
        hide-details
        @update:model-value="$emit('update:statusFilter', String($event ?? ''))"
      />
    </div>

    <div class="adk-workflow-resource-list">
      <button
        v-for="workflow in workflows"
        :key="workflow.id"
        type="button"
        class="adk-workflow-resource"
        :class="{ 'is-active': workflow.id === selectedWorkflowId }"
        @click="$emit('select-workflow', workflow)"
      >
        <span class="adk-workflow-resource__icon">
          <v-icon size="14">fa-solid fa-diagram-project</v-icon>
        </span>
        <span class="adk-workflow-resource__main">
          <strong>{{ workflow.name }}</strong>
          <small>{{ agentName(workflow.agentId) }} · {{ workModeLabel(workflow.workMode) }}</small>
        </span>
        <span class="adk-workflow-pill" :class="workflowTone(workflow.status)">
          {{ statusLabel(workflow.status) }}
        </span>
      </button>
      <div v-if="!loading && workflows.length === 0" class="adk-workflow-empty">
        暂无工作流
      </div>
    </div>

    <div class="adk-workflow-studio__pager">
      <span>{{ pageSummary }}</span>
      <v-btn
        size="x-small"
        variant="text"
        :disabled="page.offset === 0"
        @click="$emit('previous')"
      >
        上一页
      </v-btn>
      <v-btn
        size="x-small"
        variant="text"
        :disabled="!page.hasMore"
        @click="$emit('next')"
      >
        下一页
      </v-btn>
    </div>
  </aside>
</template>
