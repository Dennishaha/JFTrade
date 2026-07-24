import { describe, expect, it } from "vitest";

import {
  cloneStockScreenDraft,
  createStockScreenFilter,
  formatStockScreenValue,
  moveItem,
  normalizeScreenMarket,
  resultColumnFor,
  stockScreenEntryValue,
  stockScreenDraftFromDefinitionV2,
  stockScreenFactorRefSignature,
  stockScreenCSV,
  stockScreenValueTitle,
  toStockScreenDefinitionV2,
  validateStockScreenQuery,
} from "../../src/components/research/stockScreenModel";
import type {
  StockScreenCatalog,
  StockScreenEntry,
  StockScreenFactor,
} from "../../src/components/research/stockScreenTypes";

const catalog: StockScreenCatalog = {
  version: "futu-stock-screen-v1",
  schemaVersion: 2,
  querySchemaVersion: 2,
  provider: "futu",
  providerVersion: "10.9.6908",
  market: "US",
  markets: ["HK", "US", "SH", "SZ"],
  categories: [{ key: "indicator", label: "技术指标", count: 1 }],
  factors: [],
  enums: {
    period: [
      { key: "unknown", value: 0, label: "Unknown" },
      { key: "day", value: 11, label: "日线" },
    ],
  },
  rateLimit: { requests: 10, windowSeconds: 30 },
};

