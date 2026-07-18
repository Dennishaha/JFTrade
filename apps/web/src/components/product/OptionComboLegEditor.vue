<script setup lang="ts">
import { ref } from "vue";

import type {
  OptionComboLegDraft,
  OptionComboSide,
  OptionContractChoice,
} from "../../composables/optionComboDraft";

defineProps<{
  legs: OptionComboLegDraft[];
  contracts: OptionContractChoice[];
}>();

const emit = defineEmits<{
  add: [instrumentId: string];
  remove: [instrumentId: string];
  update: [
    instrumentId: string,
    patch: Partial<Pick<OptionComboLegDraft, "side" | "ratio">>,
  ];
}>();

const selectedInstrumentId = ref("");

function addSelected(): void {
  if (!selectedInstrumentId.value) return;
  emit("add", selectedInstrumentId.value);
  selectedInstrumentId.value = "";
}

function quoteText(value: number | null): string {
  return value == null
    ? "—"
    : new Intl.NumberFormat("zh-CN", {
        minimumFractionDigits: 2,
        maximumFractionDigits: 4,
      }).format(value);
}

function toggleSide(leg: OptionComboLegDraft): void {
  const side: OptionComboSide = leg.side === "BUY" ? "SELL" : "BUY";
  emit("update", leg.instrumentId, { side });
}
</script>

<template>
  <div class="combo-leg-editor">
    <div class="combo-leg-editor__add">
      <label for="option-combo-contract-search">自定义合约</label>
      <select
        id="option-combo-contract-search"
        v-model="selectedInstrumentId"
        aria-label="搜索并选择期权合约"
      >
        <option value="">代码 / 名称 / 到期日</option>
        <option
          v-for="contract in contracts"
          :key="contract.instrumentId"
          :value="contract.instrumentId"
        >
          {{ contract.label }}
        </option>
      </select>
      <button
        type="button"
        :disabled="!selectedInstrumentId || legs.length >= 8"
        @click="addSelected"
      >
        添加
      </button>
    </div>

    <div v-if="legs.length === 0" class="combo-leg-editor__empty">
      点击期权链买价加入卖出腿、卖价加入买入腿，也可在上方搜索合约。
    </div>

    <div v-else class="combo-leg-editor__columns" aria-hidden="true">
      <span />
      <span>合约</span>
      <span>方向</span>
      <span class="combo-leg-editor__quote-heading">买卖报价</span>
      <span>比例</span>
      <span />
    </div>

    <div
      v-for="(leg, index) in legs"
      :key="leg.instrumentId"
      class="combo-leg-editor__row"
    >
      <span class="combo-leg-editor__sequence">{{ index + 1 }}</span>
      <div class="combo-leg-editor__identity">
        <strong>{{ leg.code }}</strong>
        <small>
          {{ leg.expiry }} · {{ leg.optionType === "call" ? "Call" : "Put" }}
          {{ leg.strike }} · ×{{ leg.multiplier }}
        </small>
      </div>
      <button
        type="button"
        class="combo-leg-editor__side"
        :class="leg.side === 'BUY' ? 'is-buy' : 'is-sell'"
        :aria-label="`${leg.code} 当前${leg.side === 'BUY' ? '买入' : '卖出'}，点击反向`"
        @click="toggleSide(leg)"
      >
        {{ leg.side === "BUY" ? "买" : "卖" }}
      </button>
      <span class="combo-leg-editor__quote">
        买 {{ quoteText(leg.bidPrice) }} / 卖 {{ quoteText(leg.askPrice) }}
      </span>
      <div class="combo-leg-editor__ratio">
        <button
          type="button"
          aria-label="减少比例"
          :disabled="leg.ratio <= 1"
          @click="emit('update', leg.instrumentId, { ratio: leg.ratio - 1 })"
        >
          −
        </button>
        <span aria-label="腿比例">{{ leg.ratio }}</span>
        <button
          type="button"
          aria-label="增加比例"
          :disabled="leg.ratio >= 100"
          @click="emit('update', leg.instrumentId, { ratio: leg.ratio + 1 })"
        >
          +
        </button>
      </div>
      <button
        type="button"
        class="combo-leg-editor__remove"
        :aria-label="`移除 ${leg.code}`"
        @click="emit('remove', leg.instrumentId)"
      >
        ×
      </button>
    </div>
  </div>
