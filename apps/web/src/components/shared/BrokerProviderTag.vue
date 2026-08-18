<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";

import type { MarketDataProviderStatusDto } from "@/contracts";
import type { FutuOpenDHealthResponse } from "@/types";
import {
  brokerProviderOptions,
  brokerProviderDisplayName,
  configureBrokerProviderDefaults,
  type BrokerCapabilityState,
  type BrokerProviderOption,
  useBrokerProviderSelection,
} from "@/composables/trading/brokerProviderSelection";
import {
  getFutuOpenDHealth,
  getMarketDataProviderSettings,
  getMarketDataProviderStatus,
  putMarketDataProviderSettings,
  type MarketDataProviderID,
} from "@/composables/settings/marketDataProviderSettings";
import {
  resolveMarketDataFeedPresentation,
  resolveMarketDataFeedQuality,
} from "@/composables/market-data/marketDataFeedQuality";
import type { LiveSocketConnectionState } from "@/composables/market-data/sharedLiveSocket";
import type { ProductFeatureProvider } from "@/composables/product/productFeatures";
import {
  isPythonMarketDataProvider,
  usePythonMarketDataRuntimeWarmup,
} from "@/composables/market-data/usePythonMarketDataRuntimeWarmup";
import {
  defaultEmbeddedPythonMarketDataProviderID,
  embeddedPythonMarketDataFeatureIDs,
  embeddedPythonMarketDataProviderOption,
  pythonMarketDataProviderName,
  statusMatchesPythonMarketDataProvider,
} from "@/composables/market-data/embeddedPythonMarketDataProviders";
import BrokerProviderMenu from "./BrokerProviderMenu.vue";

const props = withDefaults(
  defineProps<{
    provider?: ProductFeatureProvider | null | undefined;
    featureId?: string | undefined;
    featureIds?: string[] | undefined;
    market?: string | undefined;
    preferredBrokerId?: string | undefined;
    connectionState?: LiveSocketConnectionState | undefined;
    transportMode?: string | null | undefined;
    menuLocation?: "bottom end" | "top end";
    enableEmbeddedMarketDataProvider?: boolean;
  }>(),
  {
    provider: null,
    featureId: "",
    featureIds: () => [],
    market: "",
    preferredBrokerId: "",
    connectionState: undefined,
    transportMode: null,
    menuLocation: "bottom end",
    enableEmbeddedMarketDataProvider: false,
  },
);
const emit = defineEmits<{
  providerChanged: [];
}>();

const embeddedProviderRefreshIntervalMs = 10_000;

const menuOpen = ref(false);
const embeddedProviderID = ref<MarketDataProviderID | null>(null);
const embeddedProviderError = ref("");
const embeddedProviderStatus = ref<MarketDataProviderStatusDto | null>(null);
const embeddedProviderStatusError = ref("");
const futuOpenDHealth = ref<FutuOpenDHealthResponse | null>(null);
const futuOpenDHealthError = ref("");
const futuOpenDRefreshRequired = ref(false);
const embeddedProviderUnavailable = ref(false);
const switchingEmbeddedProvider = ref(false);
let embeddedProviderLoad: Promise<void> | null = null;
let embeddedProviderRefresh: Promise<void> | null = null;
let embeddedProviderRefreshQueued = false;
let embeddedProviderRefreshQueuedFresh = false;
let embeddedProviderLoadRevision = -1;
let embeddedProviderRevision = 0;
let embeddedProviderRefreshTimer: ReturnType<typeof setInterval> | null = null;
let embeddedProviderRefreshTimerGeneration = 0;
let componentUnmounted = false;
const {
  loadBrokerProviders,
  loadError,
  loading,
  selectBrokerProvider,
  selectedBrokerId,
} = useBrokerProviderSelection();

