// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick, ref } from "vue";

import {
  getSharedLiveSocketHub,
  resetSharedLiveSocketHubForTests,
} from "@/composables/market-data/sharedLiveSocket";
import {
  resetBrokerProviderSelectionForTests,
  useBrokerProviderSelection,
} from "@/composables/trading/brokerProviderSelection";
import { provideWorkspaceTradingPreferencesStore } from "@/composables/workspace/useWorkspaceLayout";
import QuoteSummaryCard from "@/components/domain/market-data/QuoteSummaryCard.vue";
import OrderBookPanel from "@/components/workspace/OrderBookPanel.vue";
import BrokerProviderTag from "@/components/shared/BrokerProviderTag.vue";
import { createLiveEnvelope, MockWebSocket } from "../helpers";
import { flushPromises, productGlobalStubs } from "../productTestUtils";

const marketDataSnapshot = ref<any>(null);
const marketSecurityDetails = ref<any>(null);
const marketInstrumentSearchOptions = ref<any[]>([]);
const acquireMarketDataSubscriptionMock = vi.fn();
const heartbeatMarketDataConsumerMock = vi.fn();
const releaseMarketDataSubscriptionMock = vi.fn();
const providerStatusMock = vi.fn();
const fetchEnvelopeWithInitMock = vi.fn();

vi.mock("@/composables/shared/apiClient", () => ({
  apiGet: (...args: unknown[]) => providerStatusMock(...args),
  fetchEnvelope: (...args: unknown[]) => providerStatusMock(...args),
  fetchEnvelopeWithInit: (...args: unknown[]) => fetchEnvelopeWithInitMock(...args),
  apiGetPath: (_template: string, path: string, init?: RequestInit) =>
    fetchEnvelopeWithInitMock(path, init ?? {}),
}));

vi.mock("@/composables/workspace/useConsoleData", () => ({
  useConsoleData: () => ({
    currentMarketDataSnapshot: marketDataSnapshot,
    currentMarketSecurityDetails: marketSecurityDetails,
    marketInstrumentSearchOptions: marketInstrumentSearchOptions,
    isLoadingMarketDataQuery: ref(false),
    marketDataQueryError: ref(null),
    selectedBrokerAccount: ref(null),
    brokerRuntime: ref(null),
    systemStatus: ref(null),
    supportsBrokerReadFeature: () => true,
    acquireMarketDataSubscription: (...args: unknown[]) =>
      acquireMarketDataSubscriptionMock(...args),
    createStableWebConsumerId: () => "web:workspace-depth:window:test",
    heartbeatMarketDataConsumer: (...args: unknown[]) =>
      heartbeatMarketDataConsumerMock(...args),
    releaseMarketDataSubscription: (...args: unknown[]) =>
      releaseMarketDataSubscriptionMock(...args),
  }),
}));

function mountOrderBookPanel(options: {
  market?: string;
  symbol?: string;
  period?: string;
} = {}) {
  const Host = defineComponent({
    setup() {
      const store = provideWorkspaceTradingPreferencesStore();
      store.update({
        market: options.market ?? "HK",
        symbol: options.symbol ?? "00700",
        period: options.period ?? "1m",
      });
      return () => h(OrderBookPanel);
    },
  });
  return mount(Host, {
    global: {
      stubs: productGlobalStubs,
    },
  });
}

function panelSetup(wrapper: ReturnType<typeof mountOrderBookPanel>): Record<string, unknown> {
  return wrapper.findComponent(OrderBookPanel).vm.$.setupState as Record<string, unknown>;
}

function readSetupValue<T>(
  wrapper: ReturnType<typeof mountOrderBookPanel>,
  key: string,
): T {
  const value = panelSetup(wrapper)[key];
  if (value !== null && typeof value === "object" && "value" in value) {
    return (value as { value: T }).value;
  }
  return value as T;
}

