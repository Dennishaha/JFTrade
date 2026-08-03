// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick } from "vue";

import type { MarketSecurityDetails } from "@/types";
import { emptyRealTradeApprovals } from "@/types";

const marketProfileState = vi.hoisted(() => ({
  extendedHoursMarkets: new Set<string>(),
}));

vi.mock("@/composables/market-data/marketProfiles", () => ({
  useMarketProfiles: () => ({
    supportsExtendedHoursForMarket: (market: string | null | undefined) =>
      marketProfileState.extendedHoursMarkets.has(
        (market ?? "").trim().toUpperCase(),
      ),
  }),
}));

import { useOrderEntryPanel } from "@/composables/trading/useOrderEntryPanel";
import { provideConsoleDataStore } from "@/composables/workspace/useConsoleData";
import { provideNotificationsStore } from "@/composables/shared/useNotifications";
import { provideWorkspaceTradingPreferencesStore } from "@/composables/workspace/useWorkspaceLayout";

type Panel = ReturnType<typeof useOrderEntryPanel>;
type ConsoleStore = ReturnType<typeof provideConsoleDataStore>;
type NotificationsStore = ReturnType<typeof provideNotificationsStore>;
type PreferencesStore = ReturnType<typeof provideWorkspaceTradingPreferencesStore>;

