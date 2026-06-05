// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick } from "vue";

import {
  type BrokerMaxTradeQuantityResponse,
  type MarketSecurityDetails,
  emptyBrokerMaxTradeQuantity,
} from "@jftrade/ui-contracts";

import OrderEntryPanel from "../src/components/workspace/OrderEntryPanel.vue";
import type {
  MarketDataSnapshotQueryResult,
  MarketSecurityDetailsQueryResult,
} from "../src/composables/marketDataRealtime";
import { provideConsoleDataStore } from "../src/composables/useConsoleData";
import { provideNotificationsStore } from "../src/composables/useNotifications";
import { provideUIColorPreferencesStore } from "../src/composables/useUIColorPreferences";
import { provideWorkspaceTradingPreferencesStore } from "../src/composables/useWorkspaceLayout";

afterEach(() => {
  window.localStorage?.clear();
  vi.unstubAllGlobals();
});

describe("OrderEntryPanel", () => {
  it("defaults and syncs limit price from market data using the security price spread", async () => {
    const { wrapper, store } = mountOrderEntryPanel({
      snapshotPrice: 321.234,
      priceSpread: 0.01,
    });

    await nextTick();

    const priceInput = findPriceInput(wrapper);
    expect((priceInput.element as HTMLInputElement).value).toBe("321.23");
    expect(priceInput.attributes("step")).toBe("0.01");

    store.marketDataSnapshot.value = createSnapshotResult("HK", "00700", 322.777);
    await nextTick();
    await wrapper.find('button[title="同步市场价格"]').trigger("click");

    expect((priceInput.element as HTMLInputElement).value).toBe("322.78");
  });

  it("blocks non-positive prices before submitting orders", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    const { wrapper } = mountOrderEntryPanel({
      snapshotPrice: 321.234,
      priceSpread: 0.01,
    });
    await nextTick();

    await findPriceInput(wrapper).setValue(0);
    await findSubmitButton(wrapper).trigger("click");

    expect(
      fetchMock.mock.calls.some(([request]) =>
        String(request).includes("/api/v1/execution/orders"),
      ),
    ).toBe(false);
  });

  it("submits explicit market and code payloads", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        ok: true,
        data: { accepted: true, internalOrderId: "io-1", brokerOrderId: "bo-1" },
      }),
    });
    vi.stubGlobal("fetch", fetchMock);

    const { wrapper } = mountOrderEntryPanel({
      snapshotPrice: 321.234,
      priceSpread: 0.01,
    });

    await nextTick();
    await findSubmitButton(wrapper).trigger("click");
    await nextTick();

    const orderCall = fetchMock.mock.calls.find((entry) =>
      String(entry[0]).includes("/api/v1/execution/orders"),
    );
    expect(orderCall).toBeTruthy();

    const init = orderCall?.[1] as RequestInit | undefined;
    expect(init?.method).toBe("POST");
    const payload = JSON.parse(String(init?.body));
    expect(payload.market).toBe("HK");
    expect(payload.code).toBe("00700");
    expect(payload.symbol).toBe("HK.00700");
  });

  it("treats missing accepted as a failed order response", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        ok: true,
        data: { internalOrderId: "io-1", brokerOrderId: "bo-1" },
      }),
    });
    vi.stubGlobal("fetch", fetchMock);

    const { wrapper } = mountOrderEntryPanel({
      snapshotPrice: 321.234,
      priceSpread: 0.01,
    });

    await nextTick();
    await findSubmitButton(wrapper).trigger("click");
    await nextTick();
    await nextTick();

    expect(wrapper.text()).toContain("下单失败：券商未接受该订单。");
  });

  it("clears stale order feedback before a new submit finishes", async () => {
    let resolveSecond: ((value: unknown) => void) | null = null;
    let executionCalls = 0;
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (!url.includes("/api/v1/execution/orders")) {
        return Promise.resolve({
          ok: true,
          json: async () => ({ ok: true, data: emptyBrokerMaxTradeQuantity }),
        });
      }
      executionCalls += 1;
      if (executionCalls === 1) {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            ok: true,
            data: { accepted: false, message: "first rejected" },
          }),
        });
      }
      return new Promise((resolve) => {
        resolveSecond = resolve;
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const { wrapper } = mountOrderEntryPanel({
      snapshotPrice: 321.234,
      priceSpread: 0.01,
    });

    await nextTick();
    await findSubmitButton(wrapper).trigger("click");
    await nextTick();
    await nextTick();
    expect(wrapper.text()).toContain("first rejected");

    await findSubmitButton(wrapper).trigger("click");
    await nextTick();
    expect(wrapper.text()).not.toContain("first rejected");

    resolveSecond?.({
      ok: true,
      json: async () => ({
        ok: true,
        data: { accepted: true, internalOrderId: "io-2" },
      }),
    });
    await nextTick();
    await nextTick();
  });

  it("explains max trade quantity session, unit, and empty IM values", async () => {
    const { wrapper } = mountOrderEntryPanel({
      snapshotPrice: 321.234,
      priceSpread: 0.01,
      maxTradeQuantity: {
        checkedAt: "2026-05-27T00:00:00.000Z",
        connectivity: "connected",
        lastError: null,
        maxTradeQuantity: {
          accountId: "REAL-001",
          tradingEnvironment: "REAL",
          market: "HK",
          symbol: "HK.00700",
          orderType: "LIMIT",
          price: 321.23,
          maxCashBuy: 100,
          maxCashAndMarginBuy: 200,
          maxPositionSell: 50,
          maxSellShort: null,
          maxBuyBack: null,
          longRequiredIM: null,
          shortRequiredIM: null,
          session: "RTH",
        },
      },
    });

    await nextTick();

    expect(wrapper.text()).toContain("常规交易时段（RTH）");
    expect(wrapper.text()).toContain("单位：股 · 每手 100 股");
    expect(wrapper.text()).toContain("多头初始保证金 股票通常不返回");
    expect(wrapper.text()).toContain("空头初始保证金 股票通常不返回");
  });

  it("applies custom rise/fall colors through the canonical tv variables", async () => {
    const { wrapper } = mountOrderEntryPanel({
      snapshotPrice: 321.234,
      priceSpread: 0.01,
      colors: {
        upColor: "#0055aa",
        downColor: "#aa2200",
      },
    });

    await nextTick();

    const style = wrapper.find("section").attributes("style") ?? "";
    expect(style).not.toContain("--market-up");
    expect(style).not.toContain("--market-down");
    expect(document.documentElement.style.getPropertyValue("--tv-up")).toBe("#0055aa");
    expect(document.documentElement.style.getPropertyValue("--tv-down")).toBe("#aa2200");
    expect(wrapper.find(".tv-order-side-seg .is-buy").classes()).toContain("is-active");
  });
});

