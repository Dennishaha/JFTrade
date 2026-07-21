// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  emptyBrokerCashFlows,
  emptyBrokerRuntime,
  emptyExecutionOrders,
  emptyPortfolioCashBalances,
  emptyPortfolioPositions,
  emptyRealTradeApprovals,
  emptyRealTradeHardStopEvents,
  emptyRealTradeHardStops,
  emptyRealTradeKillSwitchEvents,
  emptyRealTradeKillSwitchState,
  emptyRealTradeRiskEvents,
  emptyRealTradeRiskState,
  emptyStorageOverview,
  emptySystemStatus,
  emptyWorkerBrokerOrderUpdates,
} from "@/contracts";
import type {
  BrokerCashFlowsResponse,
  BrokerFundsResponse,
  BrokerOrdersResponse,
  BrokerPositionsResponse,
  BrokerRuntimeResponse,
} from "@/contracts";

import {
  MockWebSocket,
  createResponse,
  enabledFutuBrokerSettings,
  flushRequests,
  mountApp,
} from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
});

describe("Account page broker route redirect", () => {
  it("shows account basics, funds, and broker positions", async () => {
    const brokerRuntime: BrokerRuntimeResponse = {
      ...emptyBrokerRuntime,
      session: {
        ...emptyBrokerRuntime.session,
        connectivity: "connected",
        accountsDiscovered: 1,
      },
      accounts: [
        {
          accountId: "REAL-001",
          tradingEnvironment: "REAL",
          accountType: "CASH",
          accountRole: null,
          securityFirm: "FUTUSECURITIES",
          marketAuthorities: ["HK"],
          simulatedAccountType: "STOCK",
        },
      ],
    };

    const brokerFunds: BrokerFundsResponse = {
      ...emptyBrokerRuntime,
      checkedAt: "2026-05-16T00:00:00.000Z",
      connectivity: "connected",
      summary: {
        accountId: "REAL-001",
        tradingEnvironment: "REAL",
        market: "HK",
        currency: "HKD",
        totalAssets: 120000,
        securitiesAssets: 120000,
        fundAssets: 0,
        bondAssets: 0,
        cash: 88000,
        marketValue: 32000,
        longMarketValue: 32000,
        shortMarketValue: 0,
        purchasingPower: 50000,
        shortSellingPower: 0,
        netCashPower: 50000,
        availableWithdrawalCash: 87000,
        maxWithdrawal: 86000,
        availableFunds: null,
        frozenCash: 500,
        pendingAsset: 1000,
        unrealizedPnl: null,
        realizedPnl: null,
        initialMargin: null,
        maintenanceMargin: null,
        marginCallMargin: null,
        riskStatus: "LEVEL2",
      },
      currencyBalances: [],
      marketAssets: [],
    };

    const brokerPositions: BrokerPositionsResponse = {
      ...emptyBrokerRuntime,
      checkedAt: "2026-05-16T00:00:00.000Z",
      connectivity: "connected",
      positions: [
        {
          accountId: "REAL-001",
          tradingEnvironment: "REAL",
          market: "HK",
          symbol: "HK.00700",
          symbolName: "Tencent",
          quantity: 100,
          sellableQuantity: 100,
          lastPrice: 320,
          costPrice: 300,
          averageCostPrice: 300,
          marketValue: 32000,
          unrealizedPnl: 2000,
          realizedPnl: 0,
          pnlRatio: 0.066,
          currency: "HKD",
        },
      ],
    };

    const brokerOrders: BrokerOrdersResponse = {
      ...emptyBrokerRuntime,
      checkedAt: "2026-05-16T00:00:00.000Z",
      connectivity: "connected",
      orders: [
        {
          accountId: "REAL-001",
          tradingEnvironment: "REAL",
          market: "HK",
          brokerOrderId: "9001",
          brokerOrderIdEx: "ex-9001",
          symbol: "HK.00700",
          symbolName: "Tencent",
          side: "BUY",
          orderType: "NORMAL",
          status: "SUBMITTED",
          quantity: 100,
          filledQuantity: 50,
          price: 320,
          filledAveragePrice: 319.5,
          submittedAt: "2026-05-16 10:00:00",
          updatedAt: "2026-05-16 10:01:00",
          remark: null,
          lastError: null,
          timeInForce: "DAY",
          currency: "HKD",
        },
      ],
    };

    const brokerCashFlows: BrokerCashFlowsResponse = {
      ...emptyBrokerCashFlows,
      checkedAt: "2026-05-16T00:00:00.000Z",
      connectivity: "connected",
      cashFlows: [
        {
          cashFlowId: "7001",
          clearingDate: "2026-05-16",
          settlementDate: "2026-05-19",
          currency: "HKD",
          type: "BUY_SETTLEMENT",
          direction: "OUT",
          amount: -32018.5,
          remark: "Order settlement",
        },
      ],
    };

    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);

      if (url.includes("/api/v1/system/status")) {
        return createResponse({
          ...emptySystemStatus,
          defaultTradingEnvironment: "REAL",
          broker: {
            ...emptySystemStatus.broker,
            displayName: "Futu Securities",
            capabilities: [
              { market: "HK", supportsQuote: true, supportsTrade: true },
            ],
          },
        });
      }
      if (url.includes("/api/v1/settings/brokers"))
        return createResponse(enabledFutuBrokerSettings());
      if (url.includes("/api/v1/system/storage/overview"))
        return createResponse(emptyStorageOverview);
      if (url.includes("/api/v1/system/real-trade-approvals"))
        return createResponse(emptyRealTradeApprovals);
      if (url.includes("/api/v1/system/real-trade-hard-stops"))
        return createResponse(emptyRealTradeHardStops);
      if (url.includes("/api/v1/system/real-trade-hard-stop-events"))
        return createResponse(emptyRealTradeHardStopEvents);
      if (url.includes("/api/v1/system/real-trade-kill-switch-events"))
        return createResponse(emptyRealTradeKillSwitchEvents);
      if (url.includes("/api/v1/system/real-trade-kill-switch"))
        return createResponse(emptyRealTradeKillSwitchState);
      if (url.includes("/api/v1/system/real-trade-risk-events"))
        return createResponse(emptyRealTradeRiskEvents);
      if (url.includes("/api/v1/system/real-trade-risk-limits"))
        return createResponse(emptyRealTradeRiskState);
      if (url.includes("/api/v1/system/worker/broker-order-updates"))
        return createResponse(emptyWorkerBrokerOrderUpdates);
      if (url.includes("/api/v1/brokers/futu/runtime"))
        return createResponse(brokerRuntime);
      if (url.includes("/api/v1/brokers/futu/funds"))
        return createResponse(brokerFunds);
      if (url.includes("/api/v1/brokers/futu/positions"))
        return createResponse(brokerPositions);
      if (url.includes("/api/v1/brokers/futu/orders"))
        return createResponse(brokerOrders);
      if (url.includes("/api/v1/brokers/futu/cash-flows"))
        return createResponse(brokerCashFlows);
      if (url.includes("/api/v1/portfolio/futu/cash-balances"))
        return createResponse(emptyPortfolioCashBalances);
      if (url.includes("/api/v1/portfolio/futu/positions"))
        return createResponse(emptyPortfolioPositions);
      if (url.includes("/api/v1/execution/orders"))
        return createResponse(emptyExecutionOrders);

      throw new Error(`Unexpected request: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/broker");

    expect(wrapper.text()).toContain("我的账户");
    expect(wrapper.text()).toContain("REAL-001");
    expect(wrapper.text()).toContain("总资产");
    expect(wrapper.text()).toContain("120,000 HKD");
    // 默认持仓 tab：终端风表格含现价 / 盈亏比例列，标的按交易所身份渲染。
    expect(wrapper.text()).toContain("现价");
    expect(wrapper.text()).toContain("盈亏比例");
    expect(wrapper.text()).toContain("00700");
    expect(wrapper.text()).toContain("Tencent");
    expect(wrapper.text()).toContain("券商");

    wrapper.unmount();
  });
});
