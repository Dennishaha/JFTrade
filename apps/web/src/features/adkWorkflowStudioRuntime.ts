import type {
  ADKWorkflowDefinition,
  ADKWorkflowNodeRun,
  ADKWorkflowTrigger,
  ADKWorkflowTriggerLog,
  ADKWorkflowTriggerType,
} from "@/contracts";
import type {
  FlowEdgeSnapshot,
  FlowNodeSnapshot,
} from "./adkWorkflowStudioLayout";
import type {
  TriggerFormModel,
  WorkflowInputRow,
  WorkflowFormModel,
} from "./adkWorkflowForms";
import {
  createTriggerForm,
  createWorkflowForm,
  createWorkflowInputRow,
  defaultWorkflowPromptTemplate,
} from "./adkWorkflowForms";
import {
  templateDescription,
  templateName,
  triggerTypeLabel,
  type WorkflowTemplate,
} from "./adkWorkflowStudioOptions";

export type WorkflowLogFilters = {
  keyword?: string;
  from?: string;
  to?: string;
};
export type WorkflowFlowConnection = {
  source?: string | null;
  target?: string | null;
  sourceHandle?: string | null;
  targetHandle?: string | null;
};
export type WorkflowFlowNodeDataContext = {
  workflowName: string;
  workflowStatus: string;
  workflowWorkMode: string;
  workflowInputCount: number;
  agentName: string;
  logsCount: number;
  logStatusFilter: string;
  selectedLog: ADKWorkflowTriggerLog | null;
  triggers: ADKWorkflowTrigger[];
  draftTriggerNodeId: string;
  draftTriggerTitle: string;
  draftTriggerType: ADKWorkflowTriggerType;
  draftTriggerStatus: string;
};

export function cloneInputRows(rows: WorkflowInputRow[]): WorkflowInputRow[] {
  return rows.map((row) => ({ ...row }));
}

export function inputRowsToInputs(rows: WorkflowInputRow[]): Record<string, unknown> {
  const inputs: Record<string, unknown> = {};
  for (const row of rows) {
    const key = row.key.trim();
    if (key === "") continue;
    if (row.type === "boolean") {
      inputs[key] = row.booleanValue;
    } else if (row.type === "number") {
      const value = Number(row.value);
      inputs[key] = Number.isFinite(value) ? value : row.value;
    } else {
      inputs[key] = row.value;
    }
  }
  return inputs;
}

export function workflowNodeRunFor(
  log: ADKWorkflowTriggerLog,
  nodeId: string,
  workflowName = "",
): ADKWorkflowNodeRun | null {
  const runs = projectedNodeRuns(log, workflowName);
  if (nodeId.startsWith("trigger:")) {
    return runs.find((run) => run.nodeId === nodeId || run.nodeType === "trigger") ?? null;
  }
  return runs.find((run) => run.nodeId === nodeId) ?? null;
}

export function refreshWorkflowFlowNodeData(
  nodes: FlowNodeSnapshot[],
  context: WorkflowFlowNodeDataContext,
): FlowNodeSnapshot[] {
  return nodes.map((node) => {
    if (node.id === "start") {
      const run = context.selectedLog ? workflowNodeRunFor(context.selectedLog, "start") : null;
      return {
        ...node,
        data: {
          ...(node.data ?? {}),
          title: "开始",
          subtitle: `${context.workflowInputCount} 个输入项`,
          status: run?.status ?? context.workflowStatus,
          runStatus: run?.status ?? "",
        },
      };
    }
    if (node.id === "agent") {
      const run = context.selectedLog ? workflowNodeRunFor(context.selectedLog, "agent") : null;
      return {
        ...node,
        data: {
          ...(node.data ?? {}),
          title: context.workflowName || "智能体",
          subtitle: context.agentName,
          status: run?.status ?? context.workflowWorkMode,
          runStatus: run?.status ?? "",
        },
      };
    }
    if (node.id === "monitor") {
      const run = context.selectedLog ? workflowNodeRunFor(context.selectedLog, "monitor") : null;
      return {
        ...node,
        data: {
          ...(node.data ?? {}),
          title: "监控",
          subtitle: `${context.logsCount} 条日志`,
          status: run?.status ?? (context.logStatusFilter || "ALL"),
          runStatus: run?.status ?? "",
        },
      };
    }
    if (node.id.startsWith("trigger:")) {
      const triggerId = node.id.slice("trigger:".length);
      const trigger = context.triggers.find((item) => item.id === triggerId);
      const run = context.selectedLog ? workflowNodeRunFor(context.selectedLog, node.id) : null;
      return {
        ...node,
        data: {
          ...(node.data ?? {}),
          title:
            trigger?.title ||
            (node.id === context.draftTriggerNodeId ? context.draftTriggerTitle : "") ||
            "触发器",
          subtitle: trigger
            ? triggerTypeLabel(trigger.type)
            : triggerTypeLabel(context.draftTriggerType),
          status: run?.status ?? trigger?.status ?? context.draftTriggerStatus,
          runStatus: run?.status ?? "",
        },
      };
    }
    return node;
  });
}

