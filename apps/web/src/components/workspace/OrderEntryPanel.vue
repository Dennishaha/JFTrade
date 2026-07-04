<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";

import type { ExecutionOrderDetailsResponse, ExecutionOrderEventResponse } from "../../contracts";
import { fetchEnvelope, fetchEnvelopeWithInit } from "../../composables/apiClient";
import {
  formatExecutionEventTypeLabel,
  formatExecutionOrderStatusLabel,
  formatOrderSideLabel,
  formatOrderTypeLabel,
  formatTimeInForceLabel,
  isFinalExecutionOrderStatus,
} from "../../composables/consoleDataFormatting";
import { useMarketProfiles } from "../../composables/marketProfiles";
import { formatMarketSessionLabel } from "../../composables/marketSessionDisplay";
import { useConsoleData } from "../../composables/useConsoleData";
import { useNotifications } from "../../composables/useNotifications";
import { useWorkspaceTradingPrefs } from "../../composables/useWorkspaceLayout";
import RealTradeConfirmationDialog from "./RealTradeConfirmationDialog.vue";

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

type Side = "BUY" | "SELL";
type OrderType = "LIMIT" | "MARKET" | "STOP" | "STOP_LIMIT";
type TIF = "DAY" | "GTC" | "IOC" | "FOK";
type OrderSession = "RTH" | "ETH" | "ALL" | "OVERNIGHT";
type OrderFeedbackLevel = "success" | "error";

interface OrderFeedback {
  level: OrderFeedbackLevel;
  title: string;
  message: string;
  internalOrderId: string | null;
  brokerOrderId: string | null;
  brokerOrderIdEx: string | null;
  orderStatus: string | null;
  rawBrokerStatus: string | null;
  latestEvent: ExecutionOrderEventResponse | null;
  checkedAt: string | null;
}

interface ExecutionOrderPayload {
  brokerId: string;
  tradingEnvironment: string;
  accountId: string;
  market: string;
  code: string;
  symbol: string;
  side: Side;
  orderType: OrderType;
  timeInForce: TIF;
  session?: OrderSession;
  quantity: number;
  price?: number;
  stopPrice?: number;
  env: string;
}

interface PendingRealTradeSubmission {
  payload: ExecutionOrderPayload;
  feedbackTitle: string;
  orderSummary: string;
}

const side = ref<Side>("BUY");
const orderType = ref<OrderType>("LIMIT");
const tif = ref<TIF>("DAY");
const orderSession = ref<OrderSession>("RTH");
const quantity = ref<number>(100);
const price = ref<number>(0);
const stopPrice = ref<number>(0);
const hasEditedPrice = ref(false);
const submitting = ref(false);
const lastOrderFeedback = ref<OrderFeedback | null>(null);
const isRefreshingOrderFeedback = ref(false);
const realTradeConfirmationOpen = ref(false);
const realTradeConfirmationText = ref("");
const pendingRealTradeSubmission = ref<PendingRealTradeSubmission | null>(null);
let maxTradeQuantityTimer: ReturnType<typeof setTimeout> | null = null;
let orderFeedbackTimer: ReturnType<typeof setTimeout> | null = null;
let orderFeedbackPollCount = 0;

