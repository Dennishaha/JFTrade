import { fetchEnvelope } from "./apiClient";

export interface ProductFeatureProvider {
  brokerId: string;
  securityFirm?: string;
  featureId: string;
  capability: "available" | "degraded" | "unavailable";
  selectionReason: string;
  resolvedAt: string;
  asOf: string;
}

export interface ProductFeatureResult {
  provider: ProductFeatureProvider;
  resolvedInstrument?: {
    instrumentId?: string;
    code?: string;
    quoteMarket?: string;
    tradeMarket?: string;
    name?: string;
    productClass?: string;
    marketSegment?: string;
  };
  asOf: string;
  entries: Record<string, unknown>[];
  nextCursor?: string;
  hasMore?: boolean;
  total?: number;
  warnings?: string[];
  partialErrors?: Array<{ scope: string; code: string; message: string }>;
  metadata?: Record<string, unknown>;
}

export function fetchProductFeature(path: string): Promise<ProductFeatureResult> {
  return fetchEnvelope<ProductFeatureResult>(path);
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
