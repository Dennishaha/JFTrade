// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick, ref } from "vue";

import KlineChart from "../src/components/KlineChart.vue";
import { provideThemeStore } from "../src/composables/useTheme";

const chartMocks = vi.hoisted(() => {
  const candlestickSetData = vi.fn();
  const volumeSetData = vi.fn();
  const macdHistogramSetData = vi.fn();
  const macdDiffSetData = vi.fn();
  const macdDeaSetData = vi.fn();
  const kdjKSetData = vi.fn();
  const kdjDSetData = vi.fn();
  const kdjJSetData = vi.fn();
  const resize = vi.fn();
  const fitContent = vi.fn();
  const setVisibleLogicalRange = vi.fn();
  const getVisibleLogicalRange = vi.fn(() => ({ from: 2, to: 3 }));
  const barsInLogicalRange = vi.fn(() => ({ barsBefore: 20 }));
  const candlestickApplyOptions = vi.fn();
  const volumeApplyOptions = vi.fn();
  const macdHistogramApplyOptions = vi.fn();
  const macdDiffApplyOptions = vi.fn();
  const macdDeaApplyOptions = vi.fn();
  const kdjKApplyOptions = vi.fn();
  const kdjDApplyOptions = vi.fn();
  const kdjJApplyOptions = vi.fn();
  const mainPriceScaleApplyOptions = vi.fn();
  const volumePriceScaleApplyOptions = vi.fn();
  const macdPriceScaleApplyOptions = vi.fn();
  const kdjPriceScaleApplyOptions = vi.fn();
  const rightPriceScaleApplyOptions = vi.fn();
  let visibleLogicalRangeCallback:
    | ((range: { from: number; to: number } | null) => void)
    | null = null;
  const subscribeVisibleLogicalRangeChange = vi.fn(
    (callback: (range: { from: number; to: number } | null) => void) => {
      visibleLogicalRangeCallback = callback;
    },
  );
  const createChart = vi.fn(() => {
    const histogramSeries = [
      {
        setData: volumeSetData,
        applyOptions: volumeApplyOptions,
        priceScale: vi.fn(() => ({
          applyOptions: volumePriceScaleApplyOptions,
        })),
      },
      {
        setData: macdHistogramSetData,
        applyOptions: macdHistogramApplyOptions,
        priceScale: vi.fn(() => ({
          applyOptions: macdPriceScaleApplyOptions,
        })),
      },
    ];
    const lineSeries = [
      {
        setData: macdDiffSetData,
        applyOptions: macdDiffApplyOptions,
        priceScale: vi.fn(() => ({
          applyOptions: macdPriceScaleApplyOptions,
        })),
      },
      {
        setData: macdDeaSetData,
        applyOptions: macdDeaApplyOptions,
        priceScale: vi.fn(() => ({
          applyOptions: macdPriceScaleApplyOptions,
        })),
      },
      {
        setData: kdjKSetData,
        applyOptions: kdjKApplyOptions,
        priceScale: vi.fn(() => ({
          applyOptions: kdjPriceScaleApplyOptions,
        })),
      },
      {
        setData: kdjDSetData,
        applyOptions: kdjDApplyOptions,
        priceScale: vi.fn(() => ({
          applyOptions: kdjPriceScaleApplyOptions,
        })),
      },
      {
        setData: kdjJSetData,
        applyOptions: kdjJApplyOptions,
        priceScale: vi.fn(() => ({
          applyOptions: kdjPriceScaleApplyOptions,
        })),
      },
    ];

    return {
      addCandlestickSeries: vi.fn(() => ({
        setData: candlestickSetData,
        applyOptions: candlestickApplyOptions,
        priceScale: vi.fn(() => ({
          applyOptions: mainPriceScaleApplyOptions,
        })),
        barsInLogicalRange,
      })),
      addHistogramSeries: vi.fn(() => histogramSeries.shift()),
      addLineSeries: vi.fn(() => lineSeries.shift()),
      applyOptions: vi.fn(),
      priceScale: vi.fn((id: string) => ({
        applyOptions:
          id === "volume"
            ? volumePriceScaleApplyOptions
            : id === "macd"
              ? macdPriceScaleApplyOptions
              : id === "kdj"
                ? kdjPriceScaleApplyOptions
                : rightPriceScaleApplyOptions,
      })),
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
    candlestickSetData,
    volumeSetData,
    macdHistogramSetData,
    macdDiffSetData,
    macdDeaSetData,
    kdjKSetData,
    kdjDSetData,
    kdjJSetData,
    resize,
    fitContent,
    getVisibleLogicalRange,
    setVisibleLogicalRange,
    subscribeVisibleLogicalRangeChange,
    candlestickApplyOptions,
    volumeApplyOptions,
    macdHistogramApplyOptions,
    macdDiffApplyOptions,
    macdDeaApplyOptions,
    kdjKApplyOptions,
    kdjDApplyOptions,
    kdjJApplyOptions,
    mainPriceScaleApplyOptions,
    volumePriceScaleApplyOptions,
    macdPriceScaleApplyOptions,
    kdjPriceScaleApplyOptions,
    rightPriceScaleApplyOptions,
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
  chartMocks.volumeSetData.mockClear();
  chartMocks.macdHistogramSetData.mockClear();
  chartMocks.macdDiffSetData.mockClear();
  chartMocks.macdDeaSetData.mockClear();
  chartMocks.kdjKSetData.mockClear();
  chartMocks.kdjDSetData.mockClear();
  chartMocks.kdjJSetData.mockClear();
  chartMocks.resize.mockClear();
  chartMocks.fitContent.mockClear();
  chartMocks.getVisibleLogicalRange.mockClear();
  chartMocks.setVisibleLogicalRange.mockClear();
  chartMocks.subscribeVisibleLogicalRangeChange.mockClear();
  chartMocks.candlestickApplyOptions.mockClear();
  chartMocks.volumeApplyOptions.mockClear();
  chartMocks.macdHistogramApplyOptions.mockClear();
  chartMocks.macdDiffApplyOptions.mockClear();
  chartMocks.macdDeaApplyOptions.mockClear();
  chartMocks.kdjKApplyOptions.mockClear();
  chartMocks.kdjDApplyOptions.mockClear();
  chartMocks.kdjJApplyOptions.mockClear();
  chartMocks.mainPriceScaleApplyOptions.mockClear();
  chartMocks.volumePriceScaleApplyOptions.mockClear();
  chartMocks.macdPriceScaleApplyOptions.mockClear();
  chartMocks.kdjPriceScaleApplyOptions.mockClear();
  chartMocks.rightPriceScaleApplyOptions.mockClear();
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
    expect(chartMocks.volumeSetData).toHaveBeenLastCalledWith([
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

  it("updates overlay indicator scales when selector buttons are toggled", async () => {
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

    const wrapper = mount(Host);
    await nextTick();
    await nextTick();

    const buttons = wrapper.findAll("button");
    await buttons[1]?.trigger("click");
    await nextTick();

    expect(chartMocks.macdHistogramApplyOptions).toHaveBeenLastCalledWith(
      expect.objectContaining({ visible: true }),
    );
    expect(chartMocks.macdDiffApplyOptions).toHaveBeenLastCalledWith(
      expect.objectContaining({ visible: true }),
    );
    expect(chartMocks.macdPriceScaleApplyOptions).toHaveBeenLastCalledWith(
      expect.objectContaining({
        visible: true,
        borderVisible: true,
        scaleMargins: expect.objectContaining({ top: expect.any(Number) }),
      }),
    );

    await buttons[2]?.trigger("click");
    await nextTick();

    expect(chartMocks.kdjKApplyOptions).toHaveBeenLastCalledWith(
      expect.objectContaining({ visible: true }),
    );
    expect(chartMocks.kdjPriceScaleApplyOptions).toHaveBeenLastCalledWith(
      expect.objectContaining({
        visible: true,
        borderVisible: true,
        scaleMargins: expect.objectContaining({ bottom: expect.any(Number) }),
      }),
    );
  });
});