const activeFeatureIds = computed(() => {
  const explicit = props.featureIds.map((feature) => feature.trim()).filter(Boolean);
  if (explicit.length > 0) return [...new Set(explicit)];
  const fallback =
    props.featureId.trim() || props.provider?.featureId?.trim() || "";
  return fallback ? [fallback] : [];
});
const embeddedProviderVisible = computed(
  () =>
    props.enableEmbeddedMarketDataProvider &&
    // Research market views combine Python-provider snapshots with
    // Futu-only ranking and industry features. The embedded provider still
    // needs to be selectable for the quote surface; requiring every visible
    // feature hid those providers from that shared toolbar entirely.
    activeFeatureIds.value.some((feature) =>
      embeddedPythonMarketDataFeatureIDs.has(feature),
    ),
);
function isFutuOpenDHealthy(health: FutuOpenDHealthResponse | null): boolean {
  return (
    health?.status === "healthy" &&
    health.runtime.connectivity === "connected" &&
    health.runtime.quoteLoggedIn === true
  );
}

function futuOpenDUnavailableReason(health: FutuOpenDHealthResponse): string {
  if (health.runtime.connectivity === "disconnected") {
    return "当前无法连接 OpenD";
  }
  if (health.runtime.quoteLoggedIn === false) {
    return "OpenD 行情会话尚未登录";
  }
  if (health.runtime.quoteLoggedIn == null) {
    return "OpenD 行情会话状态不可用";
  }
  return "OpenD 连接异常";
}

const futuProviderReadiness = computed<{
  displayState: BrokerCapabilityState;
  reason: string;
} | null>(() => {
  if (!embeddedProviderVisible.value) return null;
  if (futuOpenDRefreshRequired.value) {
    const health = futuOpenDHealth.value;
    if (health != null && !isFutuOpenDHealthy(health)) {
      return {
        displayState: "unavailable",
        reason: futuOpenDUnavailableReason(health),
      };
    }
    return { displayState: "degraded", reason: "正在检查 OpenD 连接…" };
  }
  if (futuOpenDHealthError.value) {
    return { displayState: "unavailable", reason: "无法读取 OpenD 连接状态" };
  }
  const health = futuOpenDHealth.value;
  if (health == null) {
    return { displayState: "unavailable", reason: "尚未检查 OpenD 连接" };
  }
  if (!isFutuOpenDHealthy(health)) {
    return {
      displayState: "unavailable",
      reason: futuOpenDUnavailableReason(health),
    };
  }
  return null;
});

