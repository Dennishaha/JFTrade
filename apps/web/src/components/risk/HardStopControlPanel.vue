<script setup lang="ts">
import { reactive } from "vue";

import type { RealTradeHardStopsResponse } from "@/contracts";
import {
  formatTradingEnvironment,
  resolveRealTradeHardStopScopeTagType,
} from "@/composables/consoleDataFormatting";
import {
  formatInstrumentIdentityText,
  formatUserMarketLabel,
} from "@/composables/instrumentPresentation";

defineProps<{
  entries: RealTradeHardStopsResponse["entries"];
  loadingAction: string;
}>();

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
  <v-card flat class="card-shell border-0">
    <div class="flex items-center justify-between gap-3 px-4 pt-4">
      <div>
        <div class="text-xl font-semibold text-slate-900">硬停止</div>
        <div class="mt-1 text-sm text-slate-500">按账户、市场或标的阻断实盘下单。</div>
      </div>
      <v-chip :color="entries.length ? 'error' : undefined" variant="outlined" size="small">
        {{ entries.length ? `${entries.length} 条生效` : "无硬停止" }}
      </v-chip>
    </div>

    <v-card-text>
      <div class="mb-3 grid gap-2 rounded-lg border border-slate-200 bg-white px-3 py-3 text-sm">
        <div class="grid gap-2 sm:grid-cols-2">
          <input
            v-model="form.accountId"
            class="rounded-lg border border-slate-300 bg-white px-3 py-2 text-slate-900 outline-none placeholder:text-slate-500"
            placeholder="账户 ID，空为全部"
            aria-label="硬停止账户 ID"
          />
          <select
            v-model="form.hardStopScope"
            class="rounded-lg border border-slate-300 bg-white px-3 py-2 text-slate-900 outline-none"
            aria-label="硬停止范围"
          >
            <option value="ACCOUNT">账户</option>
            <option value="MARKET">市场</option>
            <option value="SYMBOL">标的</option>
          </select>
          <input
            v-model="form.market"
            class="rounded-lg border border-slate-300 bg-white px-3 py-2 text-slate-900 uppercase outline-none placeholder:text-slate-500"
            placeholder="市场，如 US"
            aria-label="硬停止市场"
          />
          <input
            v-model="form.symbol"
            class="rounded-lg border border-slate-300 bg-white px-3 py-2 text-slate-900 uppercase outline-none placeholder:text-slate-500"
            placeholder="标的，如 AAPL"
            aria-label="硬停止标的"
          />
        </div>
        <input
          v-model="form.reason"
          class="rounded-lg border border-slate-300 bg-white px-3 py-2 text-slate-900 outline-none placeholder:text-slate-500"
          placeholder="原因"
          aria-label="硬停止原因"
        />
        <div>
          <v-btn
            color="error"
            size="small"
            variant="outlined"
            :loading="loadingAction === 'hard-stop.activate'"
            @click="activate"
          >
            创建硬停止
          </v-btn>
        </div>
      </div>

      <div v-if="entries.length" class="grid gap-2">
        <div
          v-for="item in entries.slice(0, 5)"
          :key="item.id"
          class="rounded-lg bg-slate-50 px-3 py-3"
        >
          <div class="flex items-center justify-between gap-3">
            <div class="font-medium text-slate-900">{{ item.brokerId }} / {{ item.accountId }}</div>
            <v-chip
              :color="resolveRealTradeHardStopScopeTagType(item) === 'danger' ? 'error' : resolveRealTradeHardStopScopeTagType(item)"
              variant="outlined"
              size="small"
            >
              {{ formatUserHardStopScope(item) }}
            </v-chip>
          </div>
          <div class="mt-1 text-xs text-slate-500">
            {{ formatTradingEnvironment(item.tradingEnvironment) }} / {{ formatUserMarketLabel(item.market ?? "") }} / 操作员 {{ item.operatorId }}
          </div>
          <div class="mt-1 text-xs text-slate-700">{{ item.reason || "未填写原因" }}</div>
          <div class="mt-2">
            <v-btn
              size="small"
              variant="outlined"
              :loading="loadingAction === `hard-stop.release.${item.id}`"
              @click="emit('release', item.id)"
            >
              解除硬停止
            </v-btn>
          </div>
        </div>
      </div>
      <div v-else class="text-sm text-slate-500">暂无活跃实盘硬停止。</div>
    </v-card-text>
  </v-card>
</template>
