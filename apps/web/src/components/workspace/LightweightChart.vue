<script setup lang="ts">
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";

import KlineChart from "../KlineChart.vue";
import KlineIndicatorSelector from "../KlineIndicatorSelector.vue";
import MarketFeedStatus from "../domain/market-data/MarketFeedStatus.vue";
import {
  KLINE_CHART_TYPES,
  KLINE_PERIODS,
  normalizeChartType,
  normalizeKlinePeriod,
  overlayRealtimeTickCandle,
  type ChartType,
  type KlineCandle,
  type KlineIndicatorKey,
} from "../../charting/kline";
import {
  brokerSupportedChartPeriods,
  useBrokerProviderSelection,
} from "../../composables/brokerProviderSelection";
import { getSharedLiveSocketHub } from "../../composables/sharedLiveSocket";
import { useConsoleData } from "../../composables/useConsoleData";
import { useWorkspaceTradingPrefs } from "../../composables/useWorkspaceLayout";

type RenderableKlinePeriod = (typeof KLINE_PERIODS)[number]["value"];

const props = withDefaults(
  defineProps<{
    target?: { market: string; symbol: string } | null;
    period?: string;
    variant?: "workspace" | "embedded";
    minHeight?: number;
  }>(),
  {
    period: "1d",
    variant: "workspace",
    minHeight: 320,
  },
);

const emit = defineEmits<{
  "update:period": [period: RenderableKlinePeriod];
}>();

const { prefs, update } = useWorkspaceTradingPrefs();
const {
  brokerDescriptors,
  selectedBrokerId,
  loadBrokerProviders,
  loading: isLoadingBrokerCapabilities,
  loadError: brokerCapabilitiesError,
} = useBrokerProviderSelection();
const {
  currentMarketDataCandles: marketDataCandles,
  currentMarketDataSnapshot: marketDataSnapshot,
  marketDataQueryMarket,
  marketDataQuerySymbol,
  marketDataQueryPeriod,
  marketDataQueryError,
  isLoadingMarketDataQuery,
  isLoadingOlderMarketData,
  hasMoreMarketDataHistory,
  marketDataNextBefore,
  marketDataOlderError,
  loadMarketDataQuery,
  selectMarketDataInstrument,
  selectWorkspaceInstrument,
  acquireMarketDataSubscription,
  createStableWebConsumerId,
  heartbeatMarketDataConsumer,
  releaseMarketDataSubscription,
  activeMarketDataInstrumentId,
  isMarketDataStale,
  isLiveStreamConnected,
} = useConsoleData();

const controlled = computed(() => props.target !== undefined);
const selectedIndicators = ref<KlineIndicatorKey[]>(["volume"]);
const chartTypeTriggerRef = ref<HTMLElement | null>(null);
const chartTypePanelRef = ref<HTMLElement | null>(null);
const chartTypeOptionRefs = ref<Array<HTMLButtonElement | null>>([]);
const isChartTypeMenuOpen = ref(false);
const chartTypePanelTop = ref(0);
const chartTypePanelLeft = ref(0);
const controlledChartType = ref<ChartType>("standard");
const targetMarket = computed(() =>
  (controlled.value ? props.target?.market : prefs.value.market)
    ?.trim()
    .toUpperCase() ?? "",
);
const targetSymbol = computed(() =>
  (controlled.value ? props.target?.symbol : prefs.value.symbol)
    ?.trim()
    .toUpperCase() ?? "",
);
const renderablePeriods = new Set<string>(
  KLINE_PERIODS.map((period) => period.value),
);
const chartConsumerId = createStableWebConsumerId(
  controlled.value ? "embedded-chart" : "workspace-chart",
);
const liveHub = getSharedLiveSocketHub();
let heldChartSubscription: {
  brokerId: string;
  market: string;
  symbol: string;
  channel: "KLINE" | "TICK";
  interval: string;
} | null = null;
let heartbeatTimer = 0;
let reloadInFlight: { key: string; promise: Promise<void> } | null = null;
let chartReloadSeq = 0;

