import { describe, expect, it } from "vitest";

import {
  cloneStockScreenColumn,
  cloneStockScreenDraft,
  cloneStockScreenFilter,
  cloneStockScreenSort,
  createStockScreenFilter,
  defaultParameterValue,
  factorEnumName,
  factorRefKey,
  formatStockScreenValue,
  moveItem,
  normalizeScreenMarket,
  parameterLabel,
  resultColumnFor,
  sameSort,
  sameStockScreenFactorRef,
  stockScreenCSV,
  stockScreenDraftFromDefinitionV2,
  stockScreenEntryValue,
  stockScreenFactorInstanceId,
  stockScreenFactorRefSignature,
  stockScreenQueryFingerprint,
  stockScreenValueData,
  stockScreenValueTitle,
  toStockScreenDefinitionV2,
  toStockScreenDraftFilter,
  validateStockScreenQuery,
} from "../../src/components/research/stockScreenModel";
import type {
  StockScreenCatalog,
  StockScreenDefinitionV2,
  StockScreenDraft,
  StockScreenEntry,
  StockScreenFactor,
  StockScreenFactorParameter,
  StockScreenFilter,
} from "../../src/components/research/stockScreenTypes";

const catalog: StockScreenCatalog = {
  version: "v2",
  schemaVersion: 2,
  querySchemaVersion: 2,
  provider: "futu",
  providerVersion: "1",
  market: "US",
  markets: ["HK", "US", "SH", "SZ"],
  categories: [],
  factors: [],
  enums: {
    market: [
      { key: "hk", value: 1, label: "港股" },
      { key: "us", value: 2, label: "美股" },
      { key: "cn", value: 3, label: "A 股" },
    ],
    choice: [
      { key: "unknown", value: 0, label: "未知" },
      { key: "week", value: 2, label: "周" },
    ],
    empty: [],
  },
};

function factor(overrides: Partial<StockScreenFactor> = {}): StockScreenFactor {
  return {
    key: "simple.price",
    label: "最新价",
    category: "simple",
    valueType: "number",
    filterKind: "interval",
    filter: true,
    retrieve: true,
    sort: true,
    availability: "available",
    ...overrides,
  };
}

