<script setup lang="ts">
interface AccountImpact {
  nlvChange?: number | null;
  initialMarginChange?: number | null;
  maintenanceMarginChange?: number | null;
  optionBuyingPower?: number | null;
  maxWithdrawalChange?: number | null;
  buyingPowerDecrease?: number | null;
}

defineProps<{
  accountImpact?: AccountImpact | null;
  buyingPowerImpact?: number | null;
  warnings: string[];
  remainingSeconds: number | null;
}>();

function valueText(value: number | null | undefined): string {
  return value == null
    ? "—"
    : new Intl.NumberFormat("zh-CN", {
        maximumFractionDigits: 2,
      }).format(value);
}
</script>

<template>
  <div class="combo-preview-result">
    <strong>预检通过</strong>
    <span>购买力消耗 {{ valueText(accountImpact?.buyingPowerDecrease ?? buyingPowerImpact) }}</span>
    <span>初始保证金 {{ valueText(accountImpact?.initialMarginChange) }}</span>
    <span>维持保证金 {{ valueText(accountImpact?.maintenanceMarginChange) }}</span>
    <span>期权购买力 {{ valueText(accountImpact?.optionBuyingPower) }}</span>
    <span>最大可提变化 {{ valueText(accountImpact?.maxWithdrawalChange) }}</span>
    <span :class="{ 'is-expired': remainingSeconds === 0 }">
      {{ remainingSeconds == null ? "有效期 —" : remainingSeconds === 0 ? "预检已过期" : `${remainingSeconds}s 后过期` }}
    </span>
    <small v-if="warnings.length">{{ warnings.join("；") }}</small>
  </div>
</template>

<style scoped>
.combo-preview-result {
  display: flex;
  min-height: 34px;
  align-items: center;
  gap: 12px;
  padding: 4px 9px;
  overflow-x: auto;
  border: 1px solid var(--tv-status-success-border);
  background: var(--tv-status-success-bg);
  color: var(--tv-text-dim);
  font-size: 9px;
  white-space: nowrap;
}

.combo-preview-result strong {
  color: var(--tv-status-success-fg);
}

.combo-preview-result span {
  font-variant-numeric: tabular-nums;
}

.combo-preview-result .is-expired {
  color: var(--tv-status-error-fg);
}

.combo-preview-result small {
  color: var(--tv-warn);
}
</style>
