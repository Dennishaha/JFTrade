// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  fetchFeature: vi.fn(),
  fetchWithInit: vi.fn(),
}));

vi.mock("../src/composables/productFeatures", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../src/composables/productFeatures")>();
  return { ...actual, fetchProductFeature: apiMocks.fetchFeature };
});
vi.mock("../src/composables/apiClient", () => ({
  fetchEnvelopeWithInit: apiMocks.fetchWithInit,
}));
vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => ({
    selectedBrokerAccount: {
      value: {
        brokerId: "futu",
        accountId: "account-1",
        tradingEnvironment: "SIMULATE",
      },
    },
    systemStatus: {
      value: { defaultBroker: "futu", defaultTradingEnvironment: "SIMULATE" },
    },
  }),
}));

import OptionResearchPanel from "../src/components/product/OptionResearchPanel.vue";
import {
  resetBrokerProviderSelectionForTests,
  useBrokerProviderSelection,
} from "../src/composables/brokerProviderSelection";
import {
  flushPromises,
  productGlobalStubs,
  setupState,
} from "./productTestUtils";

function feature(entries: Record<string, unknown>[], nextCursor = "") {
  return {
    provider: {
      brokerId: "futu",
      featureId: "derivatives.option_events",
      capability: "available",
      selectionReason: "explicit_broker",
      resolvedAt: "2026-07-18T00:00:00Z",
      asOf: "2026-07-18T00:00:00Z",
    },
    asOf: "2026-07-18T00:00:00Z",
    entries,
    nextCursor,
    total: entries.length,
  };
}

beforeEach(() => {
  apiMocks.fetchFeature.mockReset();
  apiMocks.fetchWithInit.mockReset();
  resetBrokerProviderSelectionForTests();
  useBrokerProviderSelection().selectBrokerProvider("futu");
});

