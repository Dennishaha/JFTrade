<script setup lang="ts">
import ToolbarActionButton from "../shared/ToolbarActionButton.vue";

defineProps<{
  title: string;
  description: string;
  status: string;
  statusTone: string;
  statusLabel: string;
  loading: boolean;
  saving: boolean;
  runningWorkflow: boolean;
  logLoading: boolean;
  hasWorkflow: boolean;
}>();

defineEmits<{
  refresh: [];
  showInspector: [];
  addTrigger: [];
  addAgent: [];
  openLogs: [];
  run: [];
  debug: [];
  duplicate: [];
  saveTemplate: [];
  remove: [];
  save: [];
}>();
</script>

<template>
  <header class="adk-workflow-studio__topbar">
    <div class="adk-workflow-title-block">
      <v-icon size="17">fa-solid fa-wand-magic-sparkles</v-icon>
      <div>
        <h1>{{ title || "新的工作流" }}</h1>
        <span>{{ description || "开始 -> 智能体 -> 监控" }}</span>
      </div>
      <span class="adk-workflow-pill" :class="statusTone">
        {{ statusLabel }}
      </span>
    </div>
    <div class="adk-workflow-actions">
      <div class="adk-workflow-action-group" role="group" aria-label="基础操作">
        <ToolbarActionButton
          icon="fa-solid fa-rotate-right"
          label="刷新"
          :loading="loading"
          @click="$emit('refresh')"
        />
        <ToolbarActionButton
          data-testid="adk-workflow-inspector-show"
          class="adk-workflow-inspector-show"
          icon="fa-solid fa-chevron-left"
          label="显示右栏"
          @click="$emit('showInspector')"
        />
      </div>
      <div class="adk-workflow-action-group" role="group" aria-label="编排操作">
        <ToolbarActionButton
          icon="fa-solid fa-bolt"
          label="添加触发器"
          tone="info"
          @click="$emit('addTrigger')"
        />
        <ToolbarActionButton
          icon="fa-solid fa-robot"
          label="添加智能体"
          tone="info"
          @click="$emit('addAgent')"
        />
        <ToolbarActionButton
          icon="fa-solid fa-clock-rotate-left"
          label="触发日志"
          tone="info"
          :loading="logLoading"
          @click="$emit('openLogs')"
        />
      </div>
      <div class="adk-workflow-action-group" role="group" aria-label="执行操作">
        <ToolbarActionButton
          data-testid="adk-workflow-run-button"
          icon="fa-solid fa-play"
          label="运行"
          tone="success"
          :loading="runningWorkflow"
          :disabled="saving"
          @click="$emit('run')"
        />
        <ToolbarActionButton
          icon="fa-solid fa-vial"
          label="调试"
          tone="success"
          :disabled="saving"
          @click="$emit('debug')"
        />
      </div>
      <div class="adk-workflow-action-group" role="group" aria-label="复用操作">
        <ToolbarActionButton
          icon="fa-solid fa-copy"
          label="复制"
          tone="violet"
          :disabled="saving || !hasWorkflow"
          @click="$emit('duplicate')"
        />
        <ToolbarActionButton
          icon="fa-solid fa-bookmark"
          label="存为模板"
          tone="violet"
          :disabled="saving || !hasWorkflow"
          @click="$emit('saveTemplate')"
        />
      </div>
      <div v-if="hasWorkflow" class="adk-workflow-action-group" role="group" aria-label="危险操作">
        <ToolbarActionButton
          icon="fa-solid fa-trash"
          label="删除工作流"
          tone="danger"
          :disabled="saving"
          @click="$emit('remove')"
        />
      </div>
      <div class="adk-workflow-action-group" role="group" aria-label="保存操作">
        <ToolbarActionButton
          icon="fa-solid fa-floppy-disk"
          label="保存"
          tone="primary"
          :loading="saving"
          @click="$emit('save')"
        />
      </div>
    </div>
  </header>
</template>