const supportedPeriodValues = computed(() =>
  brokerSupportedChartPeriods(
    selectedBrokerId.value,
    targetMarket.value,
    brokerDescriptors.value,
  ),
);
const periods = computed(() => {
  const supported = new Set(supportedPeriodValues.value ?? []);
  return KLINE_PERIODS.filter((period) => supported.has(period.value));
});
const hasResolvedPeriodCapabilities = computed(
  () =>
    !isLoadingBrokerCapabilities.value &&
    supportedPeriodValues.value != null,
);
const hasSupportedChartPeriod = computed(() => periods.value.length > 0);
const isTickChartPeriod = computed(
  () => normalizedSelectedPeriod() === "tick",
);
const activeChartType = computed<ChartType>(() => {
  if (isTickChartPeriod.value) return "standard";
  return controlled.value
    ? controlledChartType.value
    : normalizeChartType(prefs.value.chartType);
});
const activeChartTypeLabel = computed(
  () =>
    KLINE_CHART_TYPES.find((option) => option.value === activeChartType.value)
      ?.label ?? "标准 K 线",
);

const CHART_TYPE_PANEL_GAP = 4;
const CHART_TYPE_VIEWPORT_GAP = 8;

function normalizedRenderablePeriod(value: string): RenderableKlinePeriod | "" {
  try {
    const normalized = normalizeKlinePeriod(value);
    return renderablePeriods.has(normalized)
      ? (normalized as RenderableKlinePeriod)
      : "";
  } catch {
    return "";
  }
}

function normalizedSelectedPeriod(): RenderableKlinePeriod | "" {
  return normalizedRenderablePeriod(
    controlled.value ? props.period : prefs.value.period,
  );
}

function fallbackPeriod(values: readonly string[]): RenderableKlinePeriod {
  for (const candidate of ["1m", "5m", "1d"]) {
    if (values.includes(candidate)) return candidate as RenderableKlinePeriod;
  }
  return (
    values.find((period) => period !== "tick") ?? "tick"
  ) as RenderableKlinePeriod;
}

function commitPeriod(period: RenderableKlinePeriod): void {
  if (controlled.value) {
    emit("update:period", period);
    return;
  }
  update({ period });
}

function reconcileSelectedPeriod(): void {
  const supported = (supportedPeriodValues.value ?? []).filter((period) =>
    renderablePeriods.has(period),
  );
  const current = normalizedSelectedPeriod();
  if (supported.length > 0 && !supported.includes(current)) {
    commitPeriod(fallbackPeriod(supported));
  }
}

const chartTarget = computed(() => {
  const preferredPeriod = normalizedSelectedPeriod();
  const period = periods.value.some(
    (candidate) => candidate.value === preferredPeriod,
  )
    ? preferredPeriod
    : "";
  return {
    brokerId: selectedBrokerId.value,
    market: targetMarket.value,
    symbol: targetSymbol.value,
    period,
    instrumentId:
      targetMarket.value === "" || targetSymbol.value === ""
        ? ""
        : `${targetMarket.value}.${targetSymbol.value}`,
    channel: (period === "tick" ? "TICK" : "KLINE") as "TICK" | "KLINE",
    interval: period,
  };
});
const chartCandles = computed<KlineCandle[]>(() =>
  overlayRealtimeTickCandle(
    marketDataCandles.value?.candles ?? [],
    marketDataSnapshot.value?.snapshot ?? null,
    marketDataQueryPeriod.value,
  ),
);
const chartObservedAt = computed(() => {
  const snapshot = marketDataSnapshot.value?.snapshot;
  if (snapshot?.observedAt || snapshot?.at) return snapshot.observedAt ?? snapshot.at;
  const candle = marketDataCandles.value?.candles.at(-1);
  return candle?.displayAt ?? candle?.at ?? marketDataCandles.value?.meta.resolvedAt ?? null;
});
const chartConnectionState = computed(() =>
  liveHub.connectionState?.value ?? (isLiveStreamConnected.value ? "connected" : "disconnected"),
);
const chartTransportMode = computed(() =>
  liveHub.lastHeartbeatEvent?.value?.transport?.mode ?? null,
);
const chartSource = computed(() =>
  marketDataSnapshot.value?.meta.source ?? marketDataCandles.value?.meta.source ?? null,
);
const chartFromCache = computed(() =>
  marketDataSnapshot.value?.meta.fromCache ?? marketDataCandles.value?.meta.fromCache ?? false,
);
const historyLoadStatus = computed(() => {
  if (chartTarget.value.period === "tick" || chartTarget.value.period === "") {
    return "";
  }
  if (isLoadingOlderMarketData.value) return "正在加载更早数据";
  if (marketDataOlderError.value) return "加载失败，拖动或点击重试";
  if (
    marketDataCandles.value != null &&
    !isLoadingMarketDataQuery.value &&
    !hasMoreMarketDataHistory.value
  ) {
    return "已到最早数据";
  }
  return "";
});

