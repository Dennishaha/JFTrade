import type {
  BrokerDescriptor,
  BrokerReadFeatureCapability,
  OnboardingReason,
  OnboardingStateResponse,
} from "@/types";
import type { components } from "@/generated/openapi";

type OnboardingWire = components["schemas"]["settings.OnboardingStateResponse"];

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value != null && !Array.isArray(value);
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((entry) => typeof entry === "string");
}

function isOptionalBoolean(value: unknown): boolean {
  return value == null || typeof value === "boolean";
}

function isOptionalNumber(value: unknown): boolean {
  return value == null || typeof value === "number";
}

function isReadFeature(value: unknown): value is BrokerReadFeatureCapability {
  if (!isRecord(value) || !isStringArray(value.supportedEnvironments)) {
    return false;
  }
  return [
    "supportsHistory",
    "requiresSymbols",
    "requiresClearingDate",
    "requiresPrice",
    "requiresOrderIdEx",
    "requiresSymbol",
    "requiresPassword",
    "supportsRealTimePush",
  ].every((key) => isOptionalBoolean(value[key])) &&
    ["defaultNum", "minNum", "maxNum"].every((key) =>
      isOptionalNumber(value[key]),
    ) &&
    (value.numPresets == null ||
      (Array.isArray(value.numPresets) &&
        value.numPresets.every((entry) => typeof entry === "number")));
}

export function isBrokerDescriptor(value: unknown): value is BrokerDescriptor {
  if (
    !isRecord(value) ||
    typeof value.id !== "string" ||
    typeof value.displayName !== "string" ||
    !isStringArray(value.environments) ||
    !isStringArray(value.notes) ||
    !Array.isArray(value.capabilities)
  ) {
    return false;
  }
  return value.capabilities.every((capability) => {
    if (
      !isRecord(capability) ||
      typeof capability.market !== "string" ||
      typeof capability.supportsQuote !== "boolean" ||
      typeof capability.supportsTrade !== "boolean"
    ) {
      return false;
    }
    if (capability.readFeatures == null) {
      return true;
    }
    if (!isRecord(capability.readFeatures)) {
      return false;
    }
    return Object.values(capability.readFeatures).every(isReadFeature);
  });
}

function normalizeSeverity(value: string | undefined): OnboardingReason["severity"] {
  return value === "error" || value === "warning" ? value : "info";
}

export function mapOnboardingState(
  value: OnboardingWire,
): OnboardingStateResponse {
  const state = value.state;
  return {
    state: {
      completed: state?.completed ?? false,
      lastBrokerId: state?.lastBrokerId ?? "",
      ...(typeof state?.completedAt === "string"
        ? { completedAt: state.completedAt }
        : {}),
      ...(typeof state?.dismissedAt === "string"
        ? { dismissedAt: state.dismissedAt }
        : {}),
    },
    shouldShowOobe: value.shouldShowOobe ?? false,
    reasons: (value.reasons ?? []).map((reason) => ({
      code: reason.code ?? "UNKNOWN",
      severity: normalizeSeverity(reason.severity),
      message: reason.message ?? "",
    })),
    recommendedBrokerId: value.recommendedBrokerId ?? "futu",
    brokers: (value.brokers ?? []).flatMap((broker) =>
      isBrokerDescriptor(broker.descriptor)
        ? [
            {
              descriptor: broker.descriptor,
              enabled: broker.enabled ?? false,
              available: broker.available ?? false,
              configured: broker.configured ?? false,
            },
          ]
        : [],
    ),
  };
}
