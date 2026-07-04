// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick, ref } from "vue";

import BacktestChart, {
  type BacktestCandle,
  type BacktestDrawdownPoint,
  type BacktestPnlPoint,
  type BacktestTrade,
} from "../src/components/BacktestChart.vue";
import { provideThemeStore, type ThemeStore } from "../src/composables/useTheme";

const chartMocks = vi.hoisted(() => {
  const series = Array.from({ length: 6 }, () => ({
    applyOptions: vi.fn(),
    setData: vi.fn(),
  }));
  const applyOptions = vi.fn();
  const resize = vi.fn();
  const remove = vi.fn();
  const panes = Array.from({ length: 4 }, () => ({
    setHeight: vi.fn(),
  }));
  const setVisibleLogicalRange = vi.fn();
  const getVisibleLogicalRange = vi.fn(() => ({ from: 12, to: 132 }));
  let visibleRangeHandler:
    | ((range: { from: number; to: number } | null) => void)
    | null = null;
  const subscribeVisibleLogicalRangeChange = vi.fn(
    (handler: (range: { from: number; to: number } | null) => void) => {
      visibleRangeHandler = handler;
    },
  );
  const markerApi = {
    detach: vi.fn(),
    setMarkers: vi.fn(),
  };
  let chartOptions: Record<string, unknown> | null = null;

  const chart = {
    addSeries: vi.fn((_definition: unknown, _options: unknown, _pane?: number) => {
      const next = series[chart.addSeries.mock.calls.length - 1];
      if (!next) throw new Error("unexpected series");
      return next;
    }),
    applyOptions,
    remove,
    panes: vi.fn(() => panes),
    resize,
    timeScale: vi.fn(() => ({
      getVisibleLogicalRange,
      setVisibleLogicalRange,
      subscribeVisibleLogicalRangeChange,
    })),
  };

  const createChart = vi.fn((_host: HTMLElement, options: Record<string, unknown>) => {
    chartOptions = options;
    return chart;
  });
  const createSeriesMarkers = vi.fn(() => markerApi);

  return {
    applyOptions,
    chart,
    createChart,
    createSeriesMarkers,
    getChartOptions: () => chartOptions,
    getVisibleLogicalRange,
    markerApi,
    panes,
    remove,
    resize,
    series,
    setVisibleLogicalRange,
    subscribeVisibleLogicalRangeChange,
    triggerVisibleRange(range: { from: number; to: number } | null) {
      visibleRangeHandler?.(range);
    },
  };
});

vi.mock("lightweight-charts", () => ({
  CandlestickSeries: { type: "Candlestick" },
  ColorType: { Solid: "solid" },
  CrosshairMode: { Normal: 0 },
  HistogramSeries: { type: "Histogram" },
  LineSeries: { type: "Line" },
  TickMarkType: { Year: 0, Month: 1, DayOfMonth: 2, Time: 3, TimeWithSeconds: 4 },
  createChart: chartMocks.createChart,
  createSeriesMarkers: chartMocks.createSeriesMarkers,
}));

let resizeCallback: ResizeObserverCallback | null = null;
const disconnectResizeObserver = vi.fn();

class MockResizeObserver {
  constructor(callback: ResizeObserverCallback) {
    resizeCallback = callback;
  }

  observe(): void {
    resizeCallback?.([], this as unknown as ResizeObserver);
  }

  disconnect(): void {
    disconnectResizeObserver();
  }
}

const candles: BacktestCandle[] = [
  {
    time: "2026-06-01T01:30:00.000Z",
    open: 100,
    high: 103,
    low: 99,
    close: 102,
    volume: 1_000,
  },
  {
    time: "2026-06-01T01:31:00.000Z",
    open: 102,
    high: 103,
    low: 97,
    close: 98,
    volume: 1_500,
  },
];

const trades: BacktestTrade[] = [
  { time: candles[0]!.time, side: "buy", price: 100, qty: 10 },
  { time: candles[1]!.time, side: "SELL", price: 98, qty: 4 },
];

const pnlCurve: BacktestPnlPoint[] = [
  { time: candles[0]!.time, equity: 100_000 },
  { time: candles[1]!.time, equity: 100_020 },
];

const drawdownCurve: BacktestDrawdownPoint[] = [
  { time: candles[0]!.time, drawdown: -0.1 },
  { time: candles[1]!.time, drawdown: 1.2 },
];

