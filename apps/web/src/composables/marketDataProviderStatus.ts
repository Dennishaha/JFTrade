import { computed, ref } from "vue";

import type { MarketDataProviderStatusResponse } from "@/contracts";

import { fetchEnvelope } from "./apiClient";

const providerStatus = ref<MarketDataProviderStatusResponse | null>(null);
const isLoadingProviderStatus = ref(false);
const providerStatusError = ref("");
let providerStatusPromise: Promise<MarketDataProviderStatusResponse | null> | null = null;

export function useMarketDataProviderStatus() {
  async function loadMarketDataProviderStatus(options: { force?: boolean } = {}) {
    if (!options.force && providerStatus.value != null) {
      return providerStatus.value;
    }
    if (providerStatusPromise != null) {
      return providerStatusPromise;
    }

    isLoadingProviderStatus.value = true;
    providerStatusError.value = "";
    providerStatusPromise = fetchEnvelope<MarketDataProviderStatusResponse>(
      "/api/v1/market-data/provider",
    )
      .then((response) => {
        providerStatus.value = response;
        return response;
      })
      .catch((error: unknown) => {
        providerStatusError.value = error instanceof Error ? error.message : String(error);
        return null;
      })
      .finally(() => {
        isLoadingProviderStatus.value = false;
        providerStatusPromise = null;
      });

    return providerStatusPromise;
  }

  const providerDisplayName = computed(() =>
    providerStatus.value?.descriptor?.displayName?.trim() || "",
  );
  const providerCapabilitySummary = computed(() => {
    const capabilities = providerStatus.value?.descriptor?.capabilities;
    if (capabilities == null) return "";
    const labels = [
      capabilities.streamingQuotes ? "推送报价" : "",
      capabilities.historicalCandles ? "历史K线" : "",
      capabilities.orderBookDepth ? "盘口" : "",
      capabilities.extendedHours ? "扩展时段" : "",
    ].filter(Boolean);
    return labels.join(" / ");
  });

  return {
    isLoadingProviderStatus,
    loadMarketDataProviderStatus,
    providerCapabilitySummary,
    providerDisplayName,
    providerStatus,
    providerStatusError,
  };
}
