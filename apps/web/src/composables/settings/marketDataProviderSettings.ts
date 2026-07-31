import type {
  MarketDataProviderSettingsResponse as MarketDataProviderSettingsResponseDto,
  MarketDataProviderStatusDto,
} from "@/contracts";

import { apiGet, apiPut } from "@/composables/shared/apiClient";

export type MarketDataProviderID = "futu" | "yfinance";

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
    await apiPut("/api/v1/settings/market-data-provider", { activeProvider }),
  );
}

export async function getMarketDataProviderStatus(): Promise<MarketDataProviderStatusDto> {
  return apiGet("/api/v1/market-data/provider");
}

function normalizeProviderSettings(
  settings: MarketDataProviderSettingsResponseDto,
): MarketDataProviderSettings {
  if (settings?.activeProvider === "futu" || settings?.activeProvider === "yfinance") {
    return { activeProvider: settings.activeProvider };
  }
  throw new Error("服务端返回了不支持的行情提供者");
}
