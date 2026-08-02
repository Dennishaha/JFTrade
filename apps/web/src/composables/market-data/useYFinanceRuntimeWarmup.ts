import { computed, onBeforeUnmount, watch, type Ref } from "vue";

import type { MarketDataProviderStatusDto } from "@/contracts";
import type { MarketDataProviderID } from "@/composables/settings/marketDataProviderSettings";

type YFinanceRuntimeReadiness = "" | "warming" | "ready" | "failed";

export function useYFinanceRuntimeWarmup(options: {
  providerID: Ref<MarketDataProviderID | null>;
  status: Ref<MarketDataProviderStatusDto | null>;
  refresh: () => Promise<void>;
  retryAfterMs?: number;
}) {
  const readiness = computed<YFinanceRuntimeReadiness>(() => {
    if (options.providerID.value !== "yfinance") return "";
    const value = (options.status.value?.health as { readiness?: unknown } | null)
      ?.readiness;
    return value === "warming" || value === "ready" || value === "failed"
      ? value
      : "";
  });
  let timer: ReturnType<typeof setTimeout> | null = null;
  let stopped = false;

  function schedule(): void {
    if (timer != null) clearTimeout(timer);
    timer = null;
    if (stopped || readiness.value !== "warming") return;
    timer = setTimeout(() => {
      timer = null;
      void options.refresh().finally(schedule);
    }, options.retryAfterMs ?? 1_000);
  }

  watch([options.providerID, readiness], schedule, { immediate: true });
  onBeforeUnmount(() => {
    stopped = true;
    if (timer != null) clearTimeout(timer);
  });

  return { readiness };
}
