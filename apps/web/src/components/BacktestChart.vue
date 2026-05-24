<script setup lang="ts">
import {
  type UTCTimestamp,
  type SeriesMarker,
  type Time,
  type ISeriesMarkersPluginApi,
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
import { useTheme } from "../composables/useTheme";

// ── Types ──
export interface BacktestTrade {
  time: string;
  side: string;
  price: number;
  qty: number;
  pnl?: number;
}

export interface BacktestPnlPoint {
  time: string;
  equity: number;
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
    initialBalance: number;
    minHeight?: number;
    emptyText?: string;
  }>(),
  {
    minHeight: 420,
    emptyText: "暂无回测数据",
  },
);

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
        buyMarker: "#16c784",
        sellMarker: "#ea3943",
      }
    : {
        bg: "#0f172a",
        text: "#cbd5e1",
        grid: "rgba(148, 163, 184, 0.08)",
        border: "rgba(148, 163, 184, 0.16)",
        up: "#16c784",
        down: "#ea3943",
        volumeUp: "rgba(22, 199, 132, 0.45)",
        volumeDown: "rgba(234, 57, 67, 0.45)",
        pnl: "#60a5fa",
        pnlBaseline: "rgba(148, 163, 184, 0.25)",
        buyMarker: "#16c784",
        sellMarker: "#ea3943",
      },
);

const hasCandles = computed(() => props.candles.length > 0);
const hasTrades = computed(() => props.trades.length > 0);
const hasPnl = computed(() => props.pnlCurve.length > 0);
const hasData = computed(() => hasCandles.value || hasPnl.value);

type ChartHandle = ReturnType<typeof createChart>;

let chart: ChartHandle | null = null;
let candleSeries: ReturnType<ChartHandle["addSeries"]> | null = null;
let volumeSeries: ReturnType<ChartHandle["addSeries"]> | null = null;
let pnlSeries: ReturnType<ChartHandle["addSeries"]> | null = null;
let pnlBaselineSeries: ReturnType<ChartHandle["addSeries"]> | null = null;
let markersPlugin: ISeriesMarkersPluginApi<Time> | null = null;
let resizeObserver: ResizeObserver | null = null;
let resizeRaf: number | null = null;

function toTimestamp(at: string): UTCTimestamp {
  return Math.floor(new Date(at).getTime() / 1000) as UTCTimestamp;
}

function measureHost(el: HTMLElement): { width: number; height: number } {
  const rect = el.getBoundingClientRect();
  return {
    width: Math.max(1, Math.floor(rect.width || el.clientWidth || 1)),
    height: Math.max(1, Math.floor(rect.height || el.clientHeight || 1)),
  };
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
        secondsVisible: false,
      },
      crosshair: { mode: CrosshairMode.Normal },
      localization: {
        priceFormatter: (price: number) =>
          `HK$${parseFloat(price.toFixed(2)).toLocaleString()}`,
      },
    });

    // ── Pane 0: K-line candlestick (main) ──
    candleSeries = chart.addSeries(CandlestickSeries, {
      upColor: p.up,
      downColor: p.down,
      borderUpColor: p.up,
      borderDownColor: p.down,
      wickUpColor: p.up,
      wickDownColor: p.down,
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
      },
      2,
    );

    loadData();
  } catch (e) {
    chartError.value = `图表初始化失败: ${e instanceof Error ? e.message : String(e)}`;
    destroyChart();
  }
}

function loadData() {
  if (!chart || !candleSeries || !volumeSeries || !pnlSeries || !pnlBaselineSeries) return;
  const p = palette.value;

  // ── K-line candles ──
  const candleData = props.candles.map((c) => ({
    time: toTimestamp(c.time),
    open: c.open,
    high: c.high,
    low: c.low,
    close: c.close,
  }));
  if (candleData.length > 0) {
    candleSeries.setData(candleData);
  }

  // ── Volume histogram ──
  const volumeData = props.candles.map((c, i) => {
    const prevClose = i > 0 ? props.candles[i - 1]!.close : c.open;
    const color = c.close >= prevClose ? p.volumeUp : p.volumeDown;
    return { time: toTimestamp(c.time), value: c.volume, color };
  });
  if (volumeData.length > 0) {
    volumeSeries.setData(volumeData);
  }

  // ── Trade markers on K-line chart ──
  if (markersPlugin && props.trades.length > 0) {
    const markers: SeriesMarker<Time>[] = props.trades.map((t) => {
      const isBuy = t.side.toUpperCase() === "BUY";
      const amount = t.price * t.qty;
      return {
        time: toTimestamp(t.time),
        position: isBuy ? "belowBar" : "aboveBar",
        color: isBuy ? p.buyMarker : p.sellMarker,
        shape: isBuy ? "arrowUp" : "arrowDown",
        text: `${isBuy ? "BUY" : "SELL"} ${t.qty}股 HK$${amount.toLocaleString(undefined, { minimumFractionDigits: 0, maximumFractionDigits: 0 })}`,
        size: 3,
      };
    });
    markersPlugin.setMarkers(markers);
  }

  // ── P&L equity curve ──
  const pnlData = props.pnlCurve.map((pt) => ({
    time: toTimestamp(pt.time),
    value: pt.equity,
  }));
  if (pnlData.length > 0) {
    pnlSeries.setData(pnlData);
    pnlBaselineSeries.setData([
      { time: pnlData[0]!.time, value: props.initialBalance },
      { time: pnlData[pnlData.length - 1]!.time, value: props.initialBalance },
    ]);
  }

  chart.timeScale().fitContent();
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
  });
  pnlSeries?.applyOptions({ color: p.pnl });
  pnlBaselineSeries?.applyOptions({ color: p.pnlBaseline });
  // Reload data for per-bar volume colors and trade markers (baked into data)
  loadData();
}

// Shallow watch — parent passes new array references on data change, no need for deep
watch(
  () => [props.candles, props.pnlCurve, props.trades] as const,
  () => loadData(),
);
watch(palette, () => applyPalette());
</script>

<template>
  <div class="backtest-chart rounded-lg border border-slate-200 bg-white">
    <!-- Legend -->
    <div
      v-if="hasData"
      class="flex flex-wrap items-center gap-x-4 gap-y-1 border-b border-slate-100 px-4 py-2 text-xs"
    >
      <div class="flex items-center gap-1.5">
        <span class="font-semibold text-slate-600">K线</span>
        <span class="text-slate-400">{{ candles.length }} 根</span>
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
      <span class="text-slate-400">基准 {{ initialBalance.toLocaleString() }} HKD</span>
    </div>

    <!-- Error -->
    <div
      v-if="chartError"
      class="flex items-center justify-center text-sm text-red-600"
      :style="{ minHeight: `${minHeight}px` }"
    >
      {{ chartError }}
    </div>

    <!-- Empty -->
    <div
      v-else-if="!hasData"
      class="flex items-center justify-center text-sm text-slate-400"
      :style="{ minHeight: `${minHeight}px` }"
    >
      {{ emptyText }}
    </div>

    <!-- Chart -->
    <div
      ref="host"
      :style="{ minHeight: `${minHeight}px`, height: `${minHeight}px` }"
    />
  </div>
</template>
