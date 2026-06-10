import type { Ref } from "vue";

import type {
  PortfolioCashBalancesResponse,
  PortfolioCashReconciliationResponse,
  PortfolioPositionsResponse,
  PortfolioReconciliationResponse,
} from "@/contracts";

import { fetchEnvelope } from "./apiClient";

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
        fetchEnvelope<PortfolioCashBalancesResponse>(
          `/api/v1/portfolio/${encodeURIComponent(input.brokerId)}/cash-balances?${input.brokerQuery}`,
        ),
        fetchEnvelope<PortfolioPositionsResponse>(
          `/api/v1/portfolio/${encodeURIComponent(input.brokerId)}/positions?${input.brokerQuery}`,
        ),
        fetchEnvelope<PortfolioCashReconciliationResponse>(
          `/api/v1/portfolio/${encodeURIComponent(input.brokerId)}/cash-reconciliation?${input.brokerQuery}`,
        ),
        fetchEnvelope<PortfolioReconciliationResponse>(
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