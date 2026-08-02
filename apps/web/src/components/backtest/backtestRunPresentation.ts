import { formatLocalDateTime } from "@/utils/dateTime";
import { formatNumber, formatPercent } from "@/utils/numberFormat";
import { normalizeBacktestDateLabel } from "@/pages/backtestTimeWindow";

export type BacktestReportTab = "chart" | "orders" | "properties";

export function isTerminalBacktestStatus(status: string): boolean {
  return status === "completed" || status === "failed" || status === "cancelled";
}

/** 回测运行中/排队进度条的 Vuetify 配色（v-progress-linear 非 chip 形态，本地域专属）。 */
export function backtestRunProgressColor(status: string): string {
  return status === "running" ? "teal" : "warning";
}

export function formatBacktestRehabType(rehabType: string | undefined): string {
  switch ((rehabType ?? "forward").trim().toLowerCase()) {
    case "none":
      return "不复权";
    case "backward":
      return "后复权";
    default:
      return "前复权";
  }
}

export function resolveBacktestPriceBasisNote(run: {
  request: { rehabType?: string; interval: string };
}): string {
  const rehabLabel = formatBacktestRehabType(run.request.rehabType);
  const intervalLabel = run.request.interval.trim() || "当前周期";
  if ((run.request.rehabType ?? "forward").trim().toLowerCase() === "none") {
    return `价格口径：图表显示的是 ${intervalLabel} 已闭合历史 K 线；若和当前盘后/夜盘快照不同，通常是因为快照展示的是最新成交，而不是最后一根已闭合 bar。`;
  }
  return `价格口径：图表显示的是${rehabLabel}${intervalLabel}已闭合历史 K 线；不要直接和实时盘后/夜盘快照比较，后者通常是不复权的最新成交。`;
}

export function pnlColor(value: number): string {
  return value >= 0 ? "tv-up" : "tv-down";
}

export function pnlPrefix(value: number): string {
  return value >= 0 ? "+" : "";
}

export function usesClosedTradeStats(result: {
  tradeStatsVersion?: number | undefined;
}): boolean {
  return result.tradeStatsVersion === 2;
}

export function backtestFillCount(result: {
  totalFills?: number | undefined;
  totalTrades: number;
  trades?: readonly unknown[] | undefined;
}): number {
  if (Number.isFinite(result.totalFills) && (result.totalFills ?? 0) >= 0) {
    return Math.trunc(result.totalFills ?? 0);
  }
  if (result.trades != null) return result.trades.length;
  return Number.isFinite(result.totalTrades) && result.totalTrades > 0
    ? Math.trunc(result.totalTrades)
    : 0;
}

export function drawdownColor(value: number | undefined): string {
  return (value ?? 0) > 0 ? "bt-metric-negative" : "bt-text";
}

export function formatPercentMetric(value: number | undefined): string {
  const normalized = Number.isFinite(value) ? (value ?? 0) : 0;
  return formatPercent(normalized, { input: "ratio" });
}

export function formatBacktestTimestamp(value?: string): string {
  return value ? formatLocalDateTime(value, "--") : "--";
}

export function formatBacktestRunDate(date: string | undefined): string {
  return normalizeBacktestDateLabel(date ?? "") || "--";
}

export function formatBacktestOrderSide(side: string): string {
  if (side === "BUY") return "买入";
  if (side === "SELL") return "卖出";
  return side;
}

export function formatBacktestOrderStatus(status: string): string {
  const labels: Record<string, string> = {
    NEW: "已下单",
    FILLED: "已成交",
    CANCELED: "已撤单",
    REJECTED: "已拒绝",
  };
  return labels[status] ?? status;
}

export function formatBacktestOrderPrice(
  value: number | undefined,
  orderType?: string,
  raw?: string,
): string {
  if (raw && raw.trim() !== "" && raw !== "0") return raw;
  if (value !== undefined && Number.isFinite(value) && value > 0) {
    return formatNumber(value, {
      minimumFractionDigits: 2,
      maximumFractionDigits: 4,
    });
  }
  return orderType === "MARKET" ? "市价" : "--";
}

export function formatBacktestQuantity(value: number | undefined, raw?: string): string {
  if (raw && raw.trim() !== "") return raw;
  if (value === undefined || !Number.isFinite(value)) return "--";
  return formatNumber(value, {
    minimumFractionDigits: Number.isInteger(value) ? 0 : 2,
    maximumFractionDigits: 4,
  });
}

