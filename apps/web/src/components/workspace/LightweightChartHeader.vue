<script setup lang="ts">
import type { ChartType, KlineIndicatorKey } from "../../charting/kline";
import type { LiveSocketConnectionState } from "@/composables/market-data/sharedLiveSocket";
import KlineIndicatorSelector from "@/components/domain/market-data/KlineIndicatorSelector.vue";
import MarketFeedStatus from "../domain/market-data/MarketFeedStatus.vue";
import LightweightChartTypeSelector from "./LightweightChartTypeSelector.vue";
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import {
  CANDLE_SESSION_LABELS,
  CANDLE_SESSION_ORDER,
  summarizeCandleSessions,
  type CandleSession,
} from "@/composables/market-data/candleSessions";

interface PeriodOption {
  value: string;
  label: string;
}

const props = defineProps<{
  variant: "workspace" | "embedded";
  market: string;
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
  providerName: string | null;
  fromCache: boolean;
  loadingData: boolean;
  dataError: string;
  indicators: KlineIndicatorKey[];
  candleSessions: readonly CandleSession[];
  supportedCandleSessions: readonly CandleSession[];
}>();

const showCandleSessionSelector = computed(() => {
  if (props.supportedCandleSessions.length > 1) return true;
  return (
    props.market.trim().toUpperCase() === "US" &&
    props.supportedCandleSessions.length === 1 &&
    props.supportedCandleSessions[0] === "regular"
  );
});

const emit = defineEmits<{
  "update:indicators": [indicators: KlineIndicatorKey[]];
  "select-period": [period: string];
  "select-chart-type": [chartType: ChartType];
  retry: [];
  refresh: [];
  "update:candle-sessions": [sessions: CandleSession[]];
}>();

const sessionMenuOpen = ref(false);
const sessionTrigger = ref<HTMLElement | null>(null);
const sessionMenu = ref<HTMLElement | null>(null);
const sessionMenuTop = ref(0);
const sessionMenuLeft = ref(0);
const SESSION_PANEL_GAP = 4;
const SESSION_VIEWPORT_GAP = 8;

function syncSessionMenuPosition(): void {
  const trigger = sessionTrigger.value;
  const menu = sessionMenu.value;
  if (trigger == null || menu == null || typeof window === "undefined") return;

  const triggerRect = trigger.getBoundingClientRect();
  const menuRect = menu.getBoundingClientRect();
  const maxLeft = Math.max(
    SESSION_VIEWPORT_GAP,
    window.innerWidth - menuRect.width - SESSION_VIEWPORT_GAP,
  );
  sessionMenuLeft.value = Math.min(
    Math.max(triggerRect.left, SESSION_VIEWPORT_GAP),
    maxLeft,
  );

  const below = triggerRect.bottom + SESSION_PANEL_GAP;
  const above = triggerRect.top - menuRect.height - SESSION_PANEL_GAP;
  const maxTop = Math.max(
    SESSION_VIEWPORT_GAP,
    window.innerHeight - menuRect.height - SESSION_VIEWPORT_GAP,
  );
  sessionMenuTop.value =
    below + menuRect.height <= window.innerHeight - SESSION_VIEWPORT_GAP
      ? below
      : above >= SESSION_VIEWPORT_GAP
        ? above
        : Math.min(Math.max(below, SESSION_VIEWPORT_GAP), maxTop);
}

async function toggleSessionMenu(): Promise<void> {
  if (props.loadingCapabilities || props.supportedCandleSessions.length === 0) return;
  sessionMenuOpen.value = !sessionMenuOpen.value;
  if (!sessionMenuOpen.value) return;
  await nextTick();
  syncSessionMenuPosition();
}

function closeSessionMenu(options?: { restoreTriggerFocus?: boolean }): void {
  sessionMenuOpen.value = false;
  if (options?.restoreTriggerFocus) sessionTrigger.value?.focus();
}

function isSessionSelected(session: CandleSession): boolean {
  return props.candleSessions.includes(session);
}

