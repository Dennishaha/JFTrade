// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick } from "vue";

import {
  type BrokerMaxTradeQuantityResponse,
  type BrokerReadFeatureCapability,
  type BrokerReadFeatureKey,
  type MarketSecurityDetails,
  emptyBrokerMaxTradeQuantity,
  emptySystemStatus,
} from "@/contracts";

const marketProfilesState = vi.hoisted(() => ({
  extendedHoursMarkets: new Set<string>(),
}));

vi.mock("../src/composables/marketProfiles", () => ({
  useMarketProfiles: () => ({
    supportsExtendedHoursForMarket: (market: string | null | undefined) =>
      marketProfilesState.extendedHoursMarkets.has(
        (market ?? "").trim().toUpperCase(),
      ),
  }),
}));

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
  marketProfilesState.extendedHoursMarkets.clear();
  vi.unstubAllGlobals();
  vi.useRealTimers();
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

  it("supports extended-hours guidance and stop-order max trade quantity requests", async () => {
    vi.useFakeTimers();
    marketProfilesState.extendedHoursMarkets.add("US");

    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.includes("/api/v1/settings/ui")) {
        return {
          ok: true,
          json: async () => ({
            ok: true,
            data: {
              appearance: {
                upColor: "#16c784",
                downColor: "#ea3943",
              },
            },
          }),
        };
      }
      if (url.includes("/api/v1/brokers/futu/max-trade-qtys")) {
        return {
          ok: true,
          json: async () => ({
            ok: true,
            data: {
              checkedAt: "2026-05-27T00:00:00.000Z",
              connectivity: "connected",
              lastError: null,
              maxTradeQuantity: {
                accountId: "",
                tradingEnvironment: "SIMULATE",
                market: "US",
                symbol: "US.PENNY",
                orderType: "STOP",
                price: 0.8765,
                maxCashBuy: 180,
                maxCashAndMarginBuy: 250,
                maxPositionSell: 90,
                maxSellShort: 75,
                maxBuyBack: null,
                longRequiredIM: 1000,
                shortRequiredIM: 1200,
                session: "OVERNIGHT",
              },
            },
          }),
        };
      }
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    const { wrapper, store } = mountOrderEntryPanel({
      snapshotPrice: 321.234,
      priceSpread: 0.01,
    });

    applyBrokerReadFeatureSupport(store, {
      market: "US",
      tradingEnvironment: "SIMULATE",
      readFeatureOverrides: {
        maxTradeQuantity: {
          supportedEnvironments: ["SIMULATE", "REAL"],
          requiresPrice: true,
        },
      },
    });
    store.selectWorkspaceInstrument({ market: "US", symbol: "PENNY" });
    store.marketDataSnapshot.value = createSnapshotResult("US", "PENNY", 0.87654, {
      session: "pre",
    });
    store.marketSecurityDetails.value = createSecurityResult(
      createSecurityDetails({
        instrumentId: "US.PENNY",
        market: "US",
        symbol: "PENNY",
        securityType: "OPTION",
        priceSpread: 0,
        currentPrice: 0,
        bidPrice: 0.85,
        askPrice: 0.9,
        lotSize: null,
      }),
    );

    await nextTick();
    await nextTick();

    expect(findPriceInput(wrapper).attributes("step")).toBe("0.0001");
    expect((findPriceInput(wrapper).element as HTMLInputElement).value).toBe("0.8765");
    expect(wrapper.text()).toContain("当前行情时段：盘前");
    expect(wrapper.text()).toContain("下单时段：常规交易时段（RTH）");
    expect(wrapper.text()).toContain(
      "当前不是常规交易时段，RTH 订单通常要等盘中才会撮合。",
    );

    await findOrderTypeSelect(wrapper).setValue("MARKET");
    await nextTick();
    expect(wrapper.text()).toContain(
      "市价单当前没有参考价输入，暂不估算最大可交易数量。",
    );
    await vi.advanceTimersByTimeAsync(300);
    expect(
      fetchMock.mock.calls.some(([request]) =>
        String(request).includes("/api/v1/brokers/futu/max-trade-qtys"),
      ),
    ).toBe(false);

    await findOrderTypeSelect(wrapper).setValue("STOP");
    await nextTick();
    expect(wrapper.text()).toContain("输入止损价后可估算最大可交易数量。");

    await findOrderSessionSelect(wrapper).setValue("OVERNIGHT");
    await nextTick();
    expect(wrapper.text()).toContain(
      "模拟盘夜盘支持通常受限，提交成功也可能暂时不会成交。",
    );

    await findPriceInput(wrapper).setValue(0.87654);
    await findPriceInput(wrapper).trigger("blur");
    await vi.advanceTimersByTimeAsync(300);
    await nextTick();
    await nextTick();

    const maxTradeCall = fetchMock.mock.calls.find(([request]) =>
      String(request).includes("/api/v1/brokers/futu/max-trade-qtys"),
    );
    expect(maxTradeCall).toBeTruthy();
    const maxTradeUrl = String(maxTradeCall?.[0] ?? "");
    expect(maxTradeUrl).toContain("tradingEnvironment=SIMULATE");
    expect(maxTradeUrl).toContain("market=US");
    expect(maxTradeUrl).toContain("symbol=US.PENNY");
    expect(maxTradeUrl).toContain("orderType=STOP");
    expect(maxTradeUrl).toContain("price=0.8765");
    expect(maxTradeUrl).toContain("session=OVERNIGHT");
    expect(wrapper.text()).toContain("买入上限");
    expect(wrapper.text()).toContain("250 张");
    expect(wrapper.text()).toContain("单位：张");
  });

  it("falls back to security prices and resets stale price when the instrument changes", async () => {
    const { wrapper, store } = mountOrderEntryPanel({
      snapshotPrice: 321.234,
      priceSpread: 0.01,
    });

    store.selectWorkspaceInstrument({ market: "US", symbol: "BONDX" });
    store.marketDataSnapshot.value = createSnapshotResult("HK", "00700", 500);
    store.marketSecurityDetails.value = createSecurityResult(
      createSecurityDetails({
        instrumentId: "US.BONDX",
        market: "US",
        symbol: "BONDX",
        securityType: "BOND",
        priceSpread: 0,
        currentPrice: 4.2,
        bidPrice: 0,
        askPrice: 0,
        lotSize: null,
      }),
    );

    await nextTick();
    await nextTick();

    store.marketSecurityDetails.value = createSecurityResult(
      createSecurityDetails({
        instrumentId: "US.BONDX",
        market: "US",
        symbol: "BONDX",
        securityType: "BOND",
        priceSpread: 0,
        currentPrice: 4.21,
        bidPrice: 0,
        askPrice: 0,
        lotSize: null,
      }),
    );

    await nextTick();
    await nextTick();

    expect((findPriceInput(wrapper).element as HTMLInputElement).value).toBe("4.21");
    expect(wrapper.text()).toContain(
      "当前券商未为该交易环境声明最大可交易数量能力。",
    );
    expect(wrapper.text()).toContain("单位：单位");

    store.selectWorkspaceInstrument({ market: "US", symbol: "MID" });
    await nextTick();
    store.marketSecurityDetails.value = createSecurityResult(
      createSecurityDetails({
        instrumentId: "US.MID",
        market: "US",
        symbol: "MID",
        securityType: "BOND",
        priceSpread: 0,
        currentPrice: 0,
        bidPrice: 10,
        askPrice: 12,
        lotSize: null,
      }),
    );
    store.marketDataSnapshot.value = createSnapshotResult("US", "MID", 0);

    await nextTick();
    await nextTick();

    expect((findPriceInput(wrapper).element as HTMLInputElement).value).toBe("11");
  });

  it("validates the instrument, quantity, and stop price before submitting", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    const { wrapper, notifications, workspaceLayout } = mountOrderEntryPanel({
      snapshotPrice: 321.234,
      priceSpread: 0.01,
    });

    await nextTick();

    workspaceLayout.update({ market: "" });
    await nextTick();
    await findSubmitButton(wrapper).trigger("click");
    expect(notifications.items.value[0]?.title).toBe("标的无效");

    notifications.clear();
    workspaceLayout.update({ market: "HK" });
    await nextTick();
    await nextTick();

    await findQuantityInput(wrapper).setValue(0);
    await findSubmitButton(wrapper).trigger("click");
    expect(notifications.items.value[0]?.title).toBe("数量无效");

    notifications.clear();
    await findQuantityInput(wrapper).setValue(100);
    await findOrderTypeSelect(wrapper).setValue("STOP");
    await nextTick();
    await findSubmitButton(wrapper).trigger("click");
    expect(notifications.items.value[0]?.title).toBe("止损价必须大于 0");
    expect(
      fetchMock.mock.calls.some(([request]) =>
        String(request).includes("/api/v1/execution/orders"),
      ),
    ).toBe(false);
  });

  it("reports sell-side internal ids, broker rejections, and transport failures", async () => {
    let executionCalls = 0;
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.includes("/api/v1/settings/ui")) {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            ok: true,
            data: {
              appearance: {
                upColor: "#16c784",
                downColor: "#ea3943",
              },
            },
          }),
        });
      }
      if (!url.includes("/api/v1/execution/orders")) {
        throw new Error(`Unexpected request: ${url}`);
      }
      executionCalls += 1;
      if (executionCalls === 1) {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            ok: true,
            data: {
              accepted: true,
              internalOrderId: "io-9",
              brokerOrderId: "",
            },
          }),
        });
      }
      if (executionCalls === 2) {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            ok: true,
            data: {
              accepted: false,
              message: "   ",
              brokerErrorCode: "BROKER_REJECT",
            },
          }),
        });
      }
      return Promise.reject("transport down");
    });
    vi.stubGlobal("fetch", fetchMock);

    const { wrapper, notifications } = mountOrderEntryPanel({
      snapshotPrice: 321.234,
      priceSpread: 0.01,
    });

    await nextTick();
    await wrapper.get(".tv-order-side-seg .is-sell").trigger("click");
    await nextTick();

    expect(findSubmitButton(wrapper).text()).toContain("卖出 00700");

    await findSubmitButton(wrapper).trigger("click");
    await nextTick();
    await nextTick();
    expect(wrapper.text()).toContain("下单成功：已提交订单，内部单号 io-9");
    expect(notifications.items.value[0]?.level).toBe("success");

    notifications.clear();
    await findSubmitButton(wrapper).trigger("click");
    await nextTick();
    await nextTick();
    expect(wrapper.text()).toContain("下单失败：BROKER_REJECT");
    expect(notifications.items.value[0]?.level).toBe("error");

    notifications.clear();
    await findSubmitButton(wrapper).trigger("click");
    await nextTick();
    await nextTick();
    expect(wrapper.text()).toContain("下单失败：下单请求失败，请稍后重试。");
    expect(notifications.items.value[0]?.level).toBe("error");
  });

  it("rounds tiny limit prices down to zero after alignment", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    const { wrapper, notifications } = mountOrderEntryPanel({
      snapshotPrice: 321.234,
      priceSpread: 0.01,
    });

    await nextTick();
    await wrapper.get(".tv-order-side-seg .is-buy").trigger("click");
    await findTifSelect(wrapper).setValue("GTC");
    await findPriceInput(wrapper).setValue(0.004);
    await findSubmitButton(wrapper).trigger("click");

    expect(notifications.items.value[0]?.title).toBe("价格必须大于 0");
    expect(
      fetchMock.mock.calls.some(([request]) =>
        String(request).includes("/api/v1/execution/orders"),
      ),
    ).toBe(false);
  });
});

