// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

const predictionMocks = vi.hoisted(() => ({
  fetchWithInit: vi.fn(),
  fetchFeature: vi.fn(),
}));
const consoleState = {
  selectedBrokerAccount: ref<{
    brokerId: string;
    accountId: string;
    tradingEnvironment: string;
  } | null>({
    brokerId: "futu",
    accountId: "moomoo-us",
    tradingEnvironment: "REAL",
  }),
  systemStatus: ref({
    defaultBroker: "futu",
    defaultTradingEnvironment: "SIMULATE",
  }),
};

vi.mock("../src/composables/apiClient", () => ({
  fetchEnvelopeWithInit: predictionMocks.fetchWithInit,
}));
vi.mock("../src/composables/productFeatures", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../src/composables/productFeatures")>();
  return { ...actual, fetchProductFeature: predictionMocks.fetchFeature };
});
vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => consoleState,
}));

import PredictionResearchPanel from "../src/components/product/PredictionResearchPanel.vue";
import {
  resetBrokerProviderSelectionForTests,
  useBrokerProviderSelection,
} from "../src/composables/brokerProviderSelection";
import {
  flushPromises,
  productGlobalStubs,
  setupState,
} from "./productTestUtils";

function feature(
  entries: Record<string, unknown>[],
  metadata: Record<string, unknown> = {},
) {
  return {
    provider: {
      brokerId: "futu",
      featureId: "prediction",
      capability: "available",
      selectionReason: "explicit",
      resolvedAt: "2026-07-17T00:00:00Z",
      asOf: "2026-07-17T00:00:00Z",
    },
    asOf: "2026-07-17T00:00:00Z",
    entries,
    metadata,
  };
}

beforeEach(() => {
  resetBrokerProviderSelectionForTests();
  predictionMocks.fetchFeature.mockReset();
  predictionMocks.fetchWithInit.mockReset();
  consoleState.selectedBrokerAccount.value = {
    brokerId: "futu",
    accountId: "moomoo-us",
    tradingEnvironment: "REAL",
  };
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    value: "visible",
  });
});