const mountedWrappers: Array<{ unmount: () => void }> = [];

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterEach(() => {
  for (const wrapper of mountedWrappers.splice(0)) wrapper.unmount();
  marketProfileState.extendedHoursMarkets.clear();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

function mountPanel(
  options: {
    market?: string;
    symbol?: string;
    price?: number;
    session?: string;
    security?: Partial<MarketSecurityDetails>;
    tradingEnvironment?: "SIMULATE" | "REAL";
  } = {},
): {
  panel: Panel;
  store: ConsoleStore;
  notifications: NotificationsStore;
  preferences: PreferencesStore;
} {
  const market = options.market ?? "HK";
  const symbol = options.symbol ?? "00700";
  const price = options.price ?? 320;
  const session = options.session ?? "regular";
  const instrumentId = `${market}.${symbol}`;
  let panel: Panel | null = null;
  let store: ConsoleStore | null = null;
  let notifications: NotificationsStore | null = null;
  let preferences: PreferencesStore | null = null;

  const PanelHost = defineComponent({
    setup() {
      panel = useOrderEntryPanel();
      return () => h("div");
    },
  });
  const Host = defineComponent({
    setup() {
      preferences = provideWorkspaceTradingPreferencesStore();
      preferences.update({ market, symbol });
      notifications = provideNotificationsStore();
      store = provideConsoleDataStore(preferences);
      const environment = options.tradingEnvironment ?? "SIMULATE";
      store.systemStatus.value = {
        ...store.systemStatus.value,
        defaultTradingEnvironment: environment,
        realTradingEnabled: environment === "REAL",
      };
      store.marketDataSnapshot.value = {
        request: { market, symbol, instrumentId },
        snapshot: {
          price,
          bid: price - 0.1,
          ask: price + 0.1,
          previousClosePrice: price - 1,
          volume: 100,
          turnover: 1000,
          at: "2026-07-16T09:30:00.000Z",
          session,
        },
        meta: {
          instrumentId,
          source: "test",
          resolvedAt: "2026-07-16T09:30:00.000Z",
          fromCache: false,
        },
      };
      store.marketSecurityDetails.value = {
        request: { market, symbol, instrumentId },
        security: {
          instrumentId,
          market,
          symbol,
          securityId: 1,
          name: symbol,
          securityType: "STOCK",
          exchangeType: market,
          listTime: "2020-01-01",
          listTimestamp: 1,
          delisting: false,
          lotSize: 100,
          isSuspend: false,
          priceSpread: 0.01,
          updateTime: "2026-07-16 09:30:00",
          updateTimestamp: 1,
          highPrice: price,
          openPrice: price,
          lowPrice: price,
          lastClosePrice: price,
          currentPrice: price,
          volume: 100,
          turnover: 1000,
          turnoverRate: 1,
          extended: null,
          equity: null,
          warrant: null,
          option: null,
          index: null,
          plate: null,
          future: null,
          trust: null,
          ...options.security,
        },
        meta: {
          instrumentId,
          source: "test",
          resolvedAt: "2026-07-16T09:30:00.000Z",
          fromCache: false,
        },
      };
      return () => h(PanelHost);
    },
  });
  const wrapper = mount(Host);
  mountedWrappers.push(wrapper);
  if (panel == null || store == null || notifications == null || preferences == null) {
    throw new Error("order entry panel was not initialized");
  }
  return { panel, store, notifications, preferences };
}

function enableMaxTradeQuantityCapability(store: ConsoleStore, market: string): void {
  store.systemStatus.value = {
    ...store.systemStatus.value,
    broker: {
      ...store.systemStatus.value.broker,
      capabilities: [
        {
          market,
          supportsQuote: true,
          supportsTrade: true,
          readFeatures: {
            maxTradeQuantity: { supportedEnvironments: ["SIMULATE"] },
          },
        },
      ],
    },
  };
}

function envelope(data: unknown): Response {
  return {
    ok: true,
    json: async () => ({ ok: true, data }),
  } as Response;
}

function fetchCallBody(fetchMock: ReturnType<typeof vi.fn>, urlPart: string) {
  const call = fetchMock.mock.calls.find(([input]) =>
    String(input).includes(urlPart),
  );
  if (call == null) return null;
  return JSON.parse(String((call[1] as RequestInit).body)) as Record<string, unknown>;
}

describe("useOrderEntryPanel order-type derivation", () => {
  it("maps each order type onto its limit/stop price gates", () => {
    const { panel } = mountPanel();

    expect(panel.isLimit.value).toBe(true);
    expect(panel.isStop.value).toBe(false);

    panel.orderType.value = "MARKET";
    expect(panel.isLimit.value).toBe(false);
    expect(panel.isStop.value).toBe(false);

    panel.orderType.value = "STOP";
    expect(panel.isLimit.value).toBe(false);
    expect(panel.isStop.value).toBe(true);

    panel.orderType.value = "STOP_LIMIT";
    expect(panel.isLimit.value).toBe(true);
    expect(panel.isStop.value).toBe(true);
  });

  it("syncs the untouched limit price from the aligned market price", () => {
    const { panel, store } = mountPanel({
      price: 320.2347,
      security: { priceSpread: 0.001 },
    });

    expect(panel.price.value).toBe(320.235);
    expect(panel.hasEditedPrice.value).toBe(false);

    panel.markPriceEdited();
    expect(panel.hasEditedPrice.value).toBe(true);
    panel.price.value = 100;
    store.marketDataSnapshot.value = {
      ...store.marketDataSnapshot.value!,
      snapshot: {
        ...store.marketDataSnapshot.value!.snapshot,
        price: 400,
      },
    };
    expect(panel.price.value).toBe(100);
  });

  it("resolves the price step from security spread, then market conventions", () => {
    const hk = mountPanel({ security: { priceSpread: 0.01 } });
    expect(hk.panel.resolveOrderPriceStep(320)).toBe(0.01);

    const hkNoSpread = mountPanel({ security: { priceSpread: 0 } });
    expect(hkNoSpread.panel.resolveOrderPriceStep(320)).toBe(0.001);

    const usPenny = mountPanel({
      market: "US",
      symbol: "PENNY",
      price: 0.5,
      security: { priceSpread: 0 },
    });
    expect(usPenny.panel.resolveOrderPriceStep(0)).toBe(0.0001);

    const usDollar = mountPanel({
      market: "US",
      symbol: "AAPL",
      price: 190,
      security: { priceSpread: 0 },
    });
    expect(usDollar.panel.resolveOrderPriceStep(0)).toBe(0.01);
    expect(usDollar.panel.alignPriceToStep(320.2347, 0.001)).toBe(320.235);
    expect(usDollar.panel.alignPriceToStep(0, 0.01)).toBe(0);
    expect(usDollar.panel.alignPriceToStep(Number.NaN, 0.01)).toBe(0);
  });

  it("derives product class from preferences before falling back to the security", () => {
    const { panel, preferences } = mountPanel({
      security: { securityType: "STOCK" },
    });
    expect(panel.productClass.value).toBe("equity");
    expect(panel.isEventContract.value).toBe(false);

    preferences.update({ productClass: "future" });
    expect(panel.productClass.value).toBe("future");

    preferences.update({ productClass: "unknown", symbol: "EC.CPI" });
    expect(panel.productClass.value).toBe("event_contract");

    preferences.update({ marketSegment: "securities", productClass: "equity" });
    expect(panel.productClass.value).toBe("event_contract");

    preferences.update({ marketSegment: "prediction" });
    expect(panel.productClass.value).toBe("event_contract");
    expect(panel.tradeQuantityUnit.value).toBe("金额");
  });

  it("forces limit pricing when the instrument becomes an event contract", async () => {
    const { panel, preferences } = mountPanel({
      security: { securityType: "STOCK" },
    });
    panel.orderType.value = "MARKET";
    await nextTick();

    preferences.update({ marketSegment: "prediction" });
    await nextTick();

    expect(panel.isEventContract.value).toBe(true);
    expect(panel.orderType.value).toBe("LIMIT");
  });

  it("describes the share unit with its lot size", () => {
    const { panel } = mountPanel({ security: { lotSize: 200 } });

    expect(panel.tradeQuantityUnit.value).toBe("股");
    expect(panel.tradeQuantityUnitHint.value).toBe("单位：股 · 每手 200 股");
  });
});

describe("useOrderEntryPanel market price fallback", () => {
  it("ignores a snapshot that belongs to another instrument", async () => {
    const { panel, store, preferences } = mountPanel({ price: 320 });
    expect(panel.latestMarketPrice.value).toBe(320);

    preferences.update({ symbol: "00941" });
    await nextTick();

    expect(panel.latestSnapshot.value).toBeNull();
    expect(panel.latestMarketPrice.value).toBe(320);

    const details = store.marketSecurityDetails.value!;
    store.marketSecurityDetails.value = {
      ...details,
      security: { ...details.security, currentPrice: 0, bidPrice: 317.9, askPrice: 318.1 },
    };
    await nextTick();
    expect(panel.latestMarketPrice.value).toBe(318);

    store.marketSecurityDetails.value = {
      ...store.marketSecurityDetails.value!,
      security: {
        ...store.marketSecurityDetails.value!.security,
        bidPrice: 0,
        askPrice: 0,
      },
    };
    await nextTick();
    expect(panel.latestMarketPrice.value).toBeNull();
  });

  it("warns instead of overwriting the price when no market price exists", () => {
    const { panel, store, notifications } = mountPanel();
    store.marketDataSnapshot.value = null;
    const details = store.marketSecurityDetails.value!;
    store.marketSecurityDetails.value = {
      ...details,
      security: { ...details.security, currentPrice: 0, bidPrice: 0, askPrice: 0 },
    };

    panel.price.value = 5;
    panel.syncMarketPriceToPriceInput();

    expect(panel.price.value).toBe(5);
    expect(notifications.items.value.at(0)).toMatchObject({
      level: "warn",
      title: "暂无可同步的市场价格",
    });
  });
});

describe("useOrderEntryPanel payload validation", () => {
  it("rejects an empty instrument before any broker call", () => {
    const { panel, preferences, notifications } = mountPanel();
    preferences.update({ market: "", symbol: "" });

    expect(panel.activeInstrument.value).toBeNull();
    expect(panel.validateAndBuildExecutionPayload()).toBeNull();
    expect(notifications.items.value.at(0)).toMatchObject({
      level: "warn",
      title: "标的无效",
    });
  });

  it("rejects non-positive quantity and missing protective prices", () => {
    const { panel, notifications } = mountPanel();

    panel.quantity.value = 0;
    expect(panel.validateAndBuildExecutionPayload()).toBeNull();
    expect(notifications.items.value.at(0)).toMatchObject({
      level: "warn",
      title: "数量无效",
    });

    panel.quantity.value = 100;
    panel.price.value = 0;
    expect(panel.validateAndBuildExecutionPayload()).toBeNull();
    expect(notifications.items.value.at(0)).toMatchObject({
      level: "warn",
      title: "价格必须大于 0",
    });

    panel.orderType.value = "STOP";
    panel.stopPrice.value = 0;
    expect(panel.validateAndBuildExecutionPayload()).toBeNull();
    expect(notifications.items.value.at(0)).toMatchObject({
      level: "warn",
      title: "止损价必须大于 0",
    });
  });

  it("builds a simulate limit payload with a stable client order id", () => {
    const { panel } = mountPanel();
    panel.price.value = 320;

    const payload = panel.validateAndBuildExecutionPayload();

    expect(payload).toMatchObject({
      brokerId: "futu",
      tradingEnvironment: "SIMULATE",
      accountId: "",
      market: "HK",
      code: "00700",
      symbol: "HK.00700",
      side: "BUY",
      orderType: "LIMIT",
      timeInForce: "DAY",
      quantity: 100,
      productClass: "equity",
      quantityMode: "units",
      orderKind: "single",
      env: "SIMULATE",
      price: 320,
    });
    expect(payload?.clientOrderId).toMatch(/^jftrade-/);
    expect(payload?.clientOrderId).toBe(panel.draftClientOrderId.value);
    expect(payload?.session).toBeUndefined();
    expect(payload?.stopPrice).toBeUndefined();
  });

  it("attaches the order session only when the broker market supports it", () => {
    marketProfileState.extendedHoursMarkets.add("US");
    const { panel } = mountPanel({ market: "US", symbol: "AAPL", price: 190 });
    panel.price.value = 190;
    panel.orderSession.value = "ETH";

    expect(panel.supportsOrderSessionSelection.value).toBe(true);
    const payload = panel.validateAndBuildExecutionPayload();
    expect(payload?.session).toBe("ETH");
  });

  it("builds an amount-based prediction payload for event contracts", () => {
    const { panel, preferences } = mountPanel({
      market: "US",
      symbol: "EC.CPI",
      price: 0.55,
    });
    preferences.update({ marketSegment: "prediction" });
    panel.quantity.value = 25;
    panel.price.value = 0.55;
    panel.predictionSide.value = "NO";

    const payload = panel.validateAndBuildExecutionPayload();

    expect(payload).toMatchObject({
      productClass: "event_contract",
      quantityMode: "amount",
      orderKind: "event_single",
      amount: 25,
      predictionSide: "NO",
      price: 0.55,
    });
  });

  it("rotates the client order id when order parameters change", async () => {
    const { panel } = mountPanel();

    const firstId = panel.currentClientOrderId();
    expect(firstId).toMatch(/^jftrade-/);
    expect(panel.currentClientOrderId()).toBe(firstId);

    panel.setSide("SELL");
    await nextTick();

    expect(panel.side.value).toBe("SELL");
    expect(panel.draftClientOrderId.value).toBe("");
    const secondId = panel.currentClientOrderId();
    expect(secondId).toMatch(/^jftrade-/);
    expect(secondId).not.toBe(firstId);
  });
});

describe("useOrderEntryPanel max trade quantity", () => {
  it("explains when the broker has no max-quantity capability", () => {
    const { panel } = mountPanel();

    expect(panel.supportsBrokerMaxTradeQuantity.value).toBe(false);
    expect(panel.maxTradeQuantityHint.value).toBe(
      "当前券商未为该交易环境声明最大可交易数量能力。",
    );
  });

  it("guides the user to a reference price before estimating", () => {
    const { panel, store } = mountPanel();
    enableMaxTradeQuantityCapability(store, "HK");

    expect(panel.supportsBrokerMaxTradeQuantity.value).toBe(true);
    expect(panel.maxTradeQuantityRequiresPrice.value).toBe(true);
    expect(panel.maxTradeQuantityHint.value).toBe(
      "根据当前账户、订单类型和价格估算最大可交易数量。",
    );

    panel.orderType.value = "MARKET";
    expect(panel.maxTradeQuantityReferencePrice.value).toBe(0);
    expect(panel.maxTradeQuantityHint.value).toBe(
      "市价单当前没有参考价输入，暂不估算最大可交易数量。",
    );

    panel.orderType.value = "STOP";
    panel.stopPrice.value = 0;
    expect(panel.maxTradeQuantityHint.value).toBe("输入止损价后可估算最大可交易数量。");

    panel.orderType.value = "LIMIT";
    panel.price.value = 0;
    expect(panel.maxTradeQuantityHint.value).toBe("输入价格后可估算最大可交易数量。");
  });

  it("prefers margin buying power and short-selling capacity for the headline value", () => {
    const { panel, store } = mountPanel();
    store.brokerMaxTradeQuantity.value = {
      checkedAt: "2026-07-16T09:30:00Z",
      connectivity: "connected",
      lastError: null,
      maxTradeQuantity: {
        accountId: "sim",
        tradingEnvironment: "SIMULATE",
        market: "HK",
        symbol: "HK.00700",
        orderType: "LIMIT",
        price: 320,
        maxCashBuy: 1000,
        maxCashAndMarginBuy: 1500,
        maxPositionSell: 80,
        maxSellShort: 60,
        maxBuyBack: null,
        longRequiredIM: null,
        shortRequiredIM: null,
        session: null,
      },
    };

    expect(panel.maxTradeQuantityPrimaryLabel.value).toBe("买入上限");
    expect(panel.maxTradeQuantityPrimaryValue.value).toBe(1500);

    panel.setSide("SELL");
    expect(panel.maxTradeQuantityPrimaryLabel.value).toBe("卖出上限");
    expect(panel.maxTradeQuantityPrimaryValue.value).toBe(60);
  });

  it("queries the broker max-quantity endpoint with the aligned reference price", async () => {
    vi.useFakeTimers();
    const fetchMock = vi.fn().mockResolvedValue(envelope({}));
    vi.stubGlobal("fetch", fetchMock);
    const { panel, store } = mountPanel();
    enableMaxTradeQuantityCapability(store, "HK");
    panel.price.value = 320;

    await vi.advanceTimersByTimeAsync(250);

    const call = fetchMock.mock.calls.find(([input]) =>
      String(input).includes("max-trade-qtys"),
    );
    expect(call).toBeDefined();
    const url = String(call![0]);
    expect(url).toContain("/api/v1/brokers/futu/max-trade-qtys");
    expect(url).toContain("tradingEnvironment=SIMULATE");
    expect(url).toContain("market=HK");
    expect(url).toContain("symbol=HK.00700");
    expect(url).toContain("orderType=LIMIT");
    expect(url).toContain("price=320");
    expect(url).not.toContain("session=");
  });

  it("includes the order session for extended-hours markets", async () => {
    vi.useFakeTimers();
    marketProfileState.extendedHoursMarkets.add("US");
    const fetchMock = vi.fn().mockResolvedValue(envelope({}));
    vi.stubGlobal("fetch", fetchMock);
    const { panel, store } = mountPanel({ market: "US", symbol: "AAPL", price: 190 });
    enableMaxTradeQuantityCapability(store, "US");
    panel.price.value = 190;
    panel.orderSession.value = "ETH";

    await vi.advanceTimersByTimeAsync(250);

    const call = fetchMock.mock.calls.find(([input]) =>
      String(input).includes("max-trade-qtys"),
    );
    expect(call).toBeDefined();
    const url = String(call![0]);
    expect(url).toContain("symbol=US.AAPL");
    expect(url).toContain("session=ETH");
  });
});

describe("useOrderEntryPanel order session context", () => {
  it("summarizes the market session and warns about off-hours RTH orders", () => {
    marketProfileState.extendedHoursMarkets.add("US");
    const { panel } = mountPanel({
      market: "US",
      symbol: "AAPL",
      price: 190,
      session: "pre",
    });

    expect(panel.currentMarketSessionLabel.value).toBe("盘前");
    expect(panel.orderSessionSummary.value).toBe(
      "当前行情时段：盘前 · 下单时段：常规交易时段（RTH）",
    );
    expect(panel.orderSessionCaution.value).toBe(
      "当前不是常规交易时段，RTH 订单通常要等盘中才会撮合。",
    );

    panel.orderSession.value = "OVERNIGHT";
    expect(panel.orderSessionCaution.value).toBe(
      "模拟盘夜盘支持通常受限，提交成功也可能暂时不会成交。",
    );
  });

  it("stays silent for markets without extended-hours order sessions", () => {
    const { panel } = mountPanel({ session: "pre" });

    expect(panel.supportsOrderSessionSelection.value).toBe(false);
    expect(panel.orderSessionSummary.value).toBe("");
    expect(panel.orderSessionCaution.value).toBe("");
  });
});

describe("useOrderEntryPanel submission", () => {
  it("submits a simulate order and records the broker acknowledgement", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.includes("/api/v1/execution/orders")) {
        return envelope({
          accepted: true,
          internalOrderId: "io-1",
          brokerOrderId: "broker-1",
          orderStatus: "FILLED",
        });
      }
      return envelope({});
    });
    vi.stubGlobal("fetch", fetchMock);
    const { panel, notifications } = mountPanel();
    panel.price.value = 320;
    const clientOrderId = panel.currentClientOrderId();

    await panel.submit();

    const payload = fetchCallBody(fetchMock, "/api/v1/execution/orders");
    expect(payload).toMatchObject({
      market: "HK",
      code: "00700",
      side: "BUY",
      orderType: "LIMIT",
      price: 320,
      quantity: 100,
      clientOrderId,
    });
    expect(panel.lastOrderFeedback.value).toMatchObject({
      level: "success",
      internalOrderId: "io-1",
      brokerOrderId: "broker-1",
      orderStatus: "FILLED",
    });
    expect(panel.lastOrderFeedback.value?.message).toBe(
      "下单成功：已提交订单，券商单号 broker-1",
    );
    expect(panel.lastOrderFeedback.value?.title).toContain("买入 100");
    expect(panel.lastOrderFeedback.value?.title).toContain("00700");
    expect(notifications.items.value.at(0)).toMatchObject({
      level: "success",
      message: "下单成功：已提交订单，券商单号 broker-1",
    });
    expect(panel.draftClientOrderId.value).toBe("");
    expect(panel.currentClientOrderId()).not.toBe(clientOrderId);
    expect(panel.orderFeedbackPolling.isActive.value).toBe(false);
  });

  it("surfaces a broker rejection as error feedback without polling", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      if (String(input).includes("/api/v1/execution/orders")) {
        return envelope({ accepted: false, message: "券商风控拒绝该订单" });
      }
      return envelope({});
    });
    vi.stubGlobal("fetch", fetchMock);
    const { panel, notifications } = mountPanel();
    panel.price.value = 320;

    await panel.submit();

    expect(panel.lastOrderFeedback.value).toMatchObject({
      level: "error",
      internalOrderId: null,
      message: "下单失败：券商风控拒绝该订单",
    });
    expect(notifications.items.value.at(0)).toMatchObject({
      level: "error",
      message: "下单失败：券商风控拒绝该订单",
    });
    expect(panel.orderFeedbackPolling.isActive.value).toBe(false);
  });

  it("fails the submission when the derivative preview returns no previewId", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.includes("/api/v1/execution/previews")) return envelope({});
      if (url.includes("/api/v1/execution/orders")) {
        return envelope({ accepted: true, internalOrderId: "io-x" });
      }
      return envelope({});
    });
    vi.stubGlobal("fetch", fetchMock);
    const { panel, preferences, notifications } = mountPanel();
    preferences.update({ productClass: "option" });
    panel.price.value = 3.2;

    await panel.submit();

    expect(fetchCallBody(fetchMock, "/api/v1/execution/orders")).toBeNull();
    expect(panel.lastOrderFeedback.value).toMatchObject({
      level: "error",
      message: "下单失败：订单预检未返回 previewId",
    });
    expect(notifications.items.value.at(0)).toMatchObject({
      level: "error",
      message: "下单失败：订单预检未返回 previewId",
    });
  });

  it("keeps polling an acknowledged order until the broker reports a final status", async () => {
    vi.useFakeTimers();
    let status = "SUBMITTED";
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.includes("/api/v1/execution/orders/io-poll")) {
        return envelope({
          order: {
            brokerOrderId: "broker-poll",
            brokerOrderIdEx: null,
            status,
            rawBrokerStatus: "PENDING",
          },
          recentEvents: [],
          checkedAt: "2026-07-16T09:31:00Z",
        });
      }
      if (url.includes("/api/v1/execution/orders")) {
        return envelope({
          accepted: true,
          internalOrderId: "io-poll",
          orderStatus: "SUBMITTED",
        });
      }
      return envelope({});
    });
    vi.stubGlobal("fetch", fetchMock);
    const { panel } = mountPanel();
    panel.price.value = 320;

    await panel.submit();
    expect(panel.orderFeedbackPolling.isActive.value).toBe(true);

    await vi.advanceTimersByTimeAsync(2_000);
    expect(panel.lastOrderFeedback.value).toMatchObject({
      orderStatus: "SUBMITTED",
      brokerOrderId: "broker-poll",
      checkedAt: "2026-07-16T09:31:00Z",
    });
    expect(panel.orderFeedbackPolling.isActive.value).toBe(true);

    status = "FILLED";
    await vi.advanceTimersByTimeAsync(2_000);
    expect(panel.lastOrderFeedback.value?.orderStatus).toBe("FILLED");
    expect(panel.orderFeedbackPolling.isActive.value).toBe(false);
  });
});

