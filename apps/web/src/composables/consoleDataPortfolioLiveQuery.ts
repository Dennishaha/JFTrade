import type { Ref } from "vue";

import type {
  PortfolioCashBalancesResponse,
  PortfolioCashReconciliationResponse,
  PortfolioPositionsResponse,
  PortfolioReconciliationResponse,
} from "@/contracts";

import { apiGetPath } from "./apiClient";

interface CreateConsoleDataPortfolioLiveQueryControllerOptions {
  portfolioCashBalances: Ref<PortfolioCashBalancesResponse>;
  portfolioCashReconciliation: Ref<PortfolioCashReconciliationResponse>;
  portfolioPositions: Ref<PortfolioPositionsResponse>;
  portfolioReconciliation: Ref<PortfolioReconciliationResponse>;
}

export function createConsoleDataPortfolioLiveQueryController(
  options: CreateConsoleDataPortfolioLiveQueryControllerOptions,
) {
  async function loadPortfolioLiveData(input: {
    brokerId: string;
    brokerQuery: string;
  }): Promise<void> {
    const [cashBalances, positions, cashReconciliation, reconciliation] =
      await Promise.all([
        apiGetPath(
          "/api/v1/portfolio/{brokerId}/cash-balances",
          `/api/v1/portfolio/${encodeURIComponent(input.brokerId)}/cash-balances?${input.brokerQuery}`,
        ),
        apiGetPath(
          "/api/v1/portfolio/{brokerId}/positions",
          `/api/v1/portfolio/${encodeURIComponent(input.brokerId)}/positions?${input.brokerQuery}`,
        ),
        apiGetPath(
          "/api/v1/portfolio/{brokerId}/cash-reconciliation",
          `/api/v1/portfolio/${encodeURIComponent(input.brokerId)}/cash-reconciliation?${input.brokerQuery}`,
        ),
        apiGetPath(
          "/api/v1/portfolio/{brokerId}/reconciliation",
          `/api/v1/portfolio/${encodeURIComponent(input.brokerId)}/reconciliation?${input.brokerQuery}`,
        ),
      ]);

    options.portfolioCashBalances.value = cashBalances;
    options.portfolioPositions.value = positions;
    options.portfolioCashReconciliation.value = cashReconciliation;
    options.portfolioReconciliation.value = reconciliation;
  }

  return {
    loadPortfolioLiveData,
  };
}