function resolveChartSubscriptionTarget() {
  return chartTarget.value;
}

function chartTargetKey(
  target: ReturnType<typeof resolveChartSubscriptionTarget>,
): string {
  return JSON.stringify(target);
}

async function reload(options: { preserveExisting?: boolean } = {}): Promise<void> {
  const target = resolveChartSubscriptionTarget();
  const reloadKey = chartTargetKey(target);
  if (reloadInFlight != null && reloadInFlight.key === reloadKey) {
    return reloadInFlight.promise;
  }

  const requestSeq = ++chartReloadSeq;
  const promise = (async () => {
    if (target.period === "") {
      await syncChartSubscription(target, requestSeq);
      return;
    }
    const selectInstrument = controlled.value
      ? selectMarketDataInstrument
      : selectWorkspaceInstrument;
    selectInstrument({
      market: target.market,
      symbol: target.symbol,
      period: target.period,
    });
    await syncChartSubscription(target, requestSeq);
    if (requestSeq !== chartReloadSeq) {
      return;
    }
    await loadMarketDataQuery(
      options.preserveExisting == null
        ? {}
        : { preserveExisting: options.preserveExisting },
    );
  })();
  reloadInFlight = { key: reloadKey, promise };

  try {
    await promise;
  } finally {
    if (reloadInFlight?.promise === promise) {
      reloadInFlight = null;
    }
  }
}

function handleChartVisibilityChange(): void {
  if (typeof document !== "undefined" && document.visibilityState === "hidden") {
    return;
  }
  const target = chartTarget.value;
  const hasLoadedCurrentTarget =
    target.instrumentId !== "" &&
    activeMarketDataInstrumentId.value === target.instrumentId &&
    marketDataCandles.value != null;

  // Wait for the WebSocket reconnect (triggered by AppShell) before deciding
  // the recovery path, so we don't trigger a full reload while reconnecting.
  void liveHub.waitForConnection(3_000).then((connected) => {
    // A background tab can outlive the server-side lease. Re-acquire the exact
    // target whenever the page becomes visible, even if cached data is fresh.
    if (
      connected &&
      isLiveStreamConnected.value &&
      !isMarketDataStale(30_000) &&
      hasLoadedCurrentTarget
    ) {
      void syncChartSubscription(target);
      return;
    }

    // SSE connected but data stale → reload with preserveExisting to keep accumulated state
    if (
      connected &&
      isLiveStreamConnected.value &&
      hasLoadedCurrentTarget &&
      !isMarketDataStale(120_000)
    ) {
      void reload({ preserveExisting: true });
      return;
    }

    // SSE disconnected or data very stale → preserve the visible chart while
    // re-confirming its lease and refreshing transport state.
    if (hasLoadedCurrentTarget) {
      void reload({ preserveExisting: true });
      return;
    }

    void reload();
  });
}

function handleChartOnline(): void {
  void reload();
}

