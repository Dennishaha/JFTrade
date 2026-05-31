<script setup lang="ts">
import { computed, ref, watch } from "vue";

import type { ExecutionOrdersResponse } from "@jftrade/ui-contracts";

import PageHeader from "../components/PageHeader.vue";
import SectionHeader from "../components/SectionHeader.vue";
import { fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";
import {
  formatAccountTypeLabel,
  formatBooleanLabel,
  formatConnectivityLabel,
  formatDateTime,
  formatExecutionEventTypeLabel,
  formatExecutionOrderStatusLabel,
  formatMarketLabel,
  formatOrderSideLabel,
  formatOrderTypeLabel,
  formatTradingEnvironment,
  isFinalExecutionOrderStatus,
} from "../composables/consoleDataFormatting";
import { useConsoleData } from "../composables/useConsoleData";
import { useNotifications } from "../composables/useNotifications";

const {
  brokerCashFlows,
  brokerFills,
  brokerFunds,
  brokerMarginRatios,
  brokerOrderFees,
  brokerPositions,
  brokerRuntime,
  executionEventsError,
  executionOrderEvents,
  executionOrders,
  isLoadingBrokerFills,
  isLoadingBrokerMarginRatios,
  isLoadingExecutionEvents,
  isLoadingOrderFees,
  loadExecutionOrderDetails,
  orderFeesError,
  portfolioCashBalances,
  portfolioPositions,
  resolveBrokerReadFeatureQueryRequirements,
  selectedBrokerAccount,
  selectedExecutionOrder,
  selectedExecutionOrderId,
  supportsBrokerReadFeature,
} = useConsoleData();
const notifications = useNotifications();

const activeTab = ref("account");
const cancellingOrderIds = ref<Set<string>>(new Set());

type AccountExecutionOrder = ExecutionOrdersResponse["orders"][number];

interface ExecutionOrderCommandResult {
  accepted: boolean;
  operation: string;
  internalOrderId?: string | null;
  brokerOrderId?: string | null;
  brokerOrderIdEx?: string | null;
  orderStatus?: string | null;
  brokerErrorCode?: string | null;
  message: string;
  checkedAt: string;
}

const selectedRuntimeAccount = computed(() => {
  const selected = selectedBrokerAccount.value;
  if (selected == null) {
    return brokerRuntime.value.accounts[0] ?? null;
  }

  return (
    brokerRuntime.value.accounts.find(
      (account) =>
        account.accountId === selected.accountId &&
        account.tradingEnvironment === selected.tradingEnvironment,
    ) ?? null
  );
});

const accountTitle = computed(() => {
  if (selectedBrokerAccount.value != null) {
    return selectedBrokerAccount.value.displayName;
  }

  return selectedRuntimeAccount.value?.accountId ?? "未选择账户";
});

const accountSubtitle = computed(() => {
  const selected = selectedBrokerAccount.value;
  if (selected != null) {
    return `${selected.brokerId.toUpperCase()} / ${selected.accountId} / ${formatTradingEnvironment(selected.tradingEnvironment)} / ${formatMarketLabel(selected.market)}`;
  }

  const runtimeAccount = selectedRuntimeAccount.value;
  if (runtimeAccount != null) {
    return `${runtimeAccount.accountId} / ${formatTradingEnvironment(runtimeAccount.tradingEnvironment)}`;
  }

  return "请先在顶部账户范围中选择一个账户，或到设置页导入 OpenD 探测到的账户。";
});

const scopedExecutionOrders = computed(() => {
  const selected = selectedBrokerAccount.value;
  const scoped = selected == null
    ? executionOrders.value.orders
    : executionOrders.value.orders.filter(
      (order) =>
        order.brokerId === selected.brokerId &&
        order.accountId === selected.accountId &&
        order.tradingEnvironment === selected.tradingEnvironment &&
        order.market === selected.market,
    );

  return dedupeExecutionOrders(scoped);
});

const pendingOrders = computed(() =>
  scopedExecutionOrders.value.filter(
    (order) => !isFinalExecutionOrderStatus(order.status),
  ),
);

const historicalOrders = computed(() =>
  scopedExecutionOrders.value.filter((order) =>
    isFinalExecutionOrderStatus(order.status),
  ),
);

const accountCashBalances = computed(() => {
  const selected = selectedBrokerAccount.value;
  if (selected == null) {
    return portfolioCashBalances.value.balances;
  }

  return portfolioCashBalances.value.balances.filter(
    (balance) =>
      balance.brokerId === selected.brokerId &&
      balance.accountId === selected.accountId &&
      balance.tradingEnvironment === selected.tradingEnvironment,
  );
});

const accountProjectedPositions = computed(() => {
  const selected = selectedBrokerAccount.value;
  if (selected == null) {
    return portfolioPositions.value.positions;
  }

  return portfolioPositions.value.positions.filter(
    (position) =>
      position.brokerId === selected.brokerId &&
      position.accountId === selected.accountId &&
      position.tradingEnvironment === selected.tradingEnvironment &&
      position.market === selected.market,
  );
});

const accountBrokerPositions = computed(() => {
  const selected = selectedBrokerAccount.value;
  if (selected == null) {
    return brokerPositions.value.positions;
  }

  return brokerPositions.value.positions.filter(
    (position) =>
      position.accountId === selected.accountId &&
      position.tradingEnvironment === selected.tradingEnvironment &&
      position.market === selected.market,
  );
});

const accountPositions = computed(() => {
  if (accountBrokerPositions.value.length > 0) {
    return accountBrokerPositions.value.map((position) => ({
      symbol: position.symbol,
      name: position.symbolName,
      market: position.market,
      quantity: position.quantity,
      averagePrice: position.averageCostPrice ?? position.costPrice,
      marketValue: position.marketValue,
      unrealizedPnl: position.unrealizedPnl,
      currency: position.currency,
      source: "券商",
      updatedAt: brokerPositions.value.checkedAt,
    }));
  }

  return accountProjectedPositions.value.map((position) => ({
    symbol: position.symbol,
    name: null,
    market: position.market,
    quantity: position.quantity,
    averagePrice: position.averagePrice,
    marketValue: position.marketValue,
    unrealizedPnl: null,
    currency: null,
    source: "投影",
    updatedAt: position.updatedAt,
  }));
});

const totalCash = computed(() => {
  if (brokerFunds.value.summary?.cash != null) {
    return brokerFunds.value.summary.cash;
  }

  return accountCashBalances.value.reduce(
    (sum, balance) => sum + (balance.cashBalance ?? 0),
    0,
  );
});

const totalMarketValue = computed(() =>
  accountPositions.value.reduce(
    (sum, position) => sum + (position.marketValue ?? 0),
    0,
  ),
);

const accountHeaderStats = computed(() => [
  {
    label: "账户范围",
    value: selectedBrokerAccount.value == null ? "自动" : "已选择",
    hint: accountSubtitle.value,
  },
  {
    label: "资金余额",
    value: totalCash.value,
    hint: brokerFunds.value.summary?.currency ?? accountCashBalances.value[0]?.currency ?? "币种未设置",
  },
  {
    label: "持仓数量",
    value: accountPositions.value.length,
  },
  {
    label: "在途订单",
    value: pendingOrders.value.length,
    tone: pendingOrders.value.length > 0 ? "warn" : "good",
  },
]);

const accountFacts = computed(() => {
  const selected = selectedBrokerAccount.value;
  const runtimeAccount = selectedRuntimeAccount.value;

  return [
    {
      label: "券商",
      value: selected?.brokerId.toUpperCase() ?? brokerRuntime.value.descriptor.displayName ?? "未设置",
    },
    {
      label: "账户号",
      value: selected?.accountId ?? runtimeAccount?.accountId ?? "未设置",
    },
    {
      label: "交易环境",
      value: formatTradingEnvironment(
        selected?.tradingEnvironment ?? runtimeAccount?.tradingEnvironment,
      ),
    },
    {
      label: "市场",
      value: formatMarketLabel(selected?.market ?? runtimeAccount?.marketAuthorities[0]),
    },
    {
      label: "账户类型",
      value: formatAccountTypeLabel(runtimeAccount?.accountType),
    },
    {
      label: "券商机构",
      value: runtimeAccount?.securityFirm ?? selected?.securityFirm ?? "未设置",
    },
  ];
});

const fundsSummaryRows = computed(() => [
  { label: "总资产", value: brokerFunds.value.summary?.totalAssets },
  { label: "现金", value: brokerFunds.value.summary?.cash ?? totalCash.value },
  { label: "购买力", value: brokerFunds.value.summary?.purchasingPower },
  { label: "可取现金", value: brokerFunds.value.summary?.availableWithdrawalCash },
  { label: "证券市值", value: brokerFunds.value.summary?.marketValue ?? totalMarketValue.value },
  { label: "冻结资金", value: brokerFunds.value.summary?.frozenCash },
  // 融资融券资金字段
  { label: "融资可提", value: brokerFunds.value.summary?.maxWithdrawal },
  { label: "卖空购买力", value: brokerFunds.value.summary?.shortSellingPower },
  { label: "现金购买力", value: brokerFunds.value.summary?.netCashPower },
  { label: "多头市值", value: brokerFunds.value.summary?.longMarketValue },
  { label: "空头市值", value: brokerFunds.value.summary?.shortMarketValue },
  { label: "计息金额", value: brokerFunds.value.summary?.debtCash },
]);

const activeTradingEnvironment = computed(
  () =>
    selectedBrokerAccount.value?.tradingEnvironment ??
    selectedRuntimeAccount.value?.tradingEnvironment ??
    brokerFunds.value.summary?.tradingEnvironment ??
    null,
);

const activeBrokerReadContext = computed(() => {
  const selected = selectedBrokerAccount.value;
  if (selected != null) {
    return {
      brokerId: selected.brokerId,
      accountId: selected.accountId,
      tradingEnvironment: selected.tradingEnvironment,
      market: selected.market,
    };
  }

  const summary = brokerFunds.value.summary;
  if (summary != null) {
    return {
      brokerId: brokerRuntime.value.descriptor.id,
      accountId: summary.accountId,
      tradingEnvironment: summary.tradingEnvironment,
      market: summary.market,
    };
  }

  const runtimeAccount = selectedRuntimeAccount.value;
  if (runtimeAccount == null) {
    return null;
  }

  return {
    brokerId: brokerRuntime.value.descriptor.id,
    accountId: runtimeAccount.accountId,
    tradingEnvironment: runtimeAccount.tradingEnvironment,
    market:
      runtimeAccount.marketAuthorities[0] ??
      brokerRuntime.value.descriptor.capabilities[0]?.market ??
      "HK",
  };
});

const marginRatioSymbols = computed(() =>
  Array.from(
    new Set(
      accountPositions.value
        .map((position) => position.symbol?.trim())
        .filter((symbol): symbol is string => symbol != null && symbol !== ""),
    ),
  ).slice(0, 24),
);

const recentBrokerCashFlows = computed(() =>
  brokerCashFlows.value.cashFlows.slice(0, 8),
);
const recentBrokerFills = computed(() => brokerFills.value.fills.slice(0, 12));
const brokerFillsRequirements = computed(() =>
  resolveBrokerReadFeatureQueryRequirements("fills", {
    market: activeBrokerReadContext.value?.market ?? null,
    tradingEnvironment:
      activeBrokerReadContext.value?.tradingEnvironment ?? activeTradingEnvironment.value,
  }),
);
const brokerFillsDescription = computed(() =>
  brokerFillsRequirements.value.supportsHistory
    ? "展示当前账户最近 30 天的券商成交记录。"
    : "当前券商未声明历史成交能力，展示当前刷新窗口内可见的成交记录。",
);

const supportsBrokerCashFlows = computed(() =>
  supportsBrokerReadFeature("cashFlows", {
    market: activeBrokerReadContext.value?.market ?? null,
    tradingEnvironment:
      activeBrokerReadContext.value?.tradingEnvironment ?? activeTradingEnvironment.value,
  }),
);

const supportsBrokerMarginRatios = computed(() =>
  supportsBrokerReadFeature("marginRatios", {
    market: activeBrokerReadContext.value?.market ?? null,
    tradingEnvironment:
      activeBrokerReadContext.value?.tradingEnvironment ?? activeTradingEnvironment.value,
  }),
);

const supportsSelectedExecutionOrderFees = computed(() => {
  const order = selectedExecutionOrder.value;
  if (order == null) {
    return false;
  }
  return supportsBrokerReadFeature("orderFees", {
    market: order.market,
    tradingEnvironment: order.tradingEnvironment,
  });
});

function formatNumber(value: number | null | undefined): string {
  if (value == null) {
    return "暂无";
  }

  return new Intl.NumberFormat("zh-CN", {
    maximumFractionDigits: 4,
  }).format(value);
}

function formatMoney(
  value: number | null | undefined,
  currency?: string | null,
): string {
  const formatted = formatNumber(value);
  if (formatted === "暂无") {
    return formatted;
  }

  return currency != null && currency !== "" ? `${formatted} ${currency}` : formatted;
}

function executionOrderDisplayKey(order: AccountExecutionOrder): string {
  const brokerOrderIdentity =
    order.brokerOrderId?.trim() ||
    order.brokerOrderIdEx?.trim() ||
    order.internalOrderId;
  return [
    order.brokerId,
    order.accountId,
    order.tradingEnvironment,
    order.market,
    brokerOrderIdentity,
  ].join("|");
}

function dedupeExecutionOrders(
  orders: AccountExecutionOrder[],
): AccountExecutionOrder[] {
  const seen = new Set<string>();
  const deduped: AccountExecutionOrder[] = [];
  for (const order of orders) {
    const key = executionOrderDisplayKey(order);
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    deduped.push(order);
  }
  return deduped;
}

function selectOrder(internalOrderId: string): void {
  void loadExecutionOrderDetails(internalOrderId);
}

function openOrderEvents(internalOrderId: string): void {
  activeTab.value = "history";
  void loadExecutionOrderDetails(internalOrderId);
}

function isCancellingOrder(internalOrderId: string): boolean {
  return cancellingOrderIds.value.has(internalOrderId);
}

function canCancelOrder(order: AccountExecutionOrder): boolean {
  if (isFinalExecutionOrderStatus(order.status)) {
    return false;
  }
  if (isCancellingOrder(order.internalOrderId)) {
    return false;
  }

  const normalized = order.status.trim().toUpperCase();
  if (
    normalized === "CANCELING" ||
    normalized === "PENDING_CANCEL" ||
    normalized === "CANCEL_REQUESTED"
  ) {
    return false;
  }

  return true;
}

function formatExecutionStatusTransition(
  previousStatus: string | null | undefined,
  nextStatus: string | null | undefined,
): string {
  const nextLabel = formatExecutionOrderStatusLabel(nextStatus);
  if ((previousStatus ?? "").trim() === "") {
    return `首次发现，当前状态：${nextLabel}`;
  }
  return `${formatExecutionOrderStatusLabel(previousStatus)} → ${nextLabel}`;
}

async function refreshExecutionOrders(): Promise<void> {
  executionOrders.value = await fetchEnvelope<ExecutionOrdersResponse>(
    "/api/v1/execution/orders",
  );
}

async function cancelOrder(order: AccountExecutionOrder): Promise<void> {
  if (!canCancelOrder(order)) {
    return;
  }

  const nextCancelling = new Set(cancellingOrderIds.value);
  nextCancelling.add(order.internalOrderId);
  cancellingOrderIds.value = nextCancelling;

  try {
    const result = await fetchEnvelopeWithInit<ExecutionOrderCommandResult>(
      `/api/v1/execution/orders/${encodeURIComponent(order.internalOrderId)}/cancel`,
      {
        method: "POST",
      },
    );

    await refreshExecutionOrders();
    await loadExecutionOrderDetails(order.internalOrderId);

    notifications.push({
      level: "success",
      title: `已提交撤单 ${order.symbol ?? order.internalOrderId}`,
      message: result.message,
      source: "account-page",
    });
  } catch (error) {
    const message = error instanceof Error && error.message.trim() !== ""
      ? error.message
      : "撤单请求失败。";
    notifications.push({
      level: "error",
      title: `撤单失败 ${order.symbol ?? order.internalOrderId}`,
      message,
      source: "account-page",
    });
  } finally {
    const nextCancellingDone = new Set(cancellingOrderIds.value);
    nextCancellingDone.delete(order.internalOrderId);
    cancellingOrderIds.value = nextCancellingDone;
  }
}

function resolveOrderChipColor(status: string): string {
  if (isFinalExecutionOrderStatus(status)) {
    return status === "FILLED" ? "success" : "info";
  }

  const normalized = status.toUpperCase();
  if (normalized.includes("REJECT") || normalized.includes("FAIL")) {
    return "error";
  }

  if (normalized.includes("CANCEL")) {
    return "warning";
  }

  return "primary";
}

function isSelectedOrder(internalOrderId: string): boolean {
  return selectedExecutionOrderId.value === internalOrderId;
}

watch(
  [pendingOrders, historicalOrders],
  ([nextPendingOrders, nextHistoricalOrders]) => {
    const visibleOrders = [...nextPendingOrders, ...nextHistoricalOrders];
    const selectedStillVisible = visibleOrders.some(
      (order) => order.internalOrderId === selectedExecutionOrderId.value,
    );
    const nextOrderId =
      selectedStillVisible ? selectedExecutionOrderId.value : visibleOrders[0]?.internalOrderId;

    if (
      nextOrderId == null ||
      nextOrderId === "" ||
      executionOrderEvents.value.internalOrderId === nextOrderId
    ) {
      return;
    }

    void loadExecutionOrderDetails(nextOrderId);
  },
  { immediate: true },
);
</script>

<template>
  <div class="grid gap-6">
    <PageHeader
      eyebrow="我的账户"
      :title="accountTitle"
      description="账户页聚合基础资料、资金余额、持仓和订单流水。连接诊断与 OpenD 参数维护已经收敛到设置页。"
      :stats="accountHeaderStats"
    />

    <v-tabs v-model="activeTab" bg-color="transparent" class="tv-page-tabs">
      <v-tab value="account">账户信息</v-tab>
      <v-tab value="pending">在途订单</v-tab>
      <v-tab value="history">历史订单</v-tab>
    </v-tabs>

    <v-window v-model="activeTab">
      <v-window-item value="account">
        <div class="grid gap-5">
          <v-card flat class="card-shell border-0">
            <div class="flex flex-wrap items-start justify-between gap-3 px-4 pt-4">
              <div>
                <div class="text-xl font-semibold text-slate-900">账户资料</div>
                <div class="mt-1 text-sm text-slate-500">{{ accountSubtitle }}</div>
              </div>
              <v-chip variant="outlined" size="small">
                {{ formatConnectivityLabel(brokerRuntime.session.connectivity) }}
              </v-chip>
            </div>
            <v-card-text>
              <div class="grid gap-3 sm:grid-cols-4 xl:grid-cols-6">
                <div
                  v-for="fact in accountFacts"
                  :key="fact.label"
                  class="rounded-lg border border-slate-200 bg-white px-4 py-3"
                >
                  <div class="text-xs text-slate-500">{{ fact.label }}</div>
                  <div class="mt-2 text-base font-semibold text-slate-900">{{ fact.value }}</div>
                </div>
              </div>
            </v-card-text>
          </v-card>

          <section class="grid gap-5 xl:grid-cols-[0.9fr_1.1fr]">
            <v-card flat class="card-shell border-0">
              <div class="px-4 pt-4">
                <SectionHeader title="资金余额" description="按账户范围展示现金、购买力与可用资金。" />
              </div>
              <v-card-text>
                <div class="grid gap-3 sm:grid-cols-4">
                  <div
                    v-for="row in fundsSummaryRows"
                    :key="row.label"
                    class="rounded-lg bg-slate-50 px-4 py-3"
                  >
                    <div class="text-xs text-slate-500">{{ row.label }}</div>
                    <div class="mt-2 text-lg font-semibold text-slate-900">
                      {{ formatMoney(row.value, brokerFunds.summary?.currency) }}
                    </div>
                  </div>
                </div>

                <div v-if="accountCashBalances.length" class="mt-5 overflow-x-auto rounded-lg border border-slate-200 bg-white">
                  <table class="w-full text-sm">
                    <thead class="border-b border-slate-200 bg-slate-50 text-xs text-slate-500">
                      <tr>
                        <th class="px-4 py-3 text-left">币种</th>
                        <th class="px-4 py-3 text-right">现金余额</th>
                        <th class="px-4 py-3 text-left">更新时间</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr
                        v-for="balance in accountCashBalances"
                        :key="`${balance.accountId}-${balance.currency}`"
                        class="border-b border-slate-100 last:border-0"
                      >
                        <td class="px-4 py-3">{{ balance.currency }}</td>
                        <td class="px-4 py-3 text-right">{{ formatMoney(balance.cashBalance, balance.currency) }}</td>
                        <td class="px-4 py-3">{{ formatDateTime(balance.updatedAt) }}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </v-card-text>
            </v-card>

            <v-card flat class="card-shell border-0">
              <div class="px-4 pt-4">
                <SectionHeader title="持仓概览" description="优先展示券商持仓；暂无券商数据时展示内部投影持仓。" />
              </div>
              <v-card-text>
                <div v-if="accountPositions.length" class="overflow-x-auto rounded-lg border border-slate-200 bg-white">
                  <table class="w-full text-sm">
                    <thead class="border-b border-slate-200 bg-slate-50 text-xs text-slate-500">
                      <tr>
                        <th class="px-4 py-3 text-left">标的</th>
                        <th class="px-4 py-3 text-left">市场</th>
                        <th class="px-4 py-3 text-right">数量</th>
                        <th class="px-4 py-3 text-right">成本价</th>
                        <th class="px-4 py-3 text-right">市值</th>
                        <th class="px-4 py-3 text-right">未实现盈亏</th>
                        <th class="px-4 py-3 text-left">来源</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr
                        v-for="position in accountPositions"
                        :key="`${position.source}-${position.market}-${position.symbol}`"
                        class="border-b border-slate-100 last:border-0"
                      >
                        <td class="px-4 py-3">
                          <div class="font-medium text-slate-900">{{ position.symbol }}</div>
                          <div v-if="position.name" class="mt-1 text-xs text-slate-500">{{ position.name }}</div>
                        </td>
                        <td class="px-4 py-3">{{ formatMarketLabel(position.market) }}</td>
                        <td class="px-4 py-3 text-right">{{ formatNumber(position.quantity) }}</td>
                        <td class="px-4 py-3 text-right">{{ formatNumber(position.averagePrice) }}</td>
                        <td class="px-4 py-3 text-right">{{ formatMoney(position.marketValue, position.currency) }}</td>
                        <td class="px-4 py-3 text-right">{{ formatMoney(position.unrealizedPnl, position.currency) }}</td>
                        <td class="px-4 py-3">{{ position.source }}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                <v-empty-state v-else text="当前账户暂无持仓。" />
              </v-card-text>
            </v-card>
          </section>

          <v-card v-if="brokerFunds.summary?.initialMargin != null || brokerFunds.summary?.riskStatus != null" flat class="card-shell border-0">
            <div class="px-4 pt-4">
              <SectionHeader title="保证金与风控" description="展示当前账户的保证金额度、风险等级与持仓限额信息。" />
            </div>
            <v-card-text>
              <div class="grid gap-3 sm:grid-cols-4 xl:grid-cols-3">
                <div v-if="brokerFunds.summary?.riskStatus" class="rounded-lg border border-slate-200 bg-white px-4 py-3">
                  <div class="text-xs text-slate-500">风险等级</div>
                  <div class="mt-2 text-base font-semibold text-slate-900">{{ brokerFunds.summary.riskStatus }}</div>
                </div>
                <div v-if="brokerFunds.summary?.initialMargin != null" class="rounded-lg bg-slate-50 px-4 py-3">
                  <div class="text-xs text-slate-500">初始保证金</div>
                  <div class="mt-2 text-lg font-semibold text-slate-900">
                    {{ formatMoney(brokerFunds.summary.initialMargin, brokerFunds.summary?.currency) }}
                  </div>
                </div>
                <div v-if="brokerFunds.summary?.maintenanceMargin != null" class="rounded-lg bg-slate-50 px-4 py-3">
                  <div class="text-xs text-slate-500">维持保证金</div>
                  <div class="mt-2 text-lg font-semibold text-slate-900">
                    {{ formatMoney(brokerFunds.summary.maintenanceMargin, brokerFunds.summary?.currency) }}
                  </div>
                </div>
                <div v-if="brokerFunds.summary?.marginCallMargin != null" class="rounded-lg bg-slate-50 px-4 py-3">
                  <div class="text-xs text-slate-500">Margin Call 保证金</div>
                  <div class="mt-2 text-lg font-semibold text-slate-900">
                    {{ formatMoney(brokerFunds.summary.marginCallMargin, brokerFunds.summary?.currency) }}
                  </div>
                </div>
              </div>
              <!-- 持仓限额 -->
              <div v-if="brokerFunds.summary?.exposureLevel != null" class="mt-4 grid gap-3 sm:grid-cols-3">
                <div class="rounded-lg bg-slate-50 px-4 py-3">
                  <div class="text-xs text-slate-500">限额等级</div>
                  <div class="mt-2 text-base font-semibold text-slate-900">{{ brokerFunds.summary.exposureLevel }}</div>
                </div>
                <div v-if="brokerFunds.summary?.exposureLimit != null" class="rounded-lg bg-slate-50 px-4 py-3">
                  <div class="text-xs text-slate-500">持仓限额</div>
                  <div class="mt-2 text-base font-semibold text-slate-900">{{ formatMoney(brokerFunds.summary.exposureLimit, brokerFunds.summary?.currency) }}</div>
                </div>
                <div v-if="brokerFunds.summary?.remainingLimit != null" class="rounded-lg bg-slate-50 px-4 py-3">
                  <div class="text-xs text-slate-500">剩余限额</div>
                  <div class="mt-2 text-base font-semibold text-slate-900">{{ formatMoney(brokerFunds.summary.remainingLimit, brokerFunds.summary?.currency) }}</div>
                </div>
              </div>
              <!-- PDT 日内交易（美股专用） -->
              <div v-if="brokerFunds.summary?.isPdt != null || brokerFunds.summary?.dtStatus != null" class="mt-4 rounded-lg border border-amber-200 bg-amber-50/30 px-4 py-3">
                <div class="text-xs font-semibold text-amber-700 uppercase tracking-wide">美股 PDT / 日内交易</div>
                <div class="mt-2 grid gap-2 sm:grid-cols-4">
                  <div v-if="brokerFunds.summary?.isPdt != null" class="text-sm">
                    <span class="text-slate-500">PDT 账户：</span>
                    <span class="font-medium text-slate-900">{{ brokerFunds.summary.isPdt ? '是' : '否' }}</span>
                  </div>
                  <div v-if="brokerFunds.summary?.pdtSeq != null" class="text-sm">
                    <span class="text-slate-500">日内交易次数：</span>
                    <span class="font-medium text-slate-900">{{ brokerFunds.summary.pdtSeq }}</span>
                  </div>
                  <div v-if="brokerFunds.summary?.remainingDTBP != null" class="text-sm">
                    <span class="text-slate-500">剩余日内购买力：</span>
                    <span class="font-medium text-slate-900">{{ formatMoney(brokerFunds.summary.remainingDTBP) }}</span>
                  </div>
                  <div v-if="brokerFunds.summary?.dtStatus != null" class="text-sm">
                    <span class="text-slate-500">限制状态：</span>
                    <span class="font-medium text-slate-900">{{ brokerFunds.summary.dtStatus }}</span>
                  </div>
                </div>
              </div>
            </v-card-text>
          </v-card>

          <v-card flat class="card-shell border-0">
            <div class="flex flex-wrap items-center justify-between gap-3 px-4 pt-4">
              <div>
                <div class="text-xl font-semibold text-slate-900">最近资金流水</div>
                <div class="mt-1 text-sm text-slate-500">展示当前账户最近一次刷新拿到的券商现金流水，用于核对股息、入金和费用扣款。</div>
              </div>
              <v-chip variant="outlined" size="small">
                {{ formatConnectivityLabel(brokerCashFlows.connectivity) }}
              </v-chip>
            </div>
            <v-card-text>
              <v-empty-state
                v-if="!supportsBrokerCashFlows"
                text="当前券商未为该交易环境声明资金流水能力。"
              />
              <v-alert
                v-else-if="brokerCashFlows.lastError"
                type="warning"
                :closable="false"
                title="资金流水提示"
              >
                {{ brokerCashFlows.lastError }}
              </v-alert>
              <div
                v-else-if="recentBrokerCashFlows.length"
                class="overflow-x-auto rounded-lg border border-slate-200 bg-white"
              >
                <table class="w-full text-sm">
                  <thead class="border-b border-slate-200 bg-slate-50 text-xs text-slate-500">
                    <tr>
                      <th class="px-4 py-3 text-left">流水号</th>
                      <th class="px-4 py-3 text-left">清算日</th>
                      <th class="px-4 py-3 text-left">交收日</th>
                      <th class="px-4 py-3 text-left">类型 / 方向</th>
                      <th class="px-4 py-3 text-right">金额</th>
                      <th class="px-4 py-3 text-left">备注</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr
                      v-for="flow in recentBrokerCashFlows"
                      :key="flow.cashFlowId ?? `${flow.clearingDate ?? 'na'}-${flow.cashFlowType ?? 'na'}-${flow.cashFlowAmount ?? 'na'}`"
                      class="border-b border-slate-100 last:border-0"
                    >
                      <td class="px-4 py-3 font-mono text-xs">{{ flow.cashFlowId ?? '—' }}</td>
                      <td class="px-4 py-3">{{ flow.clearingDate ?? '—' }}</td>
                      <td class="px-4 py-3">{{ flow.settlementDate ?? '—' }}</td>
                      <td class="px-4 py-3">
                        <div class="font-medium text-slate-900">{{ flow.cashFlowType ?? '未分类' }}</div>
                        <div class="mt-1 text-xs text-slate-500">{{ flow.cashFlowDirection ?? '方向未标注' }}</div>
                      </td>
                      <td class="px-4 py-3 text-right">
                        {{ formatMoney(flow.cashFlowAmount, flow.currency ?? brokerFunds.summary?.currency) }}
                      </td>
                      <td class="px-4 py-3 text-slate-600">{{ flow.cashFlowRemark ?? '—' }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <v-empty-state v-else text="当前账户暂无券商资金流水。" />
            </v-card-text>
          </v-card>

          <v-card flat class="card-shell border-0">
            <div class="flex flex-wrap items-center justify-between gap-3 px-4 pt-4">
              <div>
                <div class="text-xl font-semibold text-slate-900">融资融券参数</div>
                <div class="mt-1 text-sm text-slate-500">按当前账户持仓标的查询融资许可、卖空券源和保证金阈值。</div>
              </div>
              <v-chip variant="outlined" size="small">
                {{ formatConnectivityLabel(brokerMarginRatios.connectivity) }}
              </v-chip>
            </div>
            <v-card-text>
              <v-empty-state
                v-if="!supportsBrokerMarginRatios"
                text="当前券商未为该交易环境声明融资融券参数能力。"
              />
              <v-empty-state
                v-else-if="marginRatioSymbols.length === 0"
                text="当前账户暂无持仓标的，融资融券参数按持仓标的查询；账户级保证金与风控信息见上方卡片。"
              />
              <div v-else-if="isLoadingBrokerMarginRatios" class="text-sm text-slate-500">
                正在加载融资融券参数...
              </div>
              <v-alert
                v-else-if="brokerMarginRatios.lastError"
                type="warning"
                :closable="false"
                title="融资融券提示"
              >
                {{ brokerMarginRatios.lastError }}
              </v-alert>
              <div
                v-else-if="brokerMarginRatios.marginRatios.length"
                class="overflow-x-auto rounded-lg border border-slate-200 bg-white"
              >
                <table class="w-full text-sm">
                  <thead class="border-b border-slate-200 bg-slate-50 text-xs text-slate-500">
                    <tr>
                      <th class="px-4 py-3 text-left">标的</th>
                      <th class="px-4 py-3 text-left">融资 / 融券</th>
                      <th class="px-4 py-3 text-right">券源余量</th>
                      <th class="px-4 py-3 text-right">融券费率</th>
                      <th class="px-4 py-3 text-right">预警比率</th>
                      <th class="px-4 py-3 text-right">初始保证金</th>
                      <th class="px-4 py-3 text-right">维持保证金</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr
                      v-for="ratio in brokerMarginRatios.marginRatios"
                      :key="ratio.symbol"
                      class="border-b border-slate-100 last:border-0"
                    >
                      <td class="px-4 py-3 font-medium text-slate-900">{{ ratio.symbol }}</td>
                      <td class="px-4 py-3">
                        {{ formatBooleanLabel(ratio.isLongPermit) }} / {{ formatBooleanLabel(ratio.isShortPermit) }}
                      </td>
                      <td class="px-4 py-3 text-right">{{ formatNumber(ratio.shortPoolRemain) }}</td>
                      <td class="px-4 py-3 text-right">{{ formatNumber(ratio.shortFeeRate) }}</td>
                      <td class="px-4 py-3 text-right">
                        {{ formatNumber(ratio.alertLongRatio) }} / {{ formatNumber(ratio.alertShortRatio) }}
                      </td>
                      <td class="px-4 py-3 text-right">
                        {{ formatNumber(ratio.initialMarginLongRatio) }} / {{ formatNumber(ratio.initialMarginShortRatio) }}
                      </td>
                      <td class="px-4 py-3 text-right">
                        {{ formatNumber(ratio.maintenanceLongRatio) }} / {{ formatNumber(ratio.maintenanceShortRatio) }}
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <v-empty-state v-else text="当前账户暂无融资融券参数。" />
            </v-card-text>
          </v-card>
        </div>
      </v-window-item>

      <v-window-item value="pending">
        <v-card flat class="card-shell border-0">
          <div class="flex flex-wrap items-center justify-between gap-3 px-4 pt-4">
            <div>
              <div class="text-xl font-semibold text-slate-900">在途订单</div>
              <div class="mt-1 text-sm text-slate-500">尚未进入最终状态的订单，适合快速查看成交进度和最新状态。</div>
            </div>
            <v-chip variant="outlined" size="small">{{ pendingOrders.length }} 笔</v-chip>
          </div>
          <v-card-text>
            <div v-if="pendingOrders.length" class="overflow-x-auto rounded-lg border border-slate-200 bg-white">
              <table class="w-full text-sm">
                <thead class="border-b border-slate-200 bg-slate-50 text-xs text-slate-500">
                  <tr>
                    <th class="px-4 py-3 text-left">标的</th>
                    <th class="px-4 py-3 text-left">方向</th>
                    <th class="px-4 py-3 text-left">类型</th>
                    <th class="px-4 py-3 text-right">数量</th>
                    <th class="px-4 py-3 text-right">已成交</th>
                    <th class="px-4 py-3 text-left">状态</th>
                    <th class="px-4 py-3 text-left">更新时间</th>
                    <th class="px-4 py-3 text-right">操作</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="order in pendingOrders"
                    :key="order.internalOrderId"
                    class="border-b border-slate-100 last:border-0"
                    :class="isSelectedOrder(order.internalOrderId) ? 'bg-teal-50/50' : ''"
                  >
                    <td class="px-4 py-3">
                      <div class="font-medium text-slate-900">{{ order.symbol ?? '未知标的' }}</div>
                      <div class="mt-1 text-xs text-slate-500">{{ order.internalOrderId }}</div>
                    </td>
                    <td class="px-4 py-3">{{ formatOrderSideLabel(order.side) }}</td>
                    <td class="px-4 py-3">{{ formatOrderTypeLabel(order.orderType) }}</td>
                    <td class="px-4 py-3 text-right">{{ formatNumber(order.requestedQuantity) }}</td>
                    <td class="px-4 py-3 text-right">{{ formatNumber(order.filledQuantity) }}</td>
                    <td class="px-4 py-3">
                      <v-chip :color="resolveOrderChipColor(order.status)" variant="outlined" size="small">
                        {{ formatExecutionOrderStatusLabel(order.status) }}
                      </v-chip>
                    </td>
                    <td class="px-4 py-3">{{ formatDateTime(order.updatedAt) }}</td>
                    <td class="px-4 py-3 text-right">
                      <div class="flex justify-end gap-2">
                        <v-btn variant="text" color="primary" @click="openOrderEvents(order.internalOrderId)">
                          查看事件
                        </v-btn>
                        <v-btn
                          variant="text"
                          color="warning"
                          :disabled="!canCancelOrder(order)"
                          :loading="isCancellingOrder(order.internalOrderId)"
                          @click="cancelOrder(order)"
                        >
                          撤单
                        </v-btn>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <v-empty-state v-else text="当前账户没有在途订单。" />
          </v-card-text>
        </v-card>
      </v-window-item>

      <v-window-item value="history">
        <section class="grid gap-5 xl:grid-cols-[1.05fr_0.95fr]">
          <v-card flat class="card-shell border-0">
            <div class="flex flex-wrap items-center justify-between gap-3 px-4 pt-4">
              <div>
                <div class="text-xl font-semibold text-slate-900">历史订单</div>
                <div class="mt-1 text-sm text-slate-500">已成交、已撤单、失败或过期的订单记录。</div>
              </div>
              <v-chip variant="outlined" size="small">{{ historicalOrders.length }} 笔</v-chip>
            </div>
            <v-card-text>
              <div v-if="historicalOrders.length" class="grid gap-3">
                <button
                  v-for="order in historicalOrders"
                  :key="order.internalOrderId"
                  type="button"
                  class="rounded-lg border px-4 py-3 text-left transition hover:border-teal-400"
                  :class="isSelectedOrder(order.internalOrderId) ? 'border-teal-400 bg-teal-50/70' : 'border-slate-200 bg-white'"
                  @click="selectOrder(order.internalOrderId)"
                >
                  <div class="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <div class="font-semibold text-slate-900">{{ order.symbol ?? '未知标的' }}</div>
                      <div class="mt-1 text-xs text-slate-500">{{ order.internalOrderId }}</div>
                    </div>
                    <v-chip :color="resolveOrderChipColor(order.status)" variant="outlined" size="small">
                      {{ formatExecutionOrderStatusLabel(order.status) }}
                    </v-chip>
                  </div>
                  <div class="mt-3 grid gap-2 text-sm text-slate-600 sm:grid-cols-4">
                    <span>{{ formatOrderSideLabel(order.side) }}</span>
                    <span>{{ formatOrderTypeLabel(order.orderType) }}</span>
                    <span>数量 {{ formatNumber(order.requestedQuantity) }}</span>
                    <span>成交 {{ formatNumber(order.filledQuantity) }}</span>
                  </div>
                  <div class="mt-2 text-xs text-slate-500">{{ formatDateTime(order.updatedAt) }}</div>
                </button>
              </div>
              <v-empty-state v-else text="当前账户暂无历史订单。" />
            </v-card-text>
          </v-card>

          <v-card flat class="card-shell border-0">
            <div class="flex flex-wrap items-center justify-between gap-3 px-4 pt-4">
              <div>
                <div class="text-xl font-semibold text-slate-900">订单事件与费用</div>
                <div class="mt-1 text-sm text-slate-500">
                  {{ selectedExecutionOrder?.internalOrderId ?? '请选择一笔订单' }}
                </div>
              </div>
              <v-chip variant="outlined" size="small">{{ executionOrderEvents.events.length }} 条事件</v-chip>
            </div>
            <v-card-text>
              <div v-if="isLoadingExecutionEvents" class="text-sm text-slate-500">正在加载订单事件...</div>
              <v-alert
                v-else-if="executionEventsError"
                type="warning"
                :closable="false"
                title="订单事件提示"
              >
                {{ executionEventsError }}
              </v-alert>

              <div v-else-if="executionOrderEvents.events.length" class="grid gap-3">
                <div
                  v-for="event in executionOrderEvents.events"
                  :key="event.id"
                  class="rounded-lg bg-slate-50 px-4 py-3"
                >
                  <div class="flex flex-wrap items-center justify-between gap-3">
                    <div class="font-medium text-slate-900">{{ formatExecutionEventTypeLabel(event.eventType) }}</div>
                    <div class="text-xs text-slate-500">{{ formatDateTime(event.createdAt) }}</div>
                  </div>
                  <div class="mt-2 text-sm text-slate-600">
                    {{ formatExecutionStatusTransition(event.previousStatus, event.nextStatus) }}
                  </div>
                </div>
              </div>
              <v-empty-state v-else text="当前订单暂无事件。" />

              <div class="mt-5 border-t border-slate-200 pt-4">
                <div class="flex flex-wrap items-center justify-between gap-3">
                  <div>
                    <div class="font-semibold text-slate-900">券商费用</div>
                    <div class="mt-1 text-xs text-slate-500">{{ selectedExecutionOrder?.brokerOrderIdEx ?? selectedExecutionOrder?.brokerOrderId ?? '暂无券商订单号' }}</div>
                  </div>
                  <v-chip variant="outlined" size="small">{{ brokerOrderFees.fees.length }} 条</v-chip>
                </div>

                <v-empty-state
                  v-if="selectedExecutionOrder != null && !supportsSelectedExecutionOrderFees"
                  text="当前券商未为该交易环境声明费用查询能力。"
                  class="mt-3"
                />
                <div v-else-if="isLoadingOrderFees" class="mt-3 text-sm text-slate-500">正在加载券商费用...</div>
                <v-alert
                  v-else-if="orderFeesError"
                  class="mt-3"
                  type="warning"
                  :closable="false"
                  title="费用查询提示"
                >
                  {{ orderFeesError }}
                </v-alert>
                <div v-else-if="brokerOrderFees.fees.length" class="mt-3 grid gap-3">
                  <div
                    v-for="fee in brokerOrderFees.fees"
                    :key="fee.brokerOrderIdEx"
                    class="rounded-lg bg-slate-50 px-4 py-3"
                  >
                    <div class="flex flex-wrap items-center justify-between gap-3">
                      <div class="font-medium text-slate-900">{{ fee.brokerOrderIdEx }}</div>
                      <div class="text-sm text-slate-700">{{ formatMoney(fee.feeAmount, brokerFunds.summary?.currency) }}</div>
                    </div>
                    <div v-if="fee.feeItems.length" class="mt-3 flex flex-wrap gap-2">
                      <span
                        v-for="detail in fee.feeItems"
                        :key="`${fee.brokerOrderIdEx}-${detail.title}`"
                        class="rounded-full border border-slate-200 bg-white px-2 py-1 text-xs text-slate-700"
                      >
                        {{ detail.title }}：{{ formatMoney(detail.value, brokerFunds.summary?.currency) }}
                      </span>
                    </div>
                  </div>
                </div>
                <v-empty-state v-else text="当前订单暂无券商费用。" class="mt-3" />

                <div class="mt-5 border-t border-slate-200 pt-4">
                  <div class="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <div class="font-semibold text-slate-900">最近成交</div>
                        <div class="mt-1 text-xs text-slate-500">{{ brokerFillsDescription }}</div>
                    </div>
                    <v-chip variant="outlined" size="small">{{ recentBrokerFills.length }} 条</v-chip>
                  </div>

                  <div v-if="isLoadingBrokerFills" class="mt-3 text-sm text-slate-500">正在加载券商成交...</div>
                  <v-alert
                    v-else-if="brokerFills.lastError"
                    class="mt-3"
                    type="warning"
                    :closable="false"
                    title="成交查询提示"
                  >
                    {{ brokerFills.lastError }}
                  </v-alert>
                  <div
                    v-else-if="recentBrokerFills.length"
                    class="mt-3 overflow-x-auto rounded-lg border border-slate-200 bg-white"
                  >
                    <table class="w-full text-sm">
                      <thead class="border-b border-slate-200 bg-slate-50 text-xs text-slate-500">
                        <tr>
                          <th class="px-4 py-3 text-left">成交号</th>
                          <th class="px-4 py-3 text-left">标的</th>
                          <th class="px-4 py-3 text-left">方向</th>
                          <th class="px-4 py-3 text-right">数量</th>
                          <th class="px-4 py-3 text-right">价格</th>
                          <th class="px-4 py-3 text-left">状态</th>
                          <th class="px-4 py-3 text-left">成交时间</th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr
                          v-for="fill in recentBrokerFills"
                          :key="fill.brokerFillId"
                          class="border-b border-slate-100 last:border-0"
                        >
                          <td class="px-4 py-3 font-mono text-xs">{{ fill.brokerFillIdEx ?? fill.brokerFillId }}</td>
                          <td class="px-4 py-3">
                            <div class="font-medium text-slate-900">{{ fill.symbol }}</div>
                            <div v-if="fill.symbolName" class="mt-1 text-xs text-slate-500">{{ fill.symbolName }}</div>
                          </td>
                          <td class="px-4 py-3">{{ formatOrderSideLabel(fill.side) }}</td>
                          <td class="px-4 py-3 text-right">{{ formatNumber(fill.filledQuantity) }}</td>
                          <td class="px-4 py-3 text-right">{{ formatNumber(fill.fillPrice) }}</td>
                          <td class="px-4 py-3">{{ fill.status ?? '—' }}</td>
                          <td class="px-4 py-3">{{ formatDateTime(fill.filledAt) }}</td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                  <v-empty-state v-else text="当前账户暂无券商成交。" class="mt-3" />
                </div>
              </div>
            </v-card-text>
          </v-card>
        </section>
      </v-window-item>
    </v-window>
  </div>
</template>
