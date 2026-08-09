import {
  computed,
  nextTick,
  onMounted,
  onUnmounted,
  ref,
  watch,
} from "vue";

import {
  createStockScreenPreset,
  deleteStockScreenPreset,
  fetchStockScreenCatalog,
  fetchStockScreenPresets,
  isPresetConflict,
  runStockScreen,
  updateStockScreenPreset,
} from "./stockScreenApi";
import { useActionConfirmation } from "@/composables/shared/useActionConfirmation";
import {
  normalizeScreenMarket,
  stockScreenCSV,
  stockScreenDraftFromDefinitionV2,
  stockScreenQueryFingerprint,
  toStockScreenDefinitionV2,
  toStockScreenDraftFilter,
  validateStockScreenQuery,
} from "./stockScreenModel";
import type {
  StockScreenCatalog,
  StockScreenColumn,
  StockScreenDraft,
  StockScreenEditorFilter,
  StockScreenEntry,
  StockScreenPreset,
  StockScreenSort,
} from "./stockScreenTypes";
import {
  defaultColumnsForCatalog,
  errorMessage,
  pendingDraftActionLabel as resolvePendingDraftActionLabel,
  stockScreenerStatus,
  stockScreenerStatusLabel,
  validationErrorFrom,
  type PendingDraftAction,
  type StockScreenerControllerEmit,
  type StockScreenerControllerProps,
} from "./stockScreenerControllerModels";
import { useStockScreenerFactorBuilder } from "./useStockScreenerFactorBuilder";

export type {
  StockScreenerControllerEmit,
  StockScreenerControllerProps,
} from "./stockScreenerControllerModels";
export {
  provideStockScreenerController,
  useStockScreenerControllerContext,
} from "./stockScreenerControllerContext";

const PAGE_SIZE = 50;

