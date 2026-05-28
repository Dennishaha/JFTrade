<script setup lang="ts">
import { computed, onMounted } from "vue";

import type { KlineCandle } from "../charting/kline";
import KlineChart from "../components/KlineChart.vue";
import PageHeader from "../components/PageHeader.vue";
import {
  formatApprovalDecisionLabel,
  formatConnectivityLabel,
  formatExecutionOrderStatusLabel,
  formatGenericStatusLabel,
  formatMarketLabel,
  formatOrderSideLabel,
  formatOrderTypeLabel,
  formatRealTradeOperationLabel,
  formatTradingEnvironment,
} from "../composables/consoleDataFormatting";
import { useConsoleData } from "../composables/useConsoleData";
import { useDocsLink } from "../composables/useDocsLink";

const { resolveDocsHref } = useDocsLink();

const {
  brokerOrders,
  brokerPositions,
  brokerRuntime,
  executionOrders,
  loadMarketDataQuery,
  marketDataCandles,
  marketDataSnapshot,
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
    label: "默认环境",
    value: formatTradingEnvironment(systemStatus.value.defaultTradingEnvironment),
    tone:
      systemStatus.value.defaultTradingEnvironment === "REAL"
        ? "danger"
        : "good",
    hint: systemStatus.value.realTradingEnabled
      ? "实盘执行门禁已开启。"
      : "当前优先使用模拟环境。",
  },
  {
    label: "券商连接",
    value: formatConnectivityLabel(brokerRuntime.value.session.connectivity),
    tone:
      brokerRuntime.value.session.connectivity === "connected"
        ? "good"
        : brokerRuntime.value.session.connectivity === "degraded"
          ? "warn"
          : "danger",
    hint: `已发现 ${brokerRuntime.value.accounts.length} 个账户`,
  },
  {
    label: "订单流",
    value: executionOrders.value.orders.length,
    hint: `券商侧可见 ${brokerOrders.value.orders.length} 个订单`,
  },
  {
    label: "风控门禁",
    value: realTradeHardStops.value.entries.length,
    tone: realTradeHardStops.value.entries.length ? "warn" : "good",
    hint: realTradeKillSwitchState.value.killSwitchActive
      ? "熔断开关当前已激活。"
      : "当前没有活跃硬停止范围。",
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
    value: `${realTradeHardStops.value.entries.length} 个范围`,
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
    detail: "查看构建、部署、回滚与冒烟检查流程。",
    href: resolveDocsHref("developer/deployment-and-release.html"),
  },
];

onMounted(() => {
  void loadMarketDataQuery();
});

const priceInstrumentId = computed(
  () => marketDataSnapshot.value?.request.instrumentId ?? "—",
);

const isUSMarket = computed(
  () => marketDataSnapshot.value?.request.market?.toUpperCase() === "US",
);

const snapshotSession = computed(
  () => (watchlistSnapshot.value?.session as string) ?? null,
);

const isInRegularSession = computed(
  () => snapshotSession.value === "regular",
);

const mainDisplayLabel = computed(() => {
  if (!isUSMarket.value) return "最新价";
  return isInRegularSession.value ? "最新价" : "最近盘中收盘";
});

// 大字展示逻辑：盘中展示实时价，非盘中展示最近盘中收盘价
const mainDisplayPrice = computed((): number | null => {
  const snap = watchlistSnapshot.value;
  if (!snap) return null;
  if (!isUSMarket.value) return snap.price;
  if (isInRegularSession.value) return snap.price;
  return snap.previousClosePrice ?? snap.price;
});

// priceChangePercent: 语义随时段变化，配合 mainDisplayLabel/mainDisplayPrice 展示
// ▸ 盘中：实时价 vs 昨收
// ▸ 盘外（最近盘中收盘）：盘中收盘 vs 上个交易日收盘（lastClosePrice）
const priceChangePercent = computed((): number | null => {
  const snap = watchlistSnapshot.value;
  if (!snap) return null;
  if (!isUSMarket.value || isInRegularSession.value) {
    if (snap.previousClosePrice == null || snap.previousClosePrice === 0) return null;
    return ((snap.price - snap.previousClosePrice) / snap.previousClosePrice) * 100;
  }
  // 扩展时段：最近盘中收盘 vs 上个交易日收盘
  const close = snap.previousClosePrice;
  const lastClose = snap.lastClosePrice;
  if (close == null || lastClose == null || lastClose === 0 || close === lastClose) return null;
  return ((close - lastClose) / lastClose) * 100;
});

