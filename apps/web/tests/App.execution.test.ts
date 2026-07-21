// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  emptyBrokerFunds,
  emptyBrokerSettings,
  emptyBrokerOrderFees,
  emptyBrokerOrders,
  emptyBrokerPositions,
  emptyBrokerRuntime,
  emptyFutuOpenDHealth,
  emptyExecutionOrderEvents,
  emptyPortfolioCashBalances,
  emptyPortfolioPositions,
  emptyRealTradeApprovals,
  emptyRealTradeHardStopEvents,
  emptyRealTradeHardStops,
  emptyRealTradeKillSwitchEvents,
  emptyRealTradeKillSwitchState,
  emptyRealTradeRiskEvents,
  emptyRealTradeRiskState,
  emptySystemStatus,
} from "@/contracts";
import type {
  BrokerOrderFeesResponse,
  ExecutionOrderEventsResponse,
  ExecutionOrderSummaryResponse,
  ExecutionOrdersResponse,
} from "@/contracts";

import { MockWebSocket, createResponse, flushRequests, mountApp } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
});

describe("Account page execution route redirect", () => {
  it("shows pending orders, order events, and broker order fees", async () => {
    const executionOrders: ExecutionOrdersResponse = {
      ...emptySystemStatus,
      orders: [
        {
          internalOrderId: "order-internal-1",
          brokerId: "futu",
          brokerOrderId: "9001",
          brokerOrderIdEx: "ex-9001",
          source: "system",
          sourceDetail: "command.place",
          tradingEnvironment: "REAL",
          accountId: "REAL-001",
          market: "HK",
          symbol: "HK.00700",
          side: "BUY",
          orderType: "NORMAL",
          status: "SUBMITTED",
          requestedQuantity: 100,
          requestedPrice: 320,
          filledQuantity: 50,
          filledAveragePrice: 319.5,
          remark: null,
          lastError: "modify rejected by broker",
          lastErrorCode: "MODIFY_REJECTED",
          lastErrorSource: "command.modify.broker",
          submittedAt: "2026-05-16 10:00:00",
          updatedAt: "2026-05-16 10:01:00",
          createdAt: "2026-05-16T00:00:00.000Z",
        },
      ] satisfies ExecutionOrderSummaryResponse[],
    };

    const executionOrderEvents: ExecutionOrderEventsResponse = {
      ...emptyExecutionOrderEvents,
      internalOrderId: "order-internal-1",
      events: [
        {
          id: "event-1",
          internalOrderId: "order-internal-1",
          eventType: "COMMAND_PLACE_ACCEPTED",
          previousStatus: null,
          nextStatus: "SUBMITTED",
          payloadJson: '{"ok":true}',
          createdAt: "2026-05-16T00:00:00.000Z",
        },
      ],
    };

    const brokerOrderFees: BrokerOrderFeesResponse = {
      ...emptyBrokerOrderFees,
      checkedAt: "2026-05-16T00:01:00.000Z",
      connectivity: "connected",
      fees: [
        {
          accountId: "REAL-001",
          tradingEnvironment: "REAL",
          market: "HK",
          brokerOrderIdEx: "ex-9001",
          feeAmount: 18.5,
          feeItems: [{ title: "Commission", value: 10 }],
        },
      ],
    };

    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);

      if (url.includes("/api/v1/system/status"))
        return createResponse({
          ...emptySystemStatus,
          defaultTradingEnvironment: "REAL",
        });
      if (url.includes("/api/v1/settings/brokers"))
        return createResponse({
          ...emptyBrokerSettings,
          brokers: [
            {
              descriptor: {
                ...emptyBrokerRuntime.descriptor,
                id: "futu",
                displayName: "Futu OpenAPI via OpenD",
                environments: ["SIMULATE", "REAL"],
                capabilities: [
                  {
                    market: "HK",
                    supportsQuote: true,
                    supportsTrade: true,
                    readFeatures: {
                      orderFees: {
                        supportedEnvironments: ["REAL"],
                        requiresOrderIdEx: true,
                      },
                    },
                  },
                ],
                notes: [],
              },
              integration: {
                brokerId: "futu",
                enabled: true,
                config: {
                  type: "futu",
                  host: "127.0.0.1",
                  apiPort: 11110,
                  websocketPort: 11111,
                  maxWebSocketConnections: 20,
                  useEncryption: false,
                  websocketKey: "",
                  tradeMarket: "HK",
                  simulatedAccountType: "STOCK",
                },
                updatedAt: "2026-05-16T00:00:00.000Z",
                createdAt: "2026-05-16T00:00:00.000Z",
              },
              accounts: [],
              defaults: null,
            },
          ],
        });
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
      if (url.includes("/api/v1/system/futu-opend"))
        return createResponse(emptyFutuOpenDHealth);
      if (url.includes("/api/v1/brokers/futu/runtime"))
        return createResponse({
          ...emptyBrokerRuntime,
          accounts: [
            {
              accountId: "REAL-001",
              tradingEnvironment: "REAL",
              accountType: "MARGIN",
              accountRole: null,
              securityFirm: "FUTUSECURITIES",
              marketAuthorities: ["HK"],
              simulatedAccountType: null,
            },
          ],
          descriptor: {
            ...emptyBrokerRuntime.descriptor,
            capabilities: [
              {
                market: "HK",
                supportsQuote: true,
                supportsTrade: true,
                readFeatures: {
                  orderFees: {
                    supportedEnvironments: ["REAL"],
                    requiresOrderIdEx: true,
                  },
                },
              },
            ],
          },
        });
      if (url.includes("/api/v1/brokers/futu/funds"))
        return createResponse({
          ...emptyBrokerFunds,
          checkedAt: "2026-05-16T00:00:00.000Z",
          connectivity: "connected",
          summary: {
            accountId: "REAL-001",
            tradingEnvironment: "REAL",
            market: "HK",
            currency: "HKD",
            totalAssets: 0,
            securitiesAssets: 0,
            fundAssets: 0,
            bondAssets: 0,
            cash: 0,
            marketValue: 0,
            longMarketValue: 0,
            shortMarketValue: 0,
            purchasingPower: 0,
            shortSellingPower: 0,
            netCashPower: 0,
            availableWithdrawalCash: 0,
            maxWithdrawal: 0,
            availableFunds: 0,
            frozenCash: 0,
            pendingAsset: 0,
            unrealizedPnl: null,
            realizedPnl: null,
            initialMargin: null,
            maintenanceMargin: null,
            marginCallMargin: null,
            riskStatus: null,
          },
        });
      if (url.includes("/api/v1/brokers/futu/positions"))
        return createResponse(emptyBrokerPositions);
      if (url.includes("/api/v1/brokers/futu/orders"))
        return createResponse(emptyBrokerOrders);
      if (url.includes("/api/v1/brokers/futu/order-fees"))
        return createResponse(brokerOrderFees);
      if (url.includes("/api/v1/portfolio/futu/cash-balances"))
        return createResponse(emptyPortfolioCashBalances);
      if (url.includes("/api/v1/portfolio/futu/positions"))
        return createResponse(emptyPortfolioPositions);
      if (url.includes("/api/v1/execution/orders/order-internal-1/events"))
        return createResponse(executionOrderEvents);
      if (url.includes("/api/v1/execution/orders"))
        return createResponse(executionOrders);

      throw new Error(`Unexpected request: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/execution");
    await flushRequests();

    expect(wrapper.text()).toContain("我的账户");

    // 在途订单在「订单」tab，事件与券商费用在「历史」tab 的详情侧栏。
    const ordersTab = wrapper
      .findAll('button[role="tab"]')
      .find((candidate) => candidate.text().includes("订单"));
    expect(ordersTab).toBeDefined();
    await ordersTab!.trigger("click");
    await flushRequests();
    expect(wrapper.text()).toContain("order-internal-1");
    expect(wrapper.text()).toContain("已提交");

    const historyTab = wrapper
      .findAll('button[role="tab"]')
      .find((candidate) => candidate.text().includes("历史"));
    expect(historyTab).toBeDefined();
    await historyTab!.trigger("click");
    await flushRequests();
    expect(wrapper.text()).toContain("下单已受理");
    expect(wrapper.text()).toContain("18.5");

    wrapper.unmount();
  });
});
