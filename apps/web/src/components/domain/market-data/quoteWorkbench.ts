import {
  KLINE_PERIODS,
  normalizeKlinePeriod,
} from "../../../charting/kline";

export type QuoteWorkbenchKind = "instrument" | "plate";

export type QuoteWorkbenchProductClass =
  | "equity"
  | "fund"
  | "index"
  | "warrant"
  | "cbbc"
  | "plate"
  | "unknown";

export type QuoteWorkbenchPeriod = (typeof KLINE_PERIODS)[number]["value"];

export type QuoteWorkbenchTab = "quote" | "news";

export interface QuoteWorkbenchTarget {
  kind: QuoteWorkbenchKind;
  instrumentId: string;
  name: string;
  productClass: QuoteWorkbenchProductClass;
}

const productClassAliases: Readonly<Record<string, QuoteWorkbenchProductClass>> = {
  equity: "equity",
  stock: "equity",
  fund: "fund",
  etf: "fund",
  trust: "fund",
  index: "index",
  warrant: "warrant",
  cbbc: "cbbc",
  plate: "plate",
};

const quoteWorkbenchPeriods = new Set<string>(
  KLINE_PERIODS.map((period) => period.value),
);

const legacyQuoteWorkbenchPeriods: Readonly<Record<string, QuoteWorkbenchPeriod>> = {
  "five-day": "1d",
  day: "1d",
  week: "1w",
  month: "1mo",
};

export function normalizeQuoteWorkbenchProductClass(
  value: unknown,
): QuoteWorkbenchProductClass {
  const normalized =
    typeof value === "string" ? value.trim().toLowerCase() : "";
  return productClassAliases[normalized] ?? "unknown";
}

export function isQuoteWorkbenchPeriod(
  value: unknown,
): value is QuoteWorkbenchPeriod {
  return quoteWorkbenchPeriods.has(String(value));
}

export function normalizeQuoteWorkbenchPeriod(
  value: unknown,
  fallback: QuoteWorkbenchPeriod = "1d",
): QuoteWorkbenchPeriod {
  const candidate = String(value ?? "").trim();
  const legacy = legacyQuoteWorkbenchPeriods[candidate.toLowerCase()];
  if (legacy != null) return legacy;

  try {
    const normalized = normalizeKlinePeriod(candidate);
    return isQuoteWorkbenchPeriod(normalized) ? normalized : fallback;
  } catch {
    return fallback;
  }
}

export function isQuoteWorkbenchTab(
  value: unknown,
): value is QuoteWorkbenchTab {
  return value === "quote" || value === "news";
}
