<script setup lang="ts">
import { computed, ref, watch } from "vue";

import type {
  ADKAgent,
  ADKPermissionMode,
  ADKProvider,
  ADKReasoningEffort,
  ADKToolAccessMode,
  ADKToolDescriptor,
  ADKWorkMode,
} from "@/types";
import type { ActionConfirmationController } from "@/composables/shared/useActionConfirmation";

import ActionConfirmationHost from "../shared/ActionConfirmationHost.vue";
import StatusChip from "../shared/StatusChip.vue";
import {
  ADK_REASONING_EFFORT_LABELS,
  supportedADKReasoningEfforts,
} from "@/composables/adk/adkReasoning";

const props = defineProps<{
  actionConfirmation: ActionConfirmationController;
  agentForm: {
    id: string;
    name: string;
    instruction: string;
    providerId: string;
    model: string;
    reasoningEffort: ADKReasoningEffort | "";
    tools: string[];
    toolAccessMode: ADKToolAccessMode;
    skills: string[];
    permissionMode: ADKPermissionMode;
    memoryEnabled: boolean;
    recentUserWindow: number;
    workMode: ADKWorkMode;
    loopMaxIterations: number;
    status: string;
  };
  agents: ADKAgent[];
  providers?: ADKProvider[];
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
  { title: "目标", value: "loop" },
];
const reasoningEffortOptions = computed<Array<{ title: string; value: ADKReasoningEffort | "" }>>(() => {
  const provider =
    props.providers?.find((item) => item.id === props.agentForm.providerId) ??
    props.providers?.find((item) => item.default) ??
    props.providers?.[0];
  return [
    { title: "未配置（模型默认）", value: "" },
    ...supportedADKReasoningEfforts(provider).map((value) => ({
      title: ADK_REASONING_EFFORT_LABELS[value],
      value,
    })),
  ];
});
const toolAccessModeOptions: Array<{ title: string; value: ADKToolAccessMode }> = [
  { title: "全部工具", value: "all" },
  { title: "按选择启用", value: "selected" },
  { title: "不启用工具", value: "none" },
];

function workModeLabel(mode: string): string {
  switch (mode) {
    case "loop":
      return "目标";
    default:
      return "对话";
  }
}

function enabledToolCountLabel(agent: Pick<ADKAgent, "tools" | "toolAccessMode">): string {
  if (agent.toolAccessMode === "none") return "无工具";
  if (agent.toolAccessMode === "all" || agent.tools.length === 0) return "全部工具";
  return `${agent.tools.length} 个工具`;
}

function reasoningEffortLabel(agent: Pick<ADKAgent, "reasoningEffort">): string {
  return agent.reasoningEffort
    ? ADK_REASONING_EFFORT_LABELS[agent.reasoningEffort]
    : "模型默认";
}

function templateToolCountLabel(template: Pick<ADKAgent, "tools" | "toolAccessMode">): string {
  return enabledToolCountLabel(template);
}

function primaryDefaultAgentForm(): boolean {
  return props.agentForm.id === "jftrade-default";
}

function primaryDefaultAgent(agent: Pick<ADKAgent, "id">): boolean {
  return agent.id === "jftrade-default";
}

const enabledToolNameSet = computed(() => new Set(props.agentForm.tools));
const toolAccessMode = computed(() => props.agentForm.toolAccessMode);
const allToolsEnabled = computed(() => toolAccessMode.value === "all");
const noToolsEnabled = computed(() => toolAccessMode.value === "none");
const toolDescriptorByName = computed(
  () => new Map(props.tools.map((tool) => [tool.name, tool])),
);
const displayedAgents = computed(() =>
  [...props.agents].sort((left, right) => {
    const leftDefault = primaryDefaultAgent(left);
    const rightDefault = primaryDefaultAgent(right);
    if (leftDefault === rightDefault) return 0;
    return leftDefault ? -1 : 1;
  }),
);
const availableRuntimeTools = computed(() =>
  props.tools.filter((tool) => {
    if (!allToolsEnabled.value && noToolsEnabled.value) return false;
    if (enabledToolNameSet.value.has(tool.name)) return false;
    if (props.toolCategoryFilter && tool.category !== props.toolCategoryFilter) return false;
    if (props.toolRiskFilter && tool.riskLevel !== props.toolRiskFilter) return false;
    return true;
  }),
);
const enabledRuntimeTools = computed(() =>
  allToolsEnabled.value
    ? props.tools.map((tool) => ({ name: tool.name, descriptor: tool }))
    : props.agentForm.tools.map((toolName) => ({
        name: toolName,
        descriptor: toolDescriptorByName.value.get(toolName),
      })),
);

