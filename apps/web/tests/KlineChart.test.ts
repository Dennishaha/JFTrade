// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick, ref } from "vue";

import KlineChart from "../src/components/KlineChart.vue";
import { provideThemeStore } from "../src/composables/useTheme";

const chartMocks = vi.hoisted(() => {
  const candlestickSetData = vi.fn();
  const histogramSetData = vi.fn();
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
  const createChart = vi.fn(() => ({
    addCandlestickSeries: vi.fn(() => ({
      setData: candlestickSetData,
      applyOptions: vi.fn(),
      barsInLogicalRange,
    })),
    addHistogramSeries: vi.fn(() => ({
      setData: histogramSetData,
      priceScale: vi.fn(() => ({
        applyOptions: vi.fn(),
      })),
    })),
    applyOptions: vi.fn(),
    resize,
    remove: vi.fn(),
    timeScale: vi.fn(() => ({
      fitContent,
      getVisibleLogicalRange,
      setVisibleLogicalRange,
      subscribeVisibleLogicalRangeChange,
    })),
  }));

  return {
    barsInLogicalRange,
    candlestickSetData,
    histogramSetData,
    resize,
    fitContent,
    getVisibleLogicalRange,
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
  chartMocks.barsInLogicalRange.mockClear();
  chartMocks.candlestickSetData.mockClear();
  chartMocks.histogramSetData.mockClear();
  chartMocks.resize.mockClear();
  chartMocks.fitContent.mockClear();
  chartMocks.getVisibleLogicalRange.mockClear();
  chartMocks.setVisibleLogicalRange.mockClear();
  chartMocks.subscribeVisibleLogicalRangeChange.mockClear();
  chartMocks.createChart.mockClear();
});

describe("KlineChart", () => {
  it("renders finite candles into lightweight-charts and resizes the chart host", async () => {
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
        at: "2026-05-17T01:31:00.000Z",
        open: 320.7,
        high: 321.1,
        low: 320.6,
        close: 321,
        volume: 21000,
      },
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
        time: 1778981400,
        open: 320,
        high: 320.8,
        low: 319.9,
        close: 320.5,
      },
      {
        time: 1778981460,
        open: 320.7,
        high: 321.1,
        low: 320.6,
        close: 321,
      },
    ]);
    expect(chartMocks.histogramSetData).toHaveBeenLastCalledWith([
      expect.objectContaining({ time: 1778981400, value: 18000 }),
      expect.objectContaining({ time: 1778981460, value: 21000 }),
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
});
