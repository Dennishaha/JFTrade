// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick, ref } from "vue";

import {
  getSharedLiveSocketHub,
  resetSharedLiveSocketHubForTests,
} from "../src/composables/sharedLiveSocket";
import { provideWorkspaceTradingPreferencesStore } from "../src/composables/useWorkspaceLayout";

const marketDataSnapshot = ref<any>(null);
const marketSecurityDetails = ref<any>(null);
const mocks = vi.hoisted(() => ({
  acquire: vi.fn(),
  fetchEnvelope: vi.fn(),
  fetchEnvelopeWithInit: vi.fn(),
  heartbeat: vi.fn(),
  release: vi.fn(),
}));

vi.mock("../src/composables/apiClient", () => ({
  fetchEnvelope: (...args: unknown[]) => mocks.fetchEnvelope(...args),
  fetchEnvelopeWithInit: (...args: unknown[]) => mocks.fetchEnvelopeWithInit(...args),
}));

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => ({
    currentMarketDataSnapshot: marketDataSnapshot,
    currentMarketSecurityDetails: marketSecurityDetails,
    acquireMarketDataSubscription: (...args: unknown[]) => mocks.acquire(...args),
    createStableWebConsumerId: () => "web:workspace-depth:window:lifecycle",
    heartbeatMarketDataConsumer: (...args: unknown[]) => mocks.heartbeat(...args),
    releaseMarketDataSubscription: (...args: unknown[]) => mocks.release(...args),
  }),
}));

import OrderBookPanel from "../src/components/workspace/OrderBookPanel.vue";

function mountPanel() {
  const Host = defineComponent({
    setup() {
      const store = provideWorkspaceTradingPreferencesStore();
      store.update({ market: "US", symbol: "TME", period: "1m" });
      return () => h(OrderBookPanel);
    },
  });
  return mount(Host, {
    global: {
      stubs: {
        InstrumentIdentity: true,
        MarketFeedStatus: true,
        OrderBookDepthTable: true,
      },
    },
  });
}

function setupValue<T>(wrapper: ReturnType<typeof mountPanel>, key: string): T {
  const state = wrapper.findComponent(OrderBookPanel).vm.$.setupState as Record<string, unknown>;
  const value = state[key];
  return value != null && typeof value === "object" && "value" in value
    ? (value as { value: T }).value
    : value as T;
}

function callSetup<T>(wrapper: ReturnType<typeof mountPanel>, key: string, ...args: unknown[]): T {
  const method = (wrapper.findComponent(OrderBookPanel).vm.$.setupState as Record<string, unknown>)[key] as (...input: unknown[]) => T;
  return method(...args);
}

async function flushPanel(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await nextTick();
  await Promise.resolve();
  await nextTick();
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
}

describe("OrderBookPanel subscription lifecycle boundaries", () => {
  beforeEach(() => {
    resetSharedLiveSocketHubForTests();
    vi.useRealTimers();
    for (const mock of Object.values(mocks)) mock.mockReset();
    mocks.acquire.mockResolvedValue(true);
    mocks.heartbeat.mockResolvedValue(undefined);
    mocks.release.mockResolvedValue(undefined);
    mocks.fetchEnvelope.mockResolvedValue({
      descriptor: { capabilities: [{ readFeatures: { orderBook: { defaultNum: 10 } } }] },
    });
    mocks.fetchEnvelopeWithInit.mockResolvedValue({
      request: { market: "US", symbol: "TME", instrumentId: "US.TME", num: 10 },
      depth: { symbol: "US.TME", bids: [], asks: [] },
      meta: { instrumentId: "US.TME", source: "futu", resolvedAt: "2026-07-01T00:00:00Z", fromCache: false },
    });
    marketDataSnapshot.value = null;
    marketSecurityDetails.value = null;
  });

  afterEach(() => {
    resetSharedLiveSocketHubForTests();
    vi.useRealTimers();
  });

  it("surfaces an entitlement failure without opening a depth request or live target", async () => {
    mocks.acquire.mockRejectedValueOnce(new Error("depth entitlement missing"));
    const wrapper = mountPanel();
    await flushPanel();

    expect(setupValue<string>(wrapper, "depthError")).toBe("depth entitlement missing");
    expect(mocks.fetchEnvelopeWithInit).not.toHaveBeenCalled();
    expect(getSharedLiveSocketHub().snapshotSubscriptions().depth).toEqual([]);
    wrapper.unmount();
  });

  it("does not reconnect when the operator selects the already-active depth preset", async () => {
    const wrapper = mountPanel();
    await flushPanel();
    const initialDepthRequests = mocks.fetchEnvelopeWithInit.mock.calls.length;

    callSetup<void>(wrapper, "setDepthNum", 10);
    await flushPanel();

    expect(mocks.fetchEnvelopeWithInit).toHaveBeenCalledTimes(initialDepthRequests);
    expect(mocks.acquire).toHaveBeenCalledTimes(1);
    wrapper.unmount();
  });

  it("keeps a held order-book lease alive while the panel remains mounted", async () => {
    vi.useFakeTimers();
    const wrapper = mountPanel();
    await flushPanel();
    mocks.heartbeat.mockClear();

    await vi.advanceTimersByTimeAsync(15_000);

    expect(mocks.heartbeat).toHaveBeenCalledWith("web:workspace-depth:window:lifecycle");
    wrapper.unmount();
  });

  it("keeps quote-derived values absent until either a snapshot or security detail exists", async () => {
    const wrapper = mountPanel();
    await flushPanel();

    expect(setupValue(wrapper, "snapshot")).toBeNull();
    expect(setupValue(wrapper, "security")).toBeNull();
    expect(setupValue(wrapper, "bidPrice")).toBeNull();
    expect(setupValue(wrapper, "askPrice")).toBeNull();
    expect(setupValue(wrapper, "changeFromClose")).toBeNull();
    expect(setupValue(wrapper, "changePercent")).toBeNull();
    wrapper.unmount();
  });

  it("releases a late lease acquired after the panel was already unmounted", async () => {
    const lateAcquire = deferred<boolean>();
    mocks.acquire.mockReturnValueOnce(lateAcquire.promise);
    const wrapper = mountPanel();
    await Promise.resolve();
    await nextTick();
    wrapper.unmount();

    lateAcquire.resolve(true);
    await flushPanel();

    expect(mocks.release).toHaveBeenCalledWith({
      consumerId: "web:workspace-depth:window:lifecycle",
      market: "US",
      symbol: "TME",
      channel: "ORDER_BOOK",
      keepalive: true,
    });
  });
});
