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
    apiGet: (path: string) => mocks.fetchEnvelope(path),
    apiGetPath: (_template: string, path: string) => mocks.fetchEnvelope(path),
    apiPost: (path: string, body: unknown) =>
      mocks.fetchEnvelopeWithInit(path, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      }),
    apiPatchPath: (_template: string, path: string, body: unknown) =>
      mocks.fetchEnvelopeWithInit(path, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      }),
    apiDeletePath: (_template: string, path: string) =>
      mocks.fetchEnvelopeWithInit(path, { method: "DELETE" }),
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
  mocks.fetchEnvelope.mockReset();
  mocks.fetchEnvelopeWithInit.mockReset();
});

const catalog = {
  version: "v1",
  schemaVersion: 1,
  querySchemaVersion: 2,
  provider: "futu",
  providerVersion: "1",
  markets: ["US"],
  categories: [],
  factors: [],
  enums: {},
  rateLimit: { requests: 1, windowSeconds: 1 },
};

const preset = {
  presetId: "income/core",
  name: "高股息",
  querySchemaVersion: 2,
  definition: { ...definition, pool: {} },
  revision: 1,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

describe("stock-screen API contract", () => {
  it("encodes catalog scope and supports the default broker", async () => {
    mocks.fetchEnvelope.mockResolvedValue(catalog);
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
    const query = {
      ...definition,
      page: { offset: 0, limit: 50 },
    };
    mocks.fetchEnvelopeWithInit.mockResolvedValue({ entries: [] });
    await runStockScreen(query);
    expect(mocks.fetchEnvelopeWithInit).toHaveBeenLastCalledWith(
      "/api/v1/research/screens",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: expect.any(String),
      },
    );
    mocks.fetchEnvelope.mockResolvedValue({ presets: [] });
    await fetchStockScreenPresets();
    expect(mocks.fetchEnvelope).toHaveBeenLastCalledWith(
      "/api/v1/research/screens/presets",
    );
  });

  it("maps every optional V2 screen field at the generated wire boundary", async () => {
    const fullDefinition: StockScreenDefinitionV2 = {
      ...definition,
      pool: {
        watchlistStockIds: ["stock-1"],
        plates: [
          { parentPlateId: "sector-parent", plateIds: ["sector-child"] },
          { plateIds: ["standalone"] },
        ],
      },
      conditions: [
        {
          id: "condition-1",
          factor: {
            instanceId: "factor-1",
            factorKey: "price",
            params: { days: 5 },
          },
          operator: "between",
          value: { min: 10, max: 20 },
          secondFactor: {
            instanceId: "factor-2",
            factorKey: "moving_average",
          },
        },
        {
          id: "condition-2",
          factor: { instanceId: "factor-3", factorKey: "volume" },
          operator: "greater_than",
        },
      ],
      columns: [
        {
          columnId: "price-column",
          factor: { instanceId: "factor-1", factorKey: "price" },
          label: "价格",
        },
        {
          columnId: "volume-column",
          factor: { instanceId: "factor-3", factorKey: "volume" },
        },
      ],
      sorts: [
        {
          sortId: "sort-1",
          columnId: "price-column",
          factor: { instanceId: "factor-1", factorKey: "price" },
          direction: "desc",
        },
        {
          factor: { instanceId: "factor-3", factorKey: "volume" },
          direction: "asc",
        },
      ],
    };
    mocks.fetchEnvelopeWithInit.mockResolvedValue({ entries: [] });

    await runStockScreen({
      ...fullDefinition,
      accountId: "account-1",
      tradingEnvironment: "REAL",
      page: { offset: 10, limit: 25 },
    });

    const request = mocks.fetchEnvelopeWithInit.mock.lastCall?.[1] as {
      body: string;
    };
    expect(JSON.parse(request.body)).toMatchObject({
      accountId: "account-1",
      tradingEnvironment: "REAL",
      pool: {
        watchlistStockIds: ["stock-1"],
        plates: [
          { parentPlateId: "sector-parent", plateIds: ["sector-child"] },
          { plateIds: ["standalone"] },
        ],
      },
      conditions: [
        {
          value: { min: 10, max: 20 },
          secondFactor: {
            instanceId: "factor-2",
            factorKey: "moving_average",
            params: {},
          },
        },
        {
          factor: {
            instanceId: "factor-3",
            factorKey: "volume",
            params: {},
          },
        },
      ],
      columns: [{ label: "价格" }, { columnId: "volume-column" }],
      sorts: [{ sortId: "sort-1", columnId: "price-column" }, { direction: "asc" }],
    });

    mocks.fetchEnvelope.mockResolvedValue({
      presets: [{ ...preset, definition: fullDefinition }],
    });
    await expect(fetchStockScreenPresets()).resolves.toMatchObject({
      presets: [
        {
          querySchemaVersion: 2,
          definition: {
            conditions: [
              expect.objectContaining({ value: { min: 10, max: 20 } }),
              expect.objectContaining({ id: "condition-2" }),
            ],
            columns: [
              expect.objectContaining({ label: "价格" }),
              expect.objectContaining({ columnId: "volume-column" }),
            ],
            sorts: [
              expect.objectContaining({ sortId: "sort-1" }),
              expect.objectContaining({ direction: "asc" }),
            ],
          },
        },
      ],
    });
  });

  it("normalizes catalog editor enums and rejects unknown values without widening UI types", async () => {
    mocks.fetchEnvelope.mockResolvedValue({
      ...catalog,
      factors: [
        {
          key: "price",
          category: "quote",
          label: "价格",
          valueType: "number",
          filterKind: "interval",
          conditionEditor: "range",
          filter: true,
          retrieve: true,
          sort: true,
          availability: "available",
        },
        {
          key: "custom",
          category: "custom",
          label: "自定义",
          valueType: "string",
          filterKind: "future-kind",
          conditionEditor: "future-editor",
          filter: true,
          retrieve: false,
          sort: false,
          availability: "experimental",
        },
      ],
    });

    const result = await fetchStockScreenCatalog("US");
    expect(result.factors[0]).toMatchObject({
      filterKind: "interval",
      conditionEditor: "range",
    });
    expect(result.factors[1]).toMatchObject({ filterKind: "" });
    expect(result.factors[1]).not.toHaveProperty("conditionEditor");
  });

  it("creates, revises, and deletes encoded preset identities", async () => {
    mocks.fetchEnvelopeWithInit.mockResolvedValue(preset);
    await createStockScreenPreset("高股息", definition);
    expect(mocks.fetchEnvelopeWithInit).toHaveBeenLastCalledWith(
      "/api/v1/research/screens/presets",
      expect.objectContaining({
        method: "POST",
        body: expect.any(String),
      }),
    );

    await updateStockScreenPreset("income/core", "核心高股息", definition, 7);
    expect(mocks.fetchEnvelopeWithInit).toHaveBeenLastCalledWith(
      "/api/v1/research/screens/presets/income%2Fcore",
      expect.objectContaining({
        method: "PATCH",
        body: expect.any(String),
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
