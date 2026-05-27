<script setup lang="ts">
import { computed, onMounted, ref } from "vue";

import PageHeader from "../components/PageHeader.vue";
import {
  formatApprovalDecisionLabel,
  formatConnectivityLabel,
  formatDateTime,
  formatDurationMs,
  formatGenericStatusLabel,
  formatMarketDataChannelLabel,
  formatMarketLabel,
  formatRealTradeEventTypeLabel,
  formatRealTradeHardStopScope,
  formatRealTradeKillSwitchSource,
  formatRealTradeOperationLabel,
  formatRealTradeRiskSource,
  formatTradingEnvironment,
  formatWorkerBrokerActionLabel,
  formatWorkerBrokerBackoffSourceLabel,
  formatWorkerBrokerSubscriptionStatusLabel,
  formatWorkerBrokerErrorContext,
  resolveRealTradeApprovalDecisionTagType,
  resolveRealTradeHardStopScopeTagType,
  resolveRealTradeKillSwitchEventTagType,
  resolveRealTradeRiskEventTagType,
  resolveWorkerBrokerSubscriptionTagType,
} from "../composables/consoleDataFormatting";
import { useConsoleData } from "../composables/useConsoleData";

const {
  loadError,
  loadSystemState,
  realTradeApprovals,
  realTradeHardStopEvents,
  realTradeHardStops,
  realTradeKillSwitchEvents,
  realTradeKillSwitchState,
  realTradeRiskEvents,
  realTradeRiskState,
  storageOverview,
  systemStatus,
  workerBrokerOrderUpdates,
  isLoading,
  isLoadingMarketData,
  loadMarketDataSubscriptions,
  marketDataSubscriptions,
} = useConsoleData();

onMounted(() => {
  void loadMarketDataSubscriptions();
});

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
    label: "API 端口",
    value: systemStatus.value.apiPort,
  },
  {
    label: "持久化",
    value: formatGenericStatusLabel(systemStatus.value.persistence.status),
    tone: systemStatus.value.persistence.status === "ok" ? "good" : "warn",
  },
  {
    label: "策略",
    value: systemStatus.value.strategyRuntime.activeStrategies,
  },
  {
    label: "审计日志",
    value: storageOverview.value.recentAuditLogs.length,
    hint: `跟踪 ${workerBrokerOrderUpdates.value.subscriptions.length} 个工作进程订阅`,
  },
]);

const systemActiveTab = ref("status");
</script>

