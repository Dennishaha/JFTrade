import { dayKeyOf } from "./researchEntry";

export type EarningsCalendarMode = "day" | "week" | "month";
export type EarningsCalendarSort =
  | "hot"
  | "market_cap"
  | "option_volume"
  | "iv"
  | "iv_rank"
  | "iv_percentile";
export type EarningsCalendarStockScope =
  | "all"
  | "watchlist"
  | "position"
  | "special";

export interface EarningsCalendarFilters {
  stockScope: EarningsCalendarStockScope;
  marketCapMin: string;
  marketCapMax: string;
  optionVolumeMin: string;
  optionVolumeMax: string;
  ivMin: string;
  ivMax: string;
  ivRankMin: string;
  ivRankMax: string;
  ivPercentileMin: string;
  ivPercentileMax: string;
}

export interface EarningsCalendarDay {
  key: string;
  weekday: string;
  dayOfMonth: number;
  currentMonth: boolean;
  today: boolean;
}

export interface EarningsCalendarRange {
  beginDate: string;
  endDate: string;
  days: EarningsCalendarDay[];
}

export interface EarningsCalendarFilterValidation {
  valid: boolean;
  errors: Record<string, string>;
}

export const EARNINGS_CALENDAR_SORT_OPTIONS: ReadonlyArray<{
  value: EarningsCalendarSort;
  label: string;
  optionOnly: boolean;
}> = [
  { value: "hot", label: "热门", optionOnly: false },
  { value: "market_cap", label: "市值", optionOnly: false },
  { value: "option_volume", label: "期权成交量", optionOnly: true },
  { value: "iv", label: "隐含波动率", optionOnly: true },
  { value: "iv_rank", label: "隐含波动率等级", optionOnly: true },
  { value: "iv_percentile", label: "隐含波动率百分位数", optionOnly: true },
] as const;

const WEEKDAYS = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"] as const;
const MARKET_CAP_SCALE = 100_000_000;
const OPTION_VOLUME_SCALE = 10_000;

const RANGE_DEFINITIONS: ReadonlyArray<{
  minKey: keyof EarningsCalendarFilters;
  maxKey: keyof EarningsCalendarFilters;
  label: string;
  percentage?: boolean;
  optionOnly?: boolean;
}> = [
  { minKey: "marketCapMin", maxKey: "marketCapMax", label: "市值" },
  {
    minKey: "optionVolumeMin",
    maxKey: "optionVolumeMax",
    label: "期权成交量",
    optionOnly: true,
  },
  {
    minKey: "ivMin",
    maxKey: "ivMax",
    label: "隐含波动率",
    percentage: true,
    optionOnly: true,
  },
  {
    minKey: "ivRankMin",
    maxKey: "ivRankMax",
    label: "IV 等级",
    percentage: true,
    optionOnly: true,
  },
  {
    minKey: "ivPercentileMin",
    maxKey: "ivPercentileMax",
    label: "IV 百分位数",
    percentage: true,
    optionOnly: true,
  },
] as const;

export function createEarningsCalendarFilters(): EarningsCalendarFilters {
  return {
    stockScope: "all",
    marketCapMin: "",
    marketCapMax: "",
    optionVolumeMin: "",
    optionVolumeMax: "",
    ivMin: "",
    ivMax: "",
    ivRankMin: "",
    ivRankMax: "",
    ivPercentileMin: "",
    ivPercentileMax: "",
  };
}

export function isEarningsOptionMarket(market: string): boolean {
  const normalized = market.trim().toUpperCase();
  return normalized === "US" || normalized === "HK";
}

export function clearIncompatibleEarningsFilters(
  filters: EarningsCalendarFilters,
  market: string,
): EarningsCalendarFilters {
  if (isEarningsOptionMarket(market)) return { ...filters };
  return {
    ...filters,
    optionVolumeMin: "",
    optionVolumeMax: "",
    ivMin: "",
    ivMax: "",
    ivRankMin: "",
    ivRankMax: "",
    ivPercentileMin: "",
    ivPercentileMax: "",
  };
}

export function earningsCalendarRange(
  mode: EarningsCalendarMode,
  anchorKey: string,
  todayKey = dayKeyOf(new Date()),
): EarningsCalendarRange {
  const anchor = parseDayKey(anchorKey);
  let begin = new Date(anchor);
  let end = new Date(anchor);

  if (mode === "week") {
    begin.setDate(begin.getDate() - begin.getDay());
    end = new Date(begin);
    end.setDate(end.getDate() + 6);
  } else if (mode === "month") {
    const month = anchor.getMonth();
    const first = new Date(anchor.getFullYear(), month, 1, 12);
    begin = new Date(first);
    begin.setDate(begin.getDate() - begin.getDay());
    const last = new Date(anchor.getFullYear(), month + 1, 0, 12);
    end = new Date(last);
    end.setDate(end.getDate() + (6 - end.getDay()));
    const naturalDays = dayDistance(begin, end) + 1;
    if (naturalDays < 35) end.setDate(end.getDate() + 7);
  }

  const days: EarningsCalendarDay[] = [];
  for (const cursor = new Date(begin); !isAfterDay(cursor, end); cursor.setDate(cursor.getDate() + 1)) {
    const key = dayKeyOf(cursor);
    days.push({
      key,
      weekday: WEEKDAYS[cursor.getDay()]!,
      dayOfMonth: cursor.getDate(),
      currentMonth: cursor.getMonth() === anchor.getMonth(),
      today: key === todayKey,
    });
  }
  return {
    beginDate: dayKeyOf(begin),
    endDate: dayKeyOf(end),
    days,
  };
}

