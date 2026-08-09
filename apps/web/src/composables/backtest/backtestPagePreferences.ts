import {
  KLINE_PERIODS,
  normalizeChartType,
  type ChartType,
} from "@/charting/kline";
import { categoryMarketForUser } from "@/composables/market-data/instrumentPresentation";
import { normalizeBacktestDateLabel } from "@/pages/backtestTimeWindow";
import type { BacktestFeeRulePayload } from "@/types";
import { formatLocalDate } from "@/utils/dateTime";

export const BACKTEST_FORM_STORAGE_KEY = "jftrade.backtest.form.v1";

export const BACKTEST_RESULT_STATUS_OPTIONS = [
  { value: "all", title: "全部状态" },
  { value: "queued", title: "排队中" },
  { value: "running", title: "运行中" },
  { value: "completed", title: "已完成" },
  { value: "failed", title: "失败" },
  { value: "cancelled", title: "已取消" },
];

export const BACKTEST_BROKER_FEE_MODE_OPTIONS = [
  { value: "market_preset", title: "市场预设" },
  { value: "script", title: "脚本" },
  { value: "custom", title: "自定义" },
  { value: "none", title: "关闭" },
];

export const BACKTEST_MARKET_FEE_MODE_OPTIONS = [
  { value: "market_preset", title: "市场预设" },
  { value: "custom", title: "自定义" },
  { value: "none", title: "关闭" },
];

export const EXTENDED_HOURS_INTERVALS = new Set([
  "1m",
  "5m",
  "15m",
  "30m",
  "1h",
  "2h",
  "4h",
  "6h",
  "12h",
  "1d",
  "1w",
  "1mo",
]);

export interface StoredBacktestFormPreferences {
  selectedDefinitionId: string;
  selectedMarket: string;
  codeInput: string;
  interval: string;
  chartType: ChartType;
  startDate: string;
  endDate: string;
  initialBalance: number;
  instrumentType: string;
  rehabType: string;
  useExtendedHours: boolean;
  brokerFeeMode: "market_preset" | "custom" | "script" | "none";
  marketFeeMode: "market_preset" | "custom" | "none";
  brokerFeeRulesText: string;
  marketFeeRulesText: string;
}

function defaultBacktestFormPreferences(): StoredBacktestFormPreferences {
  const today = new Date();
  const startYear = today.getFullYear() - 3;
  const daysInStartMonth = new Date(
    startYear,
    today.getMonth() + 1,
    0,
  ).getDate();
  const defaultStartDate = formatLocalDate(
    new Date(
      startYear,
      today.getMonth(),
      Math.min(today.getDate(), daysInStartMonth),
    ),
  );
  return {
    selectedDefinitionId: "",
    selectedMarket: "HK",
    codeInput: "00700",
    interval: "5m",
    chartType: "standard",
    startDate: defaultStartDate,
    endDate: formatLocalDate(today),
    initialBalance: 1000000,
    instrumentType: "stock",
    rehabType: "forward",
    useExtendedHours: false,
    brokerFeeMode: "market_preset",
    marketFeeMode: "market_preset",
    brokerFeeRulesText: "",
    marketFeeRulesText: "",
  };
}

