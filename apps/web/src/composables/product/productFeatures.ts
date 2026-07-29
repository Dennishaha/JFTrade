import type {
  BrokerFeatureResultDto,
  BrokerProviderAttributionDto,
} from "@/contracts";

import {
  apiGetPath,
  type ApiGetPath,
} from "@/composables/shared/apiClient";

export type ProductFeatureProvider = BrokerProviderAttributionDto;
export type ProductFeatureResult = BrokerFeatureResultDto;

const productFeatureRoutes = [
  [/^\/api\/v1\/alerts\/option-events$/, "/api/v1/alerts/option-events"],
  [/^\/api\/v1\/alerts\/price$/, "/api/v1/alerts/price"],
  [/^\/api\/v1\/market-data\/futures$/, "/api/v1/market-data/futures"],
  [/^\/api\/v1\/market-data\/news$/, "/api/v1/market-data/news"],
  [/^\/api\/v1\/market-data\/options\/events$/, "/api/v1/market-data/options/events"],
  [/^\/api\/v1\/market-data\/options\/screens$/, "/api/v1/market-data/options/screens"],
  [/^\/api\/v1\/market-data\/prediction\/categories$/, "/api/v1/market-data/prediction/categories"],
  [/^\/api\/v1\/market-data\/prediction\/combos\/eligible-events$/, "/api/v1/market-data/prediction/combos/eligible-events"],
  [/^\/api\/v1\/market-data\/prediction\/competitions$/, "/api/v1/market-data/prediction/competitions"],
  [/^\/api\/v1\/market-data\/prediction\/events$/, "/api/v1/market-data/prediction/events"],
  [/^\/api\/v1\/market-data\/prediction\/series$/, "/api/v1/market-data/prediction/series"],
  [/^\/api\/v1\/market-data\/warrants$/, "/api/v1/market-data/warrants"],
  [/^\/api\/v1\/research\/calendars$/, "/api/v1/research/calendars"],
  [/^\/api\/v1\/research\/industries$/, "/api/v1/research/industries"],
  [/^\/api\/v1\/research\/institutions$/, "/api/v1/research/institutions"],
  [/^\/api\/v1\/research\/macro$/, "/api/v1/research/macro"],
  [/^\/api\/v1\/research\/rankings$/, "/api/v1/research/rankings"],
  [/^\/api\/v1\/research\/screens$/, "/api/v1/research/screens"],
  [/^\/api\/v1\/watchlists\/remote$/, "/api/v1/watchlists/remote"],
  [/^\/api\/v1\/market-data\/broker-queue\/[^/]+$/, "/api/v1/market-data/broker-queue/{instrumentId}"],
  [/^\/api\/v1\/market-data\/capital-flow\/[^/]+$/, "/api/v1/market-data/capital-flow/{instrumentId}"],
  [/^\/api\/v1\/market-data\/instruments\/[^/]+\/profile$/, "/api/v1/market-data/instruments/{instrumentId}/profile"],
  [/^\/api\/v1\/market-data\/intraday\/[^/]+$/, "/api/v1/market-data/intraday/{instrumentId}"],
  [/^\/api\/v1\/market-data\/options\/analysis\/[^/]+$/, "/api/v1/market-data/options/analysis/{instrumentId}"],
  [/^\/api\/v1\/market-data\/options\/chains\/[^/]+$/, "/api/v1/market-data/options/chains/{instrumentId}"],
  [/^\/api\/v1\/market-data\/options\/expirations\/[^/]+$/, "/api/v1/market-data/options/expirations/{instrumentId}"],
  [/^\/api\/v1\/market-data\/ticks\/[^/]+$/, "/api/v1/market-data/ticks/{instrumentId}"],
  [/^\/api\/v1\/market-data\/prediction\/events\/[^/]+\/contracts$/, "/api/v1/market-data/prediction/events/{eventId}/contracts"],
  [/^\/api\/v1\/market-data\/prediction\/contracts\/[^/]+\/candles$/, "/api/v1/market-data/prediction/contracts/{code}/candles"],
  [/^\/api\/v1\/market-data\/prediction\/contracts\/[^/]+\/candles\/history$/, "/api/v1/market-data/prediction/contracts/{code}/candles/history"],
  [/^\/api\/v1\/market-data\/prediction\/contracts\/[^/]+\/milestones$/, "/api/v1/market-data/prediction/contracts/{code}/milestones"],
  [/^\/api\/v1\/market-data\/prediction\/contracts\/[^/]+\/order-book$/, "/api/v1/market-data/prediction/contracts/{code}/order-book"],
  [/^\/api\/v1\/market-data\/prediction\/contracts\/[^/]+\/snapshot$/, "/api/v1/market-data/prediction/contracts/{code}/snapshot"],
  [/^\/api\/v1\/market-data\/prediction\/contracts\/[^/]+\/ticks$/, "/api/v1/market-data/prediction/contracts/{code}/ticks"],
  [/^\/api\/v1\/research\/analyst\/[^/]+$/, "/api/v1/research/analyst/{instrumentId}"],
  [/^\/api\/v1\/research\/corporate-actions\/[^/]+$/, "/api/v1/research/corporate-actions/{instrumentId}"],
  [/^\/api\/v1\/research\/financials\/[^/]+$/, "/api/v1/research/financials/{instrumentId}"],
  [/^\/api\/v1\/research\/instruments\/[^/]+$/, "/api/v1/research/instruments/{instrumentId}"],
  [/^\/api\/v1\/research\/ownership\/[^/]+$/, "/api/v1/research/ownership/{instrumentId}"],
  [/^\/api\/v1\/research\/short-interest\/[^/]+$/, "/api/v1/research/short-interest/{instrumentId}"],
  [/^\/api\/v1\/research\/technical-indicators\/[^/]+$/, "/api/v1/research/technical-indicators/{instrumentId}"],
  [/^\/api\/v1\/research\/valuation\/[^/]+$/, "/api/v1/research/valuation/{instrumentId}"],
] as const satisfies ReadonlyArray<readonly [RegExp, ApiGetPath]>;

export async function fetchProductFeature(path: string): Promise<ProductFeatureResult> {
  const pathname = new URL(path, "http://jftrade.local").pathname;
  const route = productFeatureRoutes.find(([pattern]) => pattern.test(pathname));
  if (route === undefined) {
    throw new Error(`Unsupported product feature endpoint: ${pathname}`);
  }
  return apiGetPath(route[1], path) as Promise<ProductFeatureResult>;
}

export function instrumentIDFromFeatureEntry(
  entry: Record<string, unknown>,
): string | null {
  const direct = [
    entry.instrumentId,
    entry.code,
    entry.securityCode,
    entry.stockCode,
    entry.contractCode,
  ];
  for (const value of direct) {
    if (typeof value === "string" && value.includes(".")) {
      return value.toUpperCase();
    }
  }
  const security = entry.security;
  if (security != null && typeof security === "object") {
    const market = String((security as Record<string, unknown>).market ?? "");
    const code = String((security as Record<string, unknown>).code ?? "");
    if (market && code) return `${market}.${code}`.toUpperCase();
  }
  return null;
}

export function featureEntryTitle(
  entry: Record<string, unknown>,
  index: number,
): string {
  for (const key of [
    "name",
    "title",
    "eventName",
    "seriesName",
    "code",
    "instrumentId",
  ]) {
    const value = entry[key];
    if (typeof value === "string" && value.trim()) return value;
  }
  return `结果 ${index + 1}`;
}