export function addDraftTriggerFlowNode(
  nodes: FlowNodeSnapshot[],
  edges: FlowEdgeSnapshot[],
  options: {
    id: string;
    type: ADKWorkflowTriggerType;
    title: string;
    status: string;
  },
): { nodes: FlowNodeSnapshot[]; edges: FlowEdgeSnapshot[] } {
  return {
    nodes: [
      ...nodes,
      {
        id: options.id,
        type: "trigger",
        position: { x: 80, y: 72 + nodes.length * 44 },
        data: {
          title: options.title,
          subtitle: triggerTypeLabel(options.type),
          status: options.status,
        },
      },
    ],
    edges: [
      ...edges,
      {
        id: `${options.id}->start`,
        source: options.id,
        target: "start",
        type: "smoothstep",
        animated: true,
      },
    ],
  };
}

export function removeWorkflowFlowNode(
  nodes: FlowNodeSnapshot[],
  edges: FlowEdgeSnapshot[],
  nodeId: string,
): { nodes: FlowNodeSnapshot[]; edges: FlowEdgeSnapshot[] } {
  return {
    nodes: nodes.filter((node) => node.id !== nodeId),
    edges: edges.filter((edge) => edge.source !== nodeId && edge.target !== nodeId),
  };
}

export function connectWorkflowFlowEdge(
  edges: FlowEdgeSnapshot[],
  connection: WorkflowFlowConnection,
): FlowEdgeSnapshot[] {
  if (!connection.source || !connection.target) return edges;
  const id = `${connection.source}->${connection.target}`;
  if (edges.some((edge) => edge.id === id)) return edges;
  return [
    ...edges,
    {
      id,
      source: connection.source,
      target: connection.target,
      sourceHandle: connection.sourceHandle ?? null,
      targetHandle: connection.targetHandle ?? null,
      type: "smoothstep",
    },
  ];
}

export function projectedNodeRuns(
  log: ADKWorkflowTriggerLog,
  workflowName = "",
): ADKWorkflowNodeRun[] {
  if (Array.isArray(log.nodeRuns) && log.nodeRuns.length > 0) return log.nodeRuns;
  const startedAt = log.startedAt || log.createdAt;
  const finishedAt = log.finishedAt || log.updatedAt;
  const triggerRun: ADKWorkflowNodeRun = {
    nodeId: log.triggerId ? `trigger:${log.triggerId}` : "trigger:manual",
    nodeType: "trigger",
    title: triggerTypeLabel(log.triggerType),
    status: log.status,
    startedAt,
    finishedAt,
  };
  if (log.inputs) triggerRun.inputs = log.inputs;
  if (log.matchedEvent) triggerRun.outputs = log.matchedEvent;
  if (log.error) triggerRun.error = log.error;

  const startRun: ADKWorkflowNodeRun = {
    nodeId: "start",
    nodeType: "start",
    title: "开始",
    status: log.status === "FAILED" && !log.runId ? "FAILED" : "SUCCEEDED",
    startedAt,
    finishedAt,
  };
  if (log.inputs) startRun.inputs = log.inputs;
  if (log.status === "FAILED" && !log.runId && log.error) startRun.error = log.error;

  const agentRun: ADKWorkflowNodeRun = {
    nodeId: "agent",
    nodeType: "agent",
    title: workflowName || "智能体",
    status: log.status,
    startedAt,
    finishedAt,
  };
  if (log.result?.markdown) agentRun.outputs = { reply: log.result.markdown };
  if (log.error) agentRun.error = log.error;

  const monitorRun: ADKWorkflowNodeRun = {
    nodeId: "monitor",
    nodeType: "monitor",
    title: "监控",
    status: log.status,
    startedAt,
    finishedAt,
  };
  if (log.result?.markdown) monitorRun.outputs = { result: log.result.markdown };
  if (log.error) monitorRun.error = log.error;

  return [triggerRun, startRun, agentRun, monitorRun];
}

