// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick, ref } from "vue";

import KlineChart from "../src/components/KlineChart.vue";
import { lightweightChartsKlineFactory } from "../src/charting/lightweightChartsKline";
import { provideUIColorPreferencesStore } from "../src/composables/useUIColorPreferences";
import { provideThemeStore } from "../src/composables/useTheme";

const chartMocks = vi.hoisted(() => {
  // Persistent per-role setData spies, shared across series recreations.
  const candlestickApplyOptions = vi.fn();
  const candlestickSetData = vi.fn();
  const volumeSetData = vi.fn();
  const macdHistogramSetData = vi.fn();
  const macdDiffSetData = vi.fn();
  const macdDeaSetData = vi.fn();
  const kdjKSetData = vi.fn();
  const kdjDSetData = vi.fn();
  const kdjJSetData = vi.fn();
  const movingAveragePeriods = [5, 10, 20, 30, 60, 120, 180, 250] as const;
  const overlayLineSetDataByTitle = Object.fromEntries(
    movingAveragePeriods.flatMap((period) => [
      [`MA${period}`, vi.fn()],
      [`EMA${period}`, vi.fn()],
    ]),
  ) as Record<string, ReturnType<typeof vi.fn>>;
  const resize = vi.fn();
  const fitContent = vi.fn();
  const setVisibleLogicalRange = vi.fn();
  const getVisibleLogicalRange = vi.fn(() => ({ from: 2, to: 3 }));
  const barsInLogicalRange = vi.fn(() => ({ barsBefore: 20 }));
  let visibleLogicalRangeCallback:
    | ((range: { from: number; to: number } | null) => void)
    | null = null;
  const subscribeVisibleLogicalRangeChange = vi.fn(
    (callback: (range: { from: number; to: number } | null) => void) => {
      visibleLogicalRangeCallback = callback;
    },
  );
  let lastCandlestickSeriesOptions: Record<string, unknown> | null = null;

  // Ordered queues used to map addSeries calls to the right spy.
  const histogramSetDataFns = [volumeSetData, macdHistogramSetData];
  const lineSetDataFns = [
    macdDiffSetData,
    macdDeaSetData,
    kdjKSetData,
    kdjDSetData,
    kdjJSetData,
  ];

  const createChart = vi.fn(() => {
    // Per-chart state — fresh on each createChart() call.
    let histogramIdx = 0;
    let lineIdx = 0;
    let panesArray: Array<{
      setHeight: ReturnType<typeof vi.fn>;
      paneIndex: ReturnType<typeof vi.fn>;
      getSeries: ReturnType<typeof vi.fn>;
    }> = [
      {
        setHeight: vi.fn(),
        paneIndex: vi.fn(() => 0),
        getSeries: vi.fn(() => []),
      },
    ];

    function ensurePanes(maxIdx: number): void {
      while (panesArray.length <= maxIdx) {
        const idx = panesArray.length;
        panesArray.push({
          setHeight: vi.fn(),
          paneIndex: vi.fn(() => idx),
          getSeries: vi.fn(() => []),
        });
      }
    }

    const addSeries = vi.fn(
      (definition: { type?: string }, opts: unknown, paneIdx = 0) => {
        ensurePanes(paneIdx);
        const typeName = definition?.type ?? "";
        const title =
          typeof opts === "object" && opts != null
            ? (opts as { title?: string }).title
            : undefined;
        let setDataFn: ReturnType<typeof vi.fn>;
        let applyOptionsFn: ReturnType<typeof vi.fn> = vi.fn();
        if (typeName === "Candlestick") {
          setDataFn = candlestickSetData;
          applyOptionsFn = candlestickApplyOptions;
          lastCandlestickSeriesOptions =
            typeof opts === "object" && opts != null
              ? ({ ...(opts as Record<string, unknown>) })
              : null;
        } else if (typeName === "Histogram") {
          setDataFn =
            histogramSetDataFns[histogramIdx++ % histogramSetDataFns.length];
        } else if (
          title != null &&
          Object.hasOwn(overlayLineSetDataByTitle, title)
        ) {
          setDataFn = overlayLineSetDataByTitle[title];
        } else {
          setDataFn = lineSetDataFns[lineIdx++ % lineSetDataFns.length];
        }
        return {
          setData: setDataFn,
          applyOptions: applyOptionsFn,
          priceScale: vi.fn(() => ({ applyOptions: vi.fn() })),
          barsInLogicalRange,
        };
      },
    );

    const removePane = vi.fn((idx: number) => {
      panesArray.splice(idx, 1);
      panesArray.forEach((p, i) => p.paneIndex.mockReturnValue(i));
      // Reset indicator counters so recreated series map to the correct spies.
      histogramIdx = 0;
      lineIdx = 0;
    });
    const removeSeries = vi.fn();

    return {
      addSeries,
      panes: vi.fn(() => [...panesArray]),
      removePane,
      removeSeries,
      applyOptions: vi.fn(),
      resize,
      remove: vi.fn(),
      timeScale: vi.fn(() => ({
        fitContent,
        getVisibleLogicalRange,
        setVisibleLogicalRange,
        subscribeVisibleLogicalRangeChange,
      })),
    };
  });

  return {
    barsInLogicalRange,
    candlestickApplyOptions,
    candlestickSetData,
    volumeSetData,
    macdHistogramSetData,
    macdDiffSetData,
    macdDeaSetData,
    kdjKSetData,
    kdjDSetData,
    kdjJSetData,
    overlayLineSetDataByTitle,
    resize,
    fitContent,
    getVisibleLogicalRange,
    getLastCandlestickSeriesOptions() {
      return lastCandlestickSeriesOptions;
    },
    setVisibleLogicalRange,
    subscribeVisibleLogicalRangeChange,
    triggerVisibleLogicalRange(range: { from: number; to: number } | null) {
      visibleLogicalRangeCallback?.(range);
    },
    createChart,
  };
});

