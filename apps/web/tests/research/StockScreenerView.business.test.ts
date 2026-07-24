// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import { flushPromises } from "../productTestUtils";

const mocks = vi.hoisted(() => ({
  catalog: vi.fn(),
  presets: vi.fn(),
  run: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  conflict: vi.fn(),
}));

vi.mock("../../src/components/research/stockScreenApi", () => ({
  fetchStockScreenCatalog: mocks.catalog,
  fetchStockScreenPresets: mocks.presets,
  runStockScreen: mocks.run,
  createStockScreenPreset: mocks.create,
  updateStockScreenPreset: mocks.update,
  deleteStockScreenPreset: mocks.remove,
  isPresetConflict: mocks.conflict,
}));

import StockScreenerView from "../../src/components/research/StockScreenerView.vue";

const catalog = {
  version: "futu-stock-screen-v1",
  schemaVersion: 2,
  querySchemaVersion: 2,
  provider: "futu",
  providerVersion: "10.9.6908",
  market: "US",
  markets: ["HK", "US", "SH", "SZ"],
  categories: [
    { key: "simple", label: "基础行情", count: 2 },
    { key: "indicator", label: "技术指标", count: 1 },
    { key: "broker", label: "经纪商持仓", count: 1 },
  ],
  factors: [
    {
      key: "simple.price",
      label: "最新价格",
      category: "simple",
      valueType: "number",
      unit: "currency",
      currencyBasis: "quote",
      displayFormat: "price",
      filterKind: "interval",
      filter: true,
      retrieve: true,
      sort: true,
      availability: "available",
    },
    {
      key: "simple.market_cap",
      label: "总市值",
      category: "simple",
      valueType: "number",
      unit: "currency",
      currencyBasis: "quote",
      displayFormat: "compact_amount",
      filterKind: "interval",
      filter: true,
      retrieve: true,
      sort: true,
      availability: "available",
    },
    {
      key: "indicator.rsi",
      label: "动态 RSI",
      category: "indicator",
      valueType: "number",
      filterKind: "position",
      filter: true,
      retrieve: true,
      sort: true,
      availability: "available",
      parameters: [
        {
          name: "period",
          type: "integer",
          enum: "period",
        },
      ],
    },
    {
      key: "broker.holding",
      label: "经纪商持仓",
      category: "broker",
      valueType: "number",
      filterKind: "interval",
      filter: true,
      retrieve: true,
      sort: false,
      availability: "unsupported",
      reason: "当前市场不支持",
    },
  ],
  enums: {
    period: [
      { key: "unknown", value: 0, label: "Unknown" },
      { key: "day", value: 11, label: "日线" },
      { key: "week", value: 21, label: "周线" },
    ],
    position: [
      { key: "over", value: 1, label: "位于上方" },
      { key: "below", value: 2, label: "位于下方" },
    ],
  },
  rateLimit: { requests: 10, windowSeconds: 30 },
} as const;

const savedPreset = {
  presetId: "preset-1",
  name: "低估值",
  querySchemaVersion: 2,
  revision: 3,
  definition: {
    brokerId: "futu",
    market: "US",
    catalogVersion: "futu-stock-screen-v1",
    querySchemaVersion: 2,
    conditions: [
      {
        id: "price-range",
        factor: {
          instanceId: "price-filter",
          factorKey: "simple.price",
        },
        operator: "between",
        value: {
          min: 10,
          minIncludes: true,
          max: 100,
          maxIncludes: true,
        },
      },
    ],
    columns: [{
      columnId: "price-column",
      factor: {
        instanceId: "price-column",
        factorKey: "simple.price",
      },
    }],
    sorts: [{
      sortId: "price-sort",
      factor: {
        instanceId: "price-sort",
        factorKey: "simple.price",
      },
      direction: "desc",
    }],
  },
  createdAt: "2026-07-23T09:00:00Z",
  updatedAt: "2026-07-23T09:00:00Z",
} as const;

