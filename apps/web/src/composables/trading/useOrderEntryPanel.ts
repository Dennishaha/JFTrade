import { computed, onUnmounted, ref, watch } from "vue";

import { apiPost } from "@/composables/shared/apiClient";
import {
  formatExecutionEventTypeLabel,
  formatExecutionOrderStatusLabel,
  formatOrderSideLabel,
  formatOrderTypeLabel,
  formatTimeInForceLabel,
  isFinalExecutionOrderStatus,
} from "@/composables/shared/consoleDataFormatting";
import { useMarketProfiles } from "@/composables/market-data/marketProfiles";
import { formatMarketSessionLabel } from "@/composables/market-data/marketSessionDisplay";
import {
  formatInstrumentIdentityText,
  normalizeInstrumentSecurityType,
} from "@/composables/market-data/instrumentPresentation";
import { useConsoleData } from "@/composables/workspace/useConsoleData";
import { useNotifications } from "@/composables/shared/useNotifications";
import { useWorkspaceTradingPrefs } from "@/composables/workspace/useWorkspaceLayout";
import {
  alignPriceToStep,
  canCancelFeedbackOrder,
  countDecimalPlaces,
  createClientOrderId,
  formatBrokerAcceptance,
  formatFeedbackCheckedAt,
  formatFeedbackOrderStatus,
  formatInitialMargin,
  formatMetric,
  formatOrderSession,
  normalizeOptionalText,
  orderFeedbackAccountHref,
  resolveLatestOrderMarketPrice,
  resolveMaxTradeQuantityHint,
  resolveOrderFailureReason,
  resolveOrderProductClass,
  resolveOrderSessionCaution,
  resolvePendingOrderSummary,
  type ExecutionOrderPayload,
  type OrderFeedbackLevel,
  type OrderSession,
  type OrderType,
  type PendingRealTradeSubmission,
  type Side,
  type TIF,
} from "./orderEntryModels";
import { useOrderFeedback } from "./useOrderFeedback";

