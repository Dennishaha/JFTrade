<script setup lang="ts">
import { computed } from "vue";

interface OrderBookDepthLevel {
  bidPrice: number | null;
  askPrice: number | null;
  bidSize: number;
  askSize: number;
}

const props = withDefaults(defineProps<{
  levels: OrderBookDepthLevel[];
  loading?: boolean;
  error?: string;
  disabled?: boolean;
}>(), {
  loading: false,
  error: "",
  disabled: false,
});

const maxBidSize = computed(() => Math.max(...props.levels.map((level) => level.bidSize), 1));
const maxAskSize = computed(() => Math.max(...props.levels.map((level) => level.askSize), 1));

function formatPrice(value: number | null): string {
  if (value == null) return "--";
  return value.toFixed(value < 1 ? 4 : value < 10 ? 3 : 2);
}

function formatSize(value: number | null): string {
  if (value == null) return "--";
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)}B`;
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`;
  return value.toFixed(0);
}

function barWidth(max: number, value: number): string {
  if (max <= 0) return "0%";
  return `${((value / max) * 100).toFixed(1)}%`;
}
</script>

<template>
  <div v-if="levels.length > 0" class="tv-ob-depth-table-wrap" data-state="normal">
    <div class="tv-ob-depth-table">
      <div class="tv-ob-depth-col tv-ob-depth-bid-size-col" data-testid="depth-bid-size-col">
        <div v-for="(row, index) in levels" :key="`b${index}`" class="tv-ob-depth-row tv-ob-depth-row-bid">
          <span class="tv-ob-depth-size">{{ formatSize(row.bidSize) }}</span>
          <div class="tv-ob-depth-bar" :style="{ width: barWidth(maxBidSize, row.bidSize) }"></div>
        </div>
      </div>
      <div class="tv-ob-depth-col tv-ob-depth-bid-price-col" data-testid="depth-bid-price-col">
        <div v-for="(row, index) in levels" :key="`bp${index}`" class="tv-ob-depth-row tv-ob-depth-price tv-ob-depth-bid-price">
          {{ formatPrice(row.bidPrice) }}
        </div>
      </div>
      <div class="tv-ob-depth-col tv-ob-depth-ask-price-col" data-testid="depth-ask-price-col">
        <div v-for="(row, index) in levels" :key="`ap${index}`" class="tv-ob-depth-row tv-ob-depth-price tv-ob-depth-ask-price">
          {{ formatPrice(row.askPrice) }}
        </div>
      </div>
      <div class="tv-ob-depth-col tv-ob-depth-ask-size-col" data-testid="depth-ask-size-col">
        <div v-for="(row, index) in levels" :key="`a${index}`" class="tv-ob-depth-row tv-ob-depth-row-ask">
          <div class="tv-ob-depth-bar" :style="{ width: barWidth(maxAskSize, row.askSize) }"></div>
          <span class="tv-ob-depth-size">{{ formatSize(row.askSize) }}</span>
        </div>
      </div>
    </div>
  </div>

  <div v-else-if="loading && !error" class="tv-ob-depth-placeholder" data-state="loading" aria-live="polite">
    <div class="tv-ob-depth-placeholder-icon"><span class="fa-solid fa-spinner fa-spin"></span></div>
    <div class="tv-ob-depth-placeholder-text">正在加载盘口</div>
  </div>

  <div v-else-if="error" class="tv-ob-depth-placeholder" data-state="error" role="alert">
    <div class="tv-ob-depth-placeholder-icon"><span class="fa-solid fa-triangle-exclamation"></span></div>
    <div class="tv-ob-depth-placeholder-text">数据获取失败</div>
    <div class="tv-ob-depth-placeholder-hint">{{ error }}</div>
  </div>

  <div v-else-if="disabled" class="tv-ob-depth-placeholder" data-state="disabled" aria-disabled="true">
    <div class="tv-ob-depth-placeholder-icon"><span class="fa-solid fa-ban"></span></div>
    <div class="tv-ob-depth-placeholder-text">盘口不可用</div>
    <div class="tv-ob-depth-placeholder-hint">请先选择有效标的。</div>
  </div>

  <div v-else class="tv-ob-depth-placeholder" data-state="empty">
    <div class="tv-ob-depth-placeholder-icon"><span class="fa-solid fa-chart-bar"></span></div>
    <div class="tv-ob-depth-placeholder-text">暂无深度数据</div>
    <div class="tv-ob-depth-placeholder-hint">行情连接恢复后会自动更新。</div>
  </div>
</template>

<style scoped>
.tv-ob-depth-table-wrap {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-gutter: stable both-edges;
}

.tv-ob-depth-table {
  display: grid;
  grid-template-columns: 1fr auto auto 1fr;
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}

.tv-ob-depth-col {
  display: flex;
  flex-direction: column;
}

.tv-ob-depth-row {
  position: relative;
  display: flex;
  align-items: center;
  height: 20px;
  padding: 0 6px;
}

.tv-ob-depth-row-bid {
  justify-content: flex-end;
  color: var(--tv-up);
}

.tv-ob-depth-row-bid .tv-ob-depth-bar {
  position: absolute;
  top: 2px;
  right: 0;
  bottom: 2px;
  border-radius: 0 2px 2px 0;
  background: color-mix(in srgb, var(--tv-up) 12%, transparent);
}

.tv-ob-depth-row-ask {
  justify-content: flex-start;
  color: var(--tv-down);
}

.tv-ob-depth-row-ask .tv-ob-depth-bar {
  position: absolute;
  top: 2px;
  bottom: 2px;
  left: 0;
  border-radius: 2px 0 0 2px;
  background: color-mix(in srgb, var(--tv-down) 10%, transparent);
}

.tv-ob-depth-price {
  min-width: 64px;
  justify-content: center;
  padding: 0 10px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  font-weight: 600;
}

.tv-ob-depth-bid-price {
  justify-content: flex-end;
  border-right: 1px solid var(--tv-border);
  border-left: 1px solid var(--tv-border);
}

.tv-ob-depth-ask-price {
  justify-content: flex-start;
  border-right: 1px solid var(--tv-border);
}

.tv-ob-depth-size {
  position: relative;
  z-index: 1;
}

.tv-ob-depth-placeholder {
  display: flex;
  flex: 1;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 24px;
  color: var(--tv-text-dim);
}

.tv-ob-depth-placeholder[data-state="disabled"] {
  opacity: 0.65;
}

.tv-ob-depth-placeholder-icon {
  font-size: 28px;
  opacity: 0.4;
}

.tv-ob-depth-placeholder-text {
  font-size: 13px;
  font-weight: 500;
}

.tv-ob-depth-placeholder-hint {
  max-width: 180px;
  font-size: 11px;
  line-height: 1.4;
  text-align: center;
}
</style>
