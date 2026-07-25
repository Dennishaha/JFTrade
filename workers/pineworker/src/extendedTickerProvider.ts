import {
  aggregateCandles,
  splitTickerModifier,
  TIMEFRAME_SECONDS,
  type IProvider,
  type ISymbolInfo,
  type Kline,
} from "pinets";
import type { Candle, ChartType } from "./types";

const timeframeAliases: Record<string, string> = {
  "1m": "1",
  "3m": "3",
  "5m": "5",
  "15m": "15",
  "30m": "30",
  "45m": "45",
  "1h": "60",
  "2h": "120",
  "3h": "180",
  "4h": "240",
  "1d": "D",
  "1w": "W",
  "1M": "M",
};

/**
 * A request-scoped provider for PineTS. It owns a standard OHLC stream and
 * derives chart-type views at its boundary, which lets request.security()
 * switch between standard and Heikin Ashi data without external I/O.
 */
export class ExtendedTickerProvider implements IProvider {
  private readonly baseSymbol: string;
  private readonly baseAliases: Set<string>;
  private readonly sourceTimeframe: string;
  private readonly standardCandles: Kline[];

  constructor(symbol: string, timeframe: string, candles: readonly Candle[]) {
    this.baseSymbol = stripModifier(symbol);
    this.baseAliases = symbolAliases(this.baseSymbol);
    this.sourceTimeframe = normalizePineTimeframe(timeframe);
    this.standardCandles = candles.map(toKline).sort((left, right) => left.openTime - right.openTime);
  }

  configure(_config: unknown): void {}

  async getMarketData(
    tickerId: string,
    timeframe: string,
    limit?: number,
    sDate?: number,
    eDate?: number,
  ): Promise<Kline[]> {
    let candles = this.derivedCandles(tickerId, timeframe);
    if (sDate !== undefined || eDate !== undefined) {
      candles = candles.filter((candle) => (
        (sDate === undefined || candle.openTime >= sDate) &&
        (eDate === undefined || candle.openTime <= eDate)
      ));
    }
    if (limit !== undefined && limit > 0 && candles.length > limit) {
      candles = candles.slice(-limit);
    }
    return candles.map(copyKline);
  }

  async getSymbolInfo(tickerId: string): Promise<ISymbolInfo> {
    this.assertCanServeTicker(tickerId);
    return symbolInfo(this.baseSymbol);
  }

  /**
   * Verifies a request.security route before PineTS creates its secondary
   * runtime. PineTS does not propagate provider rejections made during its
   * asynchronous constructor, so unsupported routes must be rejected here.
   */
  assertCanServe(tickerId: string, timeframe: string): void {
    this.assertCanServeTicker(tickerId);
    this.resolveDerivableTimeframe(timeframe);
  }

  assertCanServeTicker(tickerId: string): void {
    this.assertSupportedTickerModifier(tickerId);
    const parts = splitTickerModifier(String(tickerId));
    this.assertCurrentSymbol(parts.symbol);
  }

  append(candle: Candle): void {
    const next = toKline(candle);
    const previous = this.standardCandles.at(-1);
    if (previous !== undefined && next.openTime <= previous.openTime) {
      throw new Error("Pineworker extended ticker candles must be appended in strictly increasing open-time order");
    }
    this.standardCandles.push(next);
  }

  /** Returns the complete derived series, preserving the HA seed before any date filtering. */
  candlesFor(tickerId: string, timeframe: string): Kline[] {
    return this.derivedCandles(tickerId, timeframe).map(copyKline);
  }

  private derivedCandles(tickerId: string, timeframe: string): Kline[] {
    this.assertCanServeTicker(tickerId);
    const parts = splitTickerModifier(String(tickerId));
    const standard = this.standardCandlesFor(timeframe);
    return parts.modifier === "heikinashi" ? heikinAshiCandles(standard) : standard;
  }

  private standardCandlesFor(timeframe: string): Kline[] {
    const target = this.resolveDerivableTimeframe(timeframe);
    if (target === this.sourceTimeframe) {
      return this.standardCandles;
    }
    return aggregateCandles(this.standardCandles, target, this.sourceTimeframe);
  }

  private resolveDerivableTimeframe(timeframe: string): string {
    const target = normalizePineTimeframe(timeframe);
    if (target === this.sourceTimeframe) {
      return target;
    }
    const sourceSeconds = TIMEFRAME_SECONDS[this.sourceTimeframe];
    const targetSeconds = TIMEFRAME_SECONDS[target];
    if (sourceSeconds === undefined || targetSeconds === undefined) {
      throw new Error(
        `Pineworker cannot derive ${JSON.stringify(timeframe)} from ${JSON.stringify(this.sourceTimeframe)} candles`,
      );
    }
    if (target === "W" || target === "M") {
      if (this.sourceTimeframe !== "D") {
        throw new Error(
          `Pineworker can derive ${target} candles only from daily candles; received ${JSON.stringify(this.sourceTimeframe)}`,
        );
      }
    } else if (targetSeconds <= sourceSeconds || targetSeconds % sourceSeconds !== 0) {
      throw new Error(
        `Pineworker cannot derive ${JSON.stringify(target)} from ${JSON.stringify(this.sourceTimeframe)} candles`,
      );
    }
    return target;
  }