async function syncChartSubscription(
  next: ReturnType<typeof resolveChartSubscriptionTarget>,
  requestSeq = chartReloadSeq,
): Promise<void> {
  if (
    heldChartSubscription != null &&
    (heldChartSubscription.market !== next.market ||
      heldChartSubscription.brokerId !== next.brokerId ||
      heldChartSubscription.symbol !== next.symbol ||
      heldChartSubscription.channel !== next.channel ||
      heldChartSubscription.interval !== next.interval)
  ) {
    await releaseMarketDataSubscription({
      consumerId: chartConsumerId,
      ...(heldChartSubscription.brokerId
        ? { brokerId: heldChartSubscription.brokerId }
        : {}),
      market: heldChartSubscription.market,
      symbol: heldChartSubscription.symbol,
      channel: heldChartSubscription.channel,
      ...(heldChartSubscription.channel === "KLINE"
        ? { interval: heldChartSubscription.interval }
        : {}),
    });
    heldChartSubscription = null;
  }

  if (next.market === "" || next.symbol === "" || next.period === "") {
    return;
  }

  const acquired = await acquireMarketDataSubscription({
    consumerId: chartConsumerId,
    ...(next.brokerId ? { brokerId: next.brokerId } : {}),
    market: next.market,
    symbol: next.symbol,
    channel: next.channel,
    ...(next.channel === "KLINE" ? { interval: next.interval } : {}),
  });
  if (!acquired) {
    return;
  }
  if (requestSeq !== chartReloadSeq) {
    await releaseMarketDataSubscription({
      consumerId: chartConsumerId,
      ...(next.brokerId ? { brokerId: next.brokerId } : {}),
      market: next.market,
      symbol: next.symbol,
      channel: next.channel,
      ...(next.channel === "KLINE" ? { interval: next.interval } : {}),
    });
    return;
  }

  await heartbeatChartSubscription(next.brokerId);
  if (requestSeq !== chartReloadSeq) {
    await releaseMarketDataSubscription({
      consumerId: chartConsumerId,
      ...(next.brokerId ? { brokerId: next.brokerId } : {}),
      market: next.market,
      symbol: next.symbol,
      channel: next.channel,
      ...(next.channel === "KLINE" ? { interval: next.interval } : {}),
    });
    return;
  }

  heldChartSubscription = next;
}

function heartbeatChartSubscription(brokerId: string): Promise<void> {
  return brokerId
    ? heartbeatMarketDataConsumer(chartConsumerId, brokerId)
    : heartbeatMarketDataConsumer(chartConsumerId);
}

function setPeriod(period: RenderableKlinePeriod): void {
  commitPeriod(period);
}

function syncChartTypePanelPosition(): void {
  const trigger = chartTypeTriggerRef.value;
  const panel = chartTypePanelRef.value;
  if (trigger == null || panel == null || typeof window === "undefined") {
    return;
  }

  const triggerRect = trigger.getBoundingClientRect();
  const panelRect = panel.getBoundingClientRect();
  const maxLeft = Math.max(
    CHART_TYPE_VIEWPORT_GAP,
    window.innerWidth - panelRect.width - CHART_TYPE_VIEWPORT_GAP,
  );
  chartTypePanelLeft.value = Math.min(
    Math.max(triggerRect.left, CHART_TYPE_VIEWPORT_GAP),
    maxLeft,
  );

  const below = triggerRect.bottom + CHART_TYPE_PANEL_GAP;
  const above = triggerRect.top - panelRect.height - CHART_TYPE_PANEL_GAP;
  const maxTop = Math.max(
    CHART_TYPE_VIEWPORT_GAP,
    window.innerHeight - panelRect.height - CHART_TYPE_VIEWPORT_GAP,
  );
  chartTypePanelTop.value =
    below + panelRect.height <= window.innerHeight - CHART_TYPE_VIEWPORT_GAP
      ? below
      : above >= CHART_TYPE_VIEWPORT_GAP
        ? above
        : Math.min(Math.max(below, CHART_TYPE_VIEWPORT_GAP), maxTop);
}

async function toggleChartTypeMenu(): Promise<void> {
  isChartTypeMenuOpen.value = !isChartTypeMenuOpen.value;
  if (!isChartTypeMenuOpen.value) return;
  await nextTick();
  syncChartTypePanelPosition();
  focusChartTypeOption(activeChartType.value);
}

function closeChartTypeMenu(options?: { restoreTriggerFocus?: boolean }): void {
  isChartTypeMenuOpen.value = false;
  if (options?.restoreTriggerFocus) {
    chartTypeTriggerRef.value?.focus();
  }
}