describe("option research panel", () => {
  it("scopes workspace events to BABA and drills into 0DTE contracts", async () => {
    apiMocks.fetchFeature.mockResolvedValue(
      feature([
        {
          owner: { instrumentId: "US.BABA" },
          name: "Alibaba",
          price: 180,
          iv: 32,
          drilldownContext: {
            underlyingInstrumentId: "US.BABA",
            expiryTimestamp: 1784332800,
            chain: {
              productCode: "BABA",
              multiplier: 100,
              contractSize: 100,
              expirationType: 2,
            },
          },
        },
      ]),
    );
    apiMocks.fetchWithInit.mockResolvedValue(
      feature([
        {
          option: { instrumentId: "US.BABA260724C180000" },
          optionType: "call",
          optionPrice: 2.4,
          iv: 31,
          delta: 0.51,
        },
      ]),
    );
    const wrapper = mount(OptionResearchPanel, {
      props: {
        market: "US",
        operation: "zero_dte",
        scope: "underlying",
        underlyingInstrumentId: "US.BABA",
        underlyingProductClass: "equity",
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();
    const eventPath = String(apiMocks.fetchFeature.mock.calls[0]?.[0]);
    expect(eventPath).toContain("operation=zero_dte");
    expect(eventPath).toContain("underlying=US.BABA");
    expect(eventPath).toContain("brokerId=futu");

    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("查看合约"))!
      .trigger("click");
    await flushPromises();
    const [path, init] = apiMocks.fetchWithInit.mock.calls[0]!;
    expect(path).toContain(
      "/api/v1/market-data/options/events/zero-dte-contracts",
    );
    const body = JSON.parse(String(init.body));
    expect(body).toMatchObject({
      brokerId: "futu",
      accountId: "account-1",
      tradingEnvironment: "SIMULATE",
      market: "US",
      underlyingInstrumentId: "US.BABA",
      expiryTimestamp: 1784332800,
      chain: { productCode: "BABA", contractSize: 100 },
    });
    expect(wrapper.text()).toContain("US.BABA260724C180000");
  });

  it("keeps research market-wide and translates both seller strategies", async () => {
    apiMocks.fetchFeature.mockResolvedValue(
      feature([
        {
          owner: { instrumentId: "US.BABA" },
          option: { instrumentId: "US.BABA260724C180000" },
          premium: 240,
        },
      ]),
    );
    const wrapper = mount(OptionResearchPanel, {
      props: {
        market: "US",
        operation: "seller",
        scope: "market",
        presentation: "research",
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();
    const initialPath = String(apiMocks.fetchFeature.mock.calls.at(-1)?.[0]);
    expect(initialPath).toContain("sellerStrategy=covered_call");
    expect(initialPath).not.toContain("underlying=");
    expect(
      wrapper.find(".option-research-panel__native-table").exists(),
    ).toBe(true);
    expect(
      wrapper.find(".option-research-panel__refresh-native").exists(),
    ).toBe(true);

    await wrapper.get('select[aria-label="卖方策略"]').setValue(
      "cash_secured_put",
    );
    await flushPromises();
    const switchedPath = String(apiMocks.fetchFeature.mock.calls.at(-1)?.[0]);
    expect(switchedPath).toContain("sellerStrategy=cash_secured_put");
    expect(switchedPath).not.toContain("underlying=");
  });

  it("keeps inactive underlying panels quiet and renders request failures safely", async () => {
    const initialCallCount = apiMocks.fetchFeature.mock.calls.length;
    const wrapper = mount(OptionResearchPanel, {
      props: {
        market: "US",
        operation: "unusual",
        scope: "underlying",
        underlyingInstrumentId: "",
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();
    expect(apiMocks.fetchFeature).toHaveBeenCalledTimes(initialCallCount);
    expect(wrapper.text()).toContain("正在识别当前正股标的");

    apiMocks.fetchFeature.mockRejectedValueOnce("event request failed");
    await wrapper.setProps({ underlyingInstrumentId: "US.BABA" });
    await flushPromises();
    expect(wrapper.text()).toContain("event request failed");

    const state = setupState<{
      formatCell: (value: unknown) => string;
      securityInstrumentId: (value: unknown) => string;
      entryInstrumentId: (
        entry: Record<string, unknown>,
        kind: "option" | "equity",
      ) => string;
      load: (refresh?: boolean) => Promise<void>;
    }>(wrapper);
    expect(state.formatCell(null)).toBe("—");
    expect(state.formatCell(1234.56789)).toBe("1,234.5679");
    expect(state.formatCell(true)).toBe("是");
    expect(state.formatCell(false)).toBe("否");
    expect(state.formatCell("text")).toBe("text");
    expect(state.formatCell({ unknown: true })).toBe("—");
    expect(state.securityInstrumentId([])).toBe("");
    expect(
      state.securityInstrumentId({ market: "us", code: "baba" }),
    ).toBe("US.BABA");
    expect(
      state.entryInstrumentId(
        { instrumentId: "US.BABA260724C180000" },
        "option",
      ),
    ).toBe("US.BABA260724C180000");

    apiMocks.fetchFeature.mockResolvedValueOnce(feature([]));
    await state.load(true);
    expect(String(apiMocks.fetchFeature.mock.calls.at(-1)?.[0])).toContain(
      "refresh=true",
    );
    await flushPromises();
    expect(wrapper.text()).toContain("当前范围没有符合条件");

    apiMocks.fetchFeature.mockResolvedValueOnce(feature([]));
    await wrapper.get('button[aria-label="刷新"]').trigger("click");
    await flushPromises();
    expect(String(apiMocks.fetchFeature.mock.calls.at(-1)?.[0])).toContain(
      "refresh=true",
    );

    let rejectStale!: (reason?: unknown) => void;
    apiMocks.fetchFeature
      .mockImplementationOnce(
        () =>
          new Promise((_, reject) => {
            rejectStale = reject;
          }),
      )
      .mockResolvedValueOnce(feature([]));
    const staleRequest = state.load();
    await state.load();
    rejectStale("stale request");
    await staleRequest;
    expect(wrapper.text()).not.toContain("stale request");
  });

  it("paginates stable columns and opens the appropriate workspace product", async () => {
    apiMocks.fetchFeature
      .mockResolvedValueOnce(
        feature(
          [
            {
              option: { market: "US", code: "BABA260724C180000" },
              strikePrice: 180,
              volume: 1200,
              sentiment: "bullish",
            },
          ],
          "cursor-2",
        ),
      )
      .mockResolvedValueOnce(
        feature([
          {
            option: { instrumentId: "US.BABA260724P180000" },
            strikePrice: 180,
            volume: 900,
          },
        ]),
      )
      .mockResolvedValueOnce(feature([]));
    const wrapper = mount(OptionResearchPanel, {
      props: { market: "US", operation: "unusual", scope: "market" },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();
    expect(wrapper.text()).toContain("US.BABA260724C180000");
    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("工作区"))!
      .trigger("click");
    expect(wrapper.emitted("openInstrument")?.[0]).toEqual([
      "US.BABA260724C180000",
      "option",
    ]);

    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("下一页"))!
      .trigger("click");
    await flushPromises();
    expect(String(apiMocks.fetchFeature.mock.calls.at(-1)?.[0])).toContain(
      "cursor=cursor-2",
    );
    expect(wrapper.text()).toContain("US.BABA260724P180000");

    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("上一页"))!
      .trigger("click");
    await flushPromises();
    expect(String(apiMocks.fetchFeature.mock.calls.at(-1)?.[0])).not.toContain(
      "cursor=",
    );
    const state = setupState<{
      nextPage: () => void;
      previousPage: () => void;
    }>(wrapper);
    state.nextPage();
    state.previousPage();
  });

  it("reports missing and failed 0DTE drilldown context without losing the source list", async () => {
    const missing = { owner: { instrumentId: "US.BABA" }, name: "Alibaba" };
    apiMocks.fetchFeature.mockResolvedValueOnce(feature([missing]));
    const wrapper = mount(OptionResearchPanel, {
      props: { market: "US", operation: "zero_dte", scope: "market" },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();
    const state = setupState<{
      openDrilldown: (entry: Record<string, unknown>) => Promise<void>;
      drilldownResult: ReturnType<typeof feature> | null;
    }>(wrapper);
    await state.openDrilldown(missing);
    expect(wrapper.text()).toContain("缺少期权链上下文");
    await state.openDrilldown({ ...missing, drilldownContext: {} });
    expect(wrapper.text()).toContain("缺少期权链上下文");

    apiMocks.fetchWithInit.mockRejectedValueOnce("drilldown unavailable");
    const validEntry = {
      ...missing,
      drilldownContext: {
        underlyingInstrumentId: "US.BABA",
        expiryTimestamp: 1784332800,
        chain: { productCode: "BABA" },
      },
    };
    await state.openDrilldown(validEntry);
    await flushPromises();
    expect(wrapper.text()).toContain("drilldown unavailable");
    expect(wrapper.text()).toContain("Alibaba");

    let rejectStale!: (reason?: unknown) => void;
    apiMocks.fetchWithInit
      .mockImplementationOnce(
        () =>
          new Promise((_, reject) => {
            rejectStale = reject;
          }),
      )
      .mockResolvedValueOnce(feature([]));
    const staleRequest = state.openDrilldown(validEntry);
    await state.openDrilldown(validEntry);
    rejectStale("stale drilldown");
    await staleRequest;
    expect(wrapper.text()).not.toContain("stale drilldown");

    state.drilldownResult = feature([
      { option: { instrumentId: "US.BABA260724C180000" } },
    ]);
    await wrapper.vm.$nextTick();
    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("返回 0DTE 标的"))!
      .trigger("click");
    expect(state.drilldownResult).toBeNull();
  });

  it("renders research 0DTE contract details and opens earnings equities", async () => {
    const source = {
      owner: { market: "US", code: "BABA" },
      name: "Alibaba",
      price: 180,
      drilldownContext: {
        underlyingInstrumentId: "US.BABA",
        expiryTimestamp: 1784332800,
        chain: { productCode: "BABA" },
      },
    };
    apiMocks.fetchFeature.mockResolvedValueOnce(feature([source]));
    apiMocks.fetchWithInit.mockResolvedValueOnce(
      feature([
        {
          option: { market: "US", code: "BABA260724C180000" },
          optionType: "call",
          optionPrice: 2.4,
          changeRate: 1.5,
          volume: 1200,
          openInterest: 900,
          iv: 31,
          delta: 0.51,
        },
      ]),
    );
    const wrapper = mount(OptionResearchPanel, {
      props: {
        market: "US",
        operation: "zero_dte",
        presentation: "research",
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();

    await wrapper.get(".option-research-panel__row-action").trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("US.BABA260724C180000");
    expect(wrapper.text()).toContain("1,200");
    expect(wrapper.text()).toContain("0.51");

    apiMocks.fetchFeature.mockResolvedValueOnce(
      feature([{ owner: { market: "US", code: "BABA" }, earningsTime: "盘后" }]),
    );
    await wrapper.setProps({ operation: "earnings" });
    await flushPromises();
    await wrapper.get(".option-research-panel__row-action").trigger("click");
    expect(wrapper.emitted("openInstrument")?.at(-1)).toEqual([
      "US.BABA",
      "equity",
    ]);
  });
});
