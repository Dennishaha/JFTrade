import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

import type { AccountExecutionOrder } from "@/components/domain/account/ActiveOrdersTable.vue";
import type { AccountPositionRow } from "@/components/domain/account/PositionsTable.vue";
import { apiGetPath, apiPostPathAction } from "@/composables/shared/apiClient";
import { isFinalExecutionOrderStatus } from "@/composables/shared/consoleDataFormatting";
import { formatInstrumentIdentityText } from "@/composables/market-data/instrumentPresentation";
import { useConsoleData } from "@/composables/workspace/useConsoleData";
import { useNotifications } from "@/composables/shared/useNotifications";
import { mapExecutionOrders } from "@/composables/trading/tradingApiMappers";
import {
  dedupeExecutionOrders,
  initialAccountTabFromLocation,
  initialExecutionOrderIdFromLocation,
  normalizeAccountTab,
} from "@/features/accountPage";
import type { AccountTab } from "@/features/accountPage";

export function useAccountPage() {
  const {
    activeExecutionOrders,
    activeExecutionOrdersError,
    historicalExecutionOrders,
    brokerFunds,
    brokerPositions,
    brokerRuntime,
    executionOrderEvents,
    historicalOrdersError,
    isLoadingHistoricalOrders,
    loadExecutionOrderDetails,
    loadHistoricalExecutionOrders,
    loadSystemState,
    portfolioPositions,
    portfolioLiveDataError,
    selectedBrokerAccount,
    selectedExecutionOrderId,
    supportsBrokerReadFeature,
    systemStatus,
  } = useConsoleData();
  const notifications = useNotifications();
  const route = useRoute();
  const router = useRouter();

  const ACCOUNT_AUTO_REFRESH_INTERVAL_MS = 5_000;

  const requestedExecutionOrderId = initialExecutionOrderIdFromLocation();
  const activeTab = ref<AccountTab>(initialAccountTabFromLocation(requestedExecutionOrderId));
  const cancellingOrderIds = ref<Set<string>>(new Set());
  const historicalOrdersDisplayLimit = ref(50);
  const hasLoadedHistoricalOrders = ref(false);
  const isRefreshingAccount = ref(false);
  let autoRefreshTimer: ReturnType<typeof setInterval> | null = null;

  const pendingCancelOrder = ref<AccountExecutionOrder | null>(null);
  const accountDataError = computed(() =>
    [portfolioLiveDataError.value, activeExecutionOrdersError.value]
      .filter((message) => message !== "")
      .join("；"),
  );
  const pendingCancelMessage = computed(() => {
    const order = pendingCancelOrder.value;
    if (order == null) return "";
    const instrument = order.symbol
      ? formatInstrumentIdentityText({
          market: order.market,
          instrumentId: order.symbol,
        })
      : order.internalOrderId;
    const kind = order.orderKind === "option_combo" || order.orderKind === "event_parlay"
      ? "组合订单"
      : "订单";
    return `确认撤销${kind} ${instrument}？撤单请求提交后仍以券商最终处理结果为准。`;
  });

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

  const scopedActiveExecutionOrders = computed(() => {
    const selected = selectedBrokerAccount.value;
    const scoped = activeExecutionOrders.value.orders.filter((order) => {
      if (selected == null) {
        return orderMatchesTradingEnvironment(order.tradingEnvironment);
      }
      return (
        order.brokerId === selected.brokerId &&
        order.accountId === selected.accountId &&
        order.tradingEnvironment === selected.tradingEnvironment &&
        order.market === selected.market
      );
    });

    return dedupeExecutionOrders(scoped);
  });

  const scopedHistoricalExecutionOrders = computed(() => {
    const selected = selectedBrokerAccount.value;
    const scoped = historicalExecutionOrders.value.orders.filter((order) => {
      if (selected == null) {
        return orderMatchesTradingEnvironment(order.tradingEnvironment);
      }
      return (
        order.brokerId === selected.brokerId &&
        order.accountId === selected.accountId &&
        order.tradingEnvironment === selected.tradingEnvironment &&
        order.market === selected.market
      );
    });

    return dedupeExecutionOrders(scoped);
  });

  const pendingOrders = computed(() =>
    scopedActiveExecutionOrders.value.filter(
      (order) => !isFinalExecutionOrderStatus(order.status),
    ),
  );

  const historicalOrders = computed(() =>
    scopedHistoricalExecutionOrders.value.filter((order) =>
      isFinalExecutionOrderStatus(order.status),
    ),
  );

  const displayedHistoricalOrders = computed(() =>
    historicalOrders.value.slice(0, historicalOrdersDisplayLimit.value),
  );

  const hasMoreHistoricalOrders = computed(
    () => historicalOrdersDisplayLimit.value < historicalOrders.value.length,
  );

  const accountProjectedPositions = computed(() => {
    const selected = selectedBrokerAccount.value;
    if (selected == null) {
      return portfolioPositions.value.positions.filter((position) =>
        orderMatchesTradingEnvironment(position.tradingEnvironment),
      );
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
      return brokerPositions.value.positions.filter((position) =>
        orderMatchesTradingEnvironment(position.tradingEnvironment),
      );
    }

    return brokerPositions.value.positions.filter(
      (position) =>
        position.accountId === selected.accountId &&
        position.tradingEnvironment === selected.tradingEnvironment &&
        position.market === selected.market,
    );
  });

  const accountPositions = computed<AccountPositionRow[]>(() => {
    if (accountBrokerPositions.value.length > 0) {
      return accountBrokerPositions.value.map((position) => ({
        symbol: position.symbol,
        name: position.symbolName ?? null,
        market: position.market,
        quantity: position.quantity,
        averagePrice: position.averageCostPrice ?? position.costPrice ?? null,
        lastPrice: position.lastPrice ?? null,
        marketValue: position.marketValue,
        unrealizedPnl: position.unrealizedPnl ?? null,
        pnlRatio: position.pnlRatio ?? null,
        currency: position.currency,
        productClass: null,
        strategyType: null,
        positionType: null,
        payoutIfWin: null,
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
      lastPrice: null,
      marketValue: position.marketValue,
      unrealizedPnl: null,
      pnlRatio: null,
      currency: null,
      productClass: "equity",
      strategyType: null,
      positionType: null,
      payoutIfWin: null,
      source: "券商",
      updatedAt: position.updatedAt,
    }));
  });

  const activeTradingEnvironment = computed(
    () =>
      selectedBrokerAccount.value?.tradingEnvironment ??
      selectedRuntimeAccount.value?.tradingEnvironment ??
      brokerFunds.value.summary?.tradingEnvironment ??
      systemStatus.value.defaultTradingEnvironment ??
      null,
  );

  function orderMatchesTradingEnvironment(tradingEnvironment: string): boolean {
    const activeEnvironment = activeTradingEnvironment.value;
    if (activeEnvironment == null || activeEnvironment.trim() === "") {
      return false;
    }
    return (
      tradingEnvironment.trim().toUpperCase() ===
      activeEnvironment.trim().toUpperCase()
    );
  }

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

  function selectOrder(internalOrderId: string): void {
    void loadExecutionOrderDetails(internalOrderId);
  }

  function openOrderEvents(order: AccountExecutionOrder): void {
    setActiveTab("history");
    void loadExecutionOrderDetails(order.internalOrderId);
    ensureHistoricalOrdersLoaded();
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

  function executionOrdersUrl(): string {
    const params = new URLSearchParams();
    const context = activeBrokerReadContext.value;
    if (context != null) {
      params.set("brokerId", context.brokerId);
      params.set("tradingEnvironment", context.tradingEnvironment);
      params.set("accountId", context.accountId);
      params.set("market", context.market);
    } else if (activeTradingEnvironment.value != null) {
      params.set("tradingEnvironment", activeTradingEnvironment.value);
    }
    const query = params.toString();
    return query === "" ? "/api/v1/execution/orders" : `/api/v1/execution/orders?${query}`;
  }

  async function refreshExecutionOrders(): Promise<void> {
    activeExecutionOrders.value = mapExecutionOrders(
      await apiGetPath(
        "/api/v1/execution/orders",
        executionOrdersUrl(),
      ),
    );
  }

  async function reloadHistoricalOrders(): Promise<void> {
    const context = activeBrokerReadContext.value;
    if (context == null) return;
    const params = new URLSearchParams();
    params.set("brokerId", context.brokerId);
    params.set("tradingEnvironment", context.tradingEnvironment);
    if (context.accountId) params.set("accountId", context.accountId);
    if (context.market) params.set("market", context.market);
    await loadHistoricalExecutionOrders({
      brokerId: context.brokerId,
      brokerQuery: params.toString(),
    });
  }

  function ensureHistoricalOrdersLoaded(): void {
    if (hasLoadedHistoricalOrders.value) return;
    hasLoadedHistoricalOrders.value = true;
    void reloadHistoricalOrders();
  }

  async function refreshAccountData(
    options: { background?: boolean } = {},
  ): Promise<void> {
    if (isRefreshingAccount.value) return;
    isRefreshingAccount.value = true;
    try {
      // 测试环境可能未提供 loadSystemState，需可缺省。
      if (typeof loadSystemState === "function") {
        await loadSystemState(
          options.background === true ? { background: true } : { bypassCooldown: true },
        );
      }
      // 每次刷新覆盖全部信息：除资金/持仓/订单外，历史订单也一并重拉。
      await reloadHistoricalOrders();
    } finally {
      isRefreshingAccount.value = false;
    }
  }

  onMounted(() => {
    autoRefreshTimer = setInterval(() => {
      if (typeof document !== "undefined" && document.visibilityState === "hidden") {
        return;
      }
      void refreshAccountData({ background: true });
    }, ACCOUNT_AUTO_REFRESH_INTERVAL_MS);
  });

  onBeforeUnmount(() => {
    if (autoRefreshTimer != null) {
      clearInterval(autoRefreshTimer);
      autoRefreshTimer = null;
    }
  });

  function loadMoreHistoricalOrders(): void {
    historicalOrdersDisplayLimit.value += 50;
  }

  async function cancelOrder(order: AccountExecutionOrder): Promise<void> {
    if (!canCancelOrder(order)) {
      return;
    }

    const nextCancelling = new Set(cancellingOrderIds.value);
    nextCancelling.add(order.internalOrderId);
    cancellingOrderIds.value = nextCancelling;

    try {
      const cancelPath =
        order.orderKind === "option_combo" || order.orderKind === "event_parlay"
          ? `/api/v1/execution/combos/${encodeURIComponent(order.internalOrderId)}/cancel`
          : `/api/v1/execution/orders/${encodeURIComponent(order.internalOrderId)}/cancel`;
      const result = order.orderKind === "option_combo" || order.orderKind === "event_parlay"
        ? await apiPostPathAction(
            "/api/v1/execution/combos/{internalOrderId}/cancel",
            cancelPath,
          )
        : await apiPostPathAction(
            "/api/v1/execution/orders/{internalOrderId}/cancel",
            cancelPath,
          );

      await refreshExecutionOrders();
      await loadExecutionOrderDetails(order.internalOrderId);

      notifications.push({
        level: "success",
        title: `已提交撤单 ${
          order.symbol
            ? formatInstrumentIdentityText({
                market: order.market,
                instrumentId: order.symbol,
              })
            : order.internalOrderId
        }`,
        message: result.message,
        source: "account-page",
      });
    } catch (error) {
      const message = error instanceof Error && error.message.trim() !== ""
        ? error.message
        : "撤单请求失败。";
      notifications.push({
        level: "error",
        title: `撤单失败 ${
          order.symbol
            ? formatInstrumentIdentityText({
                market: order.market,
                instrumentId: order.symbol,
              })
            : order.internalOrderId
        }`,
        message,
        source: "account-page",
      });
    } finally {
      const nextCancellingDone = new Set(cancellingOrderIds.value);
      nextCancellingDone.delete(order.internalOrderId);
      cancellingOrderIds.value = nextCancellingDone;
    }
  }

  function requestCancelOrder(order: AccountExecutionOrder): void {
    if (!canCancelOrder(order)) return;
    pendingCancelOrder.value = order;
  }

  function confirmCancelOrder(): void {
    const order = pendingCancelOrder.value;
    pendingCancelOrder.value = null;
    if (order != null) void cancelOrder(order);
  }

  function setActiveTab(tab: AccountTab): void {
    if (activeTab.value === tab) return;
    activeTab.value = tab;
  }

  // Tab 与 URL 双向同步：入口带 ?tab= 跳转时切换视图。
  watch(
    () => route?.query.tab,
    (raw) => {
      const normalized = normalizeAccountTab(
        typeof raw === "string" ? raw : null,
      );
      if (normalized != null && normalized !== activeTab.value) {
        activeTab.value = normalized;
      }
    },
    { immediate: true },
  );

  watch(activeTab, (tab) => {
    if (tab === "history") {
      ensureHistoricalOrdersLoaded();
    }
    if (route == null || router == null) return;
    const currentQueryTab = normalizeAccountTab(
      typeof route.query.tab === "string" ? route.query.tab : null,
    );
    if (currentQueryTab !== tab) {
      void router.replace({
        path: route.path,
        query: { ...route.query, tab },
      });
    }
  });

  watch(
    [pendingOrders, historicalOrders],
    ([nextPendingOrders, nextHistoricalOrders]) => {
      const visibleOrders = [...nextPendingOrders, ...nextHistoricalOrders];
      const selectedStillVisible = visibleOrders.some(
        (order) => order.internalOrderId === selectedExecutionOrderId.value,
      );
      const requestedStillVisible =
        requestedExecutionOrderId !== "" &&
        visibleOrders.some((order) => order.internalOrderId === requestedExecutionOrderId);
      const nextOrderId =
        requestedStillVisible
          ? requestedExecutionOrderId
          : selectedStillVisible ? selectedExecutionOrderId.value : visibleOrders[0]?.internalOrderId;

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

  if (requestedExecutionOrderId !== "") {
    ensureHistoricalOrdersLoaded();
    void loadExecutionOrderDetails(requestedExecutionOrderId);
  }

  return {
    requestedExecutionOrderId,
    activeTab,
    cancellingOrderIds,
    historicalOrdersDisplayLimit,
    hasLoadedHistoricalOrders,
    isRefreshingAccount,
    pendingCancelOrder,
    accountDataError,
    pendingCancelMessage,
    selectedRuntimeAccount,
    scopedActiveExecutionOrders,
    scopedHistoricalExecutionOrders,
    pendingOrders,
    historicalOrders,
    displayedHistoricalOrders,
    hasMoreHistoricalOrders,
    accountProjectedPositions,
    accountBrokerPositions,
    accountPositions,
    activeTradingEnvironment,
    activeBrokerReadContext,
    marginRatioSymbols,
    supportsBrokerCashFlows,
    supportsBrokerMarginRatios,
    historicalOrdersError,
    isLoadingHistoricalOrders,
    selectedExecutionOrderId,
    orderMatchesTradingEnvironment,
    selectOrder,
    openOrderEvents,
    isCancellingOrder,
    canCancelOrder,
    executionOrdersUrl,
    refreshExecutionOrders,
    reloadHistoricalOrders,
    ensureHistoricalOrdersLoaded,
    refreshAccountData,
    loadMoreHistoricalOrders,
    cancelOrder,
    requestCancelOrder,
    confirmCancelOrder,
    setActiveTab,
  };
}
