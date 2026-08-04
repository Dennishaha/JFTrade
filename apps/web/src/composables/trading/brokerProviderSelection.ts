import { computed, ref } from "vue";

import type {
  BrokerCapabilitiesDto,
  BrokerDescriptorDto,
  BrokerFeatureCapabilityDto,
  BrokerRuntimeCapabilityStatusDto,
} from "@/contracts";

import { apiGet } from "@/composables/shared/apiClient";
import { readLocalStorage, writeLocalStorage } from "@/composables/shared/safeStorage";

export type BrokerCapabilityState = "available" | "degraded" | "unavailable";

/**
 * Presentation state intentionally differs from capability state. A provider
 * may advertise a degraded capability (for example, delayed HTTP snapshots)
 * while still operating normally for the feature currently being viewed.
 */
export type BrokerProviderDisplayState = BrokerCapabilityState;
export type BrokerProviderDisplayTone = "success" | "warning" | "error";

export interface BrokerCapabilityPresentation {
  displayState: BrokerProviderDisplayState;
  tone: BrokerProviderDisplayTone;
}

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
  /** UI-only state; `state` remains the raw capability state for selection. */
  displayState?: BrokerProviderDisplayState;
  /** UI-only semantic color, independent of capability selection semantics. */
  tone?: BrokerProviderDisplayTone;
  /** Static capability gate; runtime health may additionally disable selection. */
  selectable: boolean;
}

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

type BrokerCapabilitiesWire = BrokerCapabilitiesDto;

function mapCapabilityState(value: string): BrokerCapabilityState {
  switch (value) {
    case "available":
    case "degraded":
    case "unavailable":
      return value;
    default:
      return "unavailable";
  }
}

function mapFeatureCapability(
  value: BrokerFeatureCapabilityDto,
): BrokerFeatureCapability {
  return {
    id: value.id,
    state: mapCapabilityState(value.state),
    ...(Array.isArray(value.markets) ? { markets: [...value.markets] } : {}),
    ...(value.supportedPeriods == null
      ? {}
      : { supportedPeriods: [...value.supportedPeriods] }),
    ...(value.reasonCode == null ? {} : { reasonCode: value.reasonCode }),
    ...(value.reason == null ? {} : { reason: value.reason }),
  };
}

function mapBrokerDescriptor(
  value: BrokerDescriptorDto,
): BrokerCapabilityDescriptor {
  const capabilityVersion = value.capabilityVersion?.trim();
  return {
    id: value.id,
    displayName: value.displayName,
    capabilities: (value.capabilities ?? []).map((capability) => ({
      market: capability.market,
      supportsQuote: capability.supportsQuote,
      supportsTrade: capability.supportsTrade,
      ...(capability.features == null
        ? {}
        : {
            features: capability.features.map(mapFeatureCapability),
          }),
    })),
    ...(value.securityFirm == null
      ? {}
      : { securityFirm: value.securityFirm }),
    ...(capabilityVersion ? { capabilityVersion } : {}),
  };
}

function mapRuntimeCapability(
  value: BrokerRuntimeCapabilityStatusDto,
): BrokerRuntimeCapabilityStatus {
  const checkedAt = value.evaluation.checkedAt?.trim();
  return {
    brokerId: value.brokerId,
    market: value.market,
    featureId: value.featureId,
    capability: mapFeatureCapability(value.capability),
    evaluation: {
      state: mapCapabilityState(value.evaluation.state),
      ...(checkedAt ? { checkedAt } : {}),
      ...(value.evaluation.code == null
        ? {}
        : { code: value.evaluation.code }),
      ...(value.evaluation.reason == null
        ? {}
        : { reason: value.evaluation.reason }),
    },
    ...(value.securityFirm == null
      ? {}
      : { securityFirm: value.securityFirm }),
  };
}

function mapBrokerCapabilities(response: BrokerCapabilitiesWire): {
  brokers: BrokerCapabilityDescriptor[];
  runtime: BrokerRuntimeCapabilityStatus[];
} {
  return {
    brokers: (response.brokers ?? []).map(mapBrokerDescriptor),
    runtime: (response.runtime ?? []).map(mapRuntimeCapability),
  };
}

function normalizedID(value: string | null | undefined): string {
  return value?.trim().toLowerCase() ?? "";
}

function shortProviderLabel(
  descriptor: Pick<BrokerCapabilityDescriptor, "id" | "displayName">,
): string {
  const providerID = normalizedID(descriptor.id);
  if (providerID === "yfinance" || providerID === "yahoo-finance") return "Yahoo";
  if (providerID === "akshare") return "AKShare";
  if (providerID === "futu" || providerID === "futu-opend") return "Futu";
  const displayName = descriptor.displayName.trim();
  const firstWord = displayName.split(/[\s·/]+/, 1)[0]?.trim();
  if (firstWord) return firstWord.slice(0, 12);
  return descriptor.id.trim().toUpperCase().slice(0, 12) || "数据源";
}

type BrokerProviderNameInput =
  | Pick<BrokerCapabilityDescriptor, "id" | "displayName">
  | string
  | null
  | undefined;

