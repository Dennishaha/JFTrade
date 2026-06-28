import type {
  StrategyInstanceItem,
  StrategySourceFormat,
} from "@/contracts";

import { normalizeText } from "./strategyRuntimeInstanceBinding";

export const PINE_WORKER_RUNTIME = "pine-pinets";
export const LEGACY_GO_PINE_RUNTIME = "pine-go-plan";
export const PINE_V6_SOURCE_FORMAT = "pine-v6";

export function isPineWorkerRuntime(runtime: unknown): boolean {
  return normalizeText(runtime) === PINE_WORKER_RUNTIME;
}

export function isLegacyGoPineRuntime(runtime: unknown): boolean {
  return normalizeText(runtime) === LEGACY_GO_PINE_RUNTIME;
}

export function isSupportedPineRuntime(runtime: unknown): boolean {
  return isPineWorkerRuntime(runtime);
}

export function formatStrategyRuntime(runtime: unknown): string {
  switch (normalizeText(runtime)) {
    case PINE_WORKER_RUNTIME:
      return "PineTS worker";
    case LEGACY_GO_PINE_RUNTIME:
      return "Legacy Go Pine";
    default:
      return "未知 / 受限";
  }
}

export function formatSourceFormat(sourceFormat: StrategySourceFormat | string | null | undefined): string {
  switch (normalizeText(sourceFormat)) {
    case PINE_V6_SOURCE_FORMAT:
      return "Pine v6";
    default:
      return "未知 / 受限";
  }
}

export function formatStrategyEligibility(strategy: StrategyInstanceItem): string {
  if (strategy.startable) return "可启动";
  if (isPineWorkerRuntime(strategy.runtime)) return "待启用";
  return "受限";
}
