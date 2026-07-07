import type { ADKWorkflowTriggerType } from "@/contracts";

export type WorkflowTemplate = "blank" | "schedule" | "risk" | "webhook" | "market";
export type InspectorNodeKind = "start" | "agent" | "trigger" | "monitor" | "workflow";

export const workflowStatusOptions = [
  { title: "全部工作流", value: "" },
  { title: "已启用", value: "ENABLED" },
  { title: "已停用", value: "DISABLED" },
];
export const workflowEditStatusOptions = [
  { title: "启用", value: "ENABLED" },
  { title: "停用", value: "DISABLED" },
];
export const triggerStatusOptions = [
  { title: "启用", value: "ENABLED" },
  { title: "停用", value: "DISABLED" },
];
export const workModeOptions = [
  { title: "对话", value: "chat" },
  { title: "目标", value: "loop" },
];
export const permissionOptions = [
  { title: "请求批准", value: "approval" },
  { title: "减少审批", value: "less_approval" },
  { title: "全部允许", value: "all" },
];
export const triggerTypeOptions: Array<{ title: string; value: ADKWorkflowTriggerType }> = [
  { title: "定时", value: "schedule" },
  { title: "网络回调", value: "webhook" },
  { title: "事件", value: "event" },
  { title: "行情阈值", value: "market_threshold" },
];
export const inputTypeOptions = [
  { title: "文本", value: "string" },
  { title: "数字", value: "number" },
  { title: "开关", value: "boolean" },
];
export const scheduleFrequencyOptions = [
  { title: "每天", value: "daily" },
  { title: "周一到五", value: "weekdays" },
  { title: "每周", value: "weekly" },
  { title: "自定义定时表达式", value: "custom" },
];
export const weekdayOptions = [
  { title: "周日", value: "0" },
  { title: "周一", value: "1" },
  { title: "周二", value: "2" },
  { title: "周三", value: "3" },
  { title: "周四", value: "4" },
  { title: "周五", value: "5" },
  { title: "周六", value: "6" },
];
export const marketEdgeOptions = [
  { title: "向上穿越", value: "cross_up" },
  { title: "向下穿越", value: "cross_down" },
  { title: "持续高于", value: "above" },
  { title: "持续低于", value: "below" },
];
export const marketOperatorOptions = [">", ">=", "<", "<="];
export const logStatusOptions = [
  { title: "全部状态", value: "" },
  { title: "排队", value: "QUEUED" },
  { title: "运行中", value: "RUNNING" },
  { title: "等待审批", value: "PENDING_APPROVAL" },
  { title: "成功", value: "SUCCEEDED" },
  { title: "失败", value: "FAILED" },
  { title: "跳过", value: "SKIPPED" },
];
export const workflowTemplates: Array<{
  value: WorkflowTemplate;
  title: string;
  description: string;
  icon: string;
}> = [
  {
    value: "blank",
    title: "空白工作流",
    description: "开始、智能体和监控基础链路",
    icon: "fa-solid fa-diagram-project",
  },
  {
    value: "schedule",
    title: "定时复盘",
    description: "周一到五开盘前自动生成观察计划",
    icon: "fa-solid fa-clock",
  },
  {
    value: "risk",
    title: "持仓风险扫描",
    description: "定时检查持仓、风险事件和待办动作",
    icon: "fa-solid fa-shield-halved",
  },
  {
    value: "webhook",
    title: "网络回调入口",
    description: "外部事件推送后启动智能体",
    icon: "fa-solid fa-link",
  },
  {
    value: "market",
    title: "行情阈值",
    description: "价格穿越阈值后触发分析",
    icon: "fa-solid fa-chart-line",
  },
];

export function workflowTone(status: string): string {
  return status === "ENABLED" ? "is-success" : "is-muted";
}

export function logTone(status: string): string {
  switch (status) {
    case "SUCCEEDED":
      return "is-success";
    case "RUNNING":
    case "QUEUED":
      return "is-info";
    case "PENDING_APPROVAL":
      return "is-warning";
    case "FAILED":
      return "is-error";
    default:
      return "is-muted";
  }
}

export function statusLabel(status: string): string {
  switch (status) {
    case "ENABLED":
      return "已启用";
    case "DISABLED":
      return "已停用";
    case "QUEUED":
      return "排队中";
    case "RUNNING":
      return "运行中";
    case "PENDING_APPROVAL":
      return "等待审批";
    case "SUCCEEDED":
      return "成功";
    case "FAILED":
      return "失败";
    case "SKIPPED":
      return "已跳过";
    case "CANCELLED":
      return "已取消";
    case "ALL":
      return "全部";
    case "chat":
      return "对话";
    case "loop":
      return "目标";
    default:
      return status || "未知";
  }
}

export function workModeLabel(mode: string): string {
  return workModeOptions.find((item) => item.value === mode)?.title ?? mode;
}

export function nodeTypeLabel(type: string): string {
  switch (type) {
    case "start":
      return "开始";
    case "agent":
      return "智能体";
    case "trigger":
      return "触发器";
    case "monitor":
      return "监控";
    case "workflow":
      return "工作流";
    default:
      return type;
  }
}

export function inspectorTitle(kind: InspectorNodeKind): string {
  return nodeTypeLabel(kind);
}

export function triggerTypeLabel(type: string): string {
  switch (type) {
    case "schedule":
      return "定时";
    case "webhook":
      return "网络回调";
    case "event":
      return "事件";
    case "market_threshold":
      return "行情阈值";
    default:
      return "手动";
  }
}

export function templateName(template: WorkflowTemplate): string {
  switch (template) {
    case "schedule":
      return "定时市场复盘";
    case "risk":
      return "持仓风险扫描";
    case "webhook":
      return "网络回调工作流";
    case "market":
      return "行情阈值提醒";
    default:
      return "新的工作流";
  }
}

export function templateDescription(template: WorkflowTemplate): string {
  switch (template) {
    case "schedule":
      return "在指定时间自动启动智能体复盘。";
    case "risk":
      return "定时检查持仓风险、事件影响与下一步待办。";
    case "webhook":
      return "由外部系统通过网络回调启动。";
    case "market":
      return "行情达到阈值后启动智能体分析。";
    default:
      return "从开始节点输入并进入智能体执行。";
  }
}
