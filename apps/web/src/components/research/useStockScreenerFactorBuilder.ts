import type { SplitpanesResizedPayload } from "splitpanes";
import {
  computed,
  nextTick,
  onUnmounted,
  ref,
  watch,
  type Ref,
} from "vue";

import {
  createStockScreenFilter,
  factorEnumName,
  factorRefKey,
  moveItem,
  normalizeScreenMarket,
  sameStockScreenFactorRef,
  stockScreenFactorInstanceId,
} from "./stockScreenModel";
import type {
  StockScreenCatalog,
  StockScreenColumn,
  StockScreenEditorFilter,
  StockScreenFactor,
  StockScreenFactorRef,
  StockScreenSort,
} from "./stockScreenTypes";

interface StockScreenerFactorBuilderInput {
  catalog: Ref<StockScreenCatalog | null>;
  columns: Ref<StockScreenColumn[]>;
  filters: Ref<StockScreenEditorFilter[]>;
  mobilePane: Ref<"builder" | "results">;
  queryError: Ref<string>;
  queryMarket: Ref<ReturnType<typeof normalizeScreenMarket>>;
  sorts: Ref<StockScreenSort[]>;
}

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

export function useStockScreenerFactorBuilder(
  input: StockScreenerFactorBuilderInput,
) {
  const factorDialogOpen = ref(false);
  const catalogSearch = ref("");
  const activeCategory = ref("");
  const activeFactorRole = ref<"filter" | "column" | "sort">("filter");
  const addFactorButton = ref<HTMLButtonElement | null>(null);
  const factorSearchInput = ref<HTMLInputElement | null>(null);
  const categoryScroller = ref<HTMLDivElement | null>(null);
  const canScrollCategoriesLeft = ref(false);
  const canScrollCategoriesRight = ref(false);
  const screenerOuterPaneSizes = ref<[number, number]>([18, 82]);
  const screenerInnerPaneSizes = ref<[number, number]>([39, 61]);
  const screenerOuterPaneMinSizes: [number, number] = [12, 70];
  const screenerInnerPaneMinSizes: [number, number] = [28, 45];
  let filterSerial = 0;
  let categoryResizeObserver: ResizeObserver | null = null;

  const factorMap = computed(
    () =>
      new Map(
        (input.catalog.value?.factors ?? []).map((factor) => [factor.key, factor]),
      ),
  );
  const commonFactors = computed(() =>
    (input.catalog.value?.factors ?? []).filter(
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
    (input.catalog.value?.factors ?? []).filter(
      (factor) => factor.retrieve && factor.availability !== "unsupported",
    ),
  );
  const sortableFactors = computed(() =>
    (input.catalog.value?.factors ?? []).filter(
      (factor) => factor.sort && factor.availability !== "unsupported",
    ),
  );
  const visibleCatalogFactors = computed(() => {
    const keyword = catalogSearch.value.trim().toLocaleLowerCase();
    return (input.catalog.value?.factors ?? []).filter((factor) => {
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

  function nextFactorSerial(): number {
    return ++filterSerial;
  }

  function factorFor(key: string): StockScreenFactor | undefined {
    return factorMap.value.get(key);
  }

  function hasDuplicateRef(
    refs: StockScreenFactorRef[],
    candidate: StockScreenFactorRef,
  ): boolean {
    return refs.some((reference) => sameStockScreenFactorRef(reference, candidate));
  }

  function columnExists(key: string): boolean {
    const factor = factorFor(key);
    if (!factor || !input.catalog.value) return false;
    const params = createStockScreenFilter(
      factor,
      0,
      input.catalog.value,
      input.queryMarket.value,
    ).params;
    return hasDuplicateRef(input.columns.value, {
      factor: key,
      ...(params ? { params } : {}),
    });
  }

  function columnIdentity(column: StockScreenColumn, index: number): string {
    return (
      column.columnId ??
      stockScreenFactorInstanceId(column, `${factorRefKey(column)}-${index}`)
    );
  }

  function sortIdentity(sort: StockScreenSort, index: number): string {
    return (
      sort.sortId ??
      stockScreenFactorInstanceId(sort, `${factorRefKey(sort)}-${index}`)
    );
  }

  function enumOptionsForFactor(factor: StockScreenFactor | undefined) {
    if (!factor || !input.catalog.value) return [];
    const name = factorEnumName(factor);
    return name ? (input.catalog.value.enums[name] ?? []) : [];
  }

  async function addFilter(factor: StockScreenFactor): Promise<void> {
    if (
      !factor.filter ||
      factor.availability === "unsupported" ||
      !input.catalog.value
    ) {
      return;
    }
    const serial = nextFactorSerial();
    const nextFilter = createStockScreenFilter(
      factor,
      serial,
      input.catalog.value,
      input.queryMarket.value,
      `${factor.key}-${serial}`,
    );
    if (hasDuplicateRef(input.filters.value, nextFilter)) {
      input.queryError.value = `已存在相同参数的“${factor.label}”条件`;
      return;
    }
    input.filters.value.push(nextFilter);
    input.mobilePane.value = "builder";
    input.queryError.value = "";
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
    input.filters.value = input.filters.value.filter((filter) => filter.id !== id);
  }

  async function addColumn(key: string): Promise<void> {
    const factor = factorFor(key);
    if (!factor || !input.catalog.value) return;
    const params = createStockScreenFilter(
      factor,
      0,
      input.catalog.value,
      input.queryMarket.value,
    ).params;
    const serial = nextFactorSerial();
    const nextColumn: StockScreenColumn = {
      factor: key,
      factorKey: key,
      instanceId: `column-${key}-${serial}`,
      ...(params ? { params } : {}),
      columnId: `column-${key}-${serial}`,
    };
    if (hasDuplicateRef(input.columns.value, nextColumn)) {
      input.queryError.value = `已存在相同参数的“${factor.label}”结果列`;
      return;
    }
    input.columns.value.push(nextColumn);
    input.queryError.value = "";
    factorDialogOpen.value = false;
    await nextTick();
    const identity = columnIdentity(nextColumn, input.columns.value.length - 1);
    const row = Array.from(
      document.querySelectorAll<HTMLElement>("[data-column-id]"),
    ).find((candidate) => candidate.dataset.columnId === identity);
    row?.querySelector<HTMLElement>("input, select")?.focus();
  }

  function removeColumn(column: StockScreenColumn): void {
    input.columns.value = input.columns.value.filter((item) => item !== column);
  }

  function moveColumn(index: number, delta: number): void {
    input.columns.value = moveItem(input.columns.value, index, delta);
  }

  async function addSort(preferredKey?: string): Promise<void> {
    if (!input.catalog.value) return;
    const candidates = preferredKey
      ? sortableFactors.value.filter((candidate) => candidate.key === preferredKey)
      : sortableFactors.value;
    const factor = candidates.find((candidate) => {
      const params = createStockScreenFilter(
        candidate,
        0,
        input.catalog.value!,
        input.queryMarket.value,
      ).params;
      return !hasDuplicateRef(input.sorts.value, {
        factor: candidate.key,
        ...(params ? { params } : {}),
      });
    });
    if (!factor) return;
    const params = createStockScreenFilter(
      factor,
      0,
      input.catalog.value,
      input.queryMarket.value,
    ).params;
    const serial = nextFactorSerial();
    const nextSort: StockScreenSort = {
      factor: factor.key,
      factorKey: factor.key,
      instanceId: `sort-${factor.key}-${serial}`,
      direction: "desc",
      ...(params ? { params } : {}),
      sortId: `sort-${factor.key}-${serial}`,
    };
    if (hasDuplicateRef(input.sorts.value, nextSort)) return;
    input.sorts.value.push(nextSort);
    factorDialogOpen.value = false;
    await nextTick();
    const identity = sortIdentity(nextSort, input.sorts.value.length - 1);
    const row = Array.from(
      document.querySelectorAll<HTMLElement>("[data-sort-id]"),
    ).find((candidate) => candidate.dataset.sortId === identity);
    row?.querySelector<HTMLElement>("input, select")?.focus();
  }

  function sortFactorInput(sort: StockScreenSort, event: Event): void {
    const key = (event.target as HTMLSelectElement).value;
    const factor = factorFor(key);
    if (!factor || !input.catalog.value) return;
    const params = createStockScreenFilter(
      factor,
      0,
      input.catalog.value,
      input.queryMarket.value,
    ).params;
    sort.factor = key;
    sort.factorKey = key;
    const serial = nextFactorSerial();
    sort.instanceId = `sort-${key}-${serial}`;
    if (params) sort.params = params;
    else delete sort.params;
    sort.sortId ??= `sort-${key}-${serial}`;
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
    if (!factor || !input.catalog.value) return;
    const params = createStockScreenFilter(
      factor,
      0,
      input.catalog.value,
      input.queryMarket.value,
    ).params;
    const serial = nextFactorSerial();
    filter.secondFactor = {
      factor: factorKey,
      instanceId: `second-${factorKey}-${serial}`,
      factorKey,
      ...(params ? { params } : {}),
    };
    delete filter.secondValue;
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

  watch(factorDialogOpen, (open) => {
    if (open) return;
    categoryResizeObserver?.disconnect();
    categoryResizeObserver = null;
    canScrollCategoriesLeft.value = false;
    canScrollCategoriesRight.value = false;
  });

  watch(
    () => input.catalog.value?.categories.length ?? 0,
    async () => {
      if (!factorDialogOpen.value) return;
      await nextTick();
      updateCategoryScrollState();
    },
  );

  onUnmounted(() => categoryResizeObserver?.disconnect());

  return {
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
  };
}