describe("useOrderEntryPanel real-trade confirmation", () => {
  it("gates real orders behind the typed confirmation and executes on match", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      if (String(input).includes("/api/v1/execution/orders")) {
        return envelope({
          accepted: true,
          internalOrderId: "io-real",
          orderStatus: "FILLED",
        });
      }
      return envelope({});
    });
    vi.stubGlobal("fetch", fetchMock);
    const { panel, store } = mountPanel({ tradingEnvironment: "REAL" });
    store.realTradeApprovals.value = {
      ...emptyRealTradeApprovals,
      realTradingEnabled: true,
    };
    panel.price.value = 320;

    expect(panel.isRealMode.value).toBe(true);
    expect(panel.requiredRealTradeConfirmationText.value).toBe("ENABLE_REAL_TRADING");

    await panel.submit();

    expect(panel.realTradeConfirmationOpen.value).toBe(true);
    expect(fetchCallBody(fetchMock, "/api/v1/execution/orders")).toBeNull();
    expect(panel.pendingRealTradeSubmission.value?.orderSummary).toContain("买入 100");
    expect(panel.pendingRealTradeSubmission.value?.orderSummary).toContain("限价 320");
    expect(panel.pendingRealTradeSubmission.value?.orderSummary).toContain("当日有效");

    panel.realTradeConfirmationText.value = "wrong text";
    expect(panel.realTradeConfirmationMatches.value).toBe(false);
    await panel.confirmRealTradeSubmission();
    expect(panel.realTradeConfirmationOpen.value).toBe(true);
    expect(fetchCallBody(fetchMock, "/api/v1/execution/orders")).toBeNull();

    panel.realTradeConfirmationText.value = " ENABLE_REAL_TRADING ";
    expect(panel.realTradeConfirmationMatches.value).toBe(true);
    await panel.confirmRealTradeSubmission();

    expect(panel.realTradeConfirmationOpen.value).toBe(false);
    expect(panel.pendingRealTradeSubmission.value).toBeNull();
    expect(fetchCallBody(fetchMock, "/api/v1/execution/orders")).toMatchObject({
      tradingEnvironment: "REAL",
      env: "REAL",
    });
    expect(panel.lastOrderFeedback.value).toMatchObject({
      level: "success",
      internalOrderId: "io-real",
    });
  });

  it("honours a broker-defined confirmation phrase with a safe fallback", () => {
    const { panel, store } = mountPanel({ tradingEnvironment: "REAL" });
    store.realTradeApprovals.value = {
      ...emptyRealTradeApprovals,
      requiredConfirmationText: "  I_ACCEPT_RISK  ",
    };
    expect(panel.requiredRealTradeConfirmationText.value).toBe("I_ACCEPT_RISK");

    store.realTradeApprovals.value = {
      ...emptyRealTradeApprovals,
      requiredConfirmationText: "   ",
    };
    expect(panel.requiredRealTradeConfirmationText.value).toBe("ENABLE_REAL_TRADING");
  });
});

