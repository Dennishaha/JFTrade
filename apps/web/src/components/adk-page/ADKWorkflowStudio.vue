<script setup lang="ts">
import type { Connection, NodeMouseEvent } from "@vue-flow/core";
import type { SplitpanesResizedPayload } from "splitpanes";
import { computed, onMounted, reactive, ref, watch } from "vue";
import "@/styles/adk-workflow-studio.css";

import type {
  ADKAgent,
  ADKProvider,
  ADKWorkflowDefinition,
  ADKWorkflowTrigger,
  ADKWorkflowTriggerLog,
  ADKWorkflowTriggerType,
} from "@/contracts";
import {
  canvasGraphToWorkflowPayload,
  defaultWorkflowCanvasGraph,
} from "@/features/adkWorkflowCanvasGraph";
import {
  createTriggerForm,
  createWorkflowForm,
  createWorkflowInputRow,
  defaultWorkflowPromptTemplate,
  triggerFormToPayload,
  triggerToForm,
  workflowRunLink,
  workflowToForm,
  type TriggerFormModel,
  type WorkflowInputRow,
  type WorkflowFormModel,
} from "@/features/adkWorkflowForms";
import {
  cloneWorkflowStudioPaneSizes,
  cloneInputRows,
  inputRowsToInputs,
  normalizeWorkflowStudioPanePair,
  statusLabel,
  triggerTypeLabel,
  createWorkflowTemplateForm,
  createWorkflowTemplateTrigger,
  workflowFormToDefinition,
  workflowInvocationMessage,
  workflowStatusOptions,
  workflowStudioOuterPaneMinSizes,
  workflowStudioWorkbenchPaneMinSizes,
  workflowTemplates,
  workflowTone,
  workModeLabel,
  type FlowNodeData,
  type InspectorNodeKind,
  type WorkflowStudioPanePair,
  type WorkflowStudioPaneSizes,
  type WorkflowTemplate,
} from "@/features/adkWorkflowStudio";
import ADKWorkflowCanvas from "./ADKWorkflowCanvas.vue";
import ADKWorkflowDebugPanel from "./ADKWorkflowDebugPanel.vue";
import ADKWorkflowNoticeStack from "./ADKWorkflowNoticeStack.vue";
import ADKWorkflowSecretDialog from "./ADKWorkflowSecretDialog.vue";
import ADKWorkflowStudioInspector from "./ADKWorkflowStudioInspector.vue";
import ADKWorkflowStudioSidebar from "./ADKWorkflowStudioSidebar.vue";
import ADKWorkflowStudioTopbar from "./ADKWorkflowStudioTopbar.vue";
import SplitPane from "../shared/SplitPane.vue";
import SplitPaneItem from "../shared/SplitPaneItem.vue";
import {
  deleteADKWorkflow,
  deleteADKWorkflowTrigger,
  pageSummary,
  runADKWorkflow,
  runADKWorkflowTrigger,
  saveADKWorkflow,
  saveADKWorkflowTrigger,
} from "../../composables/adkWorkflowsApi";
import { useADKWorkflowStudioCanvas } from "../../composables/useADKWorkflowStudioCanvas";
import { useADKWorkflowStudioResources } from "../../composables/useADKWorkflowStudioResources";
import { useADKWorkflowStudioViewModel } from "../../composables/useADKWorkflowStudioViewModel";

const props = withDefaults(
  defineProps<{
    agents: ADKAgent[];
    providers: ADKProvider[];
    formatDateTime: (value: string) => string;
    viewMode?: "workflows" | "workflow-logs";
  }>(),
  {
    viewMode: "workflows",
  },
);

const formatDateTime = (value: string): string => props.formatDateTime(value);

const saving = ref(false);
const selectedNodeId = ref("start");
const workflowSearch = ref("");
const logKeywordFilter = ref("");
const logFromFilter = ref("");
const logToFilter = ref("");
const debugPanelOpen = ref(false);
const inspectorHidden = ref(false);
const debugInputRows = ref<WorkflowInputRow[]>([]);
const showTemplatePicker = ref(false);
const webhookSecret = ref("");
const secretDialogOpen = ref(false);
const successMessage = ref("");
const errorMessage = ref("");
const lastRunHref = ref("");
const runningWorkflow = ref(false);
const runningTrigger = ref(false);
const draftTriggerPending = ref(false);
const draftTriggerNodeId = ref("");
const studioPaneSizes = ref<WorkflowStudioPaneSizes>(cloneWorkflowStudioPaneSizes());