/** Resolve a user-facing provider name from a descriptor, id, or both. */
export function resolveBrokerProviderDisplayName(
  value: BrokerProviderNameInput,
  descriptors: readonly BrokerCapabilityDescriptor[] = brokerDescriptors.value,
): string {
  const inputID =
    typeof value === "string" ? normalizedID(value) : normalizedID(value?.id);
  if (inputID === "yfinance" || inputID === "yahoo-finance") return "Yahoo";
  if (inputID === "akshare") return "AKShare";
  const descriptor =
    typeof value === "string"
      ? descriptors.find((candidate) => normalizedID(candidate.id) === inputID)
      : value;
  const displayName = descriptor?.displayName?.trim();
  if (displayName) return displayName;
  if (inputID === "futu" || inputID === "futu-opend") return "Futu OpenD";
  return inputID ? inputID.toUpperCase() : "";
}

// Short alias for callers that already use the provider-first naming style.
export const brokerProviderDisplayName = resolveBrokerProviderDisplayName;

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

function staticFeatureStateAcrossMarkets(
  descriptor: BrokerCapabilityDescriptor,
  featureId: string,
  logicalMarket: string,
): BrokerCapabilitySummary {
  const markets = logicalCapabilityMarkets(logicalMarket);
  if (markets.length === 0) {
    const declaredMarkets = (descriptor.capabilities ?? [])
      .filter((capability) =>
        (capability.features ?? []).some(
          (candidate) => candidate.id === featureId,
        ),
      )
      .map((capability) => capability.market.trim().toUpperCase())
      .filter(Boolean);
    return aggregateAlternative(
      declaredMarkets.map((market) =>
        staticFeatureState(descriptor, featureId, market),
      ),
      "未声明此项能力",
    );
  }
  const branchStates = markets.map((market) => {
    const state = staticFeatureState(descriptor, featureId, market);
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

function staticFeatureSummary(
  descriptor: BrokerCapabilityDescriptor,
  featureSelector: BrokerFeatureSelector,
  market: string,
): BrokerCapabilitySummary {
  const featureIds = normalizedFeatureIDs(featureSelector);
  if (featureIds.length === 0) return staticReadState(descriptor, market);
  const featureStates = featureIds.map((featureId) => {
    const state = staticFeatureStateAcrossMarkets(descriptor, featureId, market);
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

function matchingRuntimeCapabilities(
  descriptor: BrokerCapabilityDescriptor,
  featureSelector: BrokerFeatureSelector,
  market: string,
): BrokerRuntimeCapabilityStatus[] {
  const featureIDs = normalizedFeatureIDs(featureSelector);
  const markets = logicalCapabilityMarkets(market);
  return brokerRuntimeCapabilities.value.filter((status) => {
    if (normalizedID(status.brokerId) !== normalizedID(descriptor.id)) {
      return false;
    }
    if (
      featureIDs.length > 0 &&
      !featureIDs.includes(status.featureId.trim())
    ) {
      return false;
    }
    return (
      markets.length === 0 ||
      markets.includes(status.market.trim().toUpperCase())
    );
  });
}

function capabilityPresentation(
  summary: BrokerCapabilitySummary,
  runtimeStatuses: readonly BrokerRuntimeCapabilityStatus[],
): BrokerCapabilityPresentation {
  // Unavailable always remains an error. Runtime degradation is also visible
  // to users; static degraded declarations are normal for that provider.
  if (summary.state === "unavailable") {
    return { displayState: "unavailable", tone: "error" };
  }
  const runtimeStates = runtimeStatuses.map((status) =>
    status.evaluation?.state ?? status.capability.state,
  );
  if (runtimeStates.some((state) => state === "degraded")) {
    return { displayState: "degraded", tone: "warning" };
  }
  // `summary` already aggregates runtime unavailability for the selected
  // feature(s). An unavailable status for an unrelated capability must not
  // turn an otherwise usable provider red.
  if (
    summary.state === "degraded" &&
    runtimeStates.some((state) => state === "unavailable")
  ) {
    return { displayState: "degraded", tone: "warning" };
  }
  return { displayState: "available", tone: "success" };
}

function descriptorCapabilityPresentation(
  descriptor: BrokerCapabilityDescriptor,
  featureSelector: BrokerFeatureSelector,
  market: string,
  summary: BrokerCapabilitySummary,
): BrokerCapabilityPresentation {
  return capabilityPresentation(
    summary,
    matchingRuntimeCapabilities(descriptor, featureSelector, market),
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
  return brokerDescriptors.value.map((descriptor) => {
    const summary = featureState(descriptor, featureId, market);
    const selection = staticFeatureSummary(descriptor, featureId, market);
    return {
      id: normalizedID(descriptor.id),
      label: resolveBrokerProviderDisplayName(descriptor),
      shortLabel: shortProviderLabel(descriptor),
      securityFirm: descriptor.securityFirm?.trim() ?? "",
      selectable:
        selection.state !== "unavailable" && summary.state !== "unavailable",
      ...summary,
      ...descriptorCapabilityPresentation(
        descriptor,
        featureId,
        market,
        summary,
      ),
    };
  });
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
  const summary = featureState(descriptor, featureId, market);
  return descriptorCapabilityPresentation(
    descriptor,
    featureId,
    market,
    summary,
  );
}

export function brokerSupportedChartPeriods(
  brokerId: string,
  market: string,
  descriptors = brokerDescriptors.value,
): string[] | null {
  const normalizedBroker = normalizedID(brokerId);
  if (normalizedBroker === "yfinance" || normalizedBroker === "akshare") {
    const normalizedMarket = market.trim().toUpperCase();
    return normalizedMarket === "" ||
      new Set(["US", "HK", "CN", "SH", "SZ"]).has(normalizedMarket)
      ? ["1m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo"]
      : [];
  }
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