function setChartTypeOptionRef(index: number, element: unknown): void {
  chartTypeOptionRefs.value[index] =
    element instanceof HTMLButtonElement ? element : null;
}

function enabledChartTypeOptions(): HTMLButtonElement[] {
  return chartTypeOptionRefs.value.filter(
    (option): option is HTMLButtonElement => option != null && !option.disabled,
  );
}

function focusChartTypeOption(chartType: ChartType): void {
  const options = enabledChartTypeOptions();
  const selected = options.find(
    (option) => option.dataset.chartType === chartType,
  );
  (selected ?? options[0])?.focus();
}

function selectChartType(chartType: ChartType): void {
  if (chartType === "heikinashi" && isTickChartPeriod.value) {
    return;
  }
  if (controlled.value) {
    controlledChartType.value = chartType;
  } else {
    update({ chartType });
  }
  closeChartTypeMenu({ restoreTriggerFocus: true });
}

function handleChartTypeDocumentPointerDown(event: PointerEvent): void {
  const target = event.target;
  if (!(target instanceof Node)) return;
  if (
    !(chartTypeTriggerRef.value?.contains(target) ?? false) &&
    !(chartTypePanelRef.value?.contains(target) ?? false)
  ) {
    closeChartTypeMenu();
  }
}

function handleChartTypeDocumentKeydown(event: KeyboardEvent): void {
  if (!isChartTypeMenuOpen.value) return;

  if (event.key === "Escape") {
    event.preventDefault();
    closeChartTypeMenu({ restoreTriggerFocus: true });
    return;
  }

  if (
    event.key !== "ArrowDown" &&
    event.key !== "ArrowUp" &&
    event.key !== "Home" &&
    event.key !== "End"
  ) {
    return;
  }

  const options = enabledChartTypeOptions();
  if (options.length === 0) return;
  event.preventDefault();

  const activeIndex = options.findIndex(
    (option) => option === document.activeElement,
  );
  let nextIndex = 0;
  if (event.key === "ArrowDown") {
    nextIndex = activeIndex < 0 ? 0 : (activeIndex + 1) % options.length;
  } else if (event.key === "ArrowUp") {
    nextIndex =
      activeIndex < 0
        ? options.length - 1
        : (activeIndex - 1 + options.length) % options.length;
  } else if (event.key === "End") {
    nextIndex = options.length - 1;
  }
  options[nextIndex]?.focus();
}

function handleCompactPeriodChange(event: Event): void {
  const value = (event.currentTarget as HTMLSelectElement).value;
  const period = normalizedRenderablePeriod(value);
  if (period !== "") setPeriod(period);
}

async function handleLoadMore(): Promise<void> {
  const target = chartTarget.value;
  if (
    target.period === "" ||
    target.period === "tick" ||
    isLoadingOlderMarketData.value ||
    !hasMoreMarketDataHistory.value ||
    marketDataNextBefore.value === ""
  ) {
    return;
  }
  await loadMarketDataQuery({
    appendOlder: true,
    before: marketDataNextBefore.value,
  });
}

async function retryBrokerCapabilities(): Promise<void> {
  await loadBrokerProviders(true);
  reconcileSelectedPeriod();
}

onMounted(() => {
  document.addEventListener("visibilitychange", handleChartVisibilityChange);
  document.addEventListener("pointerdown", handleChartTypeDocumentPointerDown);
  document.addEventListener("keydown", handleChartTypeDocumentKeydown);
  window.addEventListener("online", handleChartOnline);
  window.addEventListener("resize", syncChartTypePanelPosition);
  window.addEventListener("scroll", syncChartTypePanelPosition, true);
  void loadBrokerProviders().then(() => {
    reconcileSelectedPeriod();
    void reload();
  });
  heartbeatTimer = window.setInterval(() => {
    void heartbeatChartSubscription(selectedBrokerId.value);
  }, 15_000);
});