function mountOrderEntryPanel(options: {
  snapshotPrice: number;
  priceSpread: number;
  maxTradeQuantity?: BrokerMaxTradeQuantityResponse;
  colors?: { upColor: string; downColor: string };
}) {
  let store: ReturnType<typeof provideConsoleDataStore> | null = null;
  let notifications: ReturnType<typeof provideNotificationsStore> | null = null;
  let workspaceLayout:
    | ReturnType<typeof provideWorkspaceTradingPreferencesStore>
    | null = null;

  const Host = defineComponent({
    setup() {
      workspaceLayout = provideWorkspaceTradingPreferencesStore();
      workspaceLayout.update({ market: "HK", symbol: "00700" });
      notifications = provideNotificationsStore();
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
  if (notifications == null) {
    throw new Error("Failed to create notifications store.");
  }
  if (workspaceLayout == null) {
    throw new Error("Failed to create workspace layout store.");
  }

  return { wrapper, store, notifications, workspaceLayout };
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
  const button = wrapper.find("button.tv-btn");
  if (!button.exists()) {
    throw new Error("Submit button not found.");
  }
  return button;
}

function findQuantityInput(wrapper: ReturnType<typeof mount>) {
  const input = wrapper.findAll('input[type="number"]').find(
    (candidate) => candidate.attributes("min") === "1",
  );
  if (input == null) {
    throw new Error("Quantity input not found.");
  }
  return input;
}

function findOrderTypeSelect(wrapper: ReturnType<typeof mount>) {
  const select = wrapper.findAll("select").find((candidate) =>
    candidate.findAll("option").some((option) => option.attributes("value") === "STOP_LIMIT"),
  );
  if (select == null) {
    throw new Error("Order type select not found.");
  }
  return select;
}

function findOrderSessionSelect(wrapper: ReturnType<typeof mount>) {
  const select = wrapper.findAll("select").find((candidate) =>
    candidate.findAll("option").some((option) => option.attributes("value") === "OVERNIGHT"),
  );
  if (select == null) {
    throw new Error("Order session select not found.");
  }
  return select;
}

function findTifSelect(wrapper: ReturnType<typeof mount>) {
  const select = wrapper.findAll("select").find((candidate) =>
    candidate.findAll("option").some((option) => option.attributes("value") === "IOC"),
  );
  if (select == null) {
    throw new Error("TIF select not found.");
  }
  return select;
}

function createReadFeatures(
  overrides: Partial<
    Record<BrokerReadFeatureKey, Partial<BrokerReadFeatureCapability>>
  > = {},
): Record<BrokerReadFeatureKey, BrokerReadFeatureCapability> {
  return {
    funds: {
      supportedEnvironments: ["SIMULATE", "REAL"],
      ...overrides.funds,
    },
    positions: {
      supportedEnvironments: ["SIMULATE", "REAL"],
      ...overrides.positions,
    },
    orders: {
      supportedEnvironments: ["SIMULATE", "REAL"],
      supportsHistory: true,
      ...overrides.orders,
    },
    fills: {
      supportedEnvironments: ["SIMULATE", "REAL"],
      supportsHistory: true,
      ...overrides.fills,
    },
    cashFlows: {
      supportedEnvironments: ["REAL"],
      requiresClearingDate: true,
      ...overrides.cashFlows,
    },
    orderFees: {
      supportedEnvironments: ["REAL"],
      requiresOrderIdEx: true,
      ...overrides.orderFees,
    },
    marginRatios: {
      supportedEnvironments: ["REAL"],
      requiresSymbols: true,
      ...overrides.marginRatios,
    },
    maxTradeQuantity: {
      supportedEnvironments: ["SIMULATE", "REAL"],
      requiresPrice: true,
      ...overrides.maxTradeQuantity,
    },
    orderBook: {
      supportedEnvironments: ["SIMULATE", "REAL"],
      ...overrides.orderBook,
    },
  };
}

function applyBrokerReadFeatureSupport(
  store: ReturnType<typeof provideConsoleDataStore>,
  options: {
    market: string;
    tradingEnvironment: "SIMULATE" | "REAL";
    readFeatureOverrides?: Partial<
      Record<BrokerReadFeatureKey, Partial<BrokerReadFeatureCapability>>
    >;
  },
): void {
  store.systemStatus.value = {
    ...emptySystemStatus,
    defaultBroker: "futu",
    defaultTradingEnvironment: options.tradingEnvironment,
    broker: {
      ...emptySystemStatus.broker,
      id: "futu",
      displayName: "Test Broker",
      environments: ["SIMULATE", "REAL"],
      capabilities: [
        {
          market: options.market,
          supportsQuote: true,
          supportsTrade: true,
          readFeatures: createReadFeatures(options.readFeatureOverrides),
        },
      ],
    },
  };
}

function createSnapshotResult(
  market: string,
  symbol: string,
  price: number,
  overrides: Partial<MarketDataSnapshotQueryResult["snapshot"]> = {},
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
      ...overrides,
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