</template>

<style scoped>
.combo-leg-editor {
  display: grid;
  grid-auto-rows: max-content;
  gap: 5px;
  min-width: 0;
  align-content: start;
  overflow: hidden;
}

.combo-leg-editor__add {
  display: grid;
  grid-template-columns: auto minmax(160px, 1fr) auto;
  align-items: center;
  gap: 6px;
}

.combo-leg-editor__add label,
.combo-leg-editor__quote,
.combo-leg-editor__identity small {
  color: var(--tv-text-dim);
  font-size: 9px;
}

.combo-leg-editor select,
.combo-leg-editor button {
  height: 30px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  font: inherit;
}

.combo-leg-editor select {
  min-width: 0;
  padding: 0 8px;
  font-size: 10px;
}

.combo-leg-editor button {
  padding: 0 9px;
  cursor: pointer;
}

.combo-leg-editor button:hover:not(:disabled),
.combo-leg-editor button:focus-visible {
  border-color: var(--tv-accent);
  color: var(--tv-text);
  outline: none;
}

.combo-leg-editor button:disabled {
  cursor: not-allowed;
  opacity: 0.42;
}

.combo-leg-editor__empty {
  display: grid;
  min-height: 72px;
  place-items: center;
  border: 1px dashed var(--tv-border);
  color: var(--tv-text-dim);
  font-size: 10px;
  text-align: center;
}

.combo-leg-editor__columns,
.combo-leg-editor__row {
  display: grid;
  grid-template-columns:
    22px minmax(130px, 1fr) 42px minmax(112px, auto) 92px 28px;
  min-width: 0;
  align-items: center;
  gap: 6px;
}

.combo-leg-editor__columns {
  min-height: 22px;
  color: var(--tv-text-dim);
  font-size: 8px;
}

.combo-leg-editor__row {
  min-height: 38px;
  border-bottom: 1px solid color-mix(in srgb, var(--tv-border) 68%, transparent);
}

.combo-leg-editor__sequence {
  display: grid;
  width: 18px;
  height: 18px;
  place-items: center;
  border-radius: 50%;
  background: color-mix(in srgb, var(--tv-accent) 18%, transparent);
  color: var(--tv-accent);
  font-size: 9px;
  font-weight: 750;
}

.combo-leg-editor__identity {
  display: flex;
  min-width: 0;
  flex-direction: column;
}

.combo-leg-editor__identity strong {
  overflow: hidden;
  color: var(--tv-text);
  font-family: var(--tv-font-mono, monospace);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.combo-leg-editor__identity small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.combo-leg-editor__side.is-buy {
  color: var(--tv-price-up);
}

.combo-leg-editor__side.is-sell {
  color: var(--tv-price-down);
}

.combo-leg-editor__quote {
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.combo-leg-editor__ratio {
  display: grid;
  grid-template-columns: 30px 1fr 30px;
  align-items: center;
  text-align: center;
}

.combo-leg-editor__ratio button {
  padding: 0;
}

.combo-leg-editor__remove {
  padding: 0 !important;
}

@container option-combo-ticket (max-width: 940px) {
  .combo-leg-editor__columns,
  .combo-leg-editor__row {
    grid-template-columns:
      20px minmax(74px, 1fr) 38px minmax(64px, 82px) 26px;
    gap: 4px;
  }

  .combo-leg-editor__quote-heading,
  .combo-leg-editor__quote {
    display: none;
  }
}

@container option-combo-ticket (max-width: 480px) {
  .combo-leg-editor__add {
    grid-template-columns: minmax(0, 1fr) auto;
  }

  .combo-leg-editor__add label {
    grid-column: 1 / -1;
  }

  .combo-leg-editor__columns,
  .combo-leg-editor__row {
    grid-template-columns:
      18px minmax(60px, 1fr) 34px minmax(54px, 70px) 24px;
    gap: 3px;
  }
}
</style>
