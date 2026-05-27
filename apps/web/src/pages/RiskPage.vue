<script setup lang="ts">
import { computed, ref } from "vue";

import PageHeader from "../components/PageHeader.vue";
import {
  formatApprovalDecisionLabel,
  formatGenericStatusLabel,
  formatMarketLabel,
  formatRealTradeEventTypeLabel,
  formatRealTradeHardStopScope,
  formatRealTradeKillSwitchSource,
  formatRealTradeOperationLabel,
  formatRealTradeRiskSource,
  formatTradingEnvironment,
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
    label: "实盘交易",
    value: formatGenericStatusLabel(systemStatus.value.realTradingEnabled ? "ENABLED" : "GATED"),
    tone: systemStatus.value.realTradingEnabled ? "danger" : "good",
  },
  {
    label: "熔断开关",
    value: formatGenericStatusLabel(realTradeKillSwitchState.value.killSwitchActive ? "ACTIVE" : "CLEAR"),
    tone: realTradeKillSwitchState.value.killSwitchActive ? "danger" : "good",
  },
  {
    label: "风控限额",
    value: formatGenericStatusLabel(realTradeRiskState.value.riskEnabled ? "ENFORCED" : "OFF"),
    tone: realTradeRiskState.value.riskEnabled ? "warn" : "good",
  },
  {
    label: "硬停止",
    value: realTradeHardStops.value.entries.length,
    tone: realTradeHardStops.value.entries.length ? "warn" : "good",
  },
]);
</script>

