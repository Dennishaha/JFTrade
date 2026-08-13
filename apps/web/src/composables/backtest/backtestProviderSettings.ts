import { apiGet, apiPut } from "@/composables/shared/apiClient";

export type BacktestProviderID = "futu" | "yfinance" | "akshare";

export interface BacktestProviderCapabilities {
  historicalCandles: boolean;
  streamingCandles: boolean;
  extendedHours: boolean;
  candleIntervals: string[];
  priceAdjustments: string[];
  historicalLookbackDays?: Record<string, number>;
}

export interface BacktestProviderDescriptor {
  selectionId: BacktestProviderID;
  providerId: string;
  displayName: string;
  capabilities: BacktestProviderCapabilities;
}

export interface BacktestProviderSettings {
  activeProvider: BacktestProviderID;
  availableProviders: BacktestProviderDescriptor[];
}

export function resolveHistoricalLookbackDays(
  capabilities: BacktestProviderCapabilities | null,
  market: string,
  interval: string,
): number | null {
  const limits = capabilities?.historicalLookbackDays;
  if (limits == null) return null;
  const normalizedInterval = interval.trim();
  const marketKey = `${market.trim().toUpperCase()}:${normalizedInterval}`;
  const days = limits[marketKey] ?? limits[normalizedInterval];
  return typeof days === "number" && Number.isFinite(days) && days > 0
    ? days
    : null;
}

export function backtestHistoricalRangeError(
  provider: BacktestProviderDescriptor | null,
  market: string,
  interval: string,
  startDate: string,
  now = new Date(),
): string {
  const days = resolveHistoricalLookbackDays(
    provider?.capabilities ?? null,
    market,
    interval,
  );
  if (provider == null || days == null || startDate.trim() === "") return "";
  const start = new Date(`${startDate.trim()}T00:00:00Z`);
  if (!Number.isFinite(start.getTime())) return "";
  const earliest = new Date(now.getTime() - days * 24 * 60 * 60 * 1000);
  if (start.getTime() >= earliest.getTime()) return "";
  return `${provider.displayName} 的 ${market.trim().toUpperCase()} ${interval} 历史数据仅提供最近 ${days} 天；当前起始日期超出供应商窗口。请缩短日期范围或改用日线。`;
}

function normalizeSettings(value: unknown): BacktestProviderSettings {
  const raw = value as Partial<BacktestProviderSettings> | null;
  const activeProvider = raw?.activeProvider;
  if (activeProvider !== "futu" && activeProvider !== "yfinance" && activeProvider !== "akshare") {
    throw new Error("服务端返回了不支持的回测行情提供者");
  }
  return {
    activeProvider,
    availableProviders: Array.isArray(raw?.availableProviders)
      ? raw.availableProviders
      : [],
  };
}

export async function getBacktestProviderSettings(): Promise<BacktestProviderSettings> {
  return normalizeSettings(await apiGet("/api/v1/settings/backtest-market-data-provider"));
}

export async function putBacktestProviderSettings(
  activeProvider: BacktestProviderID,
): Promise<BacktestProviderSettings> {
  return normalizeSettings(await apiPut(
    "/api/v1/settings/backtest-market-data-provider",
    { activeProvider },
  ));
}
