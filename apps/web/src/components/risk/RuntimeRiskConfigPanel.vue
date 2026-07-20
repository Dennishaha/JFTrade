<script setup lang="ts">
import { computed, reactive, watch } from "vue";

import type { RealTradeRiskStateResponse } from "@/contracts";

const props = defineProps<{
  loading?: boolean;
  riskState: RealTradeRiskStateResponse;
}>();

const emit = defineEmits<{
  disable: [payload: { operatorId: string; reason: string }];
  save: [
    payload: {
      realTradingEnabled: boolean;
      maxOrderQuantity: number | null;
      maxOrderNotional: number | null;
      operatorId: string;
      reason: string;
    },
  ];
}>();

const form = reactive({
  realTradingEnabled: false,
  maxOrderQuantity: "",
  maxOrderNotional: "",
  operatorId: "local",
  reason: "",
});

watch(
  () => props.riskState,
  (state) => {
    form.realTradingEnabled = state.realTradingEnabled;
    form.maxOrderQuantity =
      state.runtimeConfiguredMaxOrderQuantity == null
        ? ""
        : String(state.runtimeConfiguredMaxOrderQuantity);
    form.maxOrderNotional =
      state.runtimeConfiguredMaxOrderNotional == null
        ? ""
        : String(state.runtimeConfiguredMaxOrderNotional);
  },
  { immediate: true },
);

const parsedQuantity = computed(() => parseNullableNumber(form.maxOrderQuantity));
const parsedNotional = computed(() => parseNullableNumber(form.maxOrderNotional));
const realTradingEnabledChanged = computed(
  () => form.realTradingEnabled !== props.riskState.realTradingEnabled,
);
const maxOrderQuantityChanged = computed(
  () =>
    !Object.is(
      parsedQuantity.value,
      props.riskState.runtimeConfiguredMaxOrderQuantity,
    ),
);
const maxOrderNotionalChanged = computed(
  () =>
    !Object.is(
      parsedNotional.value,
      props.riskState.runtimeConfiguredMaxOrderNotional,
    ),
);
const hasValidLimit = computed(
  () =>
    (parsedQuantity.value != null && parsedQuantity.value > 0) ||
    (parsedNotional.value != null && parsedNotional.value > 0),
);
const formError = computed(() => {
  if (parsedQuantity.value === Number.NEGATIVE_INFINITY) {
    return "单笔最大数量需要是正数。";
  }
  if (parsedNotional.value === Number.NEGATIVE_INFINITY) {
    return "单笔最大金额需要是正数。";
  }
  if (form.realTradingEnabled && !hasValidLimit.value) {
    return "启用实盘前至少填写一个正数限额。";
  }
  return "";
});
const canSave = computed(() => formError.value === "");

function parseNullableNumber(value: string): number | null {
  const trimmed = value.trim();
  if (trimmed === "") return null;
  const parsed = Number(trimmed);
  if (!Number.isFinite(parsed) || parsed <= 0) return Number.NEGATIVE_INFINITY;
  return parsed;
}

function formatCurrentLimit(value: number | null | undefined): string {
  return value == null ? "未设置" : String(value);
}

function submit() {
  if (!canSave.value) return;
  emit("save", {
    realTradingEnabled: form.realTradingEnabled,
    maxOrderQuantity: parsedQuantity.value,
    maxOrderNotional: parsedNotional.value,
    operatorId: form.operatorId.trim() || "local",
    reason: form.reason.trim(),
  });
}

function disable() {
  emit("disable", {
    operatorId: form.operatorId.trim() || "local",
    reason: form.reason.trim(),
  });
}
</script>