vi.mock("lightweight-charts", () => ({
  ColorType: { Solid: "solid" },
  CrosshairMode: { Normal: 0 },
  LineStyle: { Solid: 0, Dashed: 1, Dotted: 2 },
  TickMarkType: { Year: 0, Month: 1, DayOfMonth: 2, Time: 3, TimeWithSeconds: 4 },
  CandlestickSeries: { type: "Candlestick" },
  HistogramSeries: { type: "Histogram" },
  LineSeries: { type: "Line" },
  createChart: chartMocks.createChart,
}));

class MockResizeObserver {
  constructor(private readonly callback: ResizeObserverCallback) {}

  observe(): void {
    this.callback([], this as unknown as ResizeObserver);
  }

  disconnect(): void {}
}

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  document.body.innerHTML = "";
  chartMocks.candlestickApplyOptions.mockClear();
  chartMocks.barsInLogicalRange.mockClear();
  chartMocks.candlestickSetData.mockClear();
  chartMocks.volumeSetData.mockClear();
  chartMocks.macdHistogramSetData.mockClear();
  chartMocks.macdDiffSetData.mockClear();
  chartMocks.macdDeaSetData.mockClear();
  chartMocks.kdjKSetData.mockClear();
  chartMocks.kdjDSetData.mockClear();
  chartMocks.kdjJSetData.mockClear();
  Object.values(chartMocks.overlayLineSetDataByTitle).forEach((spy) => spy.mockClear());
  chartMocks.resize.mockClear();
  chartMocks.fitContent.mockClear();
  chartMocks.getVisibleLogicalRange.mockClear();
  chartMocks.setVisibleLogicalRange.mockClear();
  chartMocks.subscribeVisibleLogicalRangeChange.mockClear();
  chartMocks.createChart.mockClear();
});

