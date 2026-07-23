<script setup lang="ts">
import type { SplitpanesResizedPayload } from "splitpanes";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import OptionResearchPanel from "../components/product/OptionResearchPanel.vue";
import PredictionResearchPanel from "../components/product/PredictionResearchPanel.vue";
import ProductFeaturePanel from "../components/product/ProductFeaturePanel.vue";
import ProductPanelToolbar from "../components/product/ProductPanelToolbar.vue";
import ConceptSectorView from "../components/research/ConceptSectorView.vue";
import EarningsCalendarView from "../components/research/EarningsCalendarView.vue";
import EconCalendarView from "../components/research/EconCalendarView.vue";
import InstitutionGridView from "../components/research/InstitutionGridView.vue";
import IpoCenterView from "../components/research/IpoCenterView.vue";
import MarketHomeView from "../components/research/MarketHomeView.vue";
import MarketRankingsView from "../components/research/MarketRankingsView.vue";
import QuoteDetailRail from "../components/research/QuoteDetailRail.vue";
import {
  isQuoteWorkbenchPeriod,
  normalizeQuoteWorkbenchProductClass,
  type QuoteWorkbenchPeriod,
  type QuoteWorkbenchTab,
} from "../components/domain/market-data/quoteWorkbench";
import {
  LEGACY_MARKET_VIEW_REDIRECTS,
  MARKET_CODE_OPTIONS,
  MARKET_VIEWS,
  RESEARCH_SECTIONS,
  type MarketView,
  type ResearchSection,
  researchFeatureIds,
  researchInstrumentActionClasses,
  validMarketCode,
  validMarketView,
  validResearchSection,
} from "../components/research/researchNavigation";
import {
  normalizeResearchQuoteTarget,
  researchQuoteTargetFromEntry,
  type ResearchQuoteTarget,
} from "../components/research/researchQuote";
import BrokerProviderTag from "../components/shared/BrokerProviderTag.vue";
import SplitPane from "../components/shared/SplitPane.vue";
import SplitPaneItem from "../components/shared/SplitPaneItem.vue";
import { useBrokerProviderSelection } from "../composables/brokerProviderSelection";
import { productCompactMenuProps } from "../composables/productControlDensity";
import { useConsoleData } from "../composables/useConsoleData";
import {
  clampResearchPaneSizesForWidth,
  readResearchViewState,
  researchPaneBoundsForWidth,
  writeResearchViewState,
} from "../composables/useResearchViewState";
import { useWorkspaceTradingPrefs } from "../composables/useWorkspaceLayout";

const sections = RESEARCH_SECTIONS;

const route = useRoute();
const router = useRouter();
const { prefs: workspacePrefs, update } = useWorkspaceTradingPrefs();
const { selectedBrokerAccount, systemStatus } = useConsoleData();
const { selectedBrokerId } = useBrokerProviderSelection();
function configFor(section: ResearchSection) {
  return sections.find((item) => item.value === section)!;
}

function operationFor(section: ResearchSection, value: unknown): string {
  const config = configFor(section);
  const candidate = String(value ?? "");
  return config.operations.some((item) => item.value === candidate)
    ? candidate
    : config.operations[0]?.value ?? "";
}

function firstQueryValue(value: unknown): string {
  if (Array.isArray(value)) return String(value[0] ?? "").trim();
  return String(value ?? "").trim();
}

function quotePeriodFromQuery(value: unknown): QuoteWorkbenchPeriod {
  const candidate = firstQueryValue(value);
  return isQuoteWorkbenchPeriod(candidate) ? candidate : "day";
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
const selectedQuoteTarget = ref<ResearchQuoteTarget | null>(initialQuoteTarget);
const selectedQuoteEntry = ref<Record<string, unknown> | null>(null);
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
  selectedQuoteEntry.value = null;
  selectedQuotePeriod.value = "day";
  selectedQuoteTab.value = "quote";
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

function redirectLegacyRoute(): boolean {
  const requestedSection = String(route.query.section ?? "");
  if (requestedSection === "options") {
    void router.replace({
      query: queryWith({ section: "derivatives", ...emptyQuoteQuery }),
    });
    return true;
  }
  if (requestedSection !== "market") return false;
  const legacy = LEGACY_MARKET_VIEW_REDIRECTS[String(route.query.view ?? "")];
  if (legacy == null) return false;
  void router.replace({
    query: queryWith({
      section: legacy.section,
      operation: legacy.operation,
      view: undefined,
      ...emptyQuoteQuery,
    }),
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
      ...emptyQuoteQuery,
    }),
  });
}

