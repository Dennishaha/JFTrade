import { computed, type Ref } from "vue";

import type {
  ADKAgent,
  ADKProvider,
  ADKWorkflowDefinition,
  ADKWorkflowTrigger,
  ADKWorkflowTriggerLog,
} from "@/contracts";
import type {
  TriggerFormModel,
  WorkflowFormModel,
} from "@/features/adkWorkflowForms";
import {
  filterWorkflowLogs,
  previewScheduleRuns,
  triggerTypeLabel,
  workflowNodeRunFor,
  workflowRunStats,
  type InspectorNodeKind,
} from "@/features/adkWorkflowStudio";

export function useADKWorkflowStudioViewModel(options: {
  agents: () => ADKAgent[];
  providers: () => ADKProvider[];
  workflows: Ref<ADKWorkflowDefinition[]>;
  triggers: Ref<ADKWorkflowTrigger[]>;
  logs: Ref<ADKWorkflowTriggerLog[]>;
  selectedWorkflowId: Ref<string>;
  selectedNodeId: Ref<string>;
  selectedLogId: Ref<string>;
  workflowSearch: Ref<string>;
  logStatusFilter: Ref<string>;
  logTriggerFilter: Ref<string>;
  logKeywordFilter: Ref<string>;
  logFromFilter: Ref<string>;
  logToFilter: Ref<string>;
  workflowForm: WorkflowFormModel;
  triggerForm: TriggerFormModel;
  formatDateTime: (value: string) => string;
  origin: () => string;
}) {
  const defaultAgentId = computed(
    () =>
      options.agents().find((agent) => agent.status === "ENABLED")?.id ??
      options.agents()[0]?.id ??
      "",
  );
  const selectedWorkflow = computed(
    () =>
      options.workflows.value.find((workflow) => workflow.id === options.selectedWorkflowId.value) ??
      null,
  );
  const visibleWorkflows = computed(() => {
    const query = options.workflowSearch.value.trim().toLowerCase();
    return options.workflows.value.filter((workflow) => {
      if (
        query !== "" &&
        !`${workflow.name} ${workflow.description ?? ""} ${workflow.tags?.join(" ") ?? ""}`
          .toLowerCase()
          .includes(query)
      ) {
        return false;
      }
      return true;
    });
  });
  const selectedWorkflowTriggers = computed(() =>
    options.triggers.value.filter((trigger) => trigger.workflowId === options.selectedWorkflowId.value),
  );
  const selectedTrigger = computed(() => {
    const triggerId = options.selectedNodeId.value.startsWith("trigger:")
      ? options.selectedNodeId.value.slice("trigger:".length)
      : "";
    return selectedWorkflowTriggers.value.find((trigger) => trigger.id === triggerId) ?? null;
  });
  const logTriggerOptions = computed(() => [
    { title: "全部触发器", value: "" },
    ...selectedWorkflowTriggers.value.map((trigger) => ({
      title: trigger.title || triggerTypeLabel(trigger.type),
      value: trigger.id,
    })),
  ]);
  const inspectorKind = computed<InspectorNodeKind>(() => {
    if (options.selectedNodeId.value === "start") return "start";
    if (options.selectedNodeId.value.startsWith("agent:")) return "agent";
    if (options.selectedNodeId.value.startsWith("trigger:")) return "trigger";
    if (options.selectedNodeId.value === "monitor") return "monitor";
    return "workflow";
  });
  const agentOptions = computed(() =>
    options.agents().map((agent) => ({
      title: `${agent.name} (${agent.id})${agent.status === "ENABLED" ? "" : " · 已停用"}`,
      value: agent.id,
    })),
  );
  const providerOptions = computed(() => [
    { title: "沿用智能体默认模型", value: "" },
    ...options.providers().map((provider) => ({
      title: `${provider.displayName} · ${provider.model}${provider.enabled ? "" : " · 已停用"}`,
      value: provider.id,
    })),
  ]);
  const inputVariableOptions = computed(() => [
    { title: "当前时间", value: "{{ .now }}" },
    { title: "工作流名称", value: "{{ .workflow.name }}" },
    { title: "触发器标题", value: "{{ .trigger.title }}" },
    ...options.workflowForm.inputRows
      .map((row) => row.key.trim())
      .filter(Boolean)
      .map((key) => ({ title: key, value: `{{ .${key} }}` })),
  ]);
  const preservedInputCount = computed(
    () => Object.keys(options.workflowForm.preservedDefaultInputs).length,
  );
  const preservedConfigCount = computed(
    () => Object.keys(options.triggerForm.preservedConfig).length,
  );
  const visibleLogs = computed(() =>
    filterWorkflowLogs(options.logs.value, {
      keyword: options.logKeywordFilter.value,
      from: options.logFromFilter.value,
      to: options.logToFilter.value,
    }),
  );
  const selectedLog = computed(() => {
    if (visibleLogs.value.length === 0) return null;
    return visibleLogs.value.find((log) => log.id === options.selectedLogId.value) ?? visibleLogs.value[0]!;
  });
  const workflowStats = computed(() => workflowRunStats(options.logs.value));
  const selectedNodeRun = computed(() => {
    const log = selectedLog.value;
    if (!log) return null;
    return workflowNodeRunFor(log, options.selectedNodeId.value);
  });
  const triggerRunSummary = computed(() => {
    const triggerId = selectedTrigger.value?.id || options.triggerForm.id;
    if (!triggerId) return null;
    const triggerLogs = options.logs.value.filter((log) => log.triggerId === triggerId);
    return {
      total: triggerLogs.length,
      latest: triggerLogs[0] ?? null,
      failures: triggerLogs.filter((log) => log.status === "FAILED").length,
    };
  });
  const schedulePreviewRuns = computed(() =>
    options.triggerForm.type === "schedule"
      ? previewScheduleRuns(options.triggerForm.schedule, 5, options.formatDateTime)
      : [],
  );
  const webhookEndpoint = computed(() => {
    if (options.triggerForm.id.trim() === "") return "保存触发器后生成网络回调地址";
    const path = `/api/v1/adk/workflow-webhooks/${encodeURIComponent(options.triggerForm.id)}`;
    const origin = options.origin();
    return origin === "" ? path : `${origin}${path}`;
  });
  const webhookCurlSample = computed(() => {
    if (options.triggerForm.type !== "webhook") return "";
    return [
      `curl -X POST "${webhookEndpoint.value}"`,
      `  -H "Content-Type: application/json"`,
      `  -H "X-JFTrade-Workflow-Secret: <secret>"`,
      `  -d '{"inputs":{"source":"external"}}'`,
    ].join(" \\\n");
  });
  const latestMarketEvent = computed(() => {
    const log = options.logs.value.find((item) => item.triggerType === "market_threshold" && item.matchedEvent);
    return log?.matchedEvent ?? null;
  });

  return {
    defaultAgentId,
    selectedWorkflow,
    visibleWorkflows,
    selectedWorkflowTriggers,
    selectedTrigger,
    logTriggerOptions,
    inspectorKind,
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
  };
}
