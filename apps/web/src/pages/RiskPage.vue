<script setup lang="ts">
import { computed, ref } from "vue";

import PageHeader from "../components/PageHeader.vue";
import {
  formatRealTradeHardStopScope,
  formatRealTradeKillSwitchSource,
  formatRealTradeRiskSource,
  resolveRealTradeApprovalDecisionTagType,
  resolveRealTradeHardStopScopeTagType,
  resolveRealTradeKillSwitchEventTagType,
  resolveRealTradeRiskEventTagType,
} from "../composables/consoleDataFormatting";
import { useConsoleData } from "../composables/useConsoleData";

const {
  isLoading,
  loadSystemState,
  realTradeApprovals,
  realTradeHardStopEvents,
  realTradeHardStops,
  realTradeKillSwitchEvents,
  realTradeKillSwitchState,
  realTradeRiskEvents,
  realTradeRiskState,
  systemStatus,
} = useConsoleData();

const riskActiveTab = ref("Limits");

const riskHeaderStats = computed(() => [
  {
    label: "Real Trading",
    value: systemStatus.value.realTradingEnabled ? "ENABLED" : "GATED",
    tone: systemStatus.value.realTradingEnabled ? "danger" : "good",
  },
  {
    label: "Kill Switch",
    value: realTradeKillSwitchState.value.killSwitchActive ? "ACTIVE" : "CLEAR",
    tone: realTradeKillSwitchState.value.killSwitchActive ? "danger" : "good",
  },
  {
    label: "Risk Limit",
    value: realTradeRiskState.value.riskEnabled ? "ENFORCED" : "OFF",
    tone: realTradeRiskState.value.riskEnabled ? "warn" : "good",
  },
  {
    label: "Hard Stops",
    value: realTradeHardStops.value.entries.length,
    tone: realTradeHardStops.value.entries.length ? "warn" : "good",
  },
]);
</script>