onBeforeUnmount(() => {
  chartReloadSeq += 1;
  document.removeEventListener("visibilitychange", handleChartVisibilityChange);
  document.removeEventListener("pointerdown", handleChartTypeDocumentPointerDown);
  document.removeEventListener("keydown", handleChartTypeDocumentKeydown);
  window.removeEventListener("online", handleChartOnline);
  window.removeEventListener("resize", syncChartTypePanelPosition);
  window.removeEventListener("scroll", syncChartTypePanelPosition, true);
  window.clearInterval(heartbeatTimer);
  if (heldChartSubscription != null) {
    void releaseMarketDataSubscription({
      consumerId: chartConsumerId,
      ...(heldChartSubscription.brokerId
        ? { brokerId: heldChartSubscription.brokerId }
        : {}),
      market: heldChartSubscription.market,
      symbol: heldChartSubscription.symbol,
      channel: heldChartSubscription.channel,
      ...(heldChartSubscription.channel === "KLINE"
        ? { interval: heldChartSubscription.interval }
        : {}),
      keepalive: true,
    });
    heldChartSubscription = null;
  }
  if (controlled.value) {
    const workspacePeriod = normalizedRenderablePeriod(prefs.value.period);
    if (
      prefs.value.market.trim() !== "" &&
      prefs.value.symbol.trim() !== "" &&
      workspacePeriod !== ""
    ) {
      selectMarketDataInstrument({
        market: prefs.value.market,
        symbol: prefs.value.symbol,
        period: workspacePeriod,
      });
    }
  }
});

watch(
  () => chartTargetKey(chartTarget.value),
  () => {
    void reload();
  },
);
watch(
  () => [
    selectedBrokerId.value,
    targetMarket.value,
    supportedPeriodValues.value?.join(",") ?? "",
  ],
  () => {
    reconcileSelectedPeriod();
  },
);
watch(
  () => normalizedSelectedPeriod(),
  (period) => {
    if (period !== "tick") return;
    closeChartTypeMenu();
    if (controlled.value) {
      controlledChartType.value = "standard";
      return;
    }
    if (normalizeChartType(prefs.value.chartType) !== "standard") {
      update({ chartType: "standard" });
    }
  },
  { immediate: true },
);
</script>

