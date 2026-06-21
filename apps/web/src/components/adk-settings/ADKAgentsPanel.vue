<script setup lang="ts">
import { computed, ref, watch } from "vue";

import type {
  ADKAgent,
  ADKPermissionMode,
  ADKToolDescriptor,
  ADKWorkMode,
} from "@/contracts";

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
    recentUserWindow: number;
    workMode: ADKWorkMode;
    loopMaxIterations: number;
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
const templateDialogOpen = ref(false);
const checkedAvailableTools = ref<string[]>([]);
const checkedEnabledTools = ref<string[]>([]);
const workModeOptions: Array<{ title: string; value: ADKWorkMode }> = [
  { title: "对话", value: "chat" },
  { title: "任务", value: "task" },
  { title: "目标", value: "loop" },
];

function workModeLabel(mode: string): string {
  switch (mode) {
    case "task":
      return "任务";
    case "loop":
      return "目标";
    default:
      return "对话";
  }
}

const enabledToolNameSet = computed(() => new Set(props.agentForm.tools));
const toolDescriptorByName = computed(
  () => new Map(props.tools.map((tool) => [tool.name, tool])),
);
const availableRuntimeTools = computed(() =>
  props.tools.filter((tool) => {
    if (enabledToolNameSet.value.has(tool.name)) return false;
    if (props.toolCategoryFilter && tool.category !== props.toolCategoryFilter) return false;
    if (props.toolRiskFilter && tool.riskLevel !== props.toolRiskFilter) return false;
    return true;
  }),
);
const enabledRuntimeTools = computed(() =>
  props.agentForm.tools.map((toolName) => ({
    name: toolName,
    descriptor: toolDescriptorByName.value.get(toolName),
  })),
);

function openCustomNewAgentDialog(): void {
  props.newAgentForm();
  agentDialogOpen.value = true;
}

function openTemplateDialog(): void {
  templateDialogOpen.value = true;
}

