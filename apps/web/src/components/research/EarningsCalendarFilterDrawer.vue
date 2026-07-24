<script setup lang="ts">
import { computed, ref, watch } from "vue";

import {
  createEarningsCalendarFilters,
  isEarningsOptionMarket,
  validateEarningsCalendarFilters,
  type EarningsCalendarFilters,
} from "./earningsCalendarModel";

const props = defineProps<{
  open: boolean;
  market: string;
  value: EarningsCalendarFilters;
}>();
const emit = defineEmits<{
  close: [];
  apply: [filters: EarningsCalendarFilters];
}>();

const draft = ref<EarningsCalendarFilters>(createEarningsCalendarFilters());
const errors = ref<Record<string, string>>({});
const optionMarket = computed(() => isEarningsOptionMarket(props.market));

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    draft.value = { ...props.value };
    errors.value = {};
  },
  { immediate: true },
);

function reset(): void {
  draft.value = createEarningsCalendarFilters();
  errors.value = {};
}

function apply(): void {
  const validation = validateEarningsCalendarFilters(draft.value, props.market);
  errors.value = validation.errors;
  if (!validation.valid) return;
  emit("apply", { ...draft.value });
}

function close(): void {
  emit("close");
}
</script>

<template>
  <div
    v-if="open"
    class="earnings-filter-drawer__backdrop"
    @click.self="close"
    @keydown.esc.stop="close"
  >
    <aside
      class="earnings-filter-drawer"
      role="dialog"
      aria-modal="true"
      aria-labelledby="earnings-filter-title"
    >
      <header class="earnings-filter-drawer__header">
        <h3 id="earnings-filter-title">筛选</h3>
        <button
          type="button"
          class="earnings-filter-drawer__icon-button"
          aria-label="关闭筛选"
          @click="close"
        >
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="M6 6l12 12M18 6L6 18" />
          </svg>
        </button>
      </header>

      <div class="earnings-filter-drawer__body">
        <label class="earnings-filter-drawer__field">
          <span>股票范围</span>
          <select v-model="draft.stockScope">
            <option value="all">全部</option>
            <option value="watchlist">自选股</option>
            <option value="position">持仓</option>
            <option value="special">特别关注</option>
          </select>
        </label>

        <div class="earnings-filter-drawer__range">
          <span class="earnings-filter-drawer__label">市值</span>
          <label>
            <input
              v-model="draft.marketCapMin"
              type="number"
              min="0"
              step="any"
              placeholder="0"
              aria-label="市值下限，单位亿"
              :aria-invalid="Boolean(errors.marketCapMin)"
            >
            <span>亿</span>
          </label>
          <i>–</i>
          <label>
            <input
              v-model="draft.marketCapMax"
              type="number"
              min="0"
              step="any"
              placeholder="+∞"
              aria-label="市值上限，单位亿"
              :aria-invalid="Boolean(errors.marketCapMax)"
            >
            <span>亿</span>
          </label>
          <small v-if="errors.marketCapMin || errors.marketCapMax">
            {{ errors.marketCapMin || errors.marketCapMax }}
          </small>
        </div>

        <template v-if="optionMarket">
          <div class="earnings-filter-drawer__range">
            <span class="earnings-filter-drawer__label">期权成交量</span>
            <label>
              <input
                v-model="draft.optionVolumeMin"
                type="number"
                min="0"
                step="any"
                placeholder="0"
                aria-label="期权成交量下限，单位万"
                :aria-invalid="Boolean(errors.optionVolumeMin)"
              >
              <span>万</span>
            </label>
            <i>–</i>
            <label>
              <input
                v-model="draft.optionVolumeMax"
                type="number"
                min="0"
                step="any"
                placeholder="+∞"
                aria-label="期权成交量上限，单位万"
                :aria-invalid="Boolean(errors.optionVolumeMax)"
              >
              <span>万</span>
            </label>
            <small v-if="errors.optionVolumeMin || errors.optionVolumeMax">
              {{ errors.optionVolumeMin || errors.optionVolumeMax }}
            </small>
          </div>

          <div class="earnings-filter-drawer__range">
            <span class="earnings-filter-drawer__label">隐含波动率</span>
            <label>
              <input
                v-model="draft.ivMin"
                type="number"
                min="0"
                max="100"
                step="any"
                placeholder="0"
                aria-label="隐含波动率下限"
                :aria-invalid="Boolean(errors.ivMin)"
              >
              <span>%</span>
            </label>
            <i>–</i>
            <label>
              <input
                v-model="draft.ivMax"
                type="number"
                min="0"
                max="100"
                step="any"
                placeholder="100"
                aria-label="隐含波动率上限"
                :aria-invalid="Boolean(errors.ivMax)"
              >
              <span>%</span>
            </label>
            <small v-if="errors.ivMin || errors.ivMax">
              {{ errors.ivMin || errors.ivMax }}
            </small>
          </div>

          <div class="earnings-filter-drawer__range">
            <span class="earnings-filter-drawer__label">IV 等级</span>
            <label>
              <input
                v-model="draft.ivRankMin"
                type="number"
                min="0"
                max="100"
                step="any"
                placeholder="0"
                aria-label="IV 等级下限"
                :aria-invalid="Boolean(errors.ivRankMin)"
              >
              <span>%</span>
            </label>
            <i>–</i>
            <label>
              <input
                v-model="draft.ivRankMax"
                type="number"
                min="0"
                max="100"
                step="any"
                placeholder="100"
                aria-label="IV 等级上限"
                :aria-invalid="Boolean(errors.ivRankMax)"
              >
              <span>%</span>
            </label>
            <small v-if="errors.ivRankMin || errors.ivRankMax">
              {{ errors.ivRankMin || errors.ivRankMax }}
            </small>
          </div>

          <div class="earnings-filter-drawer__range">
            <span class="earnings-filter-drawer__label">IV 百分位数</span>
            <label>
              <input
                v-model="draft.ivPercentileMin"
                type="number"
                min="0"
                max="100"
                step="any"
                placeholder="0"
                aria-label="IV 百分位数下限"
                :aria-invalid="Boolean(errors.ivPercentileMin)"
              >
              <span>%</span>
            </label>
            <i>–</i>
            <label>
              <input
                v-model="draft.ivPercentileMax"
                type="number"
                min="0"
                max="100"
                step="any"
                placeholder="100"
                aria-label="IV 百分位数上限"
                :aria-invalid="Boolean(errors.ivPercentileMax)"
              >
              <span>%</span>
            </label>
            <small v-if="errors.ivPercentileMin || errors.ivPercentileMax">
              {{ errors.ivPercentileMin || errors.ivPercentileMax }}
            </small>
          </div>
        </template>
      </div>

      <footer class="earnings-filter-drawer__footer">
        <button type="button" class="tv-button" @click="reset">重置</button>
        <button type="button" class="tv-button tv-button--primary" @click="apply">
          应用
        </button>
      </footer>
    </aside>
  </div>
