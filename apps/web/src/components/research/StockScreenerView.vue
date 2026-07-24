<script setup lang="ts">
import type { SplitpanesResizedPayload } from "splitpanes";
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";

import SplitPane from "../shared/SplitPane.vue";
import SplitPaneItem from "../shared/SplitPaneItem.vue";
import StockScreenParameterEditor from "./StockScreenParameterEditor.vue";
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
  formatStockScreenValue,
  factorRefKey,
  moveItem,
  normalizeScreenMarket,
  resultColumnFor,
  sameStockScreenFactorRef,
  stockScreenDraftFromDefinitionV2,
  stockScreenFactorInstanceId,
  stockScreenCSV,
  stockScreenValueTitle,
  stockScreenQueryFingerprint,
  toStockScreenDraftFilter,
  toStockScreenDefinitionV2,
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

const props = withDefaults(
  defineProps<{
    market: string;
    brokerId?: string;
    initialPresetId?: string;
    active?: boolean;
  }>(),
  {
    brokerId: "",
    initialPresetId: "",
    active: true,
  },
);

const emit = defineEmits<{
  select: [entry: StockScreenEntry];
  open: [entry: StockScreenEntry];
  presetChange: [presetId: string];
  contextChange: [context: { market: string; brokerId?: string }];
}>();

const PAGE_SIZE = 50;
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
const partialErrors = ref<Array<{ code?: string; message?: string;[key: string]: unknown }>>([]);
const executedColumns = ref<StockScreenColumn[]>([]);
const resultColumns = ref<Array<{ columnId: string; instanceId?: string; factorKey: string; label?: string }>>([]);
const lastExecutedFingerprint = ref("");
const savedFingerprint = ref("");
const baselineInitialized = ref(false);
const validationErrors = ref<Array<{ path: string; message: string }>>([]);
const retryAfterMs = ref(0);
let retryTimer: ReturnType<typeof setInterval> | undefined;
const selectedPresetId = ref("");
const presetName = ref("");
const selectedInstrumentId = ref("");
const mobilePane = ref<"builder" | "results">("builder");
const screenerOuterPaneSizes = ref<[number, number]>([18, 82]);
const screenerInnerPaneSizes = ref<[number, number]>([39, 61]);
const screenerOuterPaneMinSizes: [number, number] = [12, 70];
const screenerInnerPaneMinSizes: [number, number] = [28, 45];
type PendingDraftAction =
  | { kind: "preset"; preset: StockScreenPreset }
  | { kind: "new" };
const pendingDraftAction = ref<PendingDraftAction | null>(null);
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

function handleScreenerOuterPaneResized(payload: SplitpanesResizedPayload): void {
  const sizes = resizedPanePair(payload);
  if (sizes) screenerOuterPaneSizes.value = sizes;
}