const workflowForm = reactive<WorkflowFormModel>(
  createWorkflowForm("", defaultWorkflowPromptTemplate()),
);
const triggerForm = reactive<TriggerFormModel>(createTriggerForm("schedule"));

const {
  loading,
  triggerLoading,
  logLoading,
  workflows,
  triggers,
  logs,
  selectedWorkflowId,
  selectedLogId,
  workflowStatusFilter,
  logStatusFilter,
  logTriggerFilter,
  workflowPage,
  logPage,
  refreshWorkflows,
  refreshTriggers,
  refreshLogs,
  upsertWorkflow,
  removeWorkflow: removeWorkflowLocal,
  removeTrigger: removeTriggerLocal,
  previousWorkflowPage,
  nextWorkflowPage,
  previousLogPage,
  nextLogPage,
} = useADKWorkflowStudioResources({
  onError: (message) => {
    errorMessage.value = message;
  },
  onEmptyWorkflows: () => startDraftWorkflow("blank"),
  getSelectedNodeId: () => selectedNodeId.value,
  onSelectedTrigger: (trigger) => assignTriggerForm(triggerToForm(trigger)),
  onRefreshNodeData: () => refreshNodeData(),
});

const {
  defaultAgentId,
  visibleWorkflows,
  selectedWorkflowTriggers,
  selectedTrigger,
  logTriggerOptions,
  inspectorKind: fallbackInspectorKind,
  agentOptions,
  providerOptions,
  inputVariableOptions,
  preservedInputCount,
  preservedConfigCount,
  visibleLogs,
  selectedLog,
  workflowStats,
  selectedNodeRun,
  triggerRunSummary,
  schedulePreviewRuns,
  webhookEndpoint,
  webhookCurlSample,
  latestMarketEvent,
} = useADKWorkflowStudioViewModel({
  agents: () => props.agents,
  providers: () => props.providers,
  workflows,
  triggers,
  logs,
  selectedWorkflowId,
  selectedNodeId,
  selectedLogId,
  workflowSearch,
  logStatusFilter,
  logTriggerFilter,
  logKeywordFilter,
  logFromFilter,
  logToFilter,
  workflowForm,
  triggerForm,
  formatDateTime,
  origin: () => (typeof window === "undefined" ? "" : window.location.origin),
});
const {
  flowNodes,
  flowEdges,
  graphFromFlow,
  setFlowGraph,
  loadWorkflowGraph,
  refreshNodeData,
  addTriggerNode: addTriggerFlowNode,
  addAgentNode: addAgentFlowNode,
  removeNode: removeFlowNode,
  connect: connectFlowNode,
} = useADKWorkflowStudioCanvas({
  workflowTriggers: () => selectedWorkflowTriggers.value,
  nodeDataContext: () => ({
    workflowName: workflowForm.name,
    workflowStatus: workflowForm.status,
    workflowWorkMode: workflowForm.workMode,
    workflowInputCount: workflowForm.inputRows.length,
    workflowAgentId: workflowForm.agentId,
    agentName: agentName(workflowForm.agentId),
    agentNameForId: agentName,
    logsCount: logs.value.length,
    logStatusFilter: logStatusFilter.value,
    selectedLog: selectedLog.value,
    triggers: selectedWorkflowTriggers.value,
    draftTriggerNodeId: draftTriggerNodeId.value,
    draftTriggerTitle: triggerForm.title,
    draftTriggerType: triggerForm.type,
    draftTriggerStatus: triggerForm.status,
  }),
});
const templates = workflowTemplates;
const selectedFlowNode = computed(() =>
  flowNodes.value.find((node) => node.id === selectedNodeId.value) ?? null,
);
const inspectorKind = computed<InspectorNodeKind>(() => {
  if (selectedFlowNode.value?.type === "agent") return "agent";
  return fallbackInspectorKind.value;
});
const selectedAgentNodeData = computed<FlowNodeData>(() => {
  const node = selectedFlowNode.value?.type === "agent" ? selectedFlowNode.value : null;
  return { ...(node?.data ?? {}) };
});

