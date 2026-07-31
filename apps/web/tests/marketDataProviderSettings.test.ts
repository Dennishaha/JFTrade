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
  defaultMarketDataProviderSettings,
  getMarketDataProviderSettings,
  getMarketDataProviderStatus,
  putMarketDataProviderSettings,
} from "@/composables/settings/marketDataProviderSettings";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("market data provider settings transport", () => {
  it("defaults incomplete or unknown selections to the embedded Yahoo provider", async () => {
    mocks.apiGet.mockResolvedValue({ activeProvider: "unsupported" });

    await expect(getMarketDataProviderSettings()).resolves.toEqual(
      defaultMarketDataProviderSettings,
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