<template>
  <section
    class="tv-panel lightweight-chart"
    :class="`lightweight-chart--${variant}`"
    :style="{ '--lightweight-chart-min-height': `${minHeight}px` }"
  >
    <div class="tv-panel-head lightweight-chart-head">
      <div class="lightweight-chart-head__primary-controls">
        <div class="tv-seg lightweight-chart-head__periods">
          <button
            v-for="p in periods"
            :key="p.value"
            type="button"
            :class="{ 'is-active': normalizedSelectedPeriod() === p.value }"
            :disabled="isLoadingBrokerCapabilities"
            @click="setPeriod(p.value)"
          >
            {{ p.label }}
          </button>
        </div>
        <label class="lightweight-chart-head__period-select">
          <select
            aria-label="K 线周期"
            :value="normalizedSelectedPeriod()"
            :disabled="isLoadingBrokerCapabilities || periods.length === 0"
            @change="handleCompactPeriodChange"
          >
            <option v-if="periods.length === 0" value="">--</option>
            <option v-for="p in periods" :key="p.value" :value="p.value">
              {{ p.label }}
            </option>
          </select>
          <span
            class="fa-solid fa-chevron-down lightweight-chart-head__period-chevron"
            aria-hidden="true"
          />
        </label>
        <div class="kline-chart-type-selector">
          <button
            ref="chartTypeTriggerRef"
            class="kline-chart-type-selector__trigger"
            type="button"
            :class="{ 'is-open': isChartTypeMenuOpen }"
            :title="`图表类型：${activeChartTypeLabel}`"
            aria-label="选择图表类型"
            aria-haspopup="menu"
            :aria-expanded="isChartTypeMenuOpen"
            @click="toggleChartTypeMenu"
          >
            <span class="fa-solid fa-chart-column" aria-hidden="true" />
          </button>
          <Teleport to="body">
            <div
              v-if="isChartTypeMenuOpen"
              ref="chartTypePanelRef"
              class="kline-chart-type-selector__panel"
              role="menu"
              aria-label="图表类型"
              :style="{
                top: `${chartTypePanelTop}px`,
                left: `${chartTypePanelLeft}px`,
              }"
            >
              <button
                v-for="(option, index) in KLINE_CHART_TYPES"
                :key="option.value"
                :ref="(element) => setChartTypeOptionRef(index, element)"
                class="kline-chart-type-selector__option"
                type="button"
                role="menuitemradio"
                :data-chart-type="option.value"
                :aria-checked="activeChartType === option.value"
                :class="{ 'is-active': activeChartType === option.value }"
                :disabled="
                  option.value === 'heikinashi' && isTickChartPeriod
                "
                :title="
                  option.value === 'heikinashi' && isTickChartPeriod
                    ? 'Tick 周期不支持平均K线图'
                    : option.label
                "
                @click="selectChartType(option.value)"
              >
                <span>{{ option.label }}</span>
                <span
                  v-if="activeChartType === option.value"
                  class="fa-solid fa-check"
                  aria-hidden="true"
                />
              </button>
            </div>
          </Teleport>
        </div>
        <KlineIndicatorSelector
          v-model="selectedIndicators"
          storage-key="jftrade.workspace-chart.indicators"
          :default-indicators="['volume']"
        />
      </div>
      <span
        v-if="isLoadingBrokerCapabilities"
        class="lightweight-chart-head__capability-state"
      >
        正在读取周期能力
      </span>
      <button
        v-else-if="brokerCapabilitiesError"
        class="lightweight-chart-head__capability-retry"
        type="button"
        title="周期能力加载失败，点击重试"
        @click="retryBrokerCapabilities"
      >
        周期能力加载失败，点击重试
      </button>
      <div class="lightweight-chart-head__spacer"></div>
      <MarketFeedStatus
        class="lightweight-chart-head__feed-status"
        :connection-state="chartConnectionState"
        :observed-at="chartObservedAt"
        :transport-mode="chartTransportMode"
        :source="chartSource"
        :from-cache="chartFromCache"
        :loading="isLoadingMarketDataQuery"
        :error="marketDataQueryError"
      />
      <button
        class="tv-icon-btn lightweight-chart-head__refresh"
        type="button"
        title="刷新"
        @click="() => reload()"
      >
        ↻
      </button>
    </div>
    <div class="tv-panel-body is-flush">
      <div class="tv-chart-host">
        <KlineChart
          :candles="chartCandles"
          :chart-type="activeChartType"
          :min-height="minHeight"
          :indicators="selectedIndicators"
          empty-text="暂无 K 线数据；确认 OpenD 行情权限后点击刷新。"
          @load-more="handleLoadMore"
        />
        <button
          v-if="historyLoadStatus"
          class="tv-chart-history-status"
          :class="{ 'is-error': marketDataOlderError }"
          type="button"
          :disabled="!marketDataOlderError"
          @click="handleLoadMore"
        >
          {{ historyLoadStatus }}
        </button>
        <div
          v-if="
            hasResolvedPeriodCapabilities &&
            !hasSupportedChartPeriod &&
            !brokerCapabilitiesError
          "
          class="tv-chart-unavailable"
        >
          该提供者不支持当前市场的图表数据
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.lightweight-chart {
  container: lightweight-chart / inline-size;
}

.lightweight-chart-head {
  min-width: 0;
  overflow: hidden;
}

.lightweight-chart--workspace .lightweight-chart-head {
  padding-left: var(--workspace-chart-head-left-reserve, 10px);
}

.lightweight-chart-head__primary-controls {
  display: flex;
  min-width: 0;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
}

.lightweight-chart-head__periods {
  flex: 0 0 auto;
}

.lightweight-chart-head__period-select {
  position: relative;
  display: none;
  height: 26px;
  flex: 0 0 auto;
  align-items: center;
}

.lightweight-chart-head__period-select select {
  height: 26px;
  min-width: 58px;
  padding: 0 24px 0 8px;
  appearance: none;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  outline: none;
  background: transparent;
  color: var(--tv-text);
  cursor: pointer;
  font-size: 11px;
  font-weight: 600;
}

