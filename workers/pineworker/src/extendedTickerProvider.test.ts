import { describe, expect, test } from "vitest";
import { chartTicker, ExtendedTickerProvider, heikinAshiCandles, normalizePineTimeframe } from "./extendedTickerProvider";
import { normalizeChartType, type Candle } from "./types";

describe("ExtendedTickerProvider", () => {
  test("derives Heikin Ashi candles without mutating standard OHLC metadata", async () => {
    const candles = sampleCandles();
    const provider = new ExtendedTickerProvider("US.AAPL", "1", candles);

    const transformed = await provider.getMarketData("US.AAPL;heikinashi", "1");
    expect(transformed.map(ohlc)).toEqual([
      { open: 11, high: 13, low: 9, close: 11 },
      { open: 11, high: 15, low: 10, close: 12.5 },
      { open: 11.75, high: 16, low: 11, close: 13.5 },
    ]);
    expect(transformed.map((candle) => ({ openTime: candle.openTime, closeTime: candle.closeTime, volume: candle.volume }))).toEqual([
      { openTime: 0, closeTime: 60_000, volume: 10 },
      { openTime: 60_000, closeTime: 120_000, volume: 20 },
      { openTime: 120_000, closeTime: 180_000, volume: 30 },
    ]);
    await expect(provider.getMarketData("US.AAPL", "1")).resolves.toMatchObject(candles);
    const mutableView = await provider.getMarketData("US.AAPL", "1");
    mutableView[0]!.close = 0;
    await expect(provider.getMarketData("US.AAPL", "1")).resolves.toMatchObject(candles);
  });

  test("aggregates standard candles before applying the Heikin Ashi transform", async () => {
    const provider = new ExtendedTickerProvider("US.AAPL", "1", sampleCandles());

    const transformed = await provider.getMarketData("US.AAPL;heikinashi", "3");
    expect(transformed.map(ohlc)).toEqual([{ open: 12, high: 16, low: 9, close: 12.25 }]);
    expect(transformed[0]).toMatchObject({ openTime: 0, closeTime: 180_000, volume: 60 });
  });

  test("serves standard data for ticker.standard and accepts same-symbol exchange aliases", async () => {
    const provider = new ExtendedTickerProvider("NASDAQ:AAPL", "1", sampleCandles());

    await expect(provider.getMarketData("AAPL;standard", "1")).resolves.toMatchObject(sampleCandles());
    await expect(provider.getSymbolInfo("AAPL;heikinashi")).resolves.toMatchObject({ tickerid: "NASDAQ:AAPL" });
  });

  test("rejects unsupported symbols and timeframes rather than returning unrelated candles", async () => {
    const provider = new ExtendedTickerProvider("US.AAPL", "1", sampleCandles());

    await expect(provider.getMarketData("US.MSFT;heikinashi", "1")).rejects.toThrow("only support the current symbol");
    await expect(provider.getMarketData("US.AAPL", "2")).rejects.toThrow("cannot derive");
  });

  test("rejects unsupported extended ticker modifiers before serving data or symbol metadata", async () => {
    const provider = new ExtendedTickerProvider("US.AAPL", "1", sampleCandles());

    expect(() => provider.assertCanServeTicker("US.AAPL;renko")).toThrow(
      'extended ticker modifier "renko" is not supported',
    );
    await expect(provider.getMarketData("US.AAPL;renko", "1")).rejects.toThrow(
      'extended ticker modifier "renko" is not supported',
    );
    await expect(provider.getSymbolInfo("US.AAPL;renko")).rejects.toThrow(
      'extended ticker modifier "renko" is not supported',
    );
  });

  test("recomputes the HA tail from standard append state", async () => {
    const provider = new ExtendedTickerProvider("US.AAPL", "1", sampleCandles().slice(0, 2));
    provider.append(sampleCandles()[2]!);

    const transformed = await provider.getMarketData("US.AAPL;heikinashi", "1");
    expect(transformed.at(-1)).toMatchObject({ open: 11.75, high: 16, low: 11, close: 13.5 });
  });
});

describe("extended ticker helpers", () => {
  test("normalizes supported Pine timeframes and chart ticker modifiers", () => {
    expect(normalizePineTimeframe("1h")).toBe("60");
    expect(normalizePineTimeframe("1M")).toBe("M");
    expect(chartTicker("US.AAPL;heikinashi", "standard")).toBe("US.AAPL");
    expect(chartTicker("US.AAPL", "heikinashi")).toBe("US.AAPL;heikinashi");
    expect(normalizeChartType("renko")).toBe("standard");
    expect(normalizeChartType(" HeikinAshi ")).toBe("heikinashi");
  });

  test("can transform a raw Kline sequence independently", () => {
    const transformed = heikinAshiCandles([
      { ...sampleCandles()[0]!, quoteAssetVolume: 0, numberOfTrades: 0, takerBuyBaseAssetVolume: 0, takerBuyQuoteAssetVolume: 0, ignore: 0 },
    ]);
    expect(transformed[0]).toMatchObject({ open: 11, high: 13, low: 9, close: 11 });
  });
});

function sampleCandles(): Candle[] {
  return [
    { openTime: 0, closeTime: 60_000, open: 10, high: 13, low: 9, close: 12, volume: 10 },
    { openTime: 60_000, closeTime: 120_000, open: 12, high: 15, low: 10, close: 13, volume: 20 },
    { openTime: 120_000, closeTime: 180_000, open: 13, high: 16, low: 11, close: 14, volume: 30 },
  ];
}

function ohlc(candle: { open: number; high: number; low: number; close: number }) {
  return { open: candle.open, high: candle.high, low: candle.low, close: candle.close };
}