export function useStockScreenerController(
  props: Readonly<StockScreenerControllerProps>,
  emit: StockScreenerControllerEmit,
) {
  const catalog = ref<StockScreenCatalog | null>(null);
  const presets = ref<StockScreenPreset[]>([]);
  const catalogLoading = ref(false);
  const catalogError = ref("");
  const presetError = ref("");
  const queryError = ref("");
  const loading = ref(false);
  const loadingMore = ref(false);
  const savingPreset = ref(false);
  const queryMarket = ref(normalizeScreenMarket(props.market));
  const filters = ref<StockScreenEditorFilter[]>([]);
  const columns = ref<StockScreenColumn[]>([]);
  const sorts = ref<StockScreenSort[]>([]);
  const entries = ref<StockScreenEntry[]>([]);
  const nextOffset = ref<number | undefined>();
  const hasMore = ref(false);
  const total = ref<number | undefined>();
  const asOf = ref("");
  const warnings = ref<string[]>([]);
  const partialErrors = ref<
    Array<{ code?: string; message?: string; [key: string]: unknown }>
  >([]);
  const executedColumns = ref<StockScreenColumn[]>([]);
  const resultColumns = ref<
    Array<{
      columnId: string;
      instanceId?: string;
      factorKey: string;
      label?: string;
    }>
  >([]);
  const lastExecutedFingerprint = ref("");
  const savedFingerprint = ref("");
  const baselineInitialized = ref(false);
  const validationErrors = ref<Array<{ path: string; message: string }>>([]);
  const retryAfterMs = ref(0);
  const selectedPresetId = ref("");
  const presetName = ref("");
  const selectedInstrumentId = ref("");
  const mobilePane = ref<"builder" | "results">("builder");
  const pendingDraftAction = ref<PendingDraftAction | null>(null);
  const actionConfirmation = useActionConfirmation();
  let retryTimer: ReturnType<typeof setInterval> | undefined;
  let catalogToken = 0;
  let queryToken = 0;
  let initialPresetLoaded = "";
  let loadedContextKey = "";
  const factorBuilder = useStockScreenerFactorBuilder({
    catalog,
    columns,
    filters,
    mobilePane,
    queryError,
    queryMarket,
    sorts,
  });
  const {
    activeCategory,
    activeFactorRole,
    addColumn,
    addFactorButton,
    addFilter,
    addSort,
    boundaryInput,
    canScrollCategoriesLeft,
    canScrollCategoriesRight,
    catalogSearch,
    categoryScroller,
    closeFactorDialog,
    columnExists,
    columnIdentity,
    commonFactors,
    enumOptionsForFactor,
    factorDialogOpen,
    factorFor,
    factorMap,
    factorSearchInput,
    handleScreenerInnerPaneResized,
    handleScreenerOuterPaneResized,
    hasDuplicateRef,
    moveColumn,
    nextFactorSerial,
    openFactorDialog,
    removeColumn,
    removeFilter,
    retrievableFactors,
    screenerInnerPaneMinSizes,
    screenerInnerPaneSizes,
    screenerOuterPaneMinSizes,
    screenerOuterPaneSizes,
    scrollCategories,
    secondFactorInput,
    singleValueInput,
    sortFactorInput,
    sortIdentity,
    sortableFactors,
    updateCategoryScrollState,
    useIntervalFilter,
    useSetFilter,
    valuesInput,
    visibleCatalogFactors,
  } = factorBuilder;
  const selectedPreset = computed(() =>
    presets.value.find((preset) => preset.presetId === selectedPresetId.value),
  );
  const screenBrokerId = computed(
    () => props.brokerId.trim() || catalog.value?.provider || "futu",
  );
  const resultLabel = computed(() =>
    total.value == null
      ? `${entries.value.length} 条`
      : `${entries.value.length} / ${total.value} 条`,
  );
  const queryFingerprint = computed(() =>
    stockScreenQueryFingerprint(currentDraft()),
  );
  const currentFingerprint = computed(
    () => `${queryFingerprint.value}|name:${presetName.value.trim()}`,
  );
  const draftDirty = computed(
    () =>
      baselineInitialized.value &&
      currentFingerprint.value !== savedFingerprint.value,
  );
  const resultStale = computed(
    () =>
      entries.value.length > 0 &&
      Boolean(lastExecutedFingerprint.value) &&
      lastExecutedFingerprint.value !== queryFingerprint.value,
  );
  const screenStatus = computed(() =>
    stockScreenerStatus({
      loading: loading.value,
      hasQueryError: Boolean(queryError.value || validationErrors.value.length),
      resultStale: resultStale.value,
      draftDirty: draftDirty.value,
      selectedPresetId: selectedPresetId.value,
    }),
  );
  const screenStatusLabel = computed(() =>
    stockScreenerStatusLabel(screenStatus.value),
  );
  const pendingDraftActionLabel = computed(() =>
    resolvePendingDraftActionLabel(pendingDraftAction.value),
  );
  const displayColumns = computed(() =>
    entries.value.length && executedColumns.value.length
      ? executedColumns.value
      : columns.value,
  );
  const fieldErrorWithin = (path: string): string =>
    validationErrors.value.find(
      (error) => error.path === path || error.path.startsWith(`${path}.`),
    )?.message ?? "";

  function markSavedBaseline(): void {
    baselineInitialized.value = true;
    savedFingerprint.value = currentFingerprint.value;
  }

  function clearResults(): void {
    queryToken += 1;
    entries.value = [];
    executedColumns.value = [];
    resultColumns.value = [];
    nextOffset.value = undefined;
    hasMore.value = false;
    total.value = undefined;
    asOf.value = "";
    warnings.value = [];
    partialErrors.value = [];
    lastExecutedFingerprint.value = "";
  }

  function setRetryCountdown(delayMs: number): void {
    if (retryTimer) clearInterval(retryTimer);
    retryAfterMs.value = Math.max(0, delayMs);
    if (retryAfterMs.value <= 0) return;
    retryTimer = setInterval(() => {
      retryAfterMs.value = Math.max(0, retryAfterMs.value - 1000);
      if (retryAfterMs.value <= 0 && retryTimer) {
        clearInterval(retryTimer);
        retryTimer = undefined;
      }
    }, 1000);
  }

  function currentDraft(): StockScreenDraft {
    return {
      brokerId: screenBrokerId.value,
      market: queryMarket.value,
      filters: filters.value.map(toStockScreenDraftFilter),
      columns: columns.value.map((column) => ({ ...column })),
      sort: sorts.value.map((item) => ({ ...item })),
    };
  }

  function applyPreset(preset: StockScreenPreset): void {
    const query = stockScreenDraftFromDefinitionV2(preset.definition);
    queryMarket.value = normalizeScreenMarket(query.market);
    filters.value = (query.filters ?? []).map((filter) => ({
      ...filter,
      id: filter.conditionId ?? `${filter.factor}-${nextFactorSerial()}`,
    }));
    columns.value = (query.columns ?? []).map((column) => ({ ...column }));
    sorts.value = (query.sort ?? []).map((sort) => ({ ...sort }));
    selectedPresetId.value = preset.presetId;
    presetName.value = preset.name;
    clearResults();
    queryError.value = "";
    validationErrors.value = [];
    markSavedBaseline();
    emit("presetChange", preset.presetId);
    if (
      query.market !== normalizeScreenMarket(props.market) ||
      (query.brokerId ?? "") !== props.brokerId
    ) {
      emit("contextChange", {
        market: query.market,
        ...(query.brokerId ? { brokerId: query.brokerId } : {}),
      });
    }
    if (catalog.value?.market !== query.market) void loadCatalogAndPresets();
  }

  function choosePreset(event: Event): void {
    const target = event.target as HTMLSelectElement;
    const id = target.value;
    if (!id) {
      requestDraftAction({ kind: "new" });
      void nextTick(() => {
        target.value = selectedPresetId.value;
      });
      return;
    }
    const preset = presets.value.find((item) => item.presetId === id);
    if (preset) {
      requestDraftAction({ kind: "preset", preset });
      void nextTick(() => {
        target.value = selectedPresetId.value;
      });
    }
  }

  function choosePresetFromSidebar(preset: StockScreenPreset): void {
    if (preset.presetId === selectedPresetId.value) return;
    requestDraftAction({ kind: "preset", preset });
  }

  function requestDraftAction(action: PendingDraftAction): void {
    if (!draftDirty.value) {
      runDraftAction(action);
      return;
    }
    pendingDraftAction.value = action;
  }

  function runDraftAction(action: PendingDraftAction): void {
    pendingDraftAction.value = null;
    switch (action.kind) {
      case "preset":
        applyPreset(action.preset);
        break;
      case "new":
        applyNewPreset();
        break;
    }
  }

  function discardPendingDraft(): void {
    const action = pendingDraftAction.value;
    if (action) runDraftAction(action);
  }

  async function savePendingDraft(): Promise<void> {
    const action = pendingDraftAction.value;
    if (!action) return;
    if (!presetName.value.trim()) {
      presetError.value = "请先填写预设名称，再保存当前修改";
      return;
    }
    if (await savePreset()) runDraftAction(action);
  }

  async function loadCatalogAndPresets(): Promise<void> {
    const token = ++catalogToken;
    const contextKey = `${queryMarket.value}|${screenBrokerId.value}`;
    catalogLoading.value = true;
    catalogError.value = "";
    presetError.value = "";
    try {
      const [nextCatalog, nextPresets] = await Promise.all([
        fetchStockScreenCatalog(queryMarket.value, screenBrokerId.value),
        fetchStockScreenPresets(),
      ]);
      if (token !== catalogToken) return;
      if (Number(nextCatalog.querySchemaVersion) !== 2) {
        throw new Error("股票筛选目录不是 V2，无法执行");
      }
      catalog.value = nextCatalog;
      loadedContextKey = contextKey;
      presets.value = nextPresets.presets ?? [];
      activeCategory.value ||= nextCatalog.categories[0]?.key ?? "";
      if (!columns.value.length) {
        columns.value = defaultColumnsForCatalog(nextCatalog);
        if (!baselineInitialized.value) markSavedBaseline();
      }
      if (filters.value.length || columns.value.length || sorts.value.length) {
        validationErrors.value = validateStockScreenQuery(
          currentDraft(),
          nextCatalog,
        );
      }
      if (
        props.initialPresetId &&
        initialPresetLoaded !== props.initialPresetId
      ) {
        const preset = presets.value.find(
          (item) => item.presetId === props.initialPresetId,
        );
        if (preset) {
          initialPresetLoaded = props.initialPresetId;
          applyPreset(preset);
        }
      }
    } catch (error) {
      if (token === catalogToken) catalogError.value = errorMessage(error);
    } finally {
      if (token === catalogToken) catalogLoading.value = false;
    }
  }

  async function execute(offset = 0, append = false): Promise<void> {
    if (loading.value || loadingMore.value || !props.active) return;
    if (!catalog.value) {
      queryError.value = "股票筛选 V2 目录尚未加载";
      return;
    }
    const draft = currentDraft();
    const draftErrors = validateStockScreenQuery(draft, catalog.value);
    validationErrors.value = draftErrors;
    if (draftErrors.length) {
      queryError.value = "请先修正标红字段后再执行";
      return;
    }
    const token = ++queryToken;
    if (append) loadingMore.value = true;
    else loading.value = true;
    queryError.value = "";
    setRetryCountdown(0);
    try {
      const definition = toStockScreenDefinitionV2(
        draft,
        catalog.value.version,
      );
      const result = await runStockScreen({
        ...definition,
        page: { offset, limit: PAGE_SIZE },
      });
      if (token !== queryToken) return;
      entries.value = append
        ? [...entries.value, ...result.entries]
        : result.entries;
      if (!append) {
        executedColumns.value = columns.value.map((column) => ({ ...column }));
        resultColumns.value = result.columns ?? [];
      }
      nextOffset.value = result.nextOffset;
      hasMore.value = result.hasMore === true;
      total.value = result.total;
      asOf.value = result.asOf || result.provider.asOf || "";
      warnings.value = result.warnings ?? [];
      partialErrors.value = result.partialErrors ?? [];
      lastExecutedFingerprint.value = queryFingerprint.value;
      validationErrors.value = [];
      if (!append) mobilePane.value = "results";
    } catch (error) {
      if (token === queryToken) {
        queryError.value = errorMessage(error);
        const fieldIssue = validationErrorFrom(error);
        if (fieldIssue) validationErrors.value = [fieldIssue];
        const retry = (error as { retryAfterMs?: number }).retryAfterMs;
        if (Number.isFinite(retry)) setRetryCountdown(Number(retry));
      }
    } finally {
      if (token === queryToken) {
        loading.value = false;
        loadingMore.value = false;
      }
    }
  }

  async function savePreset(): Promise<boolean> {
    const name = presetName.value.trim();
    if (!name || savingPreset.value) return false;
    if (!catalog.value) {
      presetError.value = "股票筛选 V2 目录尚未加载";
      return false;
    }
    const draft = currentDraft();
    const draftErrors = validateStockScreenQuery(draft, catalog.value);
    validationErrors.value = draftErrors;
    if (draftErrors.length) {
      presetError.value = "请先修正标红字段后再保存";
      return false;
    }
    savingPreset.value = true;
    presetError.value = "";
    try {
      const definition = toStockScreenDefinitionV2(
        draft,
        catalog.value.version,
      );
      let saved: StockScreenPreset;
      if (selectedPreset.value) {
        saved = await updateStockScreenPreset(
          selectedPreset.value.presetId,
          name,
          definition,
          selectedPreset.value.revision,
        );
      } else {
        try {
          saved = await createStockScreenPreset(name, definition);
        } catch (error) {
          if (!isPresetConflict(error)) throw error;
          const existing = presets.value.find(
            (preset) => preset.name.trim() === name,
          );
          if (
            !existing ||
            (await actionConfirmation.requestConfirmation({
              title: "覆盖预设",
              message: `预设“${name}”已存在，是否覆盖？`,
              confirmLabel: "覆盖",
            })) === null
          ) {
            return false;
          }
          saved = await updateStockScreenPreset(
            existing.presetId,
            name,
            definition,
            existing.revision,
          );
        }
      }
      const index = presets.value.findIndex(
        (preset) => preset.presetId === saved.presetId,
      );
      if (index >= 0) presets.value.splice(index, 1, saved);
      else presets.value.push(saved);
      selectedPresetId.value = saved.presetId;
      presetName.value = saved.name;
      markSavedBaseline();
      emit("presetChange", saved.presetId);
      return true;
    } catch (error) {
      presetError.value = errorMessage(error);
      const fieldIssue = validationErrorFrom(error);
      if (fieldIssue) validationErrors.value = [fieldIssue];
      return false;
    } finally {
      savingPreset.value = false;
    }
  }

  async function removePreset(): Promise<void> {
    const preset = selectedPreset.value;
    if (!preset) return;
    const confirmed = await actionConfirmation.requestConfirmation({
      title: "删除预设",
      message: `删除预设“${preset.name}”？`,
      confirmLabel: "删除",
    });
    if (confirmed === null) return;
    presetError.value = "";
    try {
      await deleteStockScreenPreset(preset.presetId);
      presets.value = presets.value.filter(
        (item) => item.presetId !== preset.presetId,
      );
      selectedPresetId.value = "";
      presetName.value = "";
      clearResults();
      filters.value = [];
      columns.value = catalog.value
        ? defaultColumnsForCatalog(catalog.value)
        : [];
      sorts.value = [];
      markSavedBaseline();
      emit("presetChange", "");
    } catch (error) {
      presetError.value = errorMessage(error);
    }
  }

  function applyNewPreset(): void {
    selectedPresetId.value = "";
    presetName.value = "";
    filters.value = [];
    columns.value = catalog.value
      ? defaultColumnsForCatalog(catalog.value)
      : [];
    sorts.value = [];
    queryError.value = "";
    validationErrors.value = [];
    clearResults();
    markSavedBaseline();
    emit("presetChange", "");
  }

  function newPreset(): void {
    requestDraftAction({ kind: "new" });
  }

  function exportCSV(): void {
    if (!entries.value.length) return;
    const blob = new Blob(
      [stockScreenCSV(entries.value, factorMap.value, displayColumns.value)],
      { type: "text/csv;charset=utf-8" },
    );
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `stock-screen-${queryMarket.value}-${new Date()
      .toISOString()
      .slice(0, 10)}.csv`;
    anchor.click();
    URL.revokeObjectURL(url);
  }

  function selectEntry(entry: StockScreenEntry): void {
    selectedInstrumentId.value = entry.instrumentId ?? entry.stockId;
    emit("select", entry);
  }

  function openEntry(entry: StockScreenEntry): void {
    emit("open", entry);
  }

  function changeMarket(event: Event): void {
    const market = normalizeScreenMarket(
      (event.target as HTMLSelectElement).value,
    );
    if (market === queryMarket.value) return;
    applyMarket(market);
  }

  function applyMarket(market: "HK" | "US" | "SH" | "SZ"): void {
    queryMarket.value = market;
    catalog.value = null;
    clearResults();
    validationErrors.value = [];
    emit("contextChange", { market, brokerId: screenBrokerId.value });
    void loadCatalogAndPresets();
  }

  watch(
    () => [props.market, props.brokerId] as const,
    ([market]) => {
      const normalizedMarket = normalizeScreenMarket(market);
      const nextContextKey = `${normalizedMarket}|${props.brokerId.trim() || "futu"}`;
      if (nextContextKey === loadedContextKey) return;
      queryMarket.value = normalizedMarket;
      catalog.value = null;
      clearResults();
      validationErrors.value = [];
      void loadCatalogAndPresets();
    },
  );

  watch(currentFingerprint, () => {
    if (!validationErrors.value.length || !catalog.value) return;
    validationErrors.value = validateStockScreenQuery(
      currentDraft(),
      catalog.value,
    );
    if (
      !validationErrors.value.length &&
      queryError.value === "请先修正标红字段后再执行"
    ) {
      queryError.value = "";
    }
    if (
      !validationErrors.value.length &&
      presetError.value === "请先修正标红字段后再保存"
    ) {
      presetError.value = "";
    }
  });

  watch(
    () => props.initialPresetId,
    (presetId) => {
      if (!presetId || presetId === selectedPresetId.value) return;
      const preset = presets.value.find(
        (item) => item.presetId === presetId,
      );
      if (preset) applyPreset(preset);
    },
  );

  onMounted(() => {
    void loadCatalogAndPresets();
  });

  onUnmounted(() => {
    if (retryTimer) clearInterval(retryTimer);
  });

  return {
    catalog,
    presets,
    catalogLoading,
    catalogError,
    presetError,
    queryError,
    loading,
    loadingMore,
    savingPreset,
    factorDialogOpen,
    catalogSearch,
    activeCategory,
    activeFactorRole,
    addFactorButton,
    factorSearchInput,
    categoryScroller,
    canScrollCategoriesLeft,
    canScrollCategoriesRight,
    queryMarket,
    filters,
    columns,
    sorts,
    entries,
    nextOffset,
    hasMore,
    total,
    asOf,
    warnings,
    partialErrors,
    executedColumns,
    resultColumns,
    lastExecutedFingerprint,
    savedFingerprint,
    baselineInitialized,
    validationErrors,
    retryAfterMs,
    selectedPresetId,
    presetName,
    selectedInstrumentId,
    mobilePane,
    screenerOuterPaneSizes,
    screenerInnerPaneSizes,
    screenerOuterPaneMinSizes,
    screenerInnerPaneMinSizes,
    pendingDraftAction,
    actionConfirmation,
    factorMap,
    commonFactors,
    retrievableFactors,
    sortableFactors,
    visibleCatalogFactors,
    selectedPreset,
    screenBrokerId,
    resultLabel,
    queryFingerprint,
    currentFingerprint,
    draftDirty,
    resultStale,
    screenStatus,
    screenStatusLabel,
    pendingDraftActionLabel,
    displayColumns,
    handleScreenerOuterPaneResized,
    handleScreenerInnerPaneResized,
    fieldErrorWithin,
    factorFor,
    columnExists,
    hasDuplicateRef,
    columnIdentity,
    sortIdentity,
    clearResults,
    setRetryCountdown,
    enumOptionsForFactor,
    addFilter,
    openFactorDialog,
    closeFactorDialog,
    updateCategoryScrollState,
    scrollCategories,
    removeFilter,
    addColumn,
    removeColumn,
    moveColumn,
    addSort,
    sortFactorInput,
    boundaryInput,
    valuesInput,
    singleValueInput,
    useSetFilter,
    useIntervalFilter,
    secondFactorInput,
    currentDraft,
    applyPreset,
    choosePreset,
    choosePresetFromSidebar,
    requestDraftAction,
    discardPendingDraft,
    savePendingDraft,
    loadCatalogAndPresets,
    execute,
    savePreset,
    removePreset,
    newPreset,
    exportCSV,
    selectEntry,
    openEntry,
    changeMarket,
    applyMarket,
  };
}

export type StockScreenerController = ReturnType<
  typeof useStockScreenerController
>;