export function formatBacktestFee(value: number | undefined, currency?: string): string {
  if (value === undefined || !Number.isFinite(value) || value <= 0) return "--";
  const amount = formatNumber(value, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  });
  return currency?.trim() ? `${amount} ${currency}` : amount;
}

export function runtimeErrorRepeatCount(
  result: { runtimeErrorCounts?: Record<string, number> | undefined },
  message: string,
): number {
  return result.runtimeErrorCounts?.[message] ?? 1;
}

export function runtimeErrorTotal(result: {
  runtimeErrors?: string[] | undefined;
  runtimeErrorTotal?: number | undefined;
}): number {
  return result.runtimeErrorTotal ?? result.runtimeErrors?.length ?? 0;
}

export function runtimeErrorSummary(result: {
  runtimeErrors?: string[] | undefined;
  runtimeErrorTotal?: number | undefined;
  runtimeErrorsTruncated?: boolean | undefined;
}): string {
  const shown = result.runtimeErrors?.length ?? 0;
  const total = runtimeErrorTotal(result);
  if (result.runtimeErrorsTruncated || total > shown) {
    return `运行时错误 ${total} 次，仅显示 ${shown} 条样本`;
  }
  return `运行时错误 (${total})`;
}

export function warningSummary(result: {
  warnings?: string[] | undefined;
  warningTotal?: number | undefined;
  warningsTruncated?: boolean | undefined;
  ignoredOrders?: number | undefined;
}): string {
  const shown = result.warnings?.length ?? 0;
  const total = warningTotal(result);
  const ignoredOrders = result.ignoredOrders ?? 0;
  const prefix = ignoredOrders > 0
    ? `回测警告 ${total} 条，忽略订单 ${ignoredOrders} 笔`
    : `回测警告 (${total})`;
  return result.warningsTruncated || total > shown
    ? `${prefix}，仅显示 ${shown} 条样本`
    : prefix;
}

export function warningTotal(result: {
  warnings?: string[] | undefined;
  warningTotal?: number | undefined;
}): number {
  return result.warningTotal ?? result.warnings?.length ?? 0;
}

type BacktestRunCollections = {
  result?: {
    orderBook?: unknown[] | undefined;
    runtimeErrors?: string[] | undefined;
    warnings?: string[] | undefined;
    logs?: string[] | undefined;
  } | null | undefined;
};

export function visibleBacktestOrderBook(run: BacktestRunCollections): unknown[] {
  return (run.result?.orderBook ?? []).slice(0, 200);
}

export function hiddenBacktestOrderBookCount(run: BacktestRunCollections): number {
  return Math.max(0, (run.result?.orderBook?.length ?? 0) - 200);
}

export function visibleBacktestRuntimeErrors(run: BacktestRunCollections): string[] {
  return (run.result?.runtimeErrors ?? []).slice(0, 120);
}

export function hiddenBacktestRuntimeErrorCount(run: BacktestRunCollections): number {
  return Math.max(0, (run.result?.runtimeErrors?.length ?? 0) - 120);
}

export function visibleBacktestWarnings(run: BacktestRunCollections): string[] {
  return (run.result?.warnings ?? []).slice(0, 120);
}

export function hiddenBacktestWarningCount(run: BacktestRunCollections): number {
  return Math.max(0, (run.result?.warnings?.length ?? 0) - 120);
}

export function visibleBacktestLogs(run: BacktestRunCollections): string[] {
  return (run.result?.logs ?? []).slice(0, 120);
}

export function hiddenBacktestLogCount(run: BacktestRunCollections): number {
  return Math.max(0, (run.result?.logs?.length ?? 0) - 120);
}

export function resolveQueriedCandleBounds(
  candles: Array<{ time: string }> | undefined,
): { left: string; right: string; count: number } | null {
  const sorted = [...(candles ?? [])]
    .filter((candle) => Number.isFinite(new Date(candle.time).getTime()))
    .sort((left, right) => new Date(left.time).getTime() - new Date(right.time).getTime());
  const first = sorted[0];
  const last = sorted.at(-1);
  return first && last
    ? {
        left: formatBacktestTimestamp(first.time),
        right: formatBacktestTimestamp(last.time),
        count: sorted.length,
      }
    : null;
}