function selectAgentTemplate(template: Omit<ADKAgent, "createdAt" | "updatedAt">): void {
  props.applyAgentTemplate(template);
  templateDialogOpen.value = false;
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

function addTools(toolNames: string[]): void {
  const currentTools = new Set(props.agentForm.tools);
  for (const toolName of toolNames) {
    if (!currentTools.has(toolName)) {
      props.agentForm.tools.push(toolName);
      currentTools.add(toolName);
    }
  }
  checkedAvailableTools.value = [];
}

function removeTools(toolNames: string[]): void {
  const removedTools = new Set(toolNames);
  const nextTools = props.agentForm.tools.filter((toolName) => !removedTools.has(toolName));
  props.agentForm.tools.splice(0, props.agentForm.tools.length, ...nextTools);
  checkedEnabledTools.value = [];
}

function addSelectedTools(): void {
  addTools(checkedAvailableTools.value);
}

function addAllFilteredTools(): void {
  addTools(availableRuntimeTools.value.map((tool) => tool.name));
}

function removeSelectedTools(): void {
  removeTools(checkedEnabledTools.value);
}

function removeAllTools(): void {
  removeTools(props.agentForm.tools);
}

watch(
  () => [props.toolCategoryFilter, props.toolRiskFilter, props.agentForm.tools.join("\n")],
  () => {
    const availableToolNames = new Set(availableRuntimeTools.value.map((tool) => tool.name));
    const enabledToolNames = new Set(props.agentForm.tools);
    checkedAvailableTools.value = checkedAvailableTools.value.filter((toolName) =>
      availableToolNames.has(toolName),
    );
    checkedEnabledTools.value = checkedEnabledTools.value.filter((toolName) =>
      enabledToolNames.has(toolName),
    );
  },
);
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
      <v-card-actions class="flex flex-wrap gap-2 mx-3">
        <v-btn color="primary" variant="outlined" size="small" @click="openTemplateDialog">
          从模板新建
        </v-btn>
        <v-btn variant="outlined" size="small" @click="openCustomNewAgentDialog">
          自定义新建
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
                <v-chip size="x-small" variant="tonal">
                  {{ formatPermission(agent.permissionMode) }}
                </v-chip>
                <v-chip size="x-small" variant="tonal">
                  默认：{{ workModeLabel(agent.workMode) }}
                </v-chip>
              </div>
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
          尚未创建任何智能体。可以从模板开始，也可以自定义新建。
        </v-card-text>
      </v-card>
    </div>

    <v-dialog v-model="templateDialogOpen" max-width="760" content-class="adk-agent-template-dialog-overlay">
      <v-card class="adk-agent-template-dialog">
        <v-card-title class="flex items-center justify-between gap-3">
          <span>选择智能体模板</span>
          <v-btn icon="mdi-close" variant="text" size="small" @click="templateDialogOpen = false" />
        </v-card-title>
        <v-card-text class="adk-agent-template-dialog__body grid gap-3">
          <div>
            <div class="adk-agent-template-dialog__title text-sm font-semibold">内置智能体模板</div>
            <div class="adk-agent-template-dialog__hint mt-1 text-xs">
              选择模板后会进入编辑界面，保存智能体后生效。
            </div>
          </div>
          <div class="grid gap-3 md:grid-cols-2">
            <button
              v-for="template in agentTemplates"
              :key="template.id"
              type="button"
              class="adk-agent-template-card"
              @click="selectAgentTemplate(template)"
            >
              <span class="adk-agent-template-card__name">{{ template.name }}</span>
              <span class="adk-agent-template-card__meta">
                {{ formatPermission(template.permissionMode) }} · {{ template.tools.length }} 个工具 · {{ template.skills.length }} 个技能
              </span>
            </button>
          </div>
          <div v-if="agentTemplates.length === 0" class="adk-agent-template-dialog__empty">
            暂无可用模板。
          </div>
        </v-card-text>
        <v-card-actions class="justify-end">
          <v-btn variant="text" @click="templateDialogOpen = false">取消</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="agentDialogOpen" max-width="980" content-class="adk-agent-dialog-overlay">
      <v-card class="adk-agent-dialog">
        <v-card-title class="flex items-center justify-between gap-3">
          <span>{{ agentForm.id ? "编辑智能体" : "新建智能体" }}</span>
          <v-btn icon="mdi-close" variant="text" size="small" @click="agentDialogOpen = false" />
        </v-card-title>
        <v-card-text class="adk-agent-dialog__body grid gap-4">
          <div class="grid gap-3 md:grid-cols-2">
            <v-text-field v-model="agentForm.name" label="名称" density="comfortable" />
            <v-select v-model="agentForm.providerId" :items="providerOptions" label="模型服务" density="comfortable"
              clearable />
            <v-text-field v-model="agentForm.model" label="覆盖模型（可选）" density="comfortable" />
            <v-select v-model="agentForm.permissionMode" :items="permissionModes" label="默认审批等级" density="comfortable" />
            <v-radio-group
              v-model="agentForm.workMode"
              class="md:col-span-2"
              label="默认工作模式"
              inline
              hide-details
            >
              <v-radio
                v-for="mode in workModeOptions"
                :key="mode.value"
                :label="mode.title"
                :value="mode.value"
              />
            </v-radio-group>
            <v-text-field
              v-model.number="agentForm.recentUserWindow"
              label="保留最近用户消息条数"
              type="number"
              density="comfortable"
              min="2"
              max="100"
            />
            <v-text-field
              v-if="agentForm.workMode === 'loop'"
              v-model.number="agentForm.loopMaxIterations"
              label="目标循环最大轮次"
              type="number"
              density="comfortable"
              min="1"
              max="20"
            />
          </div>

          <div class="adk-tool-transfer rounded-lg border p-3">
            <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
              <div>
                <div class="adk-tool-transfer__title text-sm font-semibold">运行时工具穿梭框</div>
                <div class="adk-tool-transfer__hint text-xs">
                  左侧为已接入运行时工具，右侧为当前智能体启用工具。
                </div>
              </div>
              <v-chip size="small" variant="tonal">
                已启用 {{ agentForm.tools.length }}/{{ tools.length }}
              </v-chip>
            </div>
            <div class="grid gap-3 md:grid-cols-2">
              <v-select :model-value="toolCategoryFilter" label="按工具类别过滤" density="comfortable" clearable
                :items="toolCategoryOptions" @update:model-value="emit('update:toolCategoryFilter', $event ?? '')" />
              <v-select :model-value="toolRiskFilter" label="按风险等级过滤" density="comfortable" clearable
                :items="toolRiskOptions" @update:model-value="emit('update:toolRiskFilter', $event ?? '')" />
            </div>

            <div class="adk-tool-transfer__grid">
              <div class="adk-tool-transfer__panel">
                <div class="adk-tool-transfer__heading">
                  <span>可用运行时工具</span>
                  <span>{{ availableRuntimeTools.length }}</span>
                </div>
                <div class="adk-tool-transfer__list">
                  <label
                    v-for="tool in availableRuntimeTools"
                    :key="tool.name"
                    class="adk-tool-transfer__item"
                  >
                    <v-checkbox
                      v-model="checkedAvailableTools"
                      class="adk-tool-transfer__checkbox"
                      density="compact"
                      hide-details
                      :value="tool.name"
                    />
                    <span class="min-w-0 flex-1">
                      <span class="block truncate text-sm font-medium">{{ tool.displayName || tool.name }}</span>
                      <span class="adk-tool-transfer__meta block truncate text-xs">{{ tool.name }}</span>
                    </span>
                    <v-chip size="x-small" variant="tonal" :color="riskColor(tool.riskLevel)">
                      {{ riskLabel(tool.riskLevel) }}
                    </v-chip>
                  </label>
                  <div v-if="availableRuntimeTools.length === 0" class="adk-tool-transfer__empty">
                    当前筛选下没有可添加的运行时工具。
                  </div>
                </div>
              </div>

              <div class="adk-tool-transfer__actions">
                <v-btn
                  size="small"
                  color="primary"
                  variant="tonal"
                  :disabled="checkedAvailableTools.length === 0"
                  @click="addSelectedTools"
                >
                  添加
                </v-btn>
                <v-btn
                  size="small"
                  variant="outlined"
                  :disabled="availableRuntimeTools.length === 0"
                  @click="addAllFilteredTools"
                >
                  全部添加
                </v-btn>
                <v-btn
                  size="small"
                  variant="outlined"
                  :disabled="checkedEnabledTools.length === 0"
                  @click="removeSelectedTools"
                >
                  移除
                </v-btn>
                <v-btn
                  size="small"
                  color="error"
                  variant="tonal"
                  :disabled="agentForm.tools.length === 0"
                  @click="removeAllTools"
                >
                  全部移除
                </v-btn>
              </div>

              <div class="adk-tool-transfer__panel">
                <div class="adk-tool-transfer__heading">
                  <span>启用工具</span>
                  <span>{{ enabledRuntimeTools.length }}</span>
                </div>
                <div class="adk-tool-transfer__list">
                  <label
                    v-for="tool in enabledRuntimeTools"
                    :key="tool.name"
                    class="adk-tool-transfer__item"
                  >
                    <v-checkbox
                      v-model="checkedEnabledTools"
                      class="adk-tool-transfer__checkbox"
                      density="compact"
                      hide-details
                      :value="tool.name"
                    />
                    <span class="min-w-0 flex-1">
                      <span class="block truncate text-sm font-medium">
                        {{ tool.descriptor?.displayName || tool.name }}
                      </span>
                      <span class="adk-tool-transfer__meta block truncate text-xs">{{ tool.name }}</span>
                    </span>
                    <v-chip
                      v-if="tool.descriptor"
                      size="x-small"
                      variant="tonal"
                      :color="riskColor(tool.descriptor.riskLevel)"
                    >
                      {{ riskLabel(tool.descriptor.riskLevel) }}
                    </v-chip>
                  </label>
                  <div v-if="enabledRuntimeTools.length === 0" class="adk-tool-transfer__empty">
                    尚未为该智能体启用工具。
                  </div>
                </div>
              </div>
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
  display: flex;
  max-height: 80dvh;
  flex-direction: column;
  overflow: hidden;
  background: var(--tv-bg-surface);
  color: var(--card-text-1);
}

