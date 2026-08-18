import { computed, ref } from "vue";

import { apiGet } from "@/composables/shared/apiClient";
import {
  readLocalStorage,
  writeLocalStorage,
} from "@/composables/shared/safeStorage";
import {
  brokerProviderOption,
  brokerSupportedChartPeriods as resolveBrokerSupportedChartPeriods,
  descriptorCapabilityPresentation,
  featureState,
  logicalCapabilityMarkets,
  mapBrokerCapabilities,
  normalizedID,
  resolveBrokerProviderDisplayName as resolveDisplayName,
} from "./brokerProviderCapabilities";
import type {
  BrokerCapabilityDescriptor,
  BrokerCapabilityPresentation,
  BrokerCapabilitySummary,
  BrokerFeatureSelector,
  BrokerProviderNameInput,
  BrokerProviderOption,
  BrokerRuntimeCapabilityStatus,
} from "./brokerProviderModels";

export type {
  BrokerCapabilityDescriptor,
  BrokerCapabilityPresentation,
  BrokerCapabilityState,
  BrokerCapabilitySummary,
  BrokerFeatureCapability,
  BrokerFeatureSelector,
  BrokerMarketCapability,
  BrokerProviderDisplayState,
  BrokerProviderDisplayTone,
  BrokerProviderOption,
  BrokerRuntimeCapabilityEvaluation,
  BrokerRuntimeCapabilityStatus,
} from "./brokerProviderModels";

const STORAGE_KEY = "jftrade.market-provider.v1";
const BUILT_IN_MARKET_DATA_PROVIDER_IDS = new Set(["yfinance", "akshare"]);
const selectedBrokerId = ref(
  (readLocalStorage(STORAGE_KEY) ?? "").trim().toLowerCase(),
);
const brokerDescriptors = ref<BrokerCapabilityDescriptor[]>([]);
const brokerRuntimeCapabilities = ref<BrokerRuntimeCapabilityStatus[]>([]);
const loading = ref(false);
const loadError = ref("");
const preferredAccountBrokerId = ref("");
const serverDefaultBrokerId = ref("");
let loadPromise: Promise<BrokerCapabilityDescriptor[]> | null = null;

/** Resolve a user-facing provider name from a descriptor, id, or both. */
export function resolveBrokerProviderDisplayName(
  value: BrokerProviderNameInput,
  descriptors: readonly BrokerCapabilityDescriptor[] = brokerDescriptors.value,
): string {
  return resolveDisplayName(value, descriptors);
}

// Short alias for callers that already use the provider-first naming style.
export const brokerProviderDisplayName = resolveBrokerProviderDisplayName;

export { logicalCapabilityMarkets };

function commitBrokerProvider(brokerId: string): void {
  const value = normalizedID(brokerId);
  if (!value) return;
  selectedBrokerId.value = value;
  writeLocalStorage(STORAGE_KEY, value);
}

function selectBrokerProvider(brokerId: string): void {
  commitBrokerProvider(brokerId);
}

function resolveDefaultBrokerProvider(
  descriptors = brokerDescriptors.value,
): string {
  const available = new Set(
    descriptors.map((descriptor) => normalizedID(descriptor.id)).filter(Boolean),
  );
  const selected = normalizedID(selectedBrokerId.value);
  if (
    selected &&
    (BUILT_IN_MARKET_DATA_PROVIDER_IDS.has(selected) || available.has(selected))
  ) {
    return selected;
  }
  for (const candidate of [
    preferredAccountBrokerId.value,
    serverDefaultBrokerId.value,
    descriptors[0]?.id,
  ]) {
    const normalized = normalizedID(candidate);
    if (normalized && available.has(normalized)) return normalized;
  }
  return "";
}

export function configureBrokerProviderDefaults(input: {
  accountBrokerId?: string | null;
  defaultBrokerId?: string | null;
}): void {
  preferredAccountBrokerId.value = normalizedID(input.accountBrokerId);
  serverDefaultBrokerId.value = normalizedID(input.defaultBrokerId);
  const resolved = resolveDefaultBrokerProvider();
  if (resolved && resolved !== selectedBrokerId.value) {
    commitBrokerProvider(resolved);
  }
}