const orderFeedbackPollIntervalMs = 2_000;
const orderFeedbackMaxPolls = 60;

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
const latestMarketPrice = computed(() => {
  const snapshotPrice = latestSnapshot.value?.price;
  if (typeof snapshotPrice === "number" && snapshotPrice > 0) {
    return snapshotPrice;
  }
  const currentPrice = security.value?.currentPrice;
  if (typeof currentPrice === "number" && currentPrice > 0) {
    return currentPrice;
  }
  const bidPrice = security.value?.bidPrice;
  const askPrice = security.value?.askPrice;
  if (typeof bidPrice === "number" && bidPrice > 0 && typeof askPrice === "number" && askPrice > 0) {
    return (bidPrice + askPrice) / 2;
  }
  return null;
});
const limitPriceStep = computed(() => resolveOrderPriceStep(price.value));
const stopPriceStep = computed(() => resolveOrderPriceStep(stopPrice.value));
const tradeQuantityUnit = computed(() => {
  const securityType = security.value?.securityType.trim().toUpperCase() ?? "";
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
const supportsOrderSessionSelection = computed(() => supportsExtendedHoursForMarket(activeMarket.value));
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
const maxTradeQuantityHint = computed(() => {
  if (!supportsBrokerMaxTradeQuantity.value) {
    return "当前券商未为该交易环境声明最大可交易数量能力。";
  }
  if (maxTradeQuantityRequiresPrice.value && orderType.value === "MARKET") {
    return "市价单当前没有参考价输入，暂不估算最大可交易数量。";
  }
  if (
    maxTradeQuantityRequiresPrice.value &&
    orderType.value === "STOP" &&
    maxTradeQuantityReferencePrice.value <= 0
  ) {
    return "输入止损价后可估算最大可交易数量。";
  }
  if (
    maxTradeQuantityRequiresPrice.value &&
    maxTradeQuantityReferencePrice.value <= 0
  ) {
    return "输入价格后可估算最大可交易数量。";
  }
  return "根据当前账户、订单类型和价格估算最大可交易数量。";
});
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
const orderSessionCaution = computed(() => {
  if (!supportsOrderSessionSelection.value) {
    return "";
  }
  const currentSession = (latestSnapshot.value?.session ?? "").toString().trim().toLowerCase();
  if (
    orderSession.value === "RTH" &&
    ["pre", "after", "overnight"].includes(currentSession)
  ) {
    return "当前不是常规交易时段，RTH 订单通常要等盘中才会撮合。";
  }
  if (
    activeTradingEnvironment.value === "SIMULATE" &&
    orderSession.value === "OVERNIGHT"
  ) {
    return "模拟盘夜盘支持通常受限，提交成功也可能暂时不会成交。";
  }
  return "";
});

function estimate(): string {
  const px = isLimit.value ? price.value : 0;
  if (!px || !quantity.value) return "—";
  return (px * quantity.value).toFixed(2);
}

function formatMetric(value: number | null | undefined): string {
  if (value == null) {
    return "—";
  }
  return new Intl.NumberFormat("zh-CN", {
    maximumFractionDigits: 4,
  }).format(value);
}

function countDecimalPlaces(value: number): number {
  const text = value.toString().toLowerCase();
  if (!text.includes("e")) {
    return text.includes(".") ? (text.split(".")[1] ?? "").length : 0;
  }
  const [, exponentText] = text.split("e-");
  return Number.parseInt(exponentText ?? "0", 10) || 0;
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

function alignPriceToStep(value: number, step: number): number {
  if (!Number.isFinite(value) || value <= 0) {
    return 0;
  }
  const decimals = Math.min(8, countDecimalPlaces(step));
  return Number((Math.round(value / step) * step).toFixed(decimals));
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

function formatOrderSession(session: string): string {
  const normalized = session.trim().toUpperCase();
  if (normalized === "RTH") return "常规交易时段（RTH）";
  if (normalized === "ETH") return "扩展交易时段（ETH）";
  if (normalized === "ALL") return "全时段（ALL）";
  if (normalized === "OVERNIGHT") return "夜盘（OVERNIGHT）";
  return session;
}

function formatInitialMargin(value: number | null | undefined): string {
  if (value == null) {
    return "股票通常不返回";
  }
  return formatMetric(value);
}

function resolveOrderRequestTitle(): string {
  const market = activeMarket.value.trim();
  const symbol = prefs.value.symbol.trim();
  const instrumentLabel = market && symbol ? `${market}:${symbol}` : symbol || "当前标的";
  return `${formatOrderSideLabel(side.value)} ${quantity.value} ${instrumentLabel}`;
}

function resolvePendingOrderSummary(payload: ExecutionOrderPayload): string {
  const parts = [
    `${formatOrderSideLabel(payload.side)} ${payload.quantity} ${payload.symbol}`,
    formatOrderTypeLabel(payload.orderType),
    formatTimeInForceLabel(payload.timeInForce),
  ];
  if (payload.price != null) {
    parts.push(`限价 ${payload.price}`);
  }
  if (payload.stopPrice != null) {
    parts.push(`止损价 ${payload.stopPrice}`);
  }
  if (payload.session != null) {
    parts.push(formatOrderSession(payload.session));
  }
  return parts.join(" / ");
}

function resolveOrderFailureReason(error: unknown): string {
  if (error instanceof Error && error.message.trim() !== "") {
    return error.message.trim();
  }
  return "下单请求失败，请稍后重试。";
}

function normalizeOptionalText(value: string | null | undefined): string | null {
  const trimmed = value?.trim() ?? "";
  return trimmed === "" ? null : trimmed;
}

function orderFeedbackAccountHref(feedback: OrderFeedback): string {
  if (feedback.internalOrderId == null) {
    return "/account";
  }
  const params = new URLSearchParams();
  params.set("tab", "history");
  params.set("orderId", feedback.internalOrderId);
  return `/account?${params.toString()}`;
}

function canCancelFeedbackOrder(feedback: OrderFeedback): boolean {
  if (feedback.level !== "success" || feedback.internalOrderId == null) {
    return false;
  }
  const status = feedback.orderStatus?.trim();
  if (status == null || status === "") {
    return true;
  }
  return !isFinalExecutionOrderStatus(status);
}

function formatFeedbackOrderStatus(feedback: OrderFeedback): string {
  if (feedback.orderStatus == null) {
    return feedback.level === "success" ? "待券商回报" : "未接受";
  }
  return formatExecutionOrderStatusLabel(feedback.orderStatus);
}

function formatBrokerAcceptance(feedback: OrderFeedback): string {
  const status = feedback.orderStatus?.trim().toUpperCase() ?? "";
  if (["BROKER_ACCEPTED", "PARTIALLY_FILLED", "FILLED", "CANCEL_REQUESTED", "CANCELLED"].includes(status)) {
    return "已接受";
  }
  if (status === "REJECTED" || status === "EXPIRED") {
    return "未接受";
  }
  return "待确认";
}

function formatFeedbackCheckedAt(value: string | null): string {
  if (value == null || value.trim() === "") {
    return "";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleTimeString("zh-CN", { hour12: false });
}

function stopOrderFeedbackPolling(): void {
  if (orderFeedbackTimer != null) {
    clearTimeout(orderFeedbackTimer);
    orderFeedbackTimer = null;
  }
}

function scheduleOrderFeedbackRefresh(internalOrderId: string): void {
  stopOrderFeedbackPolling();
  if (orderFeedbackPollCount >= orderFeedbackMaxPolls) {
    return;
  }
  orderFeedbackTimer = setTimeout(() => {
    void refreshOrderFeedback(internalOrderId);
  }, orderFeedbackPollIntervalMs);
}

async function refreshOrderFeedback(internalOrderId: string, manual = false): Promise<void> {
  if (internalOrderId === "" || isRefreshingOrderFeedback.value) {
    return;
  }
  isRefreshingOrderFeedback.value = true;
  orderFeedbackPollCount += 1;
  try {
    const details = await fetchEnvelope<ExecutionOrderDetailsResponse>(
      `/api/v1/execution/orders/${encodeURIComponent(internalOrderId)}`,
    );
    const feedback = lastOrderFeedback.value;
    if (feedback == null || feedback.internalOrderId !== internalOrderId) {
      return;
    }
    feedback.brokerOrderId = normalizeOptionalText(details.order.brokerOrderId);
    feedback.brokerOrderIdEx = normalizeOptionalText(details.order.brokerOrderIdEx);
    feedback.orderStatus = normalizeOptionalText(details.order.status);
    feedback.rawBrokerStatus = normalizeOptionalText(details.order.rawBrokerStatus);
    feedback.latestEvent = details.recentEvents.at(-1) ?? null;
    feedback.checkedAt = normalizeOptionalText(details.checkedAt);
    if (isFinalExecutionOrderStatus(feedback.orderStatus)) {
      stopOrderFeedbackPolling();
    } else {
      scheduleOrderFeedbackRefresh(internalOrderId);
    }
  } catch (error) {
    if (manual) {
      notifications.push({
        level: "warn",
        title: "订单状态刷新失败",
        message: resolveOrderFailureReason(error),
        source: "order-entry",
      });
    }
    scheduleOrderFeedbackRefresh(internalOrderId);
  } finally {
    isRefreshingOrderFeedback.value = false;
  }
}

function startOrderFeedbackPolling(internalOrderId: string): void {
  orderFeedbackPollCount = 0;
  scheduleOrderFeedbackRefresh(internalOrderId);
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
    env: activeTradingEnvironment.value,
  };
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
      const body = await fetchEnvelopeWithInit<{
        accepted?: boolean;
        internalOrderId?: string | null;
        brokerOrderId?: string | null;
        brokerOrderIdEx?: string | null;
        orderStatus?: string | null;
        brokerErrorCode?: string | null;
        message?: string | null;
        checkedAt?: string | null;
      }>("/api/v1/execution/orders", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
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
  },
);

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
</script>

<template>
  <section class="tv-panel">
    <div class="tv-panel-head">
      <span class="tv-panel-title">下单</span>
      <span style="color: var(--tv-text); font-weight: 600">{{ prefs.market }}:{{ prefs.symbol }}</span>
      <div style="flex: 1"></div>
      <span
        v-if="isRealMode"
        style="font-size: 10px; padding: 2px 6px; border-radius: 4px; background: var(--tv-accent-strong); color: #fff; font-weight: 600"
      >
        实盘
      </span>
    </div>
    <div class="tv-panel-body">
      <div class="tv-seg tv-order-side-seg" style="width: 100%; margin-bottom: 10px">
        <button style="flex: 1" class="is-buy" :class="{ 'is-active': side === 'BUY' }" @click="setSide('BUY')">买入</button>
        <button style="flex: 1" class="is-sell" :class="{ 'is-active': side === 'SELL' }" @click="setSide('SELL')">卖出</button>
      </div>

      <div class="tv-form-row">
        <label>类型</label>
        <select v-model="orderType" class="tv-select">
          <option value="LIMIT">限价</option>
          <option value="MARKET">市价</option>
          <option value="STOP">止损</option>
          <option value="STOP_LIMIT">止损限价</option>
        </select>
      </div>

      <div class="tv-form-row">
        <label>数量</label>
        <input v-model.number="quantity" type="number" min="1" class="tv-input" />
      </div>

      <div v-if="isLimit" class="tv-form-row">
        <label>价格</label>
        <div style="display: grid; grid-template-columns: minmax(0, 1fr) 32px; gap: 6px; align-items: center">
          <input v-model.number="price" type="number" min="0" :step="limitPriceStep" class="tv-input" @input="markPriceEdited" @blur="alignPriceInput" />
          <button
            type="button"
            class="tv-icon-btn"
            title="同步市场价格"
            :disabled="latestMarketPrice == null"
            @click="syncMarketPriceToPriceInput(true)"
          >
            <span class="fa-solid fa-arrows-rotate" aria-hidden="true"></span>
          </button>
        </div>
      </div>

      <div v-if="isStop" class="tv-form-row">
        <label>止损价</label>
        <input v-model.number="stopPrice" type="number" min="0" :step="stopPriceStep" class="tv-input" @blur="alignStopPriceInput" />
      </div>

      <div class="tv-form-row">
        <label>有效期</label>
        <select v-model="tif" class="tv-select">
          <option value="DAY">当日有效</option>
          <option value="GTC">撤单前有效</option>
          <option value="IOC">立即成交剩余取消</option>
          <option value="FOK">全部成交否则取消</option>
        </select>
      </div>

      <div v-if="supportsOrderSessionSelection" class="tv-form-row">
        <label>时段</label>
        <select v-model="orderSession" class="tv-select">
          <option value="RTH">常规交易时段（RTH）</option>
          <option value="ETH">盘前盘后（ETH）</option>
          <option value="ALL">全时段（ALL）</option>
          <option value="OVERNIGHT">夜盘（OVERNIGHT）</option>
        </select>
      </div>

      <div v-if="supportsOrderSessionSelection && orderSessionSummary" style="margin: -2px 0 8px; font-size: 11px; color: var(--tv-text-dim)">
        {{ orderSessionSummary }}
      </div>

      <div v-if="supportsOrderSessionSelection && orderSessionCaution" style="margin: 0 0 10px; font-size: 11px; color: var(--tv-accent)">
        {{ orderSessionCaution }}
      </div>

      <div style="display: flex; justify-content: space-between; font-size: 11px; color: var(--tv-text-muted); margin: 4px 0 10px">
        <span>名义金额</span>
        <span class="tv-num" style="color: var(--tv-text)">{{ estimate() }}</span>
      </div>

      <div style="border: 1px solid var(--tv-border); border-radius: 8px; padding: 10px; margin: 0 0 10px; background: rgba(255,255,255,0.03)">
        <div style="display: flex; justify-content: space-between; gap: 8px; align-items: center">
          <span style="font-size: 11px; color: var(--tv-text-muted)">最大可交易数量</span>
          <span style="font-size: 11px; color: var(--tv-text-dim)">
            {{ formattedMaxTradeSession || tradeQuantityUnitHint }}
          </span>
        </div>
        <div v-if="isLoadingBrokerMaxTradeQuantity" style="margin-top: 6px; font-size: 11px; color: var(--tv-text-muted)">
          正在估算...
        </div>
        <div v-else-if="brokerMaxTradeQuantity.lastError" style="margin-top: 6px; font-size: 11px; color: var(--tv-accent)">
          {{ brokerMaxTradeQuantity.lastError }}
        </div>
        <template v-else-if="brokerMaxTradeQuantity.maxTradeQuantity">
          <div style="display: flex; justify-content: space-between; gap: 8px; margin-top: 6px">
            <span style="font-size: 11px; color: var(--tv-text-muted)">{{ maxTradeQuantityPrimaryLabel }}</span>
            <span class="tv-num" style="font-size: 16px; color: var(--tv-text); font-weight: 600">
              {{ formatMetric(maxTradeQuantityPrimaryValue) }} {{ tradeQuantityUnit }}
            </span>
          </div>
          <div style="margin-top: 4px; font-size: 11px; color: var(--tv-text-dim)">
            {{ tradeQuantityUnitHint }}<span v-if="formattedMaxTradeSession"> · {{ formattedMaxTradeSession }}</span>
          </div>
          <div style="display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; margin-top: 8px; font-size: 11px">
            <div>
              <div style="color: var(--tv-text-muted)">现金可买</div>
              <div class="tv-num" style="color: var(--tv-text)">{{ formatMetric(brokerMaxTradeQuantity.maxTradeQuantity.maxCashBuy) }} {{ tradeQuantityUnit }}</div>
            </div>
            <div>
              <div style="color: var(--tv-text-muted)">融资后可买</div>
              <div class="tv-num" style="color: var(--tv-text)">{{ formatMetric(brokerMaxTradeQuantity.maxTradeQuantity.maxCashAndMarginBuy) }} {{ tradeQuantityUnit }}</div>
            </div>
            <div>
              <div style="color: var(--tv-text-muted)">可卖持仓</div>
              <div class="tv-num" style="color: var(--tv-text)">{{ formatMetric(brokerMaxTradeQuantity.maxTradeQuantity.maxPositionSell) }} {{ tradeQuantityUnit }}</div>
            </div>
            <div>
              <div style="color: var(--tv-text-muted)">可卖空</div>
              <div class="tv-num" style="color: var(--tv-text)">{{ formatMetric(brokerMaxTradeQuantity.maxTradeQuantity.maxSellShort) }} {{ tradeQuantityUnit }}</div>
            </div>
          </div>
          <div style="display: flex; justify-content: space-between; gap: 8px; margin-top: 8px; font-size: 11px; color: var(--tv-text-muted)">
            <span title="多头初始保证金；股票通常不返回该字段">多头初始保证金 {{ formatInitialMargin(brokerMaxTradeQuantity.maxTradeQuantity.longRequiredIM) }}</span>
            <span title="空头初始保证金；股票通常不返回该字段">空头初始保证金 {{ formatInitialMargin(brokerMaxTradeQuantity.maxTradeQuantity.shortRequiredIM) }}</span>
          </div>
        </template>
        <div v-else style="margin-top: 6px; font-size: 11px; color: var(--tv-text-muted)">
          {{ maxTradeQuantityHint }}
        </div>
      </div>

      <button
        type="button"
        class="tv-btn"
        :class="side === 'BUY' ? 'tv-btn-buy' : 'tv-btn-sell'"
        style="width: 100%; height: 38px; font-weight: 600; letter-spacing: 0.04em"
        :disabled="submitting"
        @click="submit"
      >
        {{ submitting ? "提交中..." : `${formatOrderSideLabel(side)} ${prefs.symbol}` }}
      </button>
      <div
        v-if="lastOrderFeedback"
        class="tv-order-feedback"
        :class="`is-${lastOrderFeedback.level}`"
        role="status"
        aria-live="polite"
      >
        <div class="tv-order-feedback-title">{{ lastOrderFeedback.title }}</div>
        <div class="tv-order-feedback-message">{{ lastOrderFeedback.message }}</div>
        <div
          v-if="lastOrderFeedback.internalOrderId || lastOrderFeedback.brokerOrderId || lastOrderFeedback.brokerOrderIdEx"
          class="tv-order-receipt-grid"
        >
          <div v-if="lastOrderFeedback.internalOrderId">
            <span>内部单号</span>
            <strong>{{ lastOrderFeedback.internalOrderId }}</strong>
          </div>
          <div v-if="lastOrderFeedback.brokerOrderId || lastOrderFeedback.brokerOrderIdEx">
            <span>券商单号</span>
            <strong>{{ lastOrderFeedback.brokerOrderIdEx ?? lastOrderFeedback.brokerOrderId }}</strong>
          </div>
          <div>
            <span>当前状态</span>
            <strong>{{ formatFeedbackOrderStatus(lastOrderFeedback) }}</strong>
          </div>
          <div>
            <span>券商接受</span>
            <strong>{{ formatBrokerAcceptance(lastOrderFeedback) }}</strong>
          </div>
          <div>
            <span>撤单</span>
            <strong>{{ canCancelFeedbackOrder(lastOrderFeedback) ? "可在账户页提交" : "不可提交" }}</strong>
          </div>
          <div v-if="lastOrderFeedback.rawBrokerStatus">
            <span>券商原始状态</span>
            <strong>{{ lastOrderFeedback.rawBrokerStatus }}</strong>
          </div>
        </div>
        <div v-if="lastOrderFeedback.latestEvent" class="tv-order-feedback-event">
          最近事件：{{ formatExecutionEventTypeLabel(lastOrderFeedback.latestEvent.eventType) }}
        </div>
        <div v-if="lastOrderFeedback.internalOrderId" class="tv-order-feedback-actions">
          <a :href="orderFeedbackAccountHref(lastOrderFeedback)">查看账户订单</a>
          <span v-if="lastOrderFeedback.checkedAt">更新于 {{ formatFeedbackCheckedAt(lastOrderFeedback.checkedAt) }}</span>
          <button
            type="button"
            class="tv-icon-btn"
            title="刷新订单状态"
            :disabled="isRefreshingOrderFeedback"
            @click="refreshOrderFeedback(lastOrderFeedback.internalOrderId, true)"
          >
            <span class="fa-solid fa-arrows-rotate" aria-hidden="true"></span>
          </button>
        </div>
      </div>

      <RealTradeConfirmationDialog
        v-model="realTradeConfirmationOpen"
        v-model:confirmation-text="realTradeConfirmationText"
        :account-id="activeAccountId"
        :confirmation-matches="realTradeConfirmationMatches"
        :max-order-notional="realTradeRiskState.effectiveMaxOrderNotional"
        :max-order-quantity="realTradeRiskState.effectiveMaxOrderQuantity"
        :order-summary="pendingRealTradeSubmission?.orderSummary"
        :real-trading-enabled="systemStatus.realTradingEnabled"
        :required-confirmation-text="requiredRealTradeConfirmationText"
        :submitting="submitting"
        @cancel="cancelRealTradeConfirmation"
        @confirm="confirmRealTradeSubmission"
      />
    </div>
  </section>
</template>

<style scoped>
.tv-order-feedback {
  margin-top: 10px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  padding: 9px 10px;
  background: rgba(255, 255, 255, 0.03);
  font-size: 12px;
  line-height: 1.45;
}

.tv-order-feedback.is-success {
  border-left: 3px solid var(--tv-accent);
}

.tv-order-feedback.is-error {
  border-left: 3px solid var(--tv-accent-strong);
}

.tv-order-feedback-title {
  color: var(--tv-text);
  font-weight: 600;
}

.tv-order-feedback-message {
  margin-top: 3px;
  color: var(--tv-text-muted);
  overflow-wrap: anywhere;
}

.tv-order-receipt-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px;
  margin-top: 8px;
}

.tv-order-receipt-grid div {
  min-width: 0;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  padding: 6px;
  background: rgba(255, 255, 255, 0.025);
}

.tv-order-receipt-grid span {
  display: block;
  color: var(--tv-text-muted);
  font-size: 10px;
}

.tv-order-receipt-grid strong {
  display: block;
  margin-top: 2px;
  color: var(--tv-text);
  font-weight: 600;
  overflow-wrap: anywhere;
}

.tv-order-feedback-actions {
  margin-top: 8px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.tv-order-feedback-actions a {
  color: var(--tv-accent);
  font-size: 12px;
  font-weight: 600;
  text-decoration: none;
}

.tv-order-feedback-actions a:hover {
  text-decoration: underline;
}

.tv-order-feedback-actions > span {
  margin-left: auto;
  color: var(--tv-text-dim);
  font-size: 10px;
}

.tv-order-feedback-event {
  margin-top: 7px;
  color: var(--tv-text-muted);
  font-size: 11px;
}

</style>
