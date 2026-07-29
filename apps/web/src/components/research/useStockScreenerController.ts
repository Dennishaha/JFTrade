import type { SplitpanesResizedPayload } from "splitpanes";
import {
  computed,
  inject,
  nextTick,
  onMounted,
  onUnmounted,
  provide,
  ref,
  watch,
  type InjectionKey,
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
import {
  createStockScreenFilter,
  factorEnumName,
  factorRefKey,
  moveItem,
  normalizeScreenMarket,
  sameStockScreenFactorRef,
  stockScreenCSV,
  stockScreenDraftFromDefinitionV2,
  stockScreenFactorInstanceId,
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
  StockScreenFactor,
  StockScreenFactorRef,
  StockScreenPreset,
  StockScreenSort,
} from "./stockScreenTypes";

export interface StockScreenerControllerProps {
  market: string;
  brokerId: string;
  initialPresetId: string;
  active: boolean;
}

export interface StockScreenerControllerEmit {
  (event: "select", entry: StockScreenEntry): void;
  (event: "open", entry: StockScreenEntry): void;
  (event: "presetChange", presetId: string): void;
  (
    event: "contextChange",
    context: { market: string; brokerId?: string },
  ): void;
}

type PendingDraftAction =
  | { kind: "preset"; preset: StockScreenPreset }
  | { kind: "new" };

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
  const factorDialogOpen = ref(false);
  const catalogSearch = ref("");
  const activeCategory = ref("");
  const activeFactorRole = ref<"filter" | "column" | "sort">("filter");
  const addFactorButton = ref<HTMLButtonElement | null>(null);
  const factorSearchInput = ref<HTMLInputElement | null>(null);
  const categoryScroller = ref<HTMLDivElement | null>(null);
  const canScrollCategoriesLeft = ref(false);
  const canScrollCategoriesRight = ref(false);
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
  const screenerOuterPaneSizes = ref<[number, number]>([18, 82]);
  const screenerInnerPaneSizes = ref<[number, number]>([39, 61]);
  const screenerOuterPaneMinSizes: [number, number] = [12, 70];
  const screenerInnerPaneMinSizes: [number, number] = [28, 45];
  const pendingDraftAction = ref<PendingDraftAction | null>(null);
  let retryTimer: ReturnType<typeof setInterval> | undefined;
  let filterSerial = 0;
  let catalogToken = 0;
  let queryToken = 0;
  let initialPresetLoaded = "";
  let loadedContextKey = "";
  let categoryResizeObserver: ResizeObserver | null = null;

  function resizedPanePair(
    payload: SplitpanesResizedPayload,
  ): [number, number] | null {
    const sizes = payload.panes?.map((pane) => pane.size);
    if (
      sizes == null ||
      sizes.length !== 2 ||
      !sizes.every((size) => Number.isFinite(size) && size > 0 && size <= 100)
    ) {
      return null;
    }
    return [sizes[0]!, sizes[1]!];
  }

  function handleScreenerOuterPaneResized(
    payload: SplitpanesResizedPayload,
  ): void {
    const sizes = resizedPanePair(payload);
    if (sizes) screenerOuterPaneSizes.value = sizes;
  }

  function handleScreenerInnerPaneResized(
    payload: SplitpanesResizedPayload,
  ): void {
    const sizes = resizedPanePair(payload);
    if (sizes) screenerInnerPaneSizes.value = sizes;
  }

  const factorMap = computed(
    () =>
      new Map(
        (catalog.value?.factors ?? []).map((factor) => [factor.key, factor]),
      ),
  );
  const commonFactors = computed(() =>
    (catalog.value?.factors ?? []).filter(
      (factor) =>
        [
          "simple.price",
          "simple.market_cap",
          "simple.pe_ttm",
          "simple.pb",
        ].includes(factor.key) &&
        factor.filter &&
        factor.availability !== "unsupported",
    ),
  );
  const retrievableFactors = computed(() =>
    (catalog.value?.factors ?? []).filter(
      (factor) => factor.retrieve && factor.availability !== "unsupported",
    ),
  );
  const sortableFactors = computed(() =>
    (catalog.value?.factors ?? []).filter(
      (factor) => factor.sort && factor.availability !== "unsupported",
    ),
  );
  const visibleCatalogFactors = computed(() => {
    const keyword = catalogSearch.value.trim().toLocaleLowerCase();
    return (catalog.value?.factors ?? []).filter((factor) => {
      const roleSupported =
        activeFactorRole.value === "filter"
          ? factor.filter
          : activeFactorRole.value === "column"
            ? factor.retrieve
            : factor.sort;
      if (!roleSupported && factor.availability !== "unsupported") {
        return false;
      }
      if (
        !keyword &&
        activeCategory.value &&
        factor.category !== activeCategory.value
      ) {
        return false;
      }
      if (!keyword) return true;
      return `${factor.label} ${factor.key} ${factor.help ?? ""} ${(factor.searchKeywords ?? []).join(" ")} ${factor.reason ?? ""}`
        .toLocaleLowerCase()
        .includes(keyword);
    });
  });
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
  const screenStatus = computed(() => {
    if (loading.value) return "running";
    if (queryError.value || validationErrors.value.length) return "error";
    if (resultStale.value) return "待更新";
    if (draftDirty.value) return "有未保存修改";
    if (selectedPresetId.value) return "已保存";
    return "未保存";
  });
  const screenStatusLabel = computed(() => {
    switch (screenStatus.value) {
      case "running":
        return "执行中";
      case "error":
        return "需要修正";
      case "待更新":
        return "结果待更新";
      case "有未保存修改":
        return "有未保存修改";
      case "已保存":
        return "已保存";
      default:
        return "未保存";
    }
  });
  const pendingDraftActionLabel = computed(() => {
    const action = pendingDraftAction.value;
    if (!action) return "";
    switch (action.kind) {
      case "preset":
        return `切换到“${action.preset.name}”`;
      case "new":
        return "新建策略";
    }
  });
  const displayColumns = computed(() =>
    entries.value.length && executedColumns.value.length
      ? executedColumns.value
      : columns.value,
  );
  const fieldErrorWithin = (path: string): string =>
    validationErrors.value.find(
      (error) => error.path === path || error.path.startsWith(`${path}.`),
    )?.message ?? "";

  function errorMessage(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }

  function validationErrorFrom(
    error: unknown,
  ): { path: string; message: string } | null {
    const message = errorMessage(error);
    const match = message.match(
      /^((?:conditions|columns|sorts)\[\d+\](?:\.[A-Za-z][A-Za-z0-9]*)+):\s*(.+)$/,
    );
    if (!match) return null;
    const path = match[1]!
      .replaceAll(/\[(\d+)\]/g, ".$1")
      .replace(".factor.params.", ".params.")
      .replace(".factor.factorKey", ".factor")
      .replace(".secondFactor.factorKey", ".secondFactor");
    return { path, message: match[2]! };
  }

  function factorFor(key: string): StockScreenFactor | undefined {
    return factorMap.value.get(key);
  }

  function columnExists(key: string): boolean {
    const factor = factorFor(key);
    if (!factor || !catalog.value) return false;
    const params = createStockScreenFilter(
      factor,
      0,
      catalog.value,
      queryMarket.value,
    ).params;
    return hasDuplicateRef(columns.value, {
      factor: key,
      ...(params ? { params } : {}),
    });
  }

  function hasDuplicateRef(
    refs: StockScreenFactorRef[],
    candidate: StockScreenFactorRef,
  ): boolean {
    return refs.some((reference) =>
      sameStockScreenFactorRef(reference, candidate),
    );
  }

  function defaultColumnsForCatalog(
    nextCatalog: StockScreenCatalog,
  ): StockScreenColumn[] {
    return nextCatalog.factors
      .filter(
        (factor) =>
          [
            "basic.code",
            "basic.name",
            "simple.price",
            "simple.market_cap",
          ].includes(factor.key) &&
          factor.retrieve &&
          factor.availability !== "unsupported",
      )
      .map((factor, index) => ({
        factor: factor.key,
        factorKey: factor.key,
        instanceId: `default-${factor.key}`,
        columnId: `column-${factor.key}-${index}`,
      }));
  }

  function columnIdentity(column: StockScreenColumn, index: number): string {
    return (
      column.columnId ??
      stockScreenFactorInstanceId(
        column,
        `${factorRefKey(column)}-${index}`,
      )
    );
  }

  function sortIdentity(sort: StockScreenSort, index: number): string {
    return (
      sort.sortId ??
      stockScreenFactorInstanceId(sort, `${factorRefKey(sort)}-${index}`)
    );
  }

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

  function enumOptionsForFactor(factor: StockScreenFactor | undefined) {
    if (!factor || !catalog.value) return [];
    const name = factorEnumName(factor);
    return name ? (catalog.value.enums[name] ?? []) : [];
  }

  async function addFilter(factor: StockScreenFactor): Promise<void> {
    if (
      !factor.filter ||
      factor.availability === "unsupported" ||
      !catalog.value
    ) {
      return;
    }
    const serial = ++filterSerial;
    const instanceId = `${factor.key}-${serial}`;
    const nextFilter = createStockScreenFilter(
      factor,
      serial,
      catalog.value,
      queryMarket.value,
      instanceId,
    );
    if (hasDuplicateRef(filters.value, nextFilter)) {
      queryError.value = `已存在相同参数的“${factor.label}”条件`;
      return;
    }
    filters.value.push(nextFilter);
    mobilePane.value = "builder";
    queryError.value = "";
    factorDialogOpen.value = false;
    await nextTick();
    const row = Array.from(
      document.querySelectorAll<HTMLElement>("[data-filter-id]"),
    ).find((candidate) => candidate.dataset.filterId === nextFilter.id);
    row?.querySelector<HTMLElement>("input, select")?.focus();
  }

  async function openFactorDialog(): Promise<void> {
    factorDialogOpen.value = true;
    await nextTick();
    observeCategoryScroller();
    factorSearchInput.value?.focus();
  }

  async function closeFactorDialog(): Promise<void> {
    factorDialogOpen.value = false;
    await nextTick();
    addFactorButton.value?.focus();
  }

  function updateCategoryScrollState(): void {
    const scroller = categoryScroller.value;
    if (!scroller) {
      canScrollCategoriesLeft.value = false;
      canScrollCategoriesRight.value = false;
      return;
    }
    const maxScrollLeft = Math.max(
      0,
      scroller.scrollWidth - scroller.clientWidth,
    );
    canScrollCategoriesLeft.value = scroller.scrollLeft > 1;
    canScrollCategoriesRight.value = scroller.scrollLeft < maxScrollLeft - 1;
  }

  function observeCategoryScroller(): void {
    categoryResizeObserver?.disconnect();
    categoryResizeObserver = null;
    const scroller = categoryScroller.value;
    if (scroller && typeof ResizeObserver !== "undefined") {
      categoryResizeObserver = new ResizeObserver(updateCategoryScrollState);
      categoryResizeObserver.observe(scroller);
    }
    updateCategoryScrollState();
  }

  function scrollCategories(direction: -1 | 1): void {
    const scroller = categoryScroller.value;
    if (!scroller) return;
    const distance = direction * Math.max(120, scroller.clientWidth * 0.75);
    if (typeof scroller.scrollBy === "function") {
      scroller.scrollBy({ left: distance, behavior: "smooth" });
      return;
    }
    scroller.scrollLeft += distance;
    updateCategoryScrollState();
  }

  function removeFilter(id: string): void {
    filters.value = filters.value.filter((filter) => filter.id !== id);
  }

  async function addColumn(key: string): Promise<void> {
    const factor = factorFor(key);
    if (!factor || !catalog.value) return;
    const params = createStockScreenFilter(
      factor,
      0,
      catalog.value,
      queryMarket.value,
    ).params;
    const nextColumn: StockScreenColumn = {
      factor: key,
      factorKey: key,
      instanceId: `column-${key}-${++filterSerial}`,
      ...(params ? { params } : {}),
      columnId: `column-${key}-${filterSerial}`,
    };
    if (hasDuplicateRef(columns.value, nextColumn)) {
      queryError.value = `已存在相同参数的“${factor.label}”结果列`;
      return;
    }
    columns.value.push(nextColumn);
    queryError.value = "";
    factorDialogOpen.value = false;
    await nextTick();
    const identity = columnIdentity(nextColumn, columns.value.length - 1);
    const row = Array.from(
      document.querySelectorAll<HTMLElement>("[data-column-id]"),
    ).find((candidate) => candidate.dataset.columnId === identity);
    row?.querySelector<HTMLElement>("input, select")?.focus();
  }

  function removeColumn(column: StockScreenColumn): void {
    columns.value = columns.value.filter((item) => item !== column);
  }

  function moveColumn(index: number, delta: number): void {
    columns.value = moveItem(columns.value, index, delta);
  }

  async function addSort(preferredKey?: string): Promise<void> {
    if (!catalog.value) return;
    const candidates = preferredKey
      ? sortableFactors.value.filter(
          (candidate) => candidate.key === preferredKey,
        )
      : sortableFactors.value;
    const factor = candidates.find((candidate) => {
      const params = createStockScreenFilter(
        candidate,
        0,
        catalog.value!,
        queryMarket.value,
      ).params;
      return !hasDuplicateRef(sorts.value, {
        factor: candidate.key,
        ...(params ? { params } : {}),
      });
    });
    if (!factor) return;
    const params = createStockScreenFilter(
      factor,
      0,
      catalog.value,
      queryMarket.value,
    ).params;
    const nextSort: StockScreenSort = {
      factor: factor.key,
      factorKey: factor.key,
      instanceId: `sort-${factor.key}-${++filterSerial}`,
      direction: "desc",
      ...(params ? { params } : {}),
      sortId: `sort-${factor.key}-${filterSerial}`,
    };
    if (hasDuplicateRef(sorts.value, nextSort)) return;
    sorts.value.push(nextSort);
    factorDialogOpen.value = false;
    await nextTick();
    const identity = sortIdentity(nextSort, sorts.value.length - 1);
    const row = Array.from(
      document.querySelectorAll<HTMLElement>("[data-sort-id]"),
    ).find((candidate) => candidate.dataset.sortId === identity);
    row?.querySelector<HTMLElement>("input, select")?.focus();
  }

  function sortFactorInput(sort: StockScreenSort, event: Event): void {
    const key = (event.target as HTMLSelectElement).value;
    const factor = factorFor(key);
    if (!factor || !catalog.value) return;
    const params = createStockScreenFilter(
      factor,
      0,
      catalog.value,
      queryMarket.value,
    ).params;
    sort.factor = key;
    sort.factorKey = key;
    sort.instanceId = `sort-${key}-${++filterSerial}`;
    if (params) sort.params = params;
    else delete sort.params;
    sort.sortId ??= `sort-${key}-${filterSerial}`;
  }

  function boundaryInput(
    filter: StockScreenEditorFilter,
    event: Event,
    field: "min" | "max",
  ): void {
    const raw = (event.target as HTMLInputElement).value;
    if (raw === "") delete filter[field];
    else filter[field] = { value: Number(raw), includes: true };
  }

  function valuesInput(
    filter: StockScreenEditorFilter,
    event: Event,
  ): void {
    const raw = (event.target as HTMLInputElement).value;
    filter.values = raw
      .split(",")
      .map((value) => Number(value.trim()))
      .filter(Number.isFinite);
  }

  function singleValueInput(
    filter: StockScreenEditorFilter,
    event: Event,
  ): void {
    filter.values = [Number((event.target as HTMLSelectElement).value)];
  }

  function useSetFilter(filter: StockScreenEditorFilter): void {
    delete filter.min;
    delete filter.max;
    delete filter.intervals;
    filter.values = [0];
  }

  function useIntervalFilter(filter: StockScreenEditorFilter): void {
    delete filter.values;
    delete filter.intervals;
  }

  function secondFactorInput(
    filter: StockScreenEditorFilter,
    event: Event,
  ): void {
    const factorKey = (event.target as HTMLSelectElement).value;
    if (!factorKey) {
      delete filter.secondFactor;
      return;
    }
    const factor = factorFor(factorKey);
    if (!factor || !catalog.value) return;
    const params = createStockScreenFilter(
      factor,
      0,
      catalog.value,
      queryMarket.value,
    ).params;
    filter.secondFactor = {
      factor: factorKey,
      instanceId: `second-${factorKey}-${++filterSerial}`,
      factorKey,
      ...(params ? { params } : {}),
    };
    delete filter.secondValue;
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
      id: filter.conditionId ?? `${filter.factor}-${++filterSerial}`,
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
            !window.confirm(`预设“${name}”已存在，是否覆盖？`)
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
    if (!window.confirm(`删除预设“${preset.name}”？`)) return;
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

  watch(factorDialogOpen, (open) => {
    if (open) return;
    categoryResizeObserver?.disconnect();
    categoryResizeObserver = null;
    canScrollCategoriesLeft.value = false;
    canScrollCategoriesRight.value = false;
  });

  watch(
    () => catalog.value?.categories.length ?? 0,
    async () => {
      if (!factorDialogOpen.value) return;
      await nextTick();
      updateCategoryScrollState();
    },
  );

  onMounted(() => {
    void loadCatalogAndPresets();
  });

  onUnmounted(() => {
    if (retryTimer) clearInterval(retryTimer);
    categoryResizeObserver?.disconnect();
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

const stockScreenerControllerKey: InjectionKey<StockScreenerController> =
  Symbol("stock-screener-controller");

export function provideStockScreenerController(
  controller: StockScreenerController,
): void {
  provide(stockScreenerControllerKey, controller);
}

export function useStockScreenerControllerContext(): StockScreenerController {
  const controller = inject(stockScreenerControllerKey);
  if (!controller) {
    throw new Error("Stock screener controller is not available");
  }
  return controller;
}