:global(.adk-agent-dialog-overlay) {
  background: var(--tv-bg-surface);
  border-radius: 4px;
}

.adk-agent-dialog__body {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  background: var(--tv-bg-surface);
}

.adk-agent-template-dialog {
  display: flex;
  max-height: 80dvh;
  flex-direction: column;
  overflow: hidden;
  background: var(--tv-bg-surface);
  color: var(--card-text-1);
}

:global(.adk-agent-template-dialog-overlay) {
  background: var(--tv-bg-surface);
  border-radius: 4px;
}

.adk-agent-template-dialog__body {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  background: var(--tv-bg-surface);
}

.adk-agent-template-dialog__title {
  color: var(--card-text-1);
}

.adk-agent-template-dialog__hint,
.adk-agent-template-dialog__empty {
  color: var(--card-text-3);
}

.adk-agent-template-card {
  display: grid;
  gap: 0.35rem;
  width: 100%;
  cursor: pointer;
  border: 1px solid var(--card-border);
  border-radius: 0.9rem;
  background: var(--tv-bg-surface-2);
  color: var(--card-text-2);
  padding: 0.85rem 0.95rem;
  text-align: left;
  transition: background 0.15s ease, border-color 0.15s ease, transform 0.15s ease;
}

.adk-agent-template-card:hover,
.adk-agent-template-card:focus-visible {
  border-color: var(--card-active-border);
  background: var(--card-active-surface);
  transform: translateY(-1px);
  outline: none;
}