watch(
  () => props.viewMode,
  (mode) => {
    if (mode === "workflow-logs") {
      selectedNodeId.value = "monitor";
    }
  },
  { immediate: true },
);

watch(workflowStatusFilter, () => {
  workflowPage.value.offset = 0;
  void refreshWorkflows();
});

watch(logStatusFilter, () => {
  logPage.value.offset = 0;
  void refreshLogs();
});

watch(logTriggerFilter, () => {
  logPage.value.offset = 0;
  void refreshLogs();
});

watch(
  () => workflowForm.inputRows.map((row) => `${row.key}:${row.type}:${row.value}:${row.booleanValue}`).join("|"),
  () => {
    if (!debugPanelOpen.value) {
      debugInputRows.value = cloneInputRows(workflowForm.inputRows);
    }
  },
);

watch(
  selectedWorkflowId,
  async (id) => {
    if (id === "") return;
    const workflow = workflows.value.find((item) => item.id === id);
    if (!workflow) return;
    const nextForm = workflowToForm(workflow);
    assignWorkflowForm(nextForm);
    debugInputRows.value = cloneInputRows(nextForm.inputRows);
    draftTriggerPending.value = false;
    draftTriggerNodeId.value = "";
    await refreshTriggers(id);
    loadWorkflowGraph(workflow);
    await refreshLogs();
    if (props.viewMode === "workflow-logs") {
      selectedNodeId.value = "monitor";
    } else if (!flowNodes.value.some((node) => node.id === selectedNodeId.value)) {
      selectedNodeId.value = "start";
    }
  },
);

onMounted(async () => {
  await refreshWorkflows();
});

function assignWorkflowForm(next: WorkflowFormModel): void {
  Object.assign(workflowForm, next);
}

function assignTriggerForm(next: TriggerFormModel): void {
  Object.assign(triggerForm, next);
}

function startDraftWorkflow(template: WorkflowTemplate): void {
  const form = createWorkflowTemplateForm(template, defaultAgentId.value);
  selectedWorkflowId.value = "";
  assignWorkflowForm(form);
  debugInputRows.value = cloneInputRows(form.inputRows);
  const pseudoWorkflow = workflowFormToDefinition(form, form.inputRows);
  const trigger = createWorkflowTemplateTrigger(template);
  draftTriggerPending.value = trigger != null;
  draftTriggerNodeId.value = trigger ? "trigger:draft" : "";
  if (trigger) assignTriggerForm(trigger);
  setFlowGraph(defaultWorkflowCanvasGraph(pseudoWorkflow, trigger ? [draftTriggerFromForm(trigger)] : []));
  selectedNodeId.value = props.viewMode === "workflow-logs"
    ? "monitor"
    : trigger
      ? "trigger:draft"
      : "start";
  refreshNodeData();
  showTemplatePicker.value = false;
}

function draftTriggerFromForm(form: TriggerFormModel): ADKWorkflowTrigger {
  return {
    id: "draft",
    workflowId: "",
    type: form.type,
    title: form.title,
    status: form.status,
    config: {},
    createdAt: "",
    updatedAt: "",
  };
}

async function saveStudio(): Promise<void> {
  saving.value = true;
  errorMessage.value = "";
  successMessage.value = "";
  try {
    refreshNodeData();
    const graph = graphFromFlow();
    const savedWorkflow = await saveADKWorkflow(
      canvasGraphToWorkflowPayload(workflowForm, graph),
    );
    upsertWorkflow(savedWorkflow);
    selectedWorkflowId.value = savedWorkflow.id;
    assignWorkflowForm(workflowToForm(savedWorkflow));

    if (inspectorKind.value === "trigger" && (draftTriggerPending.value || triggerForm.id)) {
      await saveCurrentTrigger(savedWorkflow.id);
    }
    successMessage.value = "工作流已保存";
    await refreshTriggers(savedWorkflow.id);
    loadWorkflowGraph(savedWorkflow);
    await refreshLogs();
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : "保存工作流失败";
  } finally {
    saving.value = false;
  }
}

