<script setup lang="ts">
import { reactive, ref, watch } from "vue";

import type {
  InstrumentResolutionCandidate,
  RealTradeHardStopsResponse,
} from "@/contracts";
import InstrumentSearchBox from "@/components/domain/market-data/InstrumentSearchBox.vue";
import {
  formatTradingEnvironment,
  resolveRealTradeHardStopScopeTagType,
} from "@/composables/consoleDataFormatting";
import {
  formatInstrumentIdentityText,
  formatUserMarketLabel,
} from "@/composables/instrumentPresentation";

const props = withDefaults(
  defineProps<{
    entries: RealTradeHardStopsResponse["entries"];
    loadingAction: string;
    prefill?: {
      brokerId: string;
      accountId: string;
      tradingEnvironment: string;
    } | null;
  }>(),
  { prefill: null },
);

const emit = defineEmits<{
  activate: [payload: {
    brokerId: string;
    tradingEnvironment: string;
    accountId: string;
    market: string;
    symbol: string;
    hardStopScope: string;
    operatorId: string;
    reason: string;
  }];
  release: [id: string];
}>();

const form = reactive({
  brokerId: "futu",
  tradingEnvironment: "REAL",
  accountId: "",
  market: "",
  symbol: "",
  hardStopScope: "ACCOUNT",
  operatorId: "local",
  reason: "",
});

const symbolQuery = ref("");

function handleSymbolSelect(candidate: InstrumentResolutionCandidate): void {
  form.market = candidate.market;
  form.symbol = candidate.code;
  form.hardStopScope = "SYMBOL";
  symbolQuery.value = candidate.instrumentId;
}

watch(
  () => props.prefill,
  (prefill) => {
    if (prefill == null) return;
    form.brokerId = prefill.brokerId;
    form.tradingEnvironment = prefill.tradingEnvironment;
    if (form.accountId.trim() === "") {
      form.accountId = prefill.accountId;
    }
  },
  { immediate: true },
);

function formatUserHardStopScope(
  item: RealTradeHardStopsResponse["entries"][number],
): string {
  const scope =
    item.symbol != null ? "SYMBOL" : item.market != null ? "MARKET" : "ACCOUNT";
  if (scope === "SYMBOL") {
    return `标的 / ${formatUserMarketLabel(item.market)} / ${formatInstrumentIdentityText({
      market: item.market,
      instrumentId: item.symbol,
    })}`;
  }
  if (scope === "MARKET") {
    return `市场 / ${formatUserMarketLabel(item.market)}`;
  }
  return "账户";
}

function scopeClass(
  item: RealTradeHardStopsResponse["entries"][number],
): string {
  const tag = resolveRealTradeHardStopScopeTagType(item);
  return tag === "danger" ? "tv-status--error" : `tv-status--${tag}`;
}

function activate() {
  emit("activate", {
    ...form,
    market: form.market.trim().toUpperCase(),
    symbol: form.symbol.trim().toUpperCase(),
    accountId: form.accountId.trim(),
    reason: form.reason.trim(),
  });
}
</script>