</template>

<style scoped>
.earnings-filter-drawer__backdrop {
  position: absolute;
  z-index: 30;
  inset: 0;
  display: flex;
  justify-content: flex-end;
  background: rgb(3 7 18 / 42%);
}

.earnings-filter-drawer {
  display: flex;
  width: min(430px, 100%);
  height: 100%;
  flex-direction: column;
  border-left: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
  box-shadow: -16px 0 38px rgb(0 0 0 / 24%);
}

.earnings-filter-drawer__header {
  display: flex;
  min-height: 64px;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  border-bottom: 1px solid var(--tv-border);
}

.earnings-filter-drawer__header h3 {
  margin: 0;
  font-size: 20px;
  font-weight: 650;
}

.earnings-filter-drawer__icon-button {
  display: grid;
  width: 36px;
  height: 36px;
  place-items: center;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
}

.earnings-filter-drawer__icon-button:hover {
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
}

.earnings-filter-drawer__icon-button svg {
  width: 22px;
  fill: none;
  stroke: currentColor;
  stroke-linecap: round;
  stroke-width: 1.8;
}

.earnings-filter-drawer__body {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  gap: 22px;
  padding: 22px 20px;
  overflow-y: auto;
}

.earnings-filter-drawer__field {
  display: grid;
  grid-template-columns: 118px minmax(0, 1fr);
  align-items: center;
  color: var(--tv-text);
  font-size: 14px;
  font-weight: 600;
}

.earnings-filter-drawer__field select {
  height: 38px;
  padding: 0 12px;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
}

.earnings-filter-drawer__range {
  display: grid;
  grid-template-columns: 118px minmax(95px, 1fr) 14px minmax(95px, 1fr);
  align-items: center;
  gap: 7px;
}

.earnings-filter-drawer__label {
  color: var(--tv-text);
  font-size: 14px;
  font-weight: 600;
}

.earnings-filter-drawer__range label {
  display: flex;
  height: 38px;
  align-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-elevated);
}

.earnings-filter-drawer__range input {
  width: 0;
  min-width: 0;
  flex: 1;
  padding: 0 4px 0 10px;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--tv-text);
  font: inherit;
}

.earnings-filter-drawer__range input::-webkit-inner-spin-button {
  appearance: none;
}

.earnings-filter-drawer__range label:focus-within {
  border-color: var(--tv-accent);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--tv-accent) 20%, transparent);
}

.earnings-filter-drawer__range label:has(input[aria-invalid="true"]) {
  border-color: var(--tv-status-error-border);
}

.earnings-filter-drawer__range label > span {
  padding-right: 9px;
  color: var(--tv-text-muted);
}

.earnings-filter-drawer__range > i {
  color: var(--tv-text-dim);
  font-style: normal;
  text-align: center;
}

.earnings-filter-drawer__range small {
  grid-column: 2 / -1;
  color: var(--tv-status-error-fg);
  font-size: 11px;
}

.earnings-filter-drawer__footer {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  padding: 14px 20px;
  border-top: 1px solid var(--tv-border);
}

.earnings-filter-drawer__footer .tv-button {
  justify-content: center;
}

@media (max-width: 640px) {
  .earnings-filter-drawer__field,
  .earnings-filter-drawer__range {
    grid-template-columns: 1fr 14px 1fr;
  }

  .earnings-filter-drawer__field > span,
  .earnings-filter-drawer__label {
    grid-column: 1 / -1;
    margin-bottom: 4px;
  }

  .earnings-filter-drawer__field select {
    grid-column: 1 / -1;
  }

  .earnings-filter-drawer__range small {
    grid-column: 1 / -1;
  }
}
</style>
