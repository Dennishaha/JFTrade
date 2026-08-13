import type { BrokerFeatureResultDto } from "@/contracts";

import { apiGetPath, type ApiGetPath } from "@/composables/shared/apiClient";

export type MarketFeatureResource =
  | "price-alerts"
  | "option-alerts"
  | "remote-watchlists"
  | "instrument-profile"
  | "intraday"
  | "ticks"
  | "broker-queue"
  | "capital-flow"
  | "option-chains"
  | "option-expirations"
  | "option-screens"
  | "option-analysis"
  | "option-events"
  | "warrants"
  | "futures"
  | "news";

export interface MarketFeatureRequest {
  scope: "market-feature";
  resource: MarketFeatureResource;
  brokerId?: string;
  accountId?: string;
  tradingEnvironment?: string;
  market?: string;
  instrumentId?: string;
  code?: string;
  underlying?: string;
  underlyingProductClass?: string;
  operation?: string;
  sellerStrategy?: string;
  optionStrategy?: string;
  beginTime?: string;
  endTime?: string;
  cursor?: string;
  pageSize?: number;
  refresh?: boolean;
}

const fixedRoutes = {
  "price-alerts": "/api/v1/alerts/price",
  "option-alerts": "/api/v1/alerts/option-events",
  "remote-watchlists": "/api/v1/watchlists/remote",
  "option-screens": "/api/v1/market-data/options/screens",
  "option-events": "/api/v1/market-data/options/events",
  warrants: "/api/v1/market-data/warrants",
  futures: "/api/v1/market-data/futures",
  news: "/api/v1/market-data/news",
} as const satisfies Partial<Record<MarketFeatureResource, ApiGetPath>>;

const instrumentRoutes = {
  "instrument-profile": [
    "/api/v1/market-data/instruments/{instrumentId}/profile",
    "instruments",
    "profile",
  ],
  intraday: ["/api/v1/market-data/intraday/{instrumentId}", "intraday", ""],
  ticks: ["/api/v1/market-data/ticks/{instrumentId}", "ticks", ""],
  "broker-queue": [
    "/api/v1/market-data/broker-queue/{instrumentId}",
    "broker-queue",
    "",
  ],
  "capital-flow": [
    "/api/v1/market-data/capital-flow/{instrumentId}",
    "capital-flow",
    "",
  ],
  "option-chains": [
    "/api/v1/market-data/options/chains/{instrumentId}",
    "options/chains",
    "",
  ],
  "option-expirations": [
    "/api/v1/market-data/options/expirations/{instrumentId}",
    "options/expirations",
    "",
  ],
  "option-analysis": [
    "/api/v1/market-data/options/analysis/{instrumentId}",
    "options/analysis",
    "",
  ],
} as const satisfies Partial<
  Record<MarketFeatureResource, readonly [ApiGetPath, string, string]>
>;

function queryString(request: MarketFeatureRequest): string {
  const params = new URLSearchParams();
  const values: Array<[string, string | number | boolean | undefined]> = [
    ["accountId", request.accountId],
    ["tradingEnvironment", request.tradingEnvironment],
    ["market", request.market],
    ["code", request.code],
    ["underlying", request.underlying],
    ["underlyingProductClass", request.underlyingProductClass],
    ["operation", request.operation],
    ["sellerStrategy", request.sellerStrategy],
    ["option_strategy", request.optionStrategy],
    ["beginTime", request.beginTime],
    ["endTime", request.endTime],
    ["cursor", request.cursor],
    ["pageSize", request.pageSize],
    ["refresh", request.refresh ? true : undefined],
    ["brokerId", request.brokerId],
  ];
  for (const [key, value] of values) {
    if (value !== undefined && String(value).trim() !== "") {
      params.set(key, String(value));
    }
  }
  const query = params.toString();
  return query ? `?${query}` : "";
}

export function marketFeatureTarget(request: MarketFeatureRequest): {
  template: ApiGetPath;
  path: string;
} {
  const fixed = fixedRoutes[request.resource as keyof typeof fixedRoutes];
  if (fixed != null) {
    return { template: fixed, path: `${fixed}${queryString(request)}` };
  }
  const dynamic = instrumentRoutes[
    request.resource as keyof typeof instrumentRoutes
  ];
  const instrumentId = request.instrumentId?.trim();
  if (dynamic == null || !instrumentId) {
    throw new Error(`Market feature ${request.resource} requires instrumentId`);
  }
  const [, group, suffix] = dynamic;
  const encoded = encodeURIComponent(instrumentId);
  const path = `/api/v1/market-data/${group}/${encoded}${suffix ? `/${suffix}` : ""}${queryString(request)}`;
  return { template: dynamic[0], path };
}

export const marketFeatureApi = {
  async query(request: MarketFeatureRequest): Promise<BrokerFeatureResultDto> {
    const target = marketFeatureTarget(request);
    return apiGetPath(target.template, target.path) as Promise<BrokerFeatureResultDto>;
  },
};
