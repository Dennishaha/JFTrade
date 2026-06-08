<script setup lang="ts">
import { ref } from "vue";

import type {
  ADKAgent,
  ADKPermissionMode,
  ADKToolDescriptor,
} from "@jftrade/ui-contracts";

const props = defineProps<{
  agentForm: {
    id: string;
    name: string;
    instruction: string;
    providerId: string;
    model: string;
    tools: string[];
    skills: string[];
    permissionMode: ADKPermissionMode;
    memoryEnabled: boolean;
    status: string;
  };
  agents: ADKAgent[];
  agentTemplates: Array<Omit<ADKAgent, "createdAt" | "updatedAt">>;
  agentTemplateNotice: string;
  providerOptions: Array<{ title: string; value: string }>;
  toolOptions: Array<{ title: string; value: string }>;
  skillOptions: Array<{ title: string; value: string }>;
  permissionModes: Array<{ title: string; value: ADKPermissionMode }>;
  tools: ADKToolDescriptor[];
  toolCategoryFilter: string;
  toolCategoryOptions: Array<string | undefined>;
  toolRiskFilter: string;
  toolRiskOptions: Array<string | undefined>;
  formatPermission: (mode: string) => string;
  riskColor: (risk?: string) => string;
  riskLabel: (risk?: string) => string;
  applyAgentTemplate: (template: Omit<ADKAgent, "createdAt" | "updatedAt">) => void;
  saveAgent: () => void | Promise<void>;
  newAgentForm: () => void;
  editAgent: (agent: ADKAgent) => void;
  duplicateAgent: (agent: ADKAgent) => void;
  deleteAgent: (agentId: string) => void | Promise<void>;
}>();

const emit = defineEmits<{
  "update:toolCategoryFilter": [value: string];
  "update:toolRiskFilter": [value: string];
}>();

const agentDialogOpen = ref(false);

function openNewAgentDialog(): void {
  props.newAgentForm();
  agentDialogOpen.value = true;
}

function openEditAgentDialog(agent: ADKAgent): void {
  props.editAgent(agent);
  agentDialogOpen.value = true;
}

function openDuplicateAgentDialog(agent: ADKAgent): void {
  props.duplicateAgent(agent);
  agentDialogOpen.value = true;
}

async function submitAgentForm(): Promise<void> {
  await props.saveAgent();
  agentDialogOpen.value = false;
}
</script>