export function useOrderEntryPanel() {
const {
  brokerMaxTradeQuantity,
  isLoadingBrokerMaxTradeQuantity,
  loadBrokerMaxTradeQuantity,
  currentMarketDataSnapshot: marketDataSnapshot,
  currentMarketSecurityDetails: marketSecurityDetails,
  realTradeApprovals,
  realTradeRiskState,
  resolveBrokerReadFeatureQueryRequirements,
  selectedBrokerAccount,
  supportsBrokerReadFeature,
  systemStatus,
} = useConsoleData();
const { prefs } = useWorkspaceTradingPrefs();
const notifications = useNotifications();
const { supportsExtendedHoursForMarket } = useMarketProfiles();
const {
  isRefreshingOrderFeedback,
  lastOrderFeedback,
  orderFeedbackMaxPolls,
  orderFeedbackPollIntervalMs,
  orderFeedbackPolling,
  refreshOrderFeedback,
  refreshOrderFeedbackOnce,
  scheduleOrderFeedbackRefresh,
  startOrderFeedbackPolling,
  stopOrderFeedbackPolling,
} = useOrderFeedback(notifications.push);

const side = ref<Side>("BUY");
const orderType = ref<OrderType>("LIMIT");
const tif = ref<TIF>("DAY");
const orderSession = ref<OrderSession>("RTH");
const quantity = ref<number>(100);
const price = ref<number>(0);
const stopPrice = ref<number>(0);
const predictionSide = ref<"YES" | "NO">("YES");
const hasEditedPrice = ref(false);
const submitting = ref(false);
const realTradeConfirmationOpen = ref(false);
const realTradeConfirmationText = ref("");
const pendingRealTradeSubmission = ref<PendingRealTradeSubmission | null>(null);
const draftClientOrderId = ref("");
let maxTradeQuantityTimer: ReturnType<typeof setTimeout> | null = null;

const isRealMode = computed(
  () =>
    (selectedBrokerAccount.value?.tradingEnvironment ??
      systemStatus.value.defaultTradingEnvironment) === "REAL",
);
const requiredRealTradeConfirmationText = computed(
  () =>
    realTradeApprovals.value.requiredConfirmationText?.trim() ||
    "ENABLE_REAL_TRADING",
);
const realTradeConfirmationMatches = computed(
  () =>
    realTradeConfirmationText.value.trim() ===
    requiredRealTradeConfirmationText.value,
);
const isStop = computed(
  () => orderType.value === "STOP" || orderType.value === "STOP_LIMIT",
);
const isLimit = computed(
  () => orderType.value === "LIMIT" || orderType.value === "STOP_LIMIT",
);
const security = computed(() => marketSecurityDetails.value?.security ?? null);
const normalizedSecurityType = computed(() =>
  normalizeInstrumentSecurityType(security.value?.securityType),
);
const productClass = computed<ExecutionOrderPayload["productClass"]>(() =>
  resolveOrderProductClass({
    marketSegment: prefs.value.marketSegment,
    preferredProductClass: prefs.value.productClass,
    securityType: normalizedSecurityType.value,
    symbol: prefs.value.symbol.trim().toUpperCase(),
  }),
);
const isEventContract = computed(() => productClass.value === "event_contract");
const latestSnapshot = computed(() => {
  const snapshotResult = marketDataSnapshot.value;
  const currentInstrumentId = activeInstrument.value?.instrumentId ?? "";
  if (
    snapshotResult == null ||
    currentInstrumentId === "" ||
    snapshotResult.request.instrumentId.trim().toUpperCase() !== currentInstrumentId
  ) {
    return null;
  }
  return snapshotResult.snapshot;
});
const latestMarketPrice = computed(() =>
  resolveLatestOrderMarketPrice({
    snapshotPrice: latestSnapshot.value?.price,
    currentPrice: security.value?.currentPrice,
    bidPrice: security.value?.bidPrice,
    askPrice: security.value?.askPrice,
  }),
);
const limitPriceStep = computed(() => resolveOrderPriceStep(price.value));
const stopPriceStep = computed(() => resolveOrderPriceStep(stopPrice.value));
const tradeQuantityUnit = computed(() => {
  const securityType = normalizedSecurityType.value;
  if (isEventContract.value) return "金额";
  if (securityType.includes("FUTURE") || securityType.includes("OPTION")) {
    return "张";
  }
  if (
    securityType.includes("STOCK") ||
    securityType.includes("EQUITY") ||
    securityType.includes("ETF") ||
    securityType.includes("TRUST")
  ) {
    return "股";
  }
  return "单位";
});
const tradeQuantityUnitHint = computed(() => {
  const lotSize = security.value?.lotSize;
  if (tradeQuantityUnit.value === "股" && typeof lotSize === "number" && lotSize > 0) {
    return `单位：股 · 每手 ${formatMetric(lotSize)} 股`;
  }
  return `单位：${tradeQuantityUnit.value}`;
});
const formattedMaxTradeSession = computed(() => {
  const session = brokerMaxTradeQuantity.value.maxTradeQuantity?.session;
  if (session == null || session.trim() === "") {
    return "";
  }
  return formatOrderSession(session);
});

const activeBrokerId = computed(
  () => selectedBrokerAccount.value?.brokerId ?? systemStatus.value.defaultBroker,
);
const activeTradingEnvironment = computed(
  () =>
    selectedBrokerAccount.value?.tradingEnvironment ??
    systemStatus.value.defaultTradingEnvironment,
);
const activeAccountId = computed(
  () => selectedBrokerAccount.value?.accountId ?? "",
);
const activeMarket = computed(
  () => prefs.value.market.trim() || selectedBrokerAccount.value?.market || "",
);
const activeInstrument = computed(() => {
  const market = activeMarket.value.trim().toUpperCase();
  const symbol = prefs.value.symbol.trim().toUpperCase();
  if (market === "" || symbol === "") {
    return null;
  }
  return {
    market,
    code: symbol,
    symbol,
    instrumentId: `${market}.${symbol}`,
  };
});
const supportsOrderSessionSelection = computed(
  () =>
    ["equity", "fund", "warrant", "cbbc"].includes(productClass.value) &&
    supportsExtendedHoursForMarket(activeMarket.value),
);
const supportsBrokerMaxTradeQuantity = computed(() =>
  supportsBrokerReadFeature("maxTradeQuantity", {
    market: activeMarket.value,
    tradingEnvironment: activeTradingEnvironment.value,
  }),
);
const maxTradeQuantityRequirements = computed(() =>
  resolveBrokerReadFeatureQueryRequirements("maxTradeQuantity", {
    market: activeMarket.value,
    tradingEnvironment: activeTradingEnvironment.value,
  }),
);
const maxTradeQuantityRequiresPrice = computed(
  () => maxTradeQuantityRequirements.value.requiresPrice,
);
const maxTradeQuantityReferencePrice = computed(() => {
  switch (orderType.value) {
    case "LIMIT":
    case "STOP_LIMIT":
      return price.value > 0
        ? alignPriceToStep(price.value, limitPriceStep.value)
        : 0;
    case "STOP":
      return stopPrice.value > 0
        ? alignPriceToStep(stopPrice.value, stopPriceStep.value)
        : 0;
    default:
      return 0;
  }
});
const maxTradeQuantityPrimaryLabel = computed(() =>
  side.value === "BUY" ? "买入上限" : "卖出上限",
);
const maxTradeQuantityPrimaryValue = computed(() => {
  const snapshot = brokerMaxTradeQuantity.value.maxTradeQuantity;
  if (snapshot == null) {
    return null;
  }
  if (side.value === "BUY") {
    return snapshot.maxCashAndMarginBuy ?? snapshot.maxCashBuy;
  }
  return snapshot.maxSellShort ?? snapshot.maxPositionSell;
});
const maxTradeQuantityHint = computed(() =>
  resolveMaxTradeQuantityHint({
    supported: supportsBrokerMaxTradeQuantity.value,
    requiresPrice: maxTradeQuantityRequiresPrice.value,
    orderType: orderType.value,
    referencePrice: maxTradeQuantityReferencePrice.value,
  }),
);
const currentMarketSessionLabel = computed(() => {
  const session = latestSnapshot.value?.session;
  if (typeof session !== "string" || session.trim() === "") {
    return "";
  }
  return formatMarketSessionLabel(session);
});
const orderSessionSummary = computed(() => {
  if (!supportsOrderSessionSelection.value) {
    return "";
  }
  const summary: string[] = [];
  if (currentMarketSessionLabel.value !== "") {
    summary.push(`当前行情时段：${currentMarketSessionLabel.value}`);
  }
  summary.push(`下单时段：${formatOrderSession(orderSession.value)}`);
  return summary.join(" · ");
});
const orderSessionCaution = computed(() =>
  resolveOrderSessionCaution({
    supported: supportsOrderSessionSelection.value,
    currentSession: String(latestSnapshot.value?.session ?? ""),
    orderSession: orderSession.value,
    tradingEnvironment: activeTradingEnvironment.value,
  }),
);

function estimate(): string {
  const px = isLimit.value ? price.value : 0;
  if (!px || !quantity.value) return "—";
  return (px * quantity.value).toFixed(2);
}

function resolveReferencePrice(value: number): number | null {
  if (Number.isFinite(value) && value > 0) {
    return value;
  }
  const marketPrice = latestMarketPrice.value;
  if (marketPrice != null && marketPrice > 0) {
    return marketPrice;
  }
  const currentPrice = security.value?.currentPrice;
  if (typeof currentPrice === "number" && currentPrice > 0) {
    return currentPrice;
  }
  return null;
}

function resolveOrderPriceStep(value: number): number {
  const securitySpread = security.value?.priceSpread;
  if (typeof securitySpread === "number" && Number.isFinite(securitySpread) && securitySpread > 0) {
    return securitySpread;
  }
  const market = activeMarket.value.trim().toUpperCase();
  if (market === "US") {
    const referencePrice = resolveReferencePrice(value);
    return referencePrice != null && referencePrice < 1 ? 0.0001 : 0.01;
  }
  return market === "HK" ? 0.001 : 0.01;
}

function resolveAlignedMarketPrice(): number | null {
  const marketPrice = latestMarketPrice.value;
  if (marketPrice == null || marketPrice <= 0) {
    return null;
  }
  const aligned = alignPriceToStep(marketPrice, limitPriceStep.value);
  return aligned > 0 ? aligned : null;
}

function syncMarketPriceToPriceInput(showNotification = true): void {
  const aligned = resolveAlignedMarketPrice();
  if (aligned == null) {
    if (showNotification) {
      notifications.push({
        level: "warn",
        title: "暂无可同步的市场价格",
        source: "order-entry",
      });
    }
    return;
  }
  price.value = aligned;
  hasEditedPrice.value = false;
}

function markPriceEdited(): void {
  hasEditedPrice.value = true;
}

function alignPriceInput(): void {
  price.value = alignPriceToStep(price.value, limitPriceStep.value);
}

function alignStopPriceInput(): void {
  stopPrice.value = alignPriceToStep(stopPrice.value, stopPriceStep.value);
}

function resolveOrderRequestTitle(): string {
  const market = activeMarket.value.trim();
  const symbol = prefs.value.symbol.trim();
  const instrumentLabel =
    market !== "" || symbol !== ""
      ? formatInstrumentIdentityText({ market, code: symbol })
      : "当前标的";
  return `${formatOrderSideLabel(side.value)} ${quantity.value} ${instrumentLabel}`;
}

function currentClientOrderId(): string {
  if (draftClientOrderId.value === "") {
    draftClientOrderId.value = createClientOrderId();
  }
  return draftClientOrderId.value;
}

async function loadMaxTradeQuantity(): Promise<void> {
  const instrument = activeInstrument.value;
  if (instrument == null) {
    return;
  }
  const request = {
    brokerId: activeBrokerId.value,
    tradingEnvironment: activeTradingEnvironment.value,
    accountId: activeAccountId.value,
    market: instrument.market,
    symbol: instrument.instrumentId,
    orderType: orderType.value,
    price: maxTradeQuantityReferencePrice.value,
    ...(supportsOrderSessionSelection.value ? { session: orderSession.value } : {}),
  };
  await loadBrokerMaxTradeQuantity(request);
}

function validateAndBuildExecutionPayload(): ExecutionOrderPayload | null {
  const instrument = activeInstrument.value;
  if (instrument == null) {
    notifications.push({
      level: "warn",
      title: "标的无效",
      message: "请先选择有效的市场与代码。",
      source: "order-entry",
    });
    return null;
  }
  if (!quantity.value || quantity.value <= 0) {
    notifications.push({
      level: "warn",
      title: "数量无效",
      source: "order-entry",
    });
    return null;
  }
  if (isLimit.value && !price.value) {
    notifications.push({
      level: "warn",
      title: "价格必须大于 0",
      source: "order-entry",
    });
    return null;
  }
  if (isLimit.value) {
    alignPriceInput();
    if (price.value <= 0) {
      notifications.push({
        level: "warn",
        title: "价格必须大于 0",
        source: "order-entry",
      });
      return null;
    }
  }
  if (isStop.value) {
    alignStopPriceInput();
    if (stopPrice.value <= 0) {
      notifications.push({
        level: "warn",
        title: "止损价必须大于 0",
        source: "order-entry",
      });
      return null;
    }
  }

  const payload: ExecutionOrderPayload = {
    brokerId: activeBrokerId.value,
    tradingEnvironment: activeTradingEnvironment.value,
    accountId: activeAccountId.value,
    market: instrument.market,
    code: instrument.code,
    symbol: instrument.instrumentId,
    side: side.value,
    orderType: orderType.value,
    timeInForce: tif.value,
    quantity: quantity.value,
    productClass: productClass.value,
    quantityMode: isEventContract.value
      ? "amount"
      : ["option", "future"].includes(productClass.value)
        ? "contracts"
        : "units",
    orderKind: isEventContract.value ? "event_single" : "single",
    clientOrderId: currentClientOrderId(),
    env: activeTradingEnvironment.value,
  };
  if (isEventContract.value) {
    payload.amount = quantity.value;
    payload.predictionSide = predictionSide.value;
  }
  if (supportsOrderSessionSelection.value) {
    payload.session = orderSession.value;
  }
  if (isLimit.value) {
    payload.price = price.value;
  }
  if (isStop.value) {
    payload.stopPrice = stopPrice.value;
  }
  return payload;
}

async function submit(): Promise<void> {
  if (submitting.value) return;
  stopOrderFeedbackPolling();
  lastOrderFeedback.value = null;
  const payload = validateAndBuildExecutionPayload();
  if (payload == null) {
    return;
  }
  const feedbackTitle = resolveOrderRequestTitle();
  if (payload.tradingEnvironment.trim().toUpperCase() === "REAL") {
    pendingRealTradeSubmission.value = {
      payload,
      feedbackTitle,
      orderSummary: resolvePendingOrderSummary(payload),
    };
    realTradeConfirmationText.value = "";
    realTradeConfirmationOpen.value = true;
    return;
  }
  await executeOrderSubmission(payload, feedbackTitle);
}

function cancelRealTradeConfirmation(): void {
  realTradeConfirmationOpen.value = false;
  realTradeConfirmationText.value = "";
  pendingRealTradeSubmission.value = null;
}

async function confirmRealTradeSubmission(): Promise<void> {
  if (!realTradeConfirmationMatches.value || submitting.value) {
    return;
  }
  const pending = pendingRealTradeSubmission.value;
  if (pending == null) {
    cancelRealTradeConfirmation();
    return;
  }
  realTradeConfirmationOpen.value = false;
  realTradeConfirmationText.value = "";
  pendingRealTradeSubmission.value = null;
  await executeOrderSubmission(pending.payload, pending.feedbackTitle);
}

async function executeOrderSubmission(
  payload: ExecutionOrderPayload,
  feedbackTitle: string,
): Promise<void> {
  submitting.value = true;
  try {
    let feedbackLevel: OrderFeedbackLevel = "success";
    let feedbackMessage = `下单成功：已提交订单（${formatOrderTypeLabel(orderType.value)}，${formatTimeInForceLabel(tif.value)}${supportsOrderSessionSelection.value ? `，${formatOrderSession(orderSession.value)}` : ""}）`;
    try {
      if (["option", "future", "event_contract"].includes(payload.productClass)) {
        const preview = await apiPost(
          "/api/v1/execution/previews",
          payload,
        );
        if (!preview.previewId) {
          throw new Error("订单预检未返回 previewId");
        }
        payload.previewId = preview.previewId;
      }
      const body = await apiPost("/api/v1/execution/orders", payload);
      if (body.accepted !== true) {
        const reason = body.message?.trim() || body.brokerErrorCode?.trim() || "券商未接受该订单。";
        feedbackLevel = "error";
        feedbackMessage = `下单失败：${reason}`;
      }
      const brokerOrderId = normalizeOptionalText(body.brokerOrderId);
      const internalOrderId = normalizeOptionalText(body.internalOrderId);
      if (feedbackLevel === "success") {
        if (brokerOrderId) {
          feedbackMessage = `下单成功：已提交订单，券商单号 ${brokerOrderId}`;
        } else if (internalOrderId) {
          feedbackMessage = `下单成功：已提交订单，内部单号 ${internalOrderId}`;
        }
      }
      lastOrderFeedback.value = {
        level: feedbackLevel,
        title: feedbackTitle,
        message: feedbackMessage,
        internalOrderId,
        brokerOrderId,
        brokerOrderIdEx: normalizeOptionalText(body.brokerOrderIdEx),
        orderStatus: normalizeOptionalText(body.orderStatus),
        rawBrokerStatus: null,
        latestEvent: null,
        checkedAt: normalizeOptionalText(body.checkedAt),
      };
      if (feedbackLevel === "success" && internalOrderId != null && !isFinalExecutionOrderStatus(body.orderStatus)) {
        startOrderFeedbackPolling(internalOrderId);
      }
      if (feedbackLevel === "success") {
        draftClientOrderId.value = "";
      }
    } catch (error) {
      feedbackLevel = "error";
      feedbackMessage = `下单失败：${resolveOrderFailureReason(error)}`;
      lastOrderFeedback.value = {
        level: feedbackLevel,
        title: feedbackTitle,
        message: feedbackMessage,
        internalOrderId: null,
        brokerOrderId: null,
        brokerOrderIdEx: null,
        orderStatus: null,
        rawBrokerStatus: null,
        latestEvent: null,
        checkedAt: null,
      };
    }

    notifications.push({
      level: feedbackLevel,
      title: feedbackTitle,
      message: feedbackMessage,
      source: "order-entry",
    });
  } finally {
    submitting.value = false;
  }
}

function setSide(nextSide: Side): void {
  side.value = nextSide;
}

watch(
  [() => prefs.value.market, () => prefs.value.symbol],
  () => {
    hasEditedPrice.value = false;
    price.value = 0;
    draftClientOrderId.value = "";
  },
);

watch(
  [side, orderType, tif, orderSession, quantity, price, stopPrice, predictionSide],
  () => {
    if (!submitting.value) draftClientOrderId.value = "";
  },
);

watch(isEventContract, (eventContract) => {
  if (eventContract) orderType.value = "LIMIT";
});

watch(
  [latestMarketPrice, limitPriceStep, isLimit],
  () => {
    if (!isLimit.value || hasEditedPrice.value || price.value > 0) {
      return;
    }
    syncMarketPriceToPriceInput(false);
  },
  { immediate: true },
);

watch(
  [
    activeBrokerId,
    activeTradingEnvironment,
    activeAccountId,
    activeMarket,
    () => prefs.value.symbol,
    orderType,
    maxTradeQuantityReferencePrice,
    orderSession,
  ],
  () => {
    if (maxTradeQuantityTimer != null) {
      clearTimeout(maxTradeQuantityTimer);
      maxTradeQuantityTimer = null;
    }
    maxTradeQuantityTimer = setTimeout(() => {
      void loadMaxTradeQuantity();
    }, 250);
  },
  { immediate: true },
);

onUnmounted(() => {
  stopOrderFeedbackPolling();
  if (maxTradeQuantityTimer != null) {
    clearTimeout(maxTradeQuantityTimer);
  }
});

  return {
    notifications,
    side,
    orderType,
    tif,
    orderSession,
    quantity,
    price,
    stopPrice,
    predictionSide,
    hasEditedPrice,
    submitting,
    lastOrderFeedback,
    isRefreshingOrderFeedback,
    realTradeConfirmationOpen,
    realTradeConfirmationText,
    pendingRealTradeSubmission,
    draftClientOrderId,
    orderFeedbackPollIntervalMs,
    orderFeedbackMaxPolls,
    orderFeedbackPolling,
    isRealMode,
    requiredRealTradeConfirmationText,
    realTradeConfirmationMatches,
    isStop,
    isLimit,
    security,
    normalizedSecurityType,
    productClass,
    isEventContract,
    latestSnapshot,
    latestMarketPrice,
    limitPriceStep,
    stopPriceStep,
    tradeQuantityUnit,
    tradeQuantityUnitHint,
    formattedMaxTradeSession,
    activeBrokerId,
    activeTradingEnvironment,
    activeAccountId,
    activeMarket,
    activeInstrument,
    supportsOrderSessionSelection,
    supportsBrokerMaxTradeQuantity,
    maxTradeQuantityRequirements,
    maxTradeQuantityRequiresPrice,
    maxTradeQuantityReferencePrice,
    maxTradeQuantityPrimaryLabel,
    maxTradeQuantityPrimaryValue,
    maxTradeQuantityHint,
    currentMarketSessionLabel,
    orderSessionSummary,
    orderSessionCaution,
    estimate,
    formatMetric,
    countDecimalPlaces,
    resolveReferencePrice,
    resolveOrderPriceStep,
    alignPriceToStep,
    resolveAlignedMarketPrice,
    syncMarketPriceToPriceInput,
    markPriceEdited,
    alignPriceInput,
    alignStopPriceInput,
    formatOrderSession,
    formatInitialMargin,
    resolveOrderRequestTitle,
    createClientOrderId,
    currentClientOrderId,
    resolvePendingOrderSummary,
    resolveOrderFailureReason,
    normalizeOptionalText,
    orderFeedbackAccountHref,
    canCancelFeedbackOrder,
    formatFeedbackOrderStatus,
    formatBrokerAcceptance,
    formatFeedbackCheckedAt,
    stopOrderFeedbackPolling,
    scheduleOrderFeedbackRefresh,
    refreshOrderFeedbackOnce,
    refreshOrderFeedback,
    startOrderFeedbackPolling,
    loadMaxTradeQuantity,
    validateAndBuildExecutionPayload,
    submit,
    cancelRealTradeConfirmation,
    confirmRealTradeSubmission,
    executeOrderSubmission,
    setSide,
    brokerMaxTradeQuantity,
    isLoadingBrokerMaxTradeQuantity,
    loadBrokerMaxTradeQuantity,
    marketDataSnapshot,
    marketSecurityDetails,
    realTradeApprovals,
    realTradeRiskState,
    resolveBrokerReadFeatureQueryRequirements,
    selectedBrokerAccount,
    supportsBrokerReadFeature,
    systemStatus,
    prefs,
    supportsExtendedHoursForMarket,
    formatExecutionEventTypeLabel,
    formatExecutionOrderStatusLabel,
    formatOrderSideLabel,
    formatOrderTypeLabel,
    formatTimeInForceLabel,
    isFinalExecutionOrderStatus,
  };
}
