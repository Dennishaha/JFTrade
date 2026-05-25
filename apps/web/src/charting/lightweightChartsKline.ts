import {
  CandlestickSeries,
  ColorType,
  CrosshairMode,
  HistogramSeries,
  LineStyle,
  LineSeries,
  type LineWidth,
  type IChartApi,
  type ISeriesApi,
  type Logical,
  type LogicalRange,
  type Time,
  TickMarkType,
  type UTCTimestamp,
  createChart,
} from "lightweight-charts";

import {
  getKlineIndicatorDefinition,
  isKlineOverlayIndicator,
  isKlinePaneIndicator,
  normalizeKlineIndicators,
  resolveKlineCandleDisplayAt,
  type KlineCandle,
  type KlineChartAdapter,
  type KlineChartFactory,
  type KlineIndicatorKey,
  type KlineChartPalette,
} from "./kline";
import {
  computeAtr,
  computeCci,
  computeExponentialMovingAverage,
  computeKdj,
  computeMacd,
  computeSimpleMovingAverage,
  computeWilliamsR,
} from "./lightweightChartsIndicators";

const INITIAL_VISIBLE_BARS = 120;
const INITIAL_RIGHT_OFFSET_BARS = 8;
const DEFAULT_INDICATORS: KlineIndicatorKey[] = ["volume"];
const MOVING_AVERAGE_PERIODS = [5, 10, 20, 30, 60, 120, 180, 250] as const;
const OVERLAY_SERIES_COLORS = [
  "#2563eb",
  "#f97316",
  "#10b981",
  "#8b5cf6",
  "#ef4444",
  "#0ea5e9",
  "#ca8a04",
  "#db2777",
] as const;

/** Fixed height for each indicator sub-pane in pixels. */
export const INDICATOR_PANE_HEIGHT = 120;

