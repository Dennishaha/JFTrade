import type {
  BrokerCapabilitiesDto,
  BrokerDescriptorDto,
  BrokerFeatureCapabilityDto,
  BrokerRuntimeCapabilityStatusDto,
} from "@/contracts";
import {
  mapSupportedCandleSessions,
} from "./brokerCandleSessions";
import type {
  BrokerCapabilityDescriptor,
  BrokerCapabilityPresentation,
  BrokerCapabilityState,
  BrokerCapabilitySummary,
  BrokerFeatureCapability,
  BrokerFeatureSelector,
  BrokerProviderNameInput,
  BrokerProviderOption,
  BrokerRuntimeCapabilityStatus,
} from "./brokerProviderModels";

const BUILT_IN_MARKET_DATA_PROVIDER_IDS = new Set(["yfinance", "akshare"]);

export function mapCapabilityState(value: string): BrokerCapabilityState {
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
  const supportedSessions = mapSupportedCandleSessions(value.supportedSessions);
  return {
    id: value.id,
    state: mapCapabilityState(value.state),
    ...(Array.isArray(value.markets) ? { markets: [...value.markets] } : {}),
    ...(value.supportedPeriods == null
      ? {}
      : { supportedPeriods: [...value.supportedPeriods] }),
    ...(supportedSessions == null ? {} : { supportedSessions }),
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
        : { features: capability.features.map(mapFeatureCapability) }),
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
      ...(value.evaluation.code == null ? {} : { code: value.evaluation.code }),
      ...(value.evaluation.reason == null
        ? {}
        : { reason: value.evaluation.reason }),
    },
    ...(value.securityFirm == null ? {} : { securityFirm: value.securityFirm }),
  };
}

export function mapBrokerCapabilities(response: BrokerCapabilitiesDto): {
  brokers: BrokerCapabilityDescriptor[];
  runtime: BrokerRuntimeCapabilityStatus[];
} {
  return {
    brokers: (response.brokers ?? []).map(mapBrokerDescriptor),
    runtime: (response.runtime ?? []).map(mapRuntimeCapability),
  };
}

export function normalizedID(value: string | null | undefined): string {
  return value?.trim().toLowerCase() ?? "";
}