function handleScreenerInnerPaneResized(payload: SplitpanesResizedPayload): void {
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
      ["simple.price", "simple.market_cap", "simple.pe_ttm", "simple.pb"].includes(
        factor.key,
      ) &&
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
    if (!roleSupported && factor.availability !== "unsupported") return false;
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
  () => baselineInitialized.value && currentFingerprint.value !== savedFingerprint.value,
);
const resultStale = computed(
  () => entries.value.length > 0 && Boolean(lastExecutedFingerprint.value) && lastExecutedFingerprint.value !== queryFingerprint.value,
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

function validationErrorFrom(error: unknown): { path: string; message: string } | null {
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
  return {
    path,
    message: match[2]!,
  };
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
  return refs.some((ref) => sameStockScreenFactorRef(ref, candidate));
}

function defaultColumnsForCatalog(nextCatalog: StockScreenCatalog): StockScreenColumn[] {
  return nextCatalog.factors
    .filter(
      (factor) =>
        ["basic.code", "basic.name", "simple.price", "simple.market_cap"].includes(
          factor.key,
        ) &&
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
  return column.columnId ?? stockScreenFactorInstanceId(column, `${factorRefKey(column)}-${index}`);
}

function sortIdentity(sort: StockScreenSort, index: number): string {
  return sort.sortId ?? stockScreenFactorInstanceId(sort, `${factorRefKey(sort)}-${index}`);
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
  if (!factor.filter || factor.availability === "unsupported" || !catalog.value)
    return;
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
  const maxScrollLeft = Math.max(0, scroller.scrollWidth - scroller.clientWidth);
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
    ? sortableFactors.value.filter((candidate) => candidate.key === preferredKey)
    : sortableFactors.value;
  const factor = candidates.find((candidate) => {
    const params = createStockScreenFilter(candidate, 0, catalog.value!, queryMarket.value).params;
    return !hasDuplicateRef(sorts.value, { factor: candidate.key, ...(params ? { params } : {}) });
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
  const params = createStockScreenFilter(factor, 0, catalog.value, queryMarket.value).params;
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

function valuesInput(filter: StockScreenEditorFilter, event: Event): void {
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
  const id = (event.target as HTMLSelectElement).value;
  if (!id) {
    requestDraftAction({ kind: "new" });
    void nextTick(() => {
      (event.target as HTMLSelectElement).value = selectedPresetId.value;
    });
    return;
  }
  const preset = presets.value.find((item) => item.presetId === id);
  if (preset) {
    requestDraftAction({ kind: "preset", preset });
    void nextTick(() => {
      (event.target as HTMLSelectElement).value = selectedPresetId.value;
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
    if (props.initialPresetId && initialPresetLoaded !== props.initialPresetId) {
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
    const definition = toStockScreenDefinitionV2(draft, catalog.value.version);
    const result = await runStockScreen({
      ...definition,
      page: { offset, limit: PAGE_SIZE },
    });
    if (token !== queryToken) return;
    entries.value = append ? [...entries.value, ...result.entries] : result.entries;
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
    const definition = toStockScreenDefinitionV2(draft, catalog.value.version);
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
    columns.value = catalog.value ? defaultColumnsForCatalog(catalog.value) : [];
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
  columns.value = catalog.value ? defaultColumnsForCatalog(catalog.value) : [];
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
  emit("contextChange", {
    market,
    brokerId: screenBrokerId.value,
  });
  void loadCatalogAndPresets();
}

watch(
  () => [props.market, props.brokerId] as const,
  ([market]) => {
    const normalizedMarket = normalizeScreenMarket(market);
    const nextContextKey = `${normalizedMarket}|${props.brokerId.trim() || "futu"}`;
    if (nextContextKey === loadedContextKey) {
      return;
    }
    queryMarket.value = normalizedMarket;
    catalog.value = null;
    clearResults();
    validationErrors.value = [];
    void loadCatalogAndPresets();
  },
);

watch(currentFingerprint, () => {
  if (!validationErrors.value.length || !catalog.value) return;
  validationErrors.value = validateStockScreenQuery(currentDraft(), catalog.value);
  if (!validationErrors.value.length && queryError.value === "请先修正标红字段后再执行") {
    queryError.value = "";
  }
  if (!validationErrors.value.length && presetError.value === "请先修正标红字段后再保存") {
    presetError.value = "";
  }
});

watch(
  () => props.initialPresetId,
  (presetId) => {
    if (!presetId || presetId === selectedPresetId.value) return;
    const preset = presets.value.find((item) => item.presetId === presetId);
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
</script>

<template>
  <section class="stock-screener-view">
    <header class="stock-screener-view__toolbar">
      <div class="stock-screener-view__title">
        <strong>股票筛选</strong>
        <select :value="queryMarket" aria-label="筛选市场" @change="changeMarket">
          <option value="US">美股</option>
          <option value="HK">港股</option>
          <option value="SH">沪市</option>
          <option value="SZ">深市</option>
        </select>
      </div>
      <span class="stock-screener-view__scope">范围：全市场</span>
      <label>
        <span>预设</span>
        <select class="stock-screener-view__preset-select" :value="selectedPresetId" @change="choosePreset">
          <option value="">未保存</option>
          <option v-for="preset in presets" :key="preset.presetId" :value="preset.presetId">
            {{ preset.name }}
          </option>
        </select>
      </label>
      <input v-model="presetName" class="stock-screener-view__preset-name" aria-label="预设名称" placeholder="预设名称" />
      <button type="button" @click="newPreset">新建</button>
      <button type="button" :disabled="!presetName.trim() || savingPreset" @click="savePreset">
        {{ savingPreset ? "保存中…" : "保存" }}
      </button>
      <button type="button" :disabled="!selectedPreset" @click="removePreset">
        删除
      </button>
      <span class="stock-screener-view__status" :class="`is-${screenStatus}`" role="status">
        {{ screenStatusLabel }}
      </span>
      <span class="stock-screener-view__spacer" />
      <button type="button" :disabled="!entries.length" @click="exportCSV">
        导出 CSV
      </button>
      <button class="stock-screener-view__run" type="button" :disabled="loading || catalogLoading || retryAfterMs > 0"
        @click="execute(0, false)">
        {{ loading ? "筛选中…" : retryAfterMs > 0 ? `稍后重试 (${Math.ceil(retryAfterMs / 1000)}s)` : "执行筛选" }}
      </button>
    </header>

    <div v-if="catalogError || presetError || queryError"
      class="stock-screener-view__notice tv-status--error tv-status-surface">
      {{ catalogError || presetError || queryError }}
    </div>
    <div v-for="warning in warnings" :key="warning"
      class="stock-screener-view__notice tv-status--warning tv-status-surface">
      {{ warning }}
    </div>
    <div v-for="(partialError, index) in partialErrors" :key="`${partialError.code ?? 'partial'}-${index}`"
      class="stock-screener-view__notice tv-status--warning tv-status-surface">
      {{ partialError.message || partialError.code || "部分结果不可用" }}
    </div>

    <div class="stock-screener-view__mobile-tabs" role="tablist" aria-label="选股器页面">
      <button type="button" role="tab" :aria-selected="mobilePane === 'builder'"
        :class="{ 'is-active': mobilePane === 'builder' }" @click="mobilePane = 'builder'">
        条件
      </button>
      <button type="button" role="tab" :aria-selected="mobilePane === 'results'"
        :class="{ 'is-active': mobilePane === 'results' }" @click="mobilePane = 'results'">
        结果 {{ entries.length }}
      </button>
    </div>

    <SplitPane class="stock-screener-view__workspace" :pane-min-size="10" :push-other-panes="false"
      @resized="handleScreenerOuterPaneResized">
      <SplitPaneItem :size="screenerOuterPaneSizes[0]" :min-size="screenerOuterPaneMinSizes[0]" :max-size="30">
        <aside class="stock-screener-view__preset-sidebar" aria-label="筛选策略">
        <div class="stock-screener-view__preset-sidebar-head">
          <strong>我的策略</strong>
          <span>{{ presets.length }}</span>
        </div>
        <button type="button" class="stock-screener-view__new-preset" @click="newPreset">
          ＋ 新建策略
        </button>
        <div v-if="presets.length" class="stock-screener-view__preset-list">
          <button v-for="preset in presets" :key="preset.presetId" type="button"
            :class="{ 'is-active': selectedPresetId === preset.presetId }" @click="choosePresetFromSidebar(preset)">
            <span>{{ preset.name }}</span>
          </button>
        </div>
        <div v-else class="stock-screener-view__preset-empty">
          还没有保存策略
        </div>
        </aside>
      </SplitPaneItem>

      <SplitPaneItem :size="screenerOuterPaneSizes[1]" :min-size="screenerOuterPaneMinSizes[1]" :max-size="88">
        <SplitPane class="stock-screener-view__layout" :pane-min-size="20" :push-other-panes="false"
          @resized="handleScreenerInnerPaneResized">
          <SplitPaneItem :size="screenerInnerPaneSizes[0]" :min-size="screenerInnerPaneMinSizes[0]" :max-size="55">
        <aside class="stock-screener-view__builder" :class="{ 'is-mobile-hidden': mobilePane !== 'builder' }">
          <div class="stock-screener-view__panel-head">
            <strong>筛选条件</strong>
            <span>{{ filters.length }}</span>
            <button ref="addFactorButton" type="button" class="stock-screener-view__add-factor" aria-haspopup="dialog"
              :aria-expanded="factorDialogOpen" @click="openFactorDialog">
              添加因子
            </button>
          </div>
          <div style="border-bottom: 1px solid var(--tv-border);"></div>

          <div v-if="commonFactors.length" class="stock-screener-view__common">
            <span>常用</span>
            <button v-for="factor in commonFactors" :key="factor.key" type="button"
              :disabled="factor.availability === 'unsupported' || hasDuplicateRef(filters, { factor: factor.key })"
              :title="factor.reason || (hasDuplicateRef(filters, { factor: factor.key }) ? '已存在相同参数' : undefined)"
              @click="addFilter(factor)">
              + {{ factor.label }}
            </button>
          </div>

          <div v-if="filters.length === 0" class="stock-screener-view__empty-small">
            添加条件后执行；恢复预设不会自动请求。
          </div>
          <div v-for="filter in filters" :key="filter.id" :data-filter-id="filter.id"
            class="stock-screener-view__condition">
            <div class="stock-screener-view__condition-title">
              <strong>{{ factorFor(filter.factor)?.label ?? filter.factor }}</strong>
              <span v-if="
                factorFor(filter.factor)?.filterKind === 'interval_or_set'
              " class="tv-seg">
                <button type="button" :class="{ 'is-active': filter.values == null }"
                  @click="useIntervalFilter(filter)">
                  区间
                </button>
                <button type="button" :class="{ 'is-active': filter.values != null }" @click="useSetFilter(filter)">
                  集合
                </button>
              </span>
              <button type="button" @click="removeFilter(filter.id)">移除</button>
            </div>
            <div class="stock-screener-view__condition-fields" :class="{
              'stock-screener-view__condition-fields--range':
                factorFor(filter.factor)?.filterKind === 'interval' ||
                (factorFor(filter.factor)?.filterKind === 'interval_or_set' && filter.values == null),
            }">
              <template v-if="
                ['enum', 'set'].includes(
                  factorFor(filter.factor)?.filterKind ?? '',
                ) ||
                (factorFor(filter.factor)?.filterKind === 'interval_or_set' &&
                  filter.values != null)
              ">
                <select v-if="enumOptionsForFactor(factorFor(filter.factor)).length" :value="filter.values?.[0]"
                  aria-label="枚举条件值" @change="singleValueInput(filter, $event)">
                  <option v-for="option in enumOptionsForFactor(
                    factorFor(filter.factor),
                  )" :key="option.key" :value="option.value">
                    {{ option.label }}
                  </option>
                </select>
                <input v-else :value="filter.values?.join(',')" aria-label="集合条件值" placeholder="整数值，逗号分隔"
                  @input="valuesInput(filter, $event)" />
              </template>
              <template v-else-if="factorFor(filter.factor)?.filterKind === 'position'">
                <select v-model.number="filter.position" aria-label="位置关系">
                  <option v-for="option in catalog?.enums.position ?? []" :key="option.key" :value="option.value">
                    {{ option.label }}
                  </option>
                </select>
                <select :value="filter.secondFactor?.factor ?? ''" aria-label="比较指标"
                  @change="secondFactorInput(filter, $event)">
                  <option value="">固定值</option>
                  <option v-for="factor in catalog?.factors.filter(
                    (item) =>
                      item.category === 'indicator' &&
                      item.availability !== 'unsupported',
                  ) ?? []" :key="factor.key" :value="factor.key">
                    {{ factor.label }}
                  </option>
                </select>
                <input v-if="!filter.secondFactor" v-model.number="filter.secondValue" type="number" aria-label="比较值"
                  placeholder="比较值" />
              </template>
              <template v-else-if="factorFor(filter.factor)?.filterKind === 'pattern'">
                <select v-model="filter.match" aria-label="形态匹配">
                  <option :value="true">匹配</option>
                  <option :value="false">不匹配</option>
                </select>
                <input :value="filter.values?.join(',')" aria-label="子形态" placeholder="子形态值，逗号分隔"
                  @input="valuesInput(filter, $event)" />
              </template>
              <template v-else>
                <input type="number" :value="filter.min?.value" aria-label="条件下限" placeholder="最小值"
                  @input="boundaryInput(filter, $event, 'min')" />
                <span>至</span>
                <input type="number" :value="filter.max?.value" aria-label="条件上限" placeholder="最大值"
                  @input="boundaryInput(filter, $event, 'max')" />
              </template>
              <label v-if="
                ['cumulative', 'financial', 'indicator', 'pattern'].includes(
                  factorFor(filter.factor)?.category ?? '',
                )
              ">
                连续
                <input v-model.number="filter.continuousPeriod" type="number" min="0" aria-label="连续周期" />
              </label>
            </div>
            <small v-if="fieldErrorWithin(`conditions.${filters.indexOf(filter)}`)"
              class="stock-screener-view__field-error">
              {{ fieldErrorWithin(`conditions.${filters.indexOf(filter)}`) }}
            </small>
            <div v-if="factorFor(filter.factor)?.parameters?.length" class="stock-screener-view__parameters">
              <StockScreenParameterEditor :reference="filter" :parameters="factorFor(filter.factor)?.parameters ?? []"
                :enums="catalog?.enums ?? {}" :error-prefix="`conditions.${filters.indexOf(filter)}`"
                :validation-errors="validationErrors" />
            </div>
            <div v-if="filter.secondFactor && factorFor(factorRefKey(filter.secondFactor))?.parameters?.length"
              class="stock-screener-view__parameters stock-screener-view__parameters--secondary">
              <StockScreenParameterEditor :reference="filter.secondFactor"
                :parameters="factorFor(factorRefKey(filter.secondFactor))?.parameters ?? []"
                :enums="catalog?.enums ?? {}" label-prefix="比较 "
                :error-prefix="`conditions.${filters.indexOf(filter)}.secondFactor`"
                :validation-errors="validationErrors" />
            </div>
          </div>

          <div class="stock-screener-view__panel-head">
            <strong>结果列</strong>
            <span>{{ columns.length }}</span>
          </div>
          <div class="stock-screener-view__column-picker">
            <div v-for="(column, index) in columns" :key="columnIdentity(column, index)"
              :data-column-id="columnIdentity(column, index)">
              <span>{{ factorFor(factorRefKey(column))?.label ?? factorRefKey(column) }}</span>
              <button type="button" :disabled="index === 0" aria-label="上移结果列" @click="moveColumn(index, -1)">
                ↑
              </button>
              <button type="button" :disabled="index === columns.length - 1" aria-label="下移结果列"
                @click="moveColumn(index, 1)">
                ↓
              </button>
              <button type="button" @click="removeColumn(column)">X</button>
              <div v-if="factorFor(factorRefKey(column))?.parameters?.length"
                class="stock-screener-view__parameters stock-screener-view__parameters--compact">
                <StockScreenParameterEditor :reference="column"
                  :parameters="factorFor(factorRefKey(column))?.parameters ?? []" :enums="catalog?.enums ?? {}"
                  :error-prefix="`columns.${index}`" :validation-errors="validationErrors" compact />
              </div>
              <small v-if="fieldErrorWithin(`columns.${index}`)" class="stock-screener-view__field-error">
                {{ fieldErrorWithin(`columns.${index}`) }}
              </small>
            </div>
            <label>
              <span>添加列</span>
              <select aria-label="添加结果列" @change="
                addColumn(($event.target as HTMLSelectElement).value);
              ($event.target as HTMLSelectElement).value = '';
              ">
                <option value="">选择因子</option>
                <option v-for="factor in retrievableFactors" :key="factor.key" :value="factor.key"
                  :disabled="columnExists(factor.key)">
                  {{ factor.label }}
                </option>
              </select>
            </label>
          </div>

          <div class="stock-screener-view__panel-head">
            <strong>多字段排序</strong>
            <span>{{ sorts.length }}</span>
            <button type="button" @click="addSort()">添加排序</button>
          </div>
          <div style="border-bottom: 1px solid var(--tv-border);"></div>
          <div class="stock-screener-view__sorts">
            <div v-for="(sort, index) in sorts" :key="sortIdentity(sort, index)"
              :data-sort-id="sortIdentity(sort, index)">
              <select :value="factorRefKey(sort)" aria-label="排序字段" @change="sortFactorInput(sort, $event)">
                <option v-for="factor in sortableFactors" :key="factor.key" :value="factor.key">
                  {{ factor.label }}
                </option>
              </select>
              <select v-model="sort.direction" aria-label="排序方向">
                <option value="desc">降序</option>
                <option value="asc">升序</option>
                <option value="abs_desc">绝对值降序</option>
                <option value="abs_asc">绝对值升序</option>
              </select>
              <button type="button" @click="sorts.splice(index, 1)">×</button>
              <div v-if="factorFor(factorRefKey(sort))?.parameters?.length"
                class="stock-screener-view__parameters stock-screener-view__parameters--compact">
                <StockScreenParameterEditor :reference="sort"
                  :parameters="factorFor(factorRefKey(sort))?.parameters ?? []" :enums="catalog?.enums ?? {}"
                  :error-prefix="`sorts.${index}`" :validation-errors="validationErrors" compact />
              </div>
              <small v-if="fieldErrorWithin(`sorts.${index}`)" class="stock-screener-view__field-error">
                {{ fieldErrorWithin(`sorts.${index}`) }}
              </small>
            </div>
          </div>
        </aside>
      </SplitPaneItem>

          <SplitPaneItem :size="screenerInnerPaneSizes[1]" :min-size="screenerInnerPaneMinSizes[1]" :max-size="72">
        <main class="stock-screener-view__results" :class="{ 'is-mobile-hidden': mobilePane !== 'results' }">
          <div class="stock-screener-view__result-head">
            <strong>筛选结果</strong>
            <span>{{ resultLabel }}</span>
            <span v-if="asOf">数据时间 {{ asOf }}</span>
            <span v-if="resultStale" class="stock-screener-view__result-stale">条件已修改，结果待更新</span>
            <span v-if="catalog?.rateLimit">
              限流 {{ catalog.rateLimit.windowSeconds }} 秒 /
              {{ catalog.rateLimit.requests }} 次
            </span>
          </div>
          <div v-if="loading" class="stock-screener-view__empty">
            正在执行筛选…
          </div>
          <div v-else-if="entries.length === 0" class="stock-screener-view__empty">
            {{
              selectedPreset
                ? "已恢复预设，请手动执行筛选"
                : "配置条件和结果列后手动执行筛选"
            }}
          </div>
          <div v-else class="stock-screener-view__table-wrap">
            <table>
              <thead>
                <tr>
                  <th>代码</th>
                  <th>名称</th>
                  <th v-for="(column, columnIndex) in displayColumns" :key="columnIdentity(column, columnIndex)">
                    {{ factorFor(factorRefKey(column))?.label ?? factorRefKey(column) }}
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="entry in entries" :key="entry.instrumentId ?? entry.stockId" :class="{
                  'is-selected':
                    selectedInstrumentId ===
                    (entry.instrumentId ?? entry.stockId),
                }" tabindex="0" @click="selectEntry(entry)" @dblclick="emit('open', entry)"
                  @keydown.enter="selectEntry(entry)">
                  <td class="tv-num">{{ entry.symbol }}</td>
                  <td>{{ entry.name }}</td>
                  <td v-for="(column, columnIndex) in displayColumns" :key="columnIdentity(column, columnIndex)"
                    class="tv-num" :title="stockScreenValueTitle(
                      resultColumnFor(entry, column, resultColumns),
                      factorFor(factorRefKey(column)),
                      entry,
                    )">
                    {{
                      formatStockScreenValue(
                        resultColumnFor(entry, column, resultColumns),
                        factorFor(factorRefKey(column)),
                        entry,
                      )
                    }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <button v-if="hasMore" class="stock-screener-view__more" type="button" :disabled="loadingMore"
            @click="execute(nextOffset ?? entries.length, true)">
            {{ loadingMore ? "加载中…" : "加载更多" }}
          </button>
        </main>
          </SplitPaneItem>
        </SplitPane>
      </SplitPaneItem>
    </SplitPane>

    <Teleport to="body">
      <div v-if="pendingDraftAction" class="stock-screener-view__factor-dialog-backdrop">
        <section class="stock-screener-view__draft-dialog" role="dialog" aria-modal="true"
          aria-labelledby="stock-screener-draft-dialog-title">
          <header>
            <strong id="stock-screener-draft-dialog-title">当前策略有未保存修改</strong>
            <span>{{ pendingDraftActionLabel }}前，请选择如何处理当前草稿。</span>
          </header>
          <div class="stock-screener-view__draft-dialog-actions">
            <button type="button" :disabled="savingPreset" @click="savePendingDraft">
              {{ savingPreset ? "保存中…" : "保存后继续" }}
            </button>
            <button type="button" @click="discardPendingDraft">放弃修改</button>
            <button type="button" @click="pendingDraftAction = null">取消</button>
          </div>
        </section>
      </div>
    </Teleport>

    <Teleport to="body">
      <div v-if="factorDialogOpen" class="stock-screener-view__factor-dialog-backdrop" @click.self="closeFactorDialog"
        @keydown.esc.stop.prevent="closeFactorDialog">
        <section class="stock-screener-view__factor-dialog" role="dialog" aria-modal="true"
          aria-labelledby="stock-screener-factor-dialog-title">
          <header class="stock-screener-view__factor-dialog-head">
            <div>
              <strong id="stock-screener-factor-dialog-title">添加因子</strong>
              <span>选择因子后会立即插入并定位到编辑行</span>
            </div>
            <button type="button" class="stock-screener-view__factor-dialog-close" aria-label="关闭添加因子"
              @click="closeFactorDialog">
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <path d="m6 6 12 12M18 6 6 18" />
              </svg>
            </button>
          </header>

          <div class="stock-screener-view__catalog" aria-label="因子目录">
            <input ref="factorSearchInput" v-model="catalogSearch" type="search" aria-label="搜索因子"
              placeholder="搜索名称、键或说明">
            <div class="stock-screener-view__factor-roles" role="tablist" aria-label="因子用途">
              <button type="button" role="tab" :aria-selected="activeFactorRole === 'filter'"
                :class="{ 'is-active': activeFactorRole === 'filter' }" @click="activeFactorRole = 'filter'">条件</button>
              <button type="button" role="tab" :aria-selected="activeFactorRole === 'column'"
                :class="{ 'is-active': activeFactorRole === 'column' }"
                @click="activeFactorRole = 'column'">结果列</button>
              <button type="button" role="tab" :aria-selected="activeFactorRole === 'sort'"
                :class="{ 'is-active': activeFactorRole === 'sort' }" @click="activeFactorRole = 'sort'">排序</button>
            </div>
            <div class="stock-screener-view__category-nav">
              <button type="button"
                class="stock-screener-view__category-scroll stock-screener-view__category-scroll--previous"
                aria-label="向左滚动因子分类" :disabled="!canScrollCategoriesLeft" @click="scrollCategories(-1)">
                <svg viewBox="0 0 24 24" aria-hidden="true">
                  <path d="m15 18-6-6 6-6" />
                </svg>
              </button>
              <div ref="categoryScroller" class="stock-screener-view__categories" @scroll="updateCategoryScrollState">
                <button type="button" :class="{ 'is-active': activeCategory === '' }" @click="activeCategory = ''">
                  全部
                </button>
                <button v-for="category in catalog?.categories ?? []" :key="category.key" type="button"
                  :class="{ 'is-active': activeCategory === category.key }" @click="activeCategory = category.key">
                  {{ category.label }} {{ category.count }}
                </button>
              </div>
              <button type="button"
                class="stock-screener-view__category-scroll stock-screener-view__category-scroll--next"
                aria-label="向右滚动因子分类" :disabled="!canScrollCategoriesRight" @click="scrollCategories(1)">
                <svg viewBox="0 0 24 24" aria-hidden="true">
                  <path d="m9 18 6-6-6-6" />
                </svg>
              </button>
            </div>
            <div class="stock-screener-view__factor-list">
              <article v-for="factor in visibleCatalogFactors" :key="factor.key" :class="{
                'is-disabled': factor.availability === 'unsupported',
                'is-experimental': factor.availability === 'experimental',
              }">
                <div>
                  <strong>{{ factor.label }}</strong>
                  <code>{{ factor.key }}</code>
                  <small v-if="factor.availability === 'experimental'">
                    实验
                  </small>
                  <p>
                    {{
                      factor.availability === "unsupported"
                        ? factor.reason || "当前市场不可用"
                        : `${factor.category} · ${factor.filterKind || factor.valueType}`
                    }}
                  </p>
                </div>
                <span>
                  <button type="button" :disabled="!factor.filter || factor.availability === 'unsupported' || hasDuplicateRef(filters, { factor: factor.key })
                    " @click="addFilter(factor)">
                    条件
                  </button>
                  <button type="button" :disabled="!factor.retrieve ||
                    factor.availability === 'unsupported' ||
                    columnExists(factor.key)
                    " @click="addColumn(factor.key)">
                    列
                  </button>
                  <button type="button"
                    :disabled="!factor.sort || factor.availability === 'unsupported' || hasDuplicateRef(sorts, { factor: factor.key })"
                    @click="addSort(factor.key)">
                    排序
                  </button>
                </span>
              </article>
            </div>
          </div>
        </section>
      </div>
    </Teleport>
  </section>
</template>

<style scoped>
.stock-screener-view {
  box-sizing: border-box;
  display: grid;
  width: 100%;
  max-width: 100%;
  container-type: inline-size;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  gap: 8px;
  padding: 8px;
  color: var(--tv-text);
  font-size: 12px;
}

.stock-screener-view button,
.stock-screener-view input,
.stock-screener-view select {
  min-height: 28px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font: inherit;
}

.stock-screener-view button {
  padding: 0 8px;
  cursor: pointer;
}

.stock-screener-view button:hover:not(:disabled) {
  border-color: var(--tv-accent);
  background: var(--tv-bg-elevated);
}

.stock-screener-view button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.stock-screener-view input,
.stock-screener-view select {
  min-width: 0;
  padding: 0 6px;
}

.stock-screener-view__toolbar {
  display: flex;
  min-width: 0;
  min-height: 44px;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.stock-screener-view__toolbar label,
.stock-screener-view__title {
  display: flex;
  align-items: center;
  gap: 6px;
}

.stock-screener-view__title {
  padding-right: 8px;
  border-right: 1px solid var(--tv-border);
}

.stock-screener-view__title span,
.stock-screener-view__toolbar label>span {
  color: var(--tv-text-muted);
  font-size: 11px;
}

.stock-screener-view__preset-select {
  width: 120px;
}

.stock-screener-view__preset-name {
  width: 120px;
}

.stock-screener-view__scope {
  color: var(--tv-text-muted);
  font-size: 11px;
  white-space: nowrap;
}

.stock-screener-view__spacer {
  flex: 1;
}

.stock-screener-view__status {
  display: inline-flex;
  min-height: 22px;
  align-items: center;
  padding: 0 7px;
  border: 1px solid var(--tv-border);
  border-radius: 999px;
  color: var(--tv-text-muted);
  font-size: 11px;
  white-space: nowrap;
}

.stock-screener-view__status.is-已保存 {
  border-color: color-mix(in srgb, #28a879 50%, var(--tv-border));
  color: #28a879;
}

.stock-screener-view__status.is-有未保存修改,
.stock-screener-view__status.is-待更新 {
  border-color: color-mix(in srgb, #d99a2b 60%, var(--tv-border));
  color: #d99a2b;
}

.stock-screener-view__status.is-error {
  border-color: color-mix(in srgb, #d55353 60%, var(--tv-border));
  color: #d55353;
}

.stock-screener-view__run {
  border-color: var(--tv-accent) !important;
  background: color-mix(in srgb, var(--tv-accent) 14%, transparent) !important;
  color: var(--tv-accent) !important;
  font-weight: 600 !important;
}

.stock-screener-view__notice {
  min-height: 32px;
  padding: 7px 8px;
  border: 1px solid;
  border-radius: 6px;
}

.stock-screener-view__mobile-tabs {
  display: none;
}

.stock-screener-view__workspace,
.stock-screener-view__layout {
  width: 100%;
  max-width: 100%;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.stock-screener-view__workspace :deep(.splitpanes__pane) {
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.stock-screener-view__workspace :deep(.splitpanes__splitter) {
  z-index: 3;
}

.stock-screener-view__preset-sidebar {
  box-sizing: border-box;
  display: grid;
  width: 100%;
  max-width: 100%;
  height: 100%;
  align-content: start;
  min-width: 0;
  min-height: 0;
  overflow: auto;
  gap: 8px;
  padding: 8px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.stock-screener-view__preset-sidebar-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: var(--tv-text);
}

.stock-screener-view__preset-sidebar-head span {
  color: var(--tv-text-muted);
  font-size: 11px;
}

.stock-screener-view__new-preset {
  min-height: 34px !important;
  border-color: color-mix(in srgb, var(--tv-accent) 35%, var(--tv-border)) !important;
  background: color-mix(in srgb, var(--tv-accent) 9%, transparent) !important;
  color: var(--tv-accent) !important;
}

.stock-screener-view__preset-list {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.stock-screener-view__preset-list button {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  min-height: 34px;
  border-color: transparent;
  background: transparent;
  text-align: left;
}

.stock-screener-view__preset-list button span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stock-screener-view__preset-list button small {
  color: var(--tv-text-muted);
  font-size: 10px;
}

.stock-screener-view__preset-list button.is-active {
  border-color: var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.stock-screener-view__preset-empty {
  padding: 12px 4px;
  color: var(--tv-text-dim);
  font-size: 11px;
  text-align: center;
}

.stock-screener-view__builder,
.stock-screener-view__results {
  box-sizing: border-box;
  width: 100%;
  max-width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow: auto;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.stock-screener-view__builder {
  display: grid;
  align-content: start;
  gap: 8px;
  padding: 8px;
}

.stock-screener-view__panel-head,
.stock-screener-view__result-head {
  display: flex;
  min-width: 0;
  min-height: 32px;
  align-items: center;
  gap: 8px;
}

.stock-screener-view__panel-head>span,
.stock-screener-view__result-head>span {
  color: var(--tv-text-muted);
  font-size: 11px;
}

.stock-screener-view__panel-head>button {
  margin-left: auto;
}

.stock-screener-view__add-factor {
  display: inline-flex;
  min-height: 28px;
  align-self: center;
  align-items: center;
  justify-content: center;
  line-height: 1;
}

.stock-screener-view__common {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.stock-screener-view__common>span {
  align-self: center;
  margin-right: 4px;
  color: var(--tv-text-muted);
  font-size: 11px;
}

.stock-screener-view__condition {
  display: grid;
  min-width: 0;
  gap: 6px;
  padding: 7px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface-2);
}

.stock-screener-view__field-error {
  display: block;
  margin-top: 4px;
  color: #d55353;
  font-size: 11px;
}

.stock-screener-view__result-stale {
  color: #d99a2b !important;
  font-weight: 600;
}

.stock-screener-view__condition-title {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 4px;
  justify-content: space-between;
}

.stock-screener-view__condition-title>strong {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stock-screener-view__condition-title button {
  min-height: 24px;
  border: 0;
  color: var(--tv-text-muted);
}

.stock-screener-view__condition-fields {
  display: grid;
  min-width: 0;
  grid-template-columns: minmax(0, 0.7fr) minmax(0, 1fr) auto minmax(0, 1fr);
  align-items: center;
  gap: 4px;
}

.stock-screener-view__condition-fields--range {
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr) auto;
}

.stock-screener-view__parameters {
  display: flex;
  min-width: 0;
  max-width: 100%;
  flex-wrap: wrap;
  gap: 6px;
}

.stock-screener-view__parameters> :deep(.stock-screen-parameter-editor) {
  flex: 1 1 100%;
}

.stock-screener-view__parameters label {
  display: grid;
  min-width: 0;
  flex: 1 1 110px;
  gap: 2px;
}

.stock-screener-view__parameters span {
  color: var(--tv-text-muted);
  font-size: 10px;
}

.stock-screener-view__catalog {
  box-sizing: border-box;
  display: flex;
  width: 100%;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
  gap: 6px;
  overflow: hidden;
  padding: 12px;
}

.stock-screener-view__category-nav {
  box-sizing: border-box;
  display: grid;
  width: 100%;
  min-width: 0;
  max-width: 100%;
  height: 32px;
  min-height: 32px;
  flex: 0 0 32px;
  grid-template-columns: 28px minmax(0, 1fr) 28px;
  align-items: center;
  gap: 4px;
}

.stock-screener-view__categories {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 4px;
  overflow-x: auto;
  overflow-y: hidden;
  overscroll-behavior-inline: contain;
  scrollbar-width: none;
  scroll-behavior: smooth;
  white-space: nowrap;
}

.stock-screener-view__categories::-webkit-scrollbar {
  display: none;
}

.stock-screener-view__category-scroll {
  display: inline-grid;
  width: 28px;
  min-height: 28px !important;
  place-items: center;
  padding: 0 !important;
}

.stock-screener-view__category-scroll svg {
  width: 16px;
  fill: none;
  stroke: currentColor;
  stroke-linecap: round;
  stroke-linejoin: round;
  stroke-width: 2;
}

.stock-screener-view__factor-roles {
  display: flex;
  gap: 4px;
}

.stock-screener-view__factor-roles button {
  flex: 1;
  border-color: transparent;
  background: transparent;
  color: var(--tv-text-muted);
}

.stock-screener-view__factor-roles button.is-active {
  border-color: var(--tv-border);
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
}

.stock-screener-view__categories>button {
  flex: 0 0 auto;
  border-color: transparent;
  background: transparent;
  color: var(--tv-text-muted);
}

.stock-screener-view__categories>button.is-active {
  border-color: var(--tv-border);
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
}

.stock-screener-view__categories>button:last-child {
  margin-right: 0;
}

.stock-screener-view__factor-list {
  display: grid;
  min-height: 0;
  align-content: start;
  gap: 4px;
  overflow-y: auto;
  padding-right: 2px;
}

.stock-screener-view__factor-list article {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 6px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
}

.stock-screener-view__factor-list article.is-disabled {
  opacity: 0.6;
}

.stock-screener-view__factor-list article.is-experimental {
  border-color: var(--tv-status-warning-border);
}

.stock-screener-view__factor-list article>div {
  min-width: 0;
}

.stock-screener-view__factor-list code,
.stock-screener-view__factor-list small {
  margin-left: 6px;
  color: var(--tv-text-muted);
  font-size: 10px;
}

.stock-screener-view__factor-list p {
  margin: 2px 0 0;
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stock-screener-view__factor-list article>span {
  display: flex;
  flex: none;
  gap: 4px;
}

.stock-screener-view__factor-dialog-backdrop {
  position: fixed;
  z-index: 1200;
  inset: 0;
  display: grid;
  place-items: center;
  padding: 24px;
  background: rgb(0 0 0 / 54%);
  backdrop-filter: blur(2px);
}

.stock-screener-view__factor-dialog {
  display: grid;
  width: min(760px, calc(100vw - 48px));
  min-width: 0;
  max-height: min(680px, calc(100vh - 48px));
  grid-template-rows: auto minmax(0, 1fr);
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 8px;
  background: var(--tv-bg-elevated);
  box-shadow: 0 24px 64px rgb(0 0 0 / 42%);
  color: var(--tv-text);
  font-size: 12px;
}

.stock-screener-view__draft-dialog {
  display: grid;
  width: min(420px, calc(100vw - 32px));
  gap: 18px;
  padding: 18px;
  border: 1px solid var(--tv-border);
  border-radius: 8px;
  background: var(--tv-bg-elevated);
  box-shadow: 0 24px 64px rgb(0 0 0 / 42%);
  color: var(--tv-text);
}

.stock-screener-view__draft-dialog header {
  display: grid;
  gap: 6px;
}

.stock-screener-view__draft-dialog header span {
  color: var(--tv-text-muted);
  font-size: 12px;
}

.stock-screener-view__draft-dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
}

.stock-screener-view__draft-dialog-actions button {
  min-height: 32px;
  padding: 0 10px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font: inherit;
  cursor: pointer;
}

.stock-screener-view__draft-dialog-actions button:first-child {
  border-color: var(--tv-accent);
  color: var(--tv-accent);
}

.stock-screener-view__factor-dialog button,
.stock-screener-view__factor-dialog input {
  min-height: 28px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font: inherit;
}

.stock-screener-view__factor-dialog button {
  padding: 0 8px;
  cursor: pointer;
}

.stock-screener-view__factor-dialog button:hover:not(:disabled) {
  border-color: var(--tv-accent);
  background: var(--tv-bg-surface-2);
}

.stock-screener-view__factor-dialog button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.stock-screener-view__factor-dialog input {
  min-width: 0;
  padding: 0 8px;
}

.stock-screener-view__factor-dialog button:focus-visible,
.stock-screener-view__factor-dialog input:focus-visible {
  outline: 2px solid var(--tv-accent);
  outline-offset: 2px;
}

.stock-screener-view__factor-dialog-head {
  display: flex;
  min-height: 54px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--tv-border);
}

.stock-screener-view__factor-dialog-head>div {
  display: grid;
  gap: 2px;
}

.stock-screener-view__factor-dialog-head span {
  color: var(--tv-text-muted);
  font-size: 11px;
}

.stock-screener-view__factor-dialog-close {
  display: inline-grid;
  width: 30px;
  flex: 0 0 auto;
  place-items: center;
  padding: 0 !important;
  border-color: transparent !important;
  background: transparent !important;
}

.stock-screener-view__factor-dialog-close svg {
  width: 18px;
  fill: none;
  stroke: currentColor;
  stroke-linecap: round;
  stroke-width: 1.8;
}

.stock-screener-view__column-picker,
.stock-screener-view__sorts {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.stock-screener-view__column-picker>div,
.stock-screener-view__sorts>div,
.stock-screener-view__column-picker>label {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
}

.stock-screener-view__column-picker>div>span {
  min-width: 0;
  flex: 1 1 80px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stock-screener-view__column-picker button {
  min-height: 24px;
  padding: 0 6px;
}

.stock-screener-view__column-picker>label>select,
.stock-screener-view__sorts select {
  min-width: 0;
  flex: 1 1 96px;
}

.stock-screener-view__results {
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.stock-screener-view__result-head {
  flex: none;
  flex-wrap: wrap;
  padding: 0 8px;
}

.stock-screener-view__result-head span:last-child {
  margin-left: auto;
}

.stock-screener-view__empty,
.stock-screener-view__empty-small {
  display: grid;
  min-width: 0;
  min-height: 120px;
  place-items: center;
  color: var(--tv-text-dim);
}

.stock-screener-view__empty-small {
  min-height: 64px;
  border: 1px dashed var(--tv-border);
  border-radius: 6px;
}

.stock-screener-view__table-wrap {
  width: 100%;
  max-width: 100%;
  min-width: 0;
  flex: 1;
  overflow: auto;
}

.stock-screener-view table {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.stock-screener-view th {
  position: sticky;
  z-index: 2;
  top: 0;
  height: 32px;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  font-size: 11px;
  text-align: left;
}

.stock-screener-view td {
  height: 32px;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
}

.stock-screener-view tbody tr:hover {
  background: var(--tv-bg-elevated);
}

.stock-screener-view tbody tr.is-selected {
  background: color-mix(in srgb, var(--tv-accent) 10%, transparent);
}

.stock-screener-view__more {
  align-self: center;
  margin: 8px;
}

@container (max-width: 880px) {
  .stock-screener-view__workspace,
  .stock-screener-view__layout {
    display: block !important;
    height: auto;
    overflow: visible;
  }

  .stock-screener-view__workspace :deep(.splitpanes__splitter) {
    display: none !important;
  }

  .stock-screener-view__workspace :deep(.splitpanes__pane) {
    display: block;
    width: 100% !important;
    max-width: 100% !important;
    height: auto !important;
    min-width: 0 !important;
    min-height: 0 !important;
    overflow: visible;
    transform: none !important;
  }

  .stock-screener-view__workspace :deep(.splitpanes__pane + .splitpanes__pane) {
    margin-top: 8px;
  }

  .stock-screener-view__preset-sidebar {
    grid-template-columns: auto minmax(0, 1fr);
    align-items: center;
  }

  .stock-screener-view__preset-sidebar-head {
    grid-column: 1;
  }

  .stock-screener-view__new-preset {
    grid-column: 2;
  }

  .stock-screener-view__preset-list,
  .stock-screener-view__preset-empty {
    grid-column: 1 / -1;
  }

}

@media (max-width: 900px) {
  .stock-screener-view__toolbar {
    flex-wrap: wrap;
  }

  .stock-screener-view__spacer {
    display: none;
  }

  .stock-screener-view__preset-sidebar {
    grid-template-columns: 1fr;
  }

  .stock-screener-view__preset-sidebar-head,
  .stock-screener-view__new-preset,
  .stock-screener-view__preset-list,
  .stock-screener-view__preset-empty {
    grid-column: auto;
  }

  .stock-screener-view__factor-dialog-backdrop {
    padding: 12px;
  }

  .stock-screener-view__factor-dialog {
    width: calc(100vw - 24px);
    max-height: calc(100vh - 24px);
  }
}

@media (max-width: 768px) {
  .stock-screener-view {
    padding: 4px;
    padding-bottom: 56px;
  }

  .stock-screener-view__toolbar {
    position: sticky;
    z-index: 4;
    top: 0;
    min-height: 42px;
  }

  .stock-screener-view__mobile-tabs {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 4px;
  }

  .stock-screener-view__mobile-tabs button.is-active {
    border-color: var(--tv-accent);
    color: var(--tv-accent);
  }

  .stock-screener-view__builder.is-mobile-hidden,
  .stock-screener-view__results.is-mobile-hidden {
    display: none;
  }

  .stock-screener-view__layout :deep(.splitpanes__pane:has(> .is-mobile-hidden)) {
    display: none !important;
  }

  .stock-screener-view__builder,
  .stock-screener-view__results {
    max-height: none;
  }

  .stock-screener-view__builder {
    overflow: visible;
  }

  .stock-screener-view__run {
    position: fixed;
    z-index: 6;
    right: 8px;
    bottom: 8px;
    left: 8px;
    min-height: 40px;
  }
}
</style>