beforeEach(() => {
  resizeCallback = null;
  vi.stubGlobal("ResizeObserver", MockResizeObserver);
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    callback(1);
    return 1;
  });
  vi.stubGlobal("cancelAnimationFrame", vi.fn());
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    x: 0,
    y: 0,
    width: 720,
    height: 480,
    top: 0,
    right: 720,
    bottom: 480,
    left: 0,
    toJSON: () => ({}),
  });
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  window.localStorage.clear();
  chartMocks.createChart.mockReset();
  chartMocks.createChart.mockImplementation((_host, options) => {
    // Preserve the default implementation after tests that inject failures.
    void options;
    return chartMocks.chart;
  });
  chartMocks.chart.addSeries.mockClear();
  chartMocks.applyOptions.mockClear();
  chartMocks.createSeriesMarkers.mockClear();
  chartMocks.getVisibleLogicalRange.mockClear();
  chartMocks.markerApi.detach.mockClear();
  chartMocks.markerApi.setMarkers.mockClear();
  chartMocks.chart.panes.mockClear();
  chartMocks.remove.mockReset();
  chartMocks.resize.mockClear();
  chartMocks.setVisibleLogicalRange.mockClear();
  chartMocks.subscribeVisibleLogicalRangeChange.mockClear();
  chartMocks.series.forEach((series) => {
    series.applyOptions.mockClear();
    series.setData.mockClear();
  });
  chartMocks.panes.forEach((pane) => {
    pane.setHeight.mockClear();
  });
  disconnectResizeObserver.mockClear();
});

function mountChart(options: {
  candles?: BacktestCandle[];
  fitContainer?: boolean;
  trades?: BacktestTrade[];
  pnlCurve?: BacktestPnlPoint[];
  drawdownCurve?: BacktestDrawdownPoint[];
  currency?: string;
} = {}) {
  const state = {
    candles: ref(options.candles ?? candles),
    trades: ref(options.trades ?? trades),
    pnlCurve: ref(options.pnlCurve ?? pnlCurve),
    drawdownCurve: ref(options.drawdownCurve ?? drawdownCurve),
    currency: ref(options.currency ?? "hkd"),
    fitContainer: ref(options.fitContainer ?? false),
  };
  let themeStore: ThemeStore | null = null;
  const Host = defineComponent({
    components: { BacktestChart },
    setup() {
      themeStore = provideThemeStore();
      return state;
    },
    template: `
      <BacktestChart
        :candles="candles"
        :trades="trades"
        :pnl-curve="pnlCurve"
        :drawdown-curve="drawdownCurve"
        :initial-balance="100000"
        :currency-unit="currency"
        :min-height="480"
        :fit-container="fitContainer"
        empty-text="没有可绘制的结果"
      />
    `,
  });
  return { state, themeStore: () => themeStore!, wrapper: mount(Host) };
}

