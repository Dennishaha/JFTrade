<script setup lang="ts">
import {
  computed,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";
import KlineChart from "@/components/domain/market-data/KlineChart.vue";
import LightweightChartHeader from "./LightweightChartHeader.vue";
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
  brokerProviderDisplayName,
  brokerSupportedChartPeriods,
  useBrokerProviderSelection,
} from "@/composables/trading/brokerProviderSelection";
import { brokerSupportedChartSessions } from "@/composables/trading/brokerCandleSessions";
import {
  candleMatchesSessions,
  CANDLE_SESSION_ORDER,
  intersectCandleSessions,
  normalizeCandleSessions,
  type CandleSession,
} from "@/composables/market-data/candleSessions";
import { getSharedLiveSocketHub } from "@/composables/market-data/sharedLiveSocket";
import {
  fallbackInstrumentPeriod,
  resolveInstrumentRequestPeriod,
  resolveInstrumentSupportedPeriods,
} from "@/composables/market-data/instrumentPeriodCapabilities";
import { useConsoleData } from "@/composables/workspace/useConsoleData";
import { useWorkspaceTradingPrefs } from "@/composables/workspace/useWorkspaceLayout";
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
  currentMarketSecurityDetails,
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
const selectedCandleSessions = ref<CandleSession[]>([...CANDLE_SESSION_ORDER]);
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
const targetInstrumentId = computed(() =>
  targetMarket.value === "" || targetSymbol.value === ""
    ? ""
    : `${targetMarket.value}.${targetSymbol.value}`,
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
const historyLoadAttempted = ref(false);
const providerSupportedPeriodValues = computed(() =>
  brokerSupportedChartPeriods(
    selectedBrokerId.value,
    targetMarket.value,
    brokerDescriptors.value,
  ),
);
const supportedCandleSessions = computed<CandleSession[]>(() => {
  const supported = brokerSupportedChartSessions(
    selectedBrokerId.value,
    targetMarket.value,
    normalizedSelectedPeriod(),
    brokerDescriptors.value,
  );
  return supported == null ? [] : normalizeCandleSessions(supported);
});
const effectiveCandleSessions = computed<CandleSession[]>(() => {
  const available = supportedCandleSessions.value;
  if (available.length === 0) return [...selectedCandleSessions.value];
  const retained = intersectCandleSessions(selectedCandleSessions.value, available);
  return retained.length > 0 ? retained : [...available];
});
const requiresInstrumentPeriodCapabilities = computed(
  () => selectedBrokerId.value.trim().toLowerCase() === "akshare",
);
const supportedPeriodValues = computed<string[] | null>(() =>
  resolveInstrumentSupportedPeriods({
    providerID: selectedBrokerId.value,
    instrumentID: targetInstrumentId.value,
    providerPeriods: providerSupportedPeriodValues.value,
    details: currentMarketSecurityDetails.value,
    requireDetails: requiresInstrumentPeriodCapabilities.value,
  }),
);
const periodCapabilitiesError = computed(() => {
  if (brokerCapabilitiesError.value) return brokerCapabilitiesError.value;
  if (
    requiresInstrumentPeriodCapabilities.value &&
    supportedPeriodValues.value == null &&
    !isLoadingMarketDataQuery.value
  ) {
    return marketDataQueryError.value;
  }
  return "";
});
const isLoadingPeriodCapabilities = computed(
  () =>
    isLoadingBrokerCapabilities.value ||
    (requiresInstrumentPeriodCapabilities.value &&
      supportedPeriodValues.value == null &&
      periodCapabilitiesError.value === ""),
);
const displayedPeriodValues = computed(
  () =>
    supportedPeriodValues.value ??
    (periodCapabilitiesError.value
      ? []
      : providerSupportedPeriodValues.value ?? []),
);
const periods = computed(() => {
  const supported = new Set(displayedPeriodValues.value);
  return KLINE_PERIODS.filter((period) => supported.has(period.value));
});
const hasResolvedPeriodCapabilities = computed(
  () =>
    !isLoadingPeriodCapabilities.value && supportedPeriodValues.value != null,
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
    commitPeriod(fallbackInstrumentPeriod(supported) as RenderableKlinePeriod);
  }
}

