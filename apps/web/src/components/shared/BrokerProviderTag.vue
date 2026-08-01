<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";

import type { MarketDataProviderStatusDto } from "@/contracts";
import {
  brokerProviderOptions,
  brokerProviderDisplayName,
  configureBrokerProviderDefaults,
  type BrokerCapabilityState,
  type BrokerProviderOption,
  useBrokerProviderSelection,
} from "@/composables/trading/brokerProviderSelection";
import {
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

const props = withDefaults(
  defineProps<{
    provider?: ProductFeatureProvider | null | undefined;
    featureId?: string | undefined;
    featureIds?: string[] | undefined;
    market?: string | undefined;
    preferredBrokerId?: string | undefined;
    defaultBrokerId?: string | undefined;
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
    defaultBrokerId: "",
    connectionState: undefined,
    transportMode: null,
    menuLocation: "bottom end",
    enableEmbeddedMarketDataProvider: false,
  },
);
const emit = defineEmits<{
  providerChanged: [];
}>();

const menuOpen = ref(false);
const embeddedProviderID = ref<MarketDataProviderID | null>(null);
const embeddedProviderError = ref("");
const embeddedProviderStatus = ref<MarketDataProviderStatusDto | null>(null);
const embeddedProviderStatusError = ref("");
const embeddedProviderUnavailable = ref(false);
const switchingEmbeddedProvider = ref(false);
let embeddedProviderLoad: Promise<void> | null = null;
let embeddedProviderLoadRevision = -1;
let embeddedProviderRevision = 0;
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
const yfinanceFeatureIDs = new Set([
  "market.search",
  "market.instrument_profile",
  "market.snapshot",
  "market.snapshots",
  "market.candles",
]);
const embeddedProviderVisible = computed(
  () =>
    props.enableEmbeddedMarketDataProvider &&
    // Research market views combine Yahoo-supported snapshots with
    // Futu-only ranking and industry features. The embedded provider still
    // needs to be selectable for the quote surface; requiring every visible
    // feature hid Yahoo from that shared toolbar entirely.
    activeFeatureIds.value.some((feature) => yfinanceFeatureIDs.has(feature)),
);
const yfinanceOption = computed<BrokerProviderOption>(() => {
  const market = props.market.trim().toUpperCase();
  const available =
    market === "" || ["US", "HK", "CN", "SH", "SZ"].includes(market);
  return {
    id: "yfinance",
    label: "Yahoo",
    shortLabel: "Yahoo",
    securityFirm: "内置行情查询",
    state: available ? "degraded" : "unavailable",
    displayState: available ? "available" : "unavailable",
    tone: available ? "success" : "error",
    reason: available
      ? "非实时快照查询，不支持实时推流或 Level 2"
      : "当前标的市场不在内置 Yahoo 支持范围",
  };
});
const options = computed(() => {
  const values = brokerProviderOptions(activeFeatureIds.value, props.market);
  if (
    embeddedProviderVisible.value &&
    !values.some((option) => option.id === "yfinance")
  ) {
    values.push(yfinanceOption.value);
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
      // selectionReason describes why this provider was selected, not why a
      // capability is restricted. Keep it out of the capability hint.
      reason: "",
    });
  }
  return values;
});
const selectedOption = computed<BrokerProviderOption | null>(() => {
  const selected =
    embeddedProviderVisible.value && embeddedProviderID.value != null
      ? embeddedProviderID.value
      : selectedBrokerId.value;
  const actual = props.provider?.brokerId?.trim().toLowerCase() ?? "";
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
const runtimeFeedInput = computed(() => {
  const statusHealth = embeddedProviderStatus.value?.health;
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
  const statusIsAuthoritative =
    statusHealth != null &&
    (selectedOption.value?.id === "yfinance" ||
      (props.connectionState == null && props.transportMode == null));
  const hasUsableData =
    (statusIsAuthoritative && statusHealth?.connected === true) ||
    (!statusIsAuthoritative && props.connectionState === "connected");
  return {
    connectionState: statusIsAuthoritative
      ? statusConnectionState
      : props.connectionState ?? statusConnectionState,
    transportMode: statusIsAuthoritative
      ? statusHealth?.streamMode ?? props.transportMode ?? null
      : props.transportMode ?? statusHealth?.streamMode ?? null,
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
      : embeddedProviderUnavailable.value
        ? "不可用"
      : selectedOption.value?.shortLabel || (loading.value ? "加载中" : "数据源"),
);
const currentReason = computed(() => currentCapabilitySummary.value.reason);
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
  if (option.state === "unavailable" || switchingEmbeddedProvider.value) return;
  if (
    embeddedProviderVisible.value &&
    (option.id === "futu" || option.id === "yfinance")
  ) {
    await selectEmbeddedProvider(option.id);
    return;
  }
  selectBrokerProvider(option.id);
  menuOpen.value = false;
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
      void loadEmbeddedProviderStatus(revision);
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
    void loadEmbeddedProviderStatus(revision);
    emit("providerChanged");
    menuOpen.value = false;
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
  () => [
    props.preferredBrokerId,
    props.defaultBrokerId,
  ] as const,
  ([accountBrokerId, defaultBrokerId]) => {
    configureBrokerProviderDefaults({ accountBrokerId, defaultBrokerId });
  },
  { immediate: true },
);

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
    embeddedProviderID.value = null;
    embeddedProviderStatus.value = null;
    embeddedProviderStatusError.value = "";
    embeddedProviderUnavailable.value = false;
    configureBrokerProviderDefaults({
      accountBrokerId: props.preferredBrokerId,
      defaultBrokerId: props.defaultBrokerId,
    });
  },
  { immediate: true },
);

onMounted(() => {
  void loadBrokerProviders();
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
    <div
      class="broker-provider-tag__menu"
      role="listbox"
      aria-label="行情提供者"
    >
      <div class="broker-provider-tag__heading">
        <strong>行情提供者</strong>
        <small>选择会应用到研究与产品行情</small>
      </div>
      <div
        v-if="switchingEmbeddedProvider"
        class="broker-provider-tag__empty"
        aria-live="polite"
      >
        正在启动内置行情提供者，首次启动可能需要几十秒…
      </div>
      <button
        v-for="option in options"
        :key="option.id"
        type="button"
        role="option"
        :aria-selected="option.id === selectedOption?.id"
        :disabled="option.state === 'unavailable' || switchingEmbeddedProvider"
        :class="[
          `is-${option.displayState ?? option.state}`,
          { 'is-selected': option.id === selectedOption?.id },
        ]"
        @click="select(option)"
      >
        <span class="broker-provider-tag__option-dot" />
        <span>
          <strong>{{ option.label }}</strong>
          <small v-if="option.securityFirm">{{ option.securityFirm }}</small>
          <small v-if="option.reason">{{ option.reason }}</small>
        </span>
        <span v-if="option.id === selectedOption?.id" aria-hidden="true"
          >✓</span
        >
      </button>
      <div v-if="embeddedProviderError" class="broker-provider-tag__empty">
        {{ embeddedProviderError }}
      </div>
      <div v-if="options.length === 0" class="broker-provider-tag__empty">
        {{ loading ? "正在读取券商能力…" : loadError || "暂无可用提供者" }}
      </div>
    </div>
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
  font-size: 9px;
  line-height: 1;
  user-select: none;
}

.broker-provider-tag:hover,
.broker-provider-tag[aria-expanded="true"] {
  border-color: color-mix(in srgb, var(--tv-accent) 55%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 8%, var(--tv-bg-surface));
  color: var(--tv-text);
}

.broker-provider-tag__dot,
.broker-provider-tag__option-dot {
  width: 8px;
  height: 8px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: var(--tv-text-dim);
}

.broker-provider-tag.is-available .broker-provider-tag__dot,
.broker-provider-tag__menu
  button.is-available
  .broker-provider-tag__option-dot {
  background: var(--tv-status-success-fg);
}

.broker-provider-tag.is-degraded .broker-provider-tag__dot,
.broker-provider-tag__menu button.is-degraded .broker-provider-tag__option-dot {
  background: var(--tv-status-warning-fg);
}

.broker-provider-tag.is-unavailable .broker-provider-tag__dot,
.broker-provider-tag__menu
  button.is-unavailable
  .broker-provider-tag__option-dot {
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
  font-size: 9px;
  transform: translateY(-1px);
}

.broker-provider-tag[aria-expanded="true"] .broker-provider-tag__chevron {
  transform: rotate(180deg) translateY(1px);
}

.broker-provider-tag__menu {
  width: min(280px, calc(100vw - 24px));
  padding: 6px;
  border: 1px solid var(--tv-border-strong);
  border-radius: 7px;
  background: var(--tv-bg-surface);
  box-shadow: 0 12px 30px rgb(0 0 0 / 28%);
}

.broker-provider-tag__heading {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  padding: 5px 7px 7px;
}

.broker-provider-tag__heading strong {
  color: var(--tv-text);
  font-size: 10px;
}

.broker-provider-tag__heading small {
  color: var(--tv-text-dim);
  font-size: 8px;
}

.broker-provider-tag__menu button {
  display: grid;
  width: 100%;
  grid-template-columns: 7px minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
  padding: 7px;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
  text-align: left;
}

.broker-provider-tag__menu button:hover,
.broker-provider-tag__menu button.is-selected {
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
}

.broker-provider-tag__menu button:disabled {
  cursor: not-allowed;
  opacity: 0.48;
}

.broker-provider-tag__menu button > span:nth-child(2) {
  display: flex;
  min-width: 0;
  flex-direction: column;
}

.broker-provider-tag__menu button strong {
  overflow: hidden;
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.broker-provider-tag__menu button small,
.broker-provider-tag__empty {
  overflow: hidden;
  color: var(--tv-text-dim);
  font-size: 8px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.broker-provider-tag__empty {
  padding: 10px 7px;
}
</style>
