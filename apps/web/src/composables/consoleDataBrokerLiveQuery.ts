import type { Ref } from "vue";

import {
  type BrokerCashFlowsResponse,
  type BrokerFundsResponse,
  type BrokerOrdersResponse,
  type BrokerPositionsResponse,
  type ExecutionOrdersResponse,
  type SystemStatusResponse,
  emptyBrokerCashFlows,
  emptyBrokerFunds,
  emptyBrokerOrders,
  emptyBrokerPositions,
} from "@jftrade/ui-contracts";

import { fetchEnvelope } from "./apiClient";

interface CreateConsoleDataBrokerLiveQueryControllerOptions {
  systemStatus: Ref<SystemStatusResponse>;
  brokerCashFlows: Ref<BrokerCashFlowsResponse>;
  brokerFunds: Ref<BrokerFundsResponse>;
  brokerPositions: Ref<BrokerPositionsResponse>;
  brokerOrders: Ref<BrokerOrdersResponse>;
  executionOrders: Ref<ExecutionOrdersResponse>;
  loadPortfolioLiveData: (input: {
    brokerId: string;
    brokerQuery: string;
  }) => Promise<void>;
}

export function createConsoleDataBrokerLiveQueryController(
  options: CreateConsoleDataBrokerLiveQueryControllerOptions,
) {
  async function loadBrokerCashFlows(input: {
    brokerId: string;
    brokerQuery: string;
    tradingEnvironment: string;
    clearingDate: string;
  }): Promise<void> {
    options.brokerCashFlows.value = emptyBrokerCashFlows;

    if (input.tradingEnvironment !== "REAL") {
      return;
    }

    try {
      options.brokerCashFlows.value =
        await fetchEnvelope<BrokerCashFlowsResponse>(
          `/api/v1/brokers/${encodeURIComponent(input.brokerId)}/cash-flows?${input.brokerQuery}&clearingDate=${encodeURIComponent(input.clearingDate)}`,
        );
    } catch (error) {
      options.brokerCashFlows.value = {
        ...emptyBrokerCashFlows,
        connectivity: "disconnected",
        lastError:
          error instanceof Error
            ? error.message
            : "Failed to load broker cash flows.",
      };
    }
  }

  async function loadBrokerLiveData(input: {
    brokerId: string;
    brokerQuery: string;
    futuBrokerReadsPaused: boolean;
  }): Promise<void> {
    const liveBrokerDataPromise: Promise<
      readonly [
        BrokerFundsResponse,
        BrokerPositionsResponse,
        BrokerOrdersResponse,
      ]
    > = input.futuBrokerReadsPaused
      ? Promise.resolve([
          emptyBrokerFunds,
          emptyBrokerPositions,
          emptyBrokerOrders,
        ] as const)
      : Promise.all([
          fetchEnvelope<BrokerFundsResponse>(
            `/api/v1/brokers/${encodeURIComponent(input.brokerId)}/funds?${input.brokerQuery}`,
          ),
          fetchEnvelope<BrokerPositionsResponse>(
            `/api/v1/brokers/${encodeURIComponent(input.brokerId)}/positions?${input.brokerQuery}`,
          ),
          fetchEnvelope<BrokerOrdersResponse>(
            `/api/v1/brokers/${encodeURIComponent(input.brokerId)}/orders?${input.brokerQuery}`,
          ),
        ]).then(([funds, positions, orders]) => [funds, positions, orders] as const);

    const [[funds, positions, orders], , executionOrderList] =
      await Promise.all([
      liveBrokerDataPromise,
      options.loadPortfolioLiveData({
        brokerId: input.brokerId,
        brokerQuery: input.brokerQuery,
      }),
      fetchEnvelope<ExecutionOrdersResponse>("/api/v1/execution/orders"),
      ]);

    options.brokerFunds.value = funds;
    options.brokerPositions.value = positions;
    options.brokerOrders.value = orders;
    options.executionOrders.value = executionOrderList;

    if (input.futuBrokerReadsPaused) {
      options.brokerCashFlows.value = emptyBrokerCashFlows;
      return;
    }

    await loadBrokerCashFlows({
      brokerId: input.brokerId,
      brokerQuery: input.brokerQuery,
      tradingEnvironment:
        funds.summary?.tradingEnvironment ??
        options.systemStatus.value.defaultTradingEnvironment,
      clearingDate: funds.checkedAt.slice(0, 10),
    });
  }

  return {
    loadBrokerLiveData,
  };
}