// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { nextTick } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type {
  StockScreenCatalog,
  StockScreenEditorFilter,
  StockScreenFactor,
  StockScreenPreset,
} from "../../src/components/research/stockScreenTypes";
import { flushPromises, setupState } from "../productTestUtils";

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

function factor(overrides: Partial<StockScreenFactor>): StockScreenFactor {
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

const richCatalog: StockScreenCatalog = {
  version: "v2",
  schemaVersion: 2,
  querySchemaVersion: 2,
  provider: "futu",
  providerVersion: "1",
  market: "US",
  markets: ["US", "HK", "SH", "SZ"],
  categories: [
    { key: "simple", label: "行情", count: 4 },
    { key: "indicator", label: "指标", count: 2 },
    { key: "pattern", label: "形态", count: 1 },
  ],
  factors: [
    factor({ key: "basic.code", label: "代码", filter: false }),
    factor({ key: "basic.name", label: "名称", valueType: "string", filter: false, sort: false }),
    factor({ key: "simple.price", label: "最新价" }),
    factor({ key: "simple.market_cap", label: "市值" }),
    factor({ key: "field.market", label: "市场", filterKind: "enum", valueType: "integer", valueEnum: "market" }),
    factor({ key: "field.tags", label: "标签集合", filterKind: "set", valueType: "integer_array", sort: false }),
    factor({ key: "simple.range_or_set", label: "区间集合", filterKind: "interval_or_set" }),
    factor({ key: "pattern.candle", label: "K 线形态", category: "pattern", filterKind: "pattern", valueType: "integer_array", valueEnum: "pattern" }),
    factor({ key: "indicator.ma", label: "均线", category: "indicator", filterKind: "position", parameters: [{ name: "period", type: "integer", enum: "period" }] }),
    factor({ key: "indicator.rsi", label: "RSI", category: "indicator", filterKind: "position", parameters: [{ name: "days", type: "integer", required: true, minimum: 1 }] }),
    factor({ key: "experimental.alpha", label: "实验因子", filter: false, availability: "experimental", help: "alpha help", searchKeywords: ["test"] }),
    factor({ key: "unsupported.factor", label: "受限因子", availability: "unsupported", reason: "" }),
  ],
  enums: {
    market: [
      { key: "us", value: 2, label: "美股" },
      { key: "hk", value: 1, label: "港股" },
    ],
    pattern: [{ key: "hammer", value: 1, label: "锤头" }],
    period: [{ key: "day", value: 11, label: "日线" }],
    position: [
      { key: "over", value: 1, label: "上方" },
      { key: "below", value: 2, label: "下方" },
    ],
  },
  rateLimit: { requests: 10, windowSeconds: 30 },
};

const preset: StockScreenPreset = {
  presetId: "preset-1",
  name: "基础策略",
  querySchemaVersion: 2,
  revision: 2,
  definition: {
    brokerId: "futu",
    market: "US",
    catalogVersion: "v2",
    querySchemaVersion: 2,
    conditions: [],
    columns: [],
    sorts: [],
  },
  createdAt: "2026-07-24",
  updatedAt: "2026-07-24",
};

type ScreenerState = {
  catalog: StockScreenCatalog | null;
  catalogError: string;
  presetError: string;
  queryError: string;
  loading: boolean;
  savingPreset: boolean;
  filters: StockScreenEditorFilter[];
  columns: Array<Record<string, unknown>>;
  sorts: Array<Record<string, unknown>>;
  entries: Array<Record<string, unknown>>;
  presets: StockScreenPreset[];
  selectedPresetId: string;
  presetName: string;
  pendingDraftAction: Record<string, unknown> | null;
  validationErrors: Array<{ path: string; message: string }>;
  warnings: string[];
  partialErrors: Array<Record<string, unknown>>;
  retryAfterMs: number;
  categoryScroller: HTMLDivElement | null;
  screenerOuterPaneSizes: [number, number];
  screenerInnerPaneSizes: [number, number];
  handleScreenerOuterPaneResized: (payload: unknown) => void;
  handleScreenerInnerPaneResized: (payload: unknown) => void;
  factorFor: (key: string) => StockScreenFactor | undefined;
  columnExists: (key: string) => boolean;
  enumOptionsForFactor: (value?: StockScreenFactor) => unknown[];
  addFilter: (value: StockScreenFactor) => Promise<void>;
  removeFilter: (id: string) => void;
  addColumn: (key: string) => Promise<void>;
  removeColumn: (value: Record<string, unknown>) => void;
  moveColumn: (index: number, delta: number) => void;
  addSort: (key?: string) => Promise<void>;
  sortFactorInput: (sort: Record<string, unknown>, event: Event) => void;
  boundaryInput: (filter: StockScreenEditorFilter, event: Event, field: "min" | "max") => void;
  valuesInput: (filter: StockScreenEditorFilter, event: Event) => void;
  singleValueInput: (filter: StockScreenEditorFilter, event: Event) => void;
  useSetFilter: (filter: StockScreenEditorFilter) => void;
  useIntervalFilter: (filter: StockScreenEditorFilter) => void;
  secondFactorInput: (filter: StockScreenEditorFilter, event: Event) => void;
  scrollCategories: (direction: -1 | 1) => void;
  setRetryCountdown: (delay: number) => void;
  execute: (offset?: number, append?: boolean) => Promise<void>;
  clearResults: () => void;
  savePreset: () => Promise<boolean>;
  removePreset: () => Promise<void>;
  savePendingDraft: () => Promise<void>;
  discardPendingDraft: () => void;
  exportCSV: () => void;
  changeMarket: (event: Event) => void;
};

function eventWithValue(value: string): Event {
  return { target: { value } } as unknown as Event;
}

function installDefaults(): void {
  mocks.catalog.mockResolvedValue(richCatalog);
  mocks.presets.mockResolvedValue({ presets: [preset] });
  mocks.run.mockResolvedValue({ provider: {}, entries: [], hasMore: false });
  mocks.create.mockResolvedValue({ ...preset, presetId: "created", name: "新策略" });
  mocks.update.mockResolvedValue(preset);
  mocks.remove.mockResolvedValue(undefined);
  mocks.conflict.mockReturnValue(false);
}

async function mountScreener(props: Record<string, unknown> = {}) {
  const wrapper = mount(StockScreenerView, {
    props: { market: "US", brokerId: "futu", ...props },
    global: { stubs: { teleport: true } },
  });
  await flushPromises();
  return { wrapper, state: setupState<ScreenerState>(wrapper) };
}

beforeEach(() => {
  installDefaults();
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  for (const mock of Object.values(mocks)) mock.mockReset();
});

describe("StockScreenerView controller boundaries", () => {
  it("rejects malformed pane sizes and exercises factor editor operations", async () => {
    const { state } = await mountScreener();
    state.handleScreenerOuterPaneResized({ panes: [{ size: 20 }] });
    state.handleScreenerOuterPaneResized({ panes: [{ size: 0 }, { size: 100 }] });
    expect(state.screenerOuterPaneSizes).toEqual([18, 82]);
    state.handleScreenerOuterPaneResized({ panes: [{ size: 22 }, { size: 78 }] });
    state.handleScreenerInnerPaneResized({ panes: [{ size: 42 }, { size: 58 }] });
    expect(state.screenerOuterPaneSizes).toEqual([22, 78]);
    expect(state.screenerInnerPaneSizes).toEqual([42, 58]);
    expect(state.factorFor("missing")).toBeUndefined();
    expect(state.columnExists("missing")).toBe(false);
    expect(state.enumOptionsForFactor()).toEqual([]);
    expect(state.enumOptionsForFactor(state.factorFor("simple.price"))).toEqual([]);
    expect(state.enumOptionsForFactor(state.factorFor("field.market"))).toHaveLength(2);

    const blocked = state.factorFor("unsupported.factor")!;
    await state.addFilter(blocked);
    expect(state.filters).toHaveLength(0);
    await state.addFilter(state.factorFor("simple.price")!);
    await state.addFilter(state.factorFor("simple.price")!);
    expect(state.filters).toHaveLength(1);
    expect(state.queryError).toContain("相同参数");
    const price = state.filters[0]!;
    state.boundaryInput(price, eventWithValue("10"), "min");
    state.boundaryInput(price, eventWithValue(""), "min");
    expect(price.min).toBeUndefined();
    state.removeFilter(price.id);
    expect(state.filters).toHaveLength(0);

    await state.addFilter(state.factorFor("field.tags")!);
    const tags = state.filters[0]!;
    state.valuesInput(tags, eventWithValue("1, bad, 2"));
    expect(tags.values).toEqual([1, 2]);
    state.singleValueInput(tags, eventWithValue("7"));
    expect(tags.values).toEqual([7]);
    await state.addFilter(state.factorFor("simple.range_or_set")!);
    const hybrid = state.filters[1]!;
    hybrid.min = { value: 1 };
    hybrid.max = { value: 2 };
    hybrid.intervals = [];
    state.useSetFilter(hybrid);
    expect(hybrid.values).toEqual([0]);
    state.useIntervalFilter(hybrid);
    expect(hybrid.values).toBeUndefined();
  });

  it("adds, moves, removes, and retargets columns and sorts", async () => {
    const { state } = await mountScreener();
    const initialColumns = state.columns.length;
    await state.addColumn("missing");
    await state.addColumn("experimental.alpha");
    await state.addColumn("experimental.alpha");
    expect(state.columns).toHaveLength(initialColumns + 1);
    expect(state.queryError).toContain("结果列");
    state.moveColumn(state.columns.length - 1, -1);
    const extra = state.columns.find((column) => column.factor === "experimental.alpha")!;
    state.removeColumn(extra);
    expect(state.columns).toHaveLength(initialColumns);

    await state.addSort("missing");
    expect(state.sorts).toHaveLength(0);
    await state.addSort("indicator.ma");
    await state.addSort("indicator.ma");
    expect(state.sorts).toHaveLength(1);
    const sort = state.sorts[0]!;
    state.sortFactorInput(sort, eventWithValue("missing"));
    expect(sort.factor).toBe("indicator.ma");
    state.sortFactorInput(sort, eventWithValue("simple.price"));
    expect(sort.factor).toBe("simple.price");
    expect(sort.params).toBeUndefined();
  });

  it("switches comparison factors and renders every factor-role template", async () => {
    const { wrapper, state } = await mountScreener();
    await state.addFilter(state.factorFor("field.market")!);
    await state.addFilter(state.factorFor("pattern.candle")!);
    await state.addFilter(state.factorFor("indicator.ma")!);
    const position = state.filters.at(-1)!;
    state.secondFactorInput(position, eventWithValue("missing"));
    expect(position.secondFactor).toBeUndefined();
    state.secondFactorInput(position, eventWithValue("indicator.rsi"));
    expect(position.secondFactor).toMatchObject({ factor: "indicator.rsi" });
    expect(position.secondValue).toBeUndefined();
    state.secondFactorInput(position, eventWithValue(""));
    expect(position.secondFactor).toBeUndefined();
    await nextTick();

    expect(wrapper.find('[aria-label="枚举条件值"]').exists()).toBe(true);
    expect(wrapper.find('[aria-label="形态匹配"]').exists()).toBe(true);
    expect(wrapper.find('[aria-label="比较指标"]').exists()).toBe(true);
    await wrapper.get(".stock-screener-view__add-factor").trigger("click");
    const roles = wrapper.findAll('.stock-screener-view__factor-roles button');
    await roles[1]!.trigger("click");
    expect(wrapper.text()).toContain("实验因子");
    await roles[2]!.trigger("click");
    await wrapper.get('[aria-label="搜索因子"]').setValue("受限");
    expect(wrapper.text()).toContain("当前市场不可用");
    await wrapper.get('[aria-label="关闭添加因子"]').trigger("click");
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false);
  });

  it("handles inactive, missing-catalog, validation, field, and retry failures", async () => {
    vi.useFakeTimers();
    const inactive = await mountScreener({ active: false });
    await inactive.state.execute();
    expect(mocks.run).not.toHaveBeenCalled();
    inactive.wrapper.unmount();

    const { wrapper, state } = await mountScreener();
    state.catalog = null;
    await state.execute();
    expect(state.queryError).toContain("尚未加载");
    state.catalog = richCatalog;
    state.filters = [{ id: "invalid", factor: "simple.price" }];
    await state.execute();
    expect(state.queryError).toContain("修正标红字段");
    state.filters = [];

    const error = Object.assign(
      new Error("conditions[0].factor.params.period: 周期过小"),
      { retryAfterMs: 2_000 },
    );
    mocks.run.mockRejectedValueOnce(error);
    await state.execute();
    expect(state.queryError).toContain("周期过小");
    expect(state.validationErrors).toEqual([
      { path: "conditions.0.params.period", message: "周期过小" },
    ]);
    expect(state.retryAfterMs).toBe(2_000);
    await vi.advanceTimersByTimeAsync(2_000);
    expect(state.retryAfterMs).toBe(0);
    expect(wrapper.get("[role=status]").text()).toContain("需要修正");

    mocks.run.mockRejectedValueOnce("plain failure");
    state.validationErrors = [];
    await state.execute();
    expect(state.queryError).toBe("plain failure");
  });

  it("keeps successful metadata, ignores stale runs, and exports stock-id rows", async () => {
    const createObjectURL = vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:screen");
    vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    const { wrapper, state } = await mountScreener();
    mocks.run.mockResolvedValueOnce({
      provider: { asOf: "provider-time" },
      entries: [{ stockId: "42", symbol: "TEST", name: "Test", productClass: "equity", cells: {} }],
      hasMore: true,
      warnings: ["数据有延迟"],
      partialErrors: [{ message: "部分指标失败" }, { code: "NO_DATA" }, {}],
    });
    await state.execute();
    expect(wrapper.text()).toContain("provider-time");
    expect(wrapper.text()).toContain("数据有延迟");
    expect(wrapper.text()).toContain("部分指标失败");
    expect(wrapper.text()).toContain("NO_DATA");
    expect(wrapper.text()).toContain("部分结果不可用");
    await wrapper.get("tbody tr").trigger("keydown.enter");
    expect(wrapper.emitted("select")?.[0]?.[0]).toMatchObject({ stockId: "42" });
    state.exportCSV();
    expect(createObjectURL).toHaveBeenCalledOnce();

    let resolveRun: ((value: Record<string, unknown>) => void) | undefined;
    mocks.run.mockImplementationOnce(() => new Promise((resolve) => { resolveRun = resolve; }));
    const pending = state.execute();
    await flushPromises();
    state.clearResults();
    resolveRun?.({ provider: {}, entries: [], hasMore: false });
    await pending;
    expect(state.entries).toEqual([]);
  });

  it("covers save, pending-save, delete cancellation, and API failures", async () => {
    const { wrapper, state } = await mountScreener();
    expect(await state.savePreset()).toBe(false);
    state.presetName = "新策略";
    state.catalog = null;
    expect(await state.savePreset()).toBe(false);
    expect(state.presetError).toContain("尚未加载");
    state.catalog = richCatalog;
    state.filters = [{ id: "invalid", factor: "simple.price" }];
    expect(await state.savePreset()).toBe(false);
    expect(state.presetError).toContain("修正标红字段");
    state.filters = [];

    mocks.create.mockRejectedValueOnce(new Error("create failed"));
    expect(await state.savePreset()).toBe(false);
    expect(state.presetError).toBe("create failed");
    mocks.create.mockResolvedValueOnce({ ...preset, presetId: "created", name: "新策略" });
    expect(await state.savePreset()).toBe(true);
    expect(state.selectedPresetId).toBe("created");

    state.pendingDraftAction = null;
    await state.savePendingDraft();
    state.pendingDraftAction = { kind: "new" };
    state.presetName = "";
    await state.savePendingDraft();
    expect(state.presetError).toContain("填写预设名称");

    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    state.selectedPresetId = "preset-1";
    state.presets = [preset];
    await state.removePreset();
    expect(mocks.remove).not.toHaveBeenCalled();
    confirm.mockReturnValue(true);
    mocks.remove.mockRejectedValueOnce(new Error("delete failed"));
    await state.removePreset();
    expect(state.presetError).toBe("delete failed");
    mocks.remove.mockResolvedValueOnce(undefined);
    await state.removePreset();
    expect(wrapper.emitted("presetChange")?.at(-1)).toEqual([""]);
  });

  it("reports incompatible catalogs, catalog errors, and no-op market changes", async () => {
    mocks.catalog.mockResolvedValueOnce({ ...richCatalog, querySchemaVersion: 1 });
    const incompatible = await mountScreener();
    expect(incompatible.state.catalogError).toContain("不是 V2");
    incompatible.wrapper.unmount();

    mocks.catalog.mockRejectedValueOnce("catalog unavailable");
    const failed = await mountScreener();
    expect(failed.state.catalogError).toBe("catalog unavailable");
    failed.state.changeMarket(eventWithValue("US"));
    expect(failed.wrapper.emitted("contextChange")).toBeUndefined();
    failed.state.changeMarket(eventWithValue("HK"));
    await flushPromises();
    expect(failed.wrapper.emitted("contextChange")?.at(-1)).toEqual([
      { market: "HK", brokerId: "futu" },
    ]);
  });

  it("drives remaining template controls and a dirty preset transition", async () => {
    const disconnect = vi.fn();
    const observe = vi.fn();
    vi.stubGlobal(
      "ResizeObserver",
      class {
        disconnect = disconnect;
        observe = observe;
      },
    );
    const { wrapper, state } = await mountScreener();
    await wrapper.findAll('[role="tab"]')[1]!.trigger("click");
    await wrapper.findAll('[role="tab"]')[0]!.trigger("click");

    await state.addFilter(state.factorFor("simple.range_or_set")!);
    await nextTick();
    const segment = wrapper.get(".stock-screener-view__condition .tv-seg");
    await segment.findAll("button")[1]!.trigger("click");
    await wrapper.get('[aria-label="集合条件值"]').setValue("3,4");
    await segment.findAll("button")[0]!.trigger("click");
    await wrapper.get('[aria-label="条件下限"]').setValue("5");

    await wrapper.get(".stock-screener-view__preset-list button").trigger("click");
    expect(wrapper.text()).toContain("切换到“基础策略”");
    await wrapper
      .get(".stock-screener-view__draft-dialog")
      .findAll("button")[1]!
      .trigger("click");
    expect(state.selectedPresetId).toBe("preset-1");

    await wrapper.get(".stock-screener-view__add-factor").trigger("click");
    expect(observe).toHaveBeenCalled();
    const scroller = wrapper.get(".stock-screener-view__categories").element as HTMLDivElement;
    Object.defineProperties(scroller, {
      clientWidth: { configurable: true, value: 200 },
      scrollWidth: { configurable: true, value: 500 },
      scrollLeft: { configurable: true, writable: true, value: 100 },
    });
    Object.defineProperty(scroller, "scrollBy", { configurable: true, value: undefined });
    state.categoryScroller = scroller;
    await wrapper.get(".stock-screener-view__categories").trigger("scroll");
    state.scrollCategories(-1);
    await wrapper.findAll(".stock-screener-view__categories button")[0]!.trigger("click");
    await wrapper.findAll(".stock-screener-view__categories button")[1]!.trigger("click");
    await wrapper.findAll(".stock-screener-view__factor-roles button")[0]!.trigger("click");
    await wrapper.get(".stock-screener-view__factor-dialog-backdrop").trigger("keydown.esc");
    expect(disconnect).toHaveBeenCalled();
  });

  it("covers conflict cancellation, field save errors, empty actions, and prop watches", async () => {
    vi.useFakeTimers();
    const { wrapper, state } = await mountScreener();
    state.exportCSV();
    state.selectedPresetId = "";
    await state.removePreset();
    state.catalog = null;
    await state.addSort();
    state.catalog = richCatalog;

    state.presetName = "基础策略";
    mocks.create.mockRejectedValueOnce(new Error("duplicate"));
    mocks.conflict.mockReturnValueOnce(true);
    vi.spyOn(window, "confirm").mockReturnValue(false);
    expect(await state.savePreset()).toBe(false);

    mocks.create.mockRejectedValueOnce(
      new Error("columns[0].factor.factorKey: 结果列失效"),
    );
    mocks.conflict.mockReturnValue(false);
    expect(await state.savePreset()).toBe(false);
    expect(state.validationErrors).toEqual([
      { path: "columns.0.factor", message: "结果列失效" },
    ]);

    state.setRetryCountdown(5_000);
    state.setRetryCountdown(3_000);
    await wrapper.setProps({ market: " us " });
    await flushPromises();
    await wrapper.setProps({ initialPresetId: "preset-1" });
    await flushPromises();
    expect(state.selectedPresetId).toBe("preset-1");
    wrapper.unmount();
  });
});
