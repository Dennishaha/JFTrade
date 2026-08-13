import type {
  BrokerFeatureResultDto,
  BrokerProviderAttributionDto,
} from "@/contracts";
import {
  productFeaturePath,
  queryProductFeature,
  type ProductFeatureRequest,
} from "./productFeatureApi";

export type ProductFeatureProvider = BrokerProviderAttributionDto;
export type ProductFeatureResult = BrokerFeatureResultDto;

const preparedRequests = new Map<string, ProductFeatureRequest>();

export function prepareProductFeature(request: ProductFeatureRequest): string {
  const path = productFeaturePath(request);
  preparedRequests.set(path, request);
  return path;
}

// The compatibility call accepts only paths prepared from a typed semantic
// request. It intentionally performs no URL parsing or route inference.
export async function fetchProductFeature(path: string): Promise<ProductFeatureResult> {
  const request = preparedRequests.get(path);
  if (request == null) {
    throw new Error(`Product feature request was not prepared: ${path}`);
  }
  preparedRequests.delete(path);
  return queryProductFeature(request);
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
