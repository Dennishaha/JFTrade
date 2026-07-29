import type { SplitpanesResizedPayload } from "splitpanes";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

import {
  normalizeQuoteWorkbenchPeriod,
  normalizeQuoteWorkbenchProductClass,
  type QuoteWorkbenchPeriod,
  type QuoteWorkbenchTab,
} from "../components/domain/market-data/quoteWorkbench";
import {
  MARKET_CODE_OPTIONS,
  MARKET_VIEWS,
  RESEARCH_SECTIONS,
  type MarketView,
  type ResearchSection,
  researchFeatureIds,
  validMarketCode,
  validMarketView,
  validResearchSection,
} from "../components/research/researchNavigation";
import {
  normalizeResearchQuoteTarget,
  researchQuoteSeedFromEntry,
  researchQuoteTargetFromEntry,
  type QuoteSeed,
  type ResearchQuoteTarget,
} from "../components/research/researchQuote";
import { useBrokerProviderSelection } from "@/composables/trading/brokerProviderSelection";
import { useConsoleData } from "@/composables/workspace/useConsoleData";
import {
  clampResearchPaneSizesForWidth,
  readResearchViewState,
  researchPaneBoundsForWidth,
  writeResearchViewState,
} from "@/composables/research/useResearchViewState";
import { useWorkspaceTradingPrefs } from "@/composables/workspace/useWorkspaceLayout";

