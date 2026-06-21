import type { ADKPermissionMode, ADKSkill } from "@/contracts";

import { runStatusTone } from "./adkChatPresentation";

export const permissionModes: Array<{ title: string; value: ADKPermissionMode }> = [
  { title: "请求批准", value: "approval" },
  { title: "减少审批", value: "less_approval" },
  { title: "全部允许", value: "all" },
];

export function formatPermission(mode: string): string {
  switch (mode) {
    case "less_approval": return "减少审批";
    case "all": return "全部允许";
    default: return "请求批准";
  }
}

export function preview(value: unknown): string {
  try {
    return JSON.stringify(value ?? {}, null, 2);
  } catch {
    return String(value);
  }
}

export function toolCallStatusColor(status: string): string {
  const tone = runStatusTone(status);
  switch (tone) {
    case "success": return "success";
    case "error": return "error";
    case "warning": return "warning";
    case "info": return "info";
    default: return "default";
  }
}

export function riskColor(risk?: string): string {
  switch (risk) {
    case "low": return "success";
    case "medium": return "info";
    case "high": return "warning";
    case "critical": return "error";
    default: return "default";
  }
}

export function riskLabel(risk?: string): string {
  switch (risk) {
    case "low": return "低风险";
    case "medium": return "中风险";
    case "high": return "高风险";
    case "critical": return "关键风险";
    default: return "未标注";
  }
}

export function isInternalSkill(skill: ADKSkill): boolean {
  return skill.source.trim().toLowerCase() === "builtin";
}
