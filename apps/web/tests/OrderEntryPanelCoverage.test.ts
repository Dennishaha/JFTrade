// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick } from "vue";

import type { MarketSecurityDetails } from "@/contracts";

const marketProfileState = vi.hoisted(() => ({
  extendedHoursMarkets: new Set<string>(),
}));

vi.mock("../src/composables/marketProfiles", () => ({
  useMarketProfiles: () => ({
    supportsExtendedHoursForMarket: (market: string | null | undefined) =>
      marketProfileState.extendedHoursMarkets.has((market ?? "").trim().toUpperCase()),
  }),
}));

import OrderEntryPanel from "../src/components/workspace/OrderEntryPanel.vue";
import { provideConsoleDataStore } from "../src/composables/useConsoleData";
import { provideNotificationsStore } from "../src/composables/useNotifications";
import { provideWorkspaceTradingPreferencesStore } from "../src/composables/useWorkspaceLayout";

afterEach(() => {
  marketProfileState.extendedHoursMarkets.clear();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("OrderEntryPanel business coverage", () => {
  it("submits a US extended-hours stop-limit sell with both protective prices", async () => {
    marketProfileState.extendedHoursMarkets.add("US");
    const fetchMock = vi.fn().mockResolvedValue(envelope({
      accepted: true,
      internalOrderId: "io-stop-limit",
      brokerOrderId: "broker-stop-limit",
      orderStatus: "BROKER_ACCEPTED",
    }));
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper } = mountOrderEntry({
      market: "US",
      symbol: "AAPL",
      price: 190,
      session: "pre",
      security: { priceSpread: 0.01, securityType: "STOCK" },
    });

    await nextTick();
    await findSelectWithOption(wrapper, "STOP_LIMIT").setValue("STOP_LIMIT");
    await wrapper.get(".is-sell").trigger("click");
    expect(wrapper.text()).toContain("当前不是常规交易时段");
    await findSelectWithOption(wrapper, "ETH").setValue("ETH");
    const priceInputs = wrapper
      .findAll('input[type="number"]')
      .filter((input) => input.attributes("min") === "0");
    await priceInputs[0]!.setValue("189.75");
    await priceInputs[1]!.setValue("188.5");
    await findSubmitButton(wrapper).trigger("click");
    await vi.waitFor(() => expect(wrapper.text()).toContain("io-stop-limit"));

    const request = fetchMock.mock.calls.find(([input]) =>
      String(input).includes("/api/v1/execution/orders"),
    );
    const payload = JSON.parse(String((request?.[1] as RequestInit).body));
    expect(payload).toMatchObject({
      market: "US",
      code: "AAPL",
      side: "SELL",
      orderType: "STOP_LIMIT",
      session: "ETH",
      price: 189.75,
      stopPrice: 188.5,
    });
    expect(wrapper.text()).toContain("券商接受");
    expect(wrapper.text()).toContain("已接受");
  });

  it("keeps a retry path after a manual order-status refresh fails", async () => {
    let orderSubmitted = false;
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.includes("/api/v1/execution/orders/io-refresh")) {
        throw new Error("券商状态服务暂不可用");
      }
      if (url.includes("/api/v1/execution/orders")) {
        orderSubmitted = true;
        return envelope({
          accepted: true,
          internalOrderId: "io-refresh",
          orderStatus: "REJECTED",
          checkedAt: "not-a-date",
        });
      }
      throw new Error(`unexpected request: ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper, notifications } = mountOrderEntry({
      market: "HK",
      symbol: "00700",
      price: 320,
      session: "regular",
    });

    await nextTick();
    await findSubmitButton(wrapper).trigger("click");
    await vi.waitFor(() => expect(wrapper.text()).toContain("io-refresh"));
    expect(orderSubmitted).toBe(true);
    expect(wrapper.text()).toContain("未接受");

    await wrapper.get('[title="刷新订单状态"]').trigger("click");
    await nextTick();
    expect(notifications.items.value[0]).toMatchObject({
      level: "warn",
      title: "订单状态刷新失败",
      message: "券商状态服务暂不可用",
    });
  });

  it("cancels a real-trade confirmation without carrying stale typed consent into the next order", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper, store } = mountOrderEntry({
      market: "SH",
      symbol: "600519",
      price: 1500,
      session: "regular",
    });
    store.systemStatus.value = {
      ...store.systemStatus.value,
      defaultTradingEnvironment: "REAL",
      realTradingEnabled: true,
    };
    store.realTradeRiskState.value = {
      ...store.realTradeRiskState.value,
      realTradingEnabled: true,
      riskEnabled: true,
      effectiveMaxOrderQuantity: 100,
      effectiveMaxOrderNotional: 200000,
    };

    await nextTick();
    await findSubmitButton(wrapper).trigger("click");
    await nextTick();
    await wrapper.get(".tv-real-confirmation__input").setValue("ENABLE_REAL_TRADING");
    await wrapper
      .findAll(".tv-real-confirmation__actions button")
      .find((button) => button.text().trim() === "取消")!
      .trigger("click");
    await nextTick();

    expect(wrapper.find(".tv-real-confirmation").exists()).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
    await findSubmitButton(wrapper).trigger("click");
    await nextTick();
    expect((wrapper.get(".tv-real-confirmation__input").element as HTMLInputElement).value).toBe("");
  });

  it("keeps price precision, feedback formatting, and guarded confirmations safe at boundary values", async () => {
    const { wrapper, store, notifications } = mountOrderEntry({
      market: "US",
      symbol: "PENNY",
      price: 0.5,
      session: "regular",
      security: { priceSpread: 0, currentPrice: 0.5 },
    });
    const panel = wrapper.getComponent(OrderEntryPanel);
    const setup = panel.vm.$.setupState as Record<string, unknown>;
    const call = <T>(name: string, ...args: unknown[]) =>
      (setup[name] as (...values: unknown[]) => T)(...args);
    const read = <T>(value: unknown): T =>
      value !== null && typeof value === "object" && "value" in value
        ? (value as { value: T }).value
        : value as T;
    const write = (name: string, value: unknown) => {
      const target = setup[name] as { value?: unknown };
      if (target != null && typeof target === "object" && "value" in target) {
        target.value = value;
        return;
      }
      setup[name] = value;
    };

    expect(read<unknown>(setup.maxTradeQuantityPrimaryValue)).toBeNull();
    expect(call<number>("countDecimalPlaces", 1e-5)).toBe(5);
    expect(call<string>("formatOrderSession", "ALL")).toContain("全时段");
    expect(call<string>("formatOrderSession", "custom")).toBe("custom");
    expect(call<string>("orderFeedbackAccountHref", { internalOrderId: null })).toBe("/account");
    expect(call<string>("formatFeedbackCheckedAt", null)).toBe("");
    expect(call<string>("resolvePendingOrderSummary", {
      market: "US",
      code: "PENNY",
      symbol: "US.PENNY",
      side: "SELL",
      quantity: 2,
      orderType: "STOP_LIMIT",
      timeInForce: "GTC",
      price: 0.5001,
      stopPrice: 0.49,
      session: "ALL",
    })).toContain("止损价 0.49");

    store.marketDataSnapshot.value = null;
    store.marketSecurityDetails.value = {
      ...store.marketSecurityDetails.value!,
      security: {
        ...store.marketSecurityDetails.value!.security,
        currentPrice: 0.75,
        bidPrice: 0,
        askPrice: 0,
      },
    };
    expect(call<number | null>("resolveReferencePrice", 0)).toBe(0.75);

    store.marketSecurityDetails.value = {
      ...store.marketSecurityDetails.value!,
      security: {
        ...store.marketSecurityDetails.value!.security,
        currentPrice: 0,
        bidPrice: 0,
        askPrice: 0,
      },
    };
    call<void>("syncMarketPriceToPriceInput", true);
    expect(notifications.items.value[0]).toMatchObject({
      level: "warn",
      title: "暂无可同步的市场价格",
    });

    await call<Promise<void>>("refreshOrderFeedback", "", true);
    write("submitting", true);
    await call<Promise<void>>("submit");
    write("submitting", false);
    await call<Promise<void>>("confirmRealTradeSubmission");
    write("realTradeConfirmationText", "ENABLE_REAL_TRADING");
    await call<Promise<void>>("confirmRealTradeSubmission");
    expect(read<boolean>(setup.realTradeConfirmationOpen)).toBe(false);

    wrapper.unmount();
  });

  it("keeps stale broker refreshes isolated and falls back to the available sell quantity", async () => {
    vi.useFakeTimers();
    const fetchMock = vi.fn().mockResolvedValue(envelope({}));
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper, store } = mountOrderEntry({
      market: "HK",
      symbol: "00700",
      price: 320,
      session: "regular",
    });
    const panel = wrapper.getComponent(OrderEntryPanel);
    const setup = panel.vm.$.setupState as Record<string, unknown>;
    const read = <T>(value: unknown): T =>
      value !== null && typeof value === "object" && "value" in value
        ? (value as { value: T }).value
        : value as T;
    const write = (name: string, value: unknown) => {
      const target = setup[name] as { value?: unknown };
      if (target != null && typeof target === "object" && "value" in target) {
        target.value = value;
        return;
      }
      setup[name] = value;
    };
    const call = <T>(name: string, ...args: unknown[]) =>
      (setup[name] as (...values: unknown[]) => T)(...args);

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
        maxCashAndMarginBuy: null,
        maxPositionSell: 80,
        maxSellShort: null,
        maxBuyBack: null,
        longRequiredIM: null,
        shortRequiredIM: null,
        session: null,
      },
    };
    await wrapper.get(".is-sell").trigger("click");
    await nextTick();
    expect(read<number | null>(setup.maxTradeQuantityPrimaryValue)).toBe(80);
    expect(read<string>(setup.orderSessionSummary)).toBe("");

    write("lastOrderFeedback", {
      level: "success",
      title: "current",
      message: "current order",
      internalOrderId: "io-current",
      brokerOrderId: null,
      brokerOrderIdEx: null,
      orderStatus: "BROKER_ACCEPTED",
      rawBrokerStatus: null,
      latestEvent: null,
      checkedAt: null,
    });
    await call<Promise<void>>("refreshOrderFeedback", "io-stale");
    expect(read<{ internalOrderId: string } | null>(setup.lastOrderFeedback)).toMatchObject({
      internalOrderId: "io-current",
    });

    write("realTradeConfirmationOpen", true);
    write("realTradeConfirmationText", "ENABLE_REAL_TRADING");
    write("pendingRealTradeSubmission", null);
    await call<Promise<void>>("confirmRealTradeSubmission");
    expect(read<boolean>(setup.realTradeConfirmationOpen)).toBe(false);
    wrapper.unmount();
  });

  it("continues polling a pending broker order but skips max-quantity requests after the instrument is cleared", async () => {
    vi.useFakeTimers();
    let status = "SUBMITTED";
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      if (String(input).includes("/api/v1/execution/orders/io-pending")) {
        return envelope({
          order: {
            brokerOrderId: "broker-pending",
            brokerOrderIdEx: null,
            status,
            rawBrokerStatus: "PENDING",
          },
          recentEvents: [],
          checkedAt: "2026-07-16T09:31:00Z",
        });
      }
      return envelope({});
    });
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper, preferences } = mountOrderEntry({
      market: "HK",
      symbol: "00700",
      price: 320,
      session: "regular",
    });
    const panel = wrapper.getComponent(OrderEntryPanel);
    const setup = panel.vm.$.setupState as Record<string, unknown>;
    const read = <T>(value: unknown): T =>
      value !== null && typeof value === "object" && "value" in value
        ? (value as { value: T }).value
        : value as T;
    const write = (name: string, value: unknown) => {
      const target = setup[name] as { value?: unknown };
      if (target != null && typeof target === "object" && "value" in target) {
        target.value = value;
        return;
      }
      setup[name] = value;
    };
    const call = <T>(name: string, ...args: unknown[]) =>
      (setup[name] as (...values: unknown[]) => T)(...args);

    write("lastOrderFeedback", {
      level: "success",
      title: "已提交",
      message: "等待券商回报",
      internalOrderId: "io-pending",
      brokerOrderId: null,
      brokerOrderIdEx: null,
      orderStatus: "BROKER_ACCEPTED",
      rawBrokerStatus: null,
      latestEvent: null,
      checkedAt: null,
    });
    await call<Promise<void>>("refreshOrderFeedback", "io-pending");
    expect(read<{ orderStatus: string } | null>(setup.lastOrderFeedback)).toMatchObject({
      orderStatus: "SUBMITTED",
    });

    status = "FILLED";
    await vi.advanceTimersByTimeAsync(2_000);
    expect(
      fetchMock.mock.calls.filter(([input]) =>
        String(input).includes("/api/v1/execution/orders/io-pending"),
      ),
    ).toHaveLength(2);

    preferences.update({ market: "", symbol: "" });
    await nextTick();
    const requestCountBefore = fetchMock.mock.calls.length;
    await call<Promise<void>>("loadMaxTradeQuantity");
    expect(fetchMock).toHaveBeenCalledTimes(requestCountBefore);

    wrapper.unmount();
  });

  it("previews prediction orders and preserves product-specific quantity semantics", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.includes("/api/v1/execution/previews")) {
        return envelope({ previewId: "preview-event-1" });
      }
      if (url.includes("/api/v1/execution/orders")) {
        return envelope({
          accepted: true,
          internalOrderId: "io-event-1",
          brokerOrderId: "broker-event-1",
          orderStatus: "BROKER_ACCEPTED",
        });
      }
      return envelope({});
    });
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper, store } = mountOrderEntry({
      market: "US",
      symbol: "PREDTEST",
      price: 0.55,
      session: "regular",
      security: { priceSpread: 0.01, securityType: "EVENT_CONTRACT" },
    });
    const panel = wrapper.getComponent(OrderEntryPanel);
    const setup = panel.vm.$.setupState as Record<string, unknown>;
    const read = <T>(value: unknown): T =>
      value !== null && typeof value === "object" && "value" in value
        ? (value as { value: T }).value
        : value as T;

    await nextTick();
    expect(store.marketSecurityDetails.value?.security.securityType).toBe("EVENT_CONTRACT");
    expect(read<string>(setup.productClass)).toBe("event_contract");
    expect(read<string>(setup.tradeQuantityUnit)).toBe("金额");
    await findSelectWithOption(wrapper, "NO").setValue("NO");
    await wrapper.get('input[type="number"][min="1"]').setValue("25");
    await findSubmitButton(wrapper).trigger("click");
    await vi.waitFor(() => expect(wrapper.text()).toContain("io-event-1"));

    const previewRequest = fetchMock.mock.calls.find(([input]) =>
      String(input).includes("/api/v1/execution/previews"),
    );
    const orderRequest = fetchMock.mock.calls.find(([input]) =>
      String(input).includes("/api/v1/execution/orders"),
    );
    expect(previewRequest).toBeDefined();
    expect(JSON.parse(String((orderRequest?.[1] as RequestInit).body))).toMatchObject({
      productClass: "event_contract",
      quantityMode: "amount",
      orderKind: "event_single",
      amount: 25,
      predictionSide: "NO",
      previewId: "preview-event-1",
    });

    const productCases = [
      ["OPTION", "option", "张"],
      ["FUTURE", "future", "张"],
      ["CBBC", "cbbc", "单位"],
      ["WARRANT", "warrant", "单位"],
      ["ETF", "fund", "股"],
    ] as const;
    for (const [securityType, productClass, quantityUnit] of productCases) {
      const details = store.marketSecurityDetails.value!;
      store.marketSecurityDetails.value = {
        ...details,
        security: { ...details.security, securityType },
      };
      await nextTick();
      expect(read<string>(setup.productClass)).toBe(productClass);
      expect(read<string>(setup.tradeQuantityUnit)).toBe(quantityUnit);
    }

    const detailsWithoutType = store.marketSecurityDetails.value!;
    const securityWithoutType = {
      ...detailsWithoutType.security,
    } as Partial<MarketSecurityDetails>;
    delete securityWithoutType.securityType;
    store.marketSecurityDetails.value = {
      ...detailsWithoutType,
      security: securityWithoutType as MarketSecurityDetails,
    };
    await nextTick();
    expect(read<string>(setup.productClass)).toBe("equity");
    expect(read<string>(setup.tradeQuantityUnit)).toBe("单位");

    wrapper.unmount();
  });
});

function mountOrderEntry(options: {
  market: string;
  symbol: string;
  price: number;
  session: string;
  security?: Partial<MarketSecurityDetails>;
}) {
  let store: ReturnType<typeof provideConsoleDataStore> | null = null;
  let notifications: ReturnType<typeof provideNotificationsStore> | null = null;
  let preferences: ReturnType<typeof provideWorkspaceTradingPreferencesStore> | null = null;
  const Host = defineComponent({
    setup() {
      preferences = provideWorkspaceTradingPreferencesStore();
      preferences.update({ market: options.market, symbol: options.symbol });
      notifications = provideNotificationsStore();
      store = provideConsoleDataStore(preferences);
      store.systemStatus.value = {
        ...store.systemStatus.value,
        defaultTradingEnvironment: "SIMULATE",
        realTradingEnabled: false,
      };
      store.marketDataSnapshot.value = {
        request: {
          market: options.market,
          symbol: options.symbol,
          instrumentId: `${options.market}.${options.symbol}`,
        },
        snapshot: {
          price: options.price,
          bid: options.price - 0.1,
          ask: options.price + 0.1,
          previousClosePrice: options.price - 1,
          volume: 100,
          turnover: 1000,
          at: "2026-07-16T09:30:00.000Z",
          session: options.session,
        },
        meta: {
          instrumentId: `${options.market}.${options.symbol}`,
          source: "test",
          resolvedAt: "2026-07-16T09:30:00.000Z",
          fromCache: false,
        },
      };
      store.marketSecurityDetails.value = {
        request: {
          market: options.market,
          symbol: options.symbol,
          instrumentId: `${options.market}.${options.symbol}`,
        },
        security: {
          instrumentId: `${options.market}.${options.symbol}`,
          market: options.market,
          symbol: options.symbol,
          securityId: 1,
          name: options.symbol,
          securityType: "STOCK",
          exchangeType: options.market,
          listTime: "2020-01-01",
          listTimestamp: 1,
          delisting: false,
          lotSize: 100,
          isSuspend: false,
          priceSpread: 0.01,
          updateTime: "2026-07-16 09:30:00",
          updateTimestamp: 1,
          highPrice: options.price,
          openPrice: options.price,
          lowPrice: options.price,
          lastClosePrice: options.price,
          currentPrice: options.price,
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
          instrumentId: `${options.market}.${options.symbol}`,
          source: "test",
          resolvedAt: "2026-07-16T09:30:00.000Z",
          fromCache: false,
        },
      };
      return () => h(OrderEntryPanel);
    },
  });
  const wrapper = mount(Host, {
    global: {
      plugins: [createPinia()],
      stubs: {
        "v-dialog": dialogStub,
        "v-card": passthroughStub,
        "v-card-title": passthroughStub,
        "v-card-text": passthroughStub,
        "v-card-actions": passthroughStub,
        "v-btn": safeButtonStub,
      },
    },
  });
  if (store == null || notifications == null || preferences == null) {
    throw new Error("Order entry stores were not initialized.");
  }
  return { wrapper, store, notifications, preferences };
}

function findSelectWithOption(wrapper: ReturnType<typeof mount>, option: string) {
  const select = wrapper.findAll("select").find((candidate) =>
    candidate.findAll("option").some((item) => item.attributes("value") === option),
  );
  if (select == null) throw new Error(`Select with ${option} not found.`);
  return select;
}

function findSubmitButton(wrapper: ReturnType<typeof mount>) {
  const button = wrapper.find("button.tv-btn");
  if (!button.exists()) throw new Error("Order submit button not found.");
  return button;
}

function envelope(data: unknown): Response {
  return {
    ok: true,
    json: async () => ({ ok: true, data }),
  } as Response;
}

const passthroughStub = { template: "<div><slot /></div>" };

const dialogStub = defineComponent({
  props: ["modelValue"],
  emits: ["update:modelValue"],
  template: "<div v-if=\"modelValue\"><slot /></div>",
});

const safeButtonStub = defineComponent({
  props: ["disabled"],
  emits: ["click"],
  template: "<button type=\"button\" :disabled=\"disabled\" @click=\"$emit('click')\"><slot /></button>",
});