export function useResearchPageController() {
  const sections = RESEARCH_SECTIONS;

  const route = useRoute();
  const router = useRouter();
  const { prefs: workspacePrefs, update } = useWorkspaceTradingPrefs();
  const { selectedBrokerAccount, systemStatus } = useConsoleData();
  const { selectedBrokerId, selectBrokerProvider } =
    useBrokerProviderSelection();
  function configFor(section: ResearchSection) {
    return sections.find((item) => item.value === section)!;
  }

  function operationFor(section: ResearchSection, value: unknown): string {
    const config = configFor(section);
    const rawCandidate = String(value ?? "");
    const candidate =
      section === "industries" &&
        ["chain_detail", "chains_by_plate"].includes(rawCandidate)
        ? "chains"
        : rawCandidate;
    return config.operations.some((item) => item.value === candidate)
      ? candidate
      : config.operations[0]?.value ?? "";
  }

  function firstQueryValue(value: unknown): string {
    if (Array.isArray(value)) return String(value[0] ?? "").trim();
    return String(value ?? "").trim();
  }

  function quotePeriodFromQuery(value: unknown): QuoteWorkbenchPeriod {
    return normalizeQuoteWorkbenchPeriod(firstQueryValue(value));
  }

  type PredictionContractView =
    | "snapshot"
    | "depth"
    | "candles"
    | "ticks"
    | "milestones";

  function predictionContractViewFromQuery(
    value: unknown,
  ): PredictionContractView {
    const candidate = firstQueryValue(value);
    return ["snapshot", "depth", "candles", "ticks", "milestones"].includes(
      candidate,
    )
      ? (candidate as PredictionContractView)
      : "snapshot";
  }

  function quoteTargetFromQuery(): ResearchQuoteTarget | null {
    const instrumentId = firstQueryValue(route.query.quote);
    if (instrumentId === "") return null;
    return normalizeResearchQuoteTarget({
      kind: firstQueryValue(route.query.quoteKind) === "plate"
        ? "plate"
        : "instrument",
      instrumentId,
      name: firstQueryValue(route.query.quoteName),
      productClass: normalizeQuoteWorkbenchProductClass(
        firstQueryValue(route.query.quoteClass),
      ),
    });
  }

  function quoteTabFromQuery(
    target: ResearchQuoteTarget | null,
  ): QuoteWorkbenchTab {
    return target?.kind !== "plate" && firstQueryValue(route.query.quoteTab) === "news"
      ? "news"
      : "quote";
  }

  const initialResearchViewState = readResearchViewState();
  const initialQuoteTarget = quoteTargetFromQuery();

  const activeSection = ref<ResearchSection>(
    validResearchSection(route.query.section),
  );
  const activeConfig = computed(() => configFor(activeSection.value));
  const activeOperation = ref(operationFor(activeSection.value, route.query.operation));

  const activeMarketView = ref<MarketView>(validMarketView(route.query.view));
  const activeMarketCode = ref(validMarketCode(route.query.mkt));
  const workspaceInstrumentId = computed(
    () =>
      `${workspacePrefs.value.market}.${workspacePrefs.value.symbol}`
        .trim()
        .toUpperCase(),
  );
  const activeInstrumentId = computed(() => {
    const candidate = firstQueryValue(route.query.instrumentId).toUpperCase();
    return candidate.includes(".") ? candidate : workspaceInstrumentId.value;
  });
  const activeIndicatorId = computed(() =>
    firstQueryValue(route.query.indicatorId),
  );
  const activeChainId = computed(() => firstQueryValue(route.query.chainId));
  const activePlateId = computed(() => firstQueryValue(route.query.plateId));
  const activePresetId = computed(() => firstQueryValue(route.query.presetId));
  const activeInstitutionId = computed(() =>
    firstQueryValue(route.query.institutionId),
  );
  const activePredictionSeriesCode = computed(() =>
    firstQueryValue(route.query.seriesCode),
  );
  const activePredictionEventCode = computed(() =>
    firstQueryValue(route.query.eventCode),
  );
  const activePredictionContractCode = computed(() =>
    firstQueryValue(route.query.contractCode),
  );
  const activePredictionContractView = computed(() =>
    predictionContractViewFromQuery(route.query.contractView),
  );
  const activeScreenMarket = computed<"US" | "HK" | "SH" | "SZ">(() => {
    const candidate = firstQueryValue(route.query.screenMarket).toUpperCase();
    if (candidate === "US" || candidate === "HK" || candidate === "SH" || candidate === "SZ") {
      return candidate;
    }
    return activeMarketCode.value === "HK"
      ? "HK"
      : activeMarketCode.value === "CN"
        ? "SH"
        : "US";
  });
  const selectedQuoteTarget = ref<ResearchQuoteTarget | null>(initialQuoteTarget);
  const selectedQuoteSeed = ref<QuoteSeed | null>(null);
  const selectedQuotePeriod = ref<QuoteWorkbenchPeriod>(
    quotePeriodFromQuery(route.query.quotePeriod),
  );
  const selectedQuoteTab = ref<QuoteWorkbenchTab>(
    quoteTabFromQuery(initialQuoteTarget),
  );
  const marketRailCollapsed = ref(initialResearchViewState.railCollapsed);
  const marketRailDrawer = ref(false);
  const marketPaneSizes = ref<[number, number]>(
    initialResearchViewState.paneSizes,
  );
  const researchPageRef = ref<HTMLElement | null>(null);
  const researchPageWidth = ref(0);
  const researchPaneBounds = computed(() =>
    researchPaneBoundsForWidth(researchPageWidth.value),
  );
  const rankingInitialOperation = ref("top_gainers");
  let railMediaQuery: MediaQueryList | null = null;
  let researchResizeObserver: ResizeObserver | null = null;
  let suppressRailPersistence = false;

  function syncRailMode(matches: boolean): void {
    const becameNarrow = matches && !marketRailDrawer.value;
    marketRailDrawer.value = matches;
    if (becameNarrow && selectedQuoteTarget.value == null) {
      suppressRailPersistence = true;
      marketRailCollapsed.value = true;
      queueMicrotask(() => {
        suppressRailPersistence = false;
      });
    }
  }

  function handleRailMediaChange(event: MediaQueryListEvent): void { syncRailMode(event.matches); }

  function syncResearchPageWidth(width: number): void {
    if (!Number.isFinite(width) || width <= 0) return;
    researchPageWidth.value = width;
    if (marketRailDrawer.value || marketRailCollapsed.value) return;
    const normalized = clampResearchPaneSizesForWidth(
      marketPaneSizes.value,
      width,
    );
    if (
      Math.abs(normalized[0] - marketPaneSizes.value[0]) < 0.01 &&
      Math.abs(normalized[1] - marketPaneSizes.value[1]) < 0.01
    ) {
      return;
    }
    marketPaneSizes.value = normalized;
    persistResearchView();
  }

  onMounted(() => {
    if (typeof window.matchMedia === "function") {
      railMediaQuery = window.matchMedia("(max-width: 1100px)");
      syncRailMode(railMediaQuery.matches);
      railMediaQuery.addEventListener("change", handleRailMediaChange);
    }
    const element = researchPageRef.value;
    if (element == null) return;
    syncResearchPageWidth(element.getBoundingClientRect().width);
    if (typeof ResizeObserver !== "undefined") {
      researchResizeObserver = new ResizeObserver((entries) => {
        const width =
          entries[0]?.contentRect.width ??
          researchPageRef.value?.getBoundingClientRect().width ??
          0;
        syncResearchPageWidth(width);
      });
      researchResizeObserver.observe(element);
    }
  });

  onBeforeUnmount(() => {
    railMediaQuery?.removeEventListener("change", handleRailMediaChange);
    railMediaQuery = null;
    researchResizeObserver?.disconnect();
    researchResizeObserver = null;
  });

  function queryWith(
    patch: Record<string, string | undefined>,
  ): Record<string, string | string[]> {
    const next: Record<string, string | string[]> = {};
    for (const [key, value] of Object.entries({ ...route.query, ...patch })) {
      if (value == null || value === "") continue;
      next[key] = Array.isArray(value) ? value.map(String) : String(value);
    }
    return next;
  }

  const emptyQuoteQuery = {
    quote: undefined,
    quoteKind: undefined,
    quoteClass: undefined,
    quoteName: undefined,
    quotePeriod: undefined,
    quoteTab: undefined,
  } satisfies Record<string, undefined>;

  function quoteQuery(
    target: ResearchQuoteTarget,
    period = selectedQuotePeriod.value,
    tab = selectedQuoteTab.value,
  ): Record<string, string | undefined> {
    return {
      section: activeSection.value,
      operation: activeOperation.value,
      view:
        activeSection.value === "market" ? activeMarketView.value : undefined,
      mkt: activeMarketCode.value,
      quote: target.instrumentId,
      quoteKind: target.kind,
      quoteClass: target.productClass,
      quoteName: target.name || undefined,
      quotePeriod: period,
      quoteTab: target.kind === "plate" ? "quote" : tab,
    };
  }

  function clearQuoteSelection(): void {
    selectedQuoteTarget.value = null;
    selectedQuoteSeed.value = null;
    selectedQuotePeriod.value = "1d";
    selectedQuoteTab.value = "quote";
    marketRailCollapsed.value = true;
  }

  function persistResearchView(): void {
    writeResearchViewState({
      railCollapsed: marketRailCollapsed.value,
      paneSizes: marketPaneSizes.value,
    });
  }

  watch(marketRailCollapsed, () => {
    if (!suppressRailPersistence) {
      persistResearchView();
    }
  });

  function normalizeInvalidResearchRoute(): boolean {
    const requestedSection = firstQueryValue(route.query.section);
    const requestedView = firstQueryValue(route.query.view);
    const invalidSection =
      requestedSection !== "" &&
      !sections.some((section) => section.value === requestedSection);
    const invalidView =
      requestedView !== "" &&
      !MARKET_VIEWS.some((view) => view.value === requestedView);
    if (!invalidSection && !invalidView) return false;

    const requestedMarket = firstQueryValue(route.query.mkt);
    const market = MARKET_CODE_OPTIONS.some(
      (option) => option.value === requestedMarket,
    )
      ? requestedMarket
      : undefined;
    activeSection.value = "market";
    activeOperation.value = "top_movers";
    activeMarketView.value = "home";
    activeMarketCode.value = market ?? "US";
    clearQuoteSelection();
    void router.replace({
      query: {
        section: "market",
        operation: "top_movers",
        view: "home",
        ...(market ? { mkt: market } : {}),
      },
    });
    return true;
  }

  function selectSection(value: unknown): void {
    const requested = String(value ?? "");
    if (!sections.some((section) => section.value === requested)) return;
    const section = requested as ResearchSection;
    const operation = operationFor(section, "");
    const market =
      section === "institutions" && activeMarketCode.value === "CN"
        ? "US"
        : activeMarketCode.value;
    activeSection.value = section;
    activeOperation.value = operation;
    activeMarketCode.value = market;
    clearQuoteSelection();
    void router.replace({
      query: queryWith({
        section,
        operation,
        view: section === "market" ? activeMarketView.value : undefined,
        mkt: market,
        indicatorId: undefined,
        chainId: undefined,
        plateId: undefined,
        instrumentId:
          section === "instrument" ? activeInstrumentId.value : undefined,
        institutionId:
          section === "institutions" &&
            ["list", "holding_changes"].includes(operation)
            ? activeInstitutionId.value || undefined
            : undefined,
        presetId: section === "screens" ? activePresetId.value : undefined,
        screenMarket:
          section === "screens" ? activeScreenMarket.value : undefined,
        seriesCode:
          section === "prediction"
            ? activePredictionSeriesCode.value || undefined
            : undefined,
        eventCode:
          section === "prediction"
            ? activePredictionEventCode.value || undefined
            : undefined,
        contractCode:
          section === "prediction"
            ? activePredictionContractCode.value || undefined
            : undefined,
        contractView:
          section === "prediction" && activePredictionContractCode.value
            ? activePredictionContractView.value
            : undefined,
        ...emptyQuoteQuery,
      }),
    });
  }

  function selectOperation(value: unknown): void {
    const operation = operationFor(activeSection.value, value);
    if (operation === activeOperation.value) return;
    activeOperation.value = operation;
    clearQuoteSelection();
    void router.replace({
      query: queryWith({
        section: activeSection.value,
        operation,
        indicatorId:
          activeSection.value === "macro" && operation === "indicators"
            ? activeIndicatorId.value
            : undefined,
        plateId: activeSection.value === "industries" ? activePlateId.value : undefined,
        chainId: activeSection.value === "industries" ? activeChainId.value : undefined,
        instrumentId:
          activeSection.value === "instrument"
            ? activeInstrumentId.value
            : undefined,
        institutionId:
          activeSection.value === "institutions" &&
            ["list", "holding_changes"].includes(operation)
            ? activeInstitutionId.value || undefined
            : undefined,
        presetId:
          activeSection.value === "screens"
            ? activePresetId.value || undefined
            : undefined,
        seriesCode:
          activeSection.value === "prediction"
            ? activePredictionSeriesCode.value || undefined
            : undefined,
        eventCode:
          activeSection.value === "prediction"
            ? activePredictionEventCode.value || undefined
            : undefined,
        contractCode:
          activeSection.value === "prediction"
            ? activePredictionContractCode.value || undefined
            : undefined,
        contractView:
          activeSection.value === "prediction" &&
            activePredictionContractCode.value
            ? activePredictionContractView.value
            : undefined,
        ...emptyQuoteQuery,
      }),
    });
  }

  function selectMarket(value: unknown): void {
    const market = validMarketCode(value);
    if (market === activeMarketCode.value) return;
    activeMarketCode.value = market;
    const operations = visibleOperations.value;
    const operation = operations.some(
      (item) => item.value === activeOperation.value,
    )
      ? activeOperation.value
      : operations[0]?.value ?? "";
    activeOperation.value = operation;
    clearQuoteSelection();
    void router.replace({
      query: queryWith({
        section: activeSection.value,
        operation,
        mkt: market,
        screenMarket:
          activeSection.value === "screens"
            ? market === "CN"
              ? "SH"
              : market
            : undefined,
        institutionId: undefined,
        chainId: undefined,
        plateId: undefined,
        ...emptyQuoteQuery,
      }),
    });
  }

  function updateResearchContext(
    patch: Record<string, string | undefined>,
  ): void {
    void router.replace({ query: queryWith(patch) });
  }

  function selectIndicatorContext(indicatorId: string): void {
    updateResearchContext({
      section: "macro",
      operation: "indicators",
      indicatorId: indicatorId || undefined,
    });
  }

  function selectIndustryChain(chainId: string): void {
    updateResearchContext({
      section: "industries",
      operation: "chains",
      chainId: chainId || undefined,
      plateId: undefined,
    });
  }

  function selectIndustryPlate(plateId: string): void {
    updateResearchContext({
      section: "industries",
      operation: "chains",
      chainId: activeChainId.value || undefined,
      plateId: plateId || undefined,
    });
  }

  function selectResearchInstrument(instrumentId: string): void {
    const normalized = instrumentId.trim().toUpperCase();
    if (!normalized.includes(".")) return;
    clearQuoteSelection();
    updateResearchContext({
      section: "instrument",
      operation: activeOperation.value,
      instrumentId: normalized,
      ...emptyQuoteQuery,
    });
  }

  function selectScreenPreset(presetId: string): void {
    updateResearchContext({
      section: "screens",
      operation: "stock_v2",
      presetId: presetId || undefined,
    });
  }

  function selectInstitutionContext(institutionId: string): void {
    updateResearchContext({
      section: "institutions",
      operation: activeOperation.value,
      institutionId: institutionId || undefined,
    });
  }

  function selectPredictionSeries(seriesCode: string): void {
    updateResearchContext({
      section: "prediction",
      operation: "categories",
      seriesCode: seriesCode || undefined,
      eventCode: undefined,
      contractCode: undefined,
      contractView: undefined,
    });
  }

  function selectPredictionEvent(eventCode: string): void {
    updateResearchContext({
      section: "prediction",
      operation: "categories",
      seriesCode: activePredictionSeriesCode.value || undefined,
      eventCode: eventCode || undefined,
      contractCode: undefined,
      contractView: undefined,
    });
  }

  function selectPredictionContract(contractCode: string): void {
    updateResearchContext({
      section: "prediction",
      operation: "categories",
      seriesCode: activePredictionSeriesCode.value || undefined,
      eventCode: activePredictionEventCode.value || undefined,
      contractCode: contractCode || undefined,
      contractView: contractCode ? "snapshot" : undefined,
    });
  }

  function selectPredictionContractView(contractView: string): void {
    updateResearchContext({
      section: "prediction",
      operation: "categories",
      contractCode: activePredictionContractCode.value || undefined,
      contractView: activePredictionContractCode.value
        ? predictionContractViewFromQuery(contractView)
        : undefined,
    });
  }

  function selectScreenContext(context: {
    market: string;
    brokerId?: string;
  }): void {
    if (context.brokerId && context.brokerId !== selectedBrokerId.value) {
      selectBrokerProvider(context.brokerId);
    }
    const concrete = context.market.trim().toUpperCase();
    if (!["US", "HK", "SH", "SZ"].includes(concrete)) return;
    const logical = concrete === "SH" || concrete === "SZ" ? "CN" : concrete;
    activeMarketCode.value = logical;
    clearQuoteSelection();
    updateResearchContext({
      section: "screens",
      operation: "stock_v2",
      mkt: logical,
      screenMarket: concrete,
      ...emptyQuoteQuery,
    });
  }

  watch(
    () => [route.query.section, route.query.operation, route.query.view] as const,
    ([sectionValue, operationValue]) => {
      if (normalizeInvalidResearchRoute()) return;
      const section = validResearchSection(sectionValue);
      activeSection.value = section;
      activeOperation.value = operationFor(section, operationValue);
      if (section === "institutions" && activeMarketCode.value === "CN") {
        activeMarketCode.value = "US";
        clearQuoteSelection();
        void router.replace({
          query: queryWith({ mkt: "US", ...emptyQuoteQuery }),
        });
      }
      if (section === "market") {
        activeMarketView.value = validMarketView(route.query.view);
      }
    },
    { immediate: true },
  );

  watch(activeMarketView, (view) => {
    if (activeSection.value !== "market" || route.query.view === view) return;
    clearQuoteSelection();
    void router.replace({
      query: queryWith({ view, ...emptyQuoteQuery }),
    });
  });

  watch(() => route.query.mkt, (market) => {
    activeMarketCode.value = validMarketCode(market);
  });

  watch(
    () => [
      route.query.quote,
      route.query.quoteKind,
      route.query.quoteClass,
      route.query.quoteName,
      route.query.quotePeriod,
      route.query.quoteTab,
    ] as const,
    () => {
      const target = quoteTargetFromQuery();
      selectedQuoteTarget.value = target;
      selectedQuoteSeed.value = null;
      selectedQuotePeriod.value = quotePeriodFromQuery(route.query.quotePeriod);
      selectedQuoteTab.value = quoteTabFromQuery(target);
    },
    { immediate: true },
  );

  const queryMarket = computed(() => {
    if (activeSection.value === "market" || activeSection.value === "calendar") {
      return activeMarketCode.value;
    }
    if (activeSection.value === "screens") {
      return activeScreenMarket.value;
    }
    if (activeSection.value === "derivatives") {
      return activeOperation.value === "warrant" ? "HK" : "US";
    }
    if (activeSection.value === "institutions") {
      return activeMarketCode.value === "HK" ? "HK" : "US";
    }
    if (activeSection.value === "industries") {
      return activeMarketCode.value;
    }
    if (activeSection.value === "instrument") {
      return activeInstrumentId.value.split(".", 1)[0] || workspacePrefs.value.market;
    }
    return "US";
  });

  const sectionMarketOptions = computed(() =>
    activeSection.value === "institutions"
      ? MARKET_CODE_OPTIONS.filter((item) => item.value !== "CN")
      : MARKET_CODE_OPTIONS,
  );
  const showSectionMarketSwitch = computed(() =>
    ["screens", "calendar", "institutions", "industries"].includes(
      activeSection.value,
    ));
  const visibleOperations = computed(() => {
    return activeConfig.value.operations.filter((operation) => {
      if (activeSection.value === "calendar" && activeMarketCode.value === "CN") {
        return operation.value !== "dividends";
      }
      if (activeSection.value === "institutions" && activeMarketCode.value === "HK") {
        return !operation.value.startsWith("ark_");
      }
      return true;
    });
  });

  watch([activeMarketCode, visibleOperations], ([, operations]) => {
    if (!operations.some((item) => item.value === activeOperation.value)) {
      const operation = operations[0]?.value ?? "";
      activeOperation.value = operation;
      clearQuoteSelection();
      if (route.query.operation !== operation) {
        void router.replace({
          query: queryWith({
            section: activeSection.value,
            operation,
            ...emptyQuoteQuery,
          }),
        });
      }
    }
  }, { immediate: true });

  function handleMarketPaneResized(payload: SplitpanesResizedPayload): void {
    if (marketRailDrawer.value || marketRailCollapsed.value) return;
    const sizes = payload.panes?.map((pane) => pane.size);
    if (
      sizes == null ||
      sizes.length !== 2 ||
      !sizes.every((size) => Number.isFinite(size) && size > 0 && size <= 100)
    ) {
      return;
    }
    marketPaneSizes.value = clampResearchPaneSizesForWidth(
      [sizes[0]!, sizes[1]!],
      researchPageWidth.value,
    );
    persistResearchView();
  }

  const optionResearchOperation = computed(
    () =>
      (["unusual", "zero_dte", "earnings", "seller"].includes(
        activeOperation.value,
      )
        ? activeOperation.value
        : "unusual") as "unusual" | "zero_dte" | "earnings" | "seller",
  );
  const macroResearchOperation = computed(
    () =>
      (["indicators", "fed_target_rate", "fed_dot_plot"].includes(
        activeOperation.value,
      )
        ? activeOperation.value
        : "indicators") as
      | "indicators"
      | "fed_target_rate"
      | "fed_dot_plot",
  );
  const arkResearchOperation = computed(
    () =>
      (activeOperation.value === "ark_fund_holdings"
        ? "ark_fund_holdings"
        : "ark_transactions") as
      | "ark_fund_holdings"
      | "ark_transactions",
  );
  const derivativeScreenOperation = computed(
    () =>
      (activeOperation.value === "warrant"
        ? "warrant"
        : "option_screen") as "option_screen" | "warrant",
  );
  const instrumentResearchOperation = computed(
    () =>
      ([
        "profile",
        "financials",
        "valuation",
        "analyst",
        "ownership",
        "corporate_actions",
        "short_interest",
        "news",
      ].includes(activeOperation.value)
        ? activeOperation.value
        : "profile") as
      | "profile"
      | "financials"
      | "valuation"
      | "analyst"
      | "ownership"
      | "corporate_actions"
      | "short_interest"
      | "news",
  );
  const activeFeatureIDs = computed(() => researchFeatureIds(
    activeSection.value, activeOperation.value, activeMarketView.value));
  function handleMarketSelect(entry: Record<string, unknown>): void {
    const target = researchQuoteTargetFromEntry(
      entry,
      activeMarketCode.value,
    );
    selectedQuoteTarget.value = target;
    selectedQuoteSeed.value = target == null ? null : researchQuoteSeedFromEntry(entry);
    if (target != null) {
      selectedQuoteTab.value = "quote";
      marketRailCollapsed.value = false;
      void router.replace({
        query: queryWith(
          quoteQuery(target, selectedQuotePeriod.value, selectedQuoteTab.value),
        ),
      });
    }
  }

  function selectRailTarget(target: ResearchQuoteTarget): void {
    selectedQuoteTarget.value = target;
    selectedQuoteSeed.value = null;
    selectedQuoteTab.value = "quote";
    marketRailCollapsed.value = false;
    void router.replace({
      query: queryWith(
        quoteQuery(target, selectedQuotePeriod.value, selectedQuoteTab.value),
      ),
    });
  }

  function selectQuotePeriod(value: unknown): void {
    const period = quotePeriodFromQuery(value);
    selectedQuotePeriod.value = period;
    const target = selectedQuoteTarget.value;
    if (target == null || route.query.quotePeriod === period) return;
    void router.replace({
      query: queryWith(quoteQuery(target, period, selectedQuoteTab.value)),
    });
  }

  function selectQuoteTab(value: unknown): void {
    const target = selectedQuoteTarget.value;
    const tab: QuoteWorkbenchTab =
      target?.kind !== "plate" && value === "news" ? "news" : "quote";
    selectedQuoteTab.value = tab;
    if (target == null || route.query.quoteTab === tab) return;
    void router.replace({
      query: queryWith(quoteQuery(target, selectedQuotePeriod.value, tab)),
    });
  }

  function handleMarketMore(operation: string): void {
    rankingInitialOperation.value = operation === "top_movers" ? "top_gainers" : operation;
    activeMarketView.value = "rankings";
  }

  function openWorkspaceInstrument(
    instrumentID: string,
    marketSegment: "securities" | "derivatives" | "prediction" = "securities",
    productClass:
      | "equity"
      | "fund"
      | "option"
      | "warrant"
      | "cbbc"
      | "future"
      | "event_contract"
      | "index"
      | "bond"
      | "plate"
      | "unknown" = "unknown",
  ): void {
    const [market, ...symbolParts] = instrumentID.split(".");
    const symbol = symbolParts.join(".");
    if (!market || !symbol) return;
    update({ market, symbol, marketSegment, productClass });
    void router.push({
      path: "/workspace",
      query: {
        tab: marketSegment === "prediction" ? "contract" : "chart",
        marketSegment,
        returnTo: route.fullPath,
      },
    });
  }

  function openQuoteTargetInWorkspace(target: ResearchQuoteTarget): void {
    if (target.kind === "plate") return;
    const productClass = (
      [
        "equity",
        "fund",
        "warrant",
        "cbbc",
        "index",
        "bond",
      ].includes(target.productClass)
        ? target.productClass
        : "unknown"
    ) as
      | "equity"
      | "fund"
      | "warrant"
      | "cbbc"
      | "index"
      | "bond"
      | "unknown";
    openWorkspaceInstrument(target.instrumentId, "securities", productClass);
  }

  function openOptionResearchInstrument(
    instrumentID: string,
    productClass: "option" | "equity",
  ): void {
    if (productClass === "equity") {
      handleMarketSelect({ instrumentId: instrumentID, productClass });
      return;
    }
    openWorkspaceInstrument(instrumentID, "derivatives", productClass);
  }

  function researchEntry(value: unknown): Record<string, unknown> | null {
    return value != null && typeof value === "object" && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : null;
  }

  function selectResearchEntry(value: unknown): void {
    const entry = researchEntry(value);
    if (entry != null) handleMarketSelect(entry);
  }

  function openResearchEntry(
    value: unknown,
    productClassHint: "option" | "equity" | "unknown" = "unknown",
  ): void {
    const entry = researchEntry(value) ?? {};
    const target = researchQuoteTargetFromEntry(entry, queryMarket.value);
    const instrumentId =
      target?.instrumentId ||
      (activeSection.value === "instrument" ? activeInstrumentId.value : "");
    if (!instrumentId) return;
    const productClass =
      productClassHint !== "unknown"
        ? productClassHint
        : "equity";
    if (productClass === "option") {
      openWorkspaceInstrument(instrumentId, "derivatives", "option");
      return;
    }
    openWorkspaceInstrument(
      instrumentId,
      "securities",
      target?.productClass === "fund" ||
        target?.productClass === "warrant" ||
        target?.productClass === "cbbc" ||
        target?.productClass === "index"
        ? target.productClass
        : "equity",
    );
  }

  return {
    sections,
    MARKET_VIEWS,
    MARKET_CODE_OPTIONS,
    selectedBrokerAccount,
    systemStatus,
    selectedBrokerId,
    configFor,
    operationFor,
    firstQueryValue,
    quotePeriodFromQuery,
    predictionContractViewFromQuery,
    activeSection,
    activeConfig,
    activeOperation,
    activeMarketView,
    activeMarketCode,
    workspaceInstrumentId,
    activeInstrumentId,
    activeIndicatorId,
    activeChainId,
    activePlateId,
    activePresetId,
    activeInstitutionId,
    activePredictionSeriesCode,
    activePredictionEventCode,
    activePredictionContractCode,
    activePredictionContractView,
    activeScreenMarket,
    selectedQuoteTarget,
    selectedQuoteSeed,
    selectedQuotePeriod,
    selectedQuoteTab,
    marketRailCollapsed,
    marketRailDrawer,
    marketPaneSizes,
    researchPageRef,
    researchPageWidth,
    researchPaneBounds,
    rankingInitialOperation,
    syncRailMode,
    handleRailMediaChange,
    syncResearchPageWidth,
    queryWith,
    quoteQuery,
    clearQuoteSelection,
    persistResearchView,
    normalizeInvalidResearchRoute,
    selectSection,
    selectOperation,
    selectMarket,
    updateResearchContext,
    selectIndicatorContext,
    selectIndustryChain,
    selectIndustryPlate,
    selectResearchInstrument,
    selectScreenPreset,
    selectInstitutionContext,
    selectPredictionSeries,
    selectPredictionEvent,
    selectPredictionContract,
    selectPredictionContractView,
    selectScreenContext,
    queryMarket,
    sectionMarketOptions,
    showSectionMarketSwitch,
    visibleOperations,
    handleMarketPaneResized,
    optionResearchOperation,
    macroResearchOperation,
    arkResearchOperation,
    derivativeScreenOperation,
    instrumentResearchOperation,
    activeFeatureIDs,
    handleMarketSelect,
    selectRailTarget,
    selectQuotePeriod,
    selectQuoteTab,
    handleMarketMore,
    openWorkspaceInstrument,
    openQuoteTargetInWorkspace,
    openOptionResearchInstrument,
    researchEntry,
    selectResearchEntry,
    openResearchEntry,
  };
}