// extendedCardChangePercent: 延伸时段卡片专用——实时延伸价格 vs 最近盘中收盘
const extendedCardChangePercent = computed((): number | null => {
  const snap = watchlistSnapshot.value;
  if (!snap || snap.previousClosePrice == null || snap.previousClosePrice === 0) return null;
  return ((snap.price - snap.previousClosePrice) / snap.previousClosePrice) * 100;
});

// Extended session display helpers.
// snap.price is refreshed on every live ticker event; extended.*.price only
// updates on HTTP snapshot fetches (~60 s). For the currently-active extended
// session, prefer snap.price so the UI reflects live market data.
const extendedPreMarketDisplay = computed(() => {
  const snap = watchlistSnapshot.value;
  if (!snap?.extended?.preMarket) return null;
  const livePrice = snap.price > 0 ? snap.price : null;
  const isActive = snapshotSession.value === "pre";
  const price = (isActive ? livePrice : null) ?? snap.extended.preMarket.price;
  if (price == null) return null;
  return {
    price,
    changeRate: isActive
      ? (extendedCardChangePercent.value ?? snap.extended.preMarket.changeRate ?? null)
      : (snap.extended.preMarket.changeRate ?? null),
  };
});

const extendedAfterMarketDisplay = computed(() => {
  const snap = watchlistSnapshot.value;
  if (!snap?.extended?.afterMarket) return null;
  const livePrice = snap.price > 0 ? snap.price : null;
  // after-market trading is only active when session === "after"; during
  // "overnight" that window has already closed so keep the snapshot price.
  const isActive = snapshotSession.value === "after";
  const price = (isActive ? livePrice : null) ?? snap.extended.afterMarket.price;
  if (price == null) return null;
  return {
    price,
    changeRate: isActive
      ? (extendedCardChangePercent.value ?? snap.extended.afterMarket.changeRate ?? null)
      : (snap.extended.afterMarket.changeRate ?? null),
  };
});

const extendedOvernightDisplay = computed(() => {
  const snap = watchlistSnapshot.value;
  if (!snap?.extended?.overnight) return null;
  const livePrice = snap.price > 0 ? snap.price : null;
  const price = livePrice ?? snap.extended.overnight.price;
  if (price == null) return null;
  return {
    price,
    changeRate: extendedCardChangePercent.value ?? snap.extended.overnight.changeRate ?? null,
  };
});

function sessionLabel(session: string | null): string {
  if (session === "regular") return "盘中";
  if (session === "pre") return "盘前";
  if (session === "after") return "盘后";
  if (session === "overnight") return "夜盘";
  if (session === "closed") return "已收盘";
  return session?.toUpperCase() ?? "";
}
</script>

