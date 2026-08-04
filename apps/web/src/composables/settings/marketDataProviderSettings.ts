import type {
  MarketDataProviderSettingsResponse as MarketDataProviderSettingsResponseDto,
  MarketDataProviderStatusDto,
} from "@/contracts";
import type { FutuOpenDHealthResponse } from "@/types";

import { mapFutuOpenDHealth } from "@/composables/market-data/futuOpenDContract";
import { apiGet, apiPut } from "@/composables/shared/apiClient";

export type MarketDataProviderID = "futu" | "yfinance" | "akshare";

export interface MarketDataProviderSettings {
  activeProvider: MarketDataProviderID;
}

export async function getMarketDataProviderSettings(): Promise<MarketDataProviderSettings> {
  return normalizeProviderSettings(
    await apiGet("/api/v1/settings/market-data-provider"),
  );
}

export async function putMarketDataProviderSettings(
  activeProvider: MarketDataProviderID,
): Promise<MarketDataProviderSettings> {
  return normalizeProviderSettings(
    await apiPut(
      "/api/v1/settings/market-data-provider",
      { activeProvider },
    ),
  );
}

export async function getMarketDataProviderStatus(): Promise<MarketDataProviderStatusDto> {
  return apiGet("/api/v1/market-data/provider");
}

export async function getFutuOpenDHealth(): Promise<FutuOpenDHealthResponse> {
  return mapFutuOpenDHealth(
    await apiGet("/api/v1/system/futu-opend"),
  );
}

function normalizeProviderSettings(
  settings: MarketDataProviderSettingsResponseDto,
): MarketDataProviderSettings {
  const activeProvider = (settings as { activeProvider?: unknown } | null)
    ?.activeProvider;
  if (
    activeProvider === "futu" ||
    activeProvider === "yfinance" ||
    activeProvider === "akshare"
  ) {
    return { activeProvider };
  }
  throw new Error("服务端返回了不支持的行情提供者");
}
