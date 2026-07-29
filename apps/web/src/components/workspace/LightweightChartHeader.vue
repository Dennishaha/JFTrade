<script setup lang="ts">
import type { ChartType, KlineIndicatorKey } from "../../charting/kline";
import type { LiveSocketConnectionState } from "../../composables/sharedLiveSocket";
import KlineIndicatorSelector from "../KlineIndicatorSelector.vue";
import MarketFeedStatus from "../domain/market-data/MarketFeedStatus.vue";
import LightweightChartTypeSelector from "./LightweightChartTypeSelector.vue";

interface PeriodOption {
  value: string;
  label: string;
}

const props = defineProps<{
  variant: "workspace" | "embedded";
  periods: readonly PeriodOption[];
  selectedPeriod: string;
  loadingCapabilities: boolean;
  capabilitiesError: string;
  activeChartType: ChartType;
  activeChartTypeLabel: string;
  tickPeriod: boolean;
  connectionState: LiveSocketConnectionState;
  observedAt: string | null;
  transportMode: string | null;
  source: string | null;
  fromCache: boolean;
  loadingData: boolean;
  dataError: string;
  indicators: KlineIndicatorKey[];
}>();

const emit = defineEmits<{
  "update:indicators": [indicators: KlineIndicatorKey[]];
  "select-period": [period: string];
  "select-chart-type": [chartType: ChartType];
  retry: [];
  refresh: [];
}>();

function handlePeriodChange(event: Event): void {
  emit("select-period", (event.currentTarget as HTMLSelectElement).value);
}
</script>

<template>
  <div
    class="tv-panel-head lightweight-chart-head"
    :class="{ 'lightweight-chart-head--workspace': props.variant === 'workspace' }"
  >
    <div class="lightweight-chart-head__primary-controls">
      <div class="tv-seg lightweight-chart-head__periods">
        <button
          v-for="period in props.periods"
          :key="period.value"
          type="button"
          :class="{ 'is-active': props.selectedPeriod === period.value }"
          :disabled="props.loadingCapabilities"
          @click="emit('select-period', period.value)"
        >
          {{ period.label }}
        </button>
      </div>
      <label class="lightweight-chart-head__period-select">
        <select
          aria-label="K 线周期"
          :value="props.selectedPeriod"
          :disabled="props.loadingCapabilities || props.periods.length === 0"
          @change="handlePeriodChange"
        >
          <option v-if="props.periods.length === 0" value="">--</option>
          <option v-for="period in props.periods" :key="period.value" :value="period.value">
            {{ period.label }}
          </option>
        </select>
        <span
          class="fa-solid fa-chevron-down lightweight-chart-head__period-chevron"
          aria-hidden="true"
        />
      </label>
      <LightweightChartTypeSelector
        :active-chart-type="props.activeChartType"
        :active-chart-type-label="props.activeChartTypeLabel"
        :tick-period="props.tickPeriod"
        @select="emit('select-chart-type', $event)"
      />
      <KlineIndicatorSelector
        :model-value="props.indicators"
        storage-key="jftrade.workspace-chart.indicators"
        :default-indicators="['volume']"
        @update:model-value="emit('update:indicators', $event)"
      />
    </div>
    <span v-if="props.loadingCapabilities" class="lightweight-chart-head__capability-state">
      正在读取周期能力
    </span>
    <button
      v-else-if="props.capabilitiesError"
      class="lightweight-chart-head__capability-retry"
      type="button"
      title="周期能力加载失败，点击重试"
      @click="emit('retry')"
    >
      周期能力加载失败，点击重试
    </button>
    <div class="lightweight-chart-head__spacer" />
    <MarketFeedStatus
      class="lightweight-chart-head__feed-status"
      :connection-state="props.connectionState"
      :observed-at="props.observedAt"
      :transport-mode="props.transportMode"
      :source="props.source"
      :from-cache="props.fromCache"
      :loading="props.loadingData"
      :error="props.dataError"
    />
    <button
      class="tv-icon-btn lightweight-chart-head__refresh"
      type="button"
      title="刷新"
      @click="emit('refresh')"
    >
      ↻
    </button>
  </div>
</template>

<style scoped>
.lightweight-chart-head {
  min-width: 0;
  overflow: hidden;
}

.lightweight-chart-head--workspace {
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
