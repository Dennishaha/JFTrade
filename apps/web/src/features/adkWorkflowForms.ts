import type {
  ADKWorkflowDefinition,
  ADKWorkflowDefinitionWriteRequest,
  ADKWorkflowTrigger,
  ADKWorkflowTriggerType,
  ADKWorkflowTriggerWriteRequest,
} from "@/contracts";

export type WorkflowInputType = "string" | "number" | "boolean";
export type ScheduleFrequency = "daily" | "weekdays" | "weekly" | "custom";

export interface WorkflowInputRow {
  key: string;
  type: WorkflowInputType;
  value: string;
  booleanValue: boolean;
}

export interface WorkflowFormModel {
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
  inputRows: WorkflowInputRow[];
  preservedDefaultInputs: Record<string, unknown>;
  tagsText: string;
}

export interface TriggerFormModel {
  id: string;
  type: ADKWorkflowTriggerType;
  title: string;
  status: string;
  resetSecret: boolean;
  preservedConfig: Record<string, unknown>;
  schedule: {
    frequency: ScheduleFrequency;
    time: string;
    weekdays: string[];
    timezone: string;
    customCron: string;
  };
  event: {
    source: string;
    eventType: string;
    entityId: string;
    category: string;
    level: string;
    cooldownSec: string;
  };
  market: {
    instrumentIdsText: string;
    snapshotPath: string;
    operator: string;
    value: string;
    edge: string;
    cooldownSec: string;
  };
}

export function createWorkflowInputRow(
  key = "",
  value: unknown = "",
): WorkflowInputRow {
  if (typeof value === "number" && Number.isFinite(value)) {
    return { key, type: "number", value: String(value), booleanValue: false };
  }
  if (typeof value === "boolean") {
    return { key, type: "boolean", value: "", booleanValue: value };
  }
  return {
    key,
    type: "string",
    value: value == null ? "" : String(value),
    booleanValue: false,
  };
}

export function createWorkflowForm(
  defaultAgentId = "",
  promptTemplate = defaultWorkflowPromptTemplate(),
): WorkflowFormModel {
  return {
    id: "",
    name: "",
    description: "",
    status: "DISABLED",
    agentId: defaultAgentId,
    workMode: "task",
    providerId: "",
    model: "",
    permissionMode: "approval",
    promptTemplate,
    objectiveTemplate: "",
    inputRows: [],
    preservedDefaultInputs: {},
    tagsText: "",
  };
}

export function workflowToForm(
  workflow: ADKWorkflowDefinition,
): WorkflowFormModel {
  const inputRows: WorkflowInputRow[] = [];
  const preservedDefaultInputs: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(workflow.defaultInputs ?? {})) {
    if (
      typeof value === "string" ||
      typeof value === "number" ||
      typeof value === "boolean" ||
      value == null
    ) {
      inputRows.push(createWorkflowInputRow(key, value));
    } else {
      preservedDefaultInputs[key] = value;
    }
  }
  return {
    id: workflow.id,
    name: workflow.name,
    description: workflow.description ?? "",
    status: workflow.status || "DISABLED",
    agentId: workflow.agentId,
    workMode: workflow.workMode || "task",
    providerId: workflow.providerId ?? "",
    model: workflow.model ?? "",
    permissionMode: workflow.permissionMode ?? "approval",
    promptTemplate: workflow.promptTemplate || defaultWorkflowPromptTemplate(),
    objectiveTemplate: workflow.objectiveTemplate ?? "",
    inputRows,
    preservedDefaultInputs,
    tagsText: (workflow.tags ?? []).join(", "),
  };
}

export function workflowFormToPayload(
  form: WorkflowFormModel,
): ADKWorkflowDefinitionWriteRequest {
  const defaultInputs: Record<string, unknown> = {
    ...form.preservedDefaultInputs,
  };
  for (const row of form.inputRows) {
    const key = row.key.trim();
    if (key === "") continue;
    if (row.type === "number") {
      const parsed = Number(row.value);
      if (Number.isFinite(parsed)) {
        defaultInputs[key] = parsed;
      }
      continue;
    }
    if (row.type === "boolean") {
      defaultInputs[key] = row.booleanValue;
      continue;
    }
    defaultInputs[key] = row.value;
  }
  const payload: ADKWorkflowDefinitionWriteRequest = {
    name: form.name.trim(),
    description: form.description.trim(),
    status: form.status,
    agentId: form.agentId.trim(),
    workMode: form.workMode,
    providerId: form.providerId.trim(),
    model: form.model.trim(),
    permissionMode: form.permissionMode,
    promptTemplate: form.promptTemplate.trim(),
    objectiveTemplate: form.objectiveTemplate.trim(),
    defaultInputs,
    tags: tagsFromText(form.tagsText),
  };
  if (form.id.trim() !== "") {
    payload.id = form.id.trim();
  }
  return payload;
}

