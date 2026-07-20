import { computed, ref } from "vue";

import { fetchEnvelope } from "./apiClient";
import { readLocalStorage, writeLocalStorage } from "./safeStorage";

export type BrokerCapabilityState = "available" | "degraded" | "unavailable";

export interface BrokerFeatureCapability {
  id: string;
  markets?: string[];
  supportedPeriods?: string[];
  state: BrokerCapabilityState;
  reason?: string;
}

export interface BrokerMarketCapability {
  market: string;
  supportsQuote: boolean;
  supportsTrade: boolean;
  features?: BrokerFeatureCapability[];
}

export interface BrokerCapabilityDescriptor {
  id: string;
  displayName: string;
  securityFirm?: string;
  capabilityVersion?: string;
  capabilities?: BrokerMarketCapability[];
}

interface BrokerCapabilitiesResponse {
  brokers: BrokerCapabilityDescriptor[];
}

export interface BrokerProviderOption {
  id: string;
  label: string;
  shortLabel: string;
  securityFirm: string;
  state: BrokerCapabilityState;
  reason: string;
}

const STORAGE_KEY = "jftrade.market-provider.v1";
const selectedBrokerId = ref(
  (readLocalStorage(STORAGE_KEY) ?? "").trim().toLowerCase(),
);
const brokerDescriptors = ref<BrokerCapabilityDescriptor[]>([]);
const loading = ref(false);
const loadError = ref("");
const preferredAccountBrokerId = ref("");
const serverDefaultBrokerId = ref("");
let loadPromise: Promise<BrokerCapabilityDescriptor[]> | null = null;

function normalizedID(value: string | null | undefined): string {
  return value?.trim().toLowerCase() ?? "";
}

function shortProviderLabel(
  descriptor: Pick<BrokerCapabilityDescriptor, "id" | "displayName">,
): string {
  const displayName = descriptor.displayName.trim();
  const firstWord = displayName.split(/[\s·/]+/, 1)[0]?.trim();
  if (firstWord) return firstWord.slice(0, 12);
  return descriptor.id.trim().toUpperCase().slice(0, 12) || "数据源";
}

function featureState(
  descriptor: BrokerCapabilityDescriptor,
  featureId: string,
  market: string,
): Pick<BrokerProviderOption, "state" | "reason"> {
  const normalizedFeature = featureId.trim();
  const normalizedMarket = market.trim().toUpperCase();
  const marketCapabilities = descriptor.capabilities ?? [];
  const inMarket = marketCapabilities.filter(
    (capability) =>
      !normalizedMarket ||
      capability.market.trim().toUpperCase() === normalizedMarket,
  );
  const capabilities = normalizedMarket ? inMarket : marketCapabilities;

  if (!normalizedFeature) {
    if (capabilities.some((capability) => capability.supportsQuote)) {
      return { state: "available", reason: "" };
    }
    const featureStates = capabilities.flatMap(
      (capability) => capability.features ?? [],
    );
    if (featureStates.some((feature) => feature.state === "available")) {
      return { state: "available", reason: "" };
    }
    if (featureStates.some((feature) => feature.state === "degraded")) {
      return { state: "degraded", reason: "部分行情或研究能力受限" };
    }
    return { state: "unavailable", reason: "当前没有可用的读取能力" };
  }

  const matches = capabilities
    .flatMap((capability) => capability.features ?? [])
    .filter(
      (feature) =>
        feature.id === normalizedFeature &&
        (!normalizedMarket ||
          feature.markets == null ||
          feature.markets.length === 0 ||
          feature.markets.some(
            (value) => value.trim().toUpperCase() === normalizedMarket,
          )),
    );
  const available = matches.find((feature) => feature.state === "available");
  if (available) return { state: "available", reason: available.reason ?? "" };
  const degraded = matches.find((feature) => feature.state === "degraded");
  if (degraded) {
    return {
      state: "degraded",
      reason: degraded.reason || "此能力当前降级可用",
    };
  }
  const unavailable = matches.find(
    (feature) => feature.state === "unavailable",
  );
  return {
    state: "unavailable",
    reason:
      unavailable?.reason ||
      (normalizedMarket
        ? `不支持 ${normalizedMarket} 的此项能力`
        : "未声明此项能力"),
  };
}

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
  if (selected && available.has(selected)) return selected;
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
): Promise<BrokerCapabilityDescriptor[]> {
  if (!force && brokerDescriptors.value.length > 0) {
    return brokerDescriptors.value;
  }
  if (loadPromise != null) return loadPromise;

  loading.value = true;
  loadError.value = "";
  loadPromise = fetchEnvelope<BrokerCapabilitiesResponse>(
    "/api/v1/brokers/capabilities",
  )
    .then((response) => {
      brokerDescriptors.value = (response.brokers ?? [])
        .filter((broker) => normalizedID(broker.id))
        .sort((left, right) =>
          left.displayName.localeCompare(right.displayName, "zh-CN"),
        );
      const resolved = resolveDefaultBrokerProvider(brokerDescriptors.value);
      if (resolved && resolved !== selectedBrokerId.value) {
        commitBrokerProvider(resolved);
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
  featureId = "",
  market = "",
): BrokerProviderOption[] {
  return brokerDescriptors.value.map((descriptor) => ({
    id: normalizedID(descriptor.id),
    label: descriptor.displayName.trim() || descriptor.id.toUpperCase(),
    shortLabel: shortProviderLabel(descriptor),
    securityFirm: descriptor.securityFirm?.trim() ?? "",
    ...featureState(descriptor, featureId, market),
  }));
}

export function brokerSupportedChartPeriods(
  brokerId: string,
  market: string,
  descriptors = brokerDescriptors.value,
): string[] | null {
  const normalizedBroker = normalizedID(brokerId);
  const descriptor = normalizedBroker
    ? descriptors.find(
        (candidate) => normalizedID(candidate.id) === normalizedBroker,
      )
    : descriptors.length === 1
      ? descriptors[0]
      : undefined;
  if (descriptor == null) return null;

  const normalizedMarket = market.trim().toUpperCase();
  const marketCapability = (descriptor.capabilities ?? []).find(
    (capability) =>
      capability.market.trim().toUpperCase() === normalizedMarket,
  );
  if (marketCapability == null) return [];

  const supported = new Set<string>();
  for (const feature of marketCapability.features ?? []) {
    if (feature.state !== "available" && feature.state !== "degraded") continue;
    if (feature.id === "market.ticks") {
      supported.add("tick");
      continue;
    }
    if (feature.id !== "market.candles") continue;
    for (const period of feature.supportedPeriods ?? []) {
      const normalized = period.trim().toLowerCase();
      if (normalized) supported.add(normalized);
    }
  }
  return [...supported];
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