function installDefaults(): void {
  mocks.catalog.mockResolvedValue(catalog);
  mocks.presets.mockResolvedValue({ presets: [savedPreset] });
  mocks.run.mockResolvedValue({
    provider: { asOf: "" },
    asOf: "",
    entries: [],
    hasMore: false,
    total: 0,
  });
  mocks.create.mockResolvedValue(savedPreset);
  mocks.update.mockResolvedValue(savedPreset);
  mocks.remove.mockResolvedValue(undefined);
  mocks.conflict.mockReturnValue(false);
}

afterEach(() => {
  vi.restoreAllMocks();
  for (const mock of Object.values(mocks)) mock.mockReset();
});

describe("StockScreenerView", () => {
  it("renders independent outer and inner resizable pane groups", async () => {
    installDefaults();
    const wrapper = mount(StockScreenerView, {
      props: { market: "US", brokerId: "futu" },
    });
    await flushPromises();

    expect(wrapper.findAll(".splitpanes")).toHaveLength(2);
    expect(wrapper.findAll(".splitpanes__pane")).toHaveLength(4);
    expect(wrapper.findAll(".splitpanes__splitter")).toHaveLength(2);
  });

  it("normalizes the global CN market before catalog and screen requests", async () => {
    installDefaults();
    const wrapper = mount(StockScreenerView, {
      props: { market: "CN", brokerId: "futu" },
    });
    await flushPromises();

    expect(mocks.catalog).toHaveBeenCalledWith("SH", "futu");
    await wrapper.get(".stock-screener-view__run").trigger("click");
    await flushPromises();
    expect(mocks.run.mock.calls[0]?.[0]).toMatchObject({ market: "SH" });
  });

  it("restores a server preset without executing it and exposes the preset id", async () => {
    installDefaults();
    const wrapper = mount(StockScreenerView, {
      props: {
        market: "US",
        brokerId: "futu",
        initialPresetId: "preset-1",
      },
    });
    await flushPromises();

    expect(mocks.catalog).toHaveBeenCalledWith("US", "futu");
    expect(mocks.presets).toHaveBeenCalledOnce();
    expect(mocks.run).not.toHaveBeenCalled();
    expect(wrapper.get('[aria-label="预设名称"]').element).toHaveProperty(
      "value",
      "低估值",
    );
    expect(wrapper.text()).toContain("已恢复预设，请手动执行筛选");
    expect(wrapper.emitted("presetChange")?.at(-1)).toEqual(["preset-1"]);
  });

  it("submits a typed query, appends the next page, and emits preview/open", async () => {
    installDefaults();
    mocks.run
      .mockResolvedValueOnce({
        provider: { asOf: "2026-07-23T09:30:00Z" },
        asOf: "2026-07-23T09:30:00Z",
        entries: [
          {
            stockId: "1",
            instrumentId: "US.AAPL",
            market: "US",
            symbol: "AAPL",
            name: "Apple",
            quoteCurrency: "USD",
            productClass: "equity",
            cells: {
              "column-simple.price-0": {
                columnId: "column-simple.price-0",
                instanceId: "default-simple.price",
                factorKey: "simple.price",
                value: {
                  type: "number",
                  number: 200,
                  unit: "美元",
                },
              },
            },
          },
        ],
        nextOffset: 50,
        hasMore: true,
        total: 2,
      })
      .mockResolvedValueOnce({
        provider: { asOf: "2026-07-23T09:30:03Z" },
        asOf: "2026-07-23T09:30:03Z",
        entries: [
          {
            stockId: "2",
            instrumentId: "US.MSFT",
            market: "US",
            symbol: "MSFT",
            name: "Microsoft",
            quoteCurrency: "USD",
            productClass: "equity",
            cells: {
              "column-simple.price-0": {
                columnId: "column-simple.price-0",
                instanceId: "default-simple.price",
                factorKey: "simple.price",
                value: { type: "number", number: 400 },
              },
            },
          },
        ],
        hasMore: false,
        total: 2,
      });
    const wrapper = mount(StockScreenerView, {
      props: { market: "US", brokerId: "futu" },
    });
    await flushPromises();

    expect(wrapper.get(".stock-screener-view__builder").classes()).not.toContain(
      "is-mobile-hidden",
    );
    expect(wrapper.get(".stock-screener-view__results").classes()).toContain(
      "is-mobile-hidden",
    );
    await wrapper
      .findAll(".stock-screener-view__common button")
      .find((button) => button.text().includes("最新价"))!
      .trigger("click");
    await wrapper.get('[aria-label="条件下限"]').setValue("100");
    await wrapper.get('[aria-label="条件上限"]').setValue("250");
    await wrapper.get(".stock-screener-view__run").trigger("click");
    await flushPromises();

    expect(mocks.run).toHaveBeenCalledWith({
      brokerId: "futu",
      market: "US",
      catalogVersion: "futu-stock-screen-v1",
      querySchemaVersion: 2,
      conditions: [
        expect.objectContaining({
          factor: expect.objectContaining({ factorKey: "simple.price" }),
          operator: "between",
          value: {
            min: 100,
            minIncludes: true,
            max: 250,
            maxIncludes: true,
          },
        }),
      ],
      columns: [
        expect.objectContaining({
          factor: expect.objectContaining({ factorKey: "simple.price" }),
        }),
        expect.objectContaining({
          factor: expect.objectContaining({ factorKey: "simple.market_cap" }),
        }),
      ],
      sorts: [],
      page: { offset: 0, limit: 50 },
    });
    const row = wrapper.get("tbody tr");
    expect(row.text()).toContain("USD 200.00");
    await row.trigger("click");
    await row.trigger("dblclick");
    expect(wrapper.emitted("select")?.[0]?.[0]).toMatchObject({
      instrumentId: "US.AAPL",
    });
    expect(wrapper.emitted("open")?.[0]?.[0]).toMatchObject({
      instrumentId: "US.AAPL",
    });

    await wrapper.get(".stock-screener-view__more").trigger("click");
    await flushPromises();
    expect(mocks.run.mock.calls[1]?.[0]).toMatchObject({
      page: { offset: 50, limit: 50 },
    });
    expect(wrapper.findAll("tbody tr")).toHaveLength(2);
  });

  it("searches the complete catalog, keeps restricted factors visible, and edits parameters", async () => {
    installDefaults();
    const wrapper = mount(StockScreenerView, {
      props: { market: "US", brokerId: "futu" },
      global: { stubs: { teleport: true } },
    });
    await flushPromises();

    const addFactor = wrapper.get(".stock-screener-view__add-factor");
    expect(addFactor.attributes("aria-haspopup")).toBe("dialog");
    await addFactor.trigger("click");
    expect(wrapper.get('[role="dialog"]').attributes("aria-modal")).toBe("true");
    const search = wrapper.get('.stock-screener-view__catalog input[type="search"]');
    await search.setValue("经纪商");
    expect(wrapper.text()).toContain("经纪商持仓");
    expect(wrapper.text()).toContain("当前市场不支持");
    const restricted = wrapper
      .findAll(".stock-screener-view__factor-list article")
      .find((article) => article.text().includes("经纪商持仓"))!;
    expect(restricted.classes()).toContain("is-disabled");
    expect(
      restricted.findAll("button").every((button) => button.attributes("disabled") != null),
    ).toBe(true);

    await search.setValue("RSI");
    const rsi = wrapper
      .findAll(".stock-screener-view__factor-list article")
      .find((article) => article.text().includes("RSI"))!;
    await rsi.findAll("button")[0]!.trigger("click");
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false);
    expect(addFactor.attributes("aria-expanded")).toBe("false");
    expect(wrapper.text()).toContain("K 线周期");
    const period = wrapper.get(".stock-screener-view__parameters select");
    expect(period.element).toHaveProperty("value", "11");
    await period.setValue("21");
    await wrapper.get('[aria-label="比较值"]').setValue("50");
    await wrapper.get(".stock-screener-view__run").trigger("click");
    await flushPromises();
    expect(mocks.run.mock.calls.at(-1)?.[0]).toMatchObject({
      conditions: [
        expect.objectContaining({
          factor: expect.objectContaining({
            factorKey: "indicator.rsi",
            params: { period: 21 },
          }),
          operator: "position",
          value: expect.objectContaining({ position: 1, secondValue: 50 }),
        }),
      ],
    });
  });

  it("shows arrow controls and scrolls overflowing factor categories", async () => {
    installDefaults();
    const wrapper = mount(StockScreenerView, {
      props: { market: "US", brokerId: "futu" },
      global: { stubs: { teleport: true } },
    });
    await flushPromises();
    await wrapper.get(".stock-screener-view__add-factor").trigger("click");

    const categories = wrapper.get(".stock-screener-view__categories");
    const scroller = categories.element as HTMLDivElement;
    Object.defineProperties(scroller, {
      clientWidth: { configurable: true, value: 200 },
      scrollWidth: { configurable: true, value: 600 },
    });
    const scrollBy = vi.fn();
    const scrollerPrototype = Object.getPrototypeOf(scroller) as HTMLElement;
    Object.defineProperty(scrollerPrototype, "scrollBy", {
      configurable: true,
      value: scrollBy,
    });
    await categories.trigger("scroll");

    const previous = wrapper.get(
      ".stock-screener-view__category-scroll--previous",
    );
    const next = wrapper.get(".stock-screener-view__category-scroll--next");
    expect(previous.attributes("disabled")).toBeDefined();
    expect(next.attributes("disabled")).toBeUndefined();

    await next.trigger("click");
    expect(scrollBy).toHaveBeenCalledOnce();
    expect(scrollBy.mock.calls[0]?.[0]).toMatchObject({ behavior: "smooth" });
    expect(scrollBy.mock.calls[0]?.[0].left).toBeGreaterThan(0);
    delete (scrollerPrototype as Partial<HTMLElement>).scrollBy;
  });

  it("asks before overwriting a same-name preset after a 409 conflict", async () => {
    installDefaults();
    mocks.create.mockRejectedValue(new Error("duplicate"));
    mocks.conflict.mockReturnValue(true);
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    const wrapper = mount(StockScreenerView, {
      props: { market: "US", brokerId: "futu" },
    });
    await flushPromises();

    await wrapper.get('[aria-label="预设名称"]').setValue("低估值");
    await wrapper
      .findAll(".stock-screener-view__toolbar button")
      .find((button) => button.text() === "保存")!
      .trigger("click");
    await flushPromises();

    expect(confirm).toHaveBeenCalledWith("预设“低估值”已存在，是否覆盖？");
    expect(mocks.update).toHaveBeenCalledWith(
      "preset-1",
      "低估值",
      expect.objectContaining({ market: "US" }),
      3,
    );
  });

  it("supports independent result columns, multi-field sorting, and CSV for loaded rows", async () => {
    installDefaults();
    const createObjectURL = vi
      .spyOn(URL, "createObjectURL")
      .mockReturnValue("blob:test");
    vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    mocks.run.mockResolvedValue({
      provider: { asOf: "2026-07-23T09:30:00Z" },
      asOf: "2026-07-23T09:30:00Z",
      entries: [
        {
          stockId: "1",
          instrumentId: "US.AAPL",
          market: "US",
          symbol: "AAPL",
          name: "Apple",
          productClass: "equity",
          cells: {
            "column-simple.price-0": {
              columnId: "column-simple.price-0",
              instanceId: "default-simple.price",
              factorKey: "simple.price",
              value: { type: "number", number: 200 },
            },
            "column-simple.market_cap-1": {
              columnId: "column-simple.market_cap-1",
              instanceId: "default-simple.market_cap",
              factorKey: "simple.market_cap",
              value: { type: "number", number: 3_000_000 },
            },
          },
        },
      ],
      total: 1,
      hasMore: false,
    });
    const wrapper = mount(StockScreenerView, {
      props: { market: "US", brokerId: "futu" },
    });
    await flushPromises();

    await wrapper
      .findAll(".stock-screener-view__panel-head button")
      .find((button) => button.text() === "添加排序")!
      .trigger("click");
    await wrapper.get(".stock-screener-view__run").trigger("click");
    await flushPromises();
    expect(mocks.run.mock.calls[0]?.[0]).toMatchObject({
      columns: [
        { factor: expect.objectContaining({ factorKey: "simple.price" }) },
        { factor: expect.objectContaining({ factorKey: "simple.market_cap" }) },
      ],
      sorts: [{
        factor: expect.objectContaining({ factorKey: "simple.price" }),
        direction: "desc",
      }],
    });

    await wrapper
      .findAll(".stock-screener-view__toolbar button")
      .find((button) => button.text() === "导出 CSV")!
      .trigger("click");
    expect(createObjectURL).toHaveBeenCalledWith(expect.any(Blob));
  });

  it("renders mixed HKD and CNY counters with currency on every row", async () => {
    installDefaults();
    mocks.run.mockResolvedValue({
      provider: { asOf: "2026-07-23T09:30:00Z" },
      asOf: "2026-07-23T09:30:00Z",
      entries: [
        {
          stockId: "700",
          instrumentId: "HK.00700",
          market: "HK",
          symbol: "00700",
          name: "腾讯控股",
          quoteCurrency: "HKD",
          productClass: "equity",
          cells: {
            "column-simple.price-0": {
              columnId: "column-simple.price-0",
              instanceId: "default-simple.price",
              factorKey: "simple.price",
              value: {
                type: "number",
                number: 52.7,
                unit: "currency",
              },
            },
            "column-simple.market_cap-1": {
              columnId: "column-simple.market_cap-1",
              instanceId: "default-simple.market_cap",
              factorKey: "simple.market_cap",
              value: {
                type: "number",
                number: 19_425_957_800,
                unit: "currency",
              },
            },
          },
        },
        {
          stockId: "80700",
          instrumentId: "HK.80700",
          market: "HK",
          symbol: "80700",
          name: "腾讯控股-R",
          quoteCurrency: "CNY",
          productClass: "equity",
          cells: {
            "column-simple.price-0": {
              columnId: "column-simple.price-0",
              instanceId: "default-simple.price",
              factorKey: "simple.price",
              value: {
                type: "number",
                number: 52.7,
                unit: "currency",
              },
            },
            "column-simple.market_cap-1": {
              columnId: "column-simple.market_cap-1",
              instanceId: "default-simple.market_cap",
              factorKey: "simple.market_cap",
              value: {
                type: "number",
                number: 13_424_921_438.5,
                unit: "currency",
              },
            },
          },
        },
      ],
      total: 2,
      hasMore: false,
    });
    const wrapper = mount(StockScreenerView, {
      props: { market: "HK", brokerId: "futu" },
    });
    await flushPromises();

    await wrapper.get(".stock-screener-view__run").trigger("click");
    await flushPromises();

    expect(wrapper.text()).toContain("HKD 52.70");
    expect(wrapper.text()).toContain("HKD 194.26亿");
    expect(wrapper.text()).toContain("CNY 52.70");
    expect(wrapper.text()).toContain("CNY 134.25亿");
    expect(wrapper.text()).not.toContain("currency");
    expect(wrapper.text()).not.toContain("货币");
  });

  it("keeps successful results visible but marks them stale after a draft edit", async () => {
    installDefaults();
    mocks.run.mockResolvedValue({
      provider: { asOf: "2026-07-23T09:30:00Z" },
      asOf: "2026-07-23T09:30:00Z",
      entries: [
        {
          stockId: "1",
          instrumentId: "US.AAPL",
          market: "US",
          symbol: "AAPL",
          name: "Apple",
          productClass: "equity",
          cells: {
            "column-simple.price-0": {
              columnId: "column-simple.price-0",
              instanceId: "default-simple.price",
              factorKey: "simple.price",
              value: { type: "number", number: 200 },
            },
          },
        },
      ],
      hasMore: false,
      total: 1,
    });
    const wrapper = mount(StockScreenerView, {
      props: { market: "US", brokerId: "futu" },
    });
    await flushPromises();

    await wrapper.get(".stock-screener-view__run").trigger("click");
    await flushPromises();
    expect(wrapper.findAll("tbody tr")).toHaveLength(1);

    await wrapper
      .findAll(".stock-screener-view__common button")
      .find((button) => button.text().includes("最新价"))!
      .trigger("click");
    expect(wrapper.findAll("tbody tr")).toHaveLength(1);
    expect(wrapper.text()).toContain("结果待更新");
    expect(mocks.run).toHaveBeenCalledTimes(1);
  });

  it("new strategy clears the preset draft, results, sorting, and restores default columns", async () => {
    installDefaults();
    const wrapper = mount(StockScreenerView, {
      props: { market: "US", brokerId: "futu", initialPresetId: "preset-1" },
    });
    await flushPromises();

    expect(wrapper.findAll(".stock-screener-view__condition")).toHaveLength(1);
    await wrapper
      .findAll(".stock-screener-view__toolbar button")
      .find((button) => button.text() === "新建")!
      .trigger("click");

    expect(wrapper.findAll(".stock-screener-view__condition")).toHaveLength(0);
    expect(wrapper.findAll(".stock-screener-view__sorts > div")).toHaveLength(0);
    expect(wrapper.findAll(".stock-screener-view__column-picker > div")).toHaveLength(2);
    expect(wrapper.get('[aria-label="预设名称"]').element).toHaveProperty("value", "");
    expect(wrapper.emitted("presetChange")?.at(-1)).toEqual([""]);
  });

  it("sends executable V2 interval values as the only request contract", async () => {
    installDefaults();
    mocks.run.mockResolvedValue({
      provider: { asOf: "2026-07-24T02:00:00Z" },
      asOf: "2026-07-24T02:00:00Z",
      columns: [
        {
          columnId: "column-simple.price-0",
          instanceId: "column-1",
          factorKey: "simple.price",
        },
        {
          columnId: "column-simple.market_cap-1",
          instanceId: "column-2",
          factorKey: "simple.market_cap",
        },
      ],
      entries: [
        {
          stockId: "1",
          instrumentId: "US.AAPL",
          market: "US",
          symbol: "AAPL",
          name: "Apple",
          quoteCurrency: "USD",
          productClass: "equity",
          cells: {
            "column-simple.price-0": {
              columnId: "column-simple.price-0",
              instanceId: "column-1",
              factorKey: "simple.price",
              value: { type: "number", number: 55 },
            },
          },
        },
      ],
      hasMore: false,
      total: 1,
    });
    const wrapper = mount(StockScreenerView, {
      props: { market: "US", brokerId: "futu" },
    });
    await flushPromises();

    await wrapper
      .findAll(".stock-screener-view__common button")
      .find((button) => button.text().includes("最新价"))!
      .trigger("click");
    await wrapper.get('[aria-label="条件下限"]').setValue("10");
    await wrapper.get('[aria-label="条件上限"]').setValue("100");
    await wrapper.get(".stock-screener-view__run").trigger("click");
    await flushPromises();

    expect(mocks.run.mock.calls.at(-1)?.[0]).toMatchObject({
      market: "US",
      querySchemaVersion: 2,
      catalogVersion: "futu-stock-screen-v1",
      conditions: [
        {
          operator: "between",
          value: {
            min: 10,
            minIncludes: true,
            max: 100,
            maxIncludes: true,
          },
          id: expect.any(String),
          factor: expect.objectContaining({ factorKey: "simple.price" }),
        },
      ],
    });
    expect(wrapper.text()).toContain("USD 55.00");
    expect(wrapper.get(".stock-screener-view__builder").classes()).toContain(
      "is-mobile-hidden",
    );
    expect(wrapper.get(".stock-screener-view__results").classes()).not.toContain(
      "is-mobile-hidden",
    );
  });

  it("protects dirty drafts when creating a new strategy", async () => {
    installDefaults();
    const wrapper = mount(StockScreenerView, {
      props: { market: "US", brokerId: "futu", initialPresetId: "preset-1" },
      global: { stubs: { teleport: true } },
    });
    await flushPromises();

    await wrapper.get('[aria-label="条件下限"]').setValue("11");
    const newButton = wrapper
      .findAll(".stock-screener-view__toolbar button")
      .find((button) => button.text() === "新建")!;
    await newButton.trigger("click");
    expect(wrapper.text()).toContain("当前策略有未保存修改");

    const dialog = wrapper.get(".stock-screener-view__draft-dialog");
    await dialog
      .findAll("button")
      .find((button) => button.text() === "取消")!
      .trigger("click");
    expect(wrapper.findAll(".stock-screener-view__condition")).toHaveLength(1);
    expect(wrapper.get('[aria-label="预设名称"]').element).toHaveProperty("value", "低估值");

    await newButton.trigger("click");
    await wrapper
      .get(".stock-screener-view__draft-dialog")
      .findAll("button")
      .find((button) => button.text() === "放弃修改")!
      .trigger("click");
    expect(wrapper.findAll(".stock-screener-view__condition")).toHaveLength(0);
    expect(wrapper.get('[aria-label="预设名称"]').element).toHaveProperty("value", "");
  });

  it("saves a dirty draft before continuing a protected transition", async () => {
    installDefaults();
    const wrapper = mount(StockScreenerView, {
      props: { market: "US", brokerId: "futu", initialPresetId: "preset-1" },
      global: { stubs: { teleport: true } },
    });
    await flushPromises();

    await wrapper.get('[aria-label="条件下限"]').setValue("12");
    await wrapper
      .findAll(".stock-screener-view__toolbar button")
      .find((button) => button.text() === "新建")!
      .trigger("click");
    await wrapper
      .get(".stock-screener-view__draft-dialog")
      .findAll("button")
      .find((button) => button.text() === "保存后继续")!
      .trigger("click");
    await flushPromises();

    expect(mocks.update).toHaveBeenCalledWith(
      "preset-1",
      "低估值",
      expect.objectContaining({
        conditions: [
          expect.objectContaining({
            value: expect.objectContaining({ min: 12, minIncludes: true }),
          }),
        ],
      }),
      3,
    );
    expect(wrapper.findAll(".stock-screener-view__condition")).toHaveLength(0);
  });

  it("keeps the draft on market changes and marks incompatible factors", async () => {
    installDefaults();
    mocks.catalog
      .mockResolvedValueOnce(catalog)
      .mockResolvedValueOnce({
        ...catalog,
        market: "HK",
        factors: catalog.factors.map((factor) =>
          factor.key === "simple.price"
            ? {
                ...factor,
                availability: "unsupported",
                reason: "当前 HK 市场不可用",
              }
            : factor,
        ),
      });
    const wrapper = mount(StockScreenerView, {
      props: { market: "US", brokerId: "futu" },
    });
    await flushPromises();

    await wrapper
      .findAll(".stock-screener-view__common button")
      .find((button) => button.text().includes("最新价"))!
      .trigger("click");
    await wrapper.get('[aria-label="条件下限"]').setValue("10");
    await wrapper.get('[aria-label="筛选市场"]').setValue("HK");
    await flushPromises();

    expect(mocks.catalog).toHaveBeenLastCalledWith("HK", "futu");
    expect(wrapper.findAll(".stock-screener-view__condition")).toHaveLength(1);
    expect(wrapper.text()).toContain("当前 HK 市场不可用");
    expect(wrapper.find(".stock-screener-view__draft-dialog").exists()).toBe(false);
  });
});