async function saveCurrentTrigger(workflowId = selectedWorkflowId.value): Promise<void> {
  if (workflowId === "") {
    errorMessage.value = "请先保存工作流";
    return;
  }
  const result = await saveADKWorkflowTrigger(workflowId, triggerFormToPayload(triggerForm));
  if (result.secret) {
    webhookSecret.value = result.secret;
    secretDialogOpen.value = true;
  }
  draftTriggerPending.value = false;
  draftTriggerNodeId.value = "";
  assignTriggerForm(triggerToForm(result.trigger));
}

async function duplicateWorkflow(asTemplate = false): Promise<void> {
  if (workflowForm.id.trim() === "") {
    errorMessage.value = "请先保存工作流";
    return;
  }
  saving.value = true;
  errorMessage.value = "";
  successMessage.value = "";
  try {
    const payload = canvasGraphToWorkflowPayload(workflowForm, graphFromFlow());
    delete payload.id;
    payload.status = "DISABLED";
    payload.name = asTemplate ? `${workflowForm.name || "工作流"} 模板` : `${workflowForm.name || "工作流"} 副本`;
    payload.tags = Array.from(new Set([...(payload.tags ?? []), asTemplate ? "template" : "copy"]));
    const saved = await saveADKWorkflow(payload);
    upsertWorkflow(saved);
    selectedWorkflowId.value = saved.id;
    assignWorkflowForm(workflowToForm(saved));
    successMessage.value = asTemplate ? "已保存为模板副本" : "工作流已复制";
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "复制工作流失败";
  } finally {
    saving.value = false;
  }
}

function openDebugPanel(): void {
  debugInputRows.value = cloneInputRows(workflowForm.inputRows);
  debugPanelOpen.value = !debugPanelOpen.value;
}

async function runDebugWorkflow(): Promise<void> {
  await runWorkflowNow(inputRowsToInputs(debugInputRows.value));
}

async function runWorkflowNow(inputs: Record<string, unknown> = {}): Promise<void> {
  if (workflowForm.id.trim() === "") {
    errorMessage.value = "请先保存工作流后运行";
    successMessage.value = "";
    lastRunHref.value = "";
    return;
  }
  if (workflowForm.status !== "ENABLED") {
    errorMessage.value = "请先启用工作流后运行";
    successMessage.value = "";
    lastRunHref.value = "";
    return;
  }
  if (runningWorkflow.value) {
    return;
  }
  runningWorkflow.value = true;
  errorMessage.value = "";
  successMessage.value = "";
  lastRunHref.value = "";
  try {
    const result = await runADKWorkflow(workflowForm.id, inputs);
    lastRunHref.value = workflowRunLink(result);
    successMessage.value = workflowInvocationMessage("工作流", result);
    selectedLogId.value = result.log.id;
    selectedNodeId.value = "monitor";
    logPage.value.offset = 0;
    await Promise.all([refreshLogs(), refreshTriggers()]);
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : "运行工作流失败";
  } finally {
    runningWorkflow.value = false;
  }
}

async function runSelectedTrigger(): Promise<void> {
  if (triggerForm.id.trim() === "") {
    errorMessage.value = "请先保存触发器";
    successMessage.value = "";
    lastRunHref.value = "";
    return;
  }
  if (triggerForm.status !== "ENABLED") {
    errorMessage.value = "请先启用触发器后运行";
    successMessage.value = "";
    lastRunHref.value = "";
    return;
  }
  if (runningTrigger.value) {
    return;
  }
  runningTrigger.value = true;
  errorMessage.value = "";
  successMessage.value = "";
  lastRunHref.value = "";
  try {
    const result = await runADKWorkflowTrigger(triggerForm.id);
    lastRunHref.value = workflowRunLink(result);
    successMessage.value = workflowInvocationMessage("触发器", result);
    selectedLogId.value = result.log.id;
    selectedNodeId.value = "monitor";
    logPage.value.offset = 0;
    await Promise.all([refreshLogs(), refreshTriggers()]);
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : "运行触发器失败";
  } finally {
    runningTrigger.value = false;
  }
}

