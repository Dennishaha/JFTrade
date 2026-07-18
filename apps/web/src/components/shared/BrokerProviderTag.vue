<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";

import {
  brokerProviderOptions,
  configureBrokerProviderDefaults,
  type BrokerCapabilityState,
  type BrokerProviderOption,
  useBrokerProviderSelection,
} from "../../composables/brokerProviderSelection";
import {
  marketDataFeedQualityLabel,
  resolveMarketDataFeedQuality,
} from "../../composables/marketDataFeedQuality";
import type { LiveSocketConnectionState } from "../../composables/sharedLiveSocket";
import type { ProductFeatureProvider } from "../../composables/productFeatures";

const props = withDefaults(
  defineProps<{
    provider?: ProductFeatureProvider | null | undefined;
    featureId?: string | undefined;
    market?: string | undefined;
    preferredBrokerId?: string | undefined;
    defaultBrokerId?: string | undefined;
    connectionState?: LiveSocketConnectionState | undefined;
    transportMode?: string | null | undefined;
    menuLocation?: "bottom end" | "top end";
  }>(),
  {
    provider: null,
    featureId: "",
    market: "",
    preferredBrokerId: "",
    defaultBrokerId: "",
    connectionState: undefined,
    transportMode: null,
    menuLocation: "bottom end",
  },
);

const menuOpen = ref(false);
const {
  loadBrokerProviders,
  loadError,
  loading,
  selectBrokerProvider,
  selectedBrokerId,
} = useBrokerProviderSelection();

const activeFeatureId = computed(
  () => props.featureId.trim() || props.provider?.featureId?.trim() || "",
);
const options = computed(() => {
  const values = brokerProviderOptions(activeFeatureId.value, props.market);
  const actualID = props.provider?.brokerId?.trim().toLowerCase() ?? "";
  if (actualID && !values.some((option) => option.id === actualID)) {
    values.push({
      id: actualID,
      label: props.provider?.securityFirm?.trim() || actualID.toUpperCase(),
      shortLabel: actualID.toUpperCase().slice(0, 12),
      securityFirm: props.provider?.securityFirm?.trim() ?? "",
      state: props.provider?.capability ?? "unavailable",
      reason: props.provider?.selectionReason ?? "",
    });
  }
  return values;
});
const selectedOption = computed<BrokerProviderOption | null>(() => {
  const selected = selectedBrokerId.value;
  const actual = props.provider?.brokerId?.trim().toLowerCase() ?? "";
  return (
    options.value.find((option) => option.id === selected) ??
    options.value.find((option) => option.id === actual) ??
    options.value[0] ??
    null
  );
});
const capabilityState = computed<BrokerCapabilityState>(() => {
  const selected = selectedOption.value;
  const actualID = props.provider?.brokerId?.trim().toLowerCase() ?? "";
  if (selected?.id && selected.id === actualID && props.provider != null) {
    return props.provider.capability;
  }
  return selected?.state ?? "unavailable";
});
const runtimeFeedQuality = computed(() => {
  if (props.connectionState == null) return null;
  return resolveMarketDataFeedQuality({
    connectionState: props.connectionState,
    transportMode: props.transportMode,
  });
});
const runtimeFeedQualityLabel = computed(() => {
  if (props.connectionState == null || runtimeFeedQuality.value == null) {
    return "";
  }
  return marketDataFeedQualityLabel(
    {
      connectionState: props.connectionState,
      transportMode: props.transportMode,
    },
    runtimeFeedQuality.value,
  );
});
const currentState = computed<BrokerCapabilityState>(() => {
  if (capabilityState.value === "unavailable") return "unavailable";
  if (runtimeFeedQuality.value === "unavailable") return "unavailable";
  if (
    capabilityState.value === "degraded" ||
    runtimeFeedQuality.value === "degraded"
  ) {
    return "degraded";
  }
  return capabilityState.value;
});
const currentLabel = computed(
  () =>
    selectedOption.value?.shortLabel || (loading.value ? "加载中" : "数据源"),
);
const currentTitle = computed(() => {
  const selected = selectedOption.value;
  const values = [
    selected?.label || "行情提供者",
    selected?.securityFirm,
    capabilityState.value === "available"
      ? "能力可用"
      : capabilityState.value === "degraded"
        ? "能力降级"
        : "能力不可用",
    runtimeFeedQualityLabel.value
      ? `数据源质量：${runtimeFeedQualityLabel.value}`
      : "",
    selected?.reason,
    loadError.value,
  ];
  return values.filter(Boolean).join(" · ");
});

function select(option: BrokerProviderOption): void {
  if (option.state === "unavailable") return;
  selectBrokerProvider(option.id);
  menuOpen.value = false;
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
        :title="currentTitle"
        :aria-label="runtimeFeedQualityLabel
          ? `切换行情提供者，${runtimeFeedQualityLabel}`
          : '切换行情提供者'"
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
      <button
        v-for="option in options"
        :key="option.id"
        type="button"
        role="option"
        :aria-selected="option.id === selectedOption?.id"
        :disabled="option.state === 'unavailable'"
        :class="[
          `is-${option.state}`,
          { 'is-selected': option.id === selectedOption?.id },
        ]"
        @click="select(option)"
      >
        <span class="broker-provider-tag__option-dot" />
        <span>
          <strong>{{ option.label }}</strong>
          <small v-if="option.securityFirm || option.reason">
            {{ option.securityFirm || option.reason }}
          </small>
        </span>
        <span v-if="option.id === selectedOption?.id" aria-hidden="true"
          >✓</span
        >
      </button>
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