const yfinanceOption = computed<BrokerProviderOption>(() =>
  embeddedPythonMarketDataProviderOption("yfinance", props.market),
);
const akshareOption = computed<BrokerProviderOption>(() =>
  embeddedPythonMarketDataProviderOption("akshare", props.market),
);
const options = computed<BrokerProviderOption[]>(() => {
  const values = brokerProviderOptions(activeFeatureIds.value, props.market);
  if (
    embeddedProviderVisible.value &&
    !values.some((option) => option.id === "yfinance")
  ) {
    values.push(yfinanceOption.value);
  }
  if (
    embeddedProviderVisible.value &&
    !values.some((option) => option.id === "akshare")
  ) {
    values.push(akshareOption.value);
  }
  const actualID = props.provider?.brokerId?.trim().toLowerCase() ?? "";
  if (actualID && !values.some((option) => option.id === actualID)) {
    values.push({
      id: actualID,
      label: props.provider?.securityFirm?.trim() || actualID.toUpperCase(),
      shortLabel: actualID.toUpperCase().slice(0, 12),
      securityFirm: props.provider?.securityFirm?.trim() ?? "",
      state: props.provider?.capability ?? "unavailable",
      displayState: props.provider?.capability ?? "unavailable",
      tone:
        props.provider?.capability === "available"
          ? "success"
          : props.provider?.capability === "degraded"
            ? "warning"
            : "error",
      selectable: props.provider?.capability !== "unavailable",
      // selectionReason describes why this provider was selected, not why a
      // capability is restricted. Keep it out of the capability hint.
      reason: "",
    });
  }
  const futuReadiness = futuProviderReadiness.value;
  return values.map((option) =>
    option.id === "futu" && futuReadiness != null
      ? {
          ...option,
          displayState: futuReadiness.displayState,
          tone:
            futuReadiness.displayState === "degraded" ? "warning" : "error",
          reason: futuReadiness.reason,
          selectable: false,
        }
      : option,
  );
});
const selectedOption = computed<BrokerProviderOption | null>(() => {
  const selected = embeddedProviderVisible.value
    ? embeddedProviderID.value ?? defaultEmbeddedPythonMarketDataProviderID
    : selectedBrokerId.value;
  const actual = props.provider?.brokerId?.trim().toLowerCase() ?? "";
  if (embeddedProviderVisible.value) {
    return (
      options.value.find((option) => option.id === selected) ??
      options.value.find(
        (option) => option.id === defaultEmbeddedPythonMarketDataProviderID,
      ) ??
      null
    );
  }
  return (
    options.value.find((option) => option.id === selected) ??
    options.value.find((option) => option.id === actual) ??
    options.value[0] ??
    null
  );
});
const currentCapabilitySummary = computed(() => {
  const selected = selectedOption.value;
  return {
    state: selected?.state ?? ("unavailable" as BrokerCapabilityState),
    reason: selected?.reason?.trim() ?? "",
  };
});
const capabilityState = computed(() => currentCapabilitySummary.value.state);
const capabilityDisplayState = computed<BrokerCapabilityState>(
  () => selectedOption.value?.displayState ?? capabilityState.value,
);
const authoritativeProviderHealth = computed(() => {
  const selectedID = selectedOption.value?.id;
  if (embeddedProviderVisible.value && selectedID === "futu") {
    const readiness = futuProviderReadiness.value;
    const connected = readiness == null && isFutuOpenDHealthy(futuOpenDHealth.value);
    return {
      connected,
      streamMode: props.transportMode ?? (connected ? "push-stream" : "idle"),
      lastError: connected ? "" : readiness?.reason || "尚未检查 OpenD 连接",
    };
  }
  const status = embeddedProviderStatus.value;
  const statusProviderID =
    status?.descriptor?.brokerId?.trim().toLowerCase() ||
    status?.descriptor?.providerId?.trim().toLowerCase() ||
    "";
  const statusMatchesSelectedProvider =
    isPythonMarketDataProvider(selectedID) &&
    statusMatchesPythonMarketDataProvider(selectedID, statusProviderID);
  if (embeddedProviderVisible.value && statusMatchesSelectedProvider) {
    return status?.health ?? null;
  }
  return null;
});
const { readiness: pythonRuntimeReadiness } = usePythonMarketDataRuntimeWarmup({
  providerID: embeddedProviderID,
  status: embeddedProviderStatus,
  refresh: () => refreshEmbeddedProviderState(),
});
const pythonProviderName = computed(() =>
  pythonMarketDataProviderName(
    embeddedProviderID.value === "akshare" ? "akshare" : "yfinance",
  ),
);
const runtimeFeedInput = computed(() => {
  const statusHealth = authoritativeProviderHealth.value;
  if (
    props.connectionState == null &&
    props.transportMode == null &&
    statusHealth == null
  ) {
    return null;
  }
  const statusConnectionState = statusHealth?.connected
    ? "connected"
    : statusHealth?.lastError
      ? "error"
      : "disconnected";
  const statusIsAuthoritative = statusHealth != null;
  const hasUsableData =
    (statusIsAuthoritative && statusHealth?.connected === true) ||
    (!statusIsAuthoritative && props.connectionState === "connected");
  return {
    connectionState: statusIsAuthoritative
      ? statusConnectionState
      : props.connectionState ?? statusConnectionState,
    transportMode: statusIsAuthoritative
      ? statusHealth?.streamMode ?? props.transportMode ?? null
      : props.transportMode ?? null,
    hasUsableData,
    error: statusHealth?.lastError ?? null,
  };
});
const runtimeFeedQuality = computed(() => {
  const input = runtimeFeedInput.value;
  return input == null ? null : resolveMarketDataFeedQuality(input);
});
const runtimeFeedPresentation = computed(() => {
  const input = runtimeFeedInput.value;
  return input == null ? null : resolveMarketDataFeedPresentation(input);
});
const runtimeFeedQualityLabel = computed(
  () => runtimeFeedPresentation.value?.qualityLabel ?? "",
);
const currentState = computed<BrokerCapabilityState>(() => {
  if (embeddedProviderUnavailable.value) return "unavailable";
  if (pythonRuntimeReadiness.value === "warming") return "degraded";
  if (pythonRuntimeReadiness.value === "failed") return "unavailable";
  if (capabilityDisplayState.value === "unavailable") return "unavailable";
  const runtimeState = runtimeFeedPresentation.value?.state;
  if (runtimeState === "error") return "unavailable";
  if (
    runtimeState === "stale" ||
    runtimeState === "loading" ||
    runtimeState === "empty"
  ) {
    return "degraded";
  }
  return capabilityDisplayState.value;
});
const currentLabel = computed(
  () =>
    switchingEmbeddedProvider.value
      ? "启动中"
      : pythonRuntimeReadiness.value === "warming"
        ? `${pythonProviderName.value} 预热中`
      : embeddedProviderUnavailable.value
        ? "不可用"
      : selectedOption.value?.shortLabel || (loading.value ? "加载中" : "数据源"),
);
const currentReason = computed(() =>
  pythonRuntimeReadiness.value === "warming"
    ? `${pythonProviderName.value} 行情依赖正在后台预热，完成后会自动恢复查询`
    : currentCapabilitySummary.value.reason,
);
const currentReasonDetail = computed(() =>
  currentState.value === "available" ? "" : currentReason.value,
);
const currentProviderName = computed(() =>
  selectedOption.value?.label?.trim() ||
  (selectedOption.value == null
    ? "行情提供者"
    : brokerProviderDisplayName(selectedOption.value.id)),
);
const capabilityDetail = computed(() => {
  if (capabilityDisplayState.value === "unavailable") return "当前功能不可用";
  if (
    capabilityState.value === "degraded" &&
    currentState.value !== "available"
  ) {
    return "当前功能受限";
  }
  return "";
});
const currentTitle = computed(() => {
  const values = [
    `供应商：${currentProviderName.value}`,
    runtimeFeedPresentation.value?.connectionLabel
      ? `连接方式：${runtimeFeedPresentation.value.connectionLabel}`
      : "",
    runtimeFeedQualityLabel.value
      ? `数据质量：${runtimeFeedQualityLabel.value}`
      : "",
    capabilityDetail.value ? `功能范围：${capabilityDetail.value}` : "",
    currentReasonDetail.value ? `说明：${currentReasonDetail.value}` : "",
    loadError.value ? `能力目录：${loadError.value}` : "",
    embeddedProviderError.value ? `切换错误：${embeddedProviderError.value}` : "",
    embeddedProviderStatusError.value
      ? `状态详情：${embeddedProviderStatusError.value}`
      : "",
  ];
  return values.filter(Boolean).join("\n");
});
const currentAriaLabel = computed(() =>
  [
    "切换行情提供者",
    `供应商${currentProviderName.value}`,
    runtimeFeedPresentation.value?.connectionLabel
      ? `连接方式${runtimeFeedPresentation.value.connectionLabel}`
      : "",
    capabilityDetail.value ? `功能范围${capabilityDetail.value}` : "",
    runtimeFeedQualityLabel.value
      ? `数据质量${runtimeFeedQualityLabel.value}`
      : "",
    currentReasonDetail.value ? `说明${currentReasonDetail.value}` : "",
  ]
    .filter(Boolean)
    .join("，"),
);

