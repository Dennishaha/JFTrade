export const EMPTY_NUMERIC_TEXT = "—";

export interface NumericFormatOptions {
  fallback?: string;
  locale?: string;
  maximumFractionDigits?: number;
  minimumFractionDigits?: number;
  useGrouping?: boolean;
}

export interface MarketPriceFormatOptions extends NumericFormatOptions {
  market?: string | null;
  precision?: number | null;
}

export interface PercentFormatOptions extends NumericFormatOptions {
  input?: "percent" | "ratio";
  showPositiveSign?: boolean;
}

const DEFAULT_LOCALE = "zh-CN";
const formatterCache = new Map<string, Intl.NumberFormat>();

const MARKET_PRICE_PRECISION: Record<string, number> = {
  US: 2,
  NYSE: 2,
  NASDAQ: 2,
  HK: 3,
  HKEX: 3,
  CN: 2,
  SH: 2,
  SZ: 2,
  CNSH: 2,
  CNSZ: 2,
};

function finiteNumber(value: number | null | undefined): number | null {
  if (value == null || !Number.isFinite(value)) return null;
  return Object.is(value, -0) ? 0 : value;
}

function normalizeFractionDigits(value: number | null | undefined): number | null {
  if (value == null || !Number.isFinite(value)) return null;
  return Math.min(20, Math.max(0, Math.trunc(value)));
}

function numberFormatter(options: Required<Pick<
  NumericFormatOptions,
  "locale" | "maximumFractionDigits" | "minimumFractionDigits" | "useGrouping"
>>): Intl.NumberFormat {
  const key = [
    options.locale,
    options.minimumFractionDigits,
    options.maximumFractionDigits,
    options.useGrouping ? "grouped" : "plain",
  ].join("|");
  const cached = formatterCache.get(key);
  if (cached != null) return cached;
  const formatter = new Intl.NumberFormat(options.locale, {
    minimumFractionDigits: options.minimumFractionDigits,
    maximumFractionDigits: options.maximumFractionDigits,
    useGrouping: options.useGrouping,
  });
  formatterCache.set(key, formatter);
  return formatter;
}

export function formatNumber(
  value: number | null | undefined,
  options: NumericFormatOptions = {},
): string {
  const normalized = finiteNumber(value);
  if (normalized == null) return options.fallback ?? EMPTY_NUMERIC_TEXT;
  const maximumFractionDigits =
    normalizeFractionDigits(options.maximumFractionDigits) ?? 4;
  const minimumFractionDigits = Math.min(
    normalizeFractionDigits(options.minimumFractionDigits) ?? 0,
    maximumFractionDigits,
  );
  return numberFormatter({
    locale: options.locale ?? DEFAULT_LOCALE,
    maximumFractionDigits,
    minimumFractionDigits,
    useGrouping: options.useGrouping ?? true,
  }).format(normalized);
}

export function marketPricePrecision(
  market: string | null | undefined,
): number | null {
  const normalized = (market ?? "").trim().toUpperCase();
  return MARKET_PRICE_PRECISION[normalized] ?? null;
}

function adaptivePricePrecision(value: number): number {
  const absolute = Math.abs(value);
  if (absolute >= 1_000) return 2;
  if (absolute >= 1) return 3;
  return 5;
}

export function formatMarketPrice(
  value: number | null | undefined,
  options: MarketPriceFormatOptions = {},
): string {
  const normalized = finiteNumber(value);
  if (normalized == null) return options.fallback ?? EMPTY_NUMERIC_TEXT;
  const precision =
    normalizeFractionDigits(options.precision) ??
    marketPricePrecision(options.market) ??
    adaptivePricePrecision(normalized);
  return formatNumber(normalized, {
    ...options,
    minimumFractionDigits: precision,
    maximumFractionDigits: precision,
  });
}

export function formatQuantity(
  value: number | null | undefined,
  options: NumericFormatOptions = {},
): string {
  return formatNumber(value, {
    maximumFractionDigits: 8,
    ...options,
  });
}

export function formatCompactNumber(
  value: number | null | undefined,
  options: NumericFormatOptions = {},
): string {
  const normalized = finiteNumber(value);
  if (normalized == null) return options.fallback ?? EMPTY_NUMERIC_TEXT;
  const absolute = Math.abs(normalized);
  if (absolute >= 1_000_000_000) {
    return `${formatNumber(normalized / 1_000_000_000, {
      ...options,
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    })}B`;
  }
  if (absolute >= 1_000_000) {
    return `${formatNumber(normalized / 1_000_000, {
      ...options,
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    })}M`;
  }
  if (absolute >= 1_000) {
    return `${formatNumber(normalized / 1_000, {
      ...options,
      minimumFractionDigits: 1,
      maximumFractionDigits: 1,
    })}K`;
  }
  return formatNumber(normalized, {
    ...options,
    maximumFractionDigits: 0,
  });
}

export function formatPercent(
  value: number | null | undefined,
  options: PercentFormatOptions = {},
): string {
  const normalized = finiteNumber(value);
  if (normalized == null) return options.fallback ?? EMPTY_NUMERIC_TEXT;
  const percent = options.input === "ratio" ? normalized * 100 : normalized;
  const precision = normalizeFractionDigits(options.maximumFractionDigits) ?? 2;
  const prefix = options.showPositiveSign && percent > 0 ? "+" : "";
  return `${prefix}${formatNumber(percent, {
    ...options,
    minimumFractionDigits:
      normalizeFractionDigits(options.minimumFractionDigits) ?? precision,
    maximumFractionDigits: precision,
    useGrouping: options.useGrouping ?? false,
  })}%`;
}

export function formatMoney(
  value: number | null | undefined,
  currency?: string | null,
  options: NumericFormatOptions = {},
): string {
  const formatted = formatNumber(value, options);
  if (formatted === (options.fallback ?? EMPTY_NUMERIC_TEXT)) return formatted;
  const normalizedCurrency = (currency ?? "").trim();
  return normalizedCurrency === "" ? formatted : `${formatted} ${normalizedCurrency}`;
}
