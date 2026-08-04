import type { BrokerProviderOption } from "@/composables/trading/brokerProviderSelection";
import type { PythonMarketDataProviderID } from "@/composables/market-data/usePythonMarketDataRuntimeWarmup";

export const embeddedPythonMarketDataFeatureIDs = new Set([
  "market.search",
  "market.instrument_profile",
  "market.snapshot",
  "market.snapshots",
  "market.candles",
]);

const supportedMarkets = new Set(["US", "HK", "CN", "SH", "SZ"]);

export function pythonMarketDataProviderName(
  providerID: PythonMarketDataProviderID,
): string {
  return providerID === "akshare" ? "AKShare" : "Yahoo";
}

export function embeddedPythonMarketDataProviderOption(
  providerID: PythonMarketDataProviderID,
  market: string,
): BrokerProviderOption {
  const name = pythonMarketDataProviderName(providerID);
  const available = market === "" || supportedMarkets.has(market.trim().toUpperCase());
  return {
    id: providerID,
    label: name,
    shortLabel: name,
    securityFirm: "内置行情查询",
    state: available ? "degraded" : "unavailable",
    displayState: available ? "available" : "unavailable",
    tone: available ? "success" : "error",
    selectable: available,
    reason: available
      ? providerID === "akshare"
        ? "延迟 HTTP 轮询，不支持实时推流或 Level 2"
        : "非实时快照查询，不支持实时推流或 Level 2"
      : `当前标的市场不在内置 ${name} 支持范围`,
  };
}

export function statusMatchesPythonMarketDataProvider(
  providerID: PythonMarketDataProviderID,
  statusProviderID: string,
): boolean {
  const normalized = statusProviderID.trim().toLowerCase();
  return providerID === "yfinance"
    ? normalized === "yfinance" || normalized === "yahoo-finance"
    : normalized === "akshare";
}