async function select(option: BrokerProviderOption): Promise<void> {
  if (!option.selectable || switchingEmbeddedProvider.value) return;
  if (
    embeddedProviderVisible.value &&
    (option.id === "futu" ||
      option.id === "yfinance" ||
      option.id === "akshare")
  ) {
    await selectEmbeddedProvider(option.id);
    return;
  }
  selectBrokerProvider(option.id);
  menuOpen.value = false;
  stopEmbeddedProviderRefreshTimer();
}

async function loadEmbeddedProvider(): Promise<void> {
  if (!embeddedProviderVisible.value || embeddedProviderLoad != null) {
    return embeddedProviderLoad ?? Promise.resolve();
  }
  const revision = embeddedProviderRevision;
  embeddedProviderLoadRevision = revision;
  embeddedProviderLoad = getMarketDataProviderSettings()
    .then((settings) => {
      if (revision !== embeddedProviderRevision) return;
      embeddedProviderID.value = settings.activeProvider;
      embeddedProviderUnavailable.value = false;
      selectBrokerProvider(settings.activeProvider);
      embeddedProviderError.value = "";
      void refreshEmbeddedProviderState();
    })
    .catch((error: unknown) => {
      if (revision !== embeddedProviderRevision) return;
      embeddedProviderUnavailable.value = true;
      embeddedProviderError.value =
        error instanceof Error ? error.message : String(error);
    })
    .finally(() => {
      embeddedProviderLoad = null;
    });
  return embeddedProviderLoad;
}