async function removeSelectedWorkflow(): Promise<void> {
  if (workflowForm.id.trim() === "") return;
  if (!window.confirm(`删除工作流「${workflowForm.name}」？`)) return;
  try {
    await deleteADKWorkflow(workflowForm.id);
    removeWorkflowLocal(workflowForm.id);
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : "删除工作流失败";
  }
}

async function removeSelectedTrigger(): Promise<void> {
  if (triggerForm.id.trim() === "") {
    removeDraftTriggerNode();
    return;
  }
  if (!window.confirm(`删除触发器「${triggerForm.title || triggerForm.id}」？`)) return;
  try {
    await deleteADKWorkflowTrigger(selectedWorkflowId.value, triggerForm.id);
    removeTriggerLocal(triggerForm.id);
    removeFlowNode(selectedNodeId.value);
    selectedNodeId.value = "start";
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : "删除触发器失败";
  }
}

function addTriggerNode(type: ADKWorkflowTriggerType = "schedule"): void {
  const id = `trigger:draft-${Date.now()}`;
  draftTriggerPending.value = true;
  draftTriggerNodeId.value = id;
  const form = createTriggerForm(type);
  form.title = triggerTypeLabel(type);
  assignTriggerForm(form);
  addTriggerFlowNode({
    id,
    type,
    title: form.title,
    status: form.status,
  });
  selectedNodeId.value = id;
}

function addAgentNode(): void {
  const id = `agent:${Date.now()}`;
  addAgentFlowNode({
    id,
    title: "智能体",
    agentId: workflowForm.agentId,
  });
  selectedNodeId.value = id;
  refreshNodeData();
}

function updateSelectedAgentNodeData(payload: { key: string; value: unknown }): void {
  const key = payload.key.trim();
  if (key === "") return;
  flowNodes.value = flowNodes.value.map((node) => {
    if (node.id !== selectedNodeId.value || node.type !== "agent") return node;
    const data = { ...(node.data ?? {}) };
    if (typeof payload.value === "string" && payload.value.trim() === "") {
      delete data[key];
    } else {
      data[key] = payload.value;
    }
    return { ...node, data };
  });
  refreshNodeData();
}

function removeDraftTriggerNode(): void {
  const id = selectedNodeId.value;
  removeFlowNode(id);
  draftTriggerPending.value = false;
  draftTriggerNodeId.value = "";
  selectedNodeId.value = "start";
}

function addInputRow(): void {
  workflowForm.inputRows.push(createWorkflowInputRow());
  refreshNodeData();
}

function addDebugInputRow(): void {
  debugInputRows.value.push(createWorkflowInputRow());
}

function removeInputRow(index: number): void {
  workflowForm.inputRows.splice(index, 1);
  refreshNodeData();
}

function removeDebugInputRow(index: number): void {
  debugInputRows.value.splice(index, 1);
}

function insertPromptVariable(value: string): void {
  const prefix = workflowForm.promptTemplate.endsWith("\n") || workflowForm.promptTemplate === "" ? "" : "\n";
  workflowForm.promptTemplate += `${prefix}${value}`;
}

function selectWorkflow(workflow: ADKWorkflowDefinition): void {
  selectedWorkflowId.value = workflow.id;
}

function selectNode(nodeId: string): void {
  selectedNodeId.value = nodeId;
  if (nodeId.startsWith("trigger:")) {
    const triggerId = nodeId.slice("trigger:".length);
    const trigger = selectedWorkflowTriggers.value.find((item) => item.id === triggerId);
    if (trigger) {
      assignTriggerForm(triggerToForm(trigger));
    } else if (!draftTriggerPending.value || draftTriggerNodeId.value !== nodeId) {
      draftTriggerPending.value = true;
      draftTriggerNodeId.value = nodeId;
      assignTriggerForm(createTriggerForm("schedule"));
    }
  }
}

function selectLog(logId: string): void {
  selectedLogId.value = logId;
  refreshNodeData();
}