  private assertCurrentSymbol(tickerId: string): void {
    const requested = stripModifier(String(tickerId));
    if (!this.baseAliases.has(requested) && !this.baseAliases.has(withoutExchangePrefix(requested))) {
      throw new Error(
        `Pineworker extended tickers only support the current symbol ${JSON.stringify(this.baseSymbol)}; received ${JSON.stringify(tickerId)}`,
      );
    }
  }

  private assertSupportedTickerModifier(tickerId: string): void {
    const value = String(tickerId);
    const separator = value.lastIndexOf(";");
    if (separator === -1) {
      return;
    }
    const modifier = value.slice(separator + 1).trim().toLowerCase();
    if ((modifier === "standard" || modifier === "heikinashi") && !value.slice(0, separator).includes(";")) {
      return;
    }
    throw new Error(
      `Pineworker extended ticker modifier ${JSON.stringify(modifier)} is not supported; only standard and heikinashi are supported`,
    );
  }
}

export function chartTicker(symbol: string, chartType: ChartType): string {
  const base = stripModifier(symbol);
  return chartType === "heikinashi" ? `${base};heikinashi` : base;
}

export function normalizePineTimeframe(timeframe: string): string {
  const value = String(timeframe ?? "").trim();
  if (Object.hasOwn(TIMEFRAME_SECONDS, value)) {
    return value;
  }
  if (value === "1M") {
    return "M";
  }
  const lower = value.toLowerCase();
  if (timeframeAliases[lower] !== undefined) {
    return timeframeAliases[lower]!;
  }
  if (/^\d+s$/i.test(value)) {
    return value.toUpperCase();
  }
  return value;
}

export function heikinAshiCandles(candles: readonly Kline[]): Kline[] {
  let previousOpen: number | undefined;
  let previousClose: number | undefined;
  return candles.map((candle) => {
    const close = (candle.open + candle.high + candle.low + candle.close) / 4;
    const open = previousOpen === undefined || previousClose === undefined
      ? (candle.open + candle.close) / 2
      : (previousOpen + previousClose) / 2;
    const transformed: Kline = {
      ...candle,
      open,
      high: Math.max(candle.high, open, close),
      low: Math.min(candle.low, open, close),
      close,
    };
    previousOpen = open;
    previousClose = close;
    return transformed;
  });
}

function toKline(candle: Candle): Kline {
  return {
    openTime: candle.openTime,
    closeTime: candle.closeTime ?? candle.openTime,
    open: candle.open,
    high: candle.high,
    low: candle.low,
    close: candle.close,
    volume: candle.volume,
    quoteAssetVolume: 0,
    numberOfTrades: 0,
    takerBuyBaseAssetVolume: 0,
    takerBuyQuoteAssetVolume: 0,
    ignore: 0,
  };
}

function copyKline(candle: Kline): Kline {
  return { ...candle };
}

function stripModifier(symbol: string): string {
  return splitTickerModifier(String(symbol)).symbol;
}

function symbolAliases(symbol: string): Set<string> {
  return new Set([symbol, withoutExchangePrefix(symbol)]);
}

function withoutExchangePrefix(symbol: string): string {
  const separator = symbol.indexOf(":");
  return separator === -1 ? symbol : symbol.slice(separator + 1);
}

function symbolInfo(symbol: string): ISymbolInfo {
  const ticker = withoutExchangePrefix(symbol);
  return {
    current_contract: "",
    description: symbol,
    isin: "",
    main_tickerid: symbol,
    prefix: symbol.includes(":") ? symbol.slice(0, symbol.indexOf(":")) : "",
    root: ticker,
    ticker,
    tickerid: symbol,
    type: "stock",
    basecurrency: "",
    country: "",
    currency: "",
    timezone: "Etc/UTC",
    employees: 0,
    industry: "",
    sector: "",
    shareholders: 0,
    shares_outstanding_float: 0,
    shares_outstanding_total: 0,
    expiration_date: 0,
    session: "24x7",
    volumetype: "base",
    mincontract: 0,
    minmove: 1,
    mintick: 0.01,
    pointvalue: 1,
    pricescale: 100,
    recommendations_buy: 0,
    recommendations_buy_strong: 0,
    recommendations_date: 0,
    recommendations_hold: 0,
    recommendations_sell: 0,
    recommendations_sell_strong: 0,
    recommendations_total: 0,
    target_price_average: 0,
    target_price_date: 0,
    target_price_estimates: 0,
    target_price_high: 0,
    target_price_low: 0,
    target_price_median: 0,
  };
}
