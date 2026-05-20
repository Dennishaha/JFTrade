<script setup lang="ts">
import { computed, onMounted } from "vue";

import type { KlineCandle } from "../charting/kline";
import KlineChart from "../components/KlineChart.vue";
import PageHeader from "../components/PageHeader.vue";
import { useConsoleData } from "../composables/useConsoleData";
import { useDocsLink } from "../composables/useDocsLink";

const { resolveDocsHref } = useDocsLink();

const {
  brokerOrders,
  brokerPositions,
  brokerRuntime,
  executionOrders,
  loadMarketDataQuery,
  loadMarketDataSubscriptions,
  marketDataCandles,
  marketDataSnapshot,
  marketDataSubscriptions,
  portfolioCashReconciliation,
  portfolioPositions,
  realTradeApprovals,
  realTradeHardStops,
  realTradeKillSwitchState,
  realTradeRiskState,
  storageOverview,
  systemStatus,
} = useConsoleData();

const overviewStats = computed(() => [
  {
    label: "Default Runtime",
    value: systemStatus.value.defaultTradingEnvironment,
    tone:
      systemStatus.value.defaultTradingEnvironment === "REAL"
        ? "danger"
        : "good",
    hint: systemStatus.value.realTradingEnabled
      ? "REAL execution gate enabled."
      : "Simulation-first workspace.",
  },
  {
    label: "Broker Pulse",
    value: brokerRuntime.value.session.connectivity.toUpperCase(),
    tone:
      brokerRuntime.value.session.connectivity === "connected"
        ? "good"
        : brokerRuntime.value.session.connectivity === "degraded"
          ? "warn"
          : "danger",
    hint: `${brokerRuntime.value.accounts.length} account(s) discovered`,
  },
  {
    label: "Open Orders",
    value: executionOrders.value.orders.length,
    hint: `${brokerOrders.value.orders.length} broker-side order(s) visible`,
  },
  {
    label: "Risk Gates",
    value: realTradeHardStops.value.entries.length,
    tone: realTradeHardStops.value.entries.length ? "warn" : "good",
    hint: realTradeKillSwitchState.value.killSwitchActive
      ? "Kill switch currently active."
      : "No hard-stop scope is active.",
  },
]);

const watchlistSnapshot = computed(
  () => marketDataSnapshot.value?.snapshot ?? null,
);
const overviewCandles = computed<KlineCandle[]>(
  () => marketDataCandles.value?.candles ?? [],
);

const recentExecutionOrders = computed(() =>
  executionOrders.value.orders.slice(0, 5),
);
const recentBrokerOrders = computed(() =>
  brokerOrders.value.orders.slice(0, 5),
);
const recentAuditLogs = computed(() =>
  storageOverview.value.recentAuditLogs.slice(0, 4),
);
const recentExecutionCommands = computed(() =>
  storageOverview.value.recentExecutionCommands.slice(0, 4),
);
const approvalEntries = computed(() =>
  realTradeApprovals.value.entries.slice(0, 4),
);
const projectedPositions = computed(() =>
  portfolioPositions.value.positions.slice(0, 5),
);

const riskSummary = computed(() => [
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
    value: `${realTradeHardStops.value.entries.length} scope`,
    tone: realTradeHardStops.value.entries.length ? "warn" : "good",
  },
]);

const docsCards = [
  {
    title: "控制台使用指南",
    detail: "梳理启动方式、页面入口和模拟交易最短路径。",
    href: resolveDocsHref("user/getting-started.html"),
  },
  {
    title: "本地开发与命令",
    detail: "查工作区命令、环境变量和接口入口。",
    href: resolveDocsHref("developer/local-development.html"),
  },
  {
    title: "开发部署与发布",
    detail: "查看 build、deploy、rollback 与 smoke 流程。",
    href: resolveDocsHref("developer/deployment-and-release.html"),
  },
];

onMounted(() => {
  void Promise.all([loadMarketDataSubscriptions(), loadMarketDataQuery()]);
});
</script>