export function moveEarningsCalendarAnchor(
  mode: EarningsCalendarMode,
  anchorKey: string,
  direction: -1 | 1,
): string {
  const date = parseDayKey(anchorKey);
  if (mode === "day") {
    date.setDate(date.getDate() + direction);
  } else if (mode === "week") {
    date.setDate(date.getDate() + direction * 7);
  } else {
    const originalDay = date.getDate();
    date.setDate(1);
    date.setMonth(date.getMonth() + direction);
    const lastDay = new Date(date.getFullYear(), date.getMonth() + 1, 0, 12).getDate();
    date.setDate(Math.min(originalDay, lastDay));
  }
  return dayKeyOf(date);
}

export function earningsCalendarPeriodLabel(
  mode: EarningsCalendarMode,
  anchorKey: string,
): string {
  const range = earningsCalendarRange(mode, anchorKey);
  if (mode === "day") return anchorKey.replaceAll("-", "/");
  if (mode === "month") return anchorKey.slice(0, 7).replace("-", "/");
  const begin = range.beginDate;
  const end = range.endDate;
  if (begin.slice(0, 4) === end.slice(0, 4)) {
    return `${begin.replaceAll("-", "/")} – ${end.slice(5).replace("-", "/")}`;
  }
  return `${begin.replaceAll("-", "/")} – ${end.replaceAll("-", "/")}`;
}

export function earningsCalendarFilterCount(filters: EarningsCalendarFilters): number {
  let count = filters.stockScope === "all" ? 0 : 1;
  for (const definition of RANGE_DEFINITIONS) {
    if (
      String(filters[definition.minKey] ?? "").trim() !== "" ||
      String(filters[definition.maxKey] ?? "").trim() !== ""
    ) {
      count += 1;
    }
  }
  return count;
}

export function validateEarningsCalendarFilters(
  filters: EarningsCalendarFilters,
  market: string,
): EarningsCalendarFilterValidation {
  const errors: Record<string, string> = {};
  const optionMarket = isEarningsOptionMarket(market);

  for (const definition of RANGE_DEFINITIONS) {
    if (definition.optionOnly && !optionMarket) continue;
    const min = parseFilterNumber(filters[definition.minKey]);
    const max = parseFilterNumber(filters[definition.maxKey]);
    if (min.invalid) errors[definition.minKey] = "请输入非负数字";
    if (max.invalid) errors[definition.maxKey] = "请输入非负数字";
    if (definition.percentage) {
      if (min.value != null && min.value > 100) errors[definition.minKey] = "不能超过 100";
      if (max.value != null && max.value > 100) errors[definition.maxKey] = "不能超过 100";
    }
    if (!min.invalid && !max.invalid && min.value != null && max.value != null && min.value > max.value) {
      errors[definition.maxKey] = `${definition.label}上限不能小于下限`;
    }
  }

  return { valid: Object.keys(errors).length === 0, errors };
}

export function buildEarningsCalendarPath(options: {
  market: string;
  range: Pick<EarningsCalendarRange, "beginDate" | "endDate">;
  sort: EarningsCalendarSort;
  filters: EarningsCalendarFilters;
}): string {
  const { market, range, sort } = options;
  const filters = clearIncompatibleEarningsFilters(options.filters, market);
  const params = new URLSearchParams({
    market,
    operation: "earnings",
    beginDate: range.beginDate,
    endDate: range.endDate,
    sort,
  });
  if (filters.stockScope !== "all") params.set("stockScope", filters.stockScope);

  appendScaledRange(params, filters, "marketCapMin", "marketCapMax", MARKET_CAP_SCALE);
  if (isEarningsOptionMarket(market)) {
    appendScaledRange(params, filters, "optionVolumeMin", "optionVolumeMax", OPTION_VOLUME_SCALE);
    appendScaledRange(params, filters, "ivMin", "ivMax", 1);
    appendScaledRange(params, filters, "ivRankMin", "ivRankMax", 1);
    appendScaledRange(params, filters, "ivPercentileMin", "ivPercentileMax", 1);
  }
  return `/api/v1/research/calendars?${params.toString()}`;
}

function appendScaledRange(
  params: URLSearchParams,
  filters: EarningsCalendarFilters,
  minKey: keyof EarningsCalendarFilters,
  maxKey: keyof EarningsCalendarFilters,
  scale: number,
): void {
  for (const key of [minKey, maxKey]) {
    const parsed = parseFilterNumber(filters[key]);
    if (parsed.value != null && !parsed.invalid) {
      params.set(key, String(parsed.value * scale));
    }
  }
}

function parseFilterNumber(raw: string): { value: number | null; invalid: boolean } {
  // Vue normalizes values emitted by native number inputs to numbers in some
  // runtimes even when the declared draft shape is string-based.
  const value = String(raw ?? "").trim();
  if (value === "") return { value: null, invalid: false };
  const parsed = Number(value);
  return {
    value: Number.isFinite(parsed) ? parsed : null,
    invalid: !Number.isFinite(parsed) || parsed < 0,
  };
}

function parseDayKey(value: string): Date {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (match == null) return new Date();
  return new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]), 12);
}

function dayDistance(begin: Date, end: Date): number {
  const beginUTC = Date.UTC(begin.getFullYear(), begin.getMonth(), begin.getDate());
  const endUTC = Date.UTC(end.getFullYear(), end.getMonth(), end.getDate());
  return Math.round((endUTC - beginUTC) / 86_400_000);
}

function isAfterDay(left: Date, right: Date): boolean {
  return dayDistance(right, left) > 0;
}
