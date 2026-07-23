export type QuoteWorkbenchKind = "instrument" | "plate";

export type QuoteWorkbenchProductClass =
  | "equity"
  | "fund"
  | "index"
  | "warrant"
  | "cbbc"
  | "plate"
  | "unknown";

export type QuoteWorkbenchPeriod = "five-day" | "day" | "week" | "month";

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
  return ["five-day", "day", "week", "month"].includes(String(value));
}

export function isQuoteWorkbenchTab(
  value: unknown,
): value is QuoteWorkbenchTab {
  return value === "quote" || value === "news";
}
