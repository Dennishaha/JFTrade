<script setup lang="ts">
import type {
  RealTradeKillSwitchEventsResponse,
  RealTradeRiskEventsResponse,
} from "@/contracts";
import {
  formatDateTime,
  formatRealTradeEventTypeLabel,
  resolveRealTradeKillSwitchEventTagType,
  resolveRealTradeRiskEventTagType,
} from "@/composables/consoleDataFormatting";

defineProps<{
  killSwitchEvents: RealTradeKillSwitchEventsResponse["entries"];
  riskEvents: RealTradeRiskEventsResponse["entries"];
}>();

function chipColor(tag: "success" | "warning" | "danger" | "info") {
  return tag === "danger" ? "error" : tag;
}
</script>

<template>
  <v-card flat class="card-shell border-0">
    <div class="px-4 pt-4">
      <div class="text-xl font-semibold text-slate-900">最近风控事件</div>
      <div class="mt-1 text-sm text-slate-500">运行时配置、熔断和拒单事件会出现在这里。</div>
    </div>

    <v-card-text>
      <div class="grid gap-3 lg:grid-cols-2">
        <div>
          <div class="mb-2 text-xs font-medium uppercase text-slate-500">配置与拒单</div>
          <div v-if="riskEvents.length" class="grid gap-2">
            <div
              v-for="item in riskEvents.slice(0, 5)"
              :key="item.id"
              class="rounded-lg bg-slate-50 px-3 py-3"
            >
              <div class="flex items-center justify-between gap-3">
                <div class="font-medium text-slate-900">{{ item.action || formatRealTradeEventTypeLabel(item.eventType) }}</div>
                <v-chip
                  :color="chipColor(resolveRealTradeRiskEventTagType(item.eventType))"
                  variant="outlined"
                  size="small"
                >
                  {{ formatRealTradeEventTypeLabel(item.eventType) }}
                </v-chip>
              </div>
              <div class="mt-1 text-xs text-slate-500">
                {{ formatDateTime(item.createdAt) }} / {{ item.operatorId ?? "system" }}
              </div>
              <div class="mt-1 text-xs text-slate-700">{{ item.reason || item.errorCode || "暂无原因" }}</div>
            </div>
          </div>
          <div v-else class="text-sm text-slate-500">暂无运行时风控事件。</div>
        </div>

        <div>
          <div class="mb-2 text-xs font-medium uppercase text-slate-500">熔断</div>
          <div v-if="killSwitchEvents.length" class="grid gap-2">
            <div
              v-for="item in killSwitchEvents.slice(0, 5)"
              :key="item.id"
              class="rounded-lg bg-slate-50 px-3 py-3"
            >
              <div class="flex items-center justify-between gap-3">
                <div class="font-medium text-slate-900">{{ item.action || formatRealTradeEventTypeLabel(item.eventType) }}</div>
                <v-chip
                  :color="chipColor(resolveRealTradeKillSwitchEventTagType(item.eventType))"
                  variant="outlined"
                  size="small"
                >
                  {{ formatRealTradeEventTypeLabel(item.eventType) }}
                </v-chip>
              </div>
              <div class="mt-1 text-xs text-slate-500">
                {{ formatDateTime(item.createdAt) }} / {{ item.operatorId ?? "system" }}
              </div>
              <div class="mt-1 text-xs text-slate-700">{{ item.reason || item.errorCode || "暂无原因" }}</div>
            </div>
          </div>
          <div v-else class="text-sm text-slate-500">暂无熔断事件。</div>
        </div>
      </div>
    </v-card-text>
  </v-card>
</template>