<template>
  <div class="grid gap-6">
    <PageHeader
      eyebrow="控制面"
      title="风控 / 门禁"
      description="把实盘风控限额、熔断开关、硬停止和审批事件聚成一个风控平面，便于先判断门禁，再查看事件时间线。"
      :stats="riskHeaderStats"
    />

    <!-- Tabs: Limits, Kill Switch, Hard Stops, Events -->
    <v-tabs v-model="riskActiveTab" bg-color="transparent" class="tv-page-tabs">
      <v-tab value="Limits">限额</v-tab>
      <v-tab value="Kill Switch">熔断</v-tab>
      <v-tab value="Hard Stops">硬停止</v-tab>
      <v-tab value="Events">事件</v-tab>
    </v-tabs>

    <v-window v-model="riskActiveTab">
      <!-- Limits Tab -->
      <v-window-item value="Limits">
        <v-card flat class="card-shell border-0">
        <div class="px-4 pt-4 flex items-center justify-between gap-3">
          <div class="text-xl font-semibold text-slate-900">风控限额</div>
          <div class="flex items-center gap-2">
            <v-chip :color="realTradeRiskState.realTradingEnabled ? 'error' : 'default'" variant="outlined" size="small">
              {{ realTradeRiskState.realTradingEnabled ? '实盘已开放' : '实盘受限' }}
            </v-chip>
            <v-chip :color="realTradeRiskState.riskEnabled ? 'warning' : 'success'" variant="outlined" size="small">
              {{ realTradeRiskState.riskEnabled ? '风控开启' : '风控关闭' }}
            </v-chip>
          </div>
        </div>

        <v-card-text>
        <div class="grid gap-4 sm:grid-cols-2">
          <div class="rounded-3xl bg-slate-50 px-4 py-4">
            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">最大下单数量</div>
            <div class="mt-2 text-2xl font-semibold text-slate-900">
              {{ realTradeRiskState.effectiveMaxOrderQuantity ?? systemStatus.realTradingRisk.maxOrderQuantity ?? '暂无' }}
            </div>
          </div>
          <div class="rounded-3xl bg-slate-50 px-4 py-4">
            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">最大下单金额</div>
            <div class="mt-2 text-2xl font-semibold text-slate-900">
              {{ realTradeRiskState.effectiveMaxOrderNotional ?? systemStatus.realTradingRisk.maxOrderNotional ?? '暂无' }}
            </div>
          </div>
          <div class="rounded-3xl bg-slate-50 px-4 py-4">
            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">配置来源</div>
            <div class="mt-2 text-lg font-semibold text-slate-900">
              {{ formatRealTradeRiskSource(realTradeRiskState.riskConfigSource) }}
            </div>
          </div>
          <div class="rounded-3xl bg-slate-50 px-4 py-4">
            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">控制面</div>
            <div class="mt-2 text-lg font-semibold" :class="realTradeRiskState.controlPlaneActive ? 'text-amber-700' : 'text-slate-900'">
              {{ formatGenericStatusLabel(realTradeRiskState.controlPlaneActive ? 'ACTIVE' : 'INACTIVE') }}
            </div>
          </div>
        </div>

        <div v-if="realTradeRiskState.entry" class="mt-4 rounded-3xl border border-slate-200 bg-white px-4 py-4 text-sm">
          <div class="text-xs uppercase tracking-[0.2em] text-slate-500">当前风控配置</div>
          <div class="mt-2 text-slate-900 font-medium">{{ formatTradingEnvironment(realTradeRiskState.entry.tradingEnvironment) }}</div>
          <div class="mt-1 text-xs text-slate-500">
            数量 {{ realTradeRiskState.entry.maxOrderQuantity ?? '暂无' }} / 金额 {{ realTradeRiskState.entry.maxOrderNotional ?? '暂无' }}
          </div>
          <div class="mt-1 text-xs text-slate-500">
            操作员 {{ realTradeRiskState.entry.operatorId }} / {{ realTradeRiskState.entry.reason }}
          </div>
          <div class="mt-1 text-xs text-slate-500">激活时间 {{ realTradeRiskState.entry.activatedAt }}</div>
        </div>
        <v-empty-state v-else :text="realTradeRiskState.riskEnabled ? '风控限额来自环境变量，当前没有控制面条目。' : '无有效实盘风控限额。'" class="mt-4" />

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
          <div class="text-xl font-semibold text-slate-900">熔断开关</div>
          <v-chip
            :color="realTradeKillSwitchState.killSwitchActive ? 'error' : 'success'"
            variant="outlined"
            size="small"
          >
            {{ formatGenericStatusLabel(realTradeKillSwitchState.killSwitchActive ? 'ACTIVE' : 'INACTIVE') }}
          </v-chip>
        </div>

        <v-card-text>
        <div class="grid gap-4 sm:grid-cols-2">
          <div class="rounded-3xl bg-slate-50 px-4 py-4">
            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">来源</div>
            <div class="mt-2 text-lg font-semibold" :class="realTradeKillSwitchState.killSwitchActive ? 'text-amber-700' : 'text-slate-900'">
              {{ formatRealTradeKillSwitchSource(realTradeKillSwitchState.killSwitchSource) }}
            </div>
          </div>
          <div class="rounded-3xl bg-slate-50 px-4 py-4">
            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">允许撤单</div>
            <div class="mt-2 text-lg font-semibold" :class="realTradeKillSwitchState.allowsCancel ? 'text-teal-700' : 'text-red-600'">
              {{ formatGenericStatusLabel(realTradeKillSwitchState.allowsCancel ? 'YES' : 'NO') }}
            </div>
          </div>
        </div>

        <div v-if="realTradeKillSwitchState.blockedOperations.length" class="mt-4 rounded-3xl border border-amber-200 bg-amber-50 px-4 py-4">
          <div class="text-xs uppercase tracking-[0.2em] text-amber-700">受阻操作</div>
          <div class="mt-2 flex flex-wrap gap-2">
            <v-chip
              v-for="op in realTradeKillSwitchState.blockedOperations"
              :key="op"
              color="warning"
              variant="outlined"
              size="small"
            >
              {{ formatRealTradeOperationLabel(op) }}
            </v-chip>
          </div>
        </div>
        <v-empty-state v-else text="熔断开关未激活，当前没有受阻操作。" class="mt-4" />

        <div v-if="realTradeKillSwitchState.entry" class="mt-3 rounded-3xl border border-slate-200 bg-white px-4 py-4 text-sm">
          <div class="text-xs uppercase tracking-[0.2em] text-slate-500">当前条目</div>
          <div class="mt-2 text-slate-900 font-medium">{{ formatTradingEnvironment(realTradeKillSwitchState.entry.tradingEnvironment) }}</div>
          <div class="mt-1 text-xs text-slate-500">
            操作员 {{ realTradeKillSwitchState.entry.operatorId }} / {{ realTradeKillSwitchState.entry.reason }}
          </div>
          <div class="mt-1 text-xs text-slate-500">激活时间 {{ realTradeKillSwitchState.entry.activatedAt }}</div>
        </div>

        <div class="mt-3 rounded-3xl bg-slate-50 px-4 py-3 text-xs text-slate-500">
          环境变量 {{ formatGenericStatusLabel(realTradeKillSwitchState.envConfiguredActive ? 'ACTIVE' : 'INACTIVE') }} /
          控制面 {{ formatGenericStatusLabel(realTradeKillSwitchState.controlPlaneActive ? 'ACTIVE' : 'INACTIVE') }}
        </div>
        </v-card-text>
      </v-card>
      </v-window-item>

      <!-- Hard Stops Tab -->
      <v-window-item value="Hard Stops">
        <v-card flat class="card-shell border-0">
        <div class="px-4 pt-4 flex items-center justify-between gap-3">
          <div class="text-xl font-semibold text-slate-900">硬停止</div>
          <v-chip :color="realTradeHardStops.entries.length ? 'error' : 'success'" variant="outlined" size="small">
            {{ realTradeHardStops.entries.length ? `${realTradeHardStops.entries.length} 个活跃` : formatGenericStatusLabel('NONE') }}
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
              {{ formatRealTradeOperationLabel(op) }}已阻断
            </v-chip>
            <v-chip :color="realTradeHardStops.allowsCancel ? 'success' : 'error'" variant="outlined" size="small">
              撤单{{ realTradeHardStops.allowsCancel ? '允许' : '阻断' }}
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
              <div class="mt-1 text-xs text-slate-500">{{ formatTradingEnvironment(entry.tradingEnvironment) }} / {{ entry.accountId }}</div>
              <div class="mt-1 text-xs text-amber-700">{{ entry.reason }}</div>
              <div class="mt-1 text-xs text-slate-500">激活时间 {{ entry.activatedAt }}</div>
            </div>
          </div>
        </div>
        <v-empty-state v-else text="无活跃实盘硬停止。" />
        </v-card-text>
      </v-card>
      </v-window-item>

      <!-- Events Tab -->
      <v-window-item value="Events">
        <div class="grid gap-5 lg:grid-cols-[1fr_1fr]">
          <!-- Real Trade Approvals -->
          <v-card flat class="card-shell border-0">
        <div class="px-4 pt-4 flex items-center justify-between gap-3">
          <div class="text-xl font-semibold text-slate-900">实盘审批</div>
          <v-chip :color="realTradeApprovals.realTradingEnabled ? 'error' : 'default'" variant="outlined" size="small">
            {{ realTradeApprovals.realTradingEnabled ? '实盘已开放' : '实盘受限' }}
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
              <div class="font-medium text-slate-900">{{ formatRealTradeOperationLabel(entry.operation) }} / {{ entry.brokerId }}</div>
              <v-chip :color="resolveRealTradeApprovalDecisionTagType(entry.decision)" variant="outlined" size="small">
                {{ formatApprovalDecisionLabel(entry.decision) }}
              </v-chip>
            </div>
            <div class="mt-1 text-xs text-slate-500">
              {{ formatTradingEnvironment(entry.tradingEnvironment) }} / {{ entry.accountId ?? '暂无' }} / {{ formatMarketLabel(entry.market) }}
            </div>
            <div v-if="entry.reason" class="mt-1 text-xs text-amber-700">{{ entry.reason }}</div>
            <div class="mt-1 text-xs text-slate-500">{{ entry.createdAt }}</div>
          </div>
          <div class="text-xs text-slate-500">
            确认文本：{{ realTradeApprovals.requiredConfirmationText }} / 有效窗口 {{ realTradeApprovals.maxApprovalAgeMs }}ms
          </div>
        </div>
        <v-empty-state v-else :text="`暂无审批记录。确认文本：${realTradeApprovals.requiredConfirmationText} / 有效窗口 ${realTradeApprovals.maxApprovalAgeMs}ms`" />
        </v-card-text>
      </v-card>

      <!-- Kill Switch + Risk + Hard Stop Event Log -->
      <v-card flat class="card-shell border-0">
        <div class="px-4 pt-4">
          <div class="text-xl font-semibold text-slate-900">风控事件日志</div>
        </div>

        <v-card-text>
        <div class="grid gap-4">
          <!-- Kill Switch Events -->
          <div>
            <div class="mb-2 text-xs uppercase tracking-[0.2em] text-slate-500">熔断事件</div>
            <div v-if="realTradeKillSwitchEvents.entries.length" class="grid gap-2">
              <div
                v-for="entry in realTradeKillSwitchEvents.entries.slice(0, 3)"
                :key="entry.id"
                class="rounded-2xl bg-slate-50 px-3 py-3"
              >
                <div class="flex items-center justify-between gap-2">
                  <div class="text-sm font-medium text-slate-900">{{ entry.action }}</div>
                  <v-chip :color="resolveRealTradeKillSwitchEventTagType(entry.eventType)" variant="outlined" size="small">
                    {{ formatRealTradeEventTypeLabel(entry.eventType) }}
                  </v-chip>
                </div>
                <div class="mt-1 text-xs text-slate-500">
                  {{ entry.brokerId }} / {{ formatTradingEnvironment(entry.tradingEnvironment) }}
                </div>
                <div v-if="entry.reason" class="mt-1 text-xs text-amber-700">{{ entry.reason }}</div>
                <div class="mt-1 text-xs text-slate-500">{{ entry.createdAt }}</div>
              </div>
            </div>
            <div v-else class="text-sm text-slate-500">暂无熔断事件。</div>
          </div>

          <!-- Risk Events -->
          <div>
            <div class="mb-2 text-xs uppercase tracking-[0.2em] text-slate-500">风控事件</div>
            <div v-if="realTradeRiskEvents.entries.length" class="grid gap-2">
              <div
                v-for="entry in realTradeRiskEvents.entries.slice(0, 3)"
                :key="entry.id"
                class="rounded-2xl bg-slate-50 px-3 py-3"
              >
                <div class="flex items-center justify-between gap-2">
                  <div class="text-sm font-medium text-slate-900">{{ entry.action }}</div>
                  <v-chip :color="resolveRealTradeRiskEventTagType(entry.eventType)" variant="outlined" size="small">
                    {{ formatRealTradeEventTypeLabel(entry.eventType) }}
                  </v-chip>
                </div>
                <div class="mt-1 text-xs text-slate-500">
                  {{ entry.brokerId }} / {{ formatTradingEnvironment(entry.tradingEnvironment) }}
                  <span v-if="entry.quantity != null"> / 数量 {{ entry.quantity }}</span>
                </div>
                <div v-if="entry.reason" class="mt-1 text-xs text-amber-700">{{ entry.reason }}</div>
                <div class="mt-1 text-xs text-slate-500">{{ entry.createdAt }}</div>
              </div>
            </div>
            <div v-else class="text-sm text-slate-500">暂无风控事件。</div>
          </div>

          <!-- Hard Stop Events -->
          <div>
            <div class="mb-2 text-xs uppercase tracking-[0.2em] text-slate-500">硬停止事件</div>
            <div v-if="realTradeHardStopEvents.entries.length" class="grid gap-2">
              <div
                v-for="entry in realTradeHardStopEvents.entries.slice(0, 3)"
                :key="entry.id"
                class="rounded-2xl bg-slate-50 px-3 py-3"
              >
                <div class="flex items-center justify-between gap-2">
                  <div class="text-sm font-medium text-slate-900">{{ entry.action }}</div>
                  <v-chip :color="resolveRealTradeHardStopScopeTagType(entry)" variant="outlined" size="small">
                    {{ formatRealTradeEventTypeLabel(entry.eventType) }}
                  </v-chip>
                </div>
                <div class="mt-1 text-xs text-slate-500">
                  {{ entry.brokerId }} / {{ formatTradingEnvironment(entry.tradingEnvironment) }}
                  <span v-if="entry.market"> / {{ formatMarketLabel(entry.market) }}</span>
                  <span v-if="entry.symbol"> / {{ entry.symbol }}</span>
                </div>
                <div v-if="entry.reason" class="mt-1 text-xs text-amber-700">{{ entry.reason }}</div>
                <div class="mt-1 text-xs text-slate-500">{{ entry.createdAt }}</div>
              </div>
            </div>
            <div v-else class="text-sm text-slate-500">暂无硬停止事件。</div>
          </div>
        </div>
        </v-card-text>
      </v-card>
        </div>
      </v-window-item>
    </v-window>
  </div>
</template>