async function loadEmbeddedProviderStatus(
  revision = embeddedProviderRevision,
): Promise<void> {
  if (!embeddedProviderVisible.value) return;
  try {
    const status = await getMarketDataProviderStatus();
    if (revision !== embeddedProviderRevision) return;
    embeddedProviderStatus.value = status;
    embeddedProviderStatusError.value = "";
  } catch (error: unknown) {
    if (revision !== embeddedProviderRevision) return;
    embeddedProviderStatusError.value =
      error instanceof Error ? error.message : String(error);
  }
}

async function loadFutuOpenDProviderHealth(
  revision = embeddedProviderRevision,
): Promise<void> {
  const wasHealthy = isFutuOpenDHealthy(futuOpenDHealth.value);
  try {
    const health = await getFutuOpenDHealth();
    if (revision !== embeddedProviderRevision) return;
    futuOpenDHealth.value = health;
    futuOpenDHealthError.value = "";
    if (isFutuOpenDHealthy(health) && !wasHealthy) {
      await loadBrokerProviders(true, !embeddedProviderVisible.value);
    }
  } catch (error: unknown) {
    if (revision !== embeddedProviderRevision) return;
    futuOpenDHealth.value = null;
    futuOpenDHealthError.value =
      error instanceof Error ? error.message : String(error);
  }
}

async function refreshEmbeddedProviderState(
  requireFreshFutu = false,
): Promise<void> {
  if (componentUnmounted || !embeddedProviderVisible.value) return;
  if (requireFreshFutu) futuOpenDRefreshRequired.value = true;
  if (embeddedProviderRefresh != null) {
    embeddedProviderRefreshQueued = true;
    embeddedProviderRefreshQueuedFresh ||= requireFreshFutu;
    return embeddedProviderRefresh;
  }

  const revision = embeddedProviderRevision;
  const includeFutu =
    menuOpen.value || embeddedProviderID.value === "futu";
  const includeActiveStatus = isPythonMarketDataProvider(
    embeddedProviderID.value,
  );
  embeddedProviderRefresh = Promise.all([
    includeFutu
      ? loadFutuOpenDProviderHealth(revision)
      : Promise.resolve(),
    includeActiveStatus
      ? loadEmbeddedProviderStatus(revision)
      : Promise.resolve(),
  ])
    .then(() => undefined)
    .finally(() => {
      if (revision === embeddedProviderRevision && includeFutu) {
        futuOpenDRefreshRequired.value = false;
      }
      embeddedProviderRefresh = null;
      if (componentUnmounted || !embeddedProviderRefreshQueued) return;
      const queuedFresh = embeddedProviderRefreshQueuedFresh;
      embeddedProviderRefreshQueued = false;
      embeddedProviderRefreshQueuedFresh = false;
      void refreshEmbeddedProviderState(queuedFresh);
    });
  return embeddedProviderRefresh;
}