watch(
  () => [route.query.section, route.query.operation, route.query.view] as const,
  ([sectionValue, operationValue]) => {
    if (redirectLegacyRoute()) return;
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
    selectedQuoteEntry.value = null;
    selectedQuotePeriod.value = quotePeriodFromQuery(route.query.quotePeriod);
    selectedQuoteTab.value = quoteTabFromQuery(target);
  },
  { immediate: true },
);

const queryMarket = computed(() => {
  if (activeSection.value === "market" || activeSection.value === "calendar") {
    return activeMarketCode.value;
  }
  if (activeSection.value === "derivatives") {
    return activeOperation.value === "warrant" ? "HK" : "US";
  }
  if (activeSection.value === "institutions") {
    return activeMarketCode.value === "HK" ? "HK" : "US";
  }
  if (activeSection.value === "instrument") {
    return workspacePrefs.value.market;
  }
  return "US";
});

const sectionMarketOptions = computed(() =>
  activeSection.value === "institutions"
    ? MARKET_CODE_OPTIONS.filter((item) => item.value !== "CN")
    : MARKET_CODE_OPTIONS,
);
const showSectionMarketSwitch = computed(() =>
  ["calendar", "institutions"].includes(activeSection.value));
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

function replacePathMarket(path: string, market: string): string {
  if (!path || !path.includes("?")) return path;
  const [pathname, query = ""] = path.split("?", 2);
  const params = new URLSearchParams(query);
  params.set("market", market);
  return `${pathname}?${params}`;
}