<template>
  <section class="grid gap-4">
    <v-card flat class="card-shell border-0">
      <v-card-title class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div class="text-base font-semibold text-slate-900">智能体</div>
          <div class="mt-1 text-xs text-slate-500">
            默认展示已创建的智能体；新增或编辑时再打开配置表单。
          </div>
        </div>
      </v-card-title>
      <v-card-actions>
        <v-btn color="primary" size="small" @click="openNewAgentDialog">
          新增智能体
        </v-btn>
      </v-card-actions>
    </v-card>

    <div class="grid auto-rows-max gap-3 md:grid-cols-2 xl:grid-cols-3">
      <v-card v-for="agent in agents" :key="agent.id" flat class="card-shell border-0">
        <v-card-text>
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <span class="font-semibold text-slate-900">{{ agent.name }}</span>
                <v-chip size="x-small" variant="tonal" :color="agent.status === 'ENABLED' ? 'success' : 'default'">
                  {{ agent.status }}
                </v-chip>
              </div>
              <div class="text-xs text-slate-500">{{ formatPermission(agent.permissionMode) }}</div>
              <div class="mt-1 text-xs text-slate-500">
                {{ agent.memoryEnabled ? "记忆已开启" : "记忆已关闭" }}
              </div>
              <div class="mt-2 flex flex-wrap gap-1">
                <v-chip v-for="tool in agent.tools.slice(0, 5)" :key="tool" size="x-small" variant="outlined">
                  {{ tool }}
                </v-chip>
                <span v-if="agent.tools.length > 5" class="text-xs text-slate-500">
                  +{{ agent.tools.length - 5 }}
                </span>
              </div>
            </div>
            <div class="flex shrink-0 flex-col gap-1">
              <v-btn size="x-small" variant="outlined" @click="openEditAgentDialog(agent)">编辑</v-btn>
              <v-btn size="x-small" variant="outlined" @click="openDuplicateAgentDialog(agent)">复制</v-btn>
              <v-btn size="x-small" variant="outlined" color="error" @click="deleteAgent(agent.id)">删除</v-btn>
            </div>
          </div>
        </v-card-text>
      </v-card>
      <v-card v-if="agents.length === 0" flat class="card-shell border-0 md:col-span-2 xl:col-span-3">
        <v-card-text class="text-sm text-slate-500">
          尚未创建任何智能体。点击“新增智能体”后，可以从内置模板开始配置。
        </v-card-text>
      </v-card>
    </div>

    <v-dialog v-model="agentDialogOpen" max-width="980">
      <v-card class="adk-agent-dialog">
        <v-card-title class="flex items-center justify-between gap-3">
          <span>{{ agentForm.id ? "编辑智能体" : "新建智能体" }}</span>
          <v-btn icon="mdi-close" variant="text" size="small" @click="agentDialogOpen = false" />
        </v-card-title>
        <v-card-text class="grid gap-4">
          <div class="adk-template-panel grid gap-3 rounded-lg p-4">
            <div>
              <div class="adk-template-panel__title text-sm font-semibold">内置智能体模板</div>
              <div class="adk-template-panel__hint text-xs">
                选择模板会填充当前表单，保存智能体后生效。
              </div>
            </div>
            <div class="flex flex-wrap gap-2">
              <v-btn v-for="template in agentTemplates" :key="template.id" class="adk-template-panel__template-btn"
                size="small" variant="outlined" @click="applyAgentTemplate(template)">
                {{ template.name }}
              </v-btn>
            </div>
            <v-alert v-if="agentTemplateNotice" type="info" variant="tonal" density="compact">
              {{ agentTemplateNotice }}
            </v-alert>
            <div class="grid gap-3 md:grid-cols-2">
              <v-select :model-value="toolCategoryFilter" label="按工具类别过滤" density="comfortable" clearable
                :items="toolCategoryOptions" @update:model-value="emit('update:toolCategoryFilter', $event ?? '')" />
              <v-select :model-value="toolRiskFilter" label="按风险等级过滤" density="comfortable" clearable
                :items="toolRiskOptions" @update:model-value="emit('update:toolRiskFilter', $event ?? '')" />
            </div>
          </div>

          <div class="grid gap-3 md:grid-cols-2">
            <v-text-field v-model="agentForm.name" label="名称" density="comfortable" />
            <v-select v-model="agentForm.providerId" :items="providerOptions" label="模型服务" density="comfortable"
              clearable />
            <v-text-field v-model="agentForm.model" label="覆盖模型（可选）" density="comfortable" />
            <v-select v-model="agentForm.permissionMode" :items="permissionModes" label="权限模式" density="comfortable" />
          </div>

          <v-select v-model="agentForm.tools" :items="toolOptions" label="启用工具" density="comfortable" multiple chips
            closable-chips />
          <div class="grid gap-2 rounded border border-slate-200 p-3">
            <div class="text-xs font-medium text-slate-600">已接入运行时工具</div>
            <div class="flex flex-wrap gap-1">
              <v-chip v-for="tool in tools" :key="tool.name" size="x-small" variant="tonal"
                :color="riskColor(tool.riskLevel)" :title="`${tool.description} ${tool.outputSummary ?? ''}`">
                {{ tool.name }} · {{ riskLabel(tool.riskLevel) }}
              </v-chip>
            </div>
          </div>
          <v-select v-model="agentForm.skills" :items="skillOptions" label="启用技能" density="comfortable" multiple chips
            closable-chips />
          <v-textarea v-model="agentForm.instruction" label="系统指令" :rows="5" density="comfortable" />
          <div class="flex gap-6">
            <v-switch v-model="agentForm.memoryEnabled" label="记忆" color="primary" hide-details />
            <v-switch v-model="agentForm.status" true-value="ENABLED" false-value="DISABLED" label="启用" color="primary"
              hide-details />
          </div>
        </v-card-text>
        <v-card-actions class="justify-end gap-2">
          <v-btn variant="text" @click="agentDialogOpen = false">取消</v-btn>
          <v-btn color="primary" @click="submitAgentForm">保存智能体</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </section>
</template>

<style scoped>
.adk-agent-dialog {
  background: var(--card-surface);
  color: var(--card-text-1);
}

.adk-template-panel {
  border: 1px solid var(--card-border);
  background:
    linear-gradient(180deg,
      color-mix(in srgb, var(--card-surface) 96%, var(--tv-accent) 4%),
      var(--card-surface));
  color: var(--card-text-1);
  box-shadow: 0 18px 56px rgba(2, 6, 23, 0.16);
}

.adk-template-panel__title {
  color: var(--card-text-1);
}

.adk-template-panel__hint {
  color: var(--card-text-3);
}

.adk-template-panel__template-btn {
  color: var(--card-text-2);
  border-color: var(--card-border);
}

.adk-template-panel__template-btn:hover {
  color: var(--card-active-text);
  border-color: var(--card-active-border);
  background: var(--card-active-surface);
}
</style>