export function shortProviderLabel(
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

/** Resolve a user-facing provider name from a descriptor, id, or both. */
export function resolveBrokerProviderDisplayName(
  value: BrokerProviderNameInput,
  descriptors: readonly BrokerCapabilityDescriptor[] = [],
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

export function logicalCapabilityMarkets(market: string): string[] {
  const normalized = market.trim().toUpperCase();
  if (normalized === "CN") return ["SH", "SZ"];
  return normalized ? [normalized] : [];
}

function normalizedFeatureIDs(value: BrokerFeatureSelector): string[] {
  const values = Array.isArray(value) ? value : [value];
  return [...new Set(values.map((feature) => feature.trim()).filter(Boolean))];
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

function runtimeCapabilityReason(status: BrokerRuntimeCapabilityStatus): string {
  const rawReason =
    status.evaluation?.reason?.trim() || status.capability.reason?.trim() || "";
  if (/[\u3400-\u9fff]/u.test(rawReason)) return rawReason;
  const code =
    status.evaluation?.code?.trim() || status.capability.reasonCode?.trim() || "";
  return localizedRuntimeCapabilityReasons[code] || rawReason || code;
}

function uniqueReasons(values: readonly BrokerCapabilitySummary[]): string[] {
  return [
    ...new Set(values.map((value) => value.reason.trim()).filter(Boolean)),
  ];
}

function aggregateRequired(
  values: readonly BrokerCapabilitySummary[],
  degradedFallback: string,
  unavailableFallback: string,
): BrokerCapabilitySummary {
  if (values.length === 0) {
    return { state: "unavailable", reason: unavailableFallback };
  }
  if (values.every((value) => value.state === "available")) {
    return { state: "available", reason: "" };
  }
  const reasons = uniqueReasons(values.filter((value) => value.state !== "available"));
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
  values: readonly BrokerCapabilitySummary[],
  unavailableFallback: string,
): BrokerCapabilitySummary {
  if (values.some((value) => value.state === "available")) {
    return { state: "available", reason: "" };
  }
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
  runtimeCapabilities: readonly BrokerRuntimeCapabilityStatus[],
): BrokerCapabilitySummary | null {
  const status = runtimeCapabilities.find(
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
  return { state, reason: runtimeCapabilityReason(status) };
}

function staticFeatureState(
  descriptor: BrokerCapabilityDescriptor,
  featureId: string,
  market: string,
): BrokerCapabilitySummary {
  const marketCapability = (descriptor.capabilities ?? []).find(
    (capability) => capability.market.trim().toUpperCase() === market,
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
    return { state: "unavailable", reason: `不支持 ${market} 的此项能力` };
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
  runtimeCapabilities: readonly BrokerRuntimeCapabilityStatus[],
): BrokerCapabilitySummary {
  return (
    runtimeFeatureState(descriptor, featureId, market, runtimeCapabilities) ??
    staticFeatureState(descriptor, featureId, market)
  );
}

function featureStateAcrossMarkets(
  descriptor: BrokerCapabilityDescriptor,
  featureId: string,
  logicalMarket: string,
  runtimeCapabilities: readonly BrokerRuntimeCapabilityStatus[],
): BrokerCapabilitySummary {
  const markets = logicalCapabilityMarkets(logicalMarket);
  if (markets.length === 0) {
    const hasDeclaredFeature = (descriptor.capabilities ?? []).some((capability) =>
      (capability.features ?? []).some((candidate) => candidate.id === featureId),
    );
    const runtimeForBroker = runtimeCapabilities.filter(
      (status) =>
        normalizedID(status.brokerId) === normalizedID(descriptor.id) &&
        status.featureId.trim() === featureId,
    );
    if (!hasDeclaredFeature && runtimeForBroker.length === 0) {
      return { state: "unavailable", reason: "未声明此项能力" };
    }
    const declaredMarkets = [
      ...new Set([
        ...(descriptor.capabilities ?? []).map((value) => value.market),
        ...runtimeForBroker.map((status) => status.market),
      ].map((value) => value.trim().toUpperCase()).filter(Boolean)),
    ];
    return aggregateAlternative(
      declaredMarkets.map((market) =>
        featureStateForMarket(descriptor, featureId, market, runtimeCapabilities),
      ),
      "未声明此项能力",
    );
  }
  const branchStates = markets.map((market) => {
    const state = featureStateForMarket(
      descriptor,
      featureId,
      market,
      runtimeCapabilities,
    );
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
    const featureStates = capabilities.flatMap((capability) => capability.features ?? []);
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
        (candidate) => candidate.market.trim().toUpperCase() === branchMarket,
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

export function featureState(
  descriptor: BrokerCapabilityDescriptor,
  featureSelector: BrokerFeatureSelector,
  market: string,
  runtimeCapabilities: readonly BrokerRuntimeCapabilityStatus[] = [],
): BrokerCapabilitySummary {
  const featureIds = normalizedFeatureIDs(featureSelector);
  if (featureIds.length === 0) return staticReadState(descriptor, market);
  const featureStates = featureIds.map((featureId) => {
    const state = featureStateAcrossMarkets(
      descriptor,
      featureId,
      market,
      runtimeCapabilities,
    );
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
        (capability.features ?? []).some((candidate) => candidate.id === featureId),
      )
      .map((capability) => capability.market.trim().toUpperCase())
      .filter(Boolean);
    return aggregateAlternative(
      declaredMarkets.map((market) => staticFeatureState(descriptor, featureId, market)),
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

export function staticFeatureSummary(
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

export function matchingRuntimeCapabilities(
  descriptor: BrokerCapabilityDescriptor,
  featureSelector: BrokerFeatureSelector,
  market: string,
  runtimeCapabilities: readonly BrokerRuntimeCapabilityStatus[],
): BrokerRuntimeCapabilityStatus[] {
  const featureIDs = normalizedFeatureIDs(featureSelector);
  const markets = logicalCapabilityMarkets(market);
  return runtimeCapabilities.filter((status) => {
    if (normalizedID(status.brokerId) !== normalizedID(descriptor.id)) return false;
    if (featureIDs.length > 0 && !featureIDs.includes(status.featureId.trim())) {
      return false;
    }
    return (
      markets.length === 0 || markets.includes(status.market.trim().toUpperCase())
    );
  });
}

export function capabilityPresentation(
  summary: BrokerCapabilitySummary,
  runtimeStatuses: readonly BrokerRuntimeCapabilityStatus[],
): BrokerCapabilityPresentation {
  if (summary.state === "unavailable") {
    return { displayState: "unavailable", tone: "error" };
  }
  const runtimeStates = runtimeStatuses.map(
    (status) => status.evaluation?.state ?? status.capability.state,
  );
  if (runtimeStates.some((state) => state === "degraded")) {
    return { displayState: "degraded", tone: "warning" };
  }
  if (
    summary.state === "degraded" &&
    runtimeStates.some((state) => state === "unavailable")
  ) {
    return { displayState: "degraded", tone: "warning" };
  }
  return { displayState: "available", tone: "success" };
}

export function descriptorCapabilityPresentation(
  descriptor: BrokerCapabilityDescriptor,
  featureSelector: BrokerFeatureSelector,
  market: string,
  summary: BrokerCapabilitySummary,
  runtimeCapabilities: readonly BrokerRuntimeCapabilityStatus[],
): BrokerCapabilityPresentation {
  return capabilityPresentation(
    summary,
    matchingRuntimeCapabilities(
      descriptor,
      featureSelector,
      market,
      runtimeCapabilities,
    ),
  );
}

export function brokerSupportedChartPeriods(
  brokerId: string,
  market: string,
  descriptors: readonly BrokerCapabilityDescriptor[],
): string[] | null {
  const normalizedBroker = normalizedID(brokerId);
  if (BUILT_IN_MARKET_DATA_PROVIDER_IDS.has(normalizedBroker)) {
    const normalizedMarket = market.trim().toUpperCase();
    return normalizedMarket === "" ||
      new Set(["US", "HK", "CN", "SH", "SZ"]).has(normalizedMarket)
      ? ["1m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo"]
      : [];
  }
  const descriptor = normalizedBroker
    ? descriptors.find((candidate) => normalizedID(candidate.id) === normalizedBroker)
    : descriptors.length === 1
      ? descriptors[0]
      : undefined;
  if (descriptor == null) return null;

  const normalizedMarket = market.trim().toUpperCase();
  const marketCapability = (descriptor.capabilities ?? []).find(
    (capability) => capability.market.trim().toUpperCase() === normalizedMarket,
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

export function brokerProviderOption(
  descriptor: BrokerCapabilityDescriptor,
  featureSelector: BrokerFeatureSelector,
  market: string,
  runtimeCapabilities: readonly BrokerRuntimeCapabilityStatus[],
): BrokerProviderOption {
  const summary = featureState(
    descriptor,
    featureSelector,
    market,
    runtimeCapabilities,
  );
  const selection = staticFeatureSummary(descriptor, featureSelector, market);
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
      featureSelector,
      market,
      summary,
      runtimeCapabilities,
    ),
  };
}
