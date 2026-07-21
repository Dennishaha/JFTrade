import { afterEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

import {
  type PortfolioCashBalancesResponse,
  type PortfolioPositionsResponse,
  emptyPortfolioCashBalances,
  emptyPortfolioPositions,
} from "@/contracts";

const mocks = vi.hoisted(() => ({
  apiGetPath: vi.fn(),
}));

vi.mock("../src/composables/apiClient", () => ({
  apiGetPath: (...args: unknown[]) => mocks.apiGetPath(...args),
}));

import { createConsoleDataPortfolioLiveQueryController } from "../src/composables/consoleDataPortfolioLiveQuery";

afterEach(() => {
  vi.clearAllMocks();
});

describe("consoleDataPortfolioLiveQuery", () => {
  it("loads only supported portfolio resources and isolates their failures", async () => {
    const cashBalances = ref<PortfolioCashBalancesResponse>({
      ...emptyPortfolioCashBalances,
    });
    const positions = ref<PortfolioPositionsResponse>({
      ...emptyPortfolioPositions,
    });
    mocks.apiGetPath.mockImplementation(async (_template: string, path: string) => {
      if (path.includes("/cash-balances")) {
        throw new Error("cash unavailable");
      }
      if (path.includes("/positions")) {
        return {
          positions: [{ symbol: "US.AAPL" }],
        };
      }
      throw new Error(`Unexpected portfolio request: ${path}`);
    });

    const controller = createConsoleDataPortfolioLiveQueryController({
      portfolioCashBalances: cashBalances,
      portfolioPositions: positions,
    });
    await controller.loadPortfolioLiveData({
      brokerId: "futu",
      brokerQuery: "tradingEnvironment=REAL&market=US",
    });

    expect(cashBalances.value).toEqual(emptyPortfolioCashBalances);
    expect(positions.value.positions).toEqual([{ symbol: "US.AAPL" }]);
    expect(mocks.apiGetPath).toHaveBeenCalledTimes(2);
    const requestedPaths = mocks.apiGetPath.mock.calls.map((call) => String(call[1]));
    expect(requestedPaths.every((path) => !path.includes("reconciliation"))).toBe(true);
  });

  it("keeps cash balances when the positions request fails", async () => {
    const cashBalances = ref<PortfolioCashBalancesResponse>({
      ...emptyPortfolioCashBalances,
    });
    const positions = ref<PortfolioPositionsResponse>({
      ...emptyPortfolioPositions,
      lastError: "stale",
    });
    mocks.apiGetPath.mockImplementation(async (_template: string, path: string) => {
      if (path.includes("/cash-balances")) {
        return {
          balances: [{ currency: "USD", cashBalance: 100 }],
        };
      }
      throw new Error("positions unavailable");
    });

    const controller = createConsoleDataPortfolioLiveQueryController({
      portfolioCashBalances: cashBalances,
      portfolioPositions: positions,
    });
    await controller.loadPortfolioLiveData({
      brokerId: "futu",
      brokerQuery: "tradingEnvironment=REAL&market=US",
    });

    expect(cashBalances.value.balances).toEqual([
      { currency: "USD", cashBalance: 100 },
    ]);
    expect(positions.value).toEqual(emptyPortfolioPositions);
  });
});
