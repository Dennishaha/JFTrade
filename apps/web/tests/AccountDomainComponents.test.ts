// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { ref } from "vue";

import AccountAssetStrip from "../src/components/domain/account/AccountAssetStrip.vue";
import AccountSummarySidebar from "../src/components/domain/account/AccountSummarySidebar.vue";
import {
  type AccountExecutionOrder,
  formatExecutionStatusTransition,
  formatOrderKind,
  orderStatusClass,
} from "../src/components/domain/account/executionOrderFormat";
import PositionsTable, {
  type AccountPositionRow,
} from "../src/components/domain/account/PositionsTable.vue";

const assetStripState = vi.hoisted(() => ({
  store: null as null | Record<string, unknown>,
}));

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => assetStripState.store,
}));

function makePosition(overrides: Partial<AccountPositionRow> = {}): AccountPositionRow {
  return {
    symbol: "US.AAPL",
    name: "Apple",
    market: "US",
    quantity: 10,
    averagePrice: 100,
    lastPrice: 110,
    marketValue: 1100,
    unrealizedPnl: 100,
    pnlRatio: 0.1,
    currency: "USD",
    productClass: "equity",
    strategyType: null,
    positionType: null,
    payoutIfWin: null,
    source: "券商",
    updatedAt: "2026-07-16T09:30:00Z",
    ...overrides,
  };
}

describe("PositionsTable", () => {
  it("does not expose the removed reconciliation column", () => {
    const wrapper = mount(PositionsTable, {
      props: { positions: [makePosition()] },
    });

    expect(wrapper.text()).not.toContain("对账");
    expect(wrapper.find(".positions-table__recon").exists()).toBe(false);
  });

  it("renders product classes and pnl variants without losing semantic labels", () => {
    const wrapper = mount(PositionsTable, {
      props: {
        positions: [
          makePosition({ productClass: "option", pnlRatio: null }),
          makePosition({
            symbol: "US.ES",
            productClass: "future",
            unrealizedPnl: -20,
            pnlRatio: -5,
          }),
          makePosition({
            symbol: "US.EVENT",
            productClass: "event_contract",
            pnlRatio: 0,
          }),
          makePosition({ symbol: "US.SPY", productClass: "fund" }),
        ],
      },
    });

    expect(wrapper.text()).toContain("期权");
    expect(wrapper.text()).toContain("期货");
    expect(wrapper.text()).toContain("预测合约");
    expect(wrapper.text()).toContain("基金/信托");
    expect(wrapper.text()).toContain("-5.00%");
    expect(wrapper.text()).toContain("--");
    expect(wrapper.find(".tv-down").exists()).toBe(true);
  });
});

describe("execution order display formatting", () => {
  function order(
    orderKind: string,
    orderType = "LIMIT",
  ): AccountExecutionOrder {
    return { orderKind, orderType } as AccountExecutionOrder;
  }

  it("distinguishes combo and event products while preserving status severity", () => {
    expect(formatOrderKind(order("option_combo"))).toBe("期权组合");
    expect(formatOrderKind(order("event_single"))).toBe("预测单腿");
    expect(formatOrderKind(order("event_parlay"))).toBe("预测 Parlay");
    expect(formatOrderKind(order("single"))).toBe("限价");

    expect(orderStatusClass("REJECTED")).toBe("tv-status--error");
    expect(orderStatusClass("FILLED")).toBe("tv-status--success");
    expect(orderStatusClass("EXPIRED")).toBe("tv-status--warning");
    expect(orderStatusClass("CANCELLED")).toBe("tv-status--info");
    expect(orderStatusClass("CANCEL_REQUESTED")).toBe("tv-status--warning");
    expect(orderStatusClass("SUBMITTED")).toBe("tv-status--info");

    expect(formatExecutionStatusTransition(null, "SUBMITTED")).toContain(
      "首次发现",
    );
    expect(formatExecutionStatusTransition("SUBMITTED", "FILLED")).toContain(
      "→",
    );
  });
});