function stopButtonEvent(event?: Event): void {
  event?.preventDefault();
  event?.stopPropagation();
}

function openCustomNewAgentDialog(event?: Event): void {
  stopButtonEvent(event);
  templateDialogOpen.value = false;
  props.newAgentForm();
  agentDialogOpen.value = true;
}

function openTemplateDialog(event?: Event): void {
  stopButtonEvent(event);
  agentDialogOpen.value = false;
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
  props.agentForm.toolAccessMode = "selected";
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
  if (props.agentForm.toolAccessMode !== undefined) {
    props.agentForm.toolAccessMode = "none";
  }
  props.agentForm.tools.splice(0, props.agentForm.tools.length);
  checkedEnabledTools.value = [];
}

function enableAllTools(): void {
  props.agentForm.toolAccessMode = "all";
  checkedAvailableTools.value = [];
  checkedEnabledTools.value = [];
}

watch(
  () => [props.toolCategoryFilter, props.toolRiskFilter, props.agentForm.toolAccessMode, props.agentForm.tools.join("\n")],
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
        <v-btn type="button" color="primary" variant="outlined" size="small" @click="openTemplateDialog">
          从模板新建
        </v-btn>
        <v-btn type="button" variant="outlined" size="small" @click="openCustomNewAgentDialog">
          自定义新建
        </v-btn>
      </v-card-actions>
    </v-card>

    <div class="grid auto-rows-max gap-3 md:grid-cols-2 xl:grid-cols-3">
      <v-card v-for="agent in displayedAgents" :key="agent.id" flat class="card-shell border-0">
        <v-card-text>
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <span class="font-semibold text-slate-900">{{ agent.name }}</span>
                <StatusChip :status="agent.status" size="x-small" />
                <v-chip size="x-small" variant="tonal">
                  {{ formatPermission(agent.permissionMode) }}
                </v-chip>
                <v-chip size="x-small" variant="tonal">
                  默认：{{ workModeLabel(agent.workMode) }}
                </v-chip>
                <v-chip size="x-small" variant="tonal" color="primary">
                  推理：{{ reasoningEffortLabel(agent) }}
                </v-chip>
                <v-chip v-if="agent.builtin" size="x-small" variant="tonal" color="info">
                  系统默认
                </v-chip>
              </div>
              <div class="mt-1 text-xs text-slate-500">
                {{ agent.memoryEnabled ? "记忆已开启" : "记忆已关闭" }} · {{ enabledToolCountLabel(agent) }}
              </div>
              <div class="mt-2 flex flex-wrap gap-1">
                <v-chip v-if="agent.toolAccessMode === 'all' || (agent.toolAccessMode === undefined && agent.tools.length === 0)" size="x-small" variant="outlined">
                  全部工具
                </v-chip>
                <v-chip v-else-if="agent.toolAccessMode === 'none'" size="x-small" variant="outlined">
                  无工具
                </v-chip>
                <v-chip v-for="tool in agent.tools.slice(0, 5)" :key="tool" size="x-small" variant="outlined">
                  {{ tool }}
                </v-chip>
                <span v-if="agent.tools.length > 5" class="text-xs text-slate-500">
                  +{{ agent.tools.length - 5 }}
                </span>
              </div>
            </div>
            <div class="flex shrink-0 flex-col gap-1">
              <v-btn
                size="x-small"
                variant="outlined"
                @click="openEditAgentDialog(agent)"
              >
                编辑
              </v-btn>
              <v-btn size="x-small" variant="outlined" @click="openDuplicateAgentDialog(agent)">复制</v-btn>
              <v-btn
                v-if="!agent.builtin"
                size="x-small"
                variant="outlined"
                color="error"
                @click="deleteAgent(agent.id)"
              >
                删除
              </v-btn>
            </div>
          </div>
        </v-card-text>
      </v-card>
      <v-card v-if="displayedAgents.length === 0" flat class="card-shell border-0 md:col-span-2 xl:col-span-3">
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
                {{ formatPermission(template.permissionMode) }} · {{ templateToolCountLabel(template) }} · {{ template.skills.length }} 个技能
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
          <v-alert
            v-if="primaryDefaultAgentForm()"
            type="info"
            variant="tonal"
            density="compact"
          >
            系统默认智能体仅允许修改模型服务、覆盖模型和默认思考等级。
          </v-alert>
          <div class="grid gap-3 md:grid-cols-2">
            <v-text-field
              v-if="!primaryDefaultAgentForm()"
              v-model="agentForm.name"
              label="名称"
              density="comfortable"
            />
            <v-select v-model="agentForm.providerId" :items="providerOptions" label="模型服务" density="comfortable"
              clearable />
            <v-text-field v-model="agentForm.model" label="覆盖模型（可选）" density="comfortable" />
            <v-select
              v-model="agentForm.reasoningEffort"
              :items="reasoningEffortOptions"
              label="默认思考等级"
              density="comfortable"
            />
            <v-select
              v-if="!primaryDefaultAgentForm()"
              v-model="agentForm.permissionMode"
              :items="permissionModes"
              label="默认审批等级"
              density="comfortable"
            />
            <v-select
              v-if="!primaryDefaultAgentForm()"
              v-model="agentForm.toolAccessMode"
              :items="toolAccessModeOptions"
              label="工具访问范围"
              density="comfortable"
              hint="空的选择列表不再代表全部工具"
              persistent-hint
            />
            <v-radio-group
              v-if="!primaryDefaultAgentForm()"
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
              v-if="!primaryDefaultAgentForm()"
              v-model.number="agentForm.recentUserWindow"
              label="保留最近用户消息条数"
              type="number"
              density="comfortable"
              min="2"
              max="100"
            />
            <v-text-field
              v-if="!primaryDefaultAgentForm() && agentForm.workMode === 'loop'"
              v-model.number="agentForm.loopMaxIterations"
              label="目标循环最大轮次"
              type="number"
              density="comfortable"
              min="1"
              max="20"
            />
          </div>

          <div v-if="!primaryDefaultAgentForm()" class="adk-tool-transfer rounded-lg border p-3">
            <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
              <div>
                <div class="adk-tool-transfer__title text-sm font-semibold">运行时工具穿梭框</div>
                <div class="adk-tool-transfer__hint text-xs">
                  左侧为已接入运行时工具，右侧为当前智能体启用工具。
                </div>
              </div>
              <v-chip size="small" variant="tonal">
                {{ allToolsEnabled ? `全部工具 ${tools.length}` : noToolsEnabled ? "无工具" : `已启用 ${agentForm.tools.length}/${tools.length}` }}
              </v-chip>
            </div>
            <div class="grid gap-3 md:grid-cols-2">
              <v-select :model-value="toolCategoryFilter" label="按工具类别过滤" density="comfortable" clearable
                :items="toolCategoryOptions" @update:model-value="emit('update:toolCategoryFilter', $event ?? '')" />
              <v-select :model-value="toolRiskFilter" label="按风险等级过滤" density="comfortable" clearable
                :items="toolRiskOptions" @update:model-value="emit('update:toolRiskFilter', $event ?? '')" />
            </div>

            <div v-if="allToolsEnabled || noToolsEnabled" class="rounded border bg-surface p-3 text-sm text-medium-emphasis">
              {{ allToolsEnabled ? "当前智能体可使用全部运行时工具；如需收窄范围，请切换为“按选择启用”。" : "当前智能体不声明任何运行时工具；可切换为“按选择启用”后添加工具。" }}
            </div>
            <div v-if="!allToolsEnabled && !noToolsEnabled" class="adk-tool-transfer__grid">
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
                  :disabled="tools.length === 0"
                  @click="enableAllTools"
                >
                  启用全部
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
                    {{ agentForm.toolAccessMode === undefined ? "空列表表示该智能体可使用全部运行时工具。" : "尚未选择运行时工具。" }}
                  </div>
                </div>
              </div>
            </div>
          </div>
          <v-select v-if="!primaryDefaultAgentForm()" v-model="agentForm.skills" :items="skillOptions" label="启用技能" density="comfortable" multiple chips
            closable-chips />
          <v-textarea v-if="!primaryDefaultAgentForm()" v-model="agentForm.instruction" label="系统指令" :rows="5" density="comfortable" />
          <div v-if="!primaryDefaultAgentForm()" class="flex gap-6">
            <v-switch v-model="agentForm.memoryEnabled" label="记忆" color="primary" hide-details />
            <v-switch
              v-model="agentForm.status"
              true-value="ENABLED"
              false-value="DISABLED"
              label="启用"
              color="primary"
              hide-details
            />
          </div>
        </v-card-text>
        <v-card-actions class="justify-end gap-2">
          <v-btn variant="text" @click="agentDialogOpen = false">取消</v-btn>
          <v-btn color="primary" @click="submitAgentForm">保存智能体</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <ActionConfirmationHost :controller="actionConfirmation" />
  </section>
</template>

<style scoped src="./ADKAgentsPanel.css"></style>
