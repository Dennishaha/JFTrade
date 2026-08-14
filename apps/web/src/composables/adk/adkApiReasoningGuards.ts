import type {
  ADKProviderReasoningConfig,
  ADKProviderReasoningMapping,
  ADKProviderReasoningTestResponse,
  ADKProviderReasoningTestResult,
  ADKProviderTestResponse,
  ADKProviderTestMode,
  ADKReasoningEffort,
} from "@/types";
import type { ADKAgentWriteRequestDto } from "@/contracts";

type TypeGuard<T> = (value: unknown) => value is T;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isString(value: unknown): value is string {
  return typeof value === "string";
}

function isBoolean(value: unknown): value is boolean {
  return typeof value === "boolean";
}

function isArrayOf<T>(value: unknown, guard: TypeGuard<T>): value is T[] {
  return Array.isArray(value) && value.every(guard);
}

function isOptional<T>(value: unknown, guard: TypeGuard<T>): boolean {
  return value === undefined || guard(value);
}

export function isReasoningEffort(value: unknown): value is ADKReasoningEffort {
  return (
    value === "low" ||
    value === "medium" ||
    value === "high" ||
    value === "xhigh" ||
    value === "max"
  );
}

export function isOptionalReasoningEffortWire(
  value: unknown,
): value is ADKReasoningEffort | "" | undefined {
  return value === undefined || value === "" || isReasoningEffort(value);
}

function isProviderTestMode(value: unknown): value is ADKProviderTestMode {
  return value === "quick" || value === "full";
}

export function isADKProviderReasoningMapping(
  value: unknown,
): value is ADKProviderReasoningMapping {
  return (
    isRecord(value) &&
    isReasoningEffort(value.effort) &&
    isString(value.value) &&
    value.value.trim() !== ""
  );
}

export function isADKProviderReasoningConfig(
  value: unknown,
): value is ADKProviderReasoningConfig {
  return (
    isRecord(value) &&
    isString(value.requestField) &&
    isArrayOf(value.mappings, isADKProviderReasoningMapping)
  );
}

export function isADKProviderReasoningTestResult(
  value: unknown,
): value is ADKProviderReasoningTestResult {
  return (
    isRecord(value) &&
    isReasoningEffort(value.effort) &&
    isString(value.value) &&
    isBoolean(value.ok) &&
    isOptional(value.error, isString)
  );
}

export function isADKProviderReasoningTestResponse(
  value: unknown,
): value is ADKProviderReasoningTestResponse {
  return (
    isRecord(value) &&
    isProviderTestMode(value.mode) &&
    isString(value.requestField) &&
    isBoolean(value.ok) &&
    isArrayOf(value.results, isADKProviderReasoningTestResult)
  );
}

export function isADKProviderTestResponse(
  value: unknown,
): value is ADKProviderTestResponse {
  return (
    isRecord(value) &&
    isBoolean(value.ok) &&
    isString(value.reply) &&
    isRecord(value.capabilities) &&
    Object.values(value.capabilities).every(isBoolean) &&
    isADKProviderReasoningTestResponse(value.reasoning) &&
    isString(value.checkedAt)
  );
}

export function normalizeADKAgentTemplateWire(value: unknown): unknown {
  if (!isRecord(value)) return value;
  const template = value as Partial<ADKAgentWriteRequestDto>;
  return {
    ...value,
    model: template.model === undefined ? "" : template.model,
    reasoningEffort:
      template.reasoningEffort === undefined ? "" : template.reasoningEffort,
    tools: template.tools === undefined ? [] : template.tools,
    toolAccessMode:
      template.toolAccessMode === undefined
        ? template.tools && template.tools.length > 0
          ? "selected"
          : "all"
        : template.toolAccessMode,
    skills: template.skills === undefined ? [] : template.skills,
    recentUserWindow:
      template.recentUserWindow === undefined ? 6 : template.recentUserWindow,
    workMode: template.workMode === undefined ? "chat" : template.workMode,
    loopMaxIterations:
      template.loopMaxIterations === undefined ? 5 : template.loopMaxIterations,
  };
}

export function normalizeADKAgentWire(value: unknown): unknown {
  if (!isRecord(value) || value.toolAccessMode !== undefined) return value;
  return {
    ...value,
    toolAccessMode:
      Array.isArray(value.tools) && value.tools.length > 0 ? "selected" : "all",
  };
}
