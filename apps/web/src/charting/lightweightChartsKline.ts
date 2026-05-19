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

import {
  normalizeKlineIndicators,
  type KlineCandle,
  type KlineChartAdapter,
  type KlineChartFactory,
  type KlineIndicatorKey,
  type KlineChartPalette,
} from "./kline";

const INITIAL_VISIBLE_BARS = 120;
const INITIAL_RIGHT_OFFSET_BARS = 8;
const DEFAULT_INDICATORS: KlineIndicatorKey[] = ["volume"];

type IndicatorPanelKey = "volume" | "macd" | "kdj";

type PanelMargins = {
  top: number;
  bottom: number;
};

const HIDDEN_PANEL_MARGINS: PanelMargins = {
  top: 0.99,
  bottom: 0.005,
};

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

function computeExponentialMovingAverage(
  values: readonly number[],
  period: number,
): Array<number | null> {
  const multiplier = 2 / (period + 1);
  let previous: number | null = null;

  return values.map((value) => {
    previous = previous == null ? value : previous + (value - previous) * multiplier;
    return previous;
  });
}

function computeMacd(candles: readonly KlineCandle[]) {
  const closes = candles.map((candle) => candle.close);
  const ema12 = computeExponentialMovingAverage(closes, 12);
  const ema26 = computeExponentialMovingAverage(closes, 26);
  const diff = closes.map((_, index) => {
    const fast = ema12[index];
    const slow = ema26[index];
    return fast == null || slow == null ? null : fast - slow;
  });
  const dea = computeExponentialMovingAverage(
    diff.map((value) => value ?? 0),
    9,
  );

  return candles.reduce(
    (result, candle, index) => {
      const timestamp = toTimestamp(candle.at);
      const diffValue = diff[index];
      const deaValue = dea[index];
      if (diffValue == null || deaValue == null) {
        return result;
      }

      const histogramValue = (diffValue - deaValue) * 2;
      result.diff.push({ time: timestamp, value: diffValue });
      result.dea.push({ time: timestamp, value: deaValue });
      result.histogram.push({ time: timestamp, value: histogramValue });
      return result;
    },
    {
      diff: [] as Array<{ time: UTCTimestamp; value: number }>,
      dea: [] as Array<{ time: UTCTimestamp; value: number }>,
      histogram: [] as Array<{ time: UTCTimestamp; value: number }>,
    },
  );
}

function computeKdj(candles: readonly KlineCandle[]) {
  let previousK = 50;
  let previousD = 50;

  return candles.reduce(
    (result, candle, index) => {
      const window = candles.slice(Math.max(0, index - 8), index + 1);
      const highestHigh = Math.max(...window.map((item) => item.high));
      const lowestLow = Math.min(...window.map((item) => item.low));
      const rsv =
        highestHigh === lowestLow
          ? 50
          : ((candle.close - lowestLow) / (highestHigh - lowestLow)) * 100;
      const nextK = (2 * previousK + rsv) / 3;
      const nextD = (2 * previousD + nextK) / 3;
      const nextJ = 3 * nextK - 2 * nextD;
      const timestamp = toTimestamp(candle.at);

      result.k.push({ time: timestamp, value: nextK });
      result.d.push({ time: timestamp, value: nextD });
      result.j.push({ time: timestamp, value: nextJ });

      previousK = nextK;
      previousD = nextD;
      return result;
    },
    {
      k: [] as Array<{ time: UTCTimestamp; value: number }>,
      d: [] as Array<{ time: UTCTimestamp; value: number }>,
      j: [] as Array<{ time: UTCTimestamp; value: number }>,
    },
  );
}

