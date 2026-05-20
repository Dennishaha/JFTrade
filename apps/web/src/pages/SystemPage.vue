<script setup lang="ts">
import { computed, ref } from "vue";

import {
  architectureCards,
  consolePanels,
  roadmapPhases,
} from "@jftrade/ui-contracts";

import PageHeader from "../components/PageHeader.vue";
import { useConsoleData } from "../composables/useConsoleData";

const {
  formatDateTime,
  formatDurationMs,
  formatWorkerBrokerErrorContext,
  loadError,
  loadSystemState,
  realTradeApprovals,
  realTradeHardStopEvents,
  realTradeHardStops,
  realTradeKillSwitchEvents,
  realTradeKillSwitchState,
  realTradeRiskEvents,
  realTradeRiskState,
  resolveRealTradeApprovalDecisionTagType,
  resolveRealTradeHardStopScopeTagType,
  resolveRealTradeKillSwitchEventTagType,
  resolveRealTradeRiskEventTagType,
  resolveWorkerBrokerSubscriptionTagType,
  storageOverview,
  systemStatus,
  workerBrokerOrderUpdates,
  formatRealTradeHardStopScope,
  formatRealTradeKillSwitchSource,
  formatRealTradeRiskSource,
  isLoading,
} = useConsoleData();

const workerBackoffHotspots = computed(() =>
  workerBrokerOrderUpdates.value.brokers
    .flatMap((broker) =>
      broker.topBackoffHotspots.map((hotspot) => ({
        ...hotspot,
        brokerId: broker.brokerId,
      })),
    )
    .sort((left, right) => right.remainingMs - left.remainingMs)
    .slice(0, 10),
);

const systemHeaderStats = computed(() => [
  {
    label: "API Port",
    value: systemStatus.value.apiPort,
  },
  {
    label: "Persistence",
    value: systemStatus.value.persistence.status.toUpperCase(),
    tone: systemStatus.value.persistence.status === "ok" ? "good" : "warn",
  },
  {
    label: "Strategies",
    value: systemStatus.value.strategyRuntime.activeStrategies,
  },
  {
    label: "Audit Logs",
    value: storageOverview.value.recentAuditLogs.length,
    hint: `${workerBrokerOrderUpdates.value.subscriptions.length} worker subscription(s) tracked`,
  },
]);

const systemActiveTab = ref("status");
</script>