export function defaultWorkflowPromptTemplate(): string {
  return [
    "请执行「{{ .workflow.name }}」工作流。",
    "当前时间：{{ .now }}",
    "",
    "输入上下文：",
    "{{ range $key, $value := . }}- {{ $key }}: {{ $value }}",
    "{{ end }}",
  ].join("\n");
}

export function createTriggerForm(
  type: ADKWorkflowTriggerType = "schedule",
): TriggerFormModel {
  return {
    id: "",
    type,
    title: "",
    status: "DISABLED",
    resetSecret: type === "webhook",
    preservedConfig: {},
    schedule: {
      frequency: "weekdays",
      time: "08:00",
      weekdays: ["1", "2", "3", "4", "5"],
      timezone: "Asia/Shanghai",
      customCron: "0 8 * * 1-5",
    },
    event: {
      source: "notification",
      eventType: "system.notification",
      entityId: "",
      category: "",
      level: "",
      cooldownSec: "900",
    },
    market: {
      instrumentIdsText: "US.AAPL",
      snapshotPath: "snapshot.price",
      operator: ">",
      value: "100",
      edge: "cross_up",
      cooldownSec: "900",
    },
  };
}

export function triggerToForm(trigger: ADKWorkflowTrigger): TriggerFormModel {
  const form = createTriggerForm(trigger.type);
  form.id = trigger.id;
  form.type = trigger.type;
  form.title = trigger.title;
  form.status = trigger.status || "DISABLED";
  form.resetSecret = false;
  const config = trigger.config ?? {};
  if (trigger.type === "schedule") {
    const cron = stringValue(config.cron) || form.schedule.customCron;
    const preset = cronExpressionToPreset(cron);
    form.schedule = {
      ...form.schedule,
      ...preset,
      timezone: stringValue(config.timezone) || "Asia/Shanghai",
      customCron: preset.customCron,
    };
    form.preservedConfig = omitKeys(config, ["cron", "timezone"]);
  } else if (trigger.type === "event") {
    form.event = {
      source: stringValue(config.source),
      eventType: stringValue(config.eventType),
      entityId: stringValue(config.entityId),
      category: stringValue(config.category),
      level: stringValue(config.level),
      cooldownSec: stringValue(config.cooldownSec),
    };
    form.preservedConfig = omitKeys(config, [
      "source",
      "eventType",
      "entityId",
      "category",
      "level",
      "cooldownSec",
    ]);
  } else if (trigger.type === "market_threshold") {
    form.market = {
      instrumentIdsText: stringArrayValue(config.instrumentIds).join(", "),
      snapshotPath: stringValue(config.snapshotPath) || "snapshot.price",
      operator: stringValue(config.operator) || ">",
      value: stringValue(config.value),
      edge: stringValue(config.edge) || "cross_up",
      cooldownSec: stringValue(config.cooldownSec) || "900",
    };
    form.preservedConfig = omitKeys(config, [
      "instrumentIds",
      "snapshotPath",
      "operator",
      "value",
      "edge",
      "cooldownSec",
    ]);
  } else {
    form.preservedConfig = { ...config };
  }
  return form;
}

export function triggerFormToPayload(
  form: TriggerFormModel,
): ADKWorkflowTriggerWriteRequest {
  const config: Record<string, unknown> = { ...form.preservedConfig };
  if (form.type === "schedule") {
    config.cron = cronPresetToExpression(form.schedule);
    config.timezone = form.schedule.timezone.trim() || "Asia/Shanghai";
  } else if (form.type === "event") {
    assignNonEmpty(config, "source", form.event.source);
    assignNonEmpty(config, "eventType", form.event.eventType);
    assignNonEmpty(config, "entityId", form.event.entityId);
    assignNonEmpty(config, "category", form.event.category);
    assignNonEmpty(config, "level", form.event.level);
    assignNumber(config, "cooldownSec", form.event.cooldownSec);
  } else if (form.type === "market_threshold") {
    config.instrumentIds = splitTextList(form.market.instrumentIdsText);
    config.snapshotPath = form.market.snapshotPath.trim() || "snapshot.price";
    config.operator = form.market.operator || ">";
    assignNumber(config, "value", form.market.value);
    config.edge = form.market.edge || "cross_up";
    assignNumber(config, "cooldownSec", form.market.cooldownSec);
  }
  const payload: ADKWorkflowTriggerWriteRequest = {
    type: form.type,
    title: form.title.trim(),
    status: form.status,
    config,
    resetSecret: form.resetSecret,
  };
  if (form.id.trim() !== "") {
    payload.id = form.id.trim();
  }
  return payload;
}

