import {
  normalizeQuoteWorkbenchProductClass,
  type QuoteWorkbenchKind,
  type QuoteWorkbenchTarget,
} from "../domain/market-data/quoteWorkbench";

export type ResearchQuoteKind = QuoteWorkbenchKind;
export type ResearchQuoteTarget = QuoteWorkbenchTarget;

export interface ResearchQuoteTargetInput {
  kind?: unknown;
  instrumentId?: unknown;
  name?: unknown;
  productClass?: unknown;
}

export interface ResearchInstrumentParts {
  instrumentId: string;
  market: string;
  symbol: string;
}

type ResearchEntry = Record<string, unknown>;
const QUOTEABLE_PRODUCT_CLASSES = new Set([
  "equity", "fund", "index", "warrant", "cbbc", "plate",
]);
const UNSUPPORTED_BSE_SYMBOL = /^(?:43|83|87|88|92)/;

function asRecord(value: unknown): ResearchEntry | null {
  return value != null && typeof value === "object"
    ? (value as ResearchEntry)
    : null;
}

function nonEmptyString(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function firstString(
  records: readonly (ResearchEntry | null)[],
  keys: readonly string[],
): string {
  for (const record of records) {
    if (record == null) continue;
    for (const key of keys) {
      const value = nonEmptyString(record[key]);
      if (value !== "") return value;
    }
  }
  return "";
}

/**
 * Market-data routes require a concrete OpenD market. In particular, the UI's
 * logical CN market must never replace a normalized SH/SZ instrument prefix.
 */
export function parseResearchInstrumentId(
  value: unknown,
): ResearchInstrumentParts | null {
  const raw = nonEmptyString(value);
  const separator = raw.indexOf(".");
  if (separator <= 0 || separator === raw.length - 1) return null;
  const market = raw.slice(0, separator).trim().toUpperCase();
  const symbol = raw.slice(separator + 1).trim().toUpperCase();
  if (market === "" || symbol === "" || market === "CN") return null;
  return {
    instrumentId: `${market}.${symbol}`,
    market,
    symbol,
  };
}

export function normalizeResearchQuoteTarget(
  value: ResearchQuoteTargetInput | null | undefined,
): ResearchQuoteTarget | null {
  if (value == null) return null;
  const instrument = parseResearchInstrumentId(value.instrumentId);
  if (
    instrument == null ||
    (instrument.market === "SH" && UNSUPPORTED_BSE_SYMBOL.test(instrument.symbol))
  ) {
    return null;
  }
  const productClass = normalizeQuoteWorkbenchProductClass(value.productClass);
  return {
    kind:
      value.kind === "plate" || productClass === "plate"
        ? "plate"
        : "instrument",
    instrumentId: instrument.instrumentId,
    name: nonEmptyString(value.name),
    productClass,
  };
}

/** Convert a normalized or raw OpenD feature row into a quoteable target. */
export function researchQuoteTargetFromEntry(
  entry: ResearchEntry | null | undefined,
  fallbackMarket = "",
): ResearchQuoteTarget | null {
  if (entry == null) return null;

  const plate = asRecord(entry.plate);
  const basic = asRecord(entry.basic);
  const basicSecurity = asRecord(basic?.security);
  const security = asRecord(entry.security);
  const records = [entry, plate, security, basicSecurity] as const;
  let instrumentId = firstString(records, [
    "instrumentId",
    "securityCode",
    "stockCode",
    "contractCode",
  ]);

  // A bare code is only safe when the caller supplied a concrete market. CN is
  // deliberately rejected because it would lose the SH/SZ routing identity.
  if (instrumentId === "") {
    const code = firstString(records, ["code", "symbol"]);
    const market = fallbackMarket.trim().toUpperCase();
    if (code !== "" && market !== "" && market !== "CN") {
      instrumentId = `${market}.${code}`;
    }
  }

  const instrument = parseResearchInstrumentId(instrumentId);
  if (
    instrument == null ||
    (instrument.market === "SH" && UNSUPPORTED_BSE_SYMBOL.test(instrument.symbol))
  ) {
    return null;
  }
  const productClass = normalizeQuoteWorkbenchProductClass(
    plate != null
      ? "plate"
      : firstString(records, ["productClass", "securityType", "type"]),
  );
  const kind: ResearchQuoteKind =
    plate != null || productClass === "plate" ? "plate" : "instrument";

  return {
    kind,
    instrumentId: instrument.instrumentId,
    name: firstString(records, ["name", "plateName", "title"]),
    productClass,
  };
}

export function isResearchQuoteTarget(value: unknown): value is ResearchQuoteTarget {
  if (value == null || typeof value !== "object") return false;
  return normalizeResearchQuoteTarget(value as ResearchQuoteTargetInput) != null;
}

export function isResearchQuoteEntry(
  entry: ResearchEntry | null | undefined,
  fallbackMarket = "",
): boolean {
  const target = researchQuoteTargetFromEntry(entry, fallbackMarket);
  return target != null && QUOTEABLE_PRODUCT_CLASSES.has(target.productClass);
}