export function workflowRunStats(logs: ADKWorkflowTriggerLog[]): {
  total: number;
  succeeded: number;
  failed: number;
  recent: number;
  successRate: number;
  avgMs: number;
} {
  const total = logs.length;
  const succeeded = logs.filter((log) => log.status === "SUCCEEDED").length;
  const failed = logs.filter((log) => log.status === "FAILED").length;
  const recentSince = Date.now() - 24 * 60 * 60 * 1000;
  const recent = logs.filter((log) => {
    const at = Date.parse(log.startedAt || log.createdAt || "");
    return Number.isFinite(at) && at >= recentSince;
  }).length;
  const durations = logs
    .map((log) => runDurationMs(log))
    .filter((value): value is number => value != null);
  const avgMs = durations.length > 0
    ? Math.round(durations.reduce((sum, value) => sum + value, 0) / durations.length)
    : 0;
  return {
    total,
    succeeded,
    failed,
    recent,
    successRate: total > 0 ? Math.round((succeeded / total) * 100) : 0,
    avgMs,
  };
}

export function nodeRunClass(status: unknown): string {
  if (typeof status !== "string" || status.trim() === "") return "";
  return `is-run-${status.toLowerCase().replace(/_/g, "-")}`;
}

export function runDurationMs(log: ADKWorkflowTriggerLog): number | null {
  const start = Date.parse(log.startedAt || log.createdAt || "");
  const end = Date.parse(log.finishedAt || log.updatedAt || "");
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return null;
  return end - start;
}

export function runDurationLabel(
  log: ADKWorkflowTriggerLog | ADKWorkflowNodeRun | null,
): string {
  if (!log) return "-";
  const start = Date.parse(log.startedAt || "");
  const end = Date.parse(log.finishedAt || "");
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return "-";
  return formatDurationMs(end - start);
}

export function formatDurationMs(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return "-";
  if (ms < 1000) return `${ms} 毫秒`;
  return `${(ms / 1000).toFixed(1)} 秒`;
}

