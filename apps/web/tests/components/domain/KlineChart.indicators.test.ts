// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick, ref } from "vue";

import KlineChart from "@/components/domain/market-data/KlineChart.vue";
import type { KlineIndicatorKey } from "../../../src/charting/kline";
import { lightweightChartsKlineFactory } from "../../../src/charting/lightweightChartsKline";
import { provideUIColorPreferencesStore } from "@/composables/settings/useUIColorPreferences";
import { provideThemeStore } from "@/composables/settings/useTheme";

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
  const chartApplyOptions = vi.fn();
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
      applyOptions: chartApplyOptions,
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
    chartApplyOptions,
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
  chartMocks.chartApplyOptions.mockClear();
  chartMocks.resize.mockClear();
  chartMocks.fitContent.mockClear();
  chartMocks.getVisibleLogicalRange.mockClear();
  chartMocks.setVisibleLogicalRange.mockClear();
  chartMocks.subscribeVisibleLogicalRangeChange.mockClear();
  chartMocks.createChart.mockClear();
});

describe("KlineChart", () => {
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
    await wrapper.get("button.kline-indicator-selector__trigger").trigger("click");
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

    await wrapper.get("button.kline-indicator-selector__trigger").trigger("click");
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

  it("renders controlled indicators and updates pane height from the prop", async () => {
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
        volume: 18_000,
      },
    ]);
    const indicators = ref<KlineIndicatorKey[]>(["macd"]);
    const Host = defineComponent({
      components: { KlineChart },
      setup() {
        provideThemeStore();
        return { candles, indicators };
      },
      template:
        '<KlineChart :candles="candles" :min-height="320" :indicators="indicators" show-indicator-selector />',
    });

    const wrapper = mount(Host, { attachTo: document.body });
    await nextTick();
    await nextTick();
    const shell = wrapper.get(".kline-chart-shell");
    expect(shell.attributes("style") ?? "").toContain("--kline-min-h: 440px");

    await wrapper.get(".kline-indicator-selector__trigger").trigger("click");
    document.body
      .querySelector<HTMLInputElement>('input[value="ma5"]')
      ?.dispatchEvent(new Event("change", { bubbles: true }));
    await nextTick();
    expect(
      wrapper.getComponent(KlineChart).emitted("update:indicators")?.at(-1),
    ).toEqual([["macd", "ma5"]]);

    indicators.value = ["ma5"];
    await nextTick();
    await nextTick();

    expect(shell.attributes("style") ?? "").toContain("--kline-min-h: 320px");
    expect(chartMocks.overlayLineSetDataByTitle.MA5).toHaveBeenCalled();
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

    await wrapper.get("button.kline-indicator-selector__trigger").trigger("click");
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

  it("recovers from a transient zero-sized layout without reloading candle data", async () => {
    vi.useFakeTimers();
    class PassiveResizeObserver {
      observe(): void {}
      disconnect(): void {}
    }
    vi.stubGlobal("ResizeObserver", PassiveResizeObserver);

    let width = 640;
    let height = 320;
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
      () =>
        ({
          x: 0,
          y: 0,
          width,
          height,
          top: 0,
          right: width,
          bottom: height,
          left: 0,
          toJSON: () => ({}),
        }) as DOMRect,
    );

    let nextFrameId = 0;
    const pendingFrames = new Map<number, FrameRequestCallback>();
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      nextFrameId += 1;
      pendingFrames.set(nextFrameId, callback);
      return nextFrameId;
    });
    vi.stubGlobal("cancelAnimationFrame", (frameId: number) => {
      pendingFrames.delete(frameId);
    });
    const flushAnimationFrames = () => {
      const callbacks = [...pendingFrames.values()];
      pendingFrames.clear();
      for (const callback of callbacks) {
        callback(performance.now());
      }
    };

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
    const Host = defineComponent({
      components: { KlineChart },
      setup() {
        provideThemeStore();
        return { candles };
      },
      template: '<KlineChart :candles="candles" :min-height="320" />',
    });

    const wrapper = mount(Host);
    await nextTick();
    await nextTick();
    flushAnimationFrames();
    expect(chartMocks.resize).toHaveBeenLastCalledWith(640, 320, true);

    chartMocks.resize.mockClear();
    chartMocks.candlestickSetData.mockClear();
    width = 0;
    height = 0;
    window.dispatchEvent(new Event("resize"));
    flushAnimationFrames();

    expect(chartMocks.resize).not.toHaveBeenCalled();
    expect(chartMocks.candlestickSetData).not.toHaveBeenCalled();

    width = 480;
    height = 380;
    vi.advanceTimersByTime(80);
    flushAnimationFrames();

    expect(chartMocks.resize).toHaveBeenLastCalledWith(480, 380, true);
    expect(chartMocks.candlestickSetData).not.toHaveBeenCalled();

    wrapper.unmount();
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

    await wrapper.get("button.kline-indicator-selector__trigger").trigger("click");
    expect(wrapper.find(".kline-indicator-selector__panel").exists()).toBe(true);

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await nextTick();
    expect(wrapper.find(".kline-indicator-selector__panel").exists()).toBe(false);

    chartMocks.barsInLogicalRange.mockReturnValue({ barsBefore: 0 });
    chartMocks.triggerVisibleLogicalRange({ from: 0, to: 2 });
    chartMocks.triggerVisibleLogicalRange({ from: -1, to: 2 });
    expect(wrapper.emitted("load-more")).toHaveLength(1);

    await vi.advanceTimersByTimeAsync(1000);
    chartMocks.triggerVisibleLogicalRange({ from: -2, to: 2 });
    expect(wrapper.emitted("load-more")).toHaveLength(2);
  });
});
