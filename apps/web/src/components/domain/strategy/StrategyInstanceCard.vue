<script setup lang="ts">
import { computed } from "vue";

import { parseStrategyInstrumentIdsText } from "../../strategy-runtime/strategyRuntimeInstanceBinding";
import InstrumentIdentity from "../market-data/InstrumentIdentity.vue";
import RuntimeHealthBadge from "../runtime/RuntimeHealthBadge.vue";
import type { StrategyInstanceCardModel } from "./strategyInstanceCard";

const props = defineProps<{
  model: StrategyInstanceCardModel;
}>();

const emit = defineEmits<{
  select: [strategyId: string];
}>();

const statusToneClass = computed(() => {
  switch (props.model.status.trim().toUpperCase()) {
    case "RUNNING": return "strategy-instance-card--running";
    case "PAUSED": return "strategy-instance-card--paused";
    default: return "strategy-instance-card--stopped";
  }
});
const symbolInstrumentIds = computed(() =>
  parseStrategyInstrumentIdsText(props.model.symbols),
);

function selectStrategy(): void {
  if (!props.model.disabled) {
    emit("select", props.model.id);
  }
}
</script>

<template>
  <button
    :data-testid="`strategy-${model.id}`"
    class="strategy-instance-card"
    :class="[statusToneClass, { 'is-active': model.selected }]"
    :disabled="model.disabled"
    type="button"
    @click="selectStrategy"
  >
    <div class="flex items-center justify-between gap-3">
      <div class="min-w-0 break-words text-base font-semibold text-slate-900 dark:text-slate-100">{{ model.name }}</div>
      <div class="flex flex-wrap items-center justify-end gap-2">
        <div
          v-if="model.definitionStale"
          :data-testid="`strategy-definition-stale-${model.id}`"
          class="rounded-full border border-amber-200 bg-amber-50 px-2.5 py-1 text-[11px] font-semibold uppercase text-amber-700"
        >
          待刷新
        </div>
        <RuntimeHealthBadge
          :data-testid="`strategy-status-${model.id}`"
          :status="model.status"
          :label="model.statusLabel"
          :disabled="model.disabled === true"
        />
      </div>
    </div>
    <div class="mt-2 break-all text-sm text-slate-500">{{ model.id }}</div>
    <div v-if="model.definitionStale" class="mt-2 text-sm text-amber-700">
      {{ model.definitionSyncSummary }}
    </div>
    <div class="mt-2 flex flex-wrap items-center gap-1.5 text-sm text-slate-500">
      <span>标的</span>
      <template v-if="symbolInstrumentIds.length > 0">
        <InstrumentIdentity
          v-for="symbol in symbolInstrumentIds"
          :key="symbol"
          :instrument-id="symbol"
          compact
        />
      </template>
      <span v-else>{{ model.symbols }}</span>
    </div>
    <div class="mt-1 text-sm text-slate-500">周期 {{ model.interval }}</div>
    <div class="mt-1 break-all text-sm text-slate-500">{{ model.brokerAccountSummary }}</div>
    <div
      v-if="model.currentBrokerAccount"
      class="mt-1 inline-flex rounded-full border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-[11px] font-semibold uppercase text-emerald-700"
    >
      当前
    </div>
    <div class="mt-2 text-sm text-slate-500">
      创建于
      <span class="strategy-time-display" :title="model.createdAtTooltip">{{ model.createdAt }}</span>
    </div>
    <div class="mt-3 flex flex-wrap gap-2 text-[11px] font-semibold uppercase">
      <span class="rounded-full border border-slate-200 bg-slate-100 px-2.5 py-1 text-slate-600">
        {{ model.runtimeLabel }}
      </span>
      <span class="rounded-full border border-slate-200 bg-slate-100 px-2.5 py-1 text-slate-600">
        {{ model.sourceFormatLabel }}
      </span>
      <span
        class="rounded-full px-2.5 py-1"
        :class="model.startable
          ? 'border border-emerald-200 bg-emerald-50 text-emerald-700'
          : 'border border-amber-200 bg-amber-50 text-amber-700'"
      >
        {{ model.eligibilityLabel }}
      </span>
      <span
        class="rounded-full px-2.5 py-1"
        :class="model.notifyOnly
          ? 'border border-sky-200 bg-sky-50 text-sky-700'
          : 'border border-slate-200 bg-slate-100 text-slate-600'"
      >
        {{ model.executionModeLabel }}
      </span>
    </div>
  </button>
</template>

<style scoped>
.strategy-instance-card {
  display: block;
  width: 100%;
  border: 1px solid var(--tv-border);
  border-radius: 8px;
  padding: 0.9rem 1rem;
  background: color-mix(in srgb, var(--tv-bg-surface) 90%, transparent);
  cursor: pointer;
  text-align: left;
  transition: border-color 140ms ease, background-color 140ms ease;
}

.strategy-instance-card:hover {
  border-color: color-mix(in srgb, var(--tv-accent) 55%, var(--tv-border));
}

.strategy-instance-card:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.strategy-instance-card.is-active {
  border-color: var(--card-active-border);
  background: color-mix(in srgb, var(--card-active-surface) 18%, transparent);
}

.strategy-instance-card--running {
  border-color: color-mix(in srgb, rgb(52 211 153) 58%, var(--tv-border));
  background: color-mix(in srgb, rgb(236 253 245) 90%, var(--tv-bg-surface));
}

.strategy-instance-card--paused {
  border-color: color-mix(in srgb, rgb(251 191 36) 55%, var(--tv-border));
  background: color-mix(in srgb, rgb(255 251 235) 90%, var(--tv-bg-surface));
}

.strategy-instance-card--stopped {
  border-color: var(--tv-border);
}
</style>
