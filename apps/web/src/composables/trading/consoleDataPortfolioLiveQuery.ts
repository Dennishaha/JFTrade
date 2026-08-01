import type { Ref } from "vue";

import type {
  PortfolioCashBalancesResponse,
  PortfolioPositionsResponse,
} from "@/contracts";

import { apiGetPath } from "@/composables/shared/apiClient";

interface CreateConsoleDataPortfolioLiveQueryControllerOptions {
  portfolioCashBalances: Ref<PortfolioCashBalancesResponse>;
  portfolioPositions: Ref<PortfolioPositionsResponse>;
  portfolioLiveDataError: Ref<string>;
}

export function createConsoleDataPortfolioLiveQueryController(
  options: CreateConsoleDataPortfolioLiveQueryControllerOptions,
) {
  async function loadPortfolioLiveData(input: {
    brokerId: string;
    brokerQuery: string;
  }): Promise<void> {
    const [cashBalances, positions] = await Promise.allSettled([
      apiGetPath(
        "/api/v1/portfolio/{brokerId}/cash-balances",
        `/api/v1/portfolio/${encodeURIComponent(input.brokerId)}/cash-balances?${input.brokerQuery}`,
      ),
      apiGetPath(
        "/api/v1/portfolio/{brokerId}/positions",
        `/api/v1/portfolio/${encodeURIComponent(input.brokerId)}/positions?${input.brokerQuery}`,
      ),
    ]);

    const errors: string[] = [];
    if (cashBalances.status === "fulfilled") {
      const failure = brokerReadFailure(cashBalances.value);
      if (failure == null) {
        options.portfolioCashBalances.value = cashBalances.value;
      } else {
        errors.push(`现金余额加载失败: ${failure}`);
      }
    } else {
      errors.push(`现金余额加载失败: ${errorMessage(cashBalances.reason)}`);
    }
    if (positions.status === "fulfilled") {
      const failure = brokerReadFailure(positions.value);
      if (failure == null) {
        options.portfolioPositions.value = positions.value;
      } else {
        errors.push(`持仓加载失败: ${failure}`);
      }
    } else {
      errors.push(`持仓加载失败: ${errorMessage(positions.reason)}`);
    }
    options.portfolioLiveDataError.value = errors.join("；");
  }

  return {
    loadPortfolioLiveData,
  };
}

function errorMessage(cause: unknown): string {
  return cause instanceof Error && cause.message.trim() !== ""
    ? cause.message
    : "请求失败。";
}

function brokerReadFailure(
  value: PortfolioCashBalancesResponse | PortfolioPositionsResponse,
): string | null {
  const lastError = value.lastError?.trim() ?? "";
  if (lastError !== "") {
    return lastError;
  }
  // Older sidecars do not include read-status metadata on an otherwise valid payload.
  const connectivity =
    typeof value.connectivity === "string"
      ? value.connectivity.trim().toLowerCase()
      : "";
  if (connectivity === "" || connectivity === "connected") {
    return null;
  }
  if (connectivity === "disconnected") {
    return "券商连接已断开。";
  }
  if (connectivity === "degraded") {
    return "券商连接已降级。";
  }
  return `券商连接状态异常（${connectivity}）。`;
}
