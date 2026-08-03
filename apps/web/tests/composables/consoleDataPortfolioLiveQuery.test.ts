import { afterEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

import {
  type PortfolioCashBalancesResponse,
  type PortfolioPositionsResponse,
  emptyPortfolioCashBalances,
  emptyPortfolioPositions,
} from "@/types";

const mocks = vi.hoisted(() => ({
  apiGetPath: vi.fn(),
}));

vi.mock("@/composables/shared/apiClient", () => ({
  apiGetPath: (...args: unknown[]) => mocks.apiGetPath(...args),
}));

import { createConsoleDataPortfolioLiveQueryController } from "@/composables/trading/consoleDataPortfolioLiveQuery";

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
    const portfolioLiveDataError = ref("");
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
      portfolioLiveDataError,
    });
    await controller.loadPortfolioLiveData({
      brokerId: "futu",
      brokerQuery: "tradingEnvironment=REAL&market=US",
    });

    expect(cashBalances.value).toEqual(emptyPortfolioCashBalances);
    expect(positions.value.positions).toEqual([{ symbol: "US.AAPL" }]);
    expect(portfolioLiveDataError.value).toBe("现金余额加载失败: cash unavailable");
    expect(mocks.apiGetPath).toHaveBeenCalledTimes(2);
    const requestedPaths = mocks.apiGetPath.mock.calls.map((call) => String(call[1]));
    expect(requestedPaths.every((path) => !path.includes("reconciliation"))).toBe(true);
  });

  it("keeps the last successful response and reports a resource-specific failure", async () => {
    const cashBalances = ref<PortfolioCashBalancesResponse>({
      ...emptyPortfolioCashBalances,
      balances: [{ currency: "USD", cashBalance: 100 }],
    });
    const positions = ref<PortfolioPositionsResponse>({
      ...emptyPortfolioPositions,
      positions: [{ symbol: "US.AAPL" }],
    });
    const portfolioLiveDataError = ref("previous failure");
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
      portfolioLiveDataError,
    });
    await controller.loadPortfolioLiveData({
      brokerId: "futu",
      brokerQuery: "tradingEnvironment=REAL&market=US",
    });

    expect(cashBalances.value.balances).toEqual([
      { currency: "USD", cashBalance: 100 },
    ]);
    expect(positions.value.positions).toEqual([{ symbol: "US.AAPL" }]);
    expect(portfolioLiveDataError.value).toBe("持仓加载失败: positions unavailable");

    mocks.apiGetPath.mockImplementation(async (_template: string, path: string) =>
      path.includes("/cash-balances")
        ? { balances: [{ currency: "USD", cashBalance: 120 }] }
        : { positions: [{ symbol: "US.MSFT" }] },
    );
    await controller.loadPortfolioLiveData({
      brokerId: "futu",
      brokerQuery: "tradingEnvironment=REAL&market=US",
    });

    expect(portfolioLiveDataError.value).toBe("");
    expect(positions.value.positions).toEqual([{ symbol: "US.MSFT" }]);
  });

  it("keeps cached resources when a successful HTTP response reports a broker read failure", async () => {
    const cashBalances = ref<PortfolioCashBalancesResponse>({
      ...emptyPortfolioCashBalances,
      balances: [{ currency: "USD", cashBalance: 100 }],
    });
    const positions = ref<PortfolioPositionsResponse>({
      ...emptyPortfolioPositions,
      positions: [{ symbol: "US.AAPL" }],
    });
    const portfolioLiveDataError = ref("");
    mocks.apiGetPath.mockImplementation(async (_template: string, path: string) =>
      path.includes("/cash-balances")
        ? {
            balances: [],
            connectivity: "degraded",
            lastError: "cash upstream unavailable",
          }
        : {
            positions: [],
            connectivity: "degraded",
            lastError: null,
          },
    );

    const controller = createConsoleDataPortfolioLiveQueryController({
      portfolioCashBalances: cashBalances,
      portfolioPositions: positions,
      portfolioLiveDataError,
    });
    await controller.loadPortfolioLiveData({
      brokerId: "futu",
      brokerQuery: "tradingEnvironment=REAL&market=US",
    });

    expect(cashBalances.value.balances).toEqual([
      { currency: "USD", cashBalance: 100 },
    ]);
    expect(positions.value.positions).toEqual([{ symbol: "US.AAPL" }]);
    expect(portfolioLiveDataError.value).toBe(
      "现金余额加载失败: cash upstream unavailable；持仓加载失败: 券商连接已降级。",
    );
  });
});