const chartTarget = computed(() => {
  const preferredPeriod = normalizedSelectedPeriod();
  const period = resolveInstrumentRequestPeriod({
    preferredPeriod,
    providerPeriods: providerSupportedPeriodValues.value,
    supportedPeriods: supportedPeriodValues.value,
    requireDetails: requiresInstrumentPeriodCapabilities.value,
    renderablePeriods,
  }) as RenderableKlinePeriod | "";
  return {
    brokerId: selectedBrokerId.value,
    market: targetMarket.value,
    symbol: targetSymbol.value,
    period,
    instrumentId: targetInstrumentId.value,
    channel: (period === "tick" ? "TICK" : "KLINE") as "TICK" | "KLINE",
    interval: period,
    sessions: [...effectiveCandleSessions.value],
  };
});
const chartCandles = computed<KlineCandle[]>(() =>
  overlayRealtimeTickCandle(
    (marketDataCandles.value?.candles ?? []).filter((candle) =>
      candleMatchesSessions(candle.session, effectiveCandleSessions.value),
    ),
    selectedBrokerId.value.trim().toLowerCase() === "akshare" ||
      !candleMatchesSessions(
        marketDataSnapshot.value?.snapshot?.session,
        effectiveCandleSessions.value,
      )
      ? null
      : marketDataSnapshot.value?.snapshot ?? null,
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
const chartProviderName = computed(() => {
  const providerID =
    marketDataSnapshot.value?.meta.brokerId ??
    marketDataCandles.value?.meta.brokerId ??
    selectedBrokerId.value;
  return providerID?.trim() ? brokerProviderDisplayName(providerID) : null;
});
const chartFromCache = computed(() =>
  marketDataSnapshot.value?.meta.fromCache ?? marketDataCandles.value?.meta.fromCache ?? false,
);
const historyLoadStatus = computed(() => {
  if (chartTarget.value.period === "tick" || chartTarget.value.period === "") {
    return "";
  }
  if (isLoadingOlderMarketData.value) return "正在加载更早数据";
  if (marketDataOlderError.value) return "加载失败，拖动或点击重试";
  if (!historyLoadAttempted.value) return "";
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

function sessionsForQuery(): CandleSession[] | undefined {
  const available = supportedCandleSessions.value;
  if (available.length === 0) return undefined;
  if (
    available.length > 0 &&
    available.length === effectiveCandleSessions.value.length &&
    available.every((session) => effectiveCandleSessions.value.includes(session))
  ) {
    return undefined;
  }
  return effectiveCandleSessions.value;
}
async function reload(options: { preserveExisting?: boolean } = {}): Promise<void> {
  const target = resolveChartSubscriptionTarget();
  const reloadKey = chartTargetKey(target);
  if (reloadInFlight != null && reloadInFlight.key === reloadKey) {
    return reloadInFlight.promise;
  }
  historyLoadAttempted.value = false;
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
    const subscriptionReady = await syncChartSubscription(target, requestSeq);
    if (!subscriptionReady || requestSeq !== chartReloadSeq) {
      return;
    }
    const sessions = sessionsForQuery();
    await loadMarketDataQuery({
      ...(options.preserveExisting == null
        ? {}
        : { preserveExisting: options.preserveExisting }),
      ...(sessions == null ? {} : { sessions }),
    });
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
): Promise<boolean> {
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
    return false;
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
    return false;
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
    return false;
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
    return false;
  }

  heldChartSubscription = next;
  return true;
}

function heartbeatChartSubscription(brokerId: string): Promise<void> {
  return brokerId
    ? heartbeatMarketDataConsumer(chartConsumerId, brokerId)
    : heartbeatMarketDataConsumer(chartConsumerId);
}

function setPeriod(period: RenderableKlinePeriod): void {
  commitPeriod(period);
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
}

function handlePeriodSelection(value: string): void {
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
  historyLoadAttempted.value = true;
  const sessions = sessionsForQuery();
  await loadMarketDataQuery({
    appendOlder: true,
    before: marketDataNextBefore.value,
    ...(sessions == null ? {} : { sessions }),
  });
}
function updateCandleSessions(sessions: CandleSession[]): void {
  const next = intersectCandleSessions(sessions, supportedCandleSessions.value);
  if (next.length === 0) return;
  selectedCandleSessions.value = next;
  void reload();
}

async function retryBrokerCapabilities(): Promise<void> {
  if (brokerCapabilitiesError.value) await loadBrokerProviders(true);
  reconcileSelectedPeriod();
  await reload();
}

onMounted(() => {
  document.addEventListener("visibilitychange", handleChartVisibilityChange);
  window.addEventListener("online", handleChartOnline);
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
  window.removeEventListener("online", handleChartOnline);
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
  () => [
    selectedBrokerId.value,
    targetMarket.value,
    targetSymbol.value,
    normalizedSelectedPeriod(),
    supportedCandleSessions.value.join(","),
  ],
  (next, previous) => {
    const available = supportedCandleSessions.value;
    if (available.length === 0) return;
    const providerChanged = previous != null && next[0] !== previous[0];
    const retained = providerChanged
      ? []
      : intersectCandleSessions(selectedCandleSessions.value, available);
    selectedCandleSessions.value = retained.length > 0 ? retained : [...available];
  },
  { immediate: true },
);
watch(
  () => normalizedSelectedPeriod(),
  (period) => {
    if (period !== "tick") return;
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
    <LightweightChartHeader
      v-model:indicators="selectedIndicators"
      :variant="variant"
      :market="targetMarket"
      :periods="periods"
      :selected-period="normalizedSelectedPeriod()"
      :loading-capabilities="isLoadingPeriodCapabilities"
      :capabilities-error="periodCapabilitiesError"
      :active-chart-type="activeChartType"
      :active-chart-type-label="activeChartTypeLabel"
      :tick-period="isTickChartPeriod"
      :connection-state="chartConnectionState"
      :observed-at="chartObservedAt"
      :transport-mode="chartTransportMode"
      :source="chartSource"
      :provider-name="chartProviderName"
      :from-cache="chartFromCache"
      :loading-data="isLoadingMarketDataQuery"
      :data-error="marketDataQueryError"
      :candle-sessions="effectiveCandleSessions"
      :supported-candle-sessions="supportedCandleSessions"
      @select-period="handlePeriodSelection"
      @select-chart-type="selectChartType"
      @update:candle-sessions="updateCandleSessions"
      @retry="retryBrokerCapabilities"
      @refresh="reload()"
    />
    <div class="tv-panel-body is-flush">
      <div class="tv-chart-host">
        <KlineChart
          :candles="chartCandles"
          :chart-type="activeChartType"
          :min-height="minHeight"
          :indicators="selectedIndicators"
          empty-text="暂无 K 线数据；确认行情数据源可用后点击刷新。"
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
  font-size: var(--jf-text-6);
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
  font-size: var(--jf-text-7);
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

</style>