export function cronPresetToExpression(input: {
  frequency: ScheduleFrequency;
  time: string;
  weekdays: string[];
  customCron: string;
}): string {
  if (input.frequency === "custom") {
    return input.customCron.trim();
  }
  const [hour, minute] = parseTime(input.time);
  if (input.frequency === "daily") {
    return `${minute} ${hour} * * *`;
  }
  if (input.frequency === "weekly") {
    const weekdays = input.weekdays.length > 0 ? input.weekdays.join(",") : "1";
    return `${minute} ${hour} * * ${weekdays}`;
  }
  return `${minute} ${hour} * * 1-5`;
}

export function cronExpressionToPreset(cron: string): {
  frequency: ScheduleFrequency;
  time: string;
  weekdays: string[];
  customCron: string;
} {
  const fields = cron.trim().split(/\s+/);
  if (fields.length !== 5) {
    return {
      frequency: "custom",
      time: "08:00",
      weekdays: ["1", "2", "3", "4", "5"],
      customCron: cron,
    };
  }
  const [minute, hour, dayOfMonth, month, dayOfWeek] = fields as [
    string,
    string,
    string,
    string,
    string,
  ];
  const time = `${hour.padStart(2, "0")}:${minute.padStart(2, "0")}`;
  if (dayOfMonth === "*" && month === "*" && dayOfWeek === "*") {
    return { frequency: "daily", time, weekdays: [], customCron: cron };
  }
  if (dayOfMonth === "*" && month === "*" && dayOfWeek === "1-5") {
    return {
      frequency: "weekdays",
      time,
      weekdays: ["1", "2", "3", "4", "5"],
      customCron: cron,
    };
  }
  if (
    dayOfMonth === "*" &&
    month === "*" &&
    /^[0-6](,[0-6])*$/.test(dayOfWeek)
  ) {
    return {
      frequency: "weekly",
      time,
      weekdays: dayOfWeek.split(","),
      customCron: cron,
    };
  }
  return {
    frequency: "custom",
    time,
    weekdays: ["1", "2", "3", "4", "5"],
    customCron: cron,
  };
}

export function workflowRunLink(result: {
  log?: { sessionId?: string; runId?: string };
  response?: { session?: { id?: string }; run?: { id?: string } };
}): string {
  const sessionId = result.response?.session?.id || result.log?.sessionId || "";
  const runId = result.response?.run?.id || result.log?.runId || "";
  const params = new URLSearchParams();
  if (sessionId) params.set("sessionId", sessionId);
  if (runId) params.set("runId", runId);
  const query = params.toString();
  return `/adk/agents${query ? `?${query}` : ""}`;
}

function tagsFromText(raw: string): string[] {
  return raw
    .split(",")
    .map((tag) => tag.trim())
    .filter(Boolean);
}

function omitKeys(
  input: Record<string, unknown>,
  keys: string[],
): Record<string, unknown> {
  const omit = new Set(keys);
  return Object.fromEntries(
    Object.entries(input).filter(([key]) => !omit.has(key)),
  );
}

function stringValue(value: unknown): string {
  if (value == null) return "";
  return String(value);
}

function stringArrayValue(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.map((item) => String(item)).filter(Boolean);
}

function splitTextList(raw: string): string[] {
  return raw
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function assignNonEmpty(
  target: Record<string, unknown>,
  key: string,
  value: string,
): void {
  const trimmed = value.trim();
  if (trimmed !== "") {
    target[key] = trimmed;
  } else {
    delete target[key];
  }
}

function assignNumber(
  target: Record<string, unknown>,
  key: string,
  value: string,
): void {
  const parsed = Number(value);
  if (Number.isFinite(parsed)) {
    target[key] = parsed;
  } else {
    delete target[key];
  }
}

function parseTime(value: string): [string, string] {
  const match = /^(\d{1,2}):(\d{1,2})$/.exec(value.trim());
  if (!match) return ["8", "0"];
  const hour = Math.min(23, Math.max(0, Number(match[1])));
  const minute = Math.min(59, Math.max(0, Number(match[2])));
  return [String(hour), String(minute)];
}