<template>
  <section class="hardstop-panel" aria-label="硬停止">
    <header class="hardstop-panel__head">
      <span class="hardstop-panel__title">硬停止</span>
      <span
        class="hardstop-panel__state"
        :class="entries.length ? 'tv-status--error' : 'tv-status--success'"
      >
        <i class="tv-state-dot"></i>
        {{ entries.length ? `${entries.length} 条生效` : "无硬停止" }}
      </span>
    </header>

    <div class="hardstop-panel__body">
      <div class="hardstop-panel__form">
        <div class="hardstop-panel__form-grid">
          <input
            v-model="form.accountId"
            class="tv-input"
            placeholder="账户 ID，空为全部"
            aria-label="硬停止账户 ID"
          />
          <select
            v-model="form.hardStopScope"
            class="tv-select"
            aria-label="硬停止范围"
          >
            <option value="ACCOUNT">账户</option>
            <option value="MARKET">市场</option>
            <option value="SYMBOL">标的</option>
          </select>
          <input
            v-model="form.market"
            class="tv-input hardstop-panel__upper"
            placeholder="市场，如 US"
            aria-label="硬停止市场"
          />
          <InstrumentSearchBox
            v-model="symbolQuery"
            placeholder="搜索标的代码或名称"
            action-label="选择"
            variant="backtest"
            root-test-id="hardstop-symbol-search"
            input-test-id="hardstop-symbol-input"
            @select="handleSymbolSelect"
          />
        </div>
        <input
          v-model="form.reason"
          class="tv-input"
          placeholder="原因"
          aria-label="硬停止原因"
        />
        <div>
          <button
            type="button"
            class="tv-btn tv-btn-ghost hardstop-panel__create"
            :disabled="loadingAction === 'hard-stop.activate'"
            @click="activate"
          >
            {{ loadingAction === 'hard-stop.activate' ? "创建中..." : "创建硬停止" }}
          </button>
        </div>
      </div>

      <div v-if="entries.length" class="hardstop-panel__entries">
        <div
          v-for="item in entries.slice(0, 5)"
          :key="item.id"
          class="hardstop-panel__entry"
        >
          <div class="hardstop-panel__entry-head">
            <b>{{ item.brokerId }} / {{ item.accountId }}</b>
            <span
              class="hardstop-panel__scope tv-status-surface"
              :class="scopeClass(item)"
            >
              {{ formatUserHardStopScope(item) }}
            </span>
          </div>
          <div class="hardstop-panel__entry-meta">
            {{ formatTradingEnvironment(item.tradingEnvironment) }} / {{ formatUserMarketLabel(item.market ?? "") }} / 操作员 {{ item.operatorId }}
          </div>
          <div class="hardstop-panel__entry-reason">{{ item.reason || "未填写原因" }}</div>
          <div class="hardstop-panel__entry-actions">
            <button
              type="button"
              class="tv-btn tv-btn-ghost"
              :disabled="loadingAction === `hard-stop.release.${item.id}`"
              @click="emit('release', item.id)"
            >
              {{ loadingAction === `hard-stop.release.${item.id}` ? "解除中..." : "解除硬停止" }}
            </button>
          </div>
        </div>
      </div>
      <div v-else class="hardstop-panel__empty">暂无活跃实盘硬停止。</div>
    </div>
  </section>
</template>

<style scoped>
.hardstop-panel {
  display: flex;
  min-width: 0;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 8px;
  background: var(--tv-bg-surface);
}

.hardstop-panel__head {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 9px 12px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.hardstop-panel__title {
  color: var(--tv-text-muted);
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.hardstop-panel__state {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--tv-status-fg, var(--tv-text-muted));
  font-size: 10px;
}

.hardstop-panel__body {
  display: grid;
  gap: 10px;
  padding: 12px;
}

.hardstop-panel__form {
  display: grid;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface-2);
}

.hardstop-panel__form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.hardstop-panel__upper {
  text-transform: uppercase;
}

.hardstop-panel__create:not(:disabled) {
  border-color: var(--tv-status-error-border);
  color: var(--tv-status-error-fg);
}

.hardstop-panel__create:disabled,
.hardstop-panel__entry-actions .tv-btn:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.hardstop-panel__create,
.hardstop-panel__entry-actions .tv-btn {
  height: 28px;
  font-size: 12px;
}

.hardstop-panel__entries {
  display: grid;
  gap: 8px;
}

.hardstop-panel__entry {
  display: grid;
  gap: 4px;
  padding: 9px 11px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface-2);
}

.hardstop-panel__entry-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.hardstop-panel__entry-head b {
  color: var(--tv-text);
  font-size: 12px;
  font-weight: 600;
}

.hardstop-panel__scope {
  padding: 2px 8px;
  border: 1px solid;
  border-radius: 999px;
  font-size: 10px;
  white-space: nowrap;
}

.hardstop-panel__entry-meta {
  color: var(--tv-text-dim);
  font-size: 10px;
}

.hardstop-panel__entry-reason {
  color: var(--tv-text-muted);
  font-size: 11px;
}

.hardstop-panel__empty {
  padding: 8px 2px;
  color: var(--tv-text-dim);
  font-size: 11px;
}
</style>
