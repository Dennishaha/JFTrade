import type { ExecutionOrderEventResponse } from "@/contracts";
import {
  formatExecutionOrderStatusLabel,
  formatOrderSideLabel,
  formatOrderTypeLabel,
  formatTimeInForceLabel,
  isFinalExecutionOrderStatus,
} from "@/composables/shared/consoleDataFormatting";
import { formatInstrumentIdentityText } from "@/composables/market-data/instrumentPresentation";

export type Side = "BUY" | "SELL";
export type OrderType = "LIMIT" | "MARKET" | "STOP" | "STOP_LIMIT";
export type TIF = "DAY" | "GTC" | "IOC" | "FOK";
export type OrderSession = "RTH" | "ETH" | "ALL" | "OVERNIGHT";
export type OrderFeedbackLevel = "success" | "error";
export type OrderProductClass =
  | "equity"
  | "fund"
  | "option"
  | "warrant"
  | "cbbc"
  | "future"
  | "event_contract";

export interface OrderFeedback {
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

export interface ExecutionOrderPayload {
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
  productClass: OrderProductClass;
  quantityMode: "units" | "contracts" | "amount";
  orderKind: "single" | "event_single";
  clientOrderId: string;
  previewId?: string;
  amount?: number;
  predictionSide?: "YES" | "NO";
  price?: number;
  stopPrice?: number;
  env: string;
}

export interface PendingRealTradeSubmission {
  payload: ExecutionOrderPayload;
  feedbackTitle: string;
  orderSummary: string;
}

export function resolveOrderProductClass(input: {
  marketSegment: string;
  preferredProductClass: string;
  securityType: string;
  symbol: string;
}): OrderProductClass {
  if (
    input.marketSegment === "prediction" ||
    input.preferredProductClass === "event_contract"
  ) {
    return "event_contract";
  }
  for (const value of ["option", "future", "cbbc", "warrant", "fund"] as const) {
    if (input.preferredProductClass === value) return value;
  }
  const securityType = input.securityType;
  if (securityType.includes("EVENT") || input.symbol.startsWith("EC.")) {
    return "event_contract";
  }
  if (securityType.includes("OPTION")) return "option";
  if (securityType.includes("FUTURE")) return "future";
  if (securityType.includes("CBBC")) return "cbbc";
  if (securityType.includes("WARRANT")) return "warrant";
  if (
    securityType.includes("ETF") ||
    securityType.includes("FUND") ||
    securityType.includes("TRUST")
  ) {
    return "fund";
  }
  return "equity";
}

export function resolveLatestOrderMarketPrice(input: {
  snapshotPrice: number | null | undefined;
  currentPrice: number | null | undefined;
  bidPrice: number | null | undefined;
  askPrice: number | null | undefined;
}): number | null {
  if (typeof input.snapshotPrice === "number" && input.snapshotPrice > 0) {
    return input.snapshotPrice;
  }
  if (typeof input.currentPrice === "number" && input.currentPrice > 0) {
    return input.currentPrice;
  }
  if (
    typeof input.bidPrice === "number" &&
    input.bidPrice > 0 &&
    typeof input.askPrice === "number" &&
    input.askPrice > 0
  ) {
    return (input.bidPrice + input.askPrice) / 2;
  }
  return null;
}

export function resolveMaxTradeQuantityHint(input: {
  supported: boolean;
  requiresPrice: boolean;
  orderType: OrderType;
  referencePrice: number;
}): string {
  if (!input.supported) {
    return "当前券商未为该交易环境声明最大可交易数量能力。";
  }
  if (input.requiresPrice && input.orderType === "MARKET") {
    return "市价单当前没有参考价输入，暂不估算最大可交易数量。";
  }
  if (
    input.requiresPrice &&
    input.orderType === "STOP" &&
    input.referencePrice <= 0
  ) {
    return "输入止损价后可估算最大可交易数量。";
  }
  if (input.requiresPrice && input.referencePrice <= 0) {
    return "输入价格后可估算最大可交易数量。";
  }
  return "根据当前账户、订单类型和价格估算最大可交易数量。";
}

export function resolveOrderSessionCaution(input: {
  supported: boolean;
  currentSession: string;
  orderSession: OrderSession;
  tradingEnvironment: string;
}): string {
  if (!input.supported) return "";
  const currentSession = input.currentSession.trim().toLowerCase();
  if (
    input.orderSession === "RTH" &&
    ["pre", "after", "overnight"].includes(currentSession)
  ) {
    return "当前不是常规交易时段，RTH 订单通常要等盘中才会撮合。";
  }
  if (
    input.tradingEnvironment === "SIMULATE" &&
    input.orderSession === "OVERNIGHT"
  ) {
    return "模拟盘夜盘支持通常受限，提交成功也可能暂时不会成交。";
  }
  return "";
}

export function formatMetric(value: number | null | undefined): string {
  if (value == null) return "—";
  return new Intl.NumberFormat("zh-CN", {
    maximumFractionDigits: 4,
  }).format(value);
}

export function countDecimalPlaces(value: number): number {
  const text = value.toString().toLowerCase();
  if (!text.includes("e")) {
    return text.includes(".") ? (text.split(".")[1] ?? "").length : 0;
  }
  const [, exponentText] = text.split("e-");
  return Number.parseInt(exponentText ?? "0", 10) || 0;
}

export function alignPriceToStep(value: number, step: number): number {
  if (!Number.isFinite(value) || value <= 0) return 0;
  const decimals = Math.min(8, countDecimalPlaces(step));
  return Number((Math.round(value / step) * step).toFixed(decimals));
}

export function formatOrderSession(session: string): string {
  const normalized = session.trim().toUpperCase();
  if (normalized === "RTH") return "常规交易时段（RTH）";
  if (normalized === "ETH") return "扩展交易时段（ETH）";
  if (normalized === "ALL") return "全时段（ALL）";
  if (normalized === "OVERNIGHT") return "夜盘（OVERNIGHT）";
  return session;
}

export function formatInitialMargin(value: number | null | undefined): string {
  return value == null ? "股票通常不返回" : formatMetric(value);
}

export function createClientOrderId(): string {
  const suffix =
    typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `jftrade-${suffix}`;
}

export function resolvePendingOrderSummary(
  payload: ExecutionOrderPayload,
): string {
  const parts = [
    `${formatOrderSideLabel(payload.side)} ${payload.quantity} ${formatInstrumentIdentityText({
      market: payload.market,
      code: payload.code,
      instrumentId: payload.symbol,
    })}`,
    formatOrderTypeLabel(payload.orderType),
    formatTimeInForceLabel(payload.timeInForce),
  ];
  if (payload.price != null) parts.push(`限价 ${payload.price}`);
  if (payload.stopPrice != null) parts.push(`止损价 ${payload.stopPrice}`);
  if (payload.session != null) parts.push(formatOrderSession(payload.session));
  return parts.join(" / ");
}

export function resolveOrderFailureReason(error: unknown): string {
  if (error instanceof Error && error.message.trim() !== "") {
    return error.message.trim();
  }
  return "下单请求失败，请稍后重试。";
}

export function normalizeOptionalText(
  value: string | null | undefined,
): string | null {
  const trimmed = value?.trim() ?? "";
  return trimmed === "" ? null : trimmed;
}

export function orderFeedbackAccountHref(feedback: OrderFeedback): string {
  if (feedback.internalOrderId == null) return "/account";
  const params = new URLSearchParams();
  params.set("tab", "history");
  params.set("orderId", feedback.internalOrderId);
  return `/account?${params.toString()}`;
}

export function canCancelFeedbackOrder(feedback: OrderFeedback): boolean {
  if (feedback.level !== "success" || feedback.internalOrderId == null) {
    return false;
  }
  const status = feedback.orderStatus?.trim();
  return status == null || status === "" || !isFinalExecutionOrderStatus(status);
}

export function formatFeedbackOrderStatus(feedback: OrderFeedback): string {
  if (feedback.orderStatus == null) {
    return feedback.level === "success" ? "待券商回报" : "未接受";
  }
  return formatExecutionOrderStatusLabel(feedback.orderStatus);
}

export function formatBrokerAcceptance(feedback: OrderFeedback): string {
  const status = feedback.orderStatus?.trim().toUpperCase() ?? "";
  if (
    [
      "BROKER_ACCEPTED",
      "PARTIALLY_FILLED",
      "FILLED",
      "CANCEL_REQUESTED",
      "CANCELLED",
    ].includes(status)
  ) {
    return "已接受";
  }
  if (status === "REJECTED" || status === "EXPIRED") return "未接受";
  return "待确认";
}

export function formatFeedbackCheckedAt(value: string | null): string {
  if (value == null || value.trim() === "") return "";
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime())
    ? value
    : parsed.toLocaleTimeString("zh-CN", { hour12: false });
}