<template>
  <div class="grid gap-6">
    <PageHeader
      eyebrow="运维中心"
      title="系统 / 控制"
      description="把 API、持久层、工作进程健康、审计和实盘控制面放到同一个运维入口，优先识别异常与治理动作。"
      :stats="systemHeaderStats"
    />

    <v-tabs v-model="systemActiveTab" bg-color="transparent" class="tv-page-tabs">
      <v-tab value="status">状态</v-tab>
      <v-tab value="worker-broker">工作进程券商</v-tab>
      <v-tab value="real-trade-risk">实盘风控</v-tab>
      <v-tab value="approvals-hard-stops">审批与硬停止</v-tab>
    </v-tabs>
    <v-window v-model="systemActiveTab">
      <v-window-item value="status">
        <section class="grid gap-5 lg:grid-cols-[1.15fr_0.85fr]">
          <v-card flat class="card-shell border-0">
            <div class="flex items-center justify-between gap-3 px-4 pt-4">
              <div class="text-xl font-semibold text-slate-900">系统运行状态</div>
              <v-chip :color="systemStatus.persistence.status === 'ok' ? 'success' : 'warning'" variant="outlined" size="small">
                {{ formatGenericStatusLabel(systemStatus.persistence.status) }}
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
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">API 端口</div>
                  <div class="mt-2 text-2xl font-semibold text-slate-900">{{ systemStatus.apiPort }}</div>
                </div>
                <div class="rounded-3xl bg-slate-50 px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">迁移状态</div>
                  <div class="mt-2 text-2xl font-semibold text-slate-900">
                    {{ formatGenericStatusLabel(systemStatus.persistence.pendingMigrations.length === 0 ? 'READY' : 'PENDING') }}
                  </div>
                </div>
                <div class="rounded-3xl bg-slate-50 px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">数据表</div>
                  <div class="mt-2 text-2xl font-semibold text-slate-900">{{ systemStatus.persistence.tables.length }}</div>
                </div>
                <div class="rounded-3xl bg-slate-50 px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">策略</div>
                  <div class="mt-2 text-2xl font-semibold text-slate-900">
                    {{ systemStatus.strategyRuntime.activeStrategies }}
                  </div>
                </div>
              </div>

              <div class="mt-5 grid gap-3 text-sm text-slate-600">
                <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">数据库路径</div>
                  <div class="mt-2 break-all font-medium text-slate-900">{{ systemStatus.persistence.databasePath }}</div>
                </div>
                <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">实盘交易</div>
                  <div class="mt-2 flex items-center gap-2">
                    <v-chip :color="systemStatus.realTradingEnabled ? 'error' : undefined" variant="outlined" size="small">
                      {{ formatGenericStatusLabel(systemStatus.realTradingEnabled ? 'ENABLED' : 'GATED') }}
                    </v-chip>
                  </div>
                  <div class="mt-2 text-xs text-slate-500">
                    {{ systemStatus.realTradingEnabled ? 'JFTRADE_ALLOW_REAL_TRADING=true' : '未显式开启 JFTRADE_ALLOW_REAL_TRADING 前请使用模拟环境。' }}
                  </div>
                  <div class="mt-1 text-xs text-slate-500">
                    {{ systemStatus.realTradingRisk.enabled
                      ? `实盘风控：数量<=${systemStatus.realTradingRisk.maxOrderQuantity ?? '暂无'} / 金额<=${systemStatus.realTradingRisk.maxOrderNotional ?? '暂无'}，来源 ${formatRealTradeRiskSource(systemStatus.realTradingRisk.riskConfigSource)}`
                      : '未配置有效实盘风控限额。' }}
                  </div>
                  <div class="mt-1 text-xs" :class="systemStatus.realTradingKillSwitch.active ? 'text-amber-700' : 'text-slate-500'">
                    {{ realTradeKillSwitchState.killSwitchActive
                      ? `实盘熔断开关已激活，来源 ${formatRealTradeKillSwitchSource(realTradeKillSwitchState.killSwitchSource)}：阻断 ${realTradeKillSwitchState.blockedOperations.map(formatRealTradeOperationLabel).join(' / ')}；撤单${realTradeKillSwitchState.allowsCancel ? '允许' : '阻断'}。`
                      : '实盘熔断开关未激活；下单与改单仍受审批和风控门禁约束。' }}
                  </div>
                  <div class="mt-1 text-xs" :class="realTradeHardStops.entries.length ? 'text-amber-700' : 'text-slate-500'">
                    {{ realTradeHardStops.entries.length
                      ? `实盘硬停止已激活：${realTradeHardStops.entries.length} 个范围；允许撤单用于退出。`
                      : '无活跃实盘硬停止。' }}
                  </div>
                </div>
                <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">券商市场</div>
                  <div class="mt-2 flex flex-wrap gap-2">
                    <v-chip
                      v-for="capability in systemStatus.broker.capabilities"
                      :key="capability.market"
                      variant="outlined"
                      size="small"
                    >
                      {{ formatMarketLabel(capability.market) }}
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
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">最近审计日志</div>
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
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">最近执行指令</div>
                  <div v-if="storageOverview.recentExecutionCommands.length" class="mt-3 grid gap-2">
                    <div
                      v-for="item in storageOverview.recentExecutionCommands.slice(0, 4)"
                      :key="item.id"
                      class="rounded-2xl bg-slate-50 px-3 py-3"
                    >
                      <div class="flex items-center justify-between gap-3">
                        <div class="font-medium text-slate-900">{{ formatRealTradeOperationLabel(item.operation) }} / {{ item.brokerId }}</div>
                        <div class="text-[11px] uppercase tracking-[0.2em] text-slate-500">
                          {{ formatGenericStatusLabel(item.completedAt ? 'COMPLETED' : 'PENDING') }}
                        </div>
                      </div>
                      <div class="mt-1 break-all text-xs text-slate-500">{{ item.idempotencyKey }}</div>
                      <div class="mt-1 text-xs text-slate-500">
                        操作者 {{ item.actorType }} / {{ item.actorId }} / {{ item.internalOrderId ?? '未分配' }}
                      </div>
                    </div>
                  </div>
                  <div v-else class="mt-3 text-sm text-slate-500">暂无执行指令台账记录。</div>
                </div>

                <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">工作进程券商订阅健康</div>
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
                            {{ item.brokerId }} / {{ formatMarketLabel(item.market) }}
                          </div>
                          <v-chip :color="resolveWorkerBrokerSubscriptionTagType(item.status)" variant="outlined" size="small">
                            {{ formatWorkerBrokerSubscriptionStatusLabel(item.status) }}
                          </v-chip>
                        </div>
                        <div class="mt-1 text-xs text-slate-500">
                          {{ formatTradingEnvironment(item.tradingEnvironment) }} / {{ item.accountId ?? '暂无' }}
                        </div>
                        <div class="mt-1 text-xs text-slate-500">
                          {{ formatWorkerBrokerActionLabel(item.lastAction) }} / {{ formatDateTime(item.lastActionAt) }}
                        </div>
                        <div v-if="item.lastError" class="mt-1 text-xs text-amber-700">
                          {{ formatWorkerBrokerErrorContext(item.lastErrorContext, item.lastError) }}
                        </div>
                        <div v-if="item.retryDelayMs != null" class="mt-1 text-xs text-slate-500">
                          重试延迟 {{ formatDurationMs(item.retryDelayMs) }}
                          <span v-if="item.backoffUntil"> / 下次 {{ formatDateTime(item.backoffUntil) }}</span>
                        </div>
                      </div>
                    </div>

                    <div v-if="workerBrokerOrderUpdates.recentInvalidations.length" class="grid gap-2">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">最近失效记录</div>
                      <div
                        v-for="item in workerBrokerOrderUpdates.recentInvalidations.slice(0, 3)"
                        :key="`${item.subscriptionKey}:${item.createdAt}`"
                        class="rounded-2xl border border-amber-200 bg-amber-50 px-3 py-3"
                      >
                        <div class="flex items-center justify-between gap-3">
                          <div class="font-medium text-slate-900">
                            {{ item.brokerId }} / {{ formatMarketLabel(item.market) }}
                          </div>
                          <v-chip color="warning" variant="outlined" size="small">
                            {{ formatWorkerBrokerBackoffSourceLabel(item.kind) }}
                          </v-chip>
                        </div>
                        <div class="mt-1 text-xs text-slate-500">
                          {{ formatTradingEnvironment(item.tradingEnvironment) }} / {{ item.accountId ?? '暂无' }} / {{ formatDateTime(item.createdAt) }}
                        </div>
                        <div v-if="item.message" class="mt-1 text-xs text-amber-700">
                          {{ formatWorkerBrokerErrorContext(item.errorContext, item.message) }}
                        </div>
                        <div v-if="item.backoffUntil" class="mt-1 text-xs text-slate-500">
                          退避至 {{ formatDateTime(item.backoffUntil) }}
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
                          {{ formatWorkerBrokerActionLabel(item.lastAction) }} / {{ formatConnectivityLabel(item.connectivity) }} / 账户 {{ item.accountsDiscovered ?? '暂无' }}
                        </div>
                        <div v-if="item.lastError" class="mt-1 text-xs text-amber-700">
                          {{ item.lastError }}
                        </div>
                        <div class="mt-1 text-xs text-slate-500">
                          订阅：活跃 {{ item.activeSubscriptions }} / 重试 {{ item.retryingSubscriptions }} / 停用 {{ item.inactiveSubscriptions }}
                        </div>
                        <div class="mt-1 text-xs text-slate-500">
                          退避订阅 {{ item.backoffSubscriptions }}
                        </div>
                        <div v-if="item.backoffUntil" class="mt-1 text-xs" :class="item.backoffActive ? 'text-amber-700' : 'text-slate-500'">
                          {{ item.backoffActive ? '退避生效至' : '上次退避至' }} {{ formatDateTime(item.backoffUntil) }}
                          <span v-if="item.backoffRemainingMs != null"> / 剩余 {{ formatDurationMs(item.backoffRemainingMs) }}</span>
                        </div>
                        <div
                          v-if="item.layeredBackoffSummaries.length"
                          class="mt-3 grid gap-2 border-t border-slate-200 pt-3"
                        >
                          <div class="text-[11px] uppercase tracking-[0.16em] text-slate-500">
                            分层退避治理
                          </div>
                          <div
                            v-for="layer in item.layeredBackoffSummaries.slice(0, 3)"
                            :key="`${item.brokerId}:${layer.tradingEnvironment ?? 'na'}:${layer.accountId ?? 'na'}`"
                            class="rounded-2xl bg-slate-50 px-3 py-2"
                          >
                            <div class="font-medium text-slate-900">
                              {{ formatTradingEnvironment(layer.tradingEnvironment) }} / {{ layer.accountId ?? '暂无' }}
                            </div>
                            <div class="mt-1 text-xs text-slate-500">
                              订阅：活跃 {{ layer.activeSubscriptions }} / 重试 {{ layer.retryingSubscriptions }} / 停用 {{ layer.inactiveSubscriptions }}
                            </div>
                            <div class="mt-1 text-xs text-slate-500">
                              退避 {{ layer.backoffSubscriptions }} / 主要来源 {{ formatWorkerBrokerBackoffSourceLabel(layer.dominantBackoffSource) }} / 最高退避市场 {{ formatMarketLabel(layer.topBackoffMarket) }}
                            </div>
                            <div v-if="layer.longestBackoffRemainingMs != null" class="mt-1 text-xs text-amber-700">
                              最长剩余 {{ formatDurationMs(layer.longestBackoffRemainingMs) }}
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                  <div v-else class="mt-3 text-sm text-slate-500">暂无工作进程券商订阅健康摘要。</div>
                </div>

                <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                  <div class="flex items-center justify-between gap-3">
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">实盘审批</div>
                    <v-chip :color="realTradeApprovals.realTradingEnabled ? 'error' : undefined" variant="outlined" size="small">
                      {{ realTradeApprovals.realTradingEnabled ? '实盘已开放' : '实盘受限' }}
                    </v-chip>
                  </div>
                  <div v-if="realTradeApprovals.entries.length" class="mt-3 grid gap-2">
                    <div
                      v-for="item in realTradeApprovals.entries.slice(0, 3)"
                      :key="item.id"
                      class="rounded-2xl bg-slate-50 px-3 py-3"
                    >
                      <div class="flex items-center justify-between gap-3">
                        <div class="font-medium text-slate-900">{{ formatRealTradeOperationLabel(item.operation) }} / {{ item.brokerId }}</div>
                        <v-chip :color="resolveRealTradeApprovalDecisionTagType(item.decision) === 'danger' ? 'error' : resolveRealTradeApprovalDecisionTagType(item.decision)" variant="outlined" size="small">
                          {{ formatApprovalDecisionLabel(item.decision) }}
                        </v-chip>
                      </div>
                      <div class="mt-1 text-xs text-slate-500">
                        {{ formatTradingEnvironment(item.tradingEnvironment) }} / {{ item.accountId ?? '暂无' }} / {{ formatMarketLabel(item.market) }}
                      </div>
                      <div class="mt-1 text-xs text-slate-500">
                        操作员 {{ item.operatorId ?? '未分配' }} / 票据 {{ item.ticketId ?? '暂无' }}
                      </div>
                    </div>
                    <div class="text-xs text-slate-500">
                      确认文本 {{ realTradeApprovals.requiredConfirmationText }} / 有效窗口 {{ formatDurationMs(realTradeApprovals.maxApprovalAgeMs) }}
                    </div>
                  </div>
                  <div v-else class="mt-3 text-sm text-slate-500">
                    暂无实盘审批审计；确认串 {{ realTradeApprovals.requiredConfirmationText }} / 有效窗口 {{ formatDurationMs(realTradeApprovals.maxApprovalAgeMs) }}。
                  </div>
                </div>

                <div class="grid gap-3 sm:grid-cols-2">
                  <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">待发送箱</div>
                    <div class="mt-2 text-2xl font-semibold text-slate-900">
                      {{ storageOverview.pendingOutbox.length }}
                    </div>
                  </div>
                  <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">最近任务</div>
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
                <div class="text-xl font-semibold text-slate-900">工作进程退避热点</div>
                <div class="mt-1 text-xs text-slate-500">
                  按剩余退避时间排序，展示订阅范围、下一次尝试时间与整理后的错误上下文。
                </div>
              </div>
              <v-chip :color="workerBackoffHotspots.length ? 'warning' : 'success'" variant="outlined" size="small">
                {{ workerBackoffHotspots.length ? `${workerBackoffHotspots.length} 个活跃` : formatGenericStatusLabel('CLEAR') }}
              </v-chip>
            </div>
            <v-card-text>
              <div v-if="workerBackoffHotspots.length" class="overflow-x-auto">
                <table class="min-w-full text-left text-sm">
                  <thead class="text-xs uppercase tracking-[0.18em] text-slate-500">
                    <tr>
                      <th class="whitespace-nowrap px-3 py-2">券商 / 范围</th>
                      <th class="whitespace-nowrap px-3 py-2">来源</th>
                      <th class="whitespace-nowrap px-3 py-2">剩余</th>
                      <th class="whitespace-nowrap px-3 py-2">重试时间</th>
                      <th class="whitespace-nowrap px-3 py-2">最近动作</th>
                      <th class="whitespace-nowrap px-3 py-2">原因</th>
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
                          {{ formatTradingEnvironment(hotspot.tradingEnvironment) }} / {{ hotspot.accountId ?? '暂无' }} / {{ formatMarketLabel(hotspot.market) }}
                        </div>
                      </td>
                      <td class="whitespace-nowrap px-3 py-3">
                        <v-chip :color="hotspot.source === 'DISCONNECTED' ? 'error' : 'warning'" variant="outlined" size="small">
                          {{ formatWorkerBrokerBackoffSourceLabel(hotspot.source) }}
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
                          原始信息：{{ hotspot.reasonContext.rawMessage }}
                        </div>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <v-empty-state v-else text="当前没有处于退避窗口的券商订阅热点。订阅失败、断线或券商错误会在这里显示下一次重试时间，便于快速判断是否需要人工介入。" />
            </v-card-text>
          </v-card>
        </section>
      </v-window-item>

      <v-window-item value="real-trade-risk">
        <section class="grid gap-5 lg:grid-cols-3">
          <v-card flat class="card-shell border-0">
            <div class="flex items-center justify-between gap-3 px-4 pt-4">
              <div class="text-xl font-semibold text-slate-900">实盘熔断开关</div>
              <v-chip :color="realTradeKillSwitchState.killSwitchActive ? 'error' : undefined" variant="outlined" size="small">
                {{ realTradeKillSwitchState.killSwitchActive ? '熔断开启' : '熔断关闭' }}
              </v-chip>
            </div>
            <v-card-text>
              <div class="rounded-2xl bg-slate-50 px-3 py-3">
                <div class="flex items-center justify-between gap-3">
                  <div class="font-medium text-slate-900">
                    {{ formatRealTradeKillSwitchSource(realTradeKillSwitchState.killSwitchSource) }}
                  </div>
                  <v-chip :color="realTradeKillSwitchState.killSwitchActive ? 'error' : undefined" variant="outlined" size="small">
                    {{ formatGenericStatusLabel(realTradeKillSwitchState.killSwitchActive ? 'ACTIVE' : 'CLEAR') }}
                  </v-chip>
                </div>
                <div class="mt-1 text-xs text-slate-500">
                  环境变量 {{ formatGenericStatusLabel(realTradeKillSwitchState.envConfiguredActive ? 'ON' : 'OFF') }} / 控制面 {{ formatGenericStatusLabel(realTradeKillSwitchState.controlPlaneActive ? 'ON' : 'OFF') }}
                </div>
              </div>
              <div v-if="realTradeKillSwitchEvents.entries.length" class="mt-3 grid gap-2">
                <div
                  v-for="item in realTradeKillSwitchEvents.entries.slice(0, 3)"
                  :key="item.id"
                  class="rounded-2xl bg-slate-50 px-3 py-3"
                >
                  <div class="flex items-center justify-between gap-3">
                    <div class="font-medium text-slate-900">{{ formatRealTradeEventTypeLabel(item.eventType) }} / {{ item.brokerId }}</div>
                    <v-chip :color="resolveRealTradeKillSwitchEventTagType(item.eventType) === 'danger' ? 'error' : resolveRealTradeKillSwitchEventTagType(item.eventType)" variant="outlined" size="small">
                      {{ formatRealTradeEventTypeLabel(item.eventType) }}
                    </v-chip>
                  </div>
                  <div class="mt-1 text-xs text-slate-500">{{ item.createdAt }}</div>
                </div>
              </div>
            </v-card-text>
          </v-card>

          <v-card flat class="card-shell border-0">
            <div class="flex items-center justify-between gap-3 px-4 pt-4">
              <div class="text-xl font-semibold text-slate-900">实盘风控</div>
              <v-chip :color="realTradeRiskState.riskEnabled ? 'warning' : undefined" variant="outlined" size="small">
                {{ realTradeRiskState.riskEnabled ? '限额开启' : '限额关闭' }}
              </v-chip>
            </div>
            <v-card-text>
              <div class="rounded-2xl bg-slate-50 px-3 py-3">
                <div class="font-medium text-slate-900">{{ formatRealTradeRiskSource(realTradeRiskState.riskConfigSource) }}</div>
                <div class="mt-1 text-xs text-slate-500">
                  有效数量 {{ realTradeRiskState.effectiveMaxOrderQuantity ?? '暂无' }} / 有效金额 {{ realTradeRiskState.effectiveMaxOrderNotional ?? '暂无' }}
                </div>
              </div>
              <div v-if="realTradeRiskEvents.entries.length" class="mt-3 grid gap-2">
                <div
                  v-for="item in realTradeRiskEvents.entries.slice(0, 3)"
                  :key="item.id"
                  class="rounded-2xl bg-slate-50 px-3 py-3"
                >
                  <div class="flex items-center justify-between gap-3">
                    <div class="font-medium text-slate-900">{{ formatRealTradeEventTypeLabel(item.eventType) }} / {{ item.brokerId }}</div>
                    <v-chip :color="resolveRealTradeRiskEventTagType(item.eventType) === 'danger' ? 'error' : resolveRealTradeRiskEventTagType(item.eventType)" variant="outlined" size="small">
                      {{ formatRealTradeEventTypeLabel(item.eventType) }}
                    </v-chip>
                  </div>
                  <div class="mt-1 text-xs text-slate-500">{{ item.reason ?? '暂无' }}</div>
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
              <div class="text-xl font-semibold text-slate-900">实盘硬停止</div>
              <v-chip :color="realTradeHardStops.entries.length ? 'error' : undefined" variant="outlined" size="small">
                {{ formatGenericStatusLabel(realTradeHardStops.entries.length ? 'ACTIVE' : 'CLEAR') }}
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
                  <div class="mt-1 text-xs text-slate-500">{{ formatTradingEnvironment(item.tradingEnvironment) }} / 操作员 {{ item.operatorId }}</div>
                  <div class="mt-1 text-xs text-slate-700">{{ item.reason }}</div>
                </div>
              </div>
              <div v-else class="text-sm text-slate-500">暂无活跃实盘硬停止。</div>
            </v-card-text>
          </v-card>
        </section>
      </v-window-item>
    </v-window>

    <section class="grid gap-5">
      <v-card flat class="card-shell border-0">
        <div class="flex items-center justify-between gap-3 px-4 pt-4">
          <div>
            <div class="text-xl font-semibold text-slate-900">自选 / 行情数据订阅</div>
            <div class="mt-1 text-sm text-slate-500">
              系统当前所有活跃行情订阅及配额使用情况。
            </div>
          </div>
          <div class="flex items-center gap-2">
            <v-chip variant="outlined" size="small">
              {{ marketDataSubscriptions.totalActiveSubscriptions }} 个活跃订阅
            </v-chip>
            <v-btn :loading="isLoadingMarketData" variant="text" color="primary" size="small" @click="loadMarketDataSubscriptions()">
              刷新
            </v-btn>
          </div>
        </div>
        <v-card-text>
          <div class="mb-5 grid gap-3 sm:grid-cols-3">
            <div class="rounded-3xl bg-slate-50 px-4 py-4">
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">总配额使用</div>
              <div class="mt-2 text-2xl font-semibold text-slate-900">
                {{ marketDataSubscriptions.quota.totalUsed }} / {{ marketDataSubscriptions.quota.totalLimit ?? '∞' }}
              </div>
            </div>
            <div class="rounded-3xl bg-slate-50 px-4 py-4">
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">剩余配额</div>
              <div class="mt-2 text-2xl font-semibold text-slate-900">
                {{ marketDataSubscriptions.quota.totalRemaining ?? '∞' }}
              </div>
            </div>
            <div class="rounded-3xl bg-slate-50 px-4 py-4">
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">活跃订阅数</div>
              <div class="mt-2 text-2xl font-semibold text-slate-900">
                {{ marketDataSubscriptions.totalActiveSubscriptions }}
              </div>
            </div>
          </div>

          <div
            v-if="marketDataSubscriptions.quota.byMarket.length"
            class="mb-5 grid gap-2 sm:grid-cols-2 lg:grid-cols-4"
          >
            <div
              v-for="bucket in marketDataSubscriptions.quota.byMarket"
              :key="bucket.market"
              class="rounded-2xl border border-slate-200 bg-white px-4 py-3"
            >
              <div class="flex items-center justify-between gap-2">
                <div class="text-sm font-semibold text-slate-900">{{ formatMarketLabel(bucket.market) }}</div>
                <v-chip variant="outlined" size="small">
                  {{ bucket.used }} / {{ bucket.limit ?? '∞' }}
                </v-chip>
              </div>
              <div class="mt-1 text-xs text-slate-500">
                剩余 {{ bucket.remaining ?? '∞' }}
              </div>
            </div>
          </div>

          <div v-if="marketDataSubscriptions.entries.length" class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            <div
              v-for="entry in marketDataSubscriptions.entries"
              :key="entry.key"
              class="rounded-2xl border border-slate-200 bg-white px-4 py-3"
            >
              <div class="flex items-center justify-between gap-2">
                <div>
                  <div class="text-sm font-semibold text-slate-900">{{ entry.instrumentId }}</div>
                  <div class="mt-1 text-xs text-slate-500">
                    {{ formatMarketDataChannelLabel(entry.channel) }}{{ entry.interval ? ` / ${entry.interval}` : '' }} · 引用数 {{ entry.refCount }}
                  </div>
                </div>
                <v-chip variant="outlined" size="small">{{ formatMarketLabel(entry.market) }}</v-chip>
              </div>
              <div class="mt-2 text-xs text-slate-400">
                订阅时间 {{ formatDateTime(entry.createdAt) }}
              </div>
            </div>
          </div>
          <v-empty-state
            v-else
            text="当前没有活跃的行情订阅。在行情页面添加订阅后会在此处显示。"
          />
        </v-card-text>
      </v-card>
    </section>
  </div>
</template>