function documentIsVisible(): boolean {
  return typeof document === "undefined" || document.visibilityState !== "hidden";
}

function stopEmbeddedProviderRefreshTimer(): void {
  embeddedProviderRefreshTimerGeneration += 1;
  if (embeddedProviderRefreshTimer != null) {
    clearInterval(embeddedProviderRefreshTimer);
    embeddedProviderRefreshTimer = null;
  }
}

function startEmbeddedProviderRefreshTimer(): void {
  stopEmbeddedProviderRefreshTimer();
  if (!menuOpen.value || !documentIsVisible()) return;
  const generation = ++embeddedProviderRefreshTimerGeneration;
  embeddedProviderRefreshTimer = setInterval(() => {
    if (
      generation === embeddedProviderRefreshTimerGeneration &&
      menuOpen.value &&
      documentIsVisible()
    ) {
      void refreshEmbeddedProviderState();
    }
  }, embeddedProviderRefreshIntervalMs);
}

function handleEmbeddedProviderVisibilityChange(): void {
  if (!menuOpen.value) return;
  if (!documentIsVisible()) {
    stopEmbeddedProviderRefreshTimer();
    return;
  }
  void refreshEmbeddedProviderState(true);
  startEmbeddedProviderRefreshTimer();
}

async function selectEmbeddedProvider(providerID: MarketDataProviderID): Promise<void> {
  const previous = embeddedProviderID.value;
  const revision = ++embeddedProviderRevision;
  switchingEmbeddedProvider.value = true;
  embeddedProviderError.value = "";
  embeddedProviderUnavailable.value = false;
  try {
    const saved = await putMarketDataProviderSettings(providerID);
    if (revision !== embeddedProviderRevision) return;
    embeddedProviderID.value = saved.activeProvider;
    selectBrokerProvider(saved.activeProvider);
    embeddedProviderStatus.value = null;
    embeddedProviderStatusError.value = "";
    menuOpen.value = false;
    stopEmbeddedProviderRefreshTimer();
    void refreshEmbeddedProviderState();
    emit("providerChanged");
  } catch (error) {
    if (revision !== embeddedProviderRevision) return;
    embeddedProviderID.value = previous;
    embeddedProviderUnavailable.value = previous == null;
    if (previous != null) selectBrokerProvider(previous);
    embeddedProviderError.value =
      error instanceof Error ? error.message : String(error);
  } finally {
    switchingEmbeddedProvider.value = false;
  }
}

watch(
  () => [props.preferredBrokerId, embeddedProviderVisible.value] as const,
  ([accountBrokerId, embeddedVisible]) => {
    if (embeddedVisible) return;
    configureBrokerProviderDefaults({ accountBrokerId });
  },
  { immediate: true },
);

watch(menuOpen, (open) => {
  if (!open) {
    futuOpenDRefreshRequired.value = false;
    stopEmbeddedProviderRefreshTimer();
    return;
  }
  futuOpenDRefreshRequired.value = true;
  startEmbeddedProviderRefreshTimer();
  if (embeddedProviderID.value == null) {
    void loadEmbeddedProvider();
    return;
  }
  if (documentIsVisible()) void refreshEmbeddedProviderState(true);
});

watch(
  embeddedProviderVisible,
  (visible) => {
    if (visible) {
      const load = loadEmbeddedProvider();
      // A visibility change can invalidate an in-flight read. Re-run the
      // current revision after the stale promise settles so the toolbar does
      // not remain without an active provider.
      void load.then(() => {
        if (
          embeddedProviderVisible.value &&
          embeddedProviderID.value == null &&
          embeddedProviderLoadRevision !== embeddedProviderRevision
        ) {
          void loadEmbeddedProvider();
        }
      });
      return;
    }
    embeddedProviderRevision += 1;
    menuOpen.value = false;
    embeddedProviderID.value = null;
    embeddedProviderStatus.value = null;
    embeddedProviderStatusError.value = "";
    futuOpenDHealth.value = null;
    futuOpenDHealthError.value = "";
    futuOpenDRefreshRequired.value = false;
    embeddedProviderRefreshQueued = false;
    embeddedProviderRefreshQueuedFresh = false;
    embeddedProviderUnavailable.value = false;
    configureBrokerProviderDefaults({ accountBrokerId: props.preferredBrokerId });
  },
  { immediate: true },
);

