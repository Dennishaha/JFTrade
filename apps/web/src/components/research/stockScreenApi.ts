import {
  ApiClientError,
  apiDeletePath,
  apiGet,
  apiGetPath,
  apiPatchPath,
  apiPost,
} from "@/composables/shared/apiClient";
import type {
  BrokerScreenDefinitionV2Dto,
  BrokerScreenQueryV2Dto,
  ResearchScreenPresetDto,
} from "@/contracts";
import type {
  StockScreenCatalog,
  StockScreenDefinitionV2,
  StockScreenFactor,
  StockScreenPreset,
  StockScreenPresetList,
  StockScreenQuery,
  StockScreenResult,
} from "./stockScreenTypes";

function brokerQuery(brokerId?: string): string {
  return brokerId ? `&brokerId=${encodeURIComponent(brokerId)}` : "";
}

type ScreenQueryWire = BrokerScreenQueryV2Dto;
type ScreenDefinitionWire = BrokerScreenDefinitionV2Dto;
type ScreenPresetWire = ResearchScreenPresetDto;

function screenPool(pool: StockScreenDefinitionV2["pool"]): ScreenDefinitionWire["pool"] {
  return {
    ...(pool?.watchlistStockIds == null
      ? {}
      : { watchlistStockIds: pool.watchlistStockIds }),
    ...(pool?.plates == null
      ? {}
      : {
          plates: pool.plates.map((plate) => ({
            ...(plate.parentPlateId == null
              ? {}
              : { parentPlateId: plate.parentPlateId }),
            plateIds: plate.plateIds,
          })),
        }),
  };
}

function screenDefinition(
  definition: StockScreenDefinitionV2,
): ScreenDefinitionWire {
  return {
    ...(definition.brokerId == null
      ? {}
      : { brokerId: definition.brokerId }),
    market: definition.market,
    pool: screenPool(definition.pool),
    conditions: definition.conditions.map((condition) => ({
      id: condition.id,
      factor: {
        instanceId: condition.factor.instanceId,
        factorKey: condition.factor.factorKey,
        params: condition.factor.params ?? {},
      },
      operator: condition.operator,
      ...(condition.value === undefined ? {} : { value: condition.value }),
      ...(condition.secondFactor == null
        ? {}
        : {
            secondFactor: {
              instanceId: condition.secondFactor.instanceId,
              factorKey: condition.secondFactor.factorKey,
              params: condition.secondFactor.params ?? {},
            },
          }),
    })),
    columns: definition.columns.map((column) => ({
      columnId: column.columnId,
      factor: {
        instanceId: column.factor.instanceId,
        factorKey: column.factor.factorKey,
        params: column.factor.params ?? {},
      },
      ...(column.label == null ? {} : { label: column.label }),
    })),
    sorts: definition.sorts.map((sort) => ({
      ...(sort.sortId == null ? {} : { sortId: sort.sortId }),
      ...(sort.columnId == null ? {} : { columnId: sort.columnId }),
      factor: {
        instanceId: sort.factor.instanceId,
        factorKey: sort.factor.factorKey,
        params: sort.factor.params ?? {},
      },
      direction: sort.direction,
    })),
    catalogVersion: definition.catalogVersion,
    querySchemaVersion: definition.querySchemaVersion,
  };
}

function screenQuery(query: StockScreenQuery): ScreenQueryWire {
  return {
    ...screenDefinition(query),
    ...(query.accountId == null ? {} : { accountId: query.accountId }),
    ...(query.tradingEnvironment == null
      ? {}
      : { tradingEnvironment: query.tradingEnvironment }),
    page: query.page,
  };
}