<template>
  <div class="grid gap-6">
    <PageHeader
      eyebrow="Operations center"
      title="System / Control"
      description="把 API、持久层、worker 健康、审计和 REAL control-plane 放到同一个运维入口，优先识别异常与治理动作。"
      :stats="systemHeaderStats"
    />

    <v-tabs v-model="systemActiveTab" bg-color="transparent">
      <v-tab value="status">Status</v-tab>
      <v-tab value="worker-broker">Worker Broker</v-tab>
      <v-tab value="real-trade-risk">Real-Trade Risk</v-tab>
      <v-tab value="approvals-hard-stops">Approvals &amp; Hard Stops</v-tab>
    </v-tabs>
    <v-window v-model="systemActiveTab">
      <v-window-item value="status">
        <section class="grid gap-5 lg:grid-cols-[1.15fr_0.85fr]">
          <v-card flat class="card-shell border-0">
            <div class="flex items-center justify-between gap-3 px-4 pt-4">
              <div class="text-xl font-semibold text-slate-900">系统运行状态</div>
              <v-chip :color="systemStatus.persistence.status === 'ok' ? 'success' : 'warning'" variant="outlined" size="small">
                {{ systemStatus.persistence.status.toUpperCase() }}
              </v-chip>
            </div>
            <v-card-text>
              <v-alert
                v-if="loadError"
                title="无法拉取 API 状态"
                type="warning"
                :closable="false"
                class="mb-4"
              >
                {{ loadError }}。请确认 API 服务已启动。
              </v-alert>

              <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                <div class="rounded-3xl bg-slate-50 px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">API Port</div>
                  <div class="mt-2 text-2xl font-semibold text-slate-900">{{ systemStatus.apiPort }}</div>
                </div>
                <div class="rounded-3xl bg-slate-50 px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Migrations</div>
                  <div class="mt-2 text-2xl font-semibold text-slate-900">
                    {{ systemStatus.persistence.pendingMigrations.length === 0 ? 'Ready' : 'Pending' }}
                  </div>
                </div>
                <div class="rounded-3xl bg-slate-50 px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Tables</div>
                  <div class="mt-2 text-2xl font-semibold text-slate-900">{{ systemStatus.persistence.tables.length }}</div>
                </div>
                <div class="rounded-3xl bg-slate-50 px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Strategies</div>
                  <div class="mt-2 text-2xl font-semibold text-slate-900">
                    {{ systemStatus.strategyRuntime.activeStrategies }}
                  </div>
                </div>
              </div>

              <div class="mt-5 grid gap-3 text-sm text-slate-600">
                <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Database Path</div>
                  <div class="mt-2 break-all font-medium text-slate-900">{{ systemStatus.persistence.databasePath }}</div>
                </div>
                <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Real Trading</div>
                  <div class="mt-2 flex items-center gap-2">
                    <v-chip :color="systemStatus.realTradingEnabled ? 'error' : undefined" variant="outlined" size="small">
                      {{ systemStatus.realTradingEnabled ? 'ENABLED' : 'GATED' }}
                    </v-chip>
                  </div>
                  <div class="mt-2 text-xs text-slate-500">
                    {{ systemStatus.realTradingEnabled ? 'JFTRADE_ALLOW_REAL_TRADING=true' : 'Use SIMULATE until JFTRADE_ALLOW_REAL_TRADING is explicitly enabled.' }}
                  </div>
                  <div class="mt-1 text-xs text-slate-500">
                    {{ systemStatus.realTradingRisk.enabled
                      ? `REAL risk qty<=${systemStatus.realTradingRisk.maxOrderQuantity ?? 'N/A'} / notional<=${systemStatus.realTradingRisk.maxOrderNotional ?? 'N/A'} via ${formatRealTradeRiskSource(systemStatus.realTradingRisk.riskConfigSource)}`
                      : 'No effective REAL risk limits configured.' }}
                  </div>
                  <div class="mt-1 text-xs" :class="systemStatus.realTradingKillSwitch.active ? 'text-amber-700' : 'text-slate-500'">
                    {{ realTradeKillSwitchState.killSwitchActive
                      ? `REAL kill switch active via ${formatRealTradeKillSwitchSource(realTradeKillSwitchState.killSwitchSource)}: blocks ${realTradeKillSwitchState.blockedOperations.join(' / ')}; cancel ${realTradeKillSwitchState.allowsCancel ? 'allowed' : 'blocked'}.`
                      : 'REAL kill switch inactive; place / modify follow approval and risk gates.' }}
                  </div>
                  <div class="mt-1 text-xs" :class="realTradeHardStops.entries.length ? 'text-amber-700' : 'text-slate-500'">
                    {{ realTradeHardStops.entries.length
                      ? `Real-trade hard stops active: ${realTradeHardStops.entries.length} scope(s); cancel allowed for unwind.`
                      : 'No active REAL hard stops.' }}
                  </div>
                </div>
                <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Broker Markets</div>
                  <div class="mt-2 flex flex-wrap gap-2">
                    <v-chip
                      v-for="capability in systemStatus.broker.capabilities"
                      :key="capability.market"
                      variant="outlined"
                      size="small"
                    >
                      {{ capability.market }}
                    </v-chip>
                  </div>
                </div>
              </div>
            </v-card-text>
          </v-card>

          <v-card flat class="card-shell border-0">
            <div class="flex items-center justify-between gap-3 px-4 pt-4">
              <div class="text-xl font-semibold text-slate-900">存储概览</div>
              <v-btn :loading="isLoading" variant="text" color="primary" @click="loadSystemState()">
                刷新
              </v-btn>
            </div>
            <v-card-text>
              <div class="grid gap-3">
                <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Recent Audit Logs</div>
                  <div v-if="storageOverview.recentAuditLogs.length" class="mt-3 grid gap-2">
                    <div
                      v-for="item in storageOverview.recentAuditLogs.slice(0, 4)"
                      :key="item.id"
                      class="rounded-2xl bg-slate-50 px-3 py-3"
                    >
                      <div class="font-medium text-slate-900">{{ item.action }}</div>
                      <div class="mt-1 text-xs text-slate-500">{{ item.targetType }} / {{ item.targetId }}</div>
                    </div>
                  </div>
                  <div v-else class="mt-3 text-sm text-slate-500">暂无审计事件。</div>
                </div>

                <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Recent Execution Commands</div>
                  <div v-if="storageOverview.recentExecutionCommands.length" class="mt-3 grid gap-2">
                    <div
                      v-for="item in storageOverview.recentExecutionCommands.slice(0, 4)"
                      :key="item.id"
                      class="rounded-2xl bg-slate-50 px-3 py-3"
                    >
                      <div class="flex items-center justify-between gap-3">
                        <div class="font-medium text-slate-900">{{ item.operation }} / {{ item.brokerId }}</div>
                        <div class="text-[11px] uppercase tracking-[0.2em] text-slate-500">
                          {{ item.completedAt ? 'COMPLETED' : 'PENDING' }}
                        </div>
                      </div>
                      <div class="mt-1 break-all text-xs text-slate-500">{{ item.idempotencyKey }}</div>
                      <div class="mt-1 text-xs text-slate-500">
                        {{ item.actorType }} / {{ item.actorId }} / {{ item.internalOrderId ?? 'unassigned' }}
                      </div>
                    </div>
                  </div>
                  <div v-else class="mt-3 text-sm text-slate-500">暂无 execution command ledger 记录。</div>
                </div>

                <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Worker Broker Subscription Health</div>
                  <div
                    v-if="workerBrokerOrderUpdates.subscriptions.length || workerBrokerOrderUpdates.brokers.length"
                    class="mt-3 grid gap-3"
                  >
                    <div v-if="workerBrokerOrderUpdates.subscriptions.length" class="grid gap-2">
                      <div
                        v-for="item in workerBrokerOrderUpdates.subscriptions.slice(0, 4)"
                        :key="item.subscriptionKey"
                        class="rounded-2xl bg-slate-50 px-3 py-3"
                      >
                        <div class="flex items-center justify-between gap-3">
                          <div class="font-medium text-slate-900">
                            {{ item.brokerId }} / {{ item.market ?? 'unknown-market' }}
                          </div>
                          <v-chip :color="resolveWorkerBrokerSubscriptionTagType(item.status)" variant="outlined" size="small">
                            {{ item.status.toUpperCase() }}
                          </v-chip>
                        </div>
                        <div class="mt-1 text-xs text-slate-500">
                          {{ item.tradingEnvironment ?? 'N/A' }} / {{ item.accountId ?? 'N/A' }}
                        </div>
                        <div class="mt-1 text-xs text-slate-500">
                          {{ item.lastAction }} / {{ formatDateTime(item.lastActionAt) }}
                        </div>
                        <div v-if="item.lastError" class="mt-1 text-xs text-amber-700">
                          {{ formatWorkerBrokerErrorContext(item.lastErrorContext, item.lastError) }}
                        </div>
                        <div v-if="item.retryDelayMs != null" class="mt-1 text-xs text-slate-500">
                          Retry Delay {{ formatDurationMs(item.retryDelayMs) }}
                          <span v-if="item.backoffUntil"> / next {{ formatDateTime(item.backoffUntil) }}</span>
                        </div>
                      </div>
                    </div>

                    <div v-if="workerBrokerOrderUpdates.recentInvalidations.length" class="grid gap-2">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Recent Invalidations</div>
                      <div
                        v-for="item in workerBrokerOrderUpdates.recentInvalidations.slice(0, 3)"
                        :key="`${item.subscriptionKey}:${item.createdAt}`"
                        class="rounded-2xl border border-amber-200 bg-amber-50 px-3 py-3"
                      >
                        <div class="flex items-center justify-between gap-3">
                          <div class="font-medium text-slate-900">
                            {{ item.brokerId }} / {{ item.market ?? 'unknown-market' }}
                          </div>
                          <v-chip color="warning" variant="outlined" size="small">
                            {{ item.kind }}
                          </v-chip>
                        </div>
                        <div class="mt-1 text-xs text-slate-500">
                          {{ item.tradingEnvironment ?? 'N/A' }} / {{ item.accountId ?? 'N/A' }} / {{ formatDateTime(item.createdAt) }}
                        </div>
                        <div v-if="item.message" class="mt-1 text-xs text-amber-700">
                          {{ formatWorkerBrokerErrorContext(item.errorContext, item.message) }}
                        </div>
                        <div v-if="item.backoffUntil" class="mt-1 text-xs text-slate-500">
                          Backoff until {{ formatDateTime(item.backoffUntil) }}
                        </div>
                      </div>
                    </div>

                    <div v-if="workerBrokerOrderUpdates.brokers.length" class="grid gap-2">
                      <div
                        v-for="item in workerBrokerOrderUpdates.brokers.slice(0, 2)"
                        :key="item.brokerId"
                        class="rounded-2xl border border-dashed border-slate-300 px-3 py-3"
                      >
                        <div class="font-medium text-slate-900">{{ item.brokerId }}</div>
                        <div class="mt-1 text-xs text-slate-500">
                          {{ item.lastAction }} / {{ item.connectivity ?? 'unknown-connectivity' }} / accounts {{ item.accountsDiscovered ?? 'N/A' }}
                        </div>
                        <div v-if="item.lastError" class="mt-1 text-xs text-amber-700">
                          {{ item.lastError }}
                        </div>
                        <div class="mt-1 text-xs text-slate-500">
                          Subscriptions A{{ item.activeSubscriptions }} / R{{ item.retryingSubscriptions }} / I{{ item.inactiveSubscriptions }}
                        </div>
                        <div class="mt-1 text-xs text-slate-500">
                          Backoff subscriptions {{ item.backoffSubscriptions }}
                        </div>
                        <div v-if="item.backoffUntil" class="mt-1 text-xs" :class="item.backoffActive ? 'text-amber-700' : 'text-slate-500'">
                          {{ item.backoffActive ? 'Backoff active until' : 'Last backoff until' }} {{ formatDateTime(item.backoffUntil) }}
                          <span v-if="item.backoffRemainingMs != null"> / remaining {{ formatDurationMs(item.backoffRemainingMs) }}</span>
                        </div>
                        <div
                          v-if="item.layeredBackoffSummaries.length"
                          class="mt-3 grid gap-2 border-t border-slate-200 pt-3"
                        >
                          <div class="text-[11px] uppercase tracking-[0.16em] text-slate-500">
                            Layered Backoff Governance
                          </div>
                          <div
                            v-for="layer in item.layeredBackoffSummaries.slice(0, 3)"
                            :key="`${item.brokerId}:${layer.tradingEnvironment ?? 'na'}:${layer.accountId ?? 'na'}`"
                            class="rounded-2xl bg-slate-50 px-3 py-2"
                          >
                            <div class="font-medium text-slate-900">
                              {{ layer.tradingEnvironment ?? 'N/A' }} / {{ layer.accountId ?? 'N/A' }}
                            </div>
                            <div class="mt-1 text-xs text-slate-500">
                              Subscriptions A{{ layer.activeSubscriptions }} / R{{ layer.retryingSubscriptions }} / I{{ layer.inactiveSubscriptions }}
                            </div>
                            <div class="mt-1 text-xs text-slate-500">
                              Backoff {{ layer.backoffSubscriptions }} / dominant {{ layer.dominantBackoffSource ?? 'N/A' }} / top market {{ layer.topBackoffMarket ?? 'N/A' }}
                            </div>
                            <div v-if="layer.longestBackoffRemainingMs != null" class="mt-1 text-xs text-amber-700">
                              Longest remaining {{ formatDurationMs(layer.longestBackoffRemainingMs) }}
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                  <div v-else class="mt-3 text-sm text-slate-500">暂无 worker broker subscription 健康摘要。</div>
                </div>

                <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                  <div class="flex items-center justify-between gap-3">
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Real Trade Approvals</div>
                    <v-chip :color="realTradeApprovals.realTradingEnabled ? 'error' : undefined" variant="outlined" size="small">
                      {{ realTradeApprovals.realTradingEnabled ? 'REAL ENABLED' : 'REAL GATED' }}
                    </v-chip>
                  </div>
                  <div v-if="realTradeApprovals.entries.length" class="mt-3 grid gap-2">
                    <div
                      v-for="item in realTradeApprovals.entries.slice(0, 3)"
                      :key="item.id"
                      class="rounded-2xl bg-slate-50 px-3 py-3"
                    >
                      <div class="flex items-center justify-between gap-3">
                        <div class="font-medium text-slate-900">{{ item.operation }} / {{ item.brokerId }}</div>
                        <v-chip :color="resolveRealTradeApprovalDecisionTagType(item.decision) === 'danger' ? 'error' : resolveRealTradeApprovalDecisionTagType(item.decision)" variant="outlined" size="small">
                          {{ item.decision.toUpperCase() }}
                        </v-chip>
                      </div>
                      <div class="mt-1 text-xs text-slate-500">
                        {{ item.tradingEnvironment ?? 'N/A' }} / {{ item.accountId ?? 'N/A' }} / {{ item.market ?? 'N/A' }}
                      </div>
                      <div class="mt-1 text-xs text-slate-500">
                        operator {{ item.operatorId ?? 'unassigned' }} / ticket {{ item.ticketId ?? 'N/A' }}
                      </div>
                    </div>
                    <div class="text-xs text-slate-500">
                      Confirmation {{ realTradeApprovals.requiredConfirmationText }} / window {{ realTradeApprovals.maxApprovalAgeMs }}ms
                    </div>
                  </div>
                  <div v-else class="mt-3 text-sm text-slate-500">
                    暂无 real-trade approval 审计；确认串 {{ realTradeApprovals.requiredConfirmationText }} / window {{ realTradeApprovals.maxApprovalAgeMs }}ms。
                  </div>
                </div>

                <div class="grid gap-3 sm:grid-cols-2">
                  <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Pending Outbox</div>
                    <div class="mt-2 text-2xl font-semibold text-slate-900">
                      {{ storageOverview.pendingOutbox.length }}
                    </div>
                  </div>
                  <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Recent Jobs</div>
                    <div class="mt-2 text-2xl font-semibold text-slate-900">
                      {{ storageOverview.recentJobs.length }}
                    </div>
                  </div>
                </div>
              </div>
            </v-card-text>
          </v-card>
        </section>
      </v-window-item>

      <v-window-item value="worker-broker">
        <section>
          <v-card flat class="card-shell border-0">
            <div class="flex items-center justify-between gap-3 px-4 pt-4">
              <div>
                <div class="text-xl font-semibold text-slate-900">Worker Backoff Hotspots</div>
                <div class="mt-1 text-xs text-slate-500">
                  按剩余退避时间排序，展示订阅 scope、下一次尝试时间与整形后的错误上下文。
                </div>
              </div>
              <v-chip :color="workerBackoffHotspots.length ? 'warning' : 'success'" variant="outlined" size="small">
                {{ workerBackoffHotspots.length ? `${workerBackoffHotspots.length} ACTIVE` : 'CLEAR' }}
              </v-chip>
            </div>
            <v-card-text>
              <div v-if="workerBackoffHotspots.length" class="overflow-x-auto">
                <table class="min-w-full text-left text-sm">
                  <thead class="text-xs uppercase tracking-[0.18em] text-slate-500">
                    <tr>
                      <th class="whitespace-nowrap px-3 py-2">Broker / Scope</th>
                      <th class="whitespace-nowrap px-3 py-2">Source</th>
                      <th class="whitespace-nowrap px-3 py-2">Remaining</th>
                      <th class="whitespace-nowrap px-3 py-2">Retry At</th>
                      <th class="whitespace-nowrap px-3 py-2">Last Action</th>
                      <th class="whitespace-nowrap px-3 py-2">Reason</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr
                      v-for="hotspot in workerBackoffHotspots"
                      :key="`${hotspot.brokerId}:${hotspot.subscriptionKey}:${hotspot.source}`"
                      class="border-t border-slate-100 align-top"
                    >
                      <td class="px-3 py-3">
                        <div class="font-semibold text-slate-900">{{ hotspot.brokerId }}</div>
                        <div class="mt-1 break-all text-xs text-slate-500">{{ hotspot.subscriptionKey }}</div>
                        <div class="mt-1 text-xs text-slate-500">
                          {{ hotspot.tradingEnvironment ?? 'N/A' }} / {{ hotspot.accountId ?? 'N/A' }} / {{ hotspot.market ?? 'N/A' }}
                        </div>
                      </td>
                      <td class="whitespace-nowrap px-3 py-3">
                        <v-chip :color="hotspot.source === 'DISCONNECTED' ? 'error' : 'warning'" variant="outlined" size="small">
                          {{ hotspot.source }}
                        </v-chip>
                      </td>
                      <td class="whitespace-nowrap px-3 py-3 font-semibold text-amber-700">
                        {{ formatDurationMs(hotspot.remainingMs) }}
                      </td>
                      <td class="whitespace-nowrap px-3 py-3 text-slate-600">
                        {{ formatDateTime(hotspot.backoffUntil) }}
                      </td>
                      <td class="whitespace-nowrap px-3 py-3 text-slate-600">
                        {{ formatDateTime(hotspot.lastActionAt) }}
                      </td>
                      <td class="min-w-[16rem] px-3 py-3">
                        <div class="font-medium text-slate-900">
                          {{ formatWorkerBrokerErrorContext(hotspot.reasonContext, hotspot.reason) }}
                        </div>
                        <div
                          v-if="hotspot.reasonContext?.rawMessage && hotspot.reasonContext.rawMessage !== hotspot.reasonContext.summary"
                          class="mt-1 break-all text-xs text-slate-500"
                        >
                          raw: {{ hotspot.reasonContext.rawMessage }}
                        </div>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <v-empty-state v-else text="当前没有处于退避窗口的 broker subscription hotspot。订阅失败、断线或 broker error 会在这里显示下一次重试时间，便于快速判断是否需要人工介入。" />
            </v-card-text>
          </v-card>
        </section>
      </v-window-item>

      <v-window-item value="real-trade-risk">
        <section class="grid gap-5 lg:grid-cols-3">
          <v-card flat class="card-shell border-0">
            <div class="flex items-center justify-between gap-3 px-4 pt-4">
              <div class="text-xl font-semibold text-slate-900">Real Trade Kill Switch</div>
              <v-chip :color="realTradeKillSwitchState.killSwitchActive ? 'error' : undefined" variant="outlined" size="small">
                {{ realTradeKillSwitchState.killSwitchActive ? 'SWITCH ON' : 'SWITCH OFF' }}
              </v-chip>
            </div>
            <v-card-text>
              <div class="rounded-2xl bg-slate-50 px-3 py-3">
                <div class="flex items-center justify-between gap-3">
                  <div class="font-medium text-slate-900">
                    {{ formatRealTradeKillSwitchSource(realTradeKillSwitchState.killSwitchSource) }}
                  </div>
                  <v-chip :color="realTradeKillSwitchState.killSwitchActive ? 'error' : undefined" variant="outlined" size="small">
                    {{ realTradeKillSwitchState.killSwitchActive ? 'ACTIVE' : 'CLEAR' }}
                  </v-chip>
                </div>
                <div class="mt-1 text-xs text-slate-500">
                  env {{ realTradeKillSwitchState.envConfiguredActive ? 'on' : 'off' }} / control-plane {{ realTradeKillSwitchState.controlPlaneActive ? 'on' : 'off' }}
                </div>
              </div>
              <div v-if="realTradeKillSwitchEvents.entries.length" class="mt-3 grid gap-2">
                <div
                  v-for="item in realTradeKillSwitchEvents.entries.slice(0, 3)"
                  :key="item.id"
                  class="rounded-2xl bg-slate-50 px-3 py-3"
                >
                  <div class="flex items-center justify-between gap-3">
                    <div class="font-medium text-slate-900">{{ item.eventType.toUpperCase() }} / {{ item.brokerId }}</div>
                    <v-chip :color="resolveRealTradeKillSwitchEventTagType(item.eventType) === 'danger' ? 'error' : resolveRealTradeKillSwitchEventTagType(item.eventType)" variant="outlined" size="small">
                      {{ item.eventType.toUpperCase() }}
                    </v-chip>
                  </div>
                  <div class="mt-1 text-xs text-slate-500">{{ item.createdAt }}</div>
                </div>
              </div>
            </v-card-text>
          </v-card>

          <v-card flat class="card-shell border-0">
            <div class="flex items-center justify-between gap-3 px-4 pt-4">
              <div class="text-xl font-semibold text-slate-900">Real Trade Risk</div>
              <v-chip :color="realTradeRiskState.riskEnabled ? 'warning' : undefined" variant="outlined" size="small">
                {{ realTradeRiskState.riskEnabled ? 'LIMITS ON' : 'LIMITS OFF' }}
              </v-chip>
            </div>
            <v-card-text>
              <div class="rounded-2xl bg-slate-50 px-3 py-3">
                <div class="font-medium text-slate-900">{{ formatRealTradeRiskSource(realTradeRiskState.riskConfigSource) }}</div>
                <div class="mt-1 text-xs text-slate-500">
                  effective qty {{ realTradeRiskState.effectiveMaxOrderQuantity ?? 'N/A' }} / notional {{ realTradeRiskState.effectiveMaxOrderNotional ?? 'N/A' }}
                </div>
              </div>
              <div v-if="realTradeRiskEvents.entries.length" class="mt-3 grid gap-2">
                <div
                  v-for="item in realTradeRiskEvents.entries.slice(0, 3)"
                  :key="item.id"
                  class="rounded-2xl bg-slate-50 px-3 py-3"
                >
                  <div class="flex items-center justify-between gap-3">
                    <div class="font-medium text-slate-900">{{ item.eventType.toUpperCase() }} / {{ item.brokerId }}</div>
                    <v-chip :color="resolveRealTradeRiskEventTagType(item.eventType) === 'danger' ? 'error' : resolveRealTradeRiskEventTagType(item.eventType)" variant="outlined" size="small">
                      {{ item.eventType.toUpperCase() }}
                    </v-chip>
                  </div>
                  <div class="mt-1 text-xs text-slate-500">{{ item.reason ?? 'N/A' }}</div>
                </div>
              </div>
            </v-card-text>
          </v-card>
        </section>
      </v-window-item>

      <v-window-item value="approvals-hard-stops">
        <section class="grid gap-5 lg:grid-cols-3">
          <v-card flat class="card-shell border-0">
            <div class="flex items-center justify-between gap-3 px-4 pt-4">
              <div class="text-xl font-semibold text-slate-900">Real Trade Hard Stops</div>
              <v-chip :color="realTradeHardStops.entries.length ? 'error' : undefined" variant="outlined" size="small">
                {{ realTradeHardStops.entries.length ? 'ACTIVE' : 'CLEAR' }}
              </v-chip>
            </div>
            <v-card-text>
              <div v-if="realTradeHardStops.entries.length" class="grid gap-2">
                <div
                  v-for="item in realTradeHardStops.entries.slice(0, 3)"
                  :key="item.id"
                  class="rounded-2xl bg-slate-50 px-3 py-3"
                >
                  <div class="flex items-center justify-between gap-3">
                    <div class="font-medium text-slate-900">{{ item.brokerId }} / {{ item.accountId }}</div>
                    <v-chip :color="resolveRealTradeHardStopScopeTagType(item) === 'danger' ? 'error' : resolveRealTradeHardStopScopeTagType(item)" variant="outlined" size="small">
                      {{ formatRealTradeHardStopScope(item) }}
                    </v-chip>
                  </div>
                  <div class="mt-1 text-xs text-slate-500">{{ item.tradingEnvironment }} / operator {{ item.operatorId }}</div>
                  <div class="mt-1 text-xs text-slate-700">{{ item.reason }}</div>
                </div>
              </div>
              <div v-else class="text-sm text-slate-500">暂无 active real-trade hard stop。</div>
            </v-card-text>
          </v-card>
        </section>
      </v-window-item>
    </v-window>

    <section class="grid gap-5 xl:grid-cols-3">
      <v-card
        v-for="card in architectureCards"
        :key="card.title"
        flat
        class="card-shell h-full border-0"
      >
        <div class="flex items-center justify-between gap-3 px-4 pt-4">
          <div>
            <div class="text-xs uppercase tracking-[0.24em] text-teal-700">{{ card.owner }}</div>
            <div class="mt-1 text-xl font-semibold text-slate-900">{{ card.title }}</div>
          </div>
          <v-chip color="success" variant="outlined" size="small">{{ card.status }}</v-chip>
        </div>
        <v-card-text>
          <p class="text-sm leading-6 text-slate-600">{{ card.summary }}</p>
          <ul class="mt-5 grid gap-2 text-sm text-slate-700">
            <li
              v-for="bullet in card.bullets"
              :key="bullet"
              class="rounded-2xl bg-slate-50 px-4 py-3"
            >
              {{ bullet }}
            </li>
          </ul>
        </v-card-text>
      </v-card>
    </section>

    <section class="grid gap-5 lg:grid-cols-[1.1fr_0.9fr]">
      <v-card flat class="card-shell border-0">
        <div class="text-xl font-semibold text-slate-900 px-4 pt-4">交付路径</div>
        <v-card-text>
          <div class="grid gap-4">
            <div
              v-for="phase in roadmapPhases"
              :key="phase.key"
              class="rounded-3xl border border-slate-200 bg-white px-4 py-4"
            >
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">{{ phase.target }}</div>
              <div class="mt-1 text-base font-semibold text-slate-900">{{ phase.title }}</div>
              <div class="mt-1 text-sm leading-6 text-slate-600">{{ phase.summary }}</div>
            </div>
          </div>
        </v-card-text>
      </v-card>

      <v-card flat class="card-shell border-0">
        <div class="text-xl font-semibold text-slate-900 px-4 pt-4">控制台模块</div>
        <v-card-text>
          <div class="grid gap-3">
            <div
              v-for="panel in consolePanels"
              :key="panel.name"
              class="rounded-3xl border border-slate-200 bg-white px-4 py-4"
            >
              <div class="flex items-center justify-between gap-3">
                <div class="text-base font-semibold text-slate-900">{{ panel.name }}</div>
                <v-chip variant="outlined" size="small">{{ panel.state }}</v-chip>
              </div>
              <p class="mt-2 text-sm leading-6 text-slate-600">{{ panel.description }}</p>
            </div>
          </div>
        </v-card-text>
      </v-card>
    </section>
  </div>
</template>
