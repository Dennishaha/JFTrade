import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  fetchEnvelope: vi.fn(),
  fetchEnvelopeWithInit: vi.fn(),
}));

vi.mock("../../src/composables/apiClient", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../src/composables/apiClient")>();
  return {
    ...actual,
    fetchEnvelope: mocks.fetchEnvelope,
    fetchEnvelopeWithInit: mocks.fetchEnvelopeWithInit,
  };
});

import { ApiClientError } from "../../src/composables/apiClient";
import {
  createStockScreenPreset,
  deleteStockScreenPreset,
  fetchStockScreenCatalog,
  fetchStockScreenPresets,
  isPresetConflict,
  runStockScreen,
  updateStockScreenPreset,
} from "../../src/components/research/stockScreenApi";
import type { StockScreenDefinitionV2 } from "../../src/components/research/stockScreenTypes";

const definition = {
  brokerId: "futu",
  market: "US",
  catalogVersion: "v1",
  querySchemaVersion: 2,
  conditions: [],
  columns: [],
  sorts: [],
} satisfies StockScreenDefinitionV2;

beforeEach(() => {
  mocks.fetchEnvelope.mockReset().mockResolvedValue({});
  mocks.fetchEnvelopeWithInit.mockReset().mockResolvedValue({});
});

describe("stock-screen API contract", () => {
  it("encodes catalog scope and supports the default broker", async () => {
    await fetchStockScreenCatalog("HK warrants", "futu/open-d");
    expect(mocks.fetchEnvelope).toHaveBeenLastCalledWith(
      "/api/v1/research/screens/catalog?market=HK%20warrants&brokerId=futu%2Fopen-d",
    );
    await fetchStockScreenCatalog("US");
    expect(mocks.fetchEnvelope).toHaveBeenLastCalledWith(
      "/api/v1/research/screens/catalog?market=US",
    );
  });

  it("posts a screen query and lists saved presets", async () => {
    const query = { market: "US", filters: [] };
    await runStockScreen(query);
    expect(mocks.fetchEnvelopeWithInit).toHaveBeenLastCalledWith(
      "/api/v1/research/screens",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(query),
      },
    );
    await fetchStockScreenPresets();
    expect(mocks.fetchEnvelope).toHaveBeenLastCalledWith(
      "/api/v1/research/screens/presets",
    );
  });

  it("creates, revises, and deletes encoded preset identities", async () => {
    await createStockScreenPreset("高股息", definition);
    expect(mocks.fetchEnvelopeWithInit).toHaveBeenLastCalledWith(
      "/api/v1/research/screens/presets",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ name: "高股息", definition }),
      }),
    );

    await updateStockScreenPreset("income/core", "核心高股息", definition, 7);
    expect(mocks.fetchEnvelopeWithInit).toHaveBeenLastCalledWith(
      "/api/v1/research/screens/presets/income%2Fcore",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({
          name: "核心高股息",
          definition,
          expectedRevision: 7,
        }),
      }),
    );

    await deleteStockScreenPreset("income/core");
    expect(mocks.fetchEnvelopeWithInit).toHaveBeenLastCalledWith(
      "/api/v1/research/screens/presets/income%2Fcore",
      { method: "DELETE" },
    );
  });

  it("recognizes only API revision conflicts", () => {
    expect(isPresetConflict(new ApiClientError("conflict", "REVISION", 409))).toBe(true);
    expect(isPresetConflict(new ApiClientError("denied", "AUTH", 403))).toBe(false);
    expect(isPresetConflict(new Error("conflict"))).toBe(false);
  });
});