/** Canonical creation order for indicator panes. */
const INDICATOR_ORDER: readonly KlineIndicatorKey[] = ["volume", "macd", "kdj", "atr", "cci", "williamsr"];

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
    (left, right) =>
      new Date(resolveKlineCandleDisplayAt(left)).getTime() -
      new Date(resolveKlineCandleDisplayAt(right)).getTime(),
  );

  for (const candle of normalizedCandles) {
    const timestamp = new Date(resolveKlineCandleDisplayAt(candle)).getTime();
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

function getOverlaySeriesColor(period: number): string {
  const index = MOVING_AVERAGE_PERIODS.indexOf(
    period as (typeof MOVING_AVERAGE_PERIODS)[number],
  );
  const color = OVERLAY_SERIES_COLORS[index >= 0 ? index : 0];
  return color ?? OVERLAY_SERIES_COLORS[0];
}

function buildOverlaySeriesOptions(indicator: KlineIndicatorKey) {
  const definition = getKlineIndicatorDefinition(indicator);
  if (definition == null || definition.kind !== "overlay" || definition.period == null) {
    throw new Error(`Unsupported overlay indicator '${indicator}'.`);
  }

  const lineWidth: LineWidth = definition.family === "ma" ? 2 : 1;

  return {
    title: definition.label,
    color: getOverlaySeriesColor(definition.period),
    lineWidth,
    lineStyle:
      definition.family === "ma" ? LineStyle.Solid : LineStyle.Dashed,
    priceLineVisible: false,
    lastValueVisible: false,
    crosshairMarkerVisible: true,
  };
}

function buildOverlaySeriesData(
  candles: readonly KlineCandle[],
  indicator: KlineIndicatorKey,
): Array<{ time: UTCTimestamp; value: number }> {
  const definition = getKlineIndicatorDefinition(indicator);
  if (definition == null || definition.period == null) {
    return [];
  }

  const closes = candles.map((candle) => candle.close);
  const values =
    definition.family === "ma"
      ? computeSimpleMovingAverage(closes, definition.period)
      : computeExponentialMovingAverage(closes, definition.period);

  return candles.flatMap((candle, index) => {
    const value = values[index];
    if (value == null) {
      return [];
    }

    return [{ time: toTimestamp(candle.at), value }];
  });
}

export class LightweightChartsKlineAdapter implements KlineChartAdapter {
  private readonly chart: IChartApi;
  private readonly candleSeries: ISeriesApi<"Candlestick">;

  // Indicator series — null when the indicator is disabled.
  private volumeSeries: ISeriesApi<"Histogram"> | null = null;
  private macdHistogramSeries: ISeriesApi<"Histogram"> | null = null;
  private macdDiffSeries: ISeriesApi<"Line"> | null = null;
  private macdDeaSeries: ISeriesApi<"Line"> | null = null;
  private kdjKSeries: ISeriesApi<"Line"> | null = null;
  private kdjDSeries: ISeriesApi<"Line"> | null = null;
  private kdjJSeries: ISeriesApi<"Line"> | null = null;
  private atrSeries: ISeriesApi<"Line"> | null = null;
  private cciSeries: ISeriesApi<"Line"> | null = null;
  private williamsRSeries: ISeriesApi<"Line"> | null = null;
  private overlaySeries = new Map<KlineIndicatorKey, ISeriesApi<"Line">>();

  private palette: KlineChartPalette;
  private selectedIndicators: KlineIndicatorKey[];
  private currentCandles: KlineCandle[] = [];
  private loadMoreHandler: (() => void) | null = null;
  private hasFitInitialData = false;
  private firstTimestamp: number | null = null;
  private lastTimestamp: number | null = null;
  private currentPeriod: string | null = null;
  private lastLoadMoreLogicalFrom: number | null = null;

  constructor(
    host: HTMLElement,
    palette: KlineChartPalette,
    indicators: readonly KlineIndicatorKey[] = DEFAULT_INDICATORS,
  ) {
    this.palette = palette;
    this.selectedIndicators = normalizeKlineIndicators(indicators);
    const initialSize = measureHost(host);
    this.chart = createChart(host, {
      width: initialSize.width,
      height: initialSize.height,
      layout: {
        background: { type: ColorType.Solid, color: palette.bg },
        textColor: palette.text,
        panes: {
          separatorColor: palette.border,
          enableResize: true,
        },
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
        // Strip IEEE-754 float64 noise (e.g. 23.649999999999999 → "23.65").
        // parseFloat removes trailing zeros so "23.6500" becomes "23.65".
        priceFormatter: (price: number) => String(parseFloat(price.toFixed(8))),
      },
      crosshair: { mode: CrosshairMode.Normal },
    });

    // Main candlestick series always lives in pane 0.
    this.candleSeries = this.chart.addSeries(CandlestickSeries, {
      upColor: palette.up,
      downColor: palette.down,
      borderUpColor: palette.up,
      borderDownColor: palette.down,
      wickUpColor: palette.up,
      wickDownColor: palette.down,
    });

    // Build pane and overlay series for the initial indicator set.
    this.syncIndicatorSeries();

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

  /**
   * Rebuild all indicator series so pane indicators keep their own sub-panes
   * and overlay indicators stay on the main candle pane.
   */
  private syncIndicatorSeries(): void {
    for (const series of this.overlaySeries.values()) {
      this.chart.removeSeries(series);
    }
    this.overlaySeries.clear();

    // Tear down all existing indicator panes (highest index first).
    const panes = this.chart.panes();
    for (let i = panes.length - 1; i >= 1; i--) {
      this.chart.removePane(i);
    }
    this.volumeSeries = null;
    this.macdHistogramSeries = null;
    this.macdDiffSeries = null;
    this.macdDeaSeries = null;
    this.kdjKSeries = null;
    this.kdjDSeries = null;
    this.kdjJSeries = null;
    this.atrSeries = null;
    this.cciSeries = null;
    this.williamsRSeries = null;

    // Recreate in canonical order.
    for (const indicator of INDICATOR_ORDER) {
      if (!isKlinePaneIndicator(indicator) || !this.selectedIndicators.includes(indicator)) {
        continue;
      }

      const paneIdx = this.chart.panes().length;

      if (indicator === "volume") {
        this.volumeSeries = this.chart.addSeries(
          HistogramSeries,
          {
            priceFormat: { type: "volume" },
            priceLineVisible: false,
            lastValueVisible: false,
          },
          paneIdx,
        );
      } else if (indicator === "macd") {
        // All three MACD series share the same pane index.
        this.macdHistogramSeries = this.chart.addSeries(
          HistogramSeries,
          { priceLineVisible: false, lastValueVisible: false },
          paneIdx,
        );
        this.macdDiffSeries = this.chart.addSeries(
          LineSeries,
          {
            lineWidth: 2,
            color: this.palette.indicatorA,
            priceLineVisible: false,
            lastValueVisible: false,
            crosshairMarkerVisible: true,
          },
          paneIdx,
        );
        this.macdDeaSeries = this.chart.addSeries(
          LineSeries,
          {
            lineWidth: 2,
            color: this.palette.indicatorB,
            priceLineVisible: false,
            lastValueVisible: false,
            crosshairMarkerVisible: true,
          },
          paneIdx,
        );
      } else if (indicator === "kdj") {
        this.kdjKSeries = this.chart.addSeries(
          LineSeries,
          {
            lineWidth: 2,
            color: this.palette.indicatorA,
            priceLineVisible: false,
            lastValueVisible: false,
            crosshairMarkerVisible: true,
          },
          paneIdx,
        );
        this.kdjDSeries = this.chart.addSeries(
          LineSeries,
          {
            lineWidth: 2,
            color: this.palette.indicatorB,
            priceLineVisible: false,
            lastValueVisible: false,
            crosshairMarkerVisible: true,
          },
          paneIdx,
        );
        this.kdjJSeries = this.chart.addSeries(
          LineSeries,
          {
            lineWidth: 2,
            color: this.palette.indicatorC,
            priceLineVisible: false,
            lastValueVisible: false,
            crosshairMarkerVisible: true,
          },
          paneIdx,
        );
      } else if (indicator === "atr") {
        this.atrSeries = this.chart.addSeries(
          LineSeries,
          {
            lineWidth: 2,
            color: this.palette.indicatorA,
            priceLineVisible: false,
            lastValueVisible: false,
            crosshairMarkerVisible: true,
          },
          paneIdx,
        );
      } else if (indicator === "cci") {
        this.cciSeries = this.chart.addSeries(
          LineSeries,
          {
            lineWidth: 2,
            color: this.palette.indicatorB,
            priceLineVisible: false,
            lastValueVisible: false,
            crosshairMarkerVisible: true,
          },
          paneIdx,
        );
      } else if (indicator === "williamsr") {
        this.williamsRSeries = this.chart.addSeries(
          LineSeries,
          {
            lineWidth: 2,
            color: this.palette.indicatorC,
            priceLineVisible: false,
            lastValueVisible: false,
            crosshairMarkerVisible: true,
          },
          paneIdx,
        );
      }

      // Set a fixed height on the newly-created indicator pane.
      const indicatorPane = this.chart.panes()[paneIdx];
      if (indicatorPane != null) {
        indicatorPane.setHeight(INDICATOR_PANE_HEIGHT);
      }
    }

    for (const indicator of this.selectedIndicators) {
      if (!isKlineOverlayIndicator(indicator)) {
        continue;
      }

      this.overlaySeries.set(
        indicator,
        this.chart.addSeries(LineSeries, buildOverlaySeriesOptions(indicator), 0),
      );
    }
  }

  private updateIndicatorSeries(sorted: readonly KlineCandle[]): void {
    if (this.volumeSeries != null) {
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
    }

    if (this.macdHistogramSeries != null) {
      const macd = computeMacd(sorted);
      this.macdHistogramSeries.setData(
        macd.histogram.map((item) => ({
          ...item,
          color:
            item.value >= 0
              ? this.palette.macdPositive
              : this.palette.macdNegative,
        })),
      );
      this.macdDiffSeries!.setData(macd.diff);
      this.macdDeaSeries!.setData(macd.dea);
    }

    if (this.kdjKSeries != null) {
      const kdj = computeKdj(sorted);
      this.kdjKSeries.setData(kdj.k);
      this.kdjDSeries!.setData(kdj.d);
      this.kdjJSeries!.setData(kdj.j);
    }

    if (this.atrSeries != null) {
      this.atrSeries.setData(computeAtr(sorted));
    }

    if (this.cciSeries != null) {
      this.cciSeries.setData(computeCci(sorted));
    }

    if (this.williamsRSeries != null) {
      this.williamsRSeries.setData(computeWilliamsR(sorted));
    }

    for (const [indicator, series] of this.overlaySeries.entries()) {
      series.setData(buildOverlaySeriesData(sorted, indicator));
    }
  }

  setCandles(candles: readonly KlineCandle[]): void {
    const previousFirstTimestamp = this.firstTimestamp;
    const visibleRange = this.chart.timeScale().getVisibleLogicalRange();
    const sorted = sortCandles(candles);
    const nextPeriod = sorted.find((candle) => candle.period != null)?.period ?? null;
    const periodChanged =
      this.currentPeriod != null &&
      nextPeriod != null &&
      nextPeriod !== this.currentPeriod;

    if (periodChanged) {
      this.hasFitInitialData = false;
      this.lastLoadMoreLogicalFrom = null;
    }

    this.currentCandles = sorted;
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
    this.updateIndicatorSeries(sorted);
    this.currentPeriod = nextPeriod;

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

  setIndicators(indicators: readonly KlineIndicatorKey[]): void {
    this.selectedIndicators = normalizeKlineIndicators(indicators);
    this.syncIndicatorSeries();
    this.updateIndicatorSeries(this.currentCandles);
  }

  applyPalette(palette: KlineChartPalette): void {
    this.palette = palette;
    this.chart.applyOptions({
      layout: {
        background: { type: ColorType.Solid, color: palette.bg },
        textColor: palette.text,
        panes: {
          separatorColor: palette.border,
        },
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
    this.macdDiffSeries?.applyOptions({ color: palette.indicatorA });
    this.macdDeaSeries?.applyOptions({ color: palette.indicatorB });
    this.kdjKSeries?.applyOptions({ color: palette.indicatorA });
    this.kdjDSeries?.applyOptions({ color: palette.indicatorB });
    this.kdjJSeries?.applyOptions({ color: palette.indicatorC });
    this.atrSeries?.applyOptions({ color: palette.indicatorA });
    this.cciSeries?.applyOptions({ color: palette.indicatorB });
    this.williamsRSeries?.applyOptions({ color: palette.indicatorC });
    // Histogram bar colours are set per data point in updateIndicatorSeries.
    this.updateIndicatorSeries(this.currentCandles);
  }

  resize(width: number, height: number): void {
    this.chart.resize(
      Math.max(1, Math.floor(width)),
      Math.max(1, Math.floor(height)),
      true,
    );
    this.applyPaneHeights();
  }

  /**
   * Give each indicator pane a fixed height so the main candle pane always
   * receives the majority of the vertical space.
   */
  private applyPaneHeights(): void {
    const panes = this.chart.panes();
    for (let i = 1; i < panes.length; i++) {
      panes[i]?.setHeight(INDICATOR_PANE_HEIGHT);
    }
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
    return new LightweightChartsKlineAdapter(
      host,
      options.palette,
      options.indicators,
    );
  },
};