const activePath = computed(() => {
  const template =
    activeConfig.value.operations.find(
      (operation) => operation.value === activeOperation.value,
    )?.path ?? activeConfig.value.operations[0]?.path ?? "";
  const instrumentId = `${workspacePrefs.value.market}.${workspacePrefs.value.symbol}`
    .trim()
    .toUpperCase();
  let path = replacePathMarket(
    template.replace(":instrumentId", encodeURIComponent(instrumentId)),
    queryMarket.value,
  );
  if (activeSection.value === "calendar" && activeOperation.value === "dividends") {
    const today = new Date();
    const date = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, "0")}-${String(today.getDate()).padStart(2, "0")}`;
    path += `${path.includes("?") ? "&" : "?"}date=${date}`;
  }
  return path;
});

const optionResearchOperation = computed(
  () =>
    (["unusual", "zero_dte", "earnings", "seller"].includes(
      activeOperation.value,
    )
      ? activeOperation.value
      : "unusual") as "unusual" | "zero_dte" | "earnings" | "seller",
);
const activeFeatureIDs = computed(() => researchFeatureIds(
  activeSection.value, activeOperation.value, activeMarketView.value));
function handleMarketSelect(entry: Record<string, unknown>): void {
  const target = researchQuoteTargetFromEntry(
    entry,
    activeMarketCode.value,
  );
  selectedQuoteTarget.value = target;
  selectedQuoteEntry.value = target == null ? null : entry;
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
  selectedQuoteEntry.value = null;
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

const quoteableProductClasses = new Set([
  "equity",
  "stock",
  "fund",
  "etf",
  "trust",
  "index",
  "warrant",
  "cbbc",
  "plate",
]);

const featureActionClasses = computed(() =>
  researchInstrumentActionClasses(activeSection.value, activeOperation.value));

function inferredFeatureProductClass(
  entry: Record<string, unknown> | undefined,
): string {
  const explicit = String(
    entry?.productClass ?? entry?.securityType ?? entry?.type ?? "",
  )
    .trim()
    .toLowerCase();
  if (explicit) return explicit;
  return "unknown";
}

function openFeatureInstrument(
  instrumentID: string,
  entry?: Record<string, unknown>,
): void {
  const productClass = inferredFeatureProductClass(entry);
  if (!quoteableProductClasses.has(productClass)) {
    if (productClass === "option") {
      openWorkspaceInstrument(instrumentID, "derivatives", "option");
    }
    return;
  }
  handleMarketSelect({
    ...(entry ?? {}),
    instrumentId: instrumentID,
    productClass,
  });
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
</script>

<template>
  <div
    ref="researchPageRef"
    class="research-page"
    :data-capability-surface="activeConfig.surfaceId"
  >
    <SplitPane
      class="research-page__shell"
      :class="{
        'is-drawer': marketRailDrawer,
        'is-rail-collapsed': marketRailCollapsed,
      }"
      :pane-min-size="8"
      @resized="handleMarketPaneResized"
    >
      <SplitPaneItem
        :size="marketRailCollapsed ? 100 : marketPaneSizes[0]"
        :min-size="marketRailCollapsed ? 100 : researchPaneBounds.leftMinSize"
        :max-size="marketRailCollapsed ? 100 : researchPaneBounds.leftMaxSize"
      >
        <section class="research-page__center">
          <ProductPanelToolbar
            title="研究中心"
            description="真实 OpenD 研究数据与统一行情侧栏"
          >
            <BrokerProviderTag
              :feature-id="activeFeatureIDs[0]"
              :feature-ids="activeFeatureIDs"
              :market="queryMarket"
              :preferred-broker-id="selectedBrokerAccount?.brokerId"
              :default-broker-id="systemStatus.defaultBroker"
            />
          </ProductPanelToolbar>

          <div class="research-page__navigation">
            <v-tabs
              :model-value="activeSection"
              density="compact"
              show-arrows
              @update:model-value="selectSection"
            >
              <v-tab
                v-for="section in sections"
                :key="section.value"
                :value="section.value"
                :data-capability-surface="section.surfaceId"
              >
                {{ section.label }}
              </v-tab>
            </v-tabs>
          </div>

          <div
            v-if="activeSection === 'market'"
            class="research-page__market-nav"
          >
            <span class="tv-seg research-page__market-views">
              <button
                v-for="view in MARKET_VIEWS"
                :key="view.value"
                type="button"
                :class="{ 'is-active': activeMarketView === view.value }"
                @click="activeMarketView = view.value"
              >
                {{ view.label }}
              </button>
            </span>
            <span class="tv-seg research-page__market-codes">
              <button
                v-for="option in MARKET_CODE_OPTIONS"
                :key="option.value"
                type="button"
                :class="{ 'is-active': activeMarketCode === option.value }"
                @click="selectMarket(option.value)"
              >
                {{ option.label }}
              </button>
            </span>
          </div>
          <div v-else class="research-page__capabilities">
            <div class="research-page__context">
              <span>当前研究域</span>
              <strong>{{ activeConfig.label }}</strong>
              <small>{{ activeConfig.description }}</small>
            </div>
            <div class="research-page__chips">
              <v-chip
                v-for="capability in activeConfig.capabilities"
                :key="capability"
                size="small"
                variant="tonal"
              >
                {{ capability }}
              </v-chip>
            </div>
            <span
              v-if="showSectionMarketSwitch"
              class="tv-seg research-page__section-markets"
            >
              <button
                v-for="option in sectionMarketOptions"
                :key="option.value"
                type="button"
                :class="{ 'is-active': activeMarketCode === option.value }"
                @click="selectMarket(option.value)"
              >{{ option.label }}</button>
            </span>
            <v-select
              v-if="activeSection !== 'prediction'"
              :model-value="activeOperation"
              class="research-page__operation product-compact-control"
              :items="visibleOperations"
              :menu-props="productCompactMenuProps"
              item-title="label"
              item-value="value"
              label="数据视图"
              density="compact"
              variant="outlined"
              hide-details
              @update:model-value="selectOperation"
            />
          </div>

          <main class="research-page__body">
            <div class="research-page__content">
              <MarketHomeView
                v-if="activeSection === 'market' && activeMarketView === 'home'"
                :market="activeMarketCode"
                :broker-id="selectedBrokerId"
                @select="handleMarketSelect"
                @more="handleMarketMore"
              />
              <MarketRankingsView
                v-else-if="activeSection === 'market' && activeMarketView === 'rankings'"
                :market="activeMarketCode"
                :broker-id="selectedBrokerId"
                :initial-operation="rankingInitialOperation"
                @select="handleMarketSelect"
              />
              <ConceptSectorView
                v-else-if="activeSection === 'market'"
                :market="activeMarketCode"
                :broker-id="selectedBrokerId"
                @select="handleMarketSelect"
              />
              <EarningsCalendarView
                v-else-if="activeSection === 'calendar' && activeOperation === 'earnings'"
                :market="queryMarket"
                :broker-id="selectedBrokerId"
                @select="handleMarketSelect"
              />
              <EconCalendarView
                v-else-if="activeSection === 'calendar' && activeOperation === 'economic'"
                :market="queryMarket"
                :broker-id="selectedBrokerId"
              />
              <IpoCenterView
                v-else-if="activeSection === 'calendar' && activeOperation === 'ipos'"
                :market="queryMarket"
                :broker-id="selectedBrokerId"
                @select="handleMarketSelect"
              />
              <InstitutionGridView
                v-else-if="activeSection === 'institutions' && ['list', 'holding_changes'].includes(activeOperation)"
                :market="queryMarket"
                :broker-id="selectedBrokerId"
                :operation="activeOperation === 'holding_changes' ? 'holding_changes' : 'list'"
                @select="handleMarketSelect"
              />
              <PredictionResearchPanel
                v-else-if="activeSection === 'prediction'"
                @open-instrument="openWorkspaceInstrument"
              />
              <OptionResearchPanel
                v-else-if="activeSection === 'derivatives' && !['option_screen', 'warrant'].includes(activeOperation)"
                market="US"
                :operation="optionResearchOperation"
                scope="market"
                @open-instrument="openOptionResearchInstrument"
              />
              <ProductFeaturePanel
                v-else
                :key="`${activeConfig.value}:${activeOperation}:${activePath}`"
                :title="activeConfig.label"
                :description="activeConfig.description"
                :path="activePath"
                :action-label="activeSection === 'derivatives' && activeOperation === 'option_screen' ? '工作区' : '打开行情'"
                :instrument-action-classes="featureActionClasses"
                @open-instrument="openFeatureInstrument"
              />
            </div>
          </main>
        </section>
        <button
          v-if="marketRailDrawer && !marketRailCollapsed"
          type="button"
          class="research-page__rail-backdrop"
          aria-label="关闭行情详情"
          @click="marketRailCollapsed = true"
        />
        <button
          v-if="marketRailCollapsed"
          type="button"
          class="research-page__rail-toggle is-collapsed-toggle"
          title="展开行情详情"
          @click="marketRailCollapsed = false"
        >‹</button>
      </SplitPaneItem>
      <SplitPaneItem
        v-if="!marketRailCollapsed"
        :size="marketPaneSizes[1]"
        :min-size="researchPaneBounds.railMinSize"
        :max-size="researchPaneBounds.railMaxSize"
      >
        <aside class="research-page__market-rail">
          <button
            type="button"
            class="research-page__rail-toggle"
            title="收起行情详情"
            @click="marketRailCollapsed = true"
          >›</button>
          <QuoteDetailRail
            :target="selectedQuoteTarget"
            :entry="selectedQuoteEntry"
            :broker-id="selectedBrokerId"
            :visible="!marketRailCollapsed"
            :drawer="marketRailDrawer"
            :period="selectedQuotePeriod"
            :tab="selectedQuoteTab"
            @update:period="selectQuotePeriod"
            @update:tab="selectQuoteTab"
            @select="selectRailTarget"
            @open-workspace="openQuoteTargetInWorkspace"
            @close="marketRailCollapsed = true"
          />
        </aside>
      </SplitPaneItem>
    </SplitPane>
  </div>
</template>

<style scoped>
.research-page {
  min-height: 0;
  height: 100%;
  overflow: hidden;
  background: var(--tv-bg-app);
  color: var(--tv-text);
}

.research-page__shell {
  position: relative;
  min-height: 0;
  height: 100%;
}

.research-page__shell :deep(.splitpanes__pane) {
  position: relative;
  min-width: 0;
  min-height: 0;
  overflow: visible;
}

.research-page__shell :deep(.splitpanes__splitter) {
  z-index: 4;
}

.research-page__center {
  display: flex;
  min-width: 0;
  min-height: 0;
  height: 100%;
  flex-direction: column;
  overflow: hidden;
}

.research-page__navigation {
  min-height: 43px;
  flex: 0 0 auto;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.research-page__navigation :deep(.v-tab) {
  min-width: 76px;
  height: 42px;
  color: var(--tv-text-muted);
  font-size: 10px;
  font-weight: 650;
  letter-spacing: 0;
  text-transform: none;
}

.research-page__navigation :deep(.v-tab--selected) {
  color: var(--tv-text);
}

.research-page__navigation :deep(.v-tab__slider) {
  height: 2px;
  background: var(--tv-accent);
}

.research-page__capabilities {
  display: flex;
  min-height: 58px;
  flex: 0 0 auto;
  align-items: center;
  gap: 12px;
  padding: 7px 16px;
  overflow-x: auto;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.research-page__context {
  display: grid;
  min-width: 190px;
  grid-template-columns: auto 1fr;
  column-gap: 7px;
}

.research-page__context span {
  align-self: center;
  grid-row: span 2;
  color: var(--tv-text-dim);
  font-size: 8px;
  writing-mode: vertical-rl;
}

.research-page__context strong {
  font-size: 11px;
}

.research-page__context small {
  color: var(--tv-text-muted);
  font-size: 8px;
  white-space: nowrap;
}

.research-page__chips {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 5px;
  overflow-x: auto;
}

.research-page__chips :deep(.v-chip) {
  border: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  font-size: 8px;
}

.research-page__operation {
  min-width: 158px;
  max-width: 190px;
  margin-left: auto;
}

.research-page__body {
  min-height: 0;
  flex: 1;
  padding: 8px;
  overflow: hidden;
}

/* ---- 市场 section：二级导航 + 视图区 + 右侧行情详情栏 ---- */
.research-page__market-nav {
  display: flex;
  min-height: 44px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 6px 16px;
  overflow-x: auto;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.research-page__content {
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
}

.research-page__market-rail {
  position: relative;
  display: flex;
  width: 100%;
  height: 100%;
  min-height: 0;
  flex-direction: column;
}

.research-page__market-rail :deep(.quote-detail-rail) {
  width: 100%;
  max-width: none;
}

.research-page__rail-backdrop { display: none; }

.research-page__rail-toggle {
  position: absolute;
  z-index: 5;
  top: 6px;
  left: 0;
  transform: translateX(-50%);
  display: grid;
  width: 20px;
  height: 44px;
  place-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 4px 0 0 4px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  cursor: pointer;
  font-size: 12px;
  line-height: 1;
}

.research-page__rail-toggle.is-collapsed-toggle {
  right: 0;
  left: auto;
  transform: none;
  border-radius: 4px;
}

.research-page__rail-toggle:hover { color: var(--tv-text); }

.research-page__shell.is-drawer {
  display: block !important;
}

.research-page__shell.is-drawer :deep(.splitpanes__splitter) {
  display: none;
}

.research-page__shell.is-drawer
  > :deep(.splitpanes__pane:first-child) {
  width: 100% !important;
}

.research-page__shell.is-drawer:not(.is-rail-collapsed)
  > :deep(.splitpanes__pane:last-child) {
    position: absolute;
    z-index: 42;
    top: 0;
    right: 0;
    bottom: 0;
    width: min(520px, calc(100% - 32px)) !important;
    height: auto;
    background: var(--tv-bg-app);
    box-shadow: -12px 0 28px rgb(0 0 0 / 32%);
}

.research-page__shell.is-drawer .research-page__rail-backdrop {
  position: absolute;
  z-index: 41;
  inset: 0;
  display: block;
  border: 0;
  background: rgb(0 0 0 / 42%);
}
</style>