.adk-agent-template-card__name {
  color: var(--card-text-1);
  font-size: 0.9rem;
  font-weight: 700;
}

.adk-agent-template-card__meta {
  color: var(--card-text-3);
  font-size: 0.75rem;
}

.adk-tool-transfer {
  border-color: var(--card-border);
  background: var(--tv-bg-surface-2);
}

.adk-tool-transfer__title {
  color: var(--card-text-1);
}

.adk-tool-transfer__hint,
.adk-tool-transfer__meta {
  color: var(--card-text-3);
}

.adk-tool-transfer__grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
  gap: 0.75rem;
  align-items: stretch;
}

.adk-tool-transfer__panel {
  min-width: 0;
  overflow: hidden;
  border: 1px solid var(--card-border);
  border-radius: 0.75rem;
  background: var(--tv-bg-surface);
}

.adk-tool-transfer__heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.65rem 0.75rem;
  border-bottom: 1px solid var(--card-border);
  color: var(--card-text-2);
  font-size: 0.75rem;
  font-weight: 700;
}

.adk-tool-transfer__list {
  display: grid;
  gap: 0.4rem;
  max-height: min(32dvh, 20rem);
  overflow-y: auto;
  padding: 0.5rem;
}

.adk-tool-transfer__item {
  display: flex;
  min-width: 0;
  cursor: pointer;
  align-items: center;
  gap: 0.5rem;
  border-radius: 0.65rem;
  padding: 0.35rem 0.5rem 0.35rem 0.15rem;
  color: var(--card-text-1);
  transition: background 0.15s ease, transform 0.15s ease;
}

.adk-tool-transfer__item:hover {
  background: var(--card-active-surface);
  transform: translateX(1px);
}

.adk-tool-transfer__checkbox {
  flex: 0 0 auto;
}

.adk-tool-transfer__actions {
  display: flex;
  width: 7.5rem;
  flex-direction: column;
  justify-content: center;
  gap: 0.5rem;
}

.adk-tool-transfer__empty {
  padding: 1.25rem 0.75rem;
  text-align: center;
  font-size: 0.75rem;
  color: var(--card-text-3);
}

@media (max-width: 760px) {
  .adk-tool-transfer__grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .adk-tool-transfer__actions {
    width: 100%;
    flex-direction: row;
    flex-wrap: wrap;
  }

  .adk-tool-transfer__actions :deep(.v-btn) {
    flex: 1 1 7rem;
  }
}
</style>
