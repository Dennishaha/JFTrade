import type {
  MarketDataDepthResponse,
  MarketDataSubscriptionsResponse,
  OrderBookLevelDto,
} from "@/types";
import type {
  MarketDataDepthDto,
  MarketDataInstrumentCandidateDto,
  MarketDataInstrumentResolutionDto,
  MarketDataMarketsDto,
  MarketDataSubscriptionsDto,
} from "@/contracts";

export type MarketInstrumentResolution =
  MarketDataInstrumentResolutionDto;
export type MarketInstrumentCandidate = MarketDataInstrumentCandidateDto & {
  supportedPeriods?: string[];
};
export type MarketProfilesWire =
  MarketDataMarketsDto;
export type MarketSubscriptionsWire =
  MarketDataSubscriptionsDto;

export interface MarketInstrumentReference {
  market: string;
  symbol: string;
  instrumentId: string;
  name: string | null;
  securityType: string | null;
  lotSize: number | null;
  exchange: string | null;
  status: string | null;
  source: string;
  updatedAt: string;
  brokerMappings?: Array<{
    brokerId: string;
    brokerMarket: string;
    brokerSymbol: string;
    brokerInstrumentId: string;
    displayName: string | null;
    source: string;
    updatedAt: string;
  }>;
}

export interface MarketInstrumentReferenceResponse {
  query: string;
  totalReturned: number;
  entries: MarketInstrumentReference[];
}