function openWorkflowLogs(): void {
  selectedNodeId.value = "monitor";
  logPage.value.offset = 0;
  void refreshLogs();
}

function onNodeClick(event: NodeMouseEvent): void {
  selectNode(event.node.id);
}

function onConnect(connection: Connection): void {
  connectFlowNode(connection);
}

function handleStudioOuterPaneResized(payload: SplitpanesResizedPayload): void {
  const sizes = payload.panes?.map((pane) => pane.size);
  const normalized = normalizeWorkflowStudioPanePair(sizes, workflowStudioOuterPaneMinSizes);
  if (!normalized) return;
  const next = {
    ...studioPaneSizes.value,
    outer: normalized,
  };
  studioPaneSizes.value = next;
}

function handleStudioWorkbenchPaneResized(payload: SplitpanesResizedPayload): void {
  const sizes = payload.panes?.map((pane) => pane.size);
  const normalized = normalizeWorkflowStudioPanePair(sizes, workflowStudioWorkbenchPaneMinSizes);
  if (!normalized) return;
  const next = {
    ...studioPaneSizes.value,
    workbench: normalized,
  };
  studioPaneSizes.value = next;
}

function hideInspector(): void {
  inspectorHidden.value = true;
}

function showInspector(): void {
  inspectorHidden.value = false;
}

function runLink(log: ADKWorkflowTriggerLog): string {
  return workflowRunLink({ log });
}

async function copyResultMarkdown(): Promise<void> {
  const text = selectedLog.value?.result?.markdown ?? "";
  if (text === "" || typeof navigator === "undefined" || !navigator.clipboard) return;
  await navigator.clipboard.writeText(text);
  successMessage.value = "结果已复制";
}

function agentName(agentId: string): string {
  return props.agents.find((agent) => agent.id === agentId)?.name ?? agentId;
}

function providerName(providerId: string): string {
  if (!providerId) return "默认模型";
  return props.providers.find((provider) => provider.id === providerId)?.displayName ?? providerId;
}
</script>

