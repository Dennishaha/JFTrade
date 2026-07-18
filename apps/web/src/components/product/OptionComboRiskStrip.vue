<script setup lang="ts">
interface OptionAnalysis {
  maxProfit?: number | null;
  maxLoss?: number | null;
  maxProfitUnlimited?: boolean;
  maxLossUnlimited?: boolean;
  breakevenPoints?: number[];
  probability?: number | null;
  delta?: number | null;
  theta?: number | null;
}

const props = defineProps<{ analysis?: OptionAnalysis | null }>();

function metric(value: number | null | undefined): string {
  return value == null
    ? "—"
    : new Intl.NumberFormat("zh-CN", {
        maximumFractionDigits: 4,
      }).format(value);
}

function bound(
  value: number | null | undefined,
  unlimited: boolean | undefined,
): string {
  return unlimited ? "无限" : metric(value);
}
</script>

<template>
  <div class="combo-risk-strip" aria-label="组合风险摘要">
    <span>盈利概率 <strong>{{ metric(props.analysis?.probability) }}</strong></span>
    <span>最大盈利 <strong class="is-up">{{ bound(props.analysis?.maxProfit, props.analysis?.maxProfitUnlimited) }}</strong></span>
    <span>最大亏损 <strong class="is-down">{{ bound(props.analysis?.maxLoss, props.analysis?.maxLossUnlimited) }}</strong></span>
    <span>盈亏平衡 <strong>{{ props.analysis?.breakevenPoints?.join(" / ") || "—" }}</strong></span>
    <span>Delta <strong>{{ metric(props.analysis?.delta) }}</strong></span>
    <span>Theta <strong>{{ metric(props.analysis?.theta) }}</strong></span>
  </div>
</template>

<style scoped>
.combo-risk-strip {
  display: flex;
  min-height: 34px;
  align-items: center;
  gap: 18px;
  padding: 0 10px;
  overflow-x: auto;
  border-top: 1px solid var(--tv-border);
  border-bottom: 1px solid var(--tv-border);
  color: var(--tv-text-dim);
  font-size: 9px;
  white-space: nowrap;
}

.combo-risk-strip strong {
  margin-left: 4px;
  color: var(--tv-text);
  font-size: 10px;
  font-variant-numeric: tabular-nums;
}

.combo-risk-strip strong.is-up {
  color: var(--tv-price-up);
}

.combo-risk-strip strong.is-down {
  color: var(--tv-price-down);
}
</style>