function recordOrEmpty(value: unknown): Record<string, unknown> {
  return value != null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function finiteNumberOrNull(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

function mapOrderBookLevels(value: unknown): OrderBookLevelDto[] {
  if (!Array.isArray(value)) return [];
  return value.flatMap((raw) => {
    const level = recordOrEmpty(raw);
    const price = finiteNumberOrNull(level.price);
    const volume = finiteNumberOrNull(level.volume);
    const orderCount = finiteNumberOrNull(level.orderCount);
    if (price == null || volume == null || orderCount == null) return [];
    const detailList = Array.isArray(level.detailList)
      ? level.detailList.flatMap((rawDetail) => {
          const detail = recordOrEmpty(rawDetail);
          const orderId = finiteNumberOrNull(detail.orderId);
          const detailVolume = finiteNumberOrNull(detail.volume);
          return orderId == null || detailVolume == null
            ? []
            : [{ orderId, volume: detailVolume }];
        })
      : null;
    return [{ price, volume, orderCount, detailList }];
  });
}

export function mapMarketDataDepthResponse(
  response: MarketDataDepthDto,
): MarketDataDepthResponse {
  const depth = recordOrEmpty(response.depth);
  return {
    request: response.request,
    depth: {
      accountId: String(depth.accountId ?? ""),
      symbol: String(depth.symbol ?? response.request.symbol),
      name: nullableString(depth.name),
      svrRecvTimeBid: nullableString(depth.svrRecvTimeBid),
      svrRecvTimeAsk: nullableString(depth.svrRecvTimeAsk),
      bids: mapOrderBookLevels(depth.bids),
      asks: mapOrderBookLevels(depth.asks),
    },
    meta: response.meta,
  };
}

export function mapMarketInstrumentReferenceResponse(
  response: MarketInstrumentResolution,
): MarketInstrumentReferenceResponse {
  return {
    query: response.query,
    totalReturned: response.totalReturned,
    entries: response.entries.map((entry) => ({
      market: entry.market,
      symbol: entry.symbol,
      instrumentId: entry.instrumentId,
      name: entry.name ?? null,
      securityType: entry.securityType ?? null,
      lotSize: finiteNumberOrNull(entry.lotSize),
      exchange: null,
      status: entry.selectable ? null : entry.unavailableReason ?? null,
      source: entry.source ?? "",
      updatedAt: "",
    })),
  };
}

function nullableNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

function nullableString(value: unknown): string | null {
  return typeof value === "string" ? value : null;
}

function subscriptionBrokerState(
  value: string | undefined,
): "active" | "pending_subscribe" | "pending_unsubscribe" | "retrying" | "unmanaged" | undefined {
  switch (value) {
    case "active":
    case "pending_subscribe":
    case "pending_unsubscribe":
    case "retrying":
    case "unmanaged":
      return value;
    default:
      return undefined;
  }
}

export function mapMarketDataSubscriptions(
  response: MarketSubscriptionsWire,
): MarketDataSubscriptionsResponse {
  const transport = recordOrEmpty(response.transport);
  const brokerState = recordOrEmpty(response.brokerState);
  const brokerEntries = Array.isArray(brokerState.entries)
    ? brokerState.entries.map((value) => {
        const entry = recordOrEmpty(value);
        return {
          key: String(entry.key ?? ""),
          kind: String(entry.kind ?? ""),
          instrumentId: String(entry.instrumentId ?? ""),
          interval: nullableString(entry.interval),
          brokerState: String(entry.brokerState ?? ""),
          subscribedAt: nullableString(entry.subscribedAt),
          unsubscribeEligibleAt: nullableString(entry.unsubscribeEligibleAt),
          lastError: nullableString(entry.lastError),
        };
      })
    : [];

  return {
    totalActiveSubscriptions: response.totalActiveSubscriptions,
    ...(response.consumerId == null ? {} : { consumerId: response.consumerId }),
    ...(response.providerBrokerId == null
      ? {}
      : { providerBrokerId: response.providerBrokerId }),
    ...(response.action == null ? {} : { action: response.action }),
    ...(response.instruments == null
      ? {}
      : { instruments: response.instruments }),
    ...(typeof transport.mode === "string"
      ? { transport: { mode: transport.mode } }
      : {}),
    ...(response.desiredCount == null
      ? {}
      : { desiredCount: response.desiredCount }),
    ...(response.ownActiveCount == null
      ? {}
      : { ownActiveCount: response.ownActiveCount }),
    ...(response.pendingReleaseCount == null
      ? {}
      : { pendingReleaseCount: response.pendingReleaseCount }),
    ...(response.totalUsedQuota == null
      ? {}
      : { totalUsedQuota: response.totalUsedQuota }),
    ...(response.remainQuota == null
      ? {}
      : { remainQuota: response.remainQuota }),
    quota: {
      totalUsed: response.quota.totalUsed,
      totalLimit: response.quota.totalLimit,
      totalRemaining: response.quota.totalRemaining,
      byMarket: response.quota.byMarket.map((bucket) => ({
        market: bucket.market,
        used: bucket.used,
        limit: bucket.limit,
        remaining: bucket.remaining,
      })),
    },
    entries: response.entries.map((entry) => {
      const brokerStateValue = subscriptionBrokerState(entry.brokerState);
      return {
        key: entry.key,
        channel: entry.channel,
        market: entry.market,
        symbol: entry.symbol,
        instrumentId: entry.instrumentId,
        interval: entry.interval,
        depthLevel: entry.depthLevel,
        consumers: entry.consumers,
        refCount: entry.refCount,
        createdAt: entry.createdAt,
        updatedAt: entry.updatedAt,
        ...(brokerStateValue == null
          ? {}
          : { brokerState: brokerStateValue }),
        ...(entry.subscribedAt == null
          ? {}
          : { subscribedAt: entry.subscribedAt }),
        ...(entry.unsubscribeEligibleAt == null
          ? {}
          : { unsubscribeEligibleAt: entry.unsubscribeEligibleAt }),
        ...(entry.lastError == null ? {} : { lastError: entry.lastError }),
      };
    }),
    ...(Object.keys(brokerState).length === 0
      ? {}
      : {
          brokerState: {
            desiredCount: Number(brokerState.desiredCount ?? 0),
            ownActiveCount: Number(brokerState.ownActiveCount ?? 0),
            pendingReleaseCount: Number(brokerState.pendingReleaseCount ?? 0),
            totalUsedQuota: nullableNumber(brokerState.totalUsedQuota),
            remainQuota: nullableNumber(brokerState.remainQuota),
            ...(typeof brokerState.ownUsedQuota === "number"
              ? { ownUsedQuota: brokerState.ownUsedQuota }
              : {}),
            ...(brokerState.checkedAt === undefined
              ? {}
              : { checkedAt: nullableString(brokerState.checkedAt) }),
            ...(brokerState.reconciledAt === undefined
              ? {}
              : { reconciledAt: nullableString(brokerState.reconciledAt) }),
            ...(brokerState.lastError === undefined
              ? {}
              : { lastError: nullableString(brokerState.lastError) }),
            entries: brokerEntries,
          },
        }),
  };
}
