import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  apiGet: vi.fn(),
  apiPut: vi.fn(),
}));

vi.mock("@/composables/shared/apiClient", () => ({
  apiGet: mocks.apiGet,
  apiPut: mocks.apiPut,
}));

import {
  getMarketDataProviderSettings,
  getMarketDataProviderStatus,
  putMarketDataProviderSettings,
} from "@/composables/settings/marketDataProviderSettings";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("market data provider settings transport", () => {
  it("rejects incomplete or unknown provider selections", async () => {
    mocks.apiGet.mockResolvedValue({ activeProvider: "unsupported" });

    await expect(getMarketDataProviderSettings()).rejects.toThrow(
      "不支持的行情提供者",
    );
    expect(mocks.apiGet).toHaveBeenCalledWith(
      "/api/v1/settings/market-data-provider",
    );
  });

  it("preserves an explicit Futu selection", async () => {
    mocks.apiGet.mockResolvedValue({ activeProvider: "futu" });

    await expect(getMarketDataProviderSettings()).resolves.toEqual({
      activeProvider: "futu",
    });
  });

  it("preserves an explicit Yahoo Finance selection", async () => {
    mocks.apiGet.mockResolvedValue({ activeProvider: "yfinance" });

    await expect(getMarketDataProviderSettings()).resolves.toEqual({
      activeProvider: "yfinance",
    });
  });

  it("updates the provider through the single selection endpoint", async () => {
    mocks.apiPut.mockResolvedValue({ activeProvider: "futu" });

    await expect(putMarketDataProviderSettings("futu")).resolves.toEqual({
      activeProvider: "futu",
    });
    expect(mocks.apiPut).toHaveBeenCalledWith(
      "/api/v1/settings/market-data-provider",
      { activeProvider: "futu" },
    );
  });

  it("returns provider status without transforming runtime details", async () => {
    const status = { descriptor: { providerId: "futu-opend" } };
    mocks.apiGet.mockResolvedValue(status);

    await expect(getMarketDataProviderStatus()).resolves.toBe(status);
    expect(mocks.apiGet).toHaveBeenCalledWith("/api/v1/market-data/provider");
  });
});