.lightweight-chart-head__period-select select:hover,
.lightweight-chart-head__period-select select:focus-visible {
  border-color: var(--tv-border-strong);
  background: var(--tv-bg-elevated);
}

.lightweight-chart-head__period-select select:disabled {
  cursor: default;
  opacity: 0.55;
}

.lightweight-chart-head__period-chevron {
  position: absolute;
  right: 8px;
  color: var(--tv-text-muted);
  font-size: 9px;
  pointer-events: none;
}

.kline-chart-type-selector {
  display: inline-flex;
  flex: 0 0 auto;
}

.kline-chart-type-selector__trigger {
  display: inline-grid;
  width: 26px;
  height: 26px;
  place-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
  font-size: 12px;
  line-height: 1;
}

.kline-chart-type-selector__trigger:hover,
.kline-chart-type-selector__trigger.is-open,
.kline-chart-type-selector__trigger:focus-visible {
  border-color: var(--tv-border-strong);
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
}

.kline-chart-type-selector__panel {
  position: fixed;
  z-index: 9999;
  display: grid;
  width: min(188px, calc(100vw - 16px));
  padding: 4px;
  border: 1px solid var(--tv-border-strong);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  box-shadow: 0 14px 36px rgb(0 0 0 / 36%);
}

.kline-chart-type-selector__option {
  display: flex;
  min-height: 30px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 4px 8px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
  font-size: 12px;
  text-align: left;
}

.kline-chart-type-selector__option:hover,
.kline-chart-type-selector__option:focus-visible,
.kline-chart-type-selector__option.is-active {
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
}

.kline-chart-type-selector__option:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.lightweight-chart-head__spacer {
  min-width: 0;
  flex: 1;
}

.lightweight-chart-head__capability-state,
.lightweight-chart-head__capability-retry {
  min-width: 0;
  max-width: 180px;
  flex: 0 1 auto;
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.lightweight-chart-head__capability-retry {
  border: 0;
  background: transparent;
  cursor: pointer;
}

.lightweight-chart-head__feed-status {
  min-width: 0;
  flex: 0 1 auto;
}

.lightweight-chart-head__refresh {
  flex: 0 0 32px;
}

.tv-panel-body.is-flush {
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

.tv-chart-host {
  position: relative;
  flex: 1;
  height: 100%;
  min-height: 0;
  overflow: hidden;
}

.tv-chart-history-status {
  position: absolute;
  z-index: 3;
  top: 10px;
  left: 10px;
  border: 0;
  border-radius: 4px;
  padding: 4px 8px;
  background: color-mix(in srgb, var(--tv-bg-surface) 86%, transparent);
  color: var(--tv-text-muted);
  font-size: 12px;
}

.tv-chart-history-status.is-error {
  color: var(--tv-status-error-fg);
  cursor: pointer;
}

.tv-chart-unavailable {
  position: absolute;
  z-index: 4;
  inset: 0;
  display: grid;
  place-items: center;
  background: var(--tv-bg-surface);
  color: var(--tv-text-muted);
  font-size: 13px;
}

.tv-chart-host :deep(.kline-chart-shell) {
  height: 100%;
  min-height: 0;
}

.lightweight-chart--embedded {
  height: auto;
  flex: 0 0 auto;
}

.lightweight-chart--embedded .tv-panel-body.is-flush,
.lightweight-chart--embedded .tv-chart-host {
  height: var(--lightweight-chart-min-height, 320px);
  min-height: var(--lightweight-chart-min-height, 320px);
  flex: 0 0 var(--lightweight-chart-min-height, 320px);
}

.lightweight-chart--embedded .tv-chart-host :deep(.kline-chart-shell) {
  min-height: var(--lightweight-chart-min-height, 320px);
}

@container lightweight-chart (max-width: 720px) {
  .lightweight-chart-head {
    gap: 6px;
    padding-right: 6px;
    padding-left: var(--workspace-chart-head-left-reserve, 6px);
  }

  .lightweight-chart-head__periods {
    display: none;
  }

  .lightweight-chart-head__period-select {
    display: inline-flex;
  }
}
</style>
