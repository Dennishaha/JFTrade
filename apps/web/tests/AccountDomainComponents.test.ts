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

describe("PositionsTable reconciliation column", () => {
  it("hides the reconciliation column when no reconciliation entries exist", () => {
    const wrapper = mount(PositionsTable, {
      props: { positions: [makePosition()] },
    });

    expect(wrapper.text()).not.toContain("对账");
    expect(wrapper.find(".positions-table__recon").exists()).toBe(false);
  });

  it("renders reconciliation status per position when entries exist", () => {
    const wrapper = mount(PositionsTable, {
      props: {
        positions: [
          makePosition(),
          makePosition({ symbol: "HK.00700", market: "HK" }),
          makePosition({ symbol: "SH.600519", market: "SH" }),
        ],
        reconciliation: [
          {
            brokerId: "futu",
            accountId: "REAL-001",
            tradingEnvironment: "REAL",
            market: "US",
            symbol: "US.AAPL",
            status: "matched",
          },
          {
            brokerId: "futu",
            accountId: "REAL-001",
            tradingEnvironment: "REAL",
            market: "HK",
            symbol: "HK.00700",
            status: "different",
          },
        ] as never,
      },
    });

    expect(wrapper.text()).toContain("对账");

    const statuses = wrapper.findAll(".positions-table__recon");
    expect(statuses).toHaveLength(3);
    expect(statuses[0]?.text()).toContain("已匹配");
    expect(statuses[0]?.classes()).toContain("tv-status--success");
    expect(statuses[0]?.find(".tv-state-dot").exists()).toBe(true);
    expect(statuses[1]?.text()).toContain("存在差异");
    expect(statuses[1]?.classes()).toContain("tv-status--warning");
    expect(statuses[2]?.text()).toContain("未对账");
    expect(statuses[2]?.classes()).toContain("tv-status--info");
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
      brokerFunds: ref({ summary: null }),
    };

    const wrapper = mount(AccountAssetStrip);
    const realized = wrapper
      .findAll(".asset-strip__item")
      .find((item) => item.get("span").text() === "已实现盈亏");

    expect(realized?.get("b").text()).toBe("--");
    expect(realized?.get("b").classes()).not.toContain("tv-up");
    expect(realized?.get("b").classes()).not.toContain("tv-down");
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
});
