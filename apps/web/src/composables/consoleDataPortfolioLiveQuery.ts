import type { Ref } from "vue";

import type {
  PortfolioCashBalancesResponse,
  PortfolioPositionsResponse,
} from "@/contracts";
import {
  emptyPortfolioCashBalances,
  emptyPortfolioPositions,
} from "@/contracts";

import { apiGetPath } from "./apiClient";

interface CreateConsoleDataPortfolioLiveQueryControllerOptions {
  portfolioCashBalances: Ref<PortfolioCashBalancesResponse>;
  portfolioPositions: Ref<PortfolioPositionsResponse>;
}

export function createConsoleDataPortfolioLiveQueryController(
  options: CreateConsoleDataPortfolioLiveQueryControllerOptions,
) {
  async function loadPortfolioLiveData(input: {
    brokerId: string;
    brokerQuery: string;
  }): Promise<void> {
    const [cashBalances, positions] = await Promise.all([
      apiGetPath(
        "/api/v1/portfolio/{brokerId}/cash-balances",
        `/api/v1/portfolio/${encodeURIComponent(input.brokerId)}/cash-balances?${input.brokerQuery}`,
      ).catch(() => emptyPortfolioCashBalances),
      apiGetPath(
        "/api/v1/portfolio/{brokerId}/positions",
        `/api/v1/portfolio/${encodeURIComponent(input.brokerId)}/positions?${input.brokerQuery}`,
      ).catch(() => emptyPortfolioPositions),
    ]);

    options.portfolioCashBalances.value = cashBalances;
    options.portfolioPositions.value = positions;
  }

  return {
    loadPortfolioLiveData,
  };
}