function mapScreenDefinition(
  definition: ScreenDefinitionWire,
): StockScreenDefinitionV2 {
  return {
    ...(definition.brokerId == null
      ? {}
      : { brokerId: definition.brokerId }),
    market: definition.market,
    pool: definition.pool,
    conditions: (definition.conditions ?? []).map((condition) => ({
      id: condition.id,
      factor: condition.factor,
      operator: condition.operator,
      ...(condition.value === undefined ? {} : { value: condition.value }),
      ...(condition.secondFactor == null
        ? {}
        : { secondFactor: condition.secondFactor }),
    })),
    columns: (definition.columns ?? []).map((column) => ({
      columnId: column.columnId,
      factor: column.factor,
      ...(column.label == null ? {} : { label: column.label }),
    })),
    sorts: (definition.sorts ?? []).map((sort) => ({
      ...(sort.sortId == null ? {} : { sortId: sort.sortId }),
      ...(sort.columnId == null ? {} : { columnId: sort.columnId }),
      factor: sort.factor,
      direction: sort.direction,
    })),
    catalogVersion: definition.catalogVersion,
    querySchemaVersion: 2,
  };
}

function mapScreenPreset(preset: ScreenPresetWire): StockScreenPreset {
  return {
    ...preset,
    querySchemaVersion: 2,
    definition: mapScreenDefinition(preset.definition),
  };
}

export function fetchStockScreenCatalog(
  market: string,
  brokerId?: string,
): Promise<StockScreenCatalog> {
  return apiGetPath(
    "/api/v1/research/screens/catalog",
    `/api/v1/research/screens/catalog?market=${encodeURIComponent(market)}${brokerQuery(brokerId)}`,
  ).then((catalog) => ({
    ...catalog,
    querySchemaVersion: 2,
    factors: catalog.factors.map((factor) => {
      const {
        conditionEditor: rawConditionEditor,
        filterKind: rawFilterKind,
        ...rest
      } = factor;
      const filterKind = ([
        "enum",
        "interval",
        "position",
        "pattern",
        "interval_or_set",
        "set",
      ] as const).find((kind) => kind === rawFilterKind) ?? "";
      const conditionEditor: StockScreenFactor["conditionEditor"] = ([
        "singleSelect",
        "multiSelect",
        "integer",
        "integerSet",
        "range",
        "rangeOrSet",
        "indicatorCompare",
        "pattern",
      ] as const).find((editor) => editor === rawConditionEditor);
      const mapped: StockScreenFactor = {
        ...rest,
        filterKind,
        ...(conditionEditor == null ? {} : { conditionEditor }),
      };
      return mapped;
    }),
  }));
}

export function runStockScreen(
  query: StockScreenQuery,
): Promise<StockScreenResult> {
  return apiPost("/api/v1/research/screens", screenQuery(query));
}

export function fetchStockScreenPresets(): Promise<StockScreenPresetList> {
  return apiGet("/api/v1/research/screens/presets").then((response) => ({
    presets: response.presets.map(mapScreenPreset),
  }));
}

export function createStockScreenPreset(
  name: string,
  definition: StockScreenDefinitionV2,
): Promise<StockScreenPreset> {
  return apiPost("/api/v1/research/screens/presets", {
    name,
    definition: screenDefinition(definition),
  }).then(mapScreenPreset);
}

export function updateStockScreenPreset(
  presetId: string,
  name: string,
  definition: StockScreenDefinitionV2,
  expectedRevision: number,
): Promise<StockScreenPreset> {
  return apiPatchPath(
    "/api/v1/research/screens/presets/{presetId}",
    `/api/v1/research/screens/presets/${encodeURIComponent(presetId)}`,
    {
      name,
      definition: screenDefinition(definition),
      expectedRevision,
    },
  ).then(mapScreenPreset);
}

export async function deleteStockScreenPreset(presetId: string): Promise<void> {
  await apiDeletePath(
    "/api/v1/research/screens/presets/{presetId}",
    `/api/v1/research/screens/presets/${encodeURIComponent(presetId)}`,
  );
}

export function isPresetConflict(error: unknown): boolean {
  return error instanceof ApiClientError && error.status === 409;
}
