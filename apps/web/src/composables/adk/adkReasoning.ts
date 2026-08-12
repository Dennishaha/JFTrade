import type {
  ADKProvider,
  ADKProviderAPIProtocol,
  ADKProviderReasoningConfig,
  ADKReasoningEffort,
} from "@/types";

export const ADK_REASONING_EFFORTS: ADKReasoningEffort[] = [
  "low",
  "medium",
  "high",
  "xhigh",
  "max",
];

export const ADK_REASONING_EFFORT_LABELS: Record<ADKReasoningEffort, string> = {
  low: "低",
  medium: "中",
  high: "高",
  xhigh: "极高",
  max: "最大",
};

export function defaultADKProviderReasoningConfig(
  protocol: ADKProviderAPIProtocol | string = "chat_completions",
): ADKProviderReasoningConfig {
  return {
    requestField: protocol === "responses" ? "reasoning.effort" : "reasoning_effort",
    mappings: [],
  };
}

export function normalizedADKProviderReasoningConfig(
  provider: Pick<ADKProvider, "apiProtocol" | "reasoningConfig">,
): ADKProviderReasoningConfig {
  const fallback = defaultADKProviderReasoningConfig(provider.apiProtocol);
  const config = provider.reasoningConfig;
  if (!config) return fallback;
  return {
    requestField: config.requestField.trim() || fallback.requestField,
    mappings: config.mappings.map((mapping) => ({
      effort: mapping.effort,
      value: mapping.value.trim(),
    })),
  };
}

export function supportedADKReasoningEfforts(
  provider: Pick<ADKProvider, "apiProtocol" | "reasoningConfig"> | undefined,
): ADKReasoningEffort[] {
  if (!provider) return [];
  return normalizedADKProviderReasoningConfig(provider).mappings.map(
    (mapping) => mapping.effort,
  );
}

export function isADKReasoningEffortSupported(
  provider: Pick<ADKProvider, "apiProtocol" | "reasoningConfig"> | undefined,
  effort: string | undefined,
): effort is ADKReasoningEffort {
  return (
    typeof effort === "string" &&
    supportedADKReasoningEfforts(provider).includes(effort as ADKReasoningEffort)
  );
}
