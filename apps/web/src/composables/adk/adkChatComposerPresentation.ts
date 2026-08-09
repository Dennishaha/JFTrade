import type { ADKSessionContextSnapshot } from "@/types";
import { formatNumber } from "@/utils/numberFormat";

export function formatTokenCount(value: number): string {
  return formatNumber(Math.max(0, value), { maximumFractionDigits: 3 });
}

export function contextWindowLabel(
  snapshot: ADKSessionContextSnapshot | null | undefined,
): string {
  if (!snapshot || snapshot.contextWindowTokens <= 0) {
    return "未配置";
  }
  return formatTokenCount(snapshot.contextWindowTokens);
}

export function contextRevisionLabel(
  snapshot: ADKSessionContextSnapshot | null | undefined,
): string {
  const revision = snapshot?.contextRevisionId?.trim() ?? "";
  if (revision === "") return "未生成";
  return revision.length > 18 ? `${revision.slice(0, 18)}...` : revision;
}

export function compactionModeLabel(mode?: string): string {
  switch (mode) {
    case "manual":
      return "手动";
    case "auto":
      return "自动";
    case "aggressive":
      return "激进";
    default:
      return "未执行";
  }
}
