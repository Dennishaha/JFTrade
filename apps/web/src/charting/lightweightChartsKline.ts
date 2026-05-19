import {
  ColorType,
  CrosshairMode,
  type IChartApi,
  type ISeriesApi,
  type Logical,
  type LogicalRange,
  type Time,
  TickMarkType,
  type UTCTimestamp,
  createChart,
} from "lightweight-charts";

import type {
  KlineCandle,
  KlineChartAdapter,
  KlineChartFactory,
  KlineChartPalette,
} from "./kline";

const INITIAL_VISIBLE_BARS = 120;
const INITIAL_RIGHT_OFFSET_BARS = 8;

function toTimestamp(at: string): UTCTimestamp {
  return Math.floor(new Date(at).getTime() / 1000) as UTCTimestamp;
}

function toDateFromChartTime(time: Time): Date | null {
  if (typeof time === "number") {
    return new Date(time * 1000);
  }

  if (typeof time === "string") {
    const parsed = new Date(time);
    return Number.isNaN(parsed.getTime()) ? null : parsed;
  }

  return new Date(Date.UTC(time.year, time.month - 1, time.day));
}

function formatLocalChartTime(time: Time): string {
  const date = toDateFromChartTime(time);
  if (date == null) {
    return "";
  }

  return new Intl.DateTimeFormat(undefined, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(date);
}

function formatLocalTickMark(time: Time, tickMarkType: TickMarkType): string {
  const date = toDateFromChartTime(time);
  if (date == null) {
    return "";
  }

  const options: Intl.DateTimeFormatOptions =
    tickMarkType === TickMarkType.Year
      ? { year: "numeric" }
      : tickMarkType === TickMarkType.Month
        ? { month: "2-digit", year: "2-digit" }
        : tickMarkType === TickMarkType.DayOfMonth
          ? { month: "2-digit", day: "2-digit" }
          : { hour: "2-digit", minute: "2-digit", hour12: false };

  return new Intl.DateTimeFormat(undefined, options).format(date);
}

function sortCandles(candles: readonly KlineCandle[]): KlineCandle[] {
  const byTimestamp = new Map<number, KlineCandle>();
  const normalizedCandles = [...candles].sort(
    (left, right) => new Date(left.at).getTime() - new Date(right.at).getTime(),
  );

  for (const candle of normalizedCandles) {
    const timestamp = new Date(candle.at).getTime();
    if (
      !Number.isFinite(timestamp) ||
      !Number.isFinite(candle.open) ||
      !Number.isFinite(candle.high) ||
      !Number.isFinite(candle.low) ||
      !Number.isFinite(candle.close)
    ) {
      continue;
    }

    const chartTimestamp = Math.floor(timestamp / 1000) * 1000;
    const existing = byTimestamp.get(chartTimestamp);
    if (existing == null) {
      byTimestamp.set(chartTimestamp, {
        ...candle,
        at: new Date(chartTimestamp).toISOString(),
      });
      continue;
    }

    byTimestamp.set(chartTimestamp, {
      at: existing.at,
      open: existing.open,
      high: Math.max(existing.high, candle.high),
      low: Math.min(existing.low, candle.low),
      close: candle.close,
      volume: Math.max(existing.volume, candle.volume),
    });
  }

  return [...byTimestamp.values()].sort(
    (left, right) => new Date(left.at).getTime() - new Date(right.at).getTime(),
  );
}

function measureHost(host: HTMLElement): { width: number; height: number } {
  const rect = host.getBoundingClientRect();
  return {
    width: Math.max(1, Math.floor(rect.width || host.clientWidth || 1)),
    height: Math.max(1, Math.floor(rect.height || host.clientHeight || 1)),
  };
}

function toLogical(value: number): Logical {
  return value as Logical;
}

export class LightweightChartsKlineAdapter implements KlineChartAdapter {
  private readonly chart: IChartApi;
  private readonly candleSeries: ISeriesApi<"Candlestick">;
  private readonly volumeSeries: ISeriesApi<"Histogram">;
  private palette: KlineChartPalette;
  private loadMoreHandler: (() => void) | null = null;
  private hasFitInitialData = false;
  private firstTimestamp: number | null = null;
  private lastTimestamp: number | null = null;
  private lastLoadMoreLogicalFrom: number | null = null;

  constructor(host: HTMLElement, palette: KlineChartPalette) {
    this.palette = palette;
    const initialSize = measureHost(host);
    this.chart = createChart(host, {
      width: initialSize.width,
      height: initialSize.height,
      layout: {
        background: { type: ColorType.Solid, color: palette.bg },
        textColor: palette.text,
      },
      grid: {
        vertLines: { color: palette.grid },
        horzLines: { color: palette.grid },
      },
      rightPriceScale: { borderColor: palette.border },
      timeScale: {
        borderColor: palette.border,
        barSpacing: 6,
        rightOffset: INITIAL_RIGHT_OFFSET_BARS,
        timeVisible: true,
        secondsVisible: true,
        tickMarkFormatter: formatLocalTickMark,
      },
      localization: {
        timeFormatter: formatLocalChartTime,
      },
      crosshair: { mode: CrosshairMode.Normal },
    });

    this.candleSeries = this.chart.addCandlestickSeries({
      upColor: palette.up,
      downColor: palette.down,
      borderUpColor: palette.up,
      borderDownColor: palette.down,
      wickUpColor: palette.up,
      wickDownColor: palette.down,
    });
    this.volumeSeries = this.chart.addHistogramSeries({
      priceFormat: { type: "volume" },
      priceScaleId: "",
    });
    this.volumeSeries
      .priceScale()
      .applyOptions({ scaleMargins: { top: 0.8, bottom: 0 } });
    this.chart.timeScale().subscribeVisibleLogicalRangeChange((range) => {
      if (range == null || this.loadMoreHandler == null) {
        return;
      }

      const barsInfo = this.candleSeries.barsInLogicalRange(range);
      const barsBefore = barsInfo?.barsBefore;
      if (
        (typeof barsBefore === "number" ? barsBefore < 10 : range.from <= 5) &&
        this.lastLoadMoreLogicalFrom !== range.from
      ) {
        this.lastLoadMoreLogicalFrom = range.from;
        this.loadMoreHandler();
      }
    });
  }

  setCandles(candles: readonly KlineCandle[]): void {
    const previousFirstTimestamp = this.firstTimestamp;
    const visibleRange = this.chart.timeScale().getVisibleLogicalRange();
    const sorted = sortCandles(candles);
    const nextFirstTimestamp =
      sorted.length === 0 ? null : new Date(sorted[0]?.at ?? "").getTime();
    const nextLastTimestamp =
      sorted.length === 0
        ? null
        : new Date(sorted[sorted.length - 1]?.at ?? "").getTime();
    const isDifferentSeries =
      this.firstTimestamp != null &&
      this.lastTimestamp != null &&
      nextFirstTimestamp != null &&
      nextLastTimestamp != null &&
      (nextLastTimestamp < this.firstTimestamp ||
        nextFirstTimestamp > this.lastTimestamp);
    const prependedCount =
      !isDifferentSeries &&
      previousFirstTimestamp != null &&
      nextFirstTimestamp != null &&
      nextFirstTimestamp < previousFirstTimestamp
        ? sorted.findIndex(
            (candle) => new Date(candle.at).getTime() >= previousFirstTimestamp,
          )
        : 0;

    this.candleSeries.setData(
      sorted.map((candle) => ({
        time: toTimestamp(candle.at),
        open: candle.open,
        high: candle.high,
        low: candle.low,
        close: candle.close,
      })),
    );

    this.volumeSeries.setData(
      sorted.map((candle) => ({
        time: toTimestamp(candle.at),
        value: candle.volume,
        color:
          candle.close >= candle.open
            ? this.palette.volumeUp
            : this.palette.volumeDown,
      })),
    );

    this.firstTimestamp = Number.isFinite(nextFirstTimestamp)
      ? nextFirstTimestamp
      : null;
    this.lastTimestamp = Number.isFinite(nextLastTimestamp)
      ? nextLastTimestamp
      : null;

    if ((!this.hasFitInitialData || isDifferentSeries) && sorted.length > 0) {
      this.hasFitInitialData = true;
      this.setVisibleLogicalRange({
        from: toLogical(sorted.length - INITIAL_VISIBLE_BARS),
        to: toLogical(sorted.length + INITIAL_RIGHT_OFFSET_BARS),
      });
      return;
    }

    if (visibleRange != null && prependedCount > 0) {
      this.setVisibleLogicalRange({
        from: toLogical(visibleRange.from + prependedCount),
        to: toLogical(visibleRange.to + prependedCount),
      });
    }
  }

  applyPalette(palette: KlineChartPalette): void {
    this.palette = palette;
    this.chart.applyOptions({
      layout: {
        background: { type: ColorType.Solid, color: palette.bg },
        textColor: palette.text,
      },
      grid: {
        vertLines: { color: palette.grid },
        horzLines: { color: palette.grid },
      },
      rightPriceScale: { borderColor: palette.border },
      timeScale: { borderColor: palette.border },
    });
    this.candleSeries.applyOptions({
      upColor: palette.up,
      downColor: palette.down,
      borderUpColor: palette.up,
      borderDownColor: palette.down,
      wickUpColor: palette.up,
      wickDownColor: palette.down,
    });
  }

  resize(width: number, height: number): void {
    this.chart.resize(
      Math.max(1, Math.floor(width)),
      Math.max(1, Math.floor(height)),
      true,
    );
  }

  setLoadMoreHandler(handler: (() => void) | null): void {
    this.loadMoreHandler = handler;
  }

  fitContent(): void {
    this.chart.timeScale().fitContent();
  }

  private setVisibleLogicalRange(range: LogicalRange): void {
    this.chart.timeScale().setVisibleLogicalRange(range);
  }

  remove(): void {
    this.chart.remove();
  }
}

export const lightweightChartsKlineFactory: KlineChartFactory = {
  id: "tradingview-lightweight-charts",
  create(host, options) {
    return new LightweightChartsKlineAdapter(host, options.palette);
  },
};