async function loadBrokerProviders(
  force = false,
  applyDefaults = true,
): Promise<BrokerCapabilityDescriptor[]> {
  if (!force && brokerDescriptors.value.length > 0) {
    return brokerDescriptors.value;
  }
  if (loadPromise != null) return loadPromise;

  loading.value = true;
  loadError.value = "";
  loadPromise = apiGet("/api/v1/brokers/capabilities")
    .then((response) => {
      const mapped = mapBrokerCapabilities(response);
      brokerDescriptors.value = mapped.brokers
        .filter((broker) => normalizedID(broker.id))
        .sort((left, right) =>
          left.displayName.localeCompare(right.displayName, "zh-CN"),
        );
      brokerRuntimeCapabilities.value = mapped.runtime.filter(
        (status) =>
          normalizedID(status.brokerId) &&
          status.featureId.trim() &&
          status.market.trim(),
      );
      if (applyDefaults) {
        const resolved = resolveDefaultBrokerProvider(brokerDescriptors.value);
        if (resolved && resolved !== selectedBrokerId.value) {
          commitBrokerProvider(resolved);
        }
      }
      return brokerDescriptors.value;
    })
    .catch((cause: unknown) => {
      loadError.value = cause instanceof Error ? cause.message : String(cause);
      return brokerDescriptors.value;
    })
    .finally(() => {
      loading.value = false;
      loadPromise = null;
    });
  return loadPromise;
}

export function brokerProviderOptions(
  featureId: BrokerFeatureSelector = "",
  market = "",
): BrokerProviderOption[] {
  return brokerDescriptors.value.map((descriptor) =>
    brokerProviderOption(
      descriptor,
      featureId,
      market,
      brokerRuntimeCapabilities.value,
    ),
  );
}

export function brokerCapabilitySummary(
  brokerId: string,
  featureId: BrokerFeatureSelector = "",
  market = "",
): BrokerCapabilitySummary {
  const normalizedBroker = normalizedID(brokerId);
  const descriptor = brokerDescriptors.value.find(
    (candidate) => normalizedID(candidate.id) === normalizedBroker,
  );
  if (descriptor == null) {
    return {
      state: "unavailable",
      reason: normalizedBroker
        ? `未找到券商 ${normalizedBroker} 的能力目录`
        : "尚未选择行情提供者",
    };
  }
  return featureState(
    descriptor,
    featureId,
    market,
    brokerRuntimeCapabilities.value,
  );
}

/** Return UI presentation state while keeping raw capability state unchanged. */
export function brokerProviderCapabilityPresentation(
  brokerId: string,
  featureId: BrokerFeatureSelector = "",
  market = "",
): BrokerCapabilityPresentation {
  const normalizedBroker = normalizedID(brokerId);
  const descriptor = brokerDescriptors.value.find(
    (candidate) => normalizedID(candidate.id) === normalizedBroker,
  );
  if (descriptor == null) {
    return { displayState: "unavailable", tone: "error" };
  }
  const summary = featureState(
    descriptor,
    featureId,
    market,
    brokerRuntimeCapabilities.value,
  );
  return descriptorCapabilityPresentation(
    descriptor,
    featureId,
    market,
    summary,
    brokerRuntimeCapabilities.value,
  );
}

export function brokerSupportedChartPeriods(
  brokerId: string,
  market: string,
  descriptors = brokerDescriptors.value,
): string[] | null {
  return resolveBrokerSupportedChartPeriods(brokerId, market, descriptors);
}

export function withBrokerProvider(path: string, brokerId: string): string {
  const normalizedBroker = normalizedID(brokerId);
  if (!path || !normalizedBroker) return path;

  const hashIndex = path.indexOf("#");
  const hash = hashIndex >= 0 ? path.slice(hashIndex) : "";
  const resource = hashIndex >= 0 ? path.slice(0, hashIndex) : path;
  const queryIndex = resource.indexOf("?");
  const pathname = queryIndex >= 0 ? resource.slice(0, queryIndex) : resource;
  const params = new URLSearchParams(
    queryIndex >= 0 ? resource.slice(queryIndex + 1) : "",
  );
  params.set("brokerId", normalizedBroker);
  return `${pathname}?${params.toString()}${hash}`;
}

export function useBrokerProviderSelection() {
  return {
    brokerDescriptors,
    brokerRuntimeCapabilities,
    loadBrokerProviders,
    loadError,
    loading,
    options: computed(() => brokerProviderOptions()),
    selectBrokerProvider,
    selectedBrokerId,
  };
}

export function resetBrokerProviderSelectionForTests(): void {
  selectedBrokerId.value = "";
  brokerDescriptors.value = [];
  brokerRuntimeCapabilities.value = [];
  loading.value = false;
  loadError.value = "";
  preferredAccountBrokerId.value = "";
  serverDefaultBrokerId.value = "";
  loadPromise = null;
  try {
    globalThis.window?.localStorage?.removeItem(STORAGE_KEY);
  } catch {
    // Tests may use an opaque document origin.
  }
}
