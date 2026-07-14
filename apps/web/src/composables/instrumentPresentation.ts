import { formatMarketLabel } from "./consoleDataFormatting";

const A_SHARE_MARKETS = new Set(["CN", "SH", "SZ", "CNSH", "CNSZ"]);

const A_SHARE_EXCHANGE_TAGS: Record<string, string> = {
  SH: "上证",
  SZ: "深证",
  CNSH: "上证",
  CNSZ: "深证",
};

const SECURITY_TYPE_LABELS: Readonly<Record<string, string>> = {
  BOND: "债券",
  BWRT: "一揽子权证",
  CRYPTO: "数字货币",
  DRVT: "期权",
  EQTY: "股票",
  EQUITY: "股票",
  ETF: "ETF",
  FOREX: "外汇",
  FUND: "基金",
  FUTURE: "期货",
  INDEX: "指数",
  PLATE: "板块",
  PLATESET: "板块集",
  STOCK: "股票",
  TRUST: "基金/信托",
  UNKNOWN: "类型未知",
  WARRANT: "窝轮",
};

export interface InstrumentPresentationInput {
  market?: string | null | undefined;
  code?: string | null | undefined;
  instrumentId?: string | null | undefined;
}

export interface InstrumentPresentation {
  market: string;
  categoryMarket: string;
  code: string;
  instrumentId: string;
  displayCode: string;
  marketLabel: string;
  exchangeTag: string | null;
}

export function normalizeInstrumentMarket(
  market: string | null | undefined,
): string {
  return (market ?? "").trim().toUpperCase();
}

export function parseInstrumentId(
  instrumentId: string | null | undefined,
): { market: string; code: string } | null {
  const normalized = (instrumentId ?? "")
    .trim()
    .toUpperCase()
    .replace(":", ".");
  const separator = normalized.indexOf(".");
  if (separator <= 0 || separator === normalized.length - 1) {
    return null;
  }
  const market = normalized.slice(0, separator).trim();
  const code = normalized.slice(separator + 1).trim();
  if (market === "" || code === "") {
    return null;
  }
  return { market, code };
}

export function categoryMarketForUser(
  market: string | null | undefined,
): string {
  const normalized = normalizeInstrumentMarket(market);
  return A_SHARE_MARKETS.has(normalized) ? "CN" : normalized;
}

export function formatUserMarketLabel(
  market: string | null | undefined,
): string {
  const normalized = normalizeInstrumentMarket(market);
  if (A_SHARE_MARKETS.has(normalized)) {
    return "沪深";
  }
  return formatMarketLabel(normalized);
}

export function formatInstrumentExchangeTag(
  market: string | null | undefined,
): string | null {
  return A_SHARE_EXCHANGE_TAGS[normalizeInstrumentMarket(market)] ?? null;
}

export function normalizeInstrumentSecurityType(
  securityType: string | null | undefined,
): string {
  return (securityType ?? "")
    .trim()
    .replace(/^SecurityType_/i, "")
    .replace(/[\s_-]+/g, "")
    .toUpperCase();
}

export function formatInstrumentSecurityTypeLabel(
  securityType: string | null | undefined,
): string {
  const normalized = normalizeInstrumentSecurityType(securityType);
  if (normalized === "") {
    return "类型未知";
  }
  return SECURITY_TYPE_LABELS[normalized] ?? securityType?.trim() ?? "类型未知";
}

export function backtestInstrumentTypeForSecurityType(
  securityType: string | null | undefined,
): "stock" | "etf" {
  const normalized = normalizeInstrumentSecurityType(securityType);
  return normalized === "ETF" || normalized === "FUND" || normalized === "TRUST"
    ? "etf"
    : "stock";
}

export function bareInstrumentCode(
  value: string | null | undefined,
): string {
  const normalized = (value ?? "").trim().toUpperCase().replace(":", ".");
  return parseInstrumentId(normalized)?.code ?? normalized;
}

export function presentInstrument(
  input: InstrumentPresentationInput,
): InstrumentPresentation {
  const rawInstrumentId = (input.instrumentId ?? "")
    .trim()
    .toUpperCase()
    .replace(":", ".");
  const parsedInstrumentId = parseInstrumentId(rawInstrumentId);
  const parsedCode = parseInstrumentId(input.code);
  const market =
    parsedInstrumentId?.market ??
    parsedCode?.market ??
    normalizeInstrumentMarket(input.market);
  const code =
    parsedInstrumentId?.code ??
    parsedCode?.code ??
    bareInstrumentCode(input.code ?? rawInstrumentId);
  const instrumentId =
    parsedInstrumentId == null
      ? market !== "" && code !== ""
        ? `${market}.${code}`
        : rawInstrumentId || code
      : `${parsedInstrumentId.market}.${parsedInstrumentId.code}`;
  const categoryMarket = categoryMarketForUser(market);
  const isAShare = categoryMarket === "CN";

  return {
    market,
    categoryMarket,
    code,
    instrumentId,
    displayCode: isAShare ? code : instrumentId || code,
    marketLabel: formatUserMarketLabel(market),
    exchangeTag: formatInstrumentExchangeTag(market),
  };
}

// Text-only surfaces such as notifications and dense metric values cannot
// render InstrumentIdentity. Keep the same user semantics while preserving
// the canonical instrumentId in the underlying record/request.
export function formatInstrumentIdentityText(
  input: InstrumentPresentationInput,
): string {
  const presentation = presentInstrument(input);
  if (presentation.displayCode === "") {
    return "未设置";
  }
  return presentation.exchangeTag == null
    ? presentation.displayCode
    : `${presentation.displayCode}（${presentation.exchangeTag}）`;
}
