// @vitest-environment jsdom

import { mount, type VueWrapper } from "@vue/test-utils";
import { defineComponent, h, ref, type Component } from "vue";
import { describe, expect, it, vi } from "vitest";

import StockScreenerBuilder from "../../src/components/research/StockScreenerBuilder.vue";
import StockScreenerDialogs from "../../src/components/research/StockScreenerDialogs.vue";
import StockScreenerPresetSidebar from "../../src/components/research/StockScreenerPresetSidebar.vue";
import StockScreenerToolbar from "../../src/components/research/StockScreenerToolbar.vue";
import type { StockScreenFactor } from "../../src/components/research/stockScreenTypes";
import { useActionConfirmation } from "@/composables/shared/useActionConfirmation";
import {
  provideStockScreenerController,
  type StockScreenerController,
} from "../../src/components/research/useStockScreenerController";

function mountControl(
  component: Component,
  controller: Record<string, unknown>,
): VueWrapper {
  const host = defineComponent({
    setup() {
      provideStockScreenerController(
        {
          actionConfirmation: useActionConfirmation(),
          ...controller,
        } as unknown as StockScreenerController,
      );
      return () => h(component);
    },
  });
  return mount(host, {
    global: {
      stubs: {
        teleport: true,
        StockScreenParameterEditor: true,
      },
    },
  });
}

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

