<script setup lang="ts">
import {
  type Logical,
  type LogicalRange,
  type UTCTimestamp,
  type SeriesMarker,
  type Time,
  type ISeriesMarkersPluginApi,
  TickMarkType,
  CandlestickSeries,
  ColorType,
  CrosshairMode,
  HistogramSeries,
  LineSeries,
  createChart,
  createSeriesMarkers,
} from "lightweight-charts";
import {
  computed,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";
import { resolveBacktestChartPaneHeights } from "./backtestChartPaneHeights";
import { useTheme } from "../composables/useTheme";
import { formatLocalDateTime } from "../utils/dateTime";

// ── Types ──
export interface BacktestTrade {
  time: string;
  side: string;
  price: number;
  qty: number;
  pnl?: number;
  brokerFee?: number;
  marketFee?: number;
  totalFee?: number;
  feeCurrency?: string;
  warmup?: boolean;
}

export interface BacktestPnlPoint {
  time: string;
  equity: number;
}

export interface BacktestDrawdownPoint {
  time: string;
  drawdown: number;
}

export interface BacktestCandle {
  time: string;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
}

const props = withDefaults(
  defineProps<{
    candles: readonly BacktestCandle[];
    trades: readonly BacktestTrade[];
    pnlCurve: readonly BacktestPnlPoint[];
    drawdownCurve: readonly BacktestDrawdownPoint[];
    initialBalance: number;
    currencyUnit?: string;
    minHeight?: number;
    fitContainer?: boolean;
    emptyText?: string;
  }>(),
  {
    minHeight: 560,
    fitContainer: false,
    emptyText: "暂无回测数据",
  },
);

const INITIAL_WINDOW_BARS = 5000;
const WINDOW_EXPAND_BARS = 5000;
const LOAD_MORE_THRESHOLD = 200;
const INITIAL_VISIBLE_BARS = 120;
const INITIAL_RIGHT_OFFSET_BARS = 8;

const host = ref<HTMLElement | null>(null);
const chartError = ref("");
const { theme } = useTheme();

const palette = computed(() =>
  theme.value === "light"
    ? {
        bg: "#ffffff",
        text: "#0f172a",
        grid: "rgba(15, 23, 42, 0.06)",
        border: "rgba(15, 23, 42, 0.12)",
        up: "#16c784",
        down: "#ea3943",
        volumeUp: "rgba(22, 199, 132, 0.45)",
        volumeDown: "rgba(234, 57, 67, 0.45)",
        pnl: "#2563eb",
        pnlBaseline: "rgba(15, 23, 42, 0.18)",
        drawdown: "#f97316",
        drawdownBaseline: "rgba(148, 163, 184, 0.35)",
        buyMarker: "#16c784",
        sellMarker: "#ea3943",
      }
    : {
        bg: "#1a1a1a",
        text: "#cbd5e1",
        grid: "rgba(148, 163, 184, 0.08)",
        border: "rgba(148, 163, 184, 0.16)",
        up: "#16c784",
        down: "#ea3943",
        volumeUp: "rgba(22, 199, 132, 0.45)",
        volumeDown: "rgba(234, 57, 67, 0.45)",
        pnl: "#60a5fa",
        pnlBaseline: "rgba(148, 163, 184, 0.25)",
        drawdown: "#fb923c",
        drawdownBaseline: "rgba(148, 163, 184, 0.4)",
        buyMarker: "#16c784",
        sellMarker: "#ea3943",
      },
);

const hasCandles = computed(() => props.candles.length > 0);
const hasTrades = computed(() => props.trades.length > 0);
const hasPnl = computed(() => props.pnlCurve.length > 0);
const hasDrawdown = computed(() => props.drawdownCurve.length > 0);
const hasData = computed(() => hasCandles.value || hasPnl.value || hasDrawdown.value);
const displayCurrencyUnit = computed(() => props.currencyUnit?.trim().toUpperCase() || "HKD");
const chartRootClasses = computed(() => [
  "backtest-chart rounded-lg border border-slate-200 bg-white",
  props.fitContainer ? "backtest-chart--fit" : "backtest-chart--fixed",
]);
const chartBodyStyle = computed(() =>
  props.fitContainer
    ? { minHeight: "0" }
    : { minHeight: `${props.minHeight}px`, height: `${props.minHeight}px` },
);

function formatCurrencyValue(value: number) {
  return `${displayCurrencyUnit.value} ${value.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

function buildCurrencyPriceFormat() {
  return {
    type: "custom" as const,
    formatter: formatCurrencyValue,
    minMove: 0.01,
  };
}

function formatDrawdownPercent(value: number) {
  return `${(value * 100).toFixed(2)}%`;
}

function normalizePeakRatio(drawdown: number) {
  return Math.max(0, Math.min(1, 1 - drawdown));
}

type ChartHandle = ReturnType<typeof createChart>;
type NormalizedCandleDatum = {
  time: UTCTimestamp;
  open: number;
  high: number;
  low: number;
  close: number;
};
type NormalizedHistogramDatum = {
  time: UTCTimestamp;
  value: number;
  color: string;
};
type NormalizedLineDatum = {
  time: UTCTimestamp;
  value: number;
};

let chart: ChartHandle | null = null;
let candleSeries: ReturnType<ChartHandle["addSeries"]> | null = null;
let volumeSeries: ReturnType<ChartHandle["addSeries"]> | null = null;
let pnlSeries: ReturnType<ChartHandle["addSeries"]> | null = null;
let pnlBaselineSeries: ReturnType<ChartHandle["addSeries"]> | null = null;
let drawdownSeries: ReturnType<ChartHandle["addSeries"]> | null = null;
let drawdownBaselineSeries: ReturnType<ChartHandle["addSeries"]> | null = null;
let markersPlugin: ISeriesMarkersPluginApi<Time> | null = null;
let resizeObserver: ResizeObserver | null = null;
let resizeRaf: number | null = null;
let candleDataCache: NormalizedCandleDatum[] = [];
let volumeDataCache: NormalizedHistogramDatum[] = [];
let pnlDataCache: NormalizedLineDatum[] = [];
let drawdownDataCache: NormalizedLineDatum[] = [];
let markerDataCache: SeriesMarker<Time>[] = [];
let referenceTimesCache: UTCTimestamp[] = [];
let windowStartIndex = 0;
let isSyncingVisibleRange = false;

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

function formatBacktestChartTime(time: Time): string {
  const date = toDateFromChartTime(time);
  if (date == null) {
    return "";
  }
  return formatLocalDateTime(date, "");
}

function formatBacktestTickMark(time: Time, tickMarkType: TickMarkType): string {
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

function toLogical(value: number): Logical {
  return value as Logical;
}

function referenceLength() {
  return referenceTimesCache.length;
}

function setVisibleLogicalRange(range: LogicalRange) {
  if (!chart) {
    return;
  }

  isSyncingVisibleRange = true;
  chart.timeScale().setVisibleLogicalRange(range);
  queueMicrotask(() => {
    isSyncingVisibleRange = false;
  });
}

function rebuildPaletteDependentData() {
  const p = palette.value;
  const firstReferenceTime = referenceTimesCache[0];

  volumeDataCache = props.candles.map((candle, index) => {
    const prevClose = index > 0 ? props.candles[index - 1]!.close : candle.open;
    return {
      time: toTimestamp(candle.time),
      value: candle.volume,
      color: candle.close >= prevClose ? p.volumeUp : p.volumeDown,
    };
  });

  markerDataCache = props.trades.map((trade) => {
    const isBuy = trade.side.toUpperCase() === "BUY";
    const amount = trade.price * trade.qty;
    const tradeTime = toTimestamp(trade.time);
    const markerTime =
      trade.warmup &&
      firstReferenceTime != null &&
      tradeTime < firstReferenceTime
        ? firstReferenceTime
        : tradeTime;
    return {
      time: markerTime,
      position: isBuy ? "belowBar" : "aboveBar",
      color: isBuy ? p.buyMarker : p.sellMarker,
      shape: isBuy ? "arrowUp" : "arrowDown",
      text: `${trade.warmup ? "预热 · " : ""}${isBuy ? "买入" : "卖出"} ${trade.qty}股 ${formatCurrencyValue(amount)}`,
      size: 3,
    };
  });
}

function rebuildAllData() {
  candleDataCache = props.candles.map((candle) => ({
    time: toTimestamp(candle.time),
    open: candle.open,
    high: candle.high,
    low: candle.low,
    close: candle.close,
  }));
  pnlDataCache = props.pnlCurve.map((point) => ({
    time: toTimestamp(point.time),
    value: point.equity,
  }));
  drawdownDataCache = props.drawdownCurve.map((point) => ({
    time: toTimestamp(point.time),
    value: normalizePeakRatio(point.drawdown),
  }));
  referenceTimesCache = candleDataCache.length > 0
    ? candleDataCache.map((point) => point.time)
    : pnlDataCache.length > 0
      ? pnlDataCache.map((point) => point.time)
      : drawdownDataCache.map((point) => point.time);
  rebuildPaletteDependentData();

  const totalBars = referenceLength();
  windowStartIndex = totalBars > INITIAL_WINDOW_BARS ? totalBars - INITIAL_WINDOW_BARS : 0;
  applyCurrentWindow({ resetVisibleRange: true });
}

function applyCurrentWindow(options: {
  resetVisibleRange?: boolean;
  visibleRange?: LogicalRange | null;
  visibleShift?: number;
} = {}) {
  if (
    !candleSeries ||
    !volumeSeries ||
    !pnlSeries ||
    !pnlBaselineSeries ||
    !drawdownSeries ||
    !drawdownBaselineSeries
  ) {
    return;
  }

  const totalBars = referenceLength();
  const start = Math.min(windowStartIndex, totalBars);
  const visibleCandles = candleDataCache.slice(start);
  const visibleVolume = volumeDataCache.slice(start);
  const visiblePnl = pnlDataCache.slice(start);
  const visibleDrawdown = drawdownDataCache.slice(start);

  candleSeries.setData(visibleCandles);
  volumeSeries.setData(visibleVolume);
  pnlSeries.setData(visiblePnl);
  drawdownSeries.setData(visibleDrawdown);

  if (visiblePnl.length > 0) {
    pnlBaselineSeries.setData([
      { time: visiblePnl[0]!.time, value: props.initialBalance },
      { time: visiblePnl[visiblePnl.length - 1]!.time, value: props.initialBalance },
    ]);
  } else {
    pnlBaselineSeries.setData([]);
  }

  if (visibleDrawdown.length > 0) {
    drawdownBaselineSeries.setData([
      { time: visibleDrawdown[0]!.time, value: 1 },
      { time: visibleDrawdown[visibleDrawdown.length - 1]!.time, value: 1 },
    ]);
  } else {
    drawdownBaselineSeries.setData([]);
  }

  if (markersPlugin && referenceTimesCache.length > 0) {
    const firstVisibleTime = referenceTimesCache[start]!;
    const lastVisibleTime = referenceTimesCache[referenceTimesCache.length - 1]!;
    markersPlugin.setMarkers(
      markerDataCache.filter((marker) => {
        const markerTime = marker.time as UTCTimestamp;
        return markerTime >= firstVisibleTime && markerTime <= lastVisibleTime;
      }),
    );
  } else {
    markersPlugin?.setMarkers([]);
  }

  const windowLength = totalBars - start;
  if (options.resetVisibleRange && windowLength > 0) {
    setVisibleLogicalRange({
      from: toLogical(Math.max(0, windowLength - INITIAL_VISIBLE_BARS)),
      to: toLogical(windowLength + INITIAL_RIGHT_OFFSET_BARS),
    });
    return;
  }

  if (options.visibleRange != null && options.visibleShift != null && options.visibleShift > 0) {
    setVisibleLogicalRange({
      from: toLogical(options.visibleRange.from + options.visibleShift),
      to: toLogical(options.visibleRange.to + options.visibleShift),
    });
  }
}

function expandWindow(range: LogicalRange) {
  if (!chart || windowStartIndex === 0) {
    return;
  }

  const nextWindowStart = Math.max(0, windowStartIndex - WINDOW_EXPAND_BARS);
  if (nextWindowStart === windowStartIndex) {
    return;
  }

  const visibleRange = chart.timeScale().getVisibleLogicalRange() ?? range;
  const prependedCount = windowStartIndex - nextWindowStart;
  windowStartIndex = nextWindowStart;
  applyCurrentWindow({
    visibleRange,
    visibleShift: prependedCount,
  });
}

function handleVisibleLogicalRangeChange(range: LogicalRange | null) {
  if (
    range == null ||
    isSyncingVisibleRange ||
    windowStartIndex === 0 ||
    range.from > LOAD_MORE_THRESHOLD
  ) {
    return;
  }

  expandWindow(range);
}

function measureHost(el: HTMLElement): { width: number; height: number } {
  const rect = el.getBoundingClientRect();
  return {
    width: Math.max(1, Math.floor(rect.width || el.clientWidth || 1)),
    height: Math.max(1, Math.floor(rect.height || el.clientHeight || 1)),
  };
}

function applyPaneHeights(totalHeight: number) {
  if (!chart) {
    return;
  }

  const panes = chart.panes();
  const heights = resolveBacktestChartPaneHeights(totalHeight);
  panes.slice(0, heights.length).forEach((pane, index) => {
    pane.setHeight(heights[index]!);
  });
}

function buildChart() {
  const el = host.value;
  if (!el) return;
  destroyChart();
  chartError.value = "";

  try {
    const size = measureHost(el);
    const p = palette.value;

    chart = createChart(el, {
      width: size.width,
      height: size.height,
      layout: {
        background: { type: ColorType.Solid, color: p.bg },
        textColor: p.text,
        panes: {
          separatorColor: p.border,
          enableResize: true,
        },
      },
      grid: {
        vertLines: { color: p.grid },
        horzLines: { color: p.grid },
      },
      rightPriceScale: { borderColor: p.border },
      timeScale: {
        borderColor: p.border,
        barSpacing: 6,
        timeVisible: true,
        secondsVisible: true,
        tickMarkFormatter: (time: Time, tickMarkType: TickMarkType) => formatBacktestTickMark(time, tickMarkType),
      },
      localization: {
        timeFormatter: formatBacktestChartTime,
      },
      crosshair: { mode: CrosshairMode.Normal },
    });

    // ── Pane 0: K-line candlestick (main) ──
    candleSeries = chart.addSeries(CandlestickSeries, {
      upColor: p.up,
      downColor: p.down,
      borderUpColor: p.up,
      borderDownColor: p.down,
      wickUpColor: p.up,
      wickDownColor: p.down,
      priceFormat: buildCurrencyPriceFormat(),
    });

    // Markers plugin on candlestick series
    markersPlugin = createSeriesMarkers<Time>(candleSeries, []);

    // ── Pane 1: Volume histogram ──
    volumeSeries = chart.addSeries(
      HistogramSeries,
      {
        priceFormat: { type: "volume" },
        priceLineVisible: false,
        lastValueVisible: false,
      },
      1,
    );

    // ── Pane 2: P&L equity curve ──
    pnlSeries = chart.addSeries(
      LineSeries,
      {
        color: p.pnl,
        lineWidth: 2,
        priceLineVisible: false,
        lastValueVisible: true,
        crosshairMarkerVisible: true,
        priceFormat: buildCurrencyPriceFormat(),
      },
      2,
    );

    pnlBaselineSeries = chart.addSeries(
      LineSeries,
      {
        color: p.pnlBaseline,
        lineWidth: 1,
        lineStyle: 2,
        priceLineVisible: false,
        lastValueVisible: false,
        priceFormat: buildCurrencyPriceFormat(),
      },
      2,
    );

    drawdownSeries = chart.addSeries(
      LineSeries,
      {
        color: p.drawdown,
        lineWidth: 2,
        priceLineVisible: false,
        lastValueVisible: true,
        crosshairMarkerVisible: true,
        priceFormat: {
          type: "custom",
          formatter: formatDrawdownPercent,
          minMove: 0.0001,
        },
        autoscaleInfoProvider: () => ({
          priceRange: {
            minValue: 0,
            maxValue: 1,
          },
        }),
      },
      3,
    );

    drawdownBaselineSeries = chart.addSeries(
      LineSeries,
      {
        color: p.drawdownBaseline,
        lineWidth: 1,
        lineStyle: 2,
        priceLineVisible: false,
        lastValueVisible: false,
        priceFormat: {
          type: "custom",
          formatter: formatDrawdownPercent,
          minMove: 0.0001,
        },
        autoscaleInfoProvider: () => ({
          priceRange: {
            minValue: 0,
            maxValue: 1,
          },
        }),
      },
      3,
    );

    chart.timeScale().subscribeVisibleLogicalRangeChange(handleVisibleLogicalRangeChange);
    applyPaneHeights(size.height);
    rebuildAllData();
  } catch (e) {
    chartError.value = `图表初始化失败: ${e instanceof Error ? e.message : String(e)}`;
    destroyChart();
  }
}

function destroyChart() {
  markersPlugin?.detach();
  markersPlugin = null;
  if (chart) {
    try { chart.remove(); } catch { /* ignore */ }
    chart = null;
    candleSeries = null;
    volumeSeries = null;
    pnlSeries = null;
    pnlBaselineSeries = null;
    drawdownSeries = null;
    drawdownBaselineSeries = null;
  }
}

function handleResize() {
  // rAF-throttle: skip if a resize is already queued
  if (resizeRaf !== null) return;
  resizeRaf = requestAnimationFrame(() => {
    resizeRaf = null;
    const el = host.value;
    if (!el || !chart) return;
    const { width, height } = measureHost(el);
    chart.resize(width, height);
    applyPaneHeights(height);
  });
}

onMounted(() => {
  buildChart();
  resizeObserver = new ResizeObserver(() => handleResize());
  if (host.value) resizeObserver.observe(host.value);
});

onBeforeUnmount(() => {
  resizeObserver?.disconnect();
  if (resizeRaf !== null) {
    cancelAnimationFrame(resizeRaf);
    resizeRaf = null;
  }
  destroyChart();
});

// ── Apply palette without destroying chart (avoid full rebuild) ──
function applyPalette() {
  if (!chart) return;
  const p = palette.value;
  chart.applyOptions({
    layout: {
      background: { type: ColorType.Solid, color: p.bg },
      textColor: p.text,
    },
    grid: {
      vertLines: { color: p.grid },
      horzLines: { color: p.grid },
    },
    rightPriceScale: { borderColor: p.border },
    timeScale: { borderColor: p.border },
  });
  candleSeries?.applyOptions({
    upColor: p.up,
    downColor: p.down,
    borderUpColor: p.up,
    borderDownColor: p.down,
    wickUpColor: p.up,
    wickDownColor: p.down,
    priceFormat: buildCurrencyPriceFormat(),
  });
  pnlSeries?.applyOptions({ color: p.pnl, priceFormat: buildCurrencyPriceFormat() });
  pnlBaselineSeries?.applyOptions({ color: p.pnlBaseline, priceFormat: buildCurrencyPriceFormat() });
  drawdownSeries?.applyOptions({ color: p.drawdown });
  drawdownBaselineSeries?.applyOptions({ color: p.drawdownBaseline });
  rebuildPaletteDependentData();
  applyCurrentWindow();
}

// Shallow watch — parent passes new array references on data change, no need for deep
watch(
  () => [props.candles, props.pnlCurve, props.drawdownCurve, props.trades] as const,
  () => rebuildAllData(),
);
watch(displayCurrencyUnit, () => applyPalette());
watch(palette, () => applyPalette());
</script>

<template>
  <div :class="chartRootClasses">
    <!-- Legend -->
    <div
      v-if="hasData"
      class="flex shrink-0 flex-wrap items-center gap-x-4 gap-y-1 border-b border-slate-100 px-4 py-2 text-xs"
    >
      <div class="flex items-center gap-1.5">
        <span class="font-semibold text-slate-600">K线</span>
        <span class="text-slate-400">{{ candles.length }} 根 · {{ displayCurrencyUnit }}</span>
      </div>
      <div v-if="hasTrades" class="flex items-center gap-1.5">
        <span class="text-sm" :style="{ color: palette.buyMarker }">▲</span>
        <span class="text-slate-500">买</span>
        <span class="text-sm" :style="{ color: palette.sellMarker }">▼</span>
        <span class="text-slate-500">卖</span>
        <span class="text-slate-400">×{{ trades.length }}</span>
      </div>
      <div class="flex items-center gap-1.5">
        <span class="h-2.5 w-2.5 rounded-full" :style="{ backgroundColor: palette.pnl }" />
        <span class="text-slate-600">权益曲线</span>
      </div>
      <div v-if="hasDrawdown" class="flex items-center gap-1.5">
        <span class="h-2.5 w-2.5 rounded-full" :style="{ backgroundColor: palette.drawdown }" />
        <span class="text-slate-600">权益/峰值</span>
      </div>
      <span class="text-slate-400">基准 {{ formatCurrencyValue(initialBalance) }}</span>
    </div>

    <!-- Chart -->
    <div class="backtest-chart__body" :style="chartBodyStyle">
      <div
        v-if="chartError"
        class="backtest-chart__state flex items-center justify-center text-sm text-red-600"
      >
        {{ chartError }}
      </div>

      <div
        v-else-if="!hasData"
        class="backtest-chart__state flex items-center justify-center text-sm text-slate-400"
      >
        {{ emptyText }}
      </div>

      <div ref="host" class="backtest-chart__host" />
    </div>
  </div>
</template>

<style scoped>
.backtest-chart {
  display: flex;
  min-width: 0;
  overflow: hidden;
}

.backtest-chart--fixed {
  flex-direction: column;
}

.backtest-chart--fit {
  height: 100%;
  min-height: 0;
  flex-direction: column;
}

.backtest-chart__host,
.backtest-chart__state {
  min-width: 0;
}

.backtest-chart__body {
  position: relative;
  min-width: 0;
  background: inherit;
  overflow: hidden;
}

.backtest-chart__host {
  height: 100%;
}

.backtest-chart__state {
  position: absolute;
  inset: 0;
  z-index: 1;
  background: inherit;
}

.backtest-chart--fit .backtest-chart__host,
.backtest-chart--fit .backtest-chart__body {
  flex: 1 1 auto;
}
</style>