<template>
  <div class="grid gap-6">
    <PageHeader
      eyebrow="交易工作台"
      title="工作台概览"
      description="把系统运行态、风险门禁、订单流、市场快照和文档入口收敛到一个主屏，减少在多个分页之间来回切换。该页优先展示当前状态、异常信号和下一步操作入口。"
      :stats="overviewStats"
    />

    <section class="grid gap-5 xl:grid-cols-[1.35fr_0.9fr]">
      <v-card flat class="card-shell border-0">
        <div class="flex items-center justify-between gap-3 px-4 pt-4">
          <div>
            <div class="text-xl font-semibold text-slate-900">行情焦点</div>
            <div class="mt-1 text-sm text-slate-500">
              当前焦点标的行情摘要，包含最新价、涨跌幅及美股盘前 / 盘后数据。
            </div>
          </div>
        </div>
        <v-card-text>
          <div class="grid gap-4 lg:grid-cols-[0.92fr_1.08fr]">
            <div class="rounded-3xl border border-slate-200 bg-white px-5 py-5">
              <!-- Header: instrument ID + live status -->
              <div class="flex items-center justify-between gap-3">
                <div>
                  <div class="text-xs uppercase tracking-[0.24em] text-slate-500">
                    焦点标的
                  </div>
                  <div class="mt-2 text-2xl font-semibold text-slate-900">
                    {{ priceInstrumentId }}
                  </div>
                </div>
                <v-chip :color="watchlistSnapshot ? 'success' : undefined" variant="outlined" size="small">
                  {{ watchlistSnapshot ? formatGenericStatusLabel("LIVE") : "暂无数据" }}
                </v-chip>
              </div>

              <div v-if="watchlistSnapshot" class="mt-5">
                <!-- Big price + change % -->
                <div class="flex items-end gap-3">
                  <div>
                    <div class="text-[11px] uppercase tracking-[0.2em] text-slate-400">
                      {{ mainDisplayLabel }}
                    </div>
                    <div class="mt-1 text-4xl font-bold leading-none text-slate-900">
                      {{ mainDisplayPrice != null ? mainDisplayPrice.toFixed(3) : '—' }}
                    </div>
                  </div>
                  <div
                    v-if="priceChangePercent != null"
                    class="pb-0.5 text-lg font-semibold"
                    :class="priceChangePercent >= 0 ? 'tv-up' : 'tv-down'"
                  >
                    {{ priceChangePercent >= 0 ? '+' : '' }}{{ priceChangePercent.toFixed(2) }}%
                  </div>
                </div>

                <!-- Session indicator (US only) -->
                <div v-if="isUSMarket && snapshotSession" class="mt-3">
                  <v-chip
                    :color="isInRegularSession ? 'success' : 'default'"
                    variant="outlined"
                    size="small"
                  >
                    {{ sessionLabel(snapshotSession) }}
                  </v-chip>
                </div>

                <!-- US extended hours -->
                <template v-if="isUSMarket">
                  <!-- Pre-market -->
                  <div
                    v-if="snapshotSession === 'pre' && extendedPreMarketDisplay != null"
                    class="mt-4 rounded-2xl border border-sky-100 bg-sky-50 px-4 py-3"
                  >
                    <div class="text-[11px] uppercase tracking-[0.2em] text-sky-600">盘前价格</div>
                    <div class="mt-2 flex items-end gap-2">
                      <div class="text-2xl font-semibold text-slate-900">
                        {{ extendedPreMarketDisplay.price.toFixed(3) }}
                      </div>
                      <div
                        v-if="extendedPreMarketDisplay.changeRate != null"
                        class="pb-0.5 text-sm font-semibold"
                        :class="extendedPreMarketDisplay.changeRate >= 0 ? 'tv-up' : 'tv-down'"
                      >
                        {{ extendedPreMarketDisplay.changeRate >= 0 ? '+' : '' }}{{ extendedPreMarketDisplay.changeRate.toFixed(2) }}%
                      </div>
                    </div>
                  </div>

                  <!-- After-market -->
                  <div
                    v-if="(snapshotSession === 'after' || snapshotSession === 'overnight') && extendedAfterMarketDisplay != null"
                    class="mt-4 rounded-2xl border border-amber-100 bg-amber-50 px-4 py-3"
                  >
                    <div class="text-[11px] uppercase tracking-[0.2em] text-amber-600">盘后价格</div>
                    <div class="mt-2 flex items-end gap-2">
                      <div class="text-2xl font-semibold text-slate-900">
                        {{ extendedAfterMarketDisplay.price.toFixed(3) }}
                      </div>
                      <div
                        v-if="extendedAfterMarketDisplay.changeRate != null"
                        class="pb-0.5 text-sm font-semibold"
                        :class="extendedAfterMarketDisplay.changeRate >= 0 ? 'tv-up' : 'tv-down'"
                      >
                        {{ extendedAfterMarketDisplay.changeRate >= 0 ? '+' : '' }}{{ extendedAfterMarketDisplay.changeRate.toFixed(2) }}%
                      </div>
                    </div>
                  </div>

                  <!-- Overnight -->
                  <div
                    v-if="snapshotSession === 'overnight' && extendedOvernightDisplay != null"
                    class="mt-3 rounded-2xl border border-violet-100 bg-violet-50 px-4 py-3"
                  >
                    <div class="text-[11px] uppercase tracking-[0.2em] text-violet-600">夜盘价格</div>
                    <div class="mt-2 flex items-end gap-2">
                      <div class="text-2xl font-semibold text-slate-900">
                        {{ extendedOvernightDisplay.price.toFixed(3) }}
                      </div>
                      <div
                        v-if="extendedOvernightDisplay.changeRate != null"
                        class="pb-0.5 text-sm font-semibold"
                        :class="extendedOvernightDisplay.changeRate >= 0 ? 'tv-up' : 'tv-down'"
                      >
                        {{ extendedOvernightDisplay.changeRate >= 0 ? '+' : '' }}{{ extendedOvernightDisplay.changeRate.toFixed(2) }}%
                      </div>
                    </div>
                  </div>
                </template>
              </div>
              <v-empty-state v-else text="当前未命中快照缓存。启动行情数据订阅后价格信息将在此处自动更新。" class="mt-5" />
            </div>

            <div class="rounded-3xl border border-slate-200 bg-slate-950 px-5 py-5 text-slate-100">
              <div class="flex items-center justify-between gap-3">
                <div>
                  <div class="text-xs uppercase tracking-[0.24em] text-slate-400">
                    迷你K线
                  </div>
                  <div class="mt-2 text-lg font-semibold">
                    近期K线
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
                  empty-text="还没有可视化 K 线缓存；行情页查询或 OpenD 拉取成功后会同步反映到这里。"
                />
              </div>
            </div>
          </div>
        </v-card-text>
      </v-card>

      <v-card flat class="card-shell border-0">
        <div class="flex items-center justify-between gap-3 px-4 pt-4">
          <div class="text-xl font-semibold text-slate-900">风控监控</div>
          <v-chip
            :color="realTradeKillSwitchState.killSwitchActive ? 'error' : 'success'"
            variant="outlined"
            size="small"
          >
            {{
              realTradeKillSwitchState.killSwitchActive
                ? "熔断中"
                : formatGenericStatusLabel("CLEAR")
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
                实盘有效风控
              </div>
              <div class="mt-2">
                数量
                <span class="font-semibold text-slate-900">
                  {{ realTradeRiskState.effectiveMaxOrderQuantity ?? "暂无" }}
                </span>
                /
                金额
                <span class="font-semibold text-slate-900">
                  {{ realTradeRiskState.effectiveMaxOrderNotional ?? "暂无" }}
                </span>
              </div>
            </div>

            <div
              v-if="approvalEntries.length"
              class="grid gap-2"
            >
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">
                最近审批
              </div>
              <div
                v-for="entry in approvalEntries"
                :key="entry.id"
                class="rounded-2xl border border-slate-200 bg-white px-4 py-3"
              >
                <div class="flex items-center justify-between gap-3">
                  <div class="text-sm font-semibold text-slate-900">
                    {{ formatRealTradeOperationLabel(entry.operation) }} / {{ entry.brokerId }}
                  </div>
                  <v-chip
                    :color="entry.decision === 'approved' ? 'success' : 'error'"
                    variant="outlined"
                    size="small"
                  >
                    {{ formatApprovalDecisionLabel(entry.decision) }}
                  </v-chip>
                </div>
                <div class="mt-1 text-xs text-slate-500">
                  {{ entry.accountId ?? "暂无" }} / {{ formatTradingEnvironment(entry.tradingEnvironment) }}
                </div>
              </div>
            </div>
            <v-empty-state
              v-else
              text="还没有实盘审批事件；风控页面会展示更完整的控制面时间线。"
            />
          </div>
        </v-card-text>
      </v-card>
    </section>

    <section class="grid gap-5 xl:grid-cols-[1.02fr_0.98fr]">
      <v-card flat class="card-shell border-0">
        <div class="flex items-center justify-between gap-3 px-4 pt-4">
          <div class="text-xl font-semibold text-slate-900">订单执行概览</div>
          <v-chip variant="outlined" size="small">{{ recentExecutionOrders.length }}</v-chip>
        </div>
        <v-card-text>
          <div class="grid gap-4 lg:grid-cols-2">
            <div class="grid gap-3">
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">
                内部订单
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
                    <v-chip variant="outlined" size="small">{{ formatExecutionOrderStatusLabel(order.status) }}</v-chip>
                  </div>
                  <div class="mt-4 grid gap-3 sm:grid-cols-4">
                    <div class="rounded-2xl bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.18em] text-slate-500">
                        委托数量
                      </div>
                      <div class="mt-2 text-lg font-semibold text-slate-900">
                        {{ order.requestedQuantity }}
                      </div>
                    </div>
                    <div class="rounded-2xl bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.18em] text-slate-500">
                        成交数量
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
                text="当前还没有执行流水数据。"
              />
            </div>

            <div class="grid gap-3">
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">
                券商订单
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
                        {{ formatOrderSideLabel(order.side) }} / {{ formatOrderTypeLabel(order.orderType) }}
                      </div>
                    </div>
                    <v-chip variant="outlined" size="small">{{ formatExecutionOrderStatusLabel(order.status) }}</v-chip>
                  </div>
                  <div class="mt-4 text-xs text-slate-500">
                    提交时间 {{ order.submittedAt || "暂无" }}
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
            <div class="text-xl font-semibold text-slate-900">持仓概览</div>
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
                      {{ position.accountId }} / {{ formatMarketLabel(position.market) }}
                    </div>
                  </div>
                  <div class="text-right">
                    <div class="text-lg font-semibold text-slate-900">
                      {{ position.quantity }}
                    </div>
                    <div class="text-xs text-slate-500">数量</div>
                  </div>
                </div>
              </div>
              <div
                v-if="portfolioCashReconciliation.balances.length"
                class="rounded-3xl bg-slate-50 px-4 py-4 text-sm text-slate-600"
              >
                现金差额
                <span class="font-semibold text-slate-900">
                  {{ portfolioCashReconciliation.balances[0]?.cashDelta ?? "暂无" }}
                </span>
                /
                连接状态
                <span class="font-semibold text-slate-900">
                  {{ formatConnectivityLabel(portfolioCashReconciliation.connectivity) }}
                </span>
              </div>
              <v-empty-state
                v-else
                text="持仓投影和对账结果稍后会在这里形成组合脉冲。"
              />
            </div>
          </v-card-text>
        </v-card>

        <v-card flat class="card-shell border-0">
          <div class="flex items-center justify-between gap-3 px-4 pt-4">
            <div class="text-xl font-semibold text-slate-900">文档与运维</div>
            <v-chip variant="outlined" size="small">准备度</v-chip>
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
                API / 持久化 / 活跃策略：
                <span class="font-semibold text-slate-900">
                  {{ formatGenericStatusLabel(systemStatus.persistence.status) }}
                </span>
                /
                <span class="font-semibold text-slate-900">
                  {{ systemStatus.strategyRuntime.activeStrategies }}
                </span>
                个策略
              </div>
            </div>
          </v-card-text>
        </v-card>
      </div>
    </section>

    <section class="grid gap-5 xl:grid-cols-[1.1fr_0.9fr]">
      <v-card flat class="card-shell border-0">
        <div class="flex items-center justify-between gap-3 px-4 pt-4">
          <div class="text-xl font-semibold text-slate-900">审计时间线</div>
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
          <div class="text-xl font-semibold text-slate-900">指令台账</div>
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
                  {{ formatRealTradeOperationLabel(entry.operation) }} / {{ entry.brokerId }}
                </div>
                <v-chip variant="outlined" size="small">
                  {{ formatGenericStatusLabel(entry.completedAt ? "COMPLETED" : "PENDING") }}
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
            text="最近还没有执行指令台账事件。"
          />
        </v-card-text>
      </v-card>
    </section>
  </div>
</template>