describe("stock-screen model boundaries", () => {
  it("stabilizes factor references and market identities", () => {
    expect(factorRefKey({ factor: "fallback", factorKey: "  " })).toBe("fallback");
    expect(factorRefKey({ factor: "fallback", factorKey: " explicit " })).toBe("explicit");
    const left = { factor: "ma", params: { z: [2, { b: 2, a: 1 }], a: 1 } };
    const right = { factor: "ma", params: { a: 1, z: [2, { a: 1, b: 2 }] } };
    expect(stockScreenFactorRefSignature(left)).toBe(stockScreenFactorRefSignature(right));
    expect(sameStockScreenFactorRef(left, right)).toBe(true);
    expect(sameStockScreenFactorRef(left, { factor: "ma", params: { a: 2 } })).toBe(false);
    expect(stockScreenFactorInstanceId({ factor: "price", instanceId: " custom " }, "x")).toBe("custom");
    expect(stockScreenFactorInstanceId({ factor: "price" }, "fallback")).toBe("fallback");
    expect(stockScreenFactorInstanceId({ factor: "price" })).toBe("price");
    expect(normalizeScreenMarket(" hk ")).toBe("HK");
    expect(normalizeScreenMarket("us")).toBe("US");
    expect(normalizeScreenMarket("sz")).toBe("SZ");
    expect(normalizeScreenMarket("CN")).toBe("SH");
  });

  it("derives labels, enums, defaults, and each new-filter shape", () => {
    expect(parameterLabel({ name: "days", type: "integer" })).toBe("统计天数");
    expect(parameterLabel({ name: "custom", type: "string" })).toBe("custom");
    expect(factorEnumName(factor({ valueEnum: "choice" }))).toBe("choice");
    expect(factorEnumName(factor({ key: "field.market" }))).toBe("market");
    expect(factorEnumName(factor({ category: "kline_shape" }))).toBe("kline_shape_type");
    expect(factorEnumName(factor({ key: "financial.cash_flow_growth" }))).toBe("cash_flow_period");
    expect(factorEnumName(factor())).toBe("");

    const defaults: Array<[StockScreenFactorParameter, string | number | number[]]> = [
      [{ name: "array", type: "integer_array", default: [1, "2", "bad"] }, [1, 2]],
      [{ name: "number", type: "number", default: 3 }, 3],
      [{ name: "string", type: "string", default: "x" }, "x"],
      [{ name: "period", type: "integer", enum: "choice" }, 2],
      [{ name: "period", type: "integer", enum: "empty" }, 0],
      [{ name: "values", type: "integer_array" }, ""],
      [{ name: "days", type: "integer" }, 1],
      [{ name: "period", type: "integer" }, 11],
      [{ name: "floor", type: "number", minimum: -5 }, -5],
    ];
    for (const [parameter, expected] of defaults) {
      expect(defaultParameterValue(parameter, catalog)).toEqual(expected);
    }

    expect(createStockScreenFilter(factor({ key: "field.market", filterKind: "enum" }), 1, catalog, "HK").values).toEqual([1]);
    expect(createStockScreenFilter(factor({ key: "field.market", filterKind: "set" }), 2, catalog, "US").values).toEqual([2]);
    expect(createStockScreenFilter(factor({ key: "field.market", filterKind: "set" }), 3, catalog, "CN").values).toEqual([3]);
    expect(createStockScreenFilter(factor({ key: "unknown", filterKind: "set" }), 4, catalog, "US").values).toEqual([0]);
    expect(createStockScreenFilter(factor({ filterKind: "position" }), 5, catalog, "US")).toMatchObject({ position: 1, continuousPeriod: 1 });
    expect(createStockScreenFilter(factor({ filterKind: "pattern" }), 6, catalog, "US")).toMatchObject({ match: true, continuousPeriod: 1 });
    expect(
      createStockScreenFilter(
        factor({
          parameters: [
            { name: "optionParam", type: "integer", required: true },
            { name: "optional", type: "number" },
            { name: "days", type: "integer" },
            { name: "required", type: "number", required: true, default: 9 },
          ],
        }),
        7,
        catalog,
        "US",
        "price-7",
      ),
    ).toMatchObject({
      id: "simple.price-7",
      factor: "simple.price",
      factorKey: "simple.price",
      instanceId: "price-7",
      params: { days: 1, required: 9 },
    });
  });

  it("unwraps and formats every stock-screen value family", () => {
    expect(stockScreenValueData(undefined)).toBeNull();
    expect(stockScreenValueData({ type: "missing" })).toBeNull();
    expect(stockScreenValueData({ type: "string", string: "tech" })).toBe("tech");
    expect(stockScreenValueData({ type: "integer", integer: 3 })).toBe(3);
    expect(stockScreenValueData({ type: "integer_array", integers: [1, 2] })).toEqual([1, 2]);
    expect(stockScreenValueData({ type: "number", number: 1.5 })).toBe(1.5);
    expect(stockScreenValueData({ type: "string" })).toBeNull();
    expect(formatStockScreenValue({ type: "string", enumName: "科技" })).toBe("科技");
    expect(formatStockScreenValue({ type: "missing" })).toBe("—");
    expect(formatStockScreenValue({ type: "integer_array", integers: [1, 2] })).toBe("1, 2");
    expect(formatStockScreenValue({ type: "string", string: "OPEN" })).toBe("OPEN");
    expect(formatStockScreenValue({ type: "number", number: 12.34567 }, factor({ displayFormat: "price" }))).toBe("12.3457");
    expect(formatStockScreenValue({ type: "number", number: 1_200_000_000_000 }, factor({ displayFormat: "compact_amount" }))).toBe("1.2万亿");
    expect(formatStockScreenValue({ type: "number", number: 120_000_000 }, factor({ displayFormat: "compact_amount" }))).toBe("1.2亿");
    expect(formatStockScreenValue({ type: "number", number: 12_000 }, factor({ displayFormat: "compact_amount" }))).toBe("1.2万");
    expect(formatStockScreenValue({ type: "number", number: 999 }, factor({ displayFormat: "compact_amount" }))).toBe("999");
    expect(formatStockScreenValue({ type: "number", number: 1.234 }, factor({ unit: "percent" }))).toBe("1.23%");
    expect(formatStockScreenValue({ type: "integer", integer: 12.9 }, factor({ valueType: "integer" }))).toBe("13");
    expect(formatStockScreenValue({ type: "number", number: 1_725_667_200 }, factor({ unit: "timestamp" }))).toMatch(/2024/);
    expect(formatStockScreenValue({ type: "number", number: Number.NaN }, factor({ unit: "timestamp" }))).toBe("NaN");
    expect(formatStockScreenValue({ type: "number", number: 12 }, factor({ unit: "shares" }))).toBe("12股");
    expect(formatStockScreenValue({ type: "number", number: 7 }, factor({ unit: "days" }))).toBe("7天");
    expect(formatStockScreenValue({ type: "number", number: 3.14159 }, factor())).toBe("3.1416");
    expect(stockScreenValueTitle({ type: "missing" }, factor({ unit: "currency" }))).toBeUndefined();
    expect(stockScreenValueTitle({ type: "number", number: 1 }, factor())).toBeUndefined();
    expect(stockScreenValueTitle({ type: "number", number: 1 }, factor({ unit: "currency", currencyBasis: "reporting" }))).toContain("报表币种");
    expect(stockScreenValueTitle({ type: "number", number: 1 }, factor({ unit: "currency" }), { quoteCurrency: "USD", stockId: "1", productClass: "equity", cells: {} })).toBeUndefined();
  });

  it("deep-clones drafts, filter boundaries, intervals, arrays, and pools", () => {
    const filterValue: StockScreenFilter = {
      factor: "indicator.ma",
      params: { indicatorParams: [20], optionParamIntegers: [1, 2] },
      min: { value: 1 },
      max: { value: 2, includes: false },
      intervals: [{ min: { value: 3 }, max: { value: 4 }, unit: 1 }],
      values: [1, 2],
      secondFactor: { factor: "indicator.rsi", params: { indicatorParams: [14] } },
    };
    const cloned = cloneStockScreenFilter(filterValue);
    expect(cloned).toEqual(filterValue);
    expect(cloned).not.toBe(filterValue);
    expect(cloned.intervals).not.toBe(filterValue.intervals);
    expect(cloned.params?.indicatorParams).not.toBe(filterValue.params?.indicatorParams);
    expect(cloneStockScreenFilter({ factor: "simple.price" })).toEqual({ factor: "simple.price" });
    expect(cloneStockScreenColumn({ factor: "ma", params: { indicatorParams: [20] } }).params?.indicatorParams).toEqual([20]);
    expect(cloneStockScreenSort({ factor: "price", direction: "desc" })).toEqual({ factor: "price", direction: "desc" });
    expect(toStockScreenDraftFilter({ ...filterValue, id: "condition-1" })).toMatchObject({ conditionId: "condition-1" });

    const draft = cloneStockScreenDraft({
      brokerId: "futu",
      market: "CN",
      pool: {
        watchlistStockIds: ["1"],
        plates: [{ plateType: "industry", plateIds: ["BK1"] }],
      },
      filters: [filterValue],
      columns: [{ factor: "price" }],
      sort: [{ factor: "price", direction: "asc" }],
    });
    expect(draft).toMatchObject({ brokerId: "futu", market: "SH" });
    expect(draft.pool?.watchlistStockIds).toEqual(["1"]);
    expect(draft.pool?.plates?.[0]?.plateIds).toEqual(["BK1"]);
    expect(cloneStockScreenDraft({ market: "US" })).toEqual({ market: "US", filters: [], columns: [], sort: [] });
  });

  it("hydrates every V2 condition operator and preserves pool identities", () => {
    const definition: StockScreenDefinitionV2 = {
      brokerId: "futu",
      market: "CN",
      pool: { watchlistStockIds: ["1"], plates: [{ plateType: "concept", plateIds: ["BK1"] }] },
      catalogVersion: "v2",
      querySchemaVersion: 2,
      conditions: [
        { id: "set", factor: { instanceId: "set", factorKey: "field.market" }, operator: "in", value: [1, "2", "bad"] },
        { id: "position", factor: { instanceId: "ma", factorKey: "indicator.ma", params: { indicatorParams: [20] } }, operator: "position", value: { position: 2, secondValue: 10, continuousPeriod: 3, intervals: [{ min: 1, minIncludes: false, max: 2, unit: 4 }, null] }, secondFactor: { instanceId: "rsi", factorKey: "indicator.rsi" } },
        { id: "pattern", factor: { instanceId: "shape", factorKey: "shape" }, operator: "pattern", value: { match: false, values: [2, "3", "bad"], continuousPeriod: 4 } },
        { id: "range", factor: { instanceId: "price", factorKey: "price" }, operator: "between", value: { min: 5, minIncludes: false, max: 10, maxIncludes: true, continuousPeriod: 2, intervals: [{ min: Number.NaN, max: 20 }, "bad"] } },
        { id: "invalid", factor: { instanceId: "x", factorKey: "x" }, operator: "between", value: "bad" },
      ],
      columns: [{ columnId: "price", factor: { instanceId: "price-col", factorKey: "price", params: { days: 2 } } }],
      sorts: [
        { sortId: "primary", factor: { instanceId: "price-sort", factorKey: "price" }, direction: "desc" },
        { factor: { instanceId: "volume-sort", factorKey: "volume" }, direction: "asc" },
      ],
    };
    const draft = stockScreenDraftFromDefinitionV2(definition);
    expect(draft.market).toBe("SH");
    expect(draft.filters?.[0]?.values).toEqual([1, 2]);
    expect(draft.filters?.[1]).toMatchObject({ position: 2, secondValue: 10, continuousPeriod: 3 });
    expect(draft.filters?.[2]).toMatchObject({ match: false, values: [2, 3] });
    expect(draft.filters?.[3]).toMatchObject({ min: { value: 5, includes: false }, max: { value: 10, includes: true } });
    expect(draft.filters?.[4]).not.toHaveProperty("min");
    expect(draft.pool?.plates?.[0]?.plateIds).toEqual(["BK1"]);
    expect(draft.sort?.[0]?.sortId).toBe("primary");
  });

  it("exports safe CSV cells and resolves only response column identities", () => {
    const entry: StockScreenEntry = {
      stockId: "1",
      market: "US",
      symbol: "AAPL",
      name: "Apple\n\"Class A\"",
      productClass: "equity",
      cells: {
        tags: { columnId: "tags", factorKey: "tags", value: { type: "integer_array", integers: [1, 2] } },
        missing: { columnId: "missing", factorKey: "missing", value: { type: "missing" } },
      },
    };
    const columns = [{ factor: "tags", columnId: "tags" }, { factor: "unknown", columnId: "missing" }];
    const csv = stockScreenCSV(entry ? [entry] : [], new Map([["tags", factor({ key: "tags", label: "标签" })]]), columns);
    expect(csv).toContain('"Apple\n""Class A"""');
    expect(csv).toContain("1|2");
    expect(csv).toContain("标签,unknown");
    expect(stockScreenEntryValue(entry, { factor: "tags", columnId: " " })).toBeUndefined();
    expect(resultColumnFor(entry, { factor: "tags", instanceId: "tag-instance", columnId: "wrong" }, [{ columnId: "tags", instanceId: "tag-instance", factorKey: "tags" }])).toMatchObject({ integers: [1, 2] });
    expect(resultColumnFor(entry, { factor: "tags", columnId: "tags" })).toMatchObject({ integers: [1, 2] });
  });

  it("reports the full validation error matrix", () => {
    const factors: StockScreenFactor[] = [
      factor({ key: "enum", filterKind: "enum" }),
      factor({ key: "set", filterKind: "set" }),
      factor({ key: "interval", filterKind: "interval" }),
      factor({ key: "hybrid", filterKind: "interval_or_set" }),
      factor({ key: "position", category: "indicator", filterKind: "position" }),
      factor({ key: "basic", category: "basic" }),
      factor({ key: "unsupported", availability: "unsupported", reason: "行情权限不足" }),
      factor({ key: "restricted", filter: false, retrieve: false, sort: false }),
      factor({
        key: "params",
        parameters: [
          { name: "days", type: "integer", required: true, minimum: 1, maximum: 5 },
          { name: "indicatorParams", type: "integer_array", required: true },
        ],
      }),
    ];
    const query: StockScreenDraft = {
      market: "bad",
      filters: [
        { factor: "missing" },
        { factor: "enum", values: [] },
        { factor: "set" },
        { factor: "interval", min: { value: Number.NaN }, max: { value: 1 } },
        { factor: "interval", min: { value: 10 }, max: { value: 1 } },
        { factor: "hybrid", values: [] },
        { factor: "hybrid" },
        { factor: "position", position: 5, secondFactor: { factor: "basic", factorKey: "basic" } },
        { factor: "position", position: 0, secondValue: Number.NaN },
        { factor: "unsupported" },
        { factor: "restricted" },
        { factor: "params", params: { days: "bad", indicatorParams: [] } },
        { factor: "params", params: { days: 0, indicatorParams: [1] } },
        { factor: "params", params: { days: 6, indicatorParams: [1] } },
        { factor: "position", position: 1, secondValue: 1 },
        { factor: "position", position: 1, secondValue: 1 },
      ],
      columns: [
        { factor: "restricted" },
        { factor: "basic" },
        { factor: "basic" },
      ],
      sort: [{ factor: "restricted" }, { factor: "basic", direction: "asc" }],
    };
    const errors = validateStockScreenQuery(query, { ...catalog, factors });
    const messages = errors.map((error) => error.message);
    for (const expected of [
      "请选择有效市场",
      "因子不在当前市场目录中",
      "请选择至少一个条件值",
      "请至少填写一个边界",
      "下限必须是数字",
      "上限不能小于下限",
      "请选择有效的位置关系",
      "比较因子必须是技术指标",
      "行情权限不足",
      "该因子不能用于此位置",
      "统计天数必须是数字",
      "指标参数为必填项",
      "统计天数不能小于 1",
      "统计天数不能大于 5",
      "请选择排序方向",
    ]) expect(messages).toContain(expected);
    expect(messages.some((message) => message.includes("完全重复"))).toBe(true);
    expect(validateStockScreenQuery({ market: "US", filters: [{ factor: "unknown" }] })).toEqual([]);
  });

  it("round-trips conditions and creates stable query fingerprints", () => {
    const query: StockScreenDraft = {
      brokerId: "futu",
      market: "US",
      pool: { watchlistStockIds: ["1"] },
      filters: [
        { conditionId: "range", factor: "price", min: { value: 1, includes: false }, max: { value: 2 }, intervals: [{ min: { value: 3 }, max: { value: 4, includes: false }, unit: 1 }], continuousPeriod: 2 },
        { factor: "ma", instanceId: "ma-20", factorKey: "ma", position: 1, secondValue: 10, continuousPeriod: 3, secondFactor: { factor: "rsi", factorKey: "rsi", params: { indicatorParams: [14] } } },
        { factor: "shape", match: false, values: [2], continuousPeriod: 4 },
        { factor: "market", values: [1, 2] },
      ],
      columns: [{ factor: "price", params: { days: 1 } }, { factor: "volume", columnId: "volume-col" }],
      sort: [{ factor: "price", direction: "desc", sortId: "primary" }, { factor: "volume", direction: "asc" }],
    };
    const definition = toStockScreenDefinitionV2(query, "v2");
    expect(definition.conditions.map((item) => item.operator)).toEqual(["between", "position", "pattern", "in"]);
    expect(definition.conditions[0]?.value).toMatchObject({ min: 1, minIncludes: false, max: 2, continuousPeriod: 2 });
    expect(definition.conditions[1]?.secondFactor).toMatchObject({ instanceId: "second-2" });
    expect(definition.columns[0]?.columnId).toBe("column-1");
    expect(definition.sorts[0]?.sortId).toBe("primary");
    const reordered = { ...query, pool: { watchlistStockIds: ["1"] } };
    expect(stockScreenQueryFingerprint(query)).toBe(stockScreenQueryFingerprint(reordered));
    expect(stockScreenQueryFingerprint({ ...query, market: "HK" })).not.toBe(stockScreenQueryFingerprint(query));
    expect(moveItem([1, 2], 0, -1)).toEqual([1, 2]);
    expect(moveItem([1, 2], 1, 1)).toEqual([1, 2]);
    expect(sameSort([{ factor: "price", direction: "asc" }], [{ factor: "price", direction: "asc" }])).toBe(true);
    expect(sameSort([], [{ factor: "price", direction: "asc" }])).toBe(false);
  });
});