onMounted(() => {
  void loadBrokerProviders(false, !embeddedProviderVisible.value);
  if (typeof document !== "undefined") {
    document.addEventListener(
      "visibilitychange",
      handleEmbeddedProviderVisibilityChange,
    );
  }
});

onBeforeUnmount(() => {
  componentUnmounted = true;
  embeddedProviderRevision += 1;
  embeddedProviderRefreshQueued = false;
  embeddedProviderRefreshQueuedFresh = false;
  stopEmbeddedProviderRefreshTimer();
  if (typeof document !== "undefined") {
    document.removeEventListener(
      "visibilitychange",
      handleEmbeddedProviderVisibilityChange,
    );
  }
});
</script>

<template>
  <v-menu
    v-model="menuOpen"
    :location="menuLocation"
    :offset="6"
    :close-on-content-click="false"
  >
    <template #activator="{ props: activatorProps }">
      <button
        v-bind="activatorProps"
        type="button"
        class="broker-provider-tag"
        :class="`is-${currentState}`"
        :data-quality="runtimeFeedQuality || undefined"
        :data-capability-state="capabilityState"
        :data-display-state="currentState"
        :data-capability-reason="currentReason || undefined"
        :title="currentTitle"
        :aria-label="currentAriaLabel"
      >
        <span class="broker-provider-tag__dot" />
        <span class="broker-provider-tag__label">{{ currentLabel }}</span>
        <span class="broker-provider-tag__chevron">⌄</span>
      </button>
    </template>
    <BrokerProviderMenu
      :options="options"
      :selected-option="selectedOption"
      :switching="switchingEmbeddedProvider"
      :error="embeddedProviderError"
      :loading="loading"
      :load-error="loadError"
      @select="select"
    />
  </v-menu>
</template>

<style scoped>
.broker-provider-tag {
  display: inline-flex;
  height: 24px;
  max-width: 86px;
  flex: 0 0 auto;
  align-items: center;
  gap: 5px;
  padding: 0 6px;
  border: 1px solid var(--tv-border);
  border-radius: 999px;
  background: color-mix(in srgb, var(--tv-text) 3%, var(--tv-bg-surface));
  color: var(--tv-text-muted);
  cursor: pointer;
  font-size: var(--jf-text-3);
  line-height: 1;
  user-select: none;
}

.broker-provider-tag:hover,
.broker-provider-tag[aria-expanded="true"] {
  border-color: color-mix(in srgb, var(--tv-accent) 55%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 8%, var(--tv-bg-surface));
  color: var(--tv-text);
}

.broker-provider-tag__dot {
  width: 8px;
  height: 8px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: var(--tv-text-dim);
}

.broker-provider-tag.is-available .broker-provider-tag__dot {
  background: var(--tv-status-success-fg);
}

.broker-provider-tag.is-degraded .broker-provider-tag__dot {
  background: var(--tv-status-warning-fg);
}

.broker-provider-tag.is-unavailable .broker-provider-tag__dot {
  background: var(--tv-status-error-fg);
}

.broker-provider-tag__label {
  min-width: 0;
  overflow: hidden;
  font-size: 1.2em;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.broker-provider-tag__chevron {
  color: var(--tv-text-dim);
  font-size: var(--jf-text-3);
  transform: translateY(-1px);
}

.broker-provider-tag[aria-expanded="true"] .broker-provider-tag__chevron {
  transform: rotate(180deg) translateY(1px);
}

</style>
