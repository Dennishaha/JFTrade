export type OptionSide = "call" | "put";
export type OptionMoneyness = "ATM" | "ITM" | "OTM" | "";

type Entry = Record<string, unknown>;

export interface OptionChainSideModel {
  code: string;
  instrumentId: string;
  name: string;
  bidPrice: number | null;
  askPrice: number | null;
  impliedVolatility: number | null;
  delta: number | null;
  gamma: number | null;
  theta: number | null;
  vega: number | null;
  moneyness: OptionMoneyness;
  multiplier: number;
}

export interface OptionChainRowModel {
  key: string;
  strike: number | null;
  isAtm: boolean;
  call: OptionChainSideModel;
  put: OptionChainSideModel;
}

export function asOptionEntry(value: unknown): Entry {
  return value != null && typeof value === "object" && !Array.isArray(value)
    ? (value as Entry)
    : {};
}

export function optionInstrumentId(market: string, code: string): string {
  const normalized = code.trim().toUpperCase();
  if (!normalized) return "";
  return normalized.includes(".")
    ? normalized
    : `${market.trim().toUpperCase()}.${normalized}`;
}

export function optionCode(row: Entry, side: OptionSide): string {
  const info = asOptionEntry(row[side]);
  const basic = asOptionEntry(info.basic);
  return String(asOptionEntry(basic.security).code ?? "")
    .trim()
    .toUpperCase();
}

export function optionStrike(row: Entry): number | null {
  for (const side of ["call", "put"] as const) {
    const info = asOptionEntry(row[side]);
    const extra = asOptionEntry(info.optionExData);
    const strike = Number(extra.strikePrice);
    if (Number.isFinite(strike)) return strike;
  }
  return null;
}

export function nearestOptionStrike(
  rows: Entry[],
  underlyingPrice: number | null,
): number | null {
  if (underlyingPrice == null) return null;
  let nearest: number | null = null;
  for (const row of rows) {
    const strike = optionStrike(row);
    if (
      strike != null &&
      (nearest == null ||
        Math.abs(strike - underlyingPrice) <
          Math.abs(nearest - underlyingPrice))
    ) {
      nearest = strike;
    }
  }
  return nearest;
}

function finiteNumber(value: unknown): number | null {
  const number = Number(value);
  return Number.isFinite(number) ? number : null;
}

function snapshotFor(
  snapshots: Record<string, Entry>,
  instrumentId: string,
): Entry {
  return snapshots[instrumentId.trim().toUpperCase()] ?? {};
}

function moneyness(
  side: OptionSide,
  strike: number | null,
  underlyingPrice: number | null,
  atmStrike: number | null,
): OptionMoneyness {
  if (strike == null || underlyingPrice == null) return "";
  if (atmStrike != null && strike === atmStrike) return "ATM";
  if (side === "call") return underlyingPrice > strike ? "ITM" : "OTM";
  return underlyingPrice < strike ? "ITM" : "OTM";
}

function buildSide(
  row: Entry,
  side: OptionSide,
  market: string,
  snapshots: Record<string, Entry>,
  strike: number | null,
  underlyingPrice: number | null,
  atmStrike: number | null,
): OptionChainSideModel {
  const info = asOptionEntry(row[side]);
  const basic = asOptionEntry(info.basic);
  const code = optionCode(row, side);
  const instrumentId = optionInstrumentId(market, code);
  const snapshot = snapshotFor(snapshots, instrumentId);
  const option = asOptionEntry(snapshot.option);
  return {
    code,
    instrumentId,
    name: String(basic.name ?? code ?? ""),
    bidPrice: finiteNumber(snapshot.bidPrice),
    askPrice: finiteNumber(snapshot.askPrice),
    impliedVolatility: finiteNumber(option.impliedVolatility),
    delta: finiteNumber(option.delta),
    gamma: finiteNumber(option.gamma),
    theta: finiteNumber(option.theta),
    vega: finiteNumber(option.vega),
    moneyness: moneyness(
      side,
      strike,
      underlyingPrice,
      atmStrike,
    ),
    multiplier: finiteNumber(basic.lotSize) ?? 100,
  };
}

export function buildOptionChainRows(
  rows: Entry[],
  snapshots: Record<string, Entry>,
  market: string,
  underlyingPrice: number | null,
): OptionChainRowModel[] {
  const atmStrike = nearestOptionStrike(rows, underlyingPrice);
  return rows.map((row, index) => {
    const strike = optionStrike(row);
    return {
      key: `${strike ?? "unknown"}-${index}`,
      strike,
      isAtm: strike != null && strike === atmStrike,
      call: buildSide(
        row,
        "call",
        market,
        snapshots,
        strike,
        underlyingPrice,
        atmStrike,
      ),
      put: buildSide(
        row,
        "put",
        market,
        snapshots,
        strike,
        underlyingPrice,
        atmStrike,
      ),
    };
  });
}

export function formatOptionMetric(
  value: number | null,
  digits = 3,
): string {
  if (value == null) return "—";
  return new Intl.NumberFormat("zh-CN", {
    maximumFractionDigits: digits,
  }).format(value);
}
