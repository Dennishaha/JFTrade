export interface InstrumentRef {
  market: string;
  code: string;
  symbol: string;
  instrumentId: string;
}

export interface InstrumentRefInput {
  market?: string | null;
  symbol?: string | null;
  code?: string | null;
  instrumentId?: string | null;
}

function normalizeSegment(value: string | null | undefined): string {
  return (value ?? "").trim().toUpperCase();
}

export function normalizeInstrumentId(value: string): string {
  const normalized = normalizeSegment(value);
  if (normalized === "") {
    return "";
  }
  if (normalized.includes(":")) {
    const [market, code] = normalized.split(":", 2);
    if ((market ?? "") !== "" && (code ?? "") !== "") {
      return `${market}.${code}`;
    }
  }
  return normalized;
}

export function resolveInstrumentRef(
  input: InstrumentRefInput,
  fallbackMarket?: string,
): InstrumentRef | null {
  const explicitMarket = normalizeSegment(input.market) || normalizeSegment(fallbackMarket);
  const candidate = normalizeInstrumentId(
    input.instrumentId
      ?? input.symbol
      ?? input.code
      ?? "",
  );

  if (candidate !== "") {
    const separator = candidate.includes(".") ? "." : "";
    if (separator !== "") {
      const [embeddedMarket, embeddedCode] = candidate.split(separator, 2);
      const resolvedMarket = embeddedMarket ?? "";
      const resolvedCode = embeddedCode ?? "";
      if (resolvedMarket !== "" && resolvedCode !== "") {
        return {
          market: resolvedMarket,
          code: resolvedCode,
          symbol: resolvedCode,
          instrumentId: `${resolvedMarket}.${resolvedCode}`,
        };
      }
    }
  }

  const code = normalizeSegment(input.code)
    || normalizeSegment(input.symbol)
    || normalizeSegment(input.instrumentId);
  if (explicitMarket === "" || code === "") {
    return null;
  }

  return {
    market: explicitMarket,
    code,
    symbol: code,
    instrumentId: `${explicitMarket}.${code}`,
  };
}

export function resolveInstrumentRefFromText(
  value: string,
  fallbackMarket?: string,
): InstrumentRef | null {
  return resolveInstrumentRef({ instrumentId: value }, fallbackMarket);
}