<template>
  <div class="grid gap-6">
    <PageHeader
      eyebrow="Control plane"
      title="Risk / Guardrails"
      description="把 REAL risk limit、kill switch、hard stop 和审批事件聚成一个风控平面，便于先判断门禁，再查看事件时间线。"
      :stats="riskHeaderStats"
    />

    <!-- Tabs: Limits, Kill Switch, Hard Stops, Events -->
    <v-tabs v-model="riskActiveTab" bg-color="transparent" class="tv-page-tabs">
      <v-tab value="Limits">Limits</v-tab>
      <v-tab value="Kill Switch">Kill Switch</v-tab>
      <v-tab value="Hard Stops">Hard Stops</v-tab>
      <v-tab value="Events">Events</v-tab>
    </v-tabs>

    <v-window v-model="riskActiveTab">
      <!-- Limits Tab -->
      <v-window-item value="Limits">
        <v-card flat class="card-shell border-0">
        <div class="px-4 pt-4 flex items-center justify-between gap-3">
          <div class="text-xl font-semibold text-slate-900">Risk Limits</div>
          <div class="flex items-center gap-2">
            <v-chip :color="realTradeRiskState.realTradingEnabled ? 'error' : 'default'" variant="outlined" size="small">
              {{ realTradeRiskState.realTradingEnabled ? 'REAL' : 'GATED' }}
            </v-chip>
            <v-chip :color="realTradeRiskState.riskEnabled ? 'warning' : 'success'" variant="outlined" size="small">
              {{ realTradeRiskState.riskEnabled ? 'RISK ON' : 'RISK OFF' }}
            </v-chip>
          </div>
        </div>

        <v-card-text>
        <div class="grid gap-4 sm:grid-cols-2">
          <div class="rounded-3xl bg-slate-50 px-4 py-4">
            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Max Order Qty</div>
            <div class="mt-2 text-2xl font-semibold text-slate-900">
              {{ realTradeRiskState.effectiveMaxOrderQuantity ?? systemStatus.realTradingRisk.maxOrderQuantity ?? 'N/A' }}
            </div>
          </div>
          <div class="rounded-3xl bg-slate-50 px-4 py-4">
            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Max Order Notional</div>
            <div class="mt-2 text-2xl font-semibold text-slate-900">
              {{ realTradeRiskState.effectiveMaxOrderNotional ?? systemStatus.realTradingRisk.maxOrderNotional ?? 'N/A' }}
            </div>
          </div>
          <div class="rounded-3xl bg-slate-50 px-4 py-4">
            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Config Source</div>
            <div class="mt-2 text-lg font-semibold text-slate-900">
              {{ formatRealTradeRiskSource(realTradeRiskState.riskConfigSource) }}
            </div>
          </div>
          <div class="rounded-3xl bg-slate-50 px-4 py-4">
            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Control Plane</div>
            <div class="mt-2 text-lg font-semibold" :class="realTradeRiskState.controlPlaneActive ? 'text-amber-700' : 'text-slate-900'">
              {{ realTradeRiskState.controlPlaneActive ? 'ACTIVE' : 'INACTIVE' }}
            </div>
          </div>
        </div>

        <div v-if="realTradeRiskState.entry" class="mt-4 rounded-3xl border border-slate-200 bg-white px-4 py-4 text-sm">
          <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Active Risk Config Entry</div>
          <div class="mt-2 text-slate-900 font-medium">{{ realTradeRiskState.entry.tradingEnvironment }}</div>
          <div class="mt-1 text-xs text-slate-500">
            qty {{ realTradeRiskState.entry.maxOrderQuantity ?? 'N/A' }} / notional {{ realTradeRiskState.entry.maxOrderNotional ?? 'N/A' }}
          </div>
          <div class="mt-1 text-xs text-slate-500">
            operator {{ realTradeRiskState.entry.operatorId }} / {{ realTradeRiskState.entry.reason }}
          </div>
          <div class="mt-1 text-xs text-slate-500">activated {{ realTradeRiskState.entry.activatedAt }}</div>
        </div>
        <v-empty-state v-else :text="realTradeRiskState.riskEnabled ? 'Risk limits loaded from ENV — no control-plane entry.' : '无有效 REAL 风控限额。'" class="mt-4" />

        <div class="mt-3 flex justify-end">
          <v-btn :loading="isLoading" variant="text" color="primary" @click="loadSystemState()">刷新</v-btn>
        </div>
        </v-card-text>
      </v-card>
      </v-window-item>

      <!-- Kill Switch Tab -->
      <v-window-item value="Kill Switch">
        <v-card flat class="card-shell border-0">
        <div class="px-4 pt-4 flex items-center justify-between gap-3">
          <div class="text-xl font-semibold text-slate-900">Kill Switch</div>
          <v-chip
            :color="realTradeKillSwitchState.killSwitchActive ? 'error' : 'success'"
            variant="outlined"
            size="small"
          >
            {{ realTradeKillSwitchState.killSwitchActive ? 'ACTIVE' : 'INACTIVE' }}
          </v-chip>
        </div>

        <v-card-text>
        <div class="grid gap-4 sm:grid-cols-2">
          <div class="rounded-3xl bg-slate-50 px-4 py-4">
            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Source</div>
            <div class="mt-2 text-lg font-semibold" :class="realTradeKillSwitchState.killSwitchActive ? 'text-amber-700' : 'text-slate-900'">
              {{ formatRealTradeKillSwitchSource(realTradeKillSwitchState.killSwitchSource) }}
            </div>
          </div>
          <div class="rounded-3xl bg-slate-50 px-4 py-4">
            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Cancel Allowed</div>
            <div class="mt-2 text-lg font-semibold" :class="realTradeKillSwitchState.allowsCancel ? 'text-teal-700' : 'text-red-600'">
              {{ realTradeKillSwitchState.allowsCancel ? 'YES' : 'NO' }}
            </div>
          </div>
        </div>

        <div v-if="realTradeKillSwitchState.blockedOperations.length" class="mt-4 rounded-3xl border border-amber-200 bg-amber-50 px-4 py-4">
          <div class="text-xs uppercase tracking-[0.2em] text-amber-700">Blocked Operations</div>
          <div class="mt-2 flex flex-wrap gap-2">
            <v-chip
              v-for="op in realTradeKillSwitchState.blockedOperations"
              :key="op"
              color="warning"
              variant="outlined"
              size="small"
            >
              {{ op }}
            </v-chip>
          </div>
        </div>
        <v-empty-state v-else text="Kill switch inactive — no blocked operations." class="mt-4" />

        <div v-if="realTradeKillSwitchState.entry" class="mt-3 rounded-3xl border border-slate-200 bg-white px-4 py-4 text-sm">
          <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Active Entry</div>
          <div class="mt-2 text-slate-900 font-medium">{{ realTradeKillSwitchState.entry.tradingEnvironment }}</div>
          <div class="mt-1 text-xs text-slate-500">
            operator {{ realTradeKillSwitchState.entry.operatorId }} / {{ realTradeKillSwitchState.entry.reason }}
          </div>
          <div class="mt-1 text-xs text-slate-500">activated {{ realTradeKillSwitchState.entry.activatedAt }}</div>
        </div>

        <div class="mt-3 rounded-3xl bg-slate-50 px-4 py-3 text-xs text-slate-500">
          ENV {{ realTradeKillSwitchState.envConfiguredActive ? 'active' : 'inactive' }} /
          Control-Plane {{ realTradeKillSwitchState.controlPlaneActive ? 'active' : 'inactive' }}
        </div>
        </v-card-text>
      </v-card>
      </v-window-item>

      <!-- Hard Stops Tab -->
      <v-window-item value="Hard Stops">
        <v-card flat class="card-shell border-0">
        <div class="px-4 pt-4 flex items-center justify-between gap-3">
          <div class="text-xl font-semibold text-slate-900">Hard Stops</div>
          <v-chip :color="realTradeHardStops.entries.length ? 'error' : 'success'" variant="outlined" size="small">
            {{ realTradeHardStops.entries.length ? `${realTradeHardStops.entries.length} ACTIVE` : 'NONE' }}
          </v-chip>
        </div>

        <v-card-text>
        <div v-if="realTradeHardStops.entries.length">
          <div class="mb-3 flex flex-wrap gap-2">
            <v-chip
              v-for="op in realTradeHardStops.blockedOperations"
              :key="op"
              color="warning"
              variant="outlined"
              size="small"
            >
              {{ op }} BLOCKED
            </v-chip>
            <v-chip :color="realTradeHardStops.allowsCancel ? 'success' : 'error'" variant="outlined" size="small">
              CANCEL {{ realTradeHardStops.allowsCancel ? 'ALLOWED' : 'BLOCKED' }}
            </v-chip>
          </div>
          <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            <div
              v-for="entry in realTradeHardStops.entries"
              :key="entry.id"
              class="rounded-3xl border border-amber-200 bg-amber-50 px-4 py-4"
            >
              <div class="flex items-center justify-between gap-2">
                <div class="font-medium text-slate-900">{{ entry.brokerId }}</div>
                <v-chip :color="resolveRealTradeHardStopScopeTagType(entry)" variant="outlined" size="small">
                  {{ formatRealTradeHardStopScope(entry) }}
                </v-chip>
              </div>
              <div class="mt-1 text-xs text-slate-500">{{ entry.tradingEnvironment }} / {{ entry.accountId }}</div>
              <div class="mt-1 text-xs text-amber-700">{{ entry.reason }}</div>
              <div class="mt-1 text-xs text-slate-500">activated {{ entry.activatedAt }}</div>
            </div>
          </div>
        </div>
        <v-empty-state v-else text="无活跃 REAL hard stop。" />
        </v-card-text>
      </v-card>
      </v-window-item>

      <!-- Events Tab -->
      <v-window-item value="Events">
        <div class="grid gap-5 lg:grid-cols-[1fr_1fr]">
          <!-- Real Trade Approvals -->
          <v-card flat class="card-shell border-0">
        <div class="px-4 pt-4 flex items-center justify-between gap-3">
          <div class="text-xl font-semibold text-slate-900">Real Trade Approvals</div>
          <v-chip :color="realTradeApprovals.realTradingEnabled ? 'error' : 'default'" variant="outlined" size="small">
            {{ realTradeApprovals.realTradingEnabled ? 'REAL ENABLED' : 'REAL GATED' }}
          </v-chip>
        </div>

        <v-card-text>
        <div v-if="realTradeApprovals.entries.length" class="grid gap-3">
          <div
            v-for="entry in realTradeApprovals.entries.slice(0, 5)"
            :key="entry.id"
            class="rounded-3xl bg-slate-50 px-4 py-3"
          >
            <div class="flex items-center justify-between gap-2">
              <div class="font-medium text-slate-900">{{ entry.operation }} / {{ entry.brokerId }}</div>
              <v-chip :color="resolveRealTradeApprovalDecisionTagType(entry.decision)" variant="outlined" size="small">
                {{ entry.decision.toUpperCase() }}
              </v-chip>
            </div>
            <div class="mt-1 text-xs text-slate-500">
              {{ entry.tradingEnvironment ?? 'N/A' }} / {{ entry.accountId ?? 'N/A' }} / {{ entry.market ?? 'N/A' }}
            </div>
            <div v-if="entry.reason" class="mt-1 text-xs text-amber-700">{{ entry.reason }}</div>
            <div class="mt-1 text-xs text-slate-500">{{ entry.createdAt }}</div>
          </div>
          <div class="text-xs text-slate-500">
            confirmation text: {{ realTradeApprovals.requiredConfirmationText }} / window {{ realTradeApprovals.maxApprovalAgeMs }}ms
          </div>
        </div>
        <v-empty-state v-else :text="`暂无审批记录。confirmation: ${realTradeApprovals.requiredConfirmationText} / window ${realTradeApprovals.maxApprovalAgeMs}ms`" />
        </v-card-text>
      </v-card>

      <!-- Kill Switch + Risk + Hard Stop Event Log -->
      <v-card flat class="card-shell border-0">
        <div class="px-4 pt-4">
          <div class="text-xl font-semibold text-slate-900">Risk Event Log</div>
        </div>

        <v-card-text>
        <div class="grid gap-4">
          <!-- Kill Switch Events -->
          <div>
            <div class="mb-2 text-xs uppercase tracking-[0.2em] text-slate-500">Kill Switch Events</div>
            <div v-if="realTradeKillSwitchEvents.entries.length" class="grid gap-2">
              <div
                v-for="entry in realTradeKillSwitchEvents.entries.slice(0, 3)"
                :key="entry.id"
                class="rounded-2xl bg-slate-50 px-3 py-3"
              >
                <div class="flex items-center justify-between gap-2">
                  <div class="text-sm font-medium text-slate-900">{{ entry.action }}</div>
                  <v-chip :color="resolveRealTradeKillSwitchEventTagType(entry.eventType)" variant="outlined" size="small">
                    {{ entry.eventType.toUpperCase() }}
                  </v-chip>
                </div>
                <div class="mt-1 text-xs text-slate-500">
                  {{ entry.brokerId }} / {{ entry.tradingEnvironment ?? 'N/A' }}
                </div>
                <div v-if="entry.reason" class="mt-1 text-xs text-amber-700">{{ entry.reason }}</div>
                <div class="mt-1 text-xs text-slate-500">{{ entry.createdAt }}</div>
              </div>
            </div>
            <div v-else class="text-sm text-slate-500">暂无 kill switch 事件。</div>
          </div>

          <!-- Risk Events -->
          <div>
            <div class="mb-2 text-xs uppercase tracking-[0.2em] text-slate-500">Risk Events</div>
            <div v-if="realTradeRiskEvents.entries.length" class="grid gap-2">
              <div
                v-for="entry in realTradeRiskEvents.entries.slice(0, 3)"
                :key="entry.id"
                class="rounded-2xl bg-slate-50 px-3 py-3"
              >
                <div class="flex items-center justify-between gap-2">
                  <div class="text-sm font-medium text-slate-900">{{ entry.action }}</div>
                  <v-chip :color="resolveRealTradeRiskEventTagType(entry.eventType)" variant="outlined" size="small">
                    {{ entry.eventType.toUpperCase() }}
                  </v-chip>
                </div>
                <div class="mt-1 text-xs text-slate-500">
                  {{ entry.brokerId }} / {{ entry.tradingEnvironment ?? 'N/A' }}
                  <span v-if="entry.quantity != null"> / qty {{ entry.quantity }}</span>
                </div>
                <div v-if="entry.reason" class="mt-1 text-xs text-amber-700">{{ entry.reason }}</div>
                <div class="mt-1 text-xs text-slate-500">{{ entry.createdAt }}</div>
              </div>
            </div>
            <div v-else class="text-sm text-slate-500">暂无 risk 事件。</div>
          </div>

          <!-- Hard Stop Events -->
          <div>
            <div class="mb-2 text-xs uppercase tracking-[0.2em] text-slate-500">Hard Stop Events</div>
            <div v-if="realTradeHardStopEvents.entries.length" class="grid gap-2">
              <div
                v-for="entry in realTradeHardStopEvents.entries.slice(0, 3)"
                :key="entry.id"
                class="rounded-2xl bg-slate-50 px-3 py-3"
              >
                <div class="flex items-center justify-between gap-2">
                  <div class="text-sm font-medium text-slate-900">{{ entry.action }}</div>
                  <v-chip :color="resolveRealTradeHardStopScopeTagType(entry)" variant="outlined" size="small">
                    {{ entry.eventType.toUpperCase() }}
                  </v-chip>
                </div>
                <div class="mt-1 text-xs text-slate-500">
                  {{ entry.brokerId }} / {{ entry.tradingEnvironment ?? 'N/A' }}
                  <span v-if="entry.market"> / {{ entry.market }}</span>
                  <span v-if="entry.symbol"> / {{ entry.symbol }}</span>
                </div>
                <div v-if="entry.reason" class="mt-1 text-xs text-amber-700">{{ entry.reason }}</div>
                <div class="mt-1 text-xs text-slate-500">{{ entry.createdAt }}</div>
              </div>
            </div>
            <div v-else class="text-sm text-slate-500">暂无 hard stop 事件。</div>
          </div>
        </div>
        </v-card-text>
      </v-card>
        </div>
      </v-window-item>
    </v-window>
  </div>
</template>