describe("BacktestChart", () => {
  it("renders normalized candles, trade markers, equity and bounded drawdown data", async () => {
    const { wrapper } = mountChart();
    await nextTick();

    expect(wrapper.text()).toContain("2 根 · HKD");
    expect(wrapper.text()).toContain("×2");
    expect(wrapper.text()).toContain("基准 HKD 100,000.00");
    expect(chartMocks.createChart).toHaveBeenCalledWith(
      expect.any(HTMLElement),
      expect.objectContaining({ width: 720, height: 480 }),
    );
    expect(chartMocks.panes.map((pane) => pane.setHeight.mock.calls.at(-1)?.[0])).toEqual([
      251,
      57,
      105,
      67,
    ]);
    expect(
      chartMocks.panes.reduce(
        (sum, pane) => sum + (pane.setHeight.mock.calls.at(-1)?.[0] as number),
        0,
      ),
    ).toBe(480);
    expect(chartMocks.series[0]!.setData).toHaveBeenLastCalledWith([
      { time: 1780277400, open: 100, high: 103, low: 99, close: 102 },
      { time: 1780277460, open: 102, high: 103, low: 97, close: 98 },
    ]);
    expect(chartMocks.series[1]!.setData).toHaveBeenLastCalledWith([
      expect.objectContaining({ time: 1780277400, value: 1000, color: "rgba(22, 199, 132, 0.45)" }),
      expect.objectContaining({ time: 1780277460, value: 1500, color: "rgba(234, 57, 67, 0.45)" }),
    ]);
    expect(chartMocks.series[2]!.setData).toHaveBeenLastCalledWith([
      { time: 1780277400, value: 100000 },
      { time: 1780277460, value: 100020 },
    ]);
    expect(chartMocks.series[3]!.setData).toHaveBeenLastCalledWith([
      { time: 1780277400, value: 100000 },
      { time: 1780277460, value: 100000 },
    ]);
    expect(chartMocks.series[4]!.setData).toHaveBeenLastCalledWith([
      { time: 1780277400, value: 1 },
      { time: 1780277460, value: 0 },
    ]);
    expect(chartMocks.markerApi.setMarkers).toHaveBeenLastCalledWith([
      expect.objectContaining({ position: "belowBar", shape: "arrowUp", text: "买入 10股 HKD 1,000.00" }),
      expect.objectContaining({ position: "aboveBar", shape: "arrowDown", text: "卖出 4股 HKD 392.00" }),
    ]);
    expect(chartMocks.setVisibleLogicalRange).toHaveBeenCalledWith({ from: 0, to: 10 });

    const options = chartMocks.getChartOptions() as {
      localization: { timeFormatter: (time: unknown) => string };
      timeScale: { tickMarkFormatter: (time: unknown, kind: number) => string };
    };
    expect(options.localization.timeFormatter(1780277400)).not.toBe("");
    expect(options.localization.timeFormatter("not-a-date")).toBe("");
    expect(options.localization.timeFormatter({ year: 2026, month: 6, day: 1 })).not.toBe("");
    for (const kind of [0, 1, 2, 3]) {
      expect(options.timeScale.tickMarkFormatter(1780277400, kind)).not.toBe("");
    }
    expect(options.timeScale.tickMarkFormatter("not-a-date", 3)).toBe("");

    const drawdownOptions = chartMocks.chart.addSeries.mock.calls[4]?.[1] as {
      autoscaleInfoProvider: () => unknown;
      priceFormat: { formatter: (value: number) => string };
    };
    expect(drawdownOptions.priceFormat.formatter(0.1234)).toBe("12.34%");
    expect(drawdownOptions.autoscaleInfoProvider()).toEqual({
      priceRange: { minValue: 0, maxValue: 1 },
    });

    wrapper.unmount();
  });

  it("keeps fixed and container-fit layouts bounded while distributing available pane height", async () => {
    const fixed = mountChart();
    await nextTick();

    const fixedRoot = fixed.wrapper.get(".backtest-chart");
    const fixedBody = fixed.wrapper.get(".backtest-chart__body");
    expect(fixedRoot.classes()).toContain("backtest-chart--fixed");
    expect(fixedRoot.classes()).not.toContain("backtest-chart--fit");
    expect(fixedBody.attributes("style")).toContain("height: 480px");
    expect(fixedBody.attributes("style")).toContain("min-height: 480px");
    fixed.wrapper.unmount();

    chartMocks.createChart.mockClear();
    chartMocks.chart.addSeries.mockClear();
    chartMocks.panes.forEach((pane) => pane.setHeight.mockClear());
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      width: 720,
      height: 260,
      top: 0,
      right: 720,
      bottom: 260,
      left: 0,
      toJSON: () => ({}),
    });

    const fit = mountChart({ fitContainer: true });
    await nextTick();

    const fitRoot = fit.wrapper.get(".backtest-chart");
    const fitBody = fit.wrapper.get(".backtest-chart__body");
    expect(fitRoot.classes()).toContain("backtest-chart--fit");
    expect(fitBody.attributes("style")).toContain("min-height: 0");
    expect(fitBody.attributes("style") ?? "").not.toContain("height: 480px");
    expect(chartMocks.createChart).toHaveBeenCalledWith(
      expect.any(HTMLElement),
      expect.objectContaining({ width: 720, height: 260 }),
    );
    expect(chartMocks.panes.map((pane) => pane.setHeight.mock.calls.at(-1)?.[0])).toEqual([
      137,
      33,
      55,
      35,
    ]);

    fit.wrapper.unmount();
  });

  it("reacts to result, quote-currency and theme changes without recreating the chart", async () => {
    const { state, themeStore, wrapper } = mountChart();
    await nextTick();
    chartMocks.createChart.mockClear();

    state.candles.value = [candles[0]!];
    state.trades.value = [];
    state.pnlCurve.value = [];
    state.drawdownCurve.value = [];
    await nextTick();

    expect(chartMocks.series[0]!.setData).toHaveBeenLastCalledWith([
      { time: 1780277400, open: 100, high: 103, low: 99, close: 102 },
    ]);
    expect(chartMocks.series[3]!.setData).toHaveBeenLastCalledWith([]);
    expect(chartMocks.series[5]!.setData).toHaveBeenLastCalledWith([]);
    expect(chartMocks.markerApi.setMarkers).toHaveBeenLastCalledWith([]);

    state.currency.value = " usd ";
    themeStore().set("light");
    await nextTick();
    await nextTick();

    expect(wrapper.text()).toContain("1 根 · USD");
    expect(chartMocks.createChart).not.toHaveBeenCalled();
    expect(chartMocks.applyOptions).toHaveBeenLastCalledWith(
      expect.objectContaining({
        layout: expect.objectContaining({ textColor: "#0f172a" }),
      }),
    );
    expect(chartMocks.series[0]!.applyOptions).toHaveBeenLastCalledWith(
      expect.objectContaining({
        priceFormat: expect.objectContaining({ formatter: expect.any(Function) }),
      }),
    );
    const currencyFormatter = chartMocks.series[0]!.applyOptions.mock.calls.at(-1)?.[0]
      .priceFormat.formatter as (value: number) => string;
    expect(currencyFormatter(1234.5)).toBe("USD 1,234.50");

    wrapper.unmount();
  });

  it("expands a long result window at the left edge while preserving the viewport", async () => {
    const longCandles = Array.from({ length: 10_001 }, (_, index) => ({
      time: new Date(Date.UTC(2026, 0, 1, 0, index)).toISOString(),
      open: 100,
      high: 101,
      low: 99,
      close: 100,
      volume: index,
    }));
    const { wrapper } = mountChart({
      candles: longCandles,
      trades: [
        { time: longCandles[0]!.time, side: "BUY", price: 100, qty: 1 },
        { time: longCandles.at(-1)!.time, side: "SELL", price: 100, qty: 1 },
      ],
      pnlCurve: [],
      drawdownCurve: [],
    });
    await nextTick();

    expect(chartMocks.series[0]!.setData.mock.calls.at(-1)?.[0]).toHaveLength(5000);
    expect(chartMocks.markerApi.setMarkers.mock.calls.at(-1)?.[0]).toHaveLength(1);

    chartMocks.triggerVisibleRange(null);
    chartMocks.triggerVisibleRange({ from: 201, to: 321 });
    chartMocks.triggerVisibleRange({ from: 0, to: 120 });
    await Promise.resolve();
    chartMocks.triggerVisibleRange({ from: 0, to: 120 });
    await Promise.resolve();

    expect(chartMocks.getVisibleLogicalRange).toHaveBeenCalledTimes(2);
    expect(chartMocks.series[0]!.setData.mock.calls.at(-1)?.[0]).toHaveLength(10_001);
    expect(chartMocks.setVisibleLogicalRange).toHaveBeenCalledWith({ from: 5012, to: 5132 });
    expect(chartMocks.markerApi.setMarkers.mock.calls.at(-1)?.[0]).toHaveLength(2);

    wrapper.unmount();
  });

  it("shows an actionable initialization error and tolerates chart cleanup failures", async () => {
    chartMocks.createChart.mockImplementationOnce(() => {
      throw new Error("canvas unavailable");
    });
    const failed = mountChart();
    await nextTick();
    expect(failed.wrapper.text()).toContain("图表初始化失败: canvas unavailable");
    failed.wrapper.unmount();

    chartMocks.remove.mockImplementationOnce(() => {
      throw new Error("already removed");
    });
    const active = mountChart({ candles: [], trades: [], pnlCurve: [], drawdownCurve: [] });
    await nextTick();
    expect(active.wrapper.text()).toContain("没有可绘制的结果");
    active.wrapper.unmount();

    expect(disconnectResizeObserver).toHaveBeenCalled();
    expect(chartMocks.markerApi.detach).toHaveBeenCalled();
  });
});