describe("KlineChart", () => {
  it("formats chart prices consistently and proxies fit-content behavior on the direct adapter", () => {
    vi.stubGlobal("ResizeObserver", MockResizeObserver);
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      width: 640,
      height: 320,
      top: 0,
      right: 640,
      bottom: 320,
      left: 0,
      toJSON: () => ({}),
    });

    const host = document.createElement("div");
    const adapter = lightweightChartsKlineFactory.create(host, {
      palette: {
        bg: "#000000",
        text: "#ffffff",
        grid: "#111111",
        border: "#222222",
        up: "#22c55e",
        down: "#ef4444",
        volumeUp: "#16a34a",
        volumeDown: "#dc2626",
        indicatorA: "#2563eb",
        indicatorB: "#f97316",
        indicatorC: "#8b5cf6",
        macdPositive: "#22c55e",
        macdNegative: "#ef4444",
      },
      indicators: ["ma5"],
    });

    const options = chartMocks.createChart.mock.calls.at(-1)?.[1] as {
      localization?: {
        priceFormatter?: (price: number) => string;
        timeFormatter?: (time: number | string | { year: number; month: number; day: number }) => string;
      };
      timeScale?: {
        tickMarkFormatter?: (time: number | string | { year: number; month: number; day: number }, type: number) => string;
      };
    };
    expect(options.localization?.priceFormatter?.(23.649999999999999)).toBe("23.65");
    expect(options.localization?.timeFormatter?.(1_718_000_000)).toContain("2024");
    expect(options.localization?.timeFormatter?.("2026-05-17T01:31:00.000Z")).toContain("2026");
    expect(options.localization?.timeFormatter?.("not-a-time")).toBe("");
    expect(options.timeScale?.tickMarkFormatter?.({ year: 2026, month: 5, day: 17 }, 0)).toContain("2026");
    expect(options.timeScale?.tickMarkFormatter?.("2026-05-17T01:31:00.000Z", 1)).toMatch(/\d{2}\/\d{2}/);
    expect(options.timeScale?.tickMarkFormatter?.("2026-05-17T01:31:00.000Z", 2)).toMatch(/\d{2}\/\d{2}/);
    expect(options.timeScale?.tickMarkFormatter?.("2026-05-17T01:31:00.000Z", 3)).toMatch(/\d{2}:\d{2}/);
    expect(options.timeScale?.tickMarkFormatter?.("not-a-time", 3)).toBe("");

    const loadMore = vi.fn();
    adapter.setLoadMoreHandler(loadMore);
    chartMocks.triggerVisibleLogicalRange(null);
    expect(loadMore).not.toHaveBeenCalled();

    adapter.fitContent();
    expect(chartMocks.fitContent).toHaveBeenCalledOnce();

    adapter.remove();
    const chart = chartMocks.createChart.mock.results.at(-1)?.value;
    expect(chart?.remove).toHaveBeenCalledOnce();
  });

  it("renders intraday candles at the bucket-end display time", async () => {
    vi.stubGlobal("ResizeObserver", MockResizeObserver);
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(1);
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      width: 640,
      height: 320,
      top: 0,
      right: 640,
      bottom: 320,
      left: 0,
      toJSON: () => ({}),
    });

    const candles = ref([
      {
        period: "1m",
        at: "2026-05-17T01:31:00.000Z",
        displayAt: "2026-05-17T01:32:00.000Z",
        open: 320.7,
        high: 321.1,
        low: 320.6,
        close: 321,
        volume: 21000,
      },
      {
        period: "1m",
        at: "2026-05-17T01:30:00.000Z",
        open: 320,
        high: 320.8,
        low: 319.9,
        close: 320.5,
        volume: 18000,
      },
    ]);

    const Host = defineComponent({
      components: { KlineChart },
      setup() {
        provideThemeStore();
        return { candles };
      },
      template: '<KlineChart :candles="candles" :min-height="320" />',
    });

    mount(Host);
    await nextTick();
    await nextTick();

    expect(chartMocks.createChart).toHaveBeenCalledWith(
      expect.any(HTMLElement),
      expect.objectContaining({
        width: 640,
        height: 320,
      }),
    );
    expect(chartMocks.candlestickSetData).toHaveBeenLastCalledWith([
      {
        time: 1778981460,
        open: 320,
        high: 320.8,
        low: 319.9,
        close: 320.5,
      },
      {
        time: 1778981520,
        open: 320.7,
        high: 321.1,
        low: 320.6,
        close: 321,
      },
    ]);
    expect(chartMocks.volumeSetData).toHaveBeenLastCalledWith([
      expect.objectContaining({ time: 1778981460, value: 18000 }),
      expect.objectContaining({ time: 1778981520, value: 21000 }),
    ]);
    expect(chartMocks.resize).toHaveBeenCalledWith(640, 320, true);
    expect(chartMocks.setVisibleLogicalRange).toHaveBeenCalledWith({
      from: -118,
      to: 10,
    });
    expect(chartMocks.fitContent).not.toHaveBeenCalled();
  });

  it("emits load-more near the left edge and anchors the viewport after prepending candles", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("ResizeObserver", MockResizeObserver);
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(1);
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      width: 640,
      height: 320,
      top: 0,
      right: 640,
      bottom: 320,
      left: 0,
      toJSON: () => ({}),
    });

    const candles = ref([
      {
        at: "2026-05-17T01:30:00.000Z",
        open: 320,
        high: 320.8,
        low: 319.9,
        close: 320.5,
        volume: 18000,
      },
      {
        at: "2026-05-17T01:31:00.000Z",
        open: 320.7,
        high: 321.1,
        low: 320.6,
        close: 321,
        volume: 21000,
      },
    ]);

    const Host = defineComponent({
      components: { KlineChart },
      setup() {
        provideThemeStore();
        return { candles };
      },
      template:
        '<KlineChart :candles="candles" :min-height="320" @load-more="$emit(\'load-more\')" />',
    });

    const wrapper = mount(Host);
    await nextTick();
    await nextTick();

    chartMocks.barsInLogicalRange.mockReturnValueOnce({ barsBefore: 0 });
    chartMocks.triggerVisibleLogicalRange({ from: 0, to: 2 });
    expect(wrapper.emitted("load-more")).toHaveLength(1);

    candles.value = [
      {
        at: "2026-05-17T01:29:00.000Z",
        open: 319.8,
        high: 320.2,
        low: 319.6,
        close: 320,
        volume: 12000,
      },
      ...candles.value,
    ];
    await nextTick();
    await nextTick();

    expect(chartMocks.setVisibleLogicalRange).toHaveBeenCalledWith({
      from: 3,
      to: 4,
    });
  });

  it("aggregates multiple ticks in the same chart second without losing OHLC order", async () => {
    vi.stubGlobal("ResizeObserver", MockResizeObserver);
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(1);
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      width: 640,
      height: 320,
      top: 0,
      right: 640,
      bottom: 320,
      left: 0,
      toJSON: () => ({}),
    });

    const candles = ref([
      {
        at: "2026-05-17T01:31:00.100Z",
        open: 320.7,
        high: 320.7,
        low: 320.7,
        close: 320.7,
        volume: 21000,
      },
      {
        at: "2026-05-17T01:31:00.800Z",
        open: 321.2,
        high: 321.2,
        low: 321.2,
        close: 321.2,
        volume: 21010,
      },
      {
        at: "2026-05-17T01:31:00.400Z",
        open: 320.4,
        high: 320.4,
        low: 320.4,
        close: 320.4,
        volume: 21005,
      },
    ]);

    const Host = defineComponent({
      components: { KlineChart },
      setup() {
        provideThemeStore();
        return { candles };
      },
      template: '<KlineChart :candles="candles" :min-height="320" />',
    });

    mount(Host);
    await nextTick();
    await nextTick();

    expect(chartMocks.candlestickSetData).toHaveBeenLastCalledWith([
      {
        time: 1778981460,
        open: 320.7,
        high: 321.2,
        low: 320.4,
        close: 321.2,
      },
    ]);
  });

  it("drops invalid candles before they reach the chart adapter", async () => {
    vi.stubGlobal("ResizeObserver", MockResizeObserver);
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(1);
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      width: 640,
      height: 320,
      top: 0,
      right: 640,
      bottom: 320,
      left: 0,
      toJSON: () => ({}),
    });

    const candles = ref([
      {
        at: "invalid-time",
        open: 320,
        high: 321,
        low: 319,
        close: 320.5,
        volume: 1000,
      },
      {
        at: "2026-05-17T01:31:00.000Z",
        open: Number.NaN,
        high: 321.1,
        low: 320.6,
        close: 321,
        volume: 21000,
      },
      {
        at: "2026-05-17T01:32:00.000Z",
        open: 321,
        high: 321.5,
        low: 320.9,
        close: 321.2,
        volume: 22000,
      },
    ]);

    const Host = defineComponent({
      components: { KlineChart },
      setup() {
        provideThemeStore();
        return { candles };
      },
      template: '<KlineChart :candles="candles" :min-height="320" />',
    });

    mount(Host);
    await nextTick();
    await nextTick();

    expect(chartMocks.candlestickSetData).toHaveBeenLastCalledWith([
      {
        time: 1778981520,
        open: 321,
        high: 321.5,
        low: 320.9,
        close: 321.2,
      },
    ]);
  });

  it("updates candle colors when the rise-is-red preference is enabled", async () => {
    vi.stubGlobal("ResizeObserver", MockResizeObserver);
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(1);
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      width: 640,
      height: 320,
      top: 0,
      right: 640,
      bottom: 320,
      left: 0,
      toJSON: () => ({}),
    });

    const candles = ref([
      {
        period: "1m",
        at: "2026-05-17T01:31:00.000Z",
        open: 320.7,
        high: 321.1,
        low: 320.6,
        close: 321,
        volume: 20000,
      },
    ]);

    let colorStore: ReturnType<typeof provideUIColorPreferencesStore> | null = null;
    const Host = defineComponent({
      components: { KlineChart },
      setup() {
        const themeStore = provideThemeStore();
        colorStore = provideUIColorPreferencesStore(themeStore);
        return { candles };
      },
      template: '<KlineChart :candles="candles" :min-height="320" />',
    });

    mount(Host);
    await nextTick();
    await nextTick();

    expect(chartMocks.getLastCandlestickSeriesOptions()).toMatchObject({
      upColor: "#16c784",
      downColor: "#ea3943",
      borderUpColor: "#16c784",
      borderDownColor: "#ea3943",
    });

    colorStore?.setRiseIsRed(true);
    await nextTick();
    await nextTick();

    expect(chartMocks.candlestickApplyOptions).toHaveBeenLastCalledWith(
      expect.objectContaining({
        upColor: "#ea3943",
        downColor: "#16c784",
        borderUpColor: "#ea3943",
        borderDownColor: "#16c784",
      }),
    );
  });

  it("adds separate indicator panes when selector checkboxes are toggled", async () => {
    vi.stubGlobal("ResizeObserver", MockResizeObserver);
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(1);
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      width: 640,
      height: 320,
      top: 0,
      right: 640,
      bottom: 320,
      left: 0,
      toJSON: () => ({}),
    });

    const candles = ref([
      {
        at: "2026-05-17T01:30:00.000Z",
        open: 320,
        high: 320.8,
        low: 319.9,
        close: 320.5,
        volume: 18000,
      },
      {
        at: "2026-05-17T01:31:00.000Z",
        open: 320.7,
        high: 321.1,
        low: 320.6,
        close: 321,
        volume: 21000,
      },
    ]);

    const Host = defineComponent({
      components: { KlineChart },
      setup() {
        provideThemeStore();
        return { candles };
      },
      template:
        '<KlineChart :candles="candles" :min-height="320" :show-indicator-selector="true" />',
    });

    const wrapper = mount(Host, { attachTo: document.body });
    await nextTick();
    await nextTick();

    // Open the selector and enable MACD / KDJ from the popup.
    await wrapper.get("button.kline-chart-trigger").trigger("click");
    const macdInput = document.body.querySelector(
      "input[value='macd']",
    ) as HTMLInputElement | null;
    expect(macdInput).not.toBeNull();
    macdInput?.dispatchEvent(new Event("change", { bubbles: true }));
    await nextTick();

    // MACD pane series should have received data.
    expect(chartMocks.macdHistogramSetData).toHaveBeenCalled();
    expect(chartMocks.macdDiffSetData).toHaveBeenCalled();
    expect(chartMocks.macdDeaSetData).toHaveBeenCalled();

    const kdjInput = document.body.querySelector(
      "input[value='kdj']",
    ) as HTMLInputElement | null;
    expect(kdjInput).not.toBeNull();
    kdjInput?.dispatchEvent(new Event("change", { bubbles: true }));
    await nextTick();

    // KDJ pane series should have received data.
    expect(chartMocks.kdjKSetData).toHaveBeenCalled();
    expect(chartMocks.kdjDSetData).toHaveBeenCalled();
    expect(chartMocks.kdjJSetData).toHaveBeenCalled();
  });

  it("renders MA and EMA overlays in the main pane without adding extra pane height", async () => {
    vi.stubGlobal("ResizeObserver", MockResizeObserver);
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(1);
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      width: 640,
      height: 320,
      top: 0,
      right: 640,
      bottom: 320,
      left: 0,
      toJSON: () => ({}),
    });

    const candles = ref([
      {
        at: "2026-05-17T01:30:00.000Z",
        open: 320,
        high: 320.8,
        low: 319.9,
        close: 320.5,
        volume: 18000,
      },
      {
        at: "2026-05-17T01:31:00.000Z",
        open: 320.7,
        high: 321.1,
        low: 320.6,
        close: 321,
        volume: 21000,
      },
      {
        at: "2026-05-17T01:32:00.000Z",
        open: 321.1,
        high: 321.5,
        low: 320.9,
        close: 321.2,
        volume: 22000,
      },
      {
        at: "2026-05-17T01:33:00.000Z",
        open: 321.4,
        high: 321.8,
        low: 321.1,
        close: 321.6,
        volume: 23000,
      },
      {
        at: "2026-05-17T01:34:00.000Z",
        open: 321.7,
        high: 322,
        low: 321.4,
        close: 321.9,
        volume: 24000,
      },
      {
        at: "2026-05-17T01:35:00.000Z",
        open: 321.9,
        high: 322.3,
        low: 321.5,
        close: 322.1,
        volume: 25000,
      },
    ]);

    const Host = defineComponent({
      components: { KlineChart },
      setup() {
        provideThemeStore();
        return { candles };
      },
      template:
        '<KlineChart :candles="candles" :min-height="320" show-indicator-selector />',
    });

    const wrapper = mount(Host, { attachTo: document.body });
    await nextTick();
    await nextTick();

    const shell = wrapper.get(".kline-chart-shell").element as HTMLElement;
    expect(shell.getAttribute("style") ?? "").toContain("--kline-min-h: 440px");

    await wrapper.get("button.kline-chart-trigger").trigger("click");
    const ma5Input = document.body.querySelector(
      "input[value='ma5']",
    ) as HTMLInputElement | null;
    expect(ma5Input).not.toBeNull();
    ma5Input?.dispatchEvent(new Event("change", { bubbles: true }));
    const ema5Input = document.body.querySelector(
      "input[value='ema5']",
    ) as HTMLInputElement | null;
    expect(ema5Input).not.toBeNull();
    ema5Input?.dispatchEvent(new Event("change", { bubbles: true }));
    await nextTick();

    expect(chartMocks.overlayLineSetDataByTitle.MA5).toHaveBeenCalled();
    expect(chartMocks.overlayLineSetDataByTitle.EMA5).toHaveBeenCalled();
    expect(shell.getAttribute("style") ?? "").toContain("--kline-min-h: 440px");
  });

  it("recenters the chart on the latest bars when the candle period changes", async () => {
    vi.stubGlobal("ResizeObserver", MockResizeObserver);
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(1);
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      width: 640,
      height: 320,
      top: 0,
      right: 640,
      bottom: 320,
      left: 0,
      toJSON: () => ({}),
    });

    const candles = ref([
      {
        period: "1m",
        at: "2026-05-17T01:30:00.000Z",
        open: 320,
        high: 320.8,
        low: 319.9,
        close: 320.5,
        volume: 18000,
      },
      {
        period: "1m",
        at: "2026-05-17T01:31:00.000Z",
        open: 320.7,
        high: 321.1,
        low: 320.6,
        close: 321,
        volume: 21000,
      },
    ]);

    const Host = defineComponent({
      components: { KlineChart },
      setup() {
        provideThemeStore();
        return { candles };
      },
      template: '<KlineChart :candles="candles" :min-height="320" />',
    });

    mount(Host);
    await nextTick();
    await nextTick();

    chartMocks.setVisibleLogicalRange.mockClear();

    candles.value = [
      {
        period: "5m",
        at: "2026-05-17T01:25:00.000Z",
        open: 319.8,
        high: 320.2,
        low: 319.6,
        close: 320,
        volume: 12000,
      },
      {
        period: "5m",
        at: "2026-05-17T01:30:00.000Z",
        open: 320.5,
        high: 321.2,
        low: 320.4,
        close: 321,
        volume: 15000,
      },
    ];
    await nextTick();
    await nextTick();

    expect(chartMocks.setVisibleLogicalRange).toHaveBeenCalledTimes(1);
    expect(chartMocks.setVisibleLogicalRange).toHaveBeenLastCalledWith({
      from: -118,
      to: 10,
    });
  });

  it("restores persisted indicators and persists subsequent indicator changes", async () => {
    vi.stubGlobal("ResizeObserver", MockResizeObserver);
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(1);
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      width: 640,
      height: 320,
      top: 0,
      right: 640,
      bottom: 320,
      left: 0,
      toJSON: () => ({}),
    });
    window.localStorage.setItem(
      "chart-indicators",
      JSON.stringify(["ma5", "invalid"]),
    );

    const candles = ref([
      {
        at: "2026-05-17T01:30:00.000Z",
        open: 320,
        high: 320.8,
        low: 319.9,
        close: 320.5,
        volume: 18000,
      },
      {
        at: "2026-05-17T01:31:00.000Z",
        open: 320.7,
        high: 321.1,
        low: 320.6,
        close: 321,
        volume: 21000,
      },
    ]);

    const Host = defineComponent({
      components: { KlineChart },
      setup() {
        provideThemeStore();
        return { candles };
      },
      template:
        '<KlineChart :candles="candles" :min-height="320" show-indicator-selector indicator-storage-key="chart-indicators" />',
    });

    const wrapper = mount(Host, { attachTo: document.body });
    await nextTick();
    await nextTick();

    expect(chartMocks.overlayLineSetDataByTitle.MA5).toHaveBeenCalled();

    await wrapper.get("button.kline-chart-trigger").trigger("click");
    const ema5Input = document.body.querySelector(
      "input[value='ema5']",
    ) as HTMLInputElement | null;
    expect(ema5Input).not.toBeNull();
    ema5Input?.dispatchEvent(new Event("change", { bubbles: true }));
    await nextTick();

    expect(window.localStorage.getItem("chart-indicators")).toBe(
      JSON.stringify(["ma5", "ema5"]),
    );
  });

  it("restores ATR, CCI, and Williams %R panes from persisted indicators and disposes the chart on unmount", async () => {
    vi.stubGlobal("ResizeObserver", MockResizeObserver);
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(1);
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      width: 640,
      height: 320,
      top: 0,
      right: 640,
      bottom: 320,
      left: 0,
      toJSON: () => ({}),
    });
    window.localStorage.setItem(
      "indicator-panes",
      JSON.stringify(["atr", "cci", "williamsr"]),
    );

    const candles = ref([
      {
        at: "2026-05-17T01:30:00.000Z",
        open: 320,
        high: 320.8,
        low: 319.9,
        close: 320.5,
        volume: 18000,
      },
      {
        at: "2026-05-17T01:31:00.000Z",
        open: 320.7,
        high: 321.1,
        low: 320.6,
        close: 321,
        volume: 21000,
      },
    ]);

    const Host = defineComponent({
      components: { KlineChart },
      setup() {
        provideThemeStore();
        return { candles };
      },
      template:
        '<KlineChart :candles="candles" :min-height="320" show-indicator-selector indicator-storage-key="indicator-panes" />',
    });

    const wrapper = mount(Host, { attachTo: document.body });
    await nextTick();
    await nextTick();

    expect(chartMocks.macdDiffSetData).toHaveBeenCalled();
    expect(chartMocks.macdDeaSetData).toHaveBeenCalled();
    expect(chartMocks.kdjKSetData).toHaveBeenCalled();

    const chart = chartMocks.createChart.mock.results.at(-1)?.value;
    expect(chart?.panes()).toHaveLength(4);

    wrapper.unmount();
    expect(chart?.remove).toHaveBeenCalledOnce();
  });

  it("shows initialization errors when ResizeObserver is missing or chart creation fails", async () => {
    vi.stubGlobal("ResizeObserver", undefined as unknown as typeof ResizeObserver);

    const candles = ref([
      {
        at: "2026-05-17T01:30:00.000Z",
        open: 320,
        high: 320.8,
        low: 319.9,
        close: 320.5,
        volume: 18000,
      },
    ]);

    const MissingResizeHost = defineComponent({
      components: { KlineChart },
      setup() {
        provideThemeStore();
        return { candles };
      },
      template: '<KlineChart :candles="candles" :min-height="320" />',
    });

    const missingResizeWrapper = mount(MissingResizeHost);
    await nextTick();
    await nextTick();
    expect(missingResizeWrapper.text()).toContain(
      "K-line chart requires browser ResizeObserver support.",
    );

    vi.stubGlobal("ResizeObserver", MockResizeObserver);
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(1);
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      width: 640,
      height: 320,
      top: 0,
      right: 640,
      bottom: 320,
      left: 0,
      toJSON: () => ({}),
    });
    chartMocks.createChart.mockImplementationOnce(() => {
      throw new Error("chart unavailable");
    });

    const FailingChartHost = defineComponent({
      components: { KlineChart },
      setup() {
        provideThemeStore();
        return { candles };
      },
      template: '<KlineChart :candles="candles" :min-height="320" />',
    });

    const failingWrapper = mount(FailingChartHost);
    await nextTick();
    await nextTick();
    expect(failingWrapper.text()).toContain("chart unavailable");
  });

  it("closes the indicator panel via Escape and debounces load-more events", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("ResizeObserver", MockResizeObserver);
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(1);
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      width: 640,
      height: 320,
      top: 0,
      right: 640,
      bottom: 320,
      left: 0,
      toJSON: () => ({}),
    });

    const candles = ref([
      {
        at: "2026-05-17T01:30:00.000Z",
        open: 320,
        high: 320.8,
        low: 319.9,
        close: 320.5,
        volume: 18000,
      },
      {
        at: "2026-05-17T01:31:00.000Z",
        open: 320.7,
        high: 321.1,
        low: 320.6,
        close: 321,
        volume: 21000,
      },
    ]);

    const Host = defineComponent({
      components: { KlineChart },
      setup() {
        provideThemeStore();
        return { candles };
      },
      template:
        '<KlineChart :candles="candles" :min-height="320" show-indicator-selector @load-more="$emit(\'load-more\')" />',
    });

    const wrapper = mount(Host, {
      attachTo: document.body,
      global: {
        stubs: {
          teleport: true,
        },
      },
    });
    await nextTick();
    await nextTick();

    await wrapper.get("button.kline-chart-trigger").trigger("click");
    expect(wrapper.find(".kline-chart-panel").exists()).toBe(true);

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await nextTick();
    expect(wrapper.find(".kline-chart-panel").exists()).toBe(false);

    chartMocks.barsInLogicalRange.mockReturnValue({ barsBefore: 0 });
    chartMocks.triggerVisibleLogicalRange({ from: 0, to: 2 });
    chartMocks.triggerVisibleLogicalRange({ from: -1, to: 2 });
    expect(wrapper.emitted("load-more")).toHaveLength(1);

    await vi.advanceTimersByTimeAsync(1000);
    chartMocks.triggerVisibleLogicalRange({ from: -2, to: 2 });
    expect(wrapper.emitted("load-more")).toHaveLength(2);
  });
});
