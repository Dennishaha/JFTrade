<script setup lang="ts">
import { computed, ref, watch } from "vue";

import type { OptionComboLegDraft } from "../../composables/optionComboDraft";

const props = defineProps<{
  open: boolean;
  mode: "place" | "cancel";
  accountLabel: string;
  environment: string;
  strategyLabel: string;
  legs: OptionComboLegDraft[];
  price: number;
  quantity: number;
  realConfirmationRequired: boolean;
  requiredConfirmationText: string;
}>();

const emit = defineEmits<{
  close: [];
  confirm: [];
}>();

const confirmationText = ref("");
const canConfirm = computed(
  () =>
    !props.realConfirmationRequired ||
    confirmationText.value.trim() === props.requiredConfirmationText,
);

watch(
  () => props.open,
  (open) => {
    if (!open) confirmationText.value = "";
  },
);
</script>

<template>
  <div
    v-if="open"
    class="combo-confirm"
    role="dialog"
    aria-modal="true"
    :aria-label="mode === 'place' ? '确认组合期权下单' : '确认组合期权撤单'"
    @click.self="emit('close')"
  >
    <section class="combo-confirm__panel">
      <header>
        <strong>{{ mode === "place" ? "确认组合期权下单" : "确认撤销组合订单" }}</strong>
        <button type="button" aria-label="关闭确认弹窗" @click="emit('close')">×</button>
      </header>
      <dl>
        <div><dt>账户</dt><dd>{{ accountLabel || "默认账户" }} · {{ environment }}</dd></div>
        <div><dt>策略</dt><dd>{{ strategyLabel }}</dd></div>
        <div v-if="mode === 'place'"><dt>委托</dt><dd>限价 {{ price }} · {{ quantity }} 组 · 当日有效</dd></div>
      </dl>
      <div v-if="mode === 'place'" class="combo-confirm__legs">
        <div v-for="(leg, index) in legs" :key="leg.instrumentId">
          <span>{{ index + 1 }}</span>
          <strong :class="leg.side === 'BUY' ? 'is-buy' : 'is-sell'">
            {{ leg.side === "BUY" ? "买" : "卖" }} {{ leg.ratio }}
          </strong>
          <code>{{ leg.instrumentId }}</code>
          <small>{{ leg.expiry }} · {{ leg.optionType === "call" ? "Call" : "Put" }} {{ leg.strike }}</small>
        </div>
      </div>
      <label v-if="realConfirmationRequired">
        实盘确认：请输入 {{ requiredConfirmationText }}
        <input v-model="confirmationText" autocomplete="off" />
      </label>
      <footer>
        <button type="button" @click="emit('close')">返回检查</button>
        <button
          type="button"
          class="is-primary"
          :disabled="!canConfirm"
          @click="emit('confirm')"
        >
          {{ mode === "place" ? "确认提交" : "确认撤单" }}
        </button>
      </footer>
    </section>
  </div>
</template>

<style scoped>
.combo-confirm {
  position: fixed;
  z-index: 90;
  display: grid;
  inset: 0;
  place-items: center;
  padding: 20px;
  background: rgba(2, 6, 23, 0.66);
}

.combo-confirm__panel {
  width: min(620px, 100%);
  max-height: min(720px, calc(100vh - 40px));
  padding: 14px;
  overflow: auto;
  border: 1px solid var(--tv-border);
  border-radius: 7px;
  background: var(--tv-bg-elevated);
  box-shadow: 0 22px 70px rgba(0, 0, 0, 0.38);
}

.combo-confirm header,
.combo-confirm footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.combo-confirm header {
  padding-bottom: 10px;
  border-bottom: 1px solid var(--tv-border);
}

.combo-confirm button,
.combo-confirm input {
  min-height: 30px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
}

.combo-confirm button {
  padding: 0 12px;
  cursor: pointer;
}

.combo-confirm header button {
  min-width: 30px;
  padding: 0;
  border: 0;
  background: transparent;
}

.combo-confirm dl {
  display: grid;
  gap: 4px;
  margin: 10px 0;
}

.combo-confirm dl div {
  display: grid;
  grid-template-columns: 64px 1fr;
  gap: 8px;
}

.combo-confirm dt,
.combo-confirm small,
.combo-confirm label {
  color: var(--tv-text-dim);
  font-size: 10px;
}

.combo-confirm dd {
  margin: 0;
  color: var(--tv-text);
  font-size: 11px;
}

.combo-confirm__legs {
  display: grid;
  gap: 4px;
  margin: 10px 0;
}

.combo-confirm__legs div {
  display: grid;
  grid-template-columns: 20px 52px minmax(150px, 1fr) auto;
  align-items: center;
  gap: 6px;
  min-height: 32px;
  border-bottom: 1px solid var(--tv-border);
}

.combo-confirm__legs code {
  overflow: hidden;
  text-overflow: ellipsis;
}

.combo-confirm .is-buy {
  color: var(--tv-price-up);
}

.combo-confirm .is-sell {
  color: var(--tv-price-down);
}

.combo-confirm label {
  display: grid;
  gap: 5px;
  margin: 12px 0;
}

.combo-confirm input {
  padding: 0 8px;
}

.combo-confirm footer {
  justify-content: flex-end;
  gap: 8px;
  padding-top: 12px;
}

.combo-confirm button.is-primary {
  border-color: var(--tv-accent);
  background: var(--tv-accent);
  color: var(--tv-bg);
  font-weight: 700;
}

.combo-confirm button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}
</style>