export function formatJson(value: unknown): string {
  if (value == null || value === "") return "-";
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

export function parseDateFilter(value: string): number | null {
  if (value.trim() === "") return null;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : null;
}

export function previewScheduleRuns(
  schedule: TriggerFormModel["schedule"],
  count: number,
  formatDateTime: (value: string) => string,
): string[] {
  const [hourRaw, minuteRaw] = schedule.time.split(":");
  const hour = Number(hourRaw);
  const minute = Number(minuteRaw);
  if (!Number.isFinite(hour) || !Number.isFinite(minute)) return [];
  const weekdays = schedule.frequency === "daily"
    ? new Set([0, 1, 2, 3, 4, 5, 6])
    : schedule.frequency === "weekly"
      ? new Set(schedule.weekdays.map((item) => Number(item)))
      : new Set([1, 2, 3, 4, 5]);
  if (schedule.frequency === "custom") {
    return [`自定义定时表达式：${schedule.customCron || "-"}`];
  }
  const result: string[] = [];
  const cursor = new Date();
  cursor.setSeconds(0, 0);
  for (let day = 0; result.length < count && day < 28; day += 1) {
    const next = new Date(cursor);
    next.setDate(cursor.getDate() + day);
    next.setHours(hour, minute, 0, 0);
    if (next <= cursor || !weekdays.has(next.getDay())) continue;
    result.push(formatDateTime(next.toISOString()));
  }
  return result;
}

export function nodeRunDetails(run: ADKWorkflowNodeRun): string {
  return formatJson({
    输入: run.inputs,
    输出: run.outputs,
    错误: run.error,
  });
}

export function createWorkflowTemplateForm(
  template: WorkflowTemplate,
  defaultAgentId = "",
): WorkflowFormModel {
  const form = createWorkflowForm(defaultAgentId, defaultWorkflowPromptTemplate());
  form.status = "DISABLED";
  form.name = templateName(template);
  form.description = templateDescription(template);
  form.tagsText = template === "blank" ? "" : "工作流, ADK";
  if (template === "schedule") {
    form.inputRows = [createWorkflowInputRow("symbol", "US.AAPL")];
    form.promptTemplate = [
      "请执行「{{ .workflow.name }}」工作流。",
      "当前时间：{{ .now }}",
      "请围绕 {{ .symbol }} 做开盘前复盘，输出关注事项和风险提醒。",
    ].join("\n");
  } else if (template === "risk") {
    form.inputRows = [
      createWorkflowInputRow("portfolio", "核心持仓"),
      createWorkflowInputRow("market", "US/HK"),
    ];
    form.promptTemplate = [
      "请执行「{{ .workflow.name }}」工作流。",
      "当前时间：{{ .now }}",
      "请扫描 {{ .portfolio }} 在 {{ .market }} 市场中的持仓风险，输出风险等级、触发因素和待办动作。",
    ].join("\n");
  } else if (template === "market") {
    form.inputRows = [createWorkflowInputRow("symbol", "US.AAPL")];
    form.promptTemplate = [
      "行情阈值已触发。",
      "标的：{{ .symbol }}",
      "请评估当前价格变化、风险和下一步观察计划。",
    ].join("\n");
  } else if (template === "webhook") {
    form.inputRows = [createWorkflowInputRow("source", "external")];
    form.promptTemplate = [
      "收到来自 {{ .source }} 的网络回调事件。",
      "请根据事件上下文执行「{{ .workflow.name }}」。",
    ].join("\n");
  }
  return form;
}

export function createWorkflowTemplateTrigger(
  template: WorkflowTemplate,
): TriggerFormModel | null {
  if (template === "blank") return null;
  if (template === "webhook") {
    const form = createTriggerForm("webhook");
    form.title = "网络回调";
    form.status = "DISABLED";
    return form;
  }
  if (template === "market") {
    const form = createTriggerForm("market_threshold");
    form.title = "价格阈值";
    form.status = "DISABLED";
    return form;
  }
  if (template === "risk") {
    const form = createTriggerForm("schedule");
    form.title = "风险扫描";
    form.status = "DISABLED";
    return form;
  }
  const form = createTriggerForm("schedule");
  form.title = "开盘复盘";
  form.status = "DISABLED";
  return form;
}

export function workflowFormToDefinition(
  form: {
    id: string;
    name: string;
    description: string;
    status: string;
    agentId: string;
    workMode: string;
    providerId: string;
    model: string;
    permissionMode: string;
    promptTemplate: string;
    objectiveTemplate: string;
    preservedDefaultInputs: Record<string, unknown>;
    tagsText: string;
  },
  inputRows: WorkflowInputRow[],
): ADKWorkflowDefinition {
  return {
    id: form.id || "draft-workflow",
    name: form.name || "未命名工作流",
    description: form.description,
    status: form.status,
    agentId: form.agentId,
    workMode: form.workMode,
    providerId: form.providerId,
    model: form.model,
    permissionMode: form.permissionMode,
    promptTemplate: form.promptTemplate,
    objectiveTemplate: form.objectiveTemplate,
    defaultInputs: {
      ...form.preservedDefaultInputs,
      ...inputRowsToInputs(inputRows),
    },
    tags: form.tagsText
      .split(",")
      .map((tag) => tag.trim())
      .filter(Boolean),
    createdAt: "",
    updatedAt: "",
  };
}

export function filterWorkflowLogs(
  logs: ADKWorkflowTriggerLog[],
  filters: WorkflowLogFilters = {},
): ADKWorkflowTriggerLog[] {
  const keyword = (filters.keyword ?? "").trim().toLowerCase();
  const from = parseDateFilter(filters.from ?? "");
  const to = parseDateFilter(filters.to ?? "");
  return logs.filter((log) => {
    const at = Date.parse(log.startedAt || log.createdAt || "");
    if (from != null && Number.isFinite(at) && at < from) return false;
    if (to != null && Number.isFinite(at) && at > to + 24 * 60 * 60 * 1000 - 1) {
      return false;
    }
    if (keyword !== "") {
      const haystack = [
        log.status,
        log.triggerType,
        log.runId,
        log.sessionId,
        log.error,
        log.result?.markdown,
        JSON.stringify(log.inputs ?? {}),
        JSON.stringify(log.matchedEvent ?? {}),
      ].join(" ").toLowerCase();
      return haystack.includes(keyword);
    }
    return true;
  });
}

export function workflowInvocationMessage(
  label: string,
  result: {
    log?: { status?: string; runId?: string; error?: string };
    response?: { run?: { id?: string } };
  },
): string {
  const runId = result.response?.run?.id || result.log?.runId || "";
  if (result.log?.status === "SKIPPED") {
    return `${label}本次已跳过${result.log.error ? `：${result.log.error}` : ""}`;
  }
  if (runId) {
    return `${label}已启动：${runId}`;
  }
  if (result.log?.status === "QUEUED") {
    return `${label}已进入队列`;
  }
  return `${label}已提交`;
}