describe("StockScreener extracted controls", () => {
  it("routes toolbar and preset-sidebar actions through the controller", async () => {
    const changeMarket = vi.fn();
    const choosePreset = vi.fn();
    const newPreset = vi.fn();
    const savePreset = vi.fn();
    const removePreset = vi.fn();
    const exportCSV = vi.fn();
    const execute = vi.fn();
    const mobilePane = ref("builder");
    const queryMarket = ref("US");
    const presetName = ref("");
    const catalogError = ref("");
    const preset = { presetId: "preset-1", name: "策略 A" };
    const toolbar = mountControl(StockScreenerToolbar, {
      queryMarket,
      changeMarket,
      selectedPresetId: ref("preset-1"),
      choosePreset,
      presets: ref([preset]),
      presetName,
      newPreset,
      savingPreset: ref(false),
      savePreset,
      selectedPreset: ref(preset),
      removePreset,
      screenStatus: ref("warning"),
      screenStatusLabel: ref("结果已过期"),
      entries: ref([{ stockId: "1" }]),
      exportCSV,
      loading: ref(false),
      catalogLoading: ref(false),
      retryAfterMs: ref(0),
      execute,
      catalogError,
      presetError: ref(""),
      queryError: ref(""),
      warnings: ref(["行情存在延迟"]),
      partialErrors: ref([
        { code: "NO_DATA", message: "部分指标缺失" },
        { code: "FALLBACK" },
        {},
      ]),
      mobilePane,
    });

    queryMarket.value = "HK";
    catalogError.value = "目录暂不可用";
    mobilePane.value = "results";
    await toolbar.vm.$nextTick();
    await toolbar.get('[aria-label="筛选市场"]').setValue("US");
    await toolbar.get(".stock-screener-view__preset-select").setValue("preset-1");
    await toolbar.get('[aria-label="预设名称"]').setValue("策略 B");
    for (const label of ["新建", "保存", "删除", "导出 CSV", "执行筛选"]) {
      await toolbar
        .findAll("button")
        .find((button) => button.text() === label)!
        .trigger("click");
    }
    const mobileTabs = toolbar.findAll(
      ".stock-screener-view__mobile-tabs button",
    );
    await mobileTabs[1]!.trigger("click");
    await mobileTabs[0]!.trigger("click");

    expect(changeMarket).toHaveBeenCalledOnce();
    expect(choosePreset).toHaveBeenCalledOnce();
    expect(presetName.value).toBe("策略 B");
    expect([newPreset, savePreset, removePreset, exportCSV, execute]).toSatisfy(
      (actions: Array<ReturnType<typeof vi.fn>>) =>
        actions.every((action) => action.mock.calls.length === 1),
    );
    expect(execute).toHaveBeenCalledWith(0, false);
    expect(mobilePane.value).toBe("builder");
    expect(toolbar.text()).toContain("部分结果不可用");

    const choosePresetFromSidebar = vi.fn();
    const sidebar = mountControl(StockScreenerPresetSidebar, {
      presets: ref([preset]),
      newPreset,
      selectedPresetId: ref("preset-1"),
      choosePresetFromSidebar,
    });
    await sidebar.get(".stock-screener-view__new-preset").trigger("click");
    await sidebar.get(".stock-screener-view__preset-list button").trigger("click");
    expect(newPreset).toHaveBeenCalledTimes(2);
    expect(choosePresetFromSidebar).toHaveBeenCalledWith(preset);
  });

  it("routes builder edits without losing typed filter and column values", async () => {
    const factors = [
      factor({ key: "field.market", label: "市场", filterKind: "enum" }),
      factor({ key: "indicator.ma", label: "均线", category: "indicator", filterKind: "position" }),
      factor({ key: "pattern.candle", label: "形态", category: "pattern", filterKind: "pattern" }),
      factor({ key: "simple.price", label: "最新价" }),
      factor({ key: "simple.market_cap", label: "市值" }),
      factor({ key: "indicator.rsi", label: "RSI", category: "indicator" }),
    ];
    const filters = ref([
      { id: "market", factor: "field.market", values: [2] },
      { id: "position", factor: "indicator.ma", position: 1, secondValue: 10, continuousPeriod: 1 },
      { id: "pattern", factor: "pattern.candle", match: true, values: [1], continuousPeriod: 1 },
    ]);
    const columns = ref([
      { factor: "simple.price", columnId: "price" },
      { factor: "simple.market_cap", columnId: "cap" },
    ]);
    const sorts = ref([
      { factor: "simple.price", sortId: "price-sort", direction: "desc" },
    ]);
    const factorFor = (key: string) => factors.find((item) => item.key === key);
    const removeFilter = vi.fn();
    const singleValueInput = vi.fn();
    const valuesInput = vi.fn();
    const secondFactorInput = vi.fn();
    const addColumn = vi.fn();
    const moveColumn = vi.fn();
    const removeColumn = vi.fn();
    const addSort = vi.fn();
    const sortFactorInput = vi.fn();
    const catalog = ref<{
      factors: StockScreenFactor[];
      enums: { position: Array<{ key: string; value: number; label: string }> };
    } | null>({
      factors,
      enums: { position: [{ key: "above", value: 2, label: "上方" }] },
    });
    const builder = mountControl(StockScreenerBuilder, {
      mobilePane: ref("builder"),
      filters,
      addFactorButton: ref(null),
      factorDialogOpen: ref(false),
      openFactorDialog: vi.fn(),
      commonFactors: ref([]),
      hasDuplicateRef: () => false,
      addFilter: vi.fn(),
      factorFor,
      useIntervalFilter: vi.fn(),
      useSetFilter: vi.fn(),
      removeFilter,
      enumOptionsForFactor: (item?: StockScreenFactor) =>
        item?.key === "field.market" ? [{ key: "hk", value: 1, label: "港股" }] : [],
      singleValueInput,
      valuesInput,
      catalog,
      secondFactorInput,
      boundaryInput: vi.fn(),
      fieldErrorWithin: (path: string) =>
        path === "sorts.0" ? "排序参数无效" : "",
      validationErrors: ref([]),
      columns,
      columnIdentity: (column: { columnId: string }) => column.columnId,
      moveColumn,
      removeColumn,
      retrievableFactors: ref(factors),
      columnExists: (key: string) => columns.value.some((column) => column.factor === key),
      addColumn,
      sorts,
      addSort,
      sortIdentity: (sort: { sortId: string }) => sort.sortId,
      sortableFactors: ref(factors),
      sortFactorInput,
    });

    await builder.get('[aria-label="枚举条件值"]').setValue("1");
    await builder.get('[aria-label="位置关系"]').setValue("2");
    await builder.get('[aria-label="比较值"]').setValue("25");
    await builder.get('[aria-label="形态匹配"]').setValue("false");
    await builder.get('[aria-label="子形态"]').setValue("1,2");
    await builder.findAll('[aria-label="连续周期"]')[0]!.setValue("3");
    await builder.get('[aria-label="添加结果列"]').setValue("indicator.rsi");
    await builder.findAll('[aria-label="上移结果列"]')[1]!.trigger("click");
    await builder.findAll('[aria-label="下移结果列"]')[0]!.trigger("click");
    await builder
      .findAll(".stock-screener-view__condition-title button")
      .at(-1)!
      .trigger("click");
    await builder
      .findAll(".stock-screener-view__panel-head button")
      .find((button) => button.text() === "添加排序")!
      .trigger("click");
    await builder.get('[aria-label="排序字段"]').setValue("simple.market_cap");
    await builder.get('[aria-label="排序方向"]').setValue("asc");
    await builder.get(".stock-screener-view__sorts button").trigger("click");
    await builder
      .findAll(".stock-screener-view__column-picker > div")
      .at(-1)!
      .findAll("button")
      .at(-1)!
      .trigger("click");
    catalog.value = null;
    await builder.vm.$nextTick();

    expect(singleValueInput).toHaveBeenCalledOnce();
    expect(valuesInput).toHaveBeenCalledOnce();
    expect(secondFactorInput).not.toHaveBeenCalled();
    expect(addColumn).toHaveBeenCalledWith("indicator.rsi");
    expect(moveColumn).toHaveBeenCalledTimes(2);
    expect(removeColumn).toHaveBeenCalledOnce();
    expect(removeFilter).toHaveBeenCalledWith("pattern");
    expect(addSort).toHaveBeenCalledOnce();
    expect(sortFactorInput).toHaveBeenCalledOnce();
    expect(filters.value[1]).toMatchObject({ position: 2, secondValue: 25, continuousPeriod: 3 });
    expect(filters.value[2]?.match).toBe(false);
    expect(sorts.value).toHaveLength(0);
  });

  it("routes factor-dialog choices and draft-resolution actions", async () => {
    const pendingDraftAction = ref<Record<string, unknown> | null>(null);
    const pendingDraftActionLabel = ref("切换策略");
    const factorDialogOpen = ref(false);
    const activeFactorRole = ref("filter");
    const activeCategory = ref("");
    const closeFactorDialog = vi.fn();
    const scrollCategories = vi.fn();
    const updateCategoryScrollState = vi.fn();
    const addFilter = vi.fn();
    const addColumn = vi.fn();
    const addSort = vi.fn();
    const available = factor({ key: "simple.price", label: "最新价" });
    const dialogs = mountControl(StockScreenerDialogs, {
      pendingDraftAction,
      pendingDraftActionLabel,
      savingPreset: ref(false),
      savePendingDraft: vi.fn(),
      discardPendingDraft: vi.fn(),
      factorDialogOpen,
      closeFactorDialog,
      factorSearchInput: ref(null),
      catalogSearch: ref(""),
      activeFactorRole,
      canScrollCategoriesLeft: ref(true),
      canScrollCategoriesRight: ref(true),
      scrollCategories,
      categoryScroller: ref(null),
      updateCategoryScrollState,
      activeCategory,
      catalog: ref({ categories: [{ key: "simple", label: "基础", count: 1 }] }),
      visibleCatalogFactors: ref([
        available,
        factor({ key: "experimental.alpha", label: "实验", availability: "experimental" }),
        factor({ key: "unsupported.alpha", label: "受限", availability: "unsupported", reason: "无权限" }),
      ]),
      hasDuplicateRef: () => false,
      filters: ref([]),
      addFilter,
      columnExists: () => false,
      addColumn,
      sorts: ref([]),
      addSort,
    });

    pendingDraftAction.value = { kind: "new" };
    pendingDraftActionLabel.value = "新建策略";
    factorDialogOpen.value = true;
    await dialogs.vm.$nextTick();

    const draftButtons = dialogs.findAll(
      ".stock-screener-view__draft-dialog-actions button",
    );
    await draftButtons[0]!.trigger("click");
    await draftButtons[1]!.trigger("click");
    await dialogs.get('[aria-label="搜索因子"]').setValue("价格");
    const roleButtons = dialogs.findAll(".stock-screener-view__factor-roles button");
    await roleButtons[1]!.trigger("click");
    await roleButtons[2]!.trigger("click");
    await roleButtons[0]!.trigger("click");
    await dialogs.get('[aria-label="向左滚动因子分类"]').trigger("click");
    await dialogs.get('[aria-label="向右滚动因子分类"]').trigger("click");
    await dialogs.get(".stock-screener-view__categories").trigger("scroll");
    await dialogs.findAll(".stock-screener-view__categories button")[1]!.trigger("click");
    const availableActions = dialogs
      .findAll(".stock-screener-view__factor-list article")[0]!
      .findAll("button");
    await availableActions[0]!.trigger("click");
    await availableActions[1]!.trigger("click");
    await availableActions[2]!.trigger("click");
    await dialogs.get('[aria-label="关闭添加因子"]').trigger("click");
    const factorBackdrop = dialogs.findAll(
      ".stock-screener-view__factor-dialog-backdrop",
    )[1]!;
    await factorBackdrop.trigger("click");
    await factorBackdrop.trigger("keydown.esc");
    await draftButtons[2]!.trigger("click");

    expect(pendingDraftAction.value).toBeNull();
    expect(activeFactorRole.value).toBe("filter");
    expect(activeCategory.value).toBe("simple");
    expect(scrollCategories.mock.calls).toEqual([[-1], [1]]);
    expect(updateCategoryScrollState).toHaveBeenCalledOnce();
    expect(addFilter).toHaveBeenCalledWith(available);
    expect(addColumn).toHaveBeenCalledWith("simple.price");
    expect(addSort).toHaveBeenCalledWith("simple.price");
    expect(closeFactorDialog).toHaveBeenCalledTimes(3);
  });
});