describe("useOrderEntryPanel feedback presentation helpers", () => {
  it("decides whether the feedback order can still be cancelled", () => {
    const { panel } = mountPanel();
    const base = {
      title: "t",
      message: "m",
      brokerOrderId: null,
      brokerOrderIdEx: null,
      rawBrokerStatus: null,
      latestEvent: null,
      checkedAt: null,
    };

    expect(
      panel.canCancelFeedbackOrder({
        ...base,
        level: "error",
        internalOrderId: "io-1",
        orderStatus: null,
      }),
    ).toBe(false);
    expect(
      panel.canCancelFeedbackOrder({
        ...base,
        level: "success",
        internalOrderId: null,
        orderStatus: null,
      }),
    ).toBe(false);
    expect(
      panel.canCancelFeedbackOrder({
        ...base,
        level: "success",
        internalOrderId: "io-1",
        orderStatus: null,
      }),
    ).toBe(true);
    expect(
      panel.canCancelFeedbackOrder({
        ...base,
        level: "success",
        internalOrderId: "io-1",
        orderStatus: "FILLED",
      }),
    ).toBe(false);
    expect(
      panel.canCancelFeedbackOrder({
        ...base,
        level: "success",
        internalOrderId: "io-1",
        orderStatus: "SUBMITTED",
      }),
    ).toBe(true);
  });

  it("renders broker acceptance and status labels for the feedback card", () => {
    const { panel } = mountPanel();
    const base = {
      level: "success" as const,
      title: "t",
      message: "m",
      internalOrderId: "io-1",
      brokerOrderId: null,
      brokerOrderIdEx: null,
      rawBrokerStatus: null,
      latestEvent: null,
      checkedAt: null,
    };

    expect(panel.formatBrokerAcceptance({ ...base, orderStatus: "BROKER_ACCEPTED" })).toBe("已接受");
    expect(panel.formatBrokerAcceptance({ ...base, orderStatus: "PARTIALLY_FILLED" })).toBe("已接受");
    expect(panel.formatBrokerAcceptance({ ...base, orderStatus: "CANCEL_REQUESTED" })).toBe("已接受");
    expect(panel.formatBrokerAcceptance({ ...base, orderStatus: "REJECTED" })).toBe("未接受");
    expect(panel.formatBrokerAcceptance({ ...base, orderStatus: "EXPIRED" })).toBe("未接受");
    expect(panel.formatBrokerAcceptance({ ...base, orderStatus: "SUBMITTED" })).toBe("待确认");
    expect(panel.formatBrokerAcceptance({ ...base, orderStatus: null })).toBe("待确认");

    expect(panel.formatFeedbackOrderStatus({ ...base, orderStatus: null })).toBe("待券商回报");
    expect(
      panel.formatFeedbackOrderStatus({ ...base, level: "error", orderStatus: null }),
    ).toBe("未接受");
    expect(panel.formatFeedbackOrderStatus({ ...base, orderStatus: "FILLED" })).toBe("已成交");

    expect(panel.orderFeedbackAccountHref({ ...base, internalOrderId: null })).toBe("/account");
    expect(panel.orderFeedbackAccountHref(base)).toBe(
      "/account?tab=history&orderId=io-1",
    );
  });

  it("formats estimates, metrics, and failure reasons for the order form", () => {
    const { panel } = mountPanel();

    panel.price.value = 320;
    panel.quantity.value = 100;
    expect(panel.estimate()).toBe("32000.00");
    panel.quantity.value = 0;
    expect(panel.estimate()).toBe("—");
    panel.quantity.value = 100;
    panel.orderType.value = "MARKET";
    expect(panel.estimate()).toBe("—");

    expect(panel.formatMetric(null)).toBe("—");
    expect(panel.formatMetric(1234.56789)).toBe("1,234.5679");
    expect(panel.formatInitialMargin(null)).toBe("股票通常不返回");
    expect(panel.formatInitialMargin(5000)).toBe("5,000");

    expect(panel.resolveOrderFailureReason(new Error(" 券商超时 "))).toBe("券商超时");
    expect(panel.resolveOrderFailureReason(new Error("  "))).toBe(
      "下单请求失败，请稍后重试。",
    );
    expect(panel.resolveOrderFailureReason("boom")).toBe("下单请求失败，请稍后重试。");

    expect(panel.normalizeOptionalText("  broker-1 ")).toBe("broker-1");
    expect(panel.normalizeOptionalText("   ")).toBeNull();
    expect(panel.normalizeOptionalText(null)).toBeNull();

    expect(panel.countDecimalPlaces(0.001)).toBe(3);
    expect(panel.countDecimalPlaces(100)).toBe(0);
  });
});
