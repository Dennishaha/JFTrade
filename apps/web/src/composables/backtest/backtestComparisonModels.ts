import {
  formatBacktestRehabType,
  formatBacktestRunDate,
} from "@/components/backtest/backtestRunPresentation";
import type { BacktestTradingCostsPayload } from "@/types";
import { formatNumber, formatPercent } from "@/utils/numberFormat";
import type { BacktestRun } from "./backtestRunModels";

export interface ComparisonMetric {
  label: string;
  kind: "currency" | "number" | "percent";
  left: number | undefined;
  right: number | undefined;
}

export interface ComparisonConfigRow {
  label: string;
  left: string;
  right: string;
  same: boolean;
}

export function formatComparisonCurrency(
  value: number | undefined,
  currency: string,
): string {
  if (value == null || !Number.isFinite(value)) return "--";
  const rendered = formatNumber(value, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
  return currency === "" ? rendered : `${rendered} ${currency}`;
}

export function formatComparisonMetric(
  value: number | undefined,
  kind: ComparisonMetric["kind"],
  currency = "",
): string {
  if (value == null || !Number.isFinite(value)) return "--";
  if (kind === "percent") return formatPercent(value, { input: "ratio" });
  if (kind === "currency") return formatComparisonCurrency(value, currency);
  return formatNumber(value, { maximumFractionDigits: 2 });
}

export function buildComparisonMetrics(
  left: BacktestRun | undefined,
  right: BacktestRun | undefined,
): ComparisonMetric[] {
  return [
    {
      label: "最终资金",
      kind: "currency",
      left: left?.result?.finalBalance,
      right: right?.result?.finalBalance,
    },
    {
      label: "收益",
      kind: "currency",
      left: left?.result?.pnl,
      right: right?.result?.pnl,
    },
    {
      label: "最大回撤",
      kind: "percent",
      left: left?.result?.maxDrawdown,
      right: right?.result?.maxDrawdown,
    },
    {
      label: "当前回撤",
      kind: "percent",
      left: left?.result?.currentDrawdown,
      right: right?.result?.currentDrawdown,
    },
    {
      label: "交易数",
      kind: "number",
      left: left?.result?.totalTrades,
      right: right?.result?.totalTrades,
    },
    {
      label: "胜率",
      kind: "percent",
      left: left?.result?.winRate,
      right: right?.result?.winRate,
    },
    {
      label: "总费用",
      kind: "currency",
      left: left?.result?.totalFees,
      right: right?.result?.totalFees,
    },
  ];
}

export function comparisonFeeConfig(run: BacktestRun): string {
  const costs = run.result?.tradingCosts ?? run.request.tradingCosts;
  const schedule = (
    entry: BacktestTradingCostsPayload["brokerFees"] | undefined,
  ): string => {
    if (entry == null) return "market_preset";
    const mode = entry.mode ?? "market_preset";
    return entry.presetId ? `${mode}:${entry.presetId}` : mode;
  };
  return `券商 ${schedule(costs?.brokerFees)} / 市场 ${schedule(costs?.marketFees)}`;
}

export function comparisonChartType(run: BacktestRun): string {
  return (run.result?.chartType ?? run.request.chartType) === "heikinashi"
    ? "Heikin Ashi"
    : "标准K线";
}

export function compareConfigValue(
  left: string,
  right: string,
): ComparisonConfigRow {
  return { label: "", left, right, same: left === right };
}

interface ComparisonConfigInput {
  left: BacktestRun | undefined;
  right: BacktestRun | undefined;
  resolveQuoteCurrency: (run: BacktestRun) => string;
  resolveSessionMode: (run: BacktestRun) => string;
}

export function buildComparisonConfigRows(
  input: ComparisonConfigInput,
): ComparisonConfigRow[] {
  const { left, right } = input;
  if (left == null || right == null) return [];
  const rows: Array<[string, string, string]> = [
    ["标的", left.request.symbol, right.request.symbol],
    ["周期", left.request.interval, right.request.interval],
    [
      "日期",
      `${formatBacktestRunDate(left.request.startDate)} → ${formatBacktestRunDate(left.request.endDate)}`,
      `${formatBacktestRunDate(right.request.startDate)} → ${formatBacktestRunDate(right.request.endDate)}`,
    ],
    [
      "初始资金",
      formatComparisonCurrency(
        left.request.initialBalance,
        input.resolveQuoteCurrency(left),
      ),
      formatComparisonCurrency(
        right.request.initialBalance,
        input.resolveQuoteCurrency(right),
      ),
    ],
    [
      "复权",
      formatBacktestRehabType(left.request.rehabType),
      formatBacktestRehabType(right.request.rehabType),
    ],
    [
      "交易时段",
      input.resolveSessionMode(left),
      input.resolveSessionMode(right),
    ],
    ["图表类型", comparisonChartType(left), comparisonChartType(right)],
    ["费用规则", comparisonFeeConfig(left), comparisonFeeConfig(right)],
    [
      "执行模型",
      left.result?.executionModel ?? left.request.executionModel ?? "默认",
      right.result?.executionModel ?? right.request.executionModel ?? "默认",
    ],
  ];
  return rows.map(([label, leftValue, rightValue]) => ({
    label,
    left: leftValue,
    right: rightValue,
    same: leftValue === rightValue,
  }));
}