<template>
  <div class="grid gap-6">
    <PageHeader
      eyebrow="Trading workstation"
      title="Overview / 工作台概览"
      description="把系统运行态、风险门禁、订单流、市场快照和文档入口收敛到一个主屏，减少在多个分页之间来回切换。该页优先展示当下状态、异常信号和下一步操作入口。"
      :stats="overviewStats"
    />

    <section class="grid gap-5 xl:grid-cols-[1.35fr_0.9fr]">
      <v-card flat class="card-shell border-0">
        <div class="flex items-center justify-between gap-3 px-4 pt-4">
          <div>
            <div class="text-xl font-semibold text-slate-900">Market Spotlight</div>
            <div class="mt-1 text-sm text-slate-500">
              TradingView 风格的紧凑行情摘要，优先展示当前默认查询的快照和最近 K 线。
            </div>
          </div>
          <v-chip variant="outlined" size="small">
            {{ marketDataSubscriptions.totalActiveSubscriptions }} active
          </v-chip>
        </div>
        <v-card-text>
          <div class="grid gap-4 lg:grid-cols-[0.92fr_1.08fr]">
            <div class="rounded-3xl border border-slate-200 bg-white px-5 py-5">
              <div class="flex items-center justify-between gap-3">
                <div>
                  <div class="text-xs uppercase tracking-[0.24em] text-slate-500">
                    Watchlist focus
                  </div>
                  <div class="mt-2 text-2xl font-semibold text-slate-900">
                    {{
                      marketDataSnapshot?.request.instrumentId ??
                      "HK.00700"
                    }}
                  </div>
                </div>
                <v-chip :color="watchlistSnapshot ? 'success' : undefined" variant="outlined" size="small">
                  {{ watchlistSnapshot ? "LIVE SNAPSHOT" : "CACHE EMPTY" }}
                </v-chip>
              </div>

              <div
                v-if="watchlistSnapshot"
                class="mt-5 grid gap-3 sm:grid-cols-2"
              >
                <div class="rounded-2xl bg-slate-50 px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">
                    Last Price
                  </div>
                  <div class="mt-2 text-3xl font-semibold text-slate-900">
                    {{ watchlistSnapshot.price }}
                  </div>
                </div>
                <div class="rounded-2xl bg-slate-50 px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">
                    Bid / Ask
                  </div>
                  <div class="mt-2 text-xl font-semibold text-slate-900">
                    {{ watchlistSnapshot.bid }} / {{ watchlistSnapshot.ask }}
                  </div>
                </div>
                <div class="rounded-2xl bg-slate-50 px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">
                    Volume
                  </div>
                  <div class="mt-2 text-xl font-semibold text-slate-900">
                    {{ watchlistSnapshot.volume }}
                  </div>
                </div>
                <div class="rounded-2xl bg-slate-50 px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">
                    Subscription Quota
                  </div>
                  <div class="mt-2 text-xl font-semibold text-slate-900">
                    {{
                      marketDataSubscriptions.quota.totalUsed
                    }}
                    /
                    {{
                      marketDataSubscriptions.quota.totalLimit ?? "∞"
                    }}
                  </div>
                </div>
              </div>
              <v-empty-state v-else text="当前未命中快照缓存。市场数据 provider 写入后会自动在这里形成主屏 watchlist。" class="mt-5" />
            </div>

            <div class="rounded-3xl border border-slate-200 bg-slate-950 px-5 py-5 text-slate-100">
              <div class="flex items-center justify-between gap-3">
                <div>
                  <div class="text-xs uppercase tracking-[0.24em] text-slate-400">
                    Mini chart
                  </div>
                  <div class="mt-2 text-lg font-semibold">
                    Recent Candles
                  </div>
                </div>
                <div class="text-xs uppercase tracking-[0.2em] text-slate-400">
                  {{ marketDataCandles?.request.period ?? "1m" }}
                </div>
              </div>

              <div class="mt-6 rounded-3xl border border-white/10 bg-white/5 p-2">
                <KlineChart
                  :candles="overviewCandles"
                  :min-height="220"
                  empty-text="还没有可视化 K 线缓存；Market 页查询或 OpenD 拉取成功后会同步反映到这里。"
                />
              </div>
            </div>
          </div>
        </v-card-text>
      </v-card>

      <v-card flat class="card-shell border-0">
        <div class="flex items-center justify-between gap-3 px-4 pt-4">
          <div class="text-xl font-semibold text-slate-900">Risk Monitor</div>
          <v-chip
            :color="realTradeKillSwitchState.killSwitchActive ? 'error' : 'success'"
            variant="outlined"
            size="small"
          >
            {{
              realTradeKillSwitchState.killSwitchActive
                ? "KILL SWITCH"
                : "CLEAR"
            }}
          </v-chip>
        </div>
        <v-card-text>
          <div class="grid gap-3 sm:grid-cols-3">
            <div
              v-for="item in riskSummary"
              :key="item.label"
              class="rounded-2xl border border-slate-200 bg-white px-4 py-4"
            >
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">
                {{ item.label }}
              </div>
              <div
                class="mt-2 text-xl font-semibold"
                :class="item.tone === 'danger'
                  ? 'text-rose-600'
                  : item.tone === 'warn'
                    ? 'text-amber-600'
                    : 'text-emerald-600'"
              >
                {{ item.value }}
              </div>
            </div>
          </div>

          <div class="mt-4 grid gap-3">
            <div class="rounded-3xl bg-slate-50 px-4 py-4 text-sm text-slate-600">
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">
                Effective REAL guardrail
              </div>
              <div class="mt-2">
                Qty
                <span class="font-semibold text-slate-900">
                  {{ realTradeRiskState.effectiveMaxOrderQuantity ?? "N/A" }}
                </span>
                /
                Notional
                <span class="font-semibold text-slate-900">
                  {{ realTradeRiskState.effectiveMaxOrderNotional ?? "N/A" }}
                </span>
              </div>
            </div>

            <div
              v-if="approvalEntries.length"
              class="grid gap-2"
            >
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">
                Recent approvals
              </div>
              <div
                v-for="entry in approvalEntries"
                :key="entry.id"
                class="rounded-2xl border border-slate-200 bg-white px-4 py-3"
              >
                <div class="flex items-center justify-between gap-3">
                  <div class="text-sm font-semibold text-slate-900">
                    {{ entry.operation }} / {{ entry.brokerId }}
                  </div>
                  <v-chip
                    :color="entry.decision === 'approved' ? 'success' : 'error'"
                    variant="outlined"
                    size="small"
                  >
                    {{ entry.decision.toUpperCase() }}
                  </v-chip>
                </div>
                <div class="mt-1 text-xs text-slate-500">
                  {{ entry.accountId ?? "N/A" }} / {{ entry.tradingEnvironment ?? "N/A" }}
                </div>
              </div>
            </div>
            <v-empty-state
              v-else
              text="还没有 REAL 审批事件；风险页面会展示更完整的 control-plane timeline。"
            />
          </div>
        </v-card-text>
      </v-card>
    </section>

    <section class="grid gap-5 xl:grid-cols-[1.02fr_0.98fr]">
      <v-card flat class="card-shell border-0">
        <div class="flex items-center justify-between gap-3 px-4 pt-4">
          <div class="text-xl font-semibold text-slate-900">Execution / Order Blotter</div>
          <v-chip variant="outlined" size="small">{{ recentExecutionOrders.length }}</v-chip>
        </div>
        <v-card-text>
          <div class="grid gap-4 lg:grid-cols-2">
            <div class="grid gap-3">
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">
                Execution Orders
              </div>
              <div
                v-if="recentExecutionOrders.length"
                class="grid gap-3"
              >
                <div
                  v-for="order in recentExecutionOrders"
                  :key="order.internalOrderId"
                  class="rounded-3xl border border-slate-200 bg-white px-4 py-4"
                >
                  <div class="flex items-center justify-between gap-3">
                    <div>
                      <div class="text-base font-semibold text-slate-900">
                        {{ order.symbol }}
                      </div>
                      <div class="mt-1 text-xs text-slate-500">
                        {{ order.internalOrderId }}
                      </div>
                    </div>
                    <v-chip variant="outlined" size="small">{{ order.status }}</v-chip>
                  </div>
                  <div class="mt-4 grid gap-3 sm:grid-cols-2">
                    <div class="rounded-2xl bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.18em] text-slate-500">
                        Requested
                      </div>
                      <div class="mt-2 text-lg font-semibold text-slate-900">
                        {{ order.requestedQuantity }}
                      </div>
                    </div>
                    <div class="rounded-2xl bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.18em] text-slate-500">
                        Filled
                      </div>
                      <div class="mt-2 text-lg font-semibold text-slate-900">
                        {{ order.filledQuantity ?? 0 }}
                      </div>
                    </div>
                  </div>
                </div>
              </div>
              <v-empty-state
                v-else
                text="当前还没有 execution blotter 数据。"
              />
            </div>

            <div class="grid gap-3">
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">
                Broker Orders
              </div>
              <div
                v-if="recentBrokerOrders.length"
                class="grid gap-3"
              >
                <div
                  v-for="order in recentBrokerOrders"
                  :key="order.brokerOrderId"
                  class="rounded-3xl border border-slate-200 bg-white px-4 py-4"
                >
                  <div class="flex items-center justify-between gap-3">
                    <div>
                      <div class="text-base font-semibold text-slate-900">
                        {{ order.symbol }}
                      </div>
                      <div class="mt-1 text-xs text-slate-500">
                        {{ order.side }} / {{ order.orderType }}
                      </div>
                    </div>
                    <v-chip variant="outlined" size="small">{{ order.status }}</v-chip>
                  </div>
                  <div class="mt-4 text-xs text-slate-500">
                    Submitted {{ order.submittedAt || "N/A" }}
                  </div>
                </div>
              </div>
              <v-empty-state
                v-else
                text="券商订单回读暂为空。"
              />
            </div>
          </div>
        </v-card-text>
      </v-card>

      <div class="grid gap-5">
        <v-card flat class="card-shell border-0">
          <div class="flex items-center justify-between gap-3 px-4 pt-4">
            <div class="text-xl font-semibold text-slate-900">Portfolio Pulse</div>
            <v-chip variant="outlined" size="small">{{ projectedPositions.length }}</v-chip>
          </div>
          <v-card-text>
            <div class="grid gap-3">
              <div
                v-if="projectedPositions.length"
                v-for="position in projectedPositions"
                :key="`${position.accountId}-${position.symbol}`"
                class="rounded-3xl border border-slate-200 bg-white px-4 py-4"
              >
                <div class="flex items-center justify-between gap-3">
                  <div>
                    <div class="text-base font-semibold text-slate-900">
                      {{ position.symbol }}
                    </div>
                    <div class="mt-1 text-xs text-slate-500">
                      {{ position.accountId }} / {{ position.market }}
                    </div>
                  </div>
                  <div class="text-right">
                    <div class="text-lg font-semibold text-slate-900">
                      {{ position.quantity }}
                    </div>
                    <div class="text-xs text-slate-500">qty</div>
                  </div>
                </div>
              </div>
              <div
                v-if="portfolioCashReconciliation.balances.length"
                class="rounded-3xl bg-slate-50 px-4 py-4 text-sm text-slate-600"
              >
                Cash delta
                <span class="font-semibold text-slate-900">
                  {{ portfolioCashReconciliation.balances[0]?.cashDelta ?? "N/A" }}
                </span>
                /
                Connectivity
                <span class="font-semibold text-slate-900">
                  {{ portfolioCashReconciliation.connectivity.toUpperCase() }}
                </span>
              </div>
              <v-empty-state
                v-else
                text="Portfolio projection 和对账结果稍后会在这里形成组合脉冲。"
              />
            </div>
          </v-card-text>
        </v-card>

        <v-card flat class="card-shell border-0">
          <div class="flex items-center justify-between gap-3 px-4 pt-4">
            <div class="text-xl font-semibold text-slate-900">Docs &amp; Operations</div>
            <v-chip variant="outlined" size="small">readiness</v-chip>
          </div>
          <v-card-text>
            <div class="grid gap-3">
              <a
                v-for="card in docsCards"
                :key="card.href"
                :href="card.href"
                class="rounded-3xl border border-slate-200 bg-white px-4 py-4 transition hover:border-cyan-300 hover:bg-cyan-50"
              >
                <div class="text-sm font-semibold uppercase tracking-[0.2em] text-slate-900">
                  {{ card.title }}
                </div>
                <div class="mt-2 text-sm leading-6 text-slate-500">
                  {{ card.detail }}
                </div>
              </a>

              <div class="rounded-3xl bg-slate-50 px-4 py-4 text-sm text-slate-600">
                API / persistence / strategy active:
                <span class="font-semibold text-slate-900">
                  {{ systemStatus.persistence.status }}
                </span>
                /
                <span class="font-semibold text-slate-900">
                  {{ systemStatus.strategyRuntime.activeStrategies }}
                </span>
                strategies
              </div>
            </div>
          </v-card-text>
        </v-card>
      </div>
    </section>

    <section class="grid gap-5 xl:grid-cols-[1.1fr_0.9fr]">
      <v-card flat class="card-shell border-0">
        <div class="flex items-center justify-between gap-3 px-4 pt-4">
          <div class="text-xl font-semibold text-slate-900">Audit Timeline</div>
          <v-chip variant="outlined" size="small">{{ recentAuditLogs.length }}</v-chip>
        </div>
        <v-card-text>
          <div
            v-if="recentAuditLogs.length"
            class="grid gap-3 md:grid-cols-2"
          >
            <div
              v-for="entry in recentAuditLogs"
              :key="entry.id"
              class="rounded-3xl border border-slate-200 bg-white px-4 py-4"
            >
              <div class="text-sm font-semibold text-slate-900">
                {{ entry.action }}
              </div>
              <div class="mt-2 text-xs text-slate-500">
                {{ entry.targetType }} / {{ entry.targetId }}
              </div>
              <div class="mt-1 text-xs text-slate-400">
                {{ entry.createdAt }}
              </div>
            </div>
          </div>
          <v-empty-state
            v-else
            text="暂无最近审计记录。"
          />
        </v-card-text>
      </v-card>

      <v-card flat class="card-shell border-0">
        <div class="flex items-center justify-between gap-3 px-4 pt-4">
          <div class="text-xl font-semibold text-slate-900">Command Ledger</div>
          <v-chip variant="outlined" size="small">{{ recentExecutionCommands.length }}</v-chip>
        </div>
        <v-card-text>
          <div
            v-if="recentExecutionCommands.length"
            class="grid gap-3"
          >
            <div
              v-for="entry in recentExecutionCommands"
              :key="entry.id"
              class="rounded-3xl border border-slate-200 bg-white px-4 py-4"
            >
              <div class="flex items-center justify-between gap-3">
                <div class="text-sm font-semibold text-slate-900">
                  {{ entry.operation }} / {{ entry.brokerId }}
                </div>
                <v-chip variant="outlined" size="small">
                  {{ entry.completedAt ? "DONE" : "PENDING" }}
                </v-chip>
              </div>
              <div class="mt-2 break-all text-xs text-slate-500">
                {{ entry.idempotencyKey }}
              </div>
              <div class="mt-1 text-xs text-slate-400">
                {{ entry.actorType }} / {{ entry.actorId }}
              </div>
            </div>
          </div>
          <v-empty-state
            v-else
            text="最近还没有 execution command ledger 事件。"
          />
        </v-card-text>
      </v-card>
    </section>
  </div>
</template>
