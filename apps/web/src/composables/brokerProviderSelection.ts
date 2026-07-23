import { computed, ref } from "vue";

import { fetchEnvelope } from "./apiClient";
import { readLocalStorage, writeLocalStorage } from "./safeStorage";

export type BrokerCapabilityState = "available" | "degraded" | "unavailable";

export interface BrokerFeatureCapability {
  id: string;
  markets?: string[];
  supportedPeriods?: string[];
  state: BrokerCapabilityState;
  reasonCode?: string;
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

export interface BrokerRuntimeCapabilityEvaluation {
  state: BrokerCapabilityState;
  code?: string;
  reason?: string;
  checkedAt?: string;
}

export interface BrokerRuntimeCapabilityStatus {
  brokerId: string;
  securityFirm?: string;
  market: string;
  featureId: string;
  capability: BrokerFeatureCapability;
  evaluation?: BrokerRuntimeCapabilityEvaluation;
}

interface BrokerCapabilitiesResponse {
  brokers: BrokerCapabilityDescriptor[];
  runtime?: BrokerRuntimeCapabilityStatus[];
}

export type BrokerFeatureSelector = string | readonly string[];

export interface BrokerCapabilitySummary {
  state: BrokerCapabilityState;
  reason: string;
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
const brokerRuntimeCapabilities = ref<BrokerRuntimeCapabilityStatus[]>([]);
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

function normalizedFeatureIDs(value: BrokerFeatureSelector): string[] {
  const values = Array.isArray(value) ? value : [value];
  return [
    ...new Set(
      values
        .map((feature) => feature.trim())
        .filter(Boolean),
    ),
  ];
}

export function logicalCapabilityMarkets(market: string): string[] {
  const normalized = market.trim().toUpperCase();
  if (normalized === "CN") return ["SH", "SZ"];
  return normalized ? [normalized] : [];
}

function validCapabilityState(value: unknown): value is BrokerCapabilityState {
  return ["available", "degraded", "unavailable"].includes(String(value));
}

const localizedRuntimeCapabilityReasons: Record<string, string> = {
  OPEND_UNCONFIGURED: "尚未配置 OpenD",
  OPEND_CONNECTION_UNAVAILABLE: "当前无法连接 OpenD",
  OPEND_NOT_LOGGED_IN: "OpenD 行情或交易会话尚未登录",
  QUOTE_RIGHT_QUERY_FAILED: "暂时无法核验当前 OpenD 行情权限",
  QUOTE_RIGHT_UNVERIFIED: "尚未完成当前 OpenD 行情权限核验",
  QUOTE_RIGHT_POLLING_ONLY: "当前权限仅支持快照轮询，不支持实时推送",
  QUOTE_RIGHT_DENIED: "当前 OpenD 会话未开通该市场或品种的行情权限",
  QUOTE_RIGHT_UNKNOWN: "OpenD 返回了无法识别的行情权限状态",
};

function runtimeCapabilityReason(
  status: BrokerRuntimeCapabilityStatus,
): string {
  const rawReason =
    status.evaluation?.reason?.trim() ||
    status.capability.reason?.trim() ||
    "";
  if (/[\u3400-\u9fff]/u.test(rawReason)) return rawReason;
  const code =
    status.evaluation?.code?.trim() ||
    status.capability.reasonCode?.trim() ||
    "";
  return localizedRuntimeCapabilityReasons[code] || rawReason || code;
}

function uniqueReasons(values: BrokerCapabilitySummary[]): string[] {
  return [
    ...new Set(
      values
        .map((value) => value.reason.trim())
        .filter(Boolean),
    ),
  ];
}

function aggregateRequired(
  values: BrokerCapabilitySummary[],
  degradedFallback: string,
  unavailableFallback: string,
): BrokerCapabilitySummary {
  if (values.length === 0) {
    return { state: "unavailable", reason: unavailableFallback };
  }
  if (values.every((value) => value.state === "available")) {
    return { state: "available", reason: "" };
  }
  const reasons = uniqueReasons(
    values.filter((value) => value.state !== "available"),
  );
  if (values.every((value) => value.state === "unavailable")) {
    return {
      state: "unavailable",
      reason: reasons.join("；") || unavailableFallback,
    };
  }
  return {
    state: "degraded",
    reason: reasons.join("；") || degradedFallback,
  };
}

function aggregateAlternative(
  values: BrokerCapabilitySummary[],
  unavailableFallback: string,
): BrokerCapabilitySummary {
  const available = values.find((value) => value.state === "available");
  if (available) return { state: "available", reason: "" };
  const degraded = values.filter((value) => value.state === "degraded");
  if (degraded.length > 0) {
    return {
      state: "degraded",
      reason: uniqueReasons(degraded).join("；") || "此能力当前降级可用",
    };
  }
  return {
    state: "unavailable",
    reason: uniqueReasons(values).join("；") || unavailableFallback,
  };
}

function runtimeFeatureState(
  descriptor: BrokerCapabilityDescriptor,
  featureId: string,
  market: string,
): BrokerCapabilitySummary | null {
  const status = brokerRuntimeCapabilities.value.find(
    (candidate) =>
      normalizedID(candidate.brokerId) === normalizedID(descriptor.id) &&
      candidate.featureId.trim() === featureId &&
      candidate.market.trim().toUpperCase() === market,
  );
  if (status == null) return null;
  const state = validCapabilityState(status.evaluation?.state)
    ? status.evaluation.state
    : validCapabilityState(status.capability.state)
      ? status.capability.state
      : null;
  if (state == null) return null;
  return {
    state,
    reason: runtimeCapabilityReason(status),
  };
}

function staticFeatureState(
  descriptor: BrokerCapabilityDescriptor,
  featureId: string,
  market: string,
): BrokerCapabilitySummary {
  const marketCapability = (descriptor.capabilities ?? []).find(
    (capability) =>
      capability.market.trim().toUpperCase() === market,
  );
  const feature = (marketCapability?.features ?? []).find(
    (candidate) =>
      candidate.id === featureId &&
      (candidate.markets == null ||
        candidate.markets.length === 0 ||
        candidate.markets.some(
          (value) => value.trim().toUpperCase() === market,
        )),
  );
  if (feature == null) {
    return {
      state: "unavailable",
      reason: `不支持 ${market} 的此项能力`,
    };
  }
  return {
    state: feature.state,
    reason:
      feature.reason?.trim() ||
      (feature.state === "degraded" ? "此能力当前降级可用" : ""),
  };
}

function featureStateForMarket(
  descriptor: BrokerCapabilityDescriptor,
  featureId: string,
  market: string,
): BrokerCapabilitySummary {
  return (
    runtimeFeatureState(descriptor, featureId, market) ??
    staticFeatureState(descriptor, featureId, market)
  );
}

function featureStateAcrossMarkets(
  descriptor: BrokerCapabilityDescriptor,
  featureId: string,
  logicalMarket: string,
): BrokerCapabilitySummary {
  const markets = logicalCapabilityMarkets(logicalMarket);
  if (markets.length === 0) {
    const hasDeclaredFeature = (descriptor.capabilities ?? []).some(
      (capability) =>
        (capability.features ?? []).some(
          (candidate) => candidate.id === featureId,
        ),
    );
    const hasRuntimeFeature = brokerRuntimeCapabilities.value.some(
      (status) =>
        normalizedID(status.brokerId) === normalizedID(descriptor.id) &&
        status.featureId.trim() === featureId,
    );
    if (!hasDeclaredFeature && !hasRuntimeFeature) {
      return { state: "unavailable", reason: "未声明此项能力" };
    }
    const declaredMarkets = [
      ...new Set(
        [
          ...(descriptor.capabilities ?? []).map((value) => value.market),
          ...brokerRuntimeCapabilities.value
            .filter(
              (status) =>
                normalizedID(status.brokerId) === normalizedID(descriptor.id) &&
                status.featureId.trim() === featureId,
            )
            .map((status) => status.market),
        ]
          .map((value) => value.trim().toUpperCase())
          .filter(Boolean),
      ),
    ];
    return aggregateAlternative(
      declaredMarkets.map((market) =>
        featureStateForMarket(descriptor, featureId, market),
      ),
      "未声明此项能力",
    );
  }
  const branchStates = markets.map((market) => {
    const state = featureStateForMarket(descriptor, featureId, market);
    if (markets.length === 1 || state.state === "available") return state;
    return {
      ...state,
      reason: state.reason ? `${market}：${state.reason}` : `${market} 能力受限`,
    };
  });
  return aggregateRequired(
    branchStates,
    "部分市场的此项能力受限",
    logicalMarket.trim().toUpperCase()
      ? `不支持 ${logicalMarket.trim().toUpperCase()} 的此项能力`
      : "未声明此项能力",
  );
}

function staticReadState(
  descriptor: BrokerCapabilityDescriptor,
  market: string,
): BrokerCapabilitySummary {
  const markets = logicalCapabilityMarkets(market);
  const capabilities = descriptor.capabilities ?? [];
  if (markets.length === 0) {
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
  return aggregateRequired(
    markets.map((branchMarket) => {
      const capability = capabilities.find(
        (candidate) =>
          candidate.market.trim().toUpperCase() === branchMarket,
      );
      if (capability?.supportsQuote) return { state: "available", reason: "" };
      const features = capability?.features ?? [];
      if (features.some((feature) => feature.state === "available")) {
        return { state: "available", reason: "" };
      }
      if (features.some((feature) => feature.state === "degraded")) {
        return {
          state: "degraded",
          reason:
            markets.length === 1
              ? "部分行情或研究能力受限"
              : `${branchMarket}：部分行情或研究能力受限`,
        };
      }
      return {
        state: "unavailable",
        reason:
          markets.length === 1
            ? "当前没有可用的读取能力"
            : `${branchMarket}：当前没有可用的读取能力`,
      };
    }),
    "部分行情或研究能力受限",
    "当前没有可用的读取能力",
  );
}

function featureState(
  descriptor: BrokerCapabilityDescriptor,
  featureSelector: BrokerFeatureSelector,
  market: string,
): BrokerCapabilitySummary {
  const featureIds = normalizedFeatureIDs(featureSelector);
  if (featureIds.length === 0) return staticReadState(descriptor, market);
  const featureStates = featureIds.map((featureId) => {
    const state = featureStateAcrossMarkets(descriptor, featureId, market);
    if (featureIds.length === 1 || state.state === "available") return state;
    return {
      ...state,
      reason: state.reason
        ? `${featureId}：${state.reason}`
        : `${featureId} 能力受限`,
    };
  });
  return aggregateRequired(
    featureStates,
    "部分行情或研究能力受限",
    market.trim()
      ? `不支持 ${market.trim().toUpperCase()} 的这些能力`
      : "未声明这些能力",
  );
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
      brokerRuntimeCapabilities.value = (response.runtime ?? []).filter(
        (status) =>
          normalizedID(status.brokerId) &&
          status.featureId.trim() &&
          status.market.trim(),
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
  featureId: BrokerFeatureSelector = "",
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
  return featureState(descriptor, featureId, market);
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