export function readStoredBacktestFormPreferences(): StoredBacktestFormPreferences {
  const defaults = defaultBacktestFormPreferences();
  if (typeof window === "undefined" || window.localStorage == null) {
    return defaults;
  }
  try {
    const raw = window.localStorage.getItem(BACKTEST_FORM_STORAGE_KEY);
    if (raw == null || raw.trim() === "") return defaults;
    const parsed = JSON.parse(raw) as Partial<StoredBacktestFormPreferences>;
    const validIntervals = new Set<string>(KLINE_PERIODS.map((period) => period.value));
    const normalizeDate = (value: unknown, fallback: string) => {
      const normalized = normalizeBacktestDateLabel(
        typeof value === "string" ? value : "",
      );
      return normalized === "" ? fallback : normalized;
    };
    const storedMarket = normalizeString(
      parsed.selectedMarket,
      defaults.selectedMarket,
    ).toUpperCase();
    let storedCode = normalizeString(
      parsed.codeInput,
      defaults.codeInput,
    ).toUpperCase().replace(":", ".");
    if (
      (storedMarket === "SH" || storedMarket === "SZ") &&
      !storedCode.includes(".")
    ) {
      storedCode = `${storedMarket}.${storedCode}`;
    }
    return {
      selectedDefinitionId: normalizeString(parsed.selectedDefinitionId, ""),
      selectedMarket: categoryMarketForUser(storedMarket),
      codeInput: storedCode,
      interval: validIntervals.has(normalizeString(parsed.interval, ""))
        ? normalizeString(parsed.interval, "")
        : defaults.interval,
      chartType: normalizeChartType(parsed.chartType),
      startDate: normalizeDate(parsed.startDate, defaults.startDate),
      endDate: normalizeDate(parsed.endDate, defaults.endDate),
      initialBalance:
        typeof parsed.initialBalance === "number" &&
        Number.isFinite(parsed.initialBalance) &&
        parsed.initialBalance > 0
          ? parsed.initialBalance
          : defaults.initialBalance,
      instrumentType: normalizedChoice(
        parsed.instrumentType,
        ["stock", "etf"],
        defaults.instrumentType,
      ),
      rehabType: normalizedChoice(
        parsed.rehabType,
        ["forward", "backward", "none"],
        defaults.rehabType,
      ),
      useExtendedHours: parsed.useExtendedHours === true,
      brokerFeeMode: normalizedChoice(
        parsed.brokerFeeMode,
        ["market_preset", "custom", "script", "none"] as const,
        defaults.brokerFeeMode,
      ),
      marketFeeMode: normalizedChoice(
        parsed.marketFeeMode,
        ["market_preset", "custom", "none"] as const,
        defaults.marketFeeMode,
      ),
      brokerFeeRulesText:
        typeof parsed.brokerFeeRulesText === "string"
          ? parsed.brokerFeeRulesText
          : defaults.brokerFeeRulesText,
      marketFeeRulesText:
        typeof parsed.marketFeeRulesText === "string"
          ? parsed.marketFeeRulesText
          : defaults.marketFeeRulesText,
    };
  } catch {
    return defaults;
  }
}

function normalizeString(value: unknown, fallback: string): string {
  return typeof value === "string" && value.trim() !== ""
    ? value.trim()
    : fallback;
}

function normalizedChoice<const T extends string>(
  value: unknown,
  choices: readonly T[],
  fallback: T,
): T {
  const normalized = typeof value === "string" ? value.trim().toLowerCase() : "";
  return choices.includes(normalized as T) ? (normalized as T) : fallback;
}

export function writeStoredBacktestFormPreferences(
  preferences: StoredBacktestFormPreferences,
): void {
  if (typeof window === "undefined" || window.localStorage == null) return;
  window.localStorage.setItem(
    BACKTEST_FORM_STORAGE_KEY,
    JSON.stringify(preferences),
  );
}

export function canonicalBacktestInstrumentInput(
  market: string,
  code: string,
): string {
  const normalizedMarket = market.trim().toUpperCase();
  const normalizedCode = code.trim().toUpperCase().replace(":", ".");
  if (normalizedCode === "" || normalizedCode.includes(".")) {
    return normalizedCode;
  }
  return normalizedMarket === "" ? normalizedCode : `${normalizedMarket}.${normalizedCode}`;
}

export function supportsExtendedHoursForInterval(
  market: string,
  interval: string,
  supportsMarket: (market: string) => boolean,
): boolean {
  return (
    supportsMarket(market) &&
    EXTENDED_HOURS_INTERVALS.has(interval.trim().toLowerCase())
  );
}

export function parseBacktestFeeRules(raw: string): BacktestFeeRulePayload[] {
  if (raw.trim() === "") return [];
  try {
    const parsed = JSON.parse(raw) as unknown;
    return Array.isArray(parsed) ? (parsed as BacktestFeeRulePayload[]) : [];
  } catch {
    return [];
  }
}