<template>
  <section class="risk-panel" aria-label="实盘总闸与单笔限额">
    <header class="risk-panel__head">
      <span class="risk-panel__title">实盘总闸与单笔限额</span>
      <span class="risk-panel__desc">保存后立即写入运行时配置，不需要重启。</span>
    </header>

    <div class="risk-panel__body">
      <div class="risk-panel__field-row">
        <label class="risk-panel__toggle">
          <input
            v-model="form.realTradingEnabled"
            type="checkbox"
            role="switch"
            aria-label="允许实盘下单"
          />
          <span class="risk-panel__toggle-track" aria-hidden="true"></span>
          <span>允许实盘下单</span>
        </label>
        <span
          class="risk-panel__current"
          data-status-for="real-trading-enabled"
        >
          当前：{{ riskState.realTradingEnabled ? "开启" : "关闭" }}
          <em v-if="realTradingEnabledChanged" class="risk-panel__modified">已修改</em>
        </span>
      </div>

      <div class="risk-panel__fields">
        <div class="risk-panel__field-row">
          <input
            v-model="form.maxOrderQuantity"
            class="tv-input"
            inputmode="decimal"
            placeholder="单笔最大数量"
            aria-label="单笔最大数量"
          />
          <span
            class="risk-panel__current"
            data-status-for="max-order-quantity"
          >
            当前：{{ formatCurrentLimit(riskState.runtimeConfiguredMaxOrderQuantity) }}
            <em v-if="maxOrderQuantityChanged" class="risk-panel__modified">已修改</em>
          </span>
        </div>
        <div class="risk-panel__field-row">
          <input
            v-model="form.maxOrderNotional"
            class="tv-input"
            inputmode="decimal"
            placeholder="单笔最大金额"
            aria-label="单笔最大金额"
          />
          <span
            class="risk-panel__current"
            data-status-for="max-order-notional"
          >
            当前：{{ formatCurrentLimit(riskState.runtimeConfiguredMaxOrderNotional) }}
            <em v-if="maxOrderNotionalChanged" class="risk-panel__modified">已修改</em>
          </span>
        </div>
      </div>

      <div class="risk-panel__fields">
        <input
          v-model="form.operatorId"
          class="tv-input"
          placeholder="操作员"
          aria-label="操作员"
        />
        <input
          v-model="form.reason"
          class="tv-input"
          placeholder="原因"
          aria-label="原因"
        />
      </div>

      <div
        v-if="formError"
        class="risk-panel__error tv-status--warning tv-status-surface"
        role="alert"
      >
        {{ formError }}
      </div>

      <div class="risk-panel__actions">
        <button
          type="button"
          class="tv-btn risk-panel__primary"
          :disabled="!canSave || loading"
          @click="submit"
        >
          {{ loading ? "保存中..." : "保存运行时配置" }}
        </button>
        <button
          type="button"
          class="tv-btn tv-btn-ghost"
          :disabled="loading"
          @click="disable"
        >
          关闭实盘配置
        </button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.risk-panel {
  display: flex;
  min-width: 0;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 8px;
  background: var(--tv-bg-surface);
}

.risk-panel__head {
  display: flex;
  flex: 0 0 auto;
  align-items: baseline;
  gap: 10px;
  padding: 9px 12px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.risk-panel__title {
  color: var(--tv-text-muted);
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.risk-panel__desc {
  overflow: hidden;
  color: var(--tv-text-dim);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.risk-panel__body {
  display: grid;
  gap: 10px;
  padding: 12px;
}

.risk-panel__fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.risk-panel__field-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
}

.risk-panel__toggle {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--tv-text);
  cursor: pointer;
  font-size: 12px;
}

.risk-panel__toggle input {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
}

.risk-panel__toggle-track {
  position: relative;
  width: 30px;
  height: 16px;
  flex: 0 0 auto;
  border: 1px solid var(--tv-border-strong);
  border-radius: 999px;
  background: var(--tv-bg-elevated);
  transition: background 0.12s ease, border-color 0.12s ease;
}

.risk-panel__toggle-track::after {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 10px;
  height: 10px;
  border-radius: 999px;
  background: var(--tv-text-muted);
  content: "";
  transition: transform 0.12s ease, background 0.12s ease;
}

.risk-panel__toggle input:checked + .risk-panel__toggle-track {
  border-color: var(--tv-accent);
  background: color-mix(in srgb, var(--tv-accent) 24%, var(--tv-bg-elevated));
}

.risk-panel__toggle input:checked + .risk-panel__toggle-track::after {
  background: var(--tv-accent);
  transform: translateX(14px);
}

.risk-panel__toggle input:focus-visible + .risk-panel__toggle-track {
  outline: 1px solid var(--tv-accent);
  outline-offset: 2px;
}

.risk-panel__current {
  color: var(--tv-text-dim);
  font-size: 10px;
  white-space: nowrap;
}

.risk-panel__modified {
  margin-left: 6px;
  padding: 1px 6px;
  border: 1px solid var(--tv-status-warning-border);
  border-radius: 999px;
  background: var(--tv-status-warning-bg);
  color: var(--tv-status-warning-fg);
  font-size: 9px;
  font-style: normal;
}

.risk-panel__error {
  padding: 7px 10px;
  border: 1px solid;
  border-radius: 6px;
  font-size: 11px;
}

.risk-panel__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.risk-panel__primary {
  border-color: var(--tv-accent);
  background: var(--tv-accent);
  color: #fff;
}

.risk-panel__actions .tv-btn:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

@media (max-width: 780px) {
  .risk-panel__fields {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