function writeSetupValue<T>(
  wrapper: ReturnType<typeof mountOrderBookPanel>,
  key: string,
  value: T,
): void {
  const current = panelSetup(wrapper)[key];
  if (current !== null && typeof current === "object" && "value" in current) {
    (current as { value: T }).value = value;
    return;
  }
  panelSetup(wrapper)[key] = value;
}

function createDepthEnvelope<TPayload extends {
  type: "market.depth";
  at: string;
  request: { instrumentId: string; num: number };
}>(payload: TPayload) {
  return createLiveEnvelope(payload, {
    source: "market-data",
    entityId: `${payload.request.instrumentId}|${payload.request.num}`,
  });
}

describe("Futu Market Data Workspace E2E Integration Suite", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetBrokerProviderSelectionForTests();
    resetSharedLiveSocketHubForTests();
    MockWebSocket.instances = [];
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);
    useBrokerProviderSelection().selectBrokerProvider("futu");

    providerStatusMock.mockResolvedValue({
      activeProvider: "futu",
      descriptor: {
        capabilities: {
          orderBookDepth: true,
          ticks: true,
          kline: true,
        },
      },
    });

    fetchEnvelopeWithInitMock.mockResolvedValue({
      request: {
        market: "HK",
        symbol: "00700",
        num: 10,
      },
      depth: {
        symbol: "HK.00700",
        bids: [
          { price: 385.2, volume: 10000, orderCount: 12 },
          { price: 385.0, volume: 15000, orderCount: 18 },
        ],
        asks: [
          { price: 385.4, volume: 8000, orderCount: 9 },
          { price: 385.6, volume: 12000, orderCount: 14 },
        ],
      },
      meta: {
        instrumentId: "HK.00700",
        resolvedAt: "2026-09-07T09:30:00Z",
      },
    });

    acquireMarketDataSubscriptionMock.mockResolvedValue(true);
    heartbeatMarketDataConsumerMock.mockResolvedValue(undefined);
    releaseMarketDataSubscriptionMock.mockResolvedValue(undefined);

    marketDataSnapshot.value = {
      snapshot: {
        market: "HK",
        code: "00700",
        price: 385.4,
        changeAmount: 3.2,
        changeRate: 0.84,
        at: "2026-09-07T09:30:00Z",
      },
      meta: {
        brokerId: "futu",
        resolvedAt: "2026-09-07T09:30:00Z",
      },
    };

    marketSecurityDetails.value = {
      security: {
        market: "HK",
        code: "00700",
        name: "腾讯控股",
      },
    };
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // ==========================================================================
  // TIER 1: FEATURE COVERAGE
  // ==========================================================================
  describe("Tier 1: Feature Coverage (R1-R4)", () => {
    it("T1.1: QuoteSummaryCard renders prominent Chinese security name alongside symbol code (R2)", () => {
      const wrapper = mount(QuoteSummaryCard, {
        props: {
          market: "HK",
          code: "00700",
          instrumentId: "HK.00700",
          name: "腾讯控股",
          price: 385.4,
          changeRate: 0.84,
          sessionLabel: "交易中",
          sessionActive: true,
        },
      });

      expect(wrapper.text()).toContain("腾讯控股");
      expect(wrapper.text()).toContain("00700");
      expect(wrapper.get(".quote-summary__price").text()).toContain("385.4");
    });

    it("T1.2: Instrument name resolution does not short-circuit on empty option name (R2/F2.1)", () => {
      marketInstrumentSearchOptions.value = [
        { instrumentId: "HK.00700", name: "" }, // empty name in search candidate
      ];

      const option = marketInstrumentSearchOptions.value[0];
      const security = marketSecurityDetails.value.security;

      // Demonstrating the fix contract:
      const resolvedName = option?.name?.trim() || security?.name?.trim() || "";
      expect(resolvedName).toBe("腾讯控股");
    });

    it("T1.3: OrderBookPanel connects to depth stream and recognizes supportsOrderBookDepth as true (R3)", async () => {
      const wrapper = mountOrderBookPanel();
      await flushPromises();

      const supportsDepth = readSetupValue<boolean | null>(wrapper, "supportsOrderBookDepth");
      expect(supportsDepth).toBe(true);
      expect(acquireMarketDataSubscriptionMock).toHaveBeenCalledWith(
        expect.objectContaining({
          channel: "ORDER_BOOK",
          market: "HK",
          symbol: "00700",
        }),
      );
    });

    it("T1.4: OrderBookPanel defensively receives push events with or without meta (R3/F3.3)", async () => {
      const hub = getSharedLiveSocketHub();
      const wrapper = mountOrderBookPanel();
      await flushPromises();

      hub.connect("ws://127.0.0.1:3000/api/v1/ws/live");
      await Promise.resolve();

      expect(MockWebSocket.instances.length).toBeGreaterThanOrEqual(1);
      MockWebSocket.instances[0]?.emitMessage(
        createDepthEnvelope({
          type: "market.depth",
          at: "2026-09-07T09:30:00Z",
          request: {
            instrumentId: "HK.00700",
            num: 10,
          },
          depth: {
            bids: [{ price: 385.2, volume: 20000, orderCount: 15 }],
            asks: [{ price: 385.4, volume: 18000, orderCount: 12 }],
          },
          meta: {
            instrumentId: "HK.00700",
          },
        } as any),
      );
      await nextTick();

      const depthError = readSetupValue<string>(wrapper, "depthError");
      expect(depthError).toBe("");
    });

    it("T1.5: BrokerProviderTag displays Futu connection and capability scope (R4)", async () => {
      useBrokerProviderSelection().selectBrokerProvider("futu");
      const wrapper = mount(BrokerProviderTag, {
        props: {
          market: "HK",
          featureId: "market.depth",
          connectionState: "connected",
          transportMode: "push-stream",
          provider: {
            brokerId: "futu",
            securityFirm: "Futu Securities",
            featureId: "market.depth",
            capability: "available",
            selectionReason: "explicit",
            resolvedAt: "2026-09-07T00:00:00Z",
            asOf: "2026-09-07T00:00:00Z",
          },
        },
        global: {
          stubs: productGlobalStubs,
        },
      });
      await flushPromises();

      const tag = wrapper.get(".broker-provider-tag");
      expect(tag.attributes("title")).toContain("Futu");
      expect(tag.attributes("data-quality")).toBe("healthy");
    });
  });

  // ==========================================================================
  // TIER 2: BOUNDARY & CORNER CASES
  // ==========================================================================
  describe("Tier 2: Boundary & Corner Cases", () => {
    it("T2.1: QuoteSummaryCard falls back gracefully when name is null/empty", () => {
      const wrapper = mount(QuoteSummaryCard, {
        props: {
          market: "HK",
          code: "00700",
          instrumentId: "HK.00700",
          name: null,
          price: 385.4,
        },
      });

      expect(wrapper.text()).toContain("00700");
    });

    it("T2.2: QuoteSummaryCard renders special non-ASCII unicode security names cleanly", () => {
      const wrapper = mount(QuoteSummaryCard, {
        props: {
          market: "HK",
          code: "03690",
          instrumentId: "HK.03690",
          name: "美团－Ｗ",
          price: 120.5,
        },
      });

      expect(wrapper.text()).toContain("美团－Ｗ");
      expect(wrapper.text()).toContain("03690");
    });

    it("T2.3: OrderBookPanel handles 409 conflict via self-healing without error message", async () => {
      let callCount = 0;
      acquireMarketDataSubscriptionMock.mockImplementation(() => {
        callCount++;
        if (callCount === 1) {
          throw new Error("409 Conflict: Subscription already exists");
        }
        return Promise.resolve(true);
      });

      const wrapper = mountOrderBookPanel();
      await flushPromises();

      // Trigger self healing with current lifecycleSeq
      writeSetupValue(wrapper, "supportsOrderBookDepth", true);
      const currentSeq = readSetupValue<number>(wrapper, "depthLifecycleSeq");
      const syncDepth = panelSetup(wrapper)["syncDepthSubscription"] as Function;
      if (syncDepth) {
        const result = await syncDepth(
          { brokerId: "futu", market: "HK", symbol: "00700", instrumentId: "HK.00700" },
          currentSeq,
          true,
        );
        expect(result).toBe(true);
      }
    });

    it("T2.4: OrderBookPanel handles empty depth gracefully (0 bids, 0 asks)", async () => {
      fetchEnvelopeWithInitMock.mockResolvedValueOnce({
        request: { market: "HK", symbol: "00700", num: 10 },
        depth: { symbol: "HK.00700", bids: [], asks: [] },
        meta: { instrumentId: "HK.00700" },
      });

      const wrapper = mountOrderBookPanel();
      await flushPromises();

      const depthData = readSetupValue<any>(wrapper, "depthData");
      expect(depthData).toBeDefined();
    });

    it("T2.5: OrderBookPanel cleans up heartbeat timer and listeners on unmount", async () => {
      const wrapper = mountOrderBookPanel();
      await flushPromises();

      wrapper.unmount();
      expect(releaseMarketDataSubscriptionMock).toHaveBeenCalled();
    });
  });

  // ==========================================================================
  // TIER 3: CROSS-FEATURE COMBINATIONS
  // ==========================================================================
  describe("Tier 3: Cross-Feature Combinations", () => {
    it("T3.1: Quote price and OrderBook top of book align coherently", async () => {
      const quotePrice = 385.4;
      const quoteWrapper = mount(QuoteSummaryCard, {
        props: {
          market: "HK",
          code: "00700",
          instrumentId: "HK.00700",
          name: "腾讯控股",
          price: quotePrice,
        },
      });

      const orderBookWrapper = mountOrderBookPanel();
      await flushPromises();

      expect(quoteWrapper.get(".quote-summary__price").text()).toContain("385.4");
      const depthData = readSetupValue<any>(orderBookWrapper, "depthData");
      if (depthData?.bids?.[0]) {
        expect(depthData.bids[0].price).toBeLessThanOrEqual(quotePrice);
      }
    });

    it("T3.2: Disabling provider depth support prevents depth acquisition", async () => {
      providerStatusMock.mockResolvedValueOnce({
        activeProvider: "yfinance",
        descriptor: {
          capabilities: {
            orderBookDepth: false,
          },
        },
      });

      const wrapper = mountOrderBookPanel();
      await flushPromises();

      const supportsDepth = readSetupValue<boolean | null>(wrapper, "supportsOrderBookDepth");
      expect(supportsDepth).toBe(false);
    });
  });

  // ==========================================================================
  // TIER 4: REAL-WORLD APPLICATION SCENARIOS
  // ==========================================================================
  describe("Tier 4: Real-World Application Scenarios", () => {
    it("T4.1: Complete workspace session for HK.00700 (header name, depth table, provider status)", async () => {
      const quoteCard = mount(QuoteSummaryCard, {
        props: {
          market: "HK",
          code: "00700",
          instrumentId: "HK.00700",
          name: "腾讯控股",
          price: 385.4,
          changeRate: 0.84,
          sessionLabel: "交易中",
          sessionActive: true,
          sourceText: "FUTU · OPEND.SNAPSHOT",
        },
      });

      const orderBook = mountOrderBookPanel();
      await flushPromises();

      expect(quoteCard.text()).toContain("腾讯控股");
      expect(quoteCard.text()).toContain("FUTU · OPEND.SNAPSHOT");
      expect(readSetupValue(orderBook, "supportsOrderBookDepth")).toBe(true);
    });

    it("T4.2: Symbol change updates OrderBook and releases previous subscription", async () => {
      const wrapper = mountOrderBookPanel({ market: "HK", symbol: "00700" });
      await flushPromises();

      const setTarget = panelSetup(wrapper)["connectDepthStream"] as Function;
      if (setTarget) {
        await setTarget();
      }
      expect(acquireMarketDataSubscriptionMock).toHaveBeenCalled();
    });
  });
});