function mountOrderEntryPanel(options: {
  snapshotPrice: number;
  priceSpread: number;
  maxTradeQuantity?: BrokerMaxTradeQuantityResponse;
  colors?: { upColor: string; downColor: string };
}) {
  let store: ReturnType<typeof provideConsoleDataStore> | null = null;

  const Host = defineComponent({
    setup() {
      const workspaceLayout = provideWorkspaceTradingPreferencesStore();
      workspaceLayout.update({ market: "HK", symbol: "00700" });
      provideNotificationsStore();
      const colorStore = provideUIColorPreferencesStore();
      if (options.colors != null) {
        colorStore.update(options.colors);
      }
      store = provideConsoleDataStore(workspaceLayout);
      store.marketDataSnapshot.value = createSnapshotResult("HK", "00700", options.snapshotPrice);
      store.marketSecurityDetails.value = createSecurityResult(
        createSecurityDetails({ priceSpread: options.priceSpread }),
      );
      if (options.maxTradeQuantity != null) {
        store.brokerMaxTradeQuantity.value = options.maxTradeQuantity;
      }
      return () => h(OrderEntryPanel);
    },
  });

  const wrapper = mount(Host, {
    global: {
      plugins: [createPinia()],
    },
  });

  if (store == null) {
    throw new Error("Failed to create console data store.");
  }

  return { wrapper, store };
}

function findPriceInput(wrapper: ReturnType<typeof mount>) {
  const input = wrapper.findAll('input[type="number"]').find(
    (candidate) => candidate.attributes("step") !== undefined,
  );
  if (input == null) {
    throw new Error("Price input not found.");
  }
  return input;
}

function findSubmitButton(wrapper: ReturnType<typeof mount>) {
  const button = wrapper.findAll("button").find((candidate) =>
    candidate.text().includes("买入 00700"),
  );
  if (button == null) {
    throw new Error("Submit button not found.");
  }
  return button;
}

function createSnapshotResult(
  market: string,
  symbol: string,
  price: number,
): MarketDataSnapshotQueryResult {
  const instrumentId = `${market}.${symbol}`;
  return {
    request: { market, symbol, instrumentId },
    snapshot: {
      price,
      bid: price - 0.1,
      ask: price + 0.1,
      previousClosePrice: price - 1,
      volume: 1000,
      turnover: 100000,
      at: "2026-05-27T00:00:00.000Z",
      session: "regular",
    },
    meta: {
      instrumentId,
      source: "test",
      resolvedAt: "2026-05-27T00:00:00.000Z",
      fromCache: false,
    },
  };
}

function createSecurityResult(
  security: MarketSecurityDetails,
): MarketSecurityDetailsQueryResult {
  return {
    request: {
      market: security.market,
      symbol: security.symbol,
      instrumentId: security.instrumentId,
    },
    security,
    meta: {
      instrumentId: security.instrumentId,
      source: "test",
      resolvedAt: "2026-05-27T00:00:00.000Z",
      fromCache: false,
    },
  };
}

function createSecurityDetails(
  overrides: Partial<MarketSecurityDetails> = {},
): MarketSecurityDetails {
  return {
    instrumentId: "HK.00700",
    market: "HK",
    symbol: "00700",
    securityId: 1,
    name: "Tencent",
    securityType: "STOCK",
    exchangeType: "HK",
    listTime: "2004-06-16",
    listTimestamp: 1087315200,
    delisting: false,
    lotSize: 100,
    isSuspend: false,
    priceSpread: 0.01,
    updateTime: "2026-05-27 09:30:00",
    updateTimestamp: 1779845400,
    highPrice: 322,
    openPrice: 320,
    lowPrice: 319,
    lastClosePrice: 318,
    currentPrice: 321,
    volume: 1000,
    turnover: 100000,
    turnoverRate: 1,
    extended: null,
    equity: null,
    warrant: null,
    option: null,
    index: null,
    plate: null,
    future: null,
    trust: null,
    ...overrides,
  };
}