function isSessionSupported(session: CandleSession): boolean {
  return props.supportedCandleSessions.includes(session);
}

function updateSession(session: CandleSession, checked: boolean): void {
  if (!isSessionSupported(session)) return;
  const selected = new Set(props.candleSessions);
  if (checked) selected.add(session);
  else if (selected.size > 1) selected.delete(session);
  emit("update:candle-sessions", CANDLE_SESSION_ORDER.filter((value) => selected.has(value)));
}

function handleDocumentPointerDown(event: PointerEvent): void {
  const target = event.target;
  if (!(target instanceof Node)) return;
  if (
    (sessionTrigger.value?.contains(target) ?? false) ||
    (sessionMenu.value?.contains(target) ?? false)
  ) return;
  closeSessionMenu();
}

function handleDocumentKeydown(event: KeyboardEvent): void {
  if (event.key !== "Escape" || !sessionMenuOpen.value) return;
  event.preventDefault();
  closeSessionMenu({ restoreTriggerFocus: true });
}

onMounted(() => {
  document.addEventListener("pointerdown", handleDocumentPointerDown);
  document.addEventListener("keydown", handleDocumentKeydown);
  window.addEventListener("resize", syncSessionMenuPosition);
  window.addEventListener("scroll", syncSessionMenuPosition, true);
});
onBeforeUnmount(() => {
  document.removeEventListener("pointerdown", handleDocumentPointerDown);
  document.removeEventListener("keydown", handleDocumentKeydown);
  window.removeEventListener("resize", syncSessionMenuPosition);
  window.removeEventListener("scroll", syncSessionMenuPosition, true);
});

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
      <div
        v-if="showCandleSessionSelector"
        class="lightweight-chart-session-selector"
      >
        <button
          ref="sessionTrigger"
          class="lightweight-chart-session-selector__trigger"
          type="button"
          :class="{ 'is-open': sessionMenuOpen }"
          :disabled="props.loadingCapabilities || props.supportedCandleSessions.length === 0"
          :aria-expanded="sessionMenuOpen"
          aria-haspopup="dialog"
          title="交易时段"
          @click="toggleSessionMenu"
        >
          <span class="lightweight-chart-session-selector__label">时段</span>
          <span class="lightweight-chart-session-selector__summary">{{ summarizeCandleSessions(props.candleSessions) }}</span>
        </button>
        <Teleport to="body">
          <div
            v-if="sessionMenuOpen"
            ref="sessionMenu"
            class="lightweight-chart-session-selector__menu"
            role="dialog"
            aria-label="交易时段"
            :style="{ top: `${sessionMenuTop}px`, left: `${sessionMenuLeft}px` }"
          >
            <header class="lightweight-chart-session-selector__header">
              <strong>交易时段</strong>
              <button
                class="lightweight-chart-session-selector__close"
                type="button"
                title="关闭"
                aria-label="关闭交易时段选择"
                @click="closeSessionMenu({ restoreTriggerFocus: true })"
              >
                <span class="fa-solid fa-xmark" aria-hidden="true" />
              </button>
            </header>
            <div class="lightweight-chart-session-selector__options">
              <label
                v-for="session in CANDLE_SESSION_ORDER"
                :key="session"
                class="lightweight-chart-session-selector__option"
                :class="{ 'is-disabled': !isSessionSupported(session) }"
              >
                <input
                  type="checkbox"
                  :checked="isSessionSelected(session)"
                  :disabled="!isSessionSupported(session) || (isSessionSelected(session) && props.candleSessions.length === 1)"
                  @change="updateSession(session, ($event.target as HTMLInputElement).checked)"
                />
                <span>{{ CANDLE_SESSION_LABELS[session] }}</span>
              </label>
            </div>
          </div>
        </Teleport>
      </div>
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
      :provider-name="props.providerName"
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
  font-size: var(--jf-text-5);
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
  font-size: var(--jf-text-3);
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
  font-size: var(--jf-text-6);
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