<template>
  <section class="adk-workflow-studio">
    <SplitPane class="adk-workflow-studio__split adk-workflow-studio__outer" :pane-min-size="10"
      @resized="handleStudioOuterPaneResized">
      <SplitPaneItem :size="studioPaneSizes.outer[0]" :min-size="workflowStudioOuterPaneMinSizes[0]" :max-size="36">
        <ADKWorkflowStudioSidebar
          v-model:show-template-picker="showTemplatePicker"
          v-model:search="workflowSearch"
          v-model:status-filter="workflowStatusFilter"
          :workflows="visibleWorkflows"
          :selected-workflow-id="selectedWorkflowId"
          :templates="templates"
          :status-options="workflowStatusOptions"
          :loading="loading"
          :page="workflowPage"
          :page-summary="pageSummary(workflowPage)"
          :agent-name="agentName"
          :work-mode-label="workModeLabel"
          :workflow-tone="workflowTone"
          :status-label="statusLabel"
          @start-template="startDraftWorkflow"
          @select-workflow="selectWorkflow"
          @previous="previousWorkflowPage"
          @next="nextWorkflowPage"
        />
      </SplitPaneItem>

      <SplitPaneItem :size="studioPaneSizes.outer[1]" :min-size="workflowStudioOuterPaneMinSizes[1]" :max-size="82">
        <div class="adk-workflow-studio__workarea" :class="{ 'is-inspector-hidden': inspectorHidden }">
          <SplitPane class="adk-workflow-studio__split adk-workflow-studio__workbench" :pane-min-size="10"
            :push-other-panes="false" @resized="handleStudioWorkbenchPaneResized">
            <SplitPaneItem :size="studioPaneSizes.workbench[0]" :min-size="workflowStudioWorkbenchPaneMinSizes[0]"
              :max-size="78">
            <main class="adk-workflow-studio__main">
              <ADKWorkflowStudioTopbar
                :title="workflowForm.name"
                :description="workflowForm.description"
                :status="workflowForm.status"
                :status-tone="workflowTone(workflowForm.status)"
                :status-label="statusLabel(workflowForm.status)"
                :loading="loading"
                :saving="saving"
                :running-workflow="runningWorkflow"
                :log-loading="logLoading"
                :has-workflow="workflowForm.id !== ''"
                @refresh="refreshWorkflows"
                @show-inspector="showInspector"
                @add-trigger="addTriggerNode('schedule')"
                @add-agent="addAgentNode"
                @open-logs="openWorkflowLogs"
                @run="runWorkflowNow()"
                @debug="openDebugPanel"
                @duplicate="duplicateWorkflow(false)"
                @save-template="duplicateWorkflow(true)"
                @remove="removeSelectedWorkflow"
                @save="saveStudio"
              />

              <ADKWorkflowNoticeStack
                :error-message="errorMessage"
                :success-message="successMessage"
                :run-href="lastRunHref"
                @dismiss-error="errorMessage = ''"
                @dismiss-success="successMessage = ''; lastRunHref = ''"
              />

              <ADKWorkflowDebugPanel
                v-if="debugPanelOpen"
                :input-rows="debugInputRows"
                :running="runningWorkflow"
                @add-input="addDebugInputRow"
                @remove-input="removeDebugInputRow"
                @run="runDebugWorkflow"
              />

              <ADKWorkflowCanvas
                v-model:nodes="flowNodes"
                v-model:edges="flowEdges"
                :selected-node-id="selectedNodeId"
                @connect="onConnect"
                @node-click="onNodeClick"
                @select-node="selectNode"
              />
              </main>
            </SplitPaneItem>

            <SplitPaneItem :size="studioPaneSizes.workbench[1]"
              :min-size="workflowStudioWorkbenchPaneMinSizes[1]" :max-size="44">
              <ADKWorkflowStudioInspector
                v-model:log-status-filter="logStatusFilter"
                v-model:log-trigger-filter="logTriggerFilter"
                v-model:log-keyword-filter="logKeywordFilter"
                v-model:log-from-filter="logFromFilter"
                v-model:log-to-filter="logToFilter"
                :inspector-kind="inspectorKind"
                :workflow-form="workflowForm"
                :trigger-form="triggerForm"
                :selected-trigger="selectedTrigger"
                :selected-node-run="selectedNodeRun"
                :selected-agent-node-data="selectedAgentNodeData"
                :selected-log="selectedLog"
                :visible-logs="visibleLogs"
                :selected-node-id="selectedNodeId"
                :workflow-stats="workflowStats"
                :trigger-run-summary="triggerRunSummary"
                :schedule-preview-runs="schedulePreviewRuns"
                :webhook-endpoint="webhookEndpoint"
                :webhook-curl-sample="webhookCurlSample"
                :latest-market-event="latestMarketEvent"
                :log-trigger-options="logTriggerOptions"
                :log-loading="logLoading"
                :trigger-loading="triggerLoading"
                :running-trigger="runningTrigger"
                :saving="saving"
                :log-page="logPage"
                :log-page-summary="pageSummary(logPage)"
                :preserved-input-count="preservedInputCount"
                :preserved-config-count="preservedConfigCount"
                :agent-options="agentOptions"
                :provider-options="providerOptions"
                :input-variable-options="inputVariableOptions"
                :provider-name="providerName"
                :format-date-time="formatDateTime"
                :run-link="runLink"
                @hide-inspector="hideInspector"
                @refresh-node-data="refreshNodeData"
                @add-input-row="addInputRow"
                @remove-input-row="removeInputRow"
                @insert-prompt-variable="insertPromptVariable"
                @update-agent-node-data="updateSelectedAgentNodeData"
                @run-selected-trigger="runSelectedTrigger"
                @remove-selected-trigger="removeSelectedTrigger"
                @refresh-logs="refreshLogs"
                @select-log="selectLog"
                @select-node="selectNode"
                @copy-result-markdown="copyResultMarkdown"
                @previous-log-page="previousLogPage"
                @next-log-page="nextLogPage"
              />
            </SplitPaneItem>
          </SplitPane>
        </div>
      </SplitPaneItem>
    </SplitPane>

    <ADKWorkflowSecretDialog v-model="secretDialogOpen" :secret="webhookSecret" />
  </section>
</template>