describe("AccountAssetStrip", () => {
  it("renders realized and unrealized pnl with directional tones", () => {
    assetStripState.store = {
      brokerFunds: ref({
        summary: {
          accountId: "REAL-001",
          tradingEnvironment: "REAL",
          market: "US",
          currency: "USD",
          totalAssets: 20000,
          unrealizedPnl: 120,
          realizedPnl: -45,
        },
      }),
    };

    const wrapper = mount(AccountAssetStrip);
    const items = wrapper.findAll(".asset-strip__item");
    const byLabel = new Map(
      items.map((item) => [item.get("span").text(), item.get("b")]),
    );

    expect(byLabel.get("已实现盈亏")?.text()).toContain("45");
    expect(byLabel.get("已实现盈亏")?.classes()).toContain("tv-down");
    expect(byLabel.get("未实现盈亏")?.text()).toContain("120");
    expect(byLabel.get("未实现盈亏")?.classes()).toContain("tv-up");
    expect(byLabel.get("总资产")?.text()).toContain("20,000");
  });

  it("falls back to placeholder when the funds summary is absent", () => {
    assetStripState.store = {
      brokerFunds: ref({
        summary: null,
        lastError: "获取账户资金数据缺少必要参数币种",
      }),
    };

    const wrapper = mount(AccountAssetStrip);
    const realized = wrapper
      .findAll(".asset-strip__item")
      .find((item) => item.get("span").text() === "已实现盈亏");

    expect(realized?.get("b").text()).toBe("--");
    expect(realized?.get("b").classes()).not.toContain("tv-up");
    expect(realized?.get("b").classes()).not.toContain("tv-down");
    expect(wrapper.text()).toContain("资金数据暂不可用");
    expect(wrapper.text()).toContain("缺少必要参数币种");
    const riskDot = wrapper.get('.asset-strip__title [title="风控数据不可用，无法判断账户风险"]');
    expect(riskDot.classes()).toContain("tv-status--warning");
  });

  it("marks risk as normal only when margin call data explicitly equals zero", () => {
    assetStripState.store = {
      brokerFunds: ref({
        summary: { currency: "USD", marginCallMargin: 0 },
        lastError: null,
      }),
    };

    const wrapper = mount(AccountAssetStrip);
    const riskDot = wrapper.get('.asset-strip__title [title="追保保证金为零"]');
    expect(riskDot.classes()).toContain("tv-status--success");
  });

  it("warns when the margin call amount is greater than zero", () => {
    assetStripState.store = {
      brokerFunds: ref({
        summary: {
          currency: "USD",
          riskStatus: "MARGIN_CALL",
          marginCallMargin: 50,
        },
        lastError: null,
      }),
    };

    const wrapper = mount(AccountAssetStrip);
    const riskDot = wrapper.get('.asset-strip__title [title="存在追保保证金，请关注账户风险"]');
    expect(riskDot.classes()).toContain("tv-status--warning");
    const marginCall = wrapper
      .findAll(".asset-strip__item")
      .find((item) => item.get("span").text() === "追保保证金");
    expect(marginCall?.get("b").classes()).toContain("is-warn");
  });

  it("does not mark an invalid negative margin call value as normal", () => {
    assetStripState.store = {
      brokerFunds: ref({
        summary: { currency: "USD", marginCallMargin: -1 },
        lastError: null,
      }),
    };

    const wrapper = mount(AccountAssetStrip);
    const riskDot = wrapper.get('.asset-strip__title [title="风控数据不可用，无法判断账户风险"]');
    expect(riskDot.classes()).toContain("tv-status--warning");
  });
});

describe("AccountSummarySidebar", () => {
  it("keeps missing account context explicit and preserves negative pnl direction", () => {
    assetStripState.store = {
      brokerFunds: ref({
        summary: {
          currency: "USD",
          totalAssets: 1000,
          unrealizedPnl: 0,
          realizedPnl: -25,
          tradingEnvironment: "",
        },
      }),
      brokerRuntime: ref({
        accounts: [],
        descriptor: { displayName: "Futu" },
        session: { connectivity: "disconnected" },
      }),
      portfolioCashBalances: ref({
        balances: [
          {
            brokerId: "futu",
            accountId: "SIM-1",
            tradingEnvironment: "SIMULATE",
            cashBalance: 500,
          },
        ],
      }),
      selectedBrokerAccount: ref(null),
      systemStatus: ref({ defaultTradingEnvironment: "" }),
    };

    const wrapper = mount(AccountSummarySidebar);

    expect(wrapper.text()).toContain("未选择账户");
    expect(wrapper.text()).toContain("Futu");
    expect(wrapper.text()).toContain("-25");
    expect(wrapper.find(".tv-down").exists()).toBe(true);
    expect(wrapper.find(".is-flat").exists()).toBe(true);
    expect(wrapper.find(".tv-status--warning").exists()).toBe(true);
  });

  it("distinguishes missing cash data from a real zero balance", () => {
    const baseStore = {
      brokerFunds: ref({ summary: null }),
      brokerRuntime: ref({
        accounts: [],
        descriptor: { displayName: "Futu" },
        session: { connectivity: "connected" },
      }),
      selectedBrokerAccount: ref({
        brokerId: "futu",
        accountId: "REAL-1",
        tradingEnvironment: "REAL",
        market: "US",
        displayName: "REAL-1",
      }),
      systemStatus: ref({ defaultTradingEnvironment: "REAL" }),
    };

    assetStripState.store = {
      ...baseStore,
      portfolioCashBalances: ref({ balances: [] }),
    };
    const missing = mount(AccountSummarySidebar);
    const missingCash = missing
      .findAll(".account-sidebar__row")
      .find((row) => row.get("span").text() === "现金");
    expect(missingCash?.get("b").text()).toBe("--");

    assetStripState.store = {
      ...baseStore,
      portfolioCashBalances: ref({
        balances: [{
          brokerId: "futu",
          accountId: "REAL-1",
          tradingEnvironment: "REAL",
          currency: "USD",
          cashBalance: 0,
        }],
      }),
    };
    const zero = mount(AccountSummarySidebar);
    const zeroCash = zero
      .findAll(".account-sidebar__row")
      .find((row) => row.get("span").text() === "现金");
    expect(zeroCash?.get("b").text()).toBe("0");
  });
});