describe("stockScreenModel", () => {
  it("builds the backend factor/min/max/params contract from catalog metadata", () => {
    const factor: StockScreenFactor = {
      key: "indicator.rsi",
      label: "动态 RSI",
      category: "indicator",
      valueType: "number",
      filterKind: "position",
      filter: true,
      retrieve: true,
      sort: true,
      availability: "available",
      parameters: [{ name: "period", type: "integer", enum: "period" }],
    };
    expect(createStockScreenFilter(factor, 2, catalog, "US")).toEqual({
      id: "indicator.rsi-2",
      factor: "indicator.rsi",
      params: { period: 11 },
      position: 1,
      continuousPeriod: 1,
    });
  });

  it("exports typed result values in the selected factor order", () => {
    const factors = new Map<string, StockScreenFactor>([
      [
        "simple.price",
        {
          key: "simple.price",
          label: "最新价",
          category: "simple",
          valueType: "number",
          filterKind: "interval",
          filter: true,
          retrieve: true,
          sort: true,
          availability: "available",
        },
      ],
      [
        "basic.industry",
        {
          key: "basic.industry",
          label: "行业",
          category: "basic",
          valueType: "string",
          filter: false,
          retrieve: true,
          sort: true,
          availability: "available",
        },
      ],
    ]);
    const csv = stockScreenCSV(
      [
        {
          stockId: "1",
          instrumentId: "US.AAPL",
          market: "US",
          symbol: "AAPL",
          name: "Apple, Inc.",
          productClass: "equity",
          cells: {
            industry: {
              columnId: "industry",
              instanceId: "industry",
              factorKey: "basic.industry",
              value: { type: "string", string: "科技" },
            },
            price: {
              columnId: "price",
              instanceId: "price",
              factorKey: "simple.price",
              value: { type: "number", number: 200, unit: "美元" },
            },
          },
        },
      ],
      factors,
      [
        { factor: "basic.industry", columnId: "industry" },
        { factor: "simple.price", columnId: "price" },
      ],
    );
    expect(csv).toBe(
      '\uFEFF市场,代码,名称,行业,最新价\r\nUS,AAPL,"Apple, Inc.",科技,200',
    );
  });

  it("maps the global CN market to a concrete backend stock-screen market", () => {
    expect(normalizeScreenMarket("CN")).toBe("SH");
    expect(normalizeScreenMarket("SZ")).toBe("SZ");
  });

  it("moves columns without mutating the source", () => {
    const source = ["price", "market_cap", "rsi"];
    expect(moveItem(source, 2, -1)).toEqual(["price", "rsi", "market_cap"]);
    expect(source).toEqual(["price", "market_cap", "rsi"]);
  });

  it("keeps parameterized factor instances distinct while rejecting exact duplicates", () => {
    const price: StockScreenFactor = {
      key: "indicator.ma",
      label: "均线",
      category: "indicator",
      valueType: "number",
      filterKind: "interval",
      filter: true,
      retrieve: true,
      sort: true,
      availability: "available",
      parameters: [{ name: "period", type: "integer", required: true, minimum: 1, maximum: 240 }],
    };
    const ma20 = createStockScreenFilter(price, 1, catalog, "US", "ma-20");
    const ma60 = createStockScreenFilter(price, 2, catalog, "US", "ma-60");
    ma20.params = { period: 20 };
    ma60.params = { period: 60 };
    ma20.min = { value: 1 };
    ma60.min = { value: 1 };
    expect(stockScreenFactorRefSignature(ma20)).not.toBe(stockScreenFactorRefSignature(ma60));
    const query = cloneStockScreenDraft({ market: "US", filters: [ma20, ma60] });
    expect(validateStockScreenQuery(query, { ...catalog, factors: [price] })).toEqual([]);
    query.filters?.push({ ...ma20 });
    expect(validateStockScreenQuery(query, { ...catalog, factors: [price] }).some((error) => error.message.includes("重复"))).toBe(true);
  });

  it("serializes V2 interval and indicator comparisons without losing condition semantics", () => {
    const definition = toStockScreenDefinitionV2(
      {
        market: "US",
        filters: [
          {
            id: "price-range",
            factor: "simple.price",
            instanceId: "price",
            factorKey: "simple.price",
            min: { value: 10, includes: false },
            max: { value: 100, includes: true },
          },
          {
            id: "ma-over-rsi",
            factor: "indicator.ma",
            instanceId: "ma-20",
            factorKey: "indicator.ma",
            params: { period: 11, indicatorParams: [20] },
            position: 1,
            continuousPeriod: 3,
            secondFactor: {
              factor: "indicator.rsi",
              instanceId: "rsi-14",
              factorKey: "indicator.rsi",
              params: { period: 11, indicatorParams: [14] },
            },
          },
        ],
      },
      "futu-stock-screen-v1",
    );

    expect(definition.conditions[0]).toMatchObject({
      operator: "between",
      value: {
        min: 10,
        minIncludes: false,
        max: 100,
        maxIncludes: true,
      },
    });
    expect(definition.conditions[1]).toMatchObject({
      operator: "position",
      value: { position: 1, continuousPeriod: 3 },
      secondFactor: {
        instanceId: "rsi-14",
        factorKey: "indicator.rsi",
      },
    });
  });

  it("rejects incomplete interval and positional conditions before execution", () => {
    const price: StockScreenFactor = {
      key: "simple.price",
      label: "最新价",
      category: "simple",
      valueType: "number",
      filterKind: "interval",
      filter: true,
      retrieve: true,
      sort: true,
      availability: "available",
    };
    const ma: StockScreenFactor = {
      key: "indicator.ma",
      label: "均线",
      category: "indicator",
      valueType: "number",
      filterKind: "position",
      filter: true,
      retrieve: true,
      sort: true,
      availability: "available",
    };
    const errors = validateStockScreenQuery(
      {
        market: "US",
        filters: [
          { factor: "simple.price" },
          { factor: "indicator.ma", position: 1 },
        ],
      },
      { ...catalog, factors: [price, ma] },
    );
    expect(errors).toEqual(
      expect.arrayContaining([
        { path: "conditions.0.min", message: "请至少填写一个边界" },
        {
          path: "conditions.1.secondValue",
          message: "请填写比较值或选择比较指标",
        },
      ]),
    );
  });

  it("resolves result cells only by column identity", () => {
    const entry = {
      stockId: "1",
      productClass: "equity",
      cells: {
        "ma-60-column": {
          columnId: "ma-60-column",
          instanceId: "ma-60",
          factorKey: "indicator.ma",
          value: { type: "number", number: 60 } as const,
        },
      },
    };
    expect(
      stockScreenEntryValue(entry, {
        factor: "indicator.ma",
        instanceId: "ma-60",
        columnId: "ma-60-column",
      })?.number,
    ).toBe(60);
    expect(
      stockScreenEntryValue(entry, {
        factor: "indicator.ma",
        instanceId: "ma-60",
        columnId: "ma-20-column",
      }),
    ).toBeUndefined();
  });

  it("hydrates an editor draft directly from a V2 definition", () => {
    const draft = stockScreenDraftFromDefinitionV2({
      brokerId: "futu",
      market: "US",
      catalogVersion: "futu-stock-screen-v1",
      querySchemaVersion: 2,
      conditions: [{
        id: "price-range",
        factor: { instanceId: "price-filter", factorKey: "simple.price" },
        operator: "between",
        value: { min: 10, minIncludes: false, max: 100, maxIncludes: true },
      }],
      columns: [{
        columnId: "price-column",
        factor: { instanceId: "price-column", factorKey: "simple.price" },
      }],
      sorts: [],
    });

    expect(draft.filters?.[0]).toMatchObject({
      conditionId: "price-range",
      factor: "simple.price",
      factorKey: "simple.price",
      instanceId: "price-filter",
      min: { value: 10, includes: false },
      max: { value: 100, includes: true },
    });
    expect(
      toStockScreenDefinitionV2(draft, "futu-stock-screen-v1").conditions[0],
    ).toMatchObject({
      id: "price-range",
      operator: "between",
      value: { min: 10, minIncludes: false, max: 100, maxIncludes: true },
    });
  });

  it("keeps same-factor result columns separate by their column identity", () => {
    const entry: StockScreenEntry = {
      stockId: "1",
      productClass: "equity",
      cells: {
        "ma-20-column": {
          columnId: "ma-20-column",
          instanceId: "ma-20",
          factorKey: "indicator.ma",
          value: { type: "number", number: 20 },
        },
        "ma-60-column": {
          columnId: "ma-60-column",
          instanceId: "ma-60",
          factorKey: "indicator.ma",
          value: { type: "number", number: 60 },
        },
      },
    };
    const resultColumns = [
      { columnId: "ma-20-column", instanceId: "ma-20", factorKey: "indicator.ma" },
      { columnId: "ma-60-column", instanceId: "ma-60", factorKey: "indicator.ma" },
    ];

    expect(
      resultColumnFor(
        entry,
        { factor: "indicator.ma", columnId: "ma-20-column", instanceId: "ma-20" },
        resultColumns,
      )?.number,
    ).toBe(20);
    expect(
      resultColumnFor(
        entry,
        { factor: "indicator.ma", columnId: "ma-60-column", instanceId: "ma-60" },
        resultColumns,
      )?.number,
    ).toBe(60);
  });

  it("formats quote-currency values per security counter", () => {
    const priceFactor: StockScreenFactor = {
      key: "simple.price",
      label: "最新价",
      category: "simple",
      valueType: "number",
      unit: "currency",
      currencyBasis: "quote",
      displayFormat: "price",
      filter: true,
      retrieve: true,
      sort: true,
      availability: "available",
    };
    const marketCapFactor: StockScreenFactor = {
      ...priceFactor,
      key: "simple.market_cap",
      label: "市值",
      displayFormat: "compact_amount",
    };
    expect(
      formatStockScreenValue(
        { type: "number", number: 52.7, unit: "currency" },
        priceFactor,
        {
          stockId: "80700",
          market: "HK",
          symbol: "80700",
          name: "腾讯控股-R",
          quoteCurrency: "CNY",
          productClass: "equity",
          cells: {},
        },
      ),
    ).toBe("CNY 52.70");
    expect(
      formatStockScreenValue(
        { type: "number", number: 19_425_957_800, unit: "currency" },
        marketCapFactor,
        {
          stockId: "700",
          market: "HK",
          symbol: "00700",
          name: "腾讯控股",
          quoteCurrency: "HKD",
          productClass: "equity",
          cells: {},
        },
      ),
    ).toBe("HKD 194.26亿");
    expect(
      formatStockScreenValue(
        { type: "number", number: -1_234_560_000, unit: "currency" },
        marketCapFactor,
        {
          stockId: "1",
          quoteCurrency: "USD",
          productClass: "equity",
          cells: {},
        },
      ),
    ).toBe("USD -12.35亿");
  });

  it("does not invent reporting or unresolved quote currencies", () => {
    const reportingFactor: StockScreenFactor = {
      key: "financial.net_profit",
      label: "净利润",
      category: "financial",
      valueType: "number",
      unit: "currency",
      currencyBasis: "reporting",
      displayFormat: "compact_amount",
      filter: true,
      retrieve: true,
      sort: true,
      availability: "available",
    };
    const value = { type: "number", number: 120_000_000 } as const;
    expect(formatStockScreenValue(value, reportingFactor)).toBe("1.2亿");
    expect(stockScreenValueTitle(value, reportingFactor)).toBe(
      "OpenD 未提供报表币种",
    );
    expect(
      stockScreenValueTitle(value, {
        ...reportingFactor,
        key: "simple.market_cap",
        category: "simple",
        currencyBasis: "quote",
      }),
    ).toBe("无法可靠确定报价币种");
    expect(formatStockScreenValue(value, undefined)).toBe("120,000,000");
  });
});