afterEach(() => {
  resetBrokerProviderSelectionForTests();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("prediction research and Parlay lifecycle", () => {
  it("restores contract context from props and emits URL-facing updates", async () => {
    predictionMocks.fetchFeature.mockResolvedValue(feature([]));
    const wrapper = mount(PredictionResearchPanel, {
      props: {
        seriesCode: "SERIES.DEEP",
        eventCode: "EVENT.DEEP",
        contractCode: "EC.DEEP",
        contractView: "snapshot",
      },
      global: {
        stubs: {
          ...productGlobalStubs,
          PredictionContractDataView: {
            props: ["path", "view"],
            template: "<div class='contract-panel'>{{ path }} · {{ view }}</div>",
          },
        },
      },
    });
    await flushPromises();
    const state = setupState<{
      stage: string;
      seriesCode: string;
      eventCode: string;
      contractCode: string;
      contractView: string;
      contractPath: string;
      backDiscover: () => void;
    }>(wrapper);

    expect(state.stage).toBe("contract");
    expect(state.contractPath).toContain("/EC.DEEP/snapshot");
    expect(wrapper.text()).toContain("SERIES.DEEP");
    expect(wrapper.text()).toContain("EVENT.DEEP");
    expect(predictionMocks.fetchFeature).not.toHaveBeenCalled();

    await wrapper
      .findAll('[aria-label="合约数据视图"] button')
      .find((button) => button.text() === "里程碑")!
      .trigger("click");
    expect(wrapper.emitted("update:contractView")?.at(-1)).toEqual([
      "milestones",
    ]);

    await wrapper.setProps({
      contractCode: "EC.RESTORED",
      contractView: "milestones",
    });
    await flushPromises();
    expect(state.contractCode).toBe("EC.RESTORED");
    expect(state.contractView).toBe("milestones");
    expect(state.contractPath).toContain("/EC.RESTORED/milestones");

    state.backDiscover();
    await flushPromises();
    expect(state.stage).toBe("contracts");
    expect(wrapper.emitted("update:contractCode")?.at(-1)).toEqual([""]);
  });

  it("walks discovery, leases only visible live views, RFQs, places, and cancels", async () => {
    vi.useFakeTimers();
    predictionMocks.fetchFeature.mockImplementation(async (path: string) => {
      if (path.includes("/categories")) {
        return feature([
          {
            category: "SPORTS",
            categoryName: "体育",
            competitionList: ["NBA"],
          },
        ]);
      }
      if (path.includes("/competitions")) {
        return feature([
          {
            tag: "NBA",
            title: "NBA",
            competitionScope: "US",
            status: "OPEN",
          },
        ]);
      }
      if (path.includes("/series")) {
        return feature([
          { seriesName: "总冠军", seriesSecurity: { code: "SERIES.1" } },
        ]);
      }
      if (path.includes("/events") && !path.includes("/contracts")) {
        return feature([
          {
            eventName: "总决赛",
            eventSecurity: { code: "EVENT.1" },
            endDate: "2026-07-20",
          },
        ]);
      }
      if (path.includes("/contracts")) {
        return feature([
          {
            title: "主队获胜",
            contractSecurity: { code: "EC.HOME" },
            closeTime: "2026-07-20",
          },
        ]);
      }
      if (path.includes("/eligible-events")) {
        return feature(
          [
            {
              eventName: "事件 A",
              comboContracts: [{ code: "EC.A" }, { code: "EC.B" }],
            },
          ],
          { mvc: "mvc-1" },
        );
      }
      throw new Error(`unexpected feature ${path}`);
    });
    predictionMocks.fetchWithInit.mockImplementation(async (path: string) => {
      if (path.includes("/subscriptions/")) return { released: true };
      if (path.includes("/subscriptions")) {
        return {
          leaseId: `lease-${path.length}`,
          instrumentId: "US.EC.HOME",
          dataTypes: ["ORDER_BOOK"],
        };
      }
      if (path.includes("/prediction/combos/quotes")) {
        return feature([], {
          quoteId: "quote-1",
          quoteExpiresAt: "2099-01-01T00:00:00Z",
          bidPrice: 0.3,
          askPrice: 0.35,
        });
      }
      if (path === "/api/v1/execution/combos/previews") {
        return { previewId: "preview-parlay" };
      }
      if (path === "/api/v1/execution/combos") {
        return {
          accepted: true,
          internalOrderId: "parlay-1",
          brokerOrderId: "broker-parlay",
          orderStatus: "BROKER_ACCEPTED",
          message: "accepted",
        };
      }
      if (path.includes("/execution/combos/parlay-1/cancel")) {
        return {
          accepted: true,
          internalOrderId: "parlay-1",
          orderStatus: "CANCELLED",
        };
      }
      throw new Error(`unexpected API ${path}`);
    });
    vi.stubGlobal("crypto", { randomUUID: () => "parlay-uuid" });
    const wrapper = mount(PredictionResearchPanel, {
      global: {
        stubs: {
          ...productGlobalStubs,
          PredictionContractDataView: {
            props: ["path", "view"],
            template: "<div class='contract-panel'>{{ path }}</div>",
          },
        },
      },
    });
    await flushPromises();
    const state = setupState<{
      mode: string;
      stage: string;
      result: { entries: Record<string, unknown>[] } | null;
      contractCode: string;
      contractView: string;
      contractPath: string;
      contractSubscriptionType: string;
      subscriptionReady: boolean;
      activeSubscription: { leaseId: string; dataType: string } | null;
      selectedLegs: Record<string, string>;
      eligible: { entries: Record<string, unknown>[] } | null;
      parlayContracts: Array<{ code: string }>;
      selectedLegCount: number;
      quote: { metadata: Record<string, unknown> } | null;
      quoteExpired: boolean;
      amount: number;
      confirmed: boolean;
      execution: { internalOrderId?: string; orderStatus?: string } | null;
      selectDiscoverEntry: (entry: Record<string, unknown>) => void;
      backDiscover: () => void;
      switchMode: (mode: "discover" | "parlay") => void;
      toggleParlayContract: (code: string) => void;
      setParlaySide: (code: string, side: "YES" | "NO") => void;
      parlaySide: (code: string) => "YES" | "NO";
      requestRFQ: () => Promise<void>;
      previewParlay: () => Promise<void>;
      placeParlay: () => Promise<void>;
      cancelParlay: () => Promise<void>;
      comboLegs: () => Record<string, unknown>[];
      itemSubtitle: (entry: Record<string, unknown>) => string;
      securityCode: (entry: unknown) => string;
      queryString: (entry: Record<string, string>) => string;
      loadDiscover: (stage?: string) => Promise<void>;
      pageVisible: boolean;
    }>(wrapper);

    expect(state.stage).toBe("categories");
    expect(wrapper.text()).toContain("体育");
    await state.loadDiscover("contract");
    await wrapper.get(".prediction-research__card").trigger("click");
    await flushPromises();
    expect(state.stage).toBe("competitions");
    state.selectDiscoverEntry(state.result!.entries[0]!);
    await flushPromises();
    expect(state.stage).toBe("series");
    state.selectDiscoverEntry(state.result!.entries[0]!);
    await flushPromises();
    expect(state.stage).toBe("events");
    state.selectDiscoverEntry(state.result!.entries[0]!);
    await flushPromises();
    expect(state.stage).toBe("contracts");
    state.selectDiscoverEntry(state.result!.entries[0]!);
    await nextTick();
    expect(state.stage).toBe("contract");
    expect(state.contractCode).toBe("EC.HOME");
    expect(state.contractPath).toContain("/snapshot");

    await wrapper
      .findAll('[role="tab"]')
      .find((tab) => tab.text() === "YES/NO 盘口")!
      .trigger("click");
    await flushPromises();
    expect(state.contractSubscriptionType).toBe("ORDER_BOOK");
    expect(state.activeSubscription?.dataType).toBe("ORDER_BOOK");
    expect(state.subscriptionReady).toBe(true);
    state.contractView = "candles";
    await flushPromises();
    expect(state.contractPath).toContain("/candles");
    expect(state.activeSubscription?.dataType).toBe("KLINE");
    state.contractView = "ticks";
    await flushPromises();
    expect(state.contractSubscriptionType).toBe("TICKER");
    expect(state.contractPath).toContain("/ticks");
    state.contractView = "milestones";
    await flushPromises();
    expect(state.contractSubscriptionType).toBe("");
    expect(state.contractPath).toContain("/milestones");

    state.backDiscover();
    await flushPromises();
    expect(state.stage).toBe("contracts");
    expect(state.itemSubtitle({ competition: "NBA", status: "OPEN" })).toBe(
      "NBA · OPEN",
    );
    expect(state.securityCode(null)).toBe("");
    expect(state.queryString({ empty: "", tag: "NBA" })).toContain("tag=NBA");

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "Parlay 组合")!
      .trigger("click");
    await flushPromises();
    expect(state.parlayContracts.map((item) => item.code)).toEqual([
      "EC.A",
      "EC.B",
    ]);
    state.toggleParlayContract("EC.A");
    state.toggleParlayContract("EC.B");
    expect(state.selectedLegCount).toBe(2);
    await wrapper
      .findAll('.prediction-research__leg input[type="checkbox"]')[0]!
      .setValue(false);
    await nextTick();
    state.toggleParlayContract("EC.A");
    expect(state.parlaySide("EC.A")).toBe("YES");
    state.setParlaySide("EC.B", "NO");
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "YES")!
      .trigger("click");
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "NO")!
      .trigger("click");
    state.setParlaySide("EC.A", "YES");
    state.setParlaySide("EC.B", "NO");
    expect(
      Object.fromEntries(
        state.comboLegs().map((leg) => [
          leg.instrumentId,
          leg.predictionSide,
        ]),
      ),
    ).toEqual({ "US.EC.A": "YES", "US.EC.B": "NO" });

    await state.requestRFQ();
    expect(state.quoteExpired).toBe(false);
    expect(wrapper.text()).toContain("quote-1");
    state.confirmed = true;
    state.amount = 25;
    await wrapper.get('input[type="number"]').setValue("25");
    await wrapper
      .get('.prediction-research__confirm input[type="checkbox"]')
      .setValue(true);
    await nextTick();
    await state.previewParlay();
    await nextTick();
    state.confirmed = true;
    await state.placeParlay();
    expect(state.execution?.internalOrderId).toBe("parlay-1");
    const place = predictionMocks.fetchWithInit.mock.calls.find(
      ([path]) => path === "/api/v1/execution/combos",
    );
    const payload = JSON.parse(String(place?.[1]?.body));
    expect(payload).toMatchObject({
      clientOrderId: "jftrade-parlay-parlay-uuid",
      rfqId: "quote-1",
      amount: 25,
      previewId: "preview-parlay",
    });
    await state.cancelParlay();
    expect(state.execution?.orderStatus).toBe("CANCELLED");

    state.toggleParlayContract("EC.A");
    expect(state.selectedLegCount).toBe(1);
    expect(state.quote).toBeNull();
    state.toggleParlayContract("EC.A");
    state.quote = {
      metadata: {
        quoteId: "expired",
        quoteExpiresAt: "2000-01-01T00:00:00Z",
      },
    };
    expect(state.quoteExpired).toBe(true);
    const calls = predictionMocks.fetchWithInit.mock.calls.length;
    await state.placeParlay();
    expect(predictionMocks.fetchWithInit.mock.calls).toHaveLength(calls);

    state.mode = "discover";
    state.stage = "contract";
    state.contractCode = "EC.HOME";
    state.contractView = "depth";
    await flushPromises();
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "在交易工作区打开")!
      .trigger("click");
    state.stage = "categories";
    state.backDiscover();
    expect(wrapper.emitted("openInstrument")?.at(-1)).toEqual([
      "US.EC.HOME",
      "prediction",
      "event_contract",
    ]);
    const providerSelection = useBrokerProviderSelection();
    state.mode = "parlay";
    providerSelection.selectBrokerProvider("alpha");
    await flushPromises();
    state.mode = "discover";
    state.stage = "categories";
    providerSelection.selectBrokerProvider("beta");
    await flushPromises();
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "hidden",
    });
    document.dispatchEvent(new Event("visibilitychange"));
    expect(state.pageVisible).toBe(false);
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });
    document.dispatchEvent(new Event("visibilitychange"));
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "事件与合约")!
      .trigger("click");
    state.stage = "contract";
    state.contractView = "depth";
    await flushPromises();
    await vi.advanceTimersByTimeAsync(3_000);
    wrapper.unmount();
    await flushPromises();
    expect(
      predictionMocks.fetchWithInit.mock.calls.some(([path]) =>
        String(path).includes("/subscriptions/"),
      ),
    ).toBe(true);
  });

  it("reports eligibility, RFQ, subscription, execution, and cancellation failures", async () => {
    predictionMocks.fetchFeature
      .mockRejectedValueOnce(new Error("预测市场不可用"))
      .mockRejectedValueOnce("资格失败");
    predictionMocks.fetchWithInit.mockRejectedValue(new Error("外部失败"));
    const wrapper = mount(PredictionResearchPanel, {
      global: {
        stubs: {
          ...productGlobalStubs,
          PredictionContractDataView: { template: "<div />" },
        },
      },
    });
    await flushPromises();
    expect(wrapper.text()).toContain("预测市场不可用");
    const state = setupState<{
      mode: string;
      stage: string;
      contractCode: string;
      contractView: string;
      selectedLegs: Record<string, "YES" | "NO">;
      eligible: ReturnType<typeof feature> | null;
      quote: ReturnType<typeof feature> | null;
      preview: {
        previewId: string;
        warnings?: string[];
      } | null;
      parlayClientOrderID: string;
      confirmed: boolean;
      amount: number;
      execution: { internalOrderId: string } | null;
      loadDiscover: () => Promise<void>;
      loadEligible: () => Promise<void>;
      syncContractSubscription: () => Promise<void>;
      requestRFQ: () => Promise<void>;
      previewParlay: () => Promise<void>;
      placeParlay: () => Promise<void>;
      cancelParlay: () => Promise<void>;
    }>(wrapper);
    await state.loadDiscover();
    expect(wrapper.text()).toContain("资格失败");
    predictionMocks.fetchFeature.mockRejectedValueOnce("合格事件失败");
    await state.loadEligible();
    expect(wrapper.text()).toContain("合格事件失败");

    state.mode = "discover";
    state.stage = "contract";
    state.contractCode = "EC.FAIL";
    state.contractView = "depth";
    await flushPromises();
    expect(wrapper.text()).toContain("外部失败");

    state.eligible = feature([], { mvc: "mvc-fail" });
    state.selectedLegs = {};
    await state.previewParlay();
    await state.requestRFQ();
    await state.cancelParlay();
    state.selectedLegs = { A: "YES", B: "NO" };
    await state.requestRFQ();
    expect(state.quote).toBeNull();
    state.quote = feature([], {
      quoteId: "quote-fail",
      quoteExpiresAt: "2099-01-01T00:00:00Z",
    });
    state.parlayClientOrderID = "";
    state.confirmed = true;
    state.amount = 10;
    predictionMocks.fetchWithInit.mockRejectedValueOnce(new Error("预检失败"));
    await state.previewParlay();
    expect(wrapper.text()).toContain("预检失败");
    state.preview = {
      previewId: "preview-fail",
      warnings: ["报价即将到期"],
    };
    state.mode = "parlay";
    await nextTick();
    expect(wrapper.text()).toContain("报价即将到期");
    state.confirmed = true;
    predictionMocks.fetchWithInit.mockRejectedValueOnce("下单失败");
    await state.placeParlay();
    expect(wrapper.text()).toContain("下单失败");
    expect(state.execution).toBeNull();
    state.execution = { internalOrderId: "cancel-fail" };
    await state.cancelParlay();
    expect(wrapper.text()).toContain("外部失败");
  });

  it("releases a stale subscription acquired after the visible view changed", async () => {
    predictionMocks.fetchFeature.mockResolvedValue(feature([]));
    let resolveFirst:
      | ((lease: {
          leaseId: string;
          instrumentId: string;
          dataTypes: string[];
        }) => void)
      | undefined;
    let subscriptionRequests = 0;
    predictionMocks.fetchWithInit.mockImplementation(
      async (path: string, init?: RequestInit) => {
        if (path.includes("/subscriptions/")) return { released: true };
        if (path.includes("/subscriptions") && init?.method === "POST") {
          subscriptionRequests++;
          if (subscriptionRequests === 1) {
            return new Promise((resolve) => {
              resolveFirst = resolve;
            });
          }
          return {
            leaseId: "lease-current",
            instrumentId: "US.EC.CURRENT",
            dataTypes: ["TICKER"],
          };
        }
        throw new Error(`unexpected API ${path}`);
      },
    );
    const wrapper = mount(PredictionResearchPanel, {
      global: {
        stubs: {
          ...productGlobalStubs,
          PredictionContractDataView: { template: "<div />" },
        },
      },
    });
    await flushPromises();
    const state = setupState<{
      mode: string;
      stage: string;
      contractCode: string;
      contractView: string;
    }>(wrapper);
    state.mode = "discover";
    state.stage = "contract";
    state.contractCode = "EC.STALE";
    state.contractView = "depth";
    await nextTick();
    expect(subscriptionRequests).toBe(1);

    state.contractView = "ticks";
    await flushPromises();
    expect(subscriptionRequests).toBe(2);
    resolveFirst!({
      leaseId: "lease-stale",
      instrumentId: "US.EC.STALE",
      dataTypes: ["ORDER_BOOK"],
    });
    await flushPromises();
    expect(
      predictionMocks.fetchWithInit.mock.calls.some(([path]) =>
        String(path).includes("/subscriptions/lease-stale"),
      ),
    ).toBe(true);
    wrapper.unmount();
    await flushPromises();
  });

  it("handles every discovery return level and sparse catalog labels", async () => {
    predictionMocks.fetchFeature.mockResolvedValue(feature([]));
    const wrapper = mount(PredictionResearchPanel, {
      global: {
        stubs: {
          ...productGlobalStubs,
          PredictionContractDataView: { template: "<div />" },
        },
      },
    });
    await flushPromises();
    const state = setupState<{
      stage: string;
      category: string;
      tag: string;
      seriesCode: string;
      eventCode: string;
      contractCode: string;
      contractView: string;
      itemTitle: (entry: Record<string, unknown>, index: number) => string;
      discoverStageFromContext: () => string;
      backDiscover: () => void;
      selectContractView: (view: string) => void;
      subscriptionQuery: () => string;
    }>(wrapper);

    expect([
      state.itemTitle({ categoryName: "分类名" }, 0),
      state.itemTitle({ eventName: "事件名" }, 0),
      state.itemTitle({ seriesName: "系列名" }, 0),
      state.itemTitle({ title: "标题" }, 0),
      state.itemTitle({ tag: "标签" }, 0),
      state.itemTitle({ category: "分类码" }, 0),
      state.itemTitle({}, 1),
    ]).toEqual(["分类名", "事件名", "系列名", "标题", "标签", "分类码", "结果 2"]);

    state.seriesCode = "SERIES.1";
    expect(state.discoverStageFromContext()).toBe("events");
    state.eventCode = "EVENT.1";
    expect(state.discoverStageFromContext()).toBe("contracts");
    state.contractCode = "EC.1";
    expect(state.discoverStageFromContext()).toBe("contract");
    state.contractCode = "";

    state.stage = "competitions";
    state.category = "SPORTS";
    state.backDiscover();
    await flushPromises();
    expect(state.category).toBe("");

    state.stage = "series";
    state.tag = "NBA";
    state.backDiscover();
    await flushPromises();
    expect(state.tag).toBe("");

    state.stage = "events";
    state.seriesCode = "SERIES.1";
    state.backDiscover();
    await flushPromises();
    expect(state.seriesCode).toBe("");

    state.stage = "contracts";
    state.eventCode = "EVENT.1";
    state.backDiscover();
    await flushPromises();
    expect(state.eventCode).toBe("");

    state.contractView = "snapshot";
    state.selectContractView("snapshot");
    expect(wrapper.emitted("update:contractView")).toBeUndefined();
    expect(state.subscriptionQuery()).toContain("brokerId=futu");
    wrapper.unmount();
  });
});