export class LightweightChartsKlineAdapter implements KlineChartAdapter {
  private readonly chart: IChartApi;
  private readonly candleSeries: ISeriesApi<"Candlestick">;
  private readonly volumeSeries: ISeriesApi<"Histogram">;
  private readonly macdHistogramSeries: ISeriesApi<"Histogram">;
  private readonly macdDiffSeries: ISeriesApi<"Line">;
  private readonly macdDeaSeries: ISeriesApi<"Line">;
  private readonly kdjKSeries: ISeriesApi<"Line">;
  private readonly kdjDSeries: ISeriesApi<"Line">;
  private readonly kdjJSeries: ISeriesApi<"Line">;
  private palette: KlineChartPalette;
  private selectedIndicators: KlineIndicatorKey[];
  private currentCandles: KlineCandle[] = [];
  private loadMoreHandler: (() => void) | null = null;
  private hasFitInitialData = false;
  private firstTimestamp: number | null = null;
  private lastTimestamp: number | null = null;
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
      priceScaleId: "volume",
      priceLineVisible: false,
      lastValueVisible: false,
    });
    this.macdHistogramSeries = this.chart.addHistogramSeries({
      priceScaleId: "macd",
      priceLineVisible: false,
      lastValueVisible: false,
    });
    this.macdDiffSeries = this.chart.addLineSeries({
      priceScaleId: "macd",
      lineWidth: 2,
      priceLineVisible: false,
      lastValueVisible: false,
      crosshairMarkerVisible: true,
    });
    this.macdDeaSeries = this.chart.addLineSeries({
      priceScaleId: "macd",
      lineWidth: 2,
      priceLineVisible: false,
      lastValueVisible: false,
      crosshairMarkerVisible: true,
    });
    this.kdjKSeries = this.chart.addLineSeries({
      priceScaleId: "kdj",
      lineWidth: 2,
      priceLineVisible: false,
      lastValueVisible: false,
      crosshairMarkerVisible: true,
    });
    this.kdjDSeries = this.chart.addLineSeries({
      priceScaleId: "kdj",
      lineWidth: 2,
      priceLineVisible: false,
      lastValueVisible: false,
      crosshairMarkerVisible: true,
    });
    this.kdjJSeries = this.chart.addLineSeries({
      priceScaleId: "kdj",
      lineWidth: 2,
      priceLineVisible: false,
      lastValueVisible: false,
      crosshairMarkerVisible: true,
    });
    this.applyPalette(palette);
    this.applyIndicatorLayout();
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

  private setSeriesVisible(series: ISeriesApi<any>, visible: boolean): void {
    series.applyOptions({ visible });
  }

  private setMainScaleMargins(margins: PanelMargins): void {
    this.candleSeries.priceScale().applyOptions({
      scaleMargins: margins,
    });
  }

  private setPanelMargins(
    priceScaleId: IndicatorPanelKey,
    margins: PanelMargins,
    visible = false,
  ): void {
    this.chart.priceScale(priceScaleId).applyOptions({
      autoScale: true,
      visible,
      borderVisible: visible,
      scaleMargins: margins,
    });
  }

  private applyIndicatorLayout(): void {
    const secondaryPanels = ["kdj", "macd", "volume"].filter((panel) =>
      this.selectedIndicators.includes(panel as KlineIndicatorKey),
    ) as IndicatorPanelKey[];

    this.setMainScaleMargins(
      secondaryPanels.length === 0
        ? { top: 0.02, bottom: 0.04 }
        : {
            top: 0.02,
            bottom:
              secondaryPanels.length === 1
                ? 0.22
                : secondaryPanels.length === 2
                  ? 0.34
                  : 0.41,
          },
    );

    const showVolume = this.selectedIndicators.includes("volume");
    const showMacd = this.selectedIndicators.includes("macd");
    const showKdj = this.selectedIndicators.includes("kdj");

    this.setSeriesVisible(this.volumeSeries, showVolume);
    this.setSeriesVisible(this.macdHistogramSeries, showMacd);
    this.setSeriesVisible(this.macdDiffSeries, showMacd);
    this.setSeriesVisible(this.macdDeaSeries, showMacd);
    this.setSeriesVisible(this.kdjKSeries, showKdj);
    this.setSeriesVisible(this.kdjDSeries, showKdj);
    this.setSeriesVisible(this.kdjJSeries, showKdj);

    this.setPanelMargins("volume", HIDDEN_PANEL_MARGINS, false);
    this.setPanelMargins("macd", HIDDEN_PANEL_MARGINS, false);
    this.setPanelMargins("kdj", HIDDEN_PANEL_MARGINS, false);

    if (secondaryPanels.length === 0) {
      return;
    }

    const bottomPadding = 0.02;
    const gap = 0.02;
    const panelHeight =
      secondaryPanels.length === 1
        ? 0.18
        : secondaryPanels.length === 2
          ? 0.14
          : 0.11;
    const totalSecondaryHeight =
      secondaryPanels.length * panelHeight +
      Math.max(0, secondaryPanels.length - 1) * gap;
    const mainBottom = 1 - bottomPadding - totalSecondaryHeight - gap;

    let currentTop = mainBottom + gap;
    for (const panel of secondaryPanels) {
      const panelBottom = currentTop + panelHeight;
      const margins = {
        top: currentTop,
        bottom: Math.max(bottomPadding, 1 - panelBottom),
      };

      if (panel === "volume") {
        this.setPanelMargins("volume", margins, true);
      } else if (panel === "macd") {
        this.setPanelMargins("macd", margins, true);
      } else {
        this.setPanelMargins("kdj", margins, true);
      }

      currentTop = panelBottom + gap;
    }
  }

  private updateIndicatorSeries(sorted: readonly KlineCandle[]): void {
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
    this.macdDiffSeries.setData(macd.diff);
    this.macdDeaSeries.setData(macd.dea);

    const kdj = computeKdj(sorted);
    this.kdjKSeries.setData(kdj.k);
    this.kdjDSeries.setData(kdj.d);
    this.kdjJSeries.setData(kdj.j);
  }

  setCandles(candles: readonly KlineCandle[]): void {
    const previousFirstTimestamp = this.firstTimestamp;
    const visibleRange = this.chart.timeScale().getVisibleLogicalRange();
    const sorted = sortCandles(candles);
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
    this.applyIndicatorLayout();
    this.updateIndicatorSeries(this.currentCandles);
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
    this.macdDiffSeries.applyOptions({ color: palette.indicatorA });
    this.macdDeaSeries.applyOptions({ color: palette.indicatorB });
    this.kdjKSeries.applyOptions({ color: palette.indicatorA });
    this.kdjDSeries.applyOptions({ color: palette.indicatorB });
    this.kdjJSeries.applyOptions({ color: palette.indicatorC });
    this.applyIndicatorLayout();
    this.updateIndicatorSeries(this.currentCandles);
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
    return new LightweightChartsKlineAdapter(
      host,
      options.palette,
      options.indicators,
    );
  },
};
