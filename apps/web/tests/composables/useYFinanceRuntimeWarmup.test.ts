// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h, ref } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { MarketDataProviderStatusDto } from "@/contracts";
import { usePythonMarketDataRuntimeWarmup } from "@/composables/market-data/usePythonMarketDataRuntimeWarmup";
import { useYFinanceRuntimeWarmup } from "@/composables/market-data/useYFinanceRuntimeWarmup";

afterEach(() => vi.useRealTimers());

describe("Yahoo runtime warmup refresh", () => {
  it("retries after one second and stops when the runtime becomes ready", async () => {
    vi.useFakeTimers();
    const providerID = ref<"yfinance" | "futu">("yfinance");
    const status = ref(providerStatus("warming"));
    const refresh = vi.fn(async () => {
      status.value = providerStatus("ready");
    });
    const harness = defineComponent({
      setup() {
        const { readiness } = useYFinanceRuntimeWarmup({
          providerID,
          status,
          refresh,
        });
        return () => h("span", readiness.value);
      },
    });
    const wrapper = mount(harness);

    await vi.advanceTimersByTimeAsync(999);
    expect(refresh).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    await flushPromises();
    expect(refresh).toHaveBeenCalledOnce();
    expect(wrapper.text()).toBe("ready");

    await vi.advanceTimersByTimeAsync(5_000);
    expect(refresh).toHaveBeenCalledOnce();
    wrapper.unmount();
  });

  it("applies the same isolated warmup polling to AKShare", async () => {
    vi.useFakeTimers();
    const providerID = ref<"akshare" | "futu">("akshare");
    const status = ref(providerStatus("warming", "akshare"));
    const refresh = vi.fn(async () => {
      status.value = providerStatus("ready", "akshare");
    });
    const harness = defineComponent({
      setup() {
        const { readiness } = usePythonMarketDataRuntimeWarmup({
          providerID,
          status,
          refresh,
        });
        return () => h("span", readiness.value);
      },
    });
    const wrapper = mount(harness);

    await vi.advanceTimersByTimeAsync(1_000);
    await flushPromises();
    expect(refresh).toHaveBeenCalledOnce();
    expect(wrapper.text()).toBe("ready");

    providerID.value = "futu";
    await flushPromises();
    expect(wrapper.text()).toBe("");
    wrapper.unmount();
  });
});

function providerStatus(
  readiness: "warming" | "ready",
  providerID: "yfinance" | "akshare" = "yfinance",
): MarketDataProviderStatusDto {
  return {
    descriptor: {
      providerId: providerID === "yfinance" ? "yahoo-finance" : "akshare",
      brokerId: providerID,
      displayName: providerID === "yfinance" ? "Yahoo" : "AKShare",
      securityFirm: "Yahoo Finance",
      supportedMarkets: ["US"],
      transports: ["snapshot-poll-delayed"],
      capabilities: {},
      constraints: {},
    },
    health: {
      connected: true,
      activeCount: 0,
      streamMode: "snapshot-poll-delayed",
      readiness,
    },
    runtime: {
      state: "running",
      generation: 1,
      reconnectAttempts: 0,
      demandCount: 0,
      demandKeys: [],
      fallbackPollCount: 0,
      fallbackPollKeys: [],
    },
    subscriptions: { entries: [], demandCount: 0 },
    checkedAt: "2026-08-02T00:00:00Z",
  };
}
