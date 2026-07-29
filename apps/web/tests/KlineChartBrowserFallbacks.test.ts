// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick } from "vue";

import KlineChart from "@/components/domain/market-data/KlineChart.vue";
import { provideThemeStore } from "@/composables/settings/useTheme";

vi.mock("lightweight-charts", () => ({
  CandlestickSeries: { type: "Candlestick" },
  ColorType: { Solid: "solid" },
  CrosshairMode: { Normal: 0 },
  HistogramSeries: { type: "Histogram" },
  LineStyle: { Solid: 0, Dashed: 1, Dotted: 2 },
  LineSeries: { type: "Line" },
  TickMarkType: { Year: 0, Month: 1, DayOfMonth: 2, Time: 3, TimeWithSeconds: 4 },
  createChart: vi.fn(),
}));

afterEach(() => {
  vi.unstubAllGlobals();
  window.localStorage.clear();
  document.body.innerHTML = "";
});

describe("KlineChart browser fallbacks", () => {
  it("preserves indicator controls and storage validation when the chart runtime is unavailable", async () => {
    vi.stubGlobal("ResizeObserver", undefined);
    window.localStorage.setItem("invalid-indicators", "{not-json");

    const Host = defineComponent({
      components: { KlineChart },
      setup() {
        provideThemeStore();
      },
      template: `
        <KlineChart
          :candles="[]"
          show-indicator-selector
          indicator-storage-key="invalid-indicators"
        />
      `,
    });
    const wrapper = mount(Host, { attachTo: document.body });
    await nextTick();
    await nextTick();

    expect(wrapper.text()).toContain("K-line chart requires browser ResizeObserver support.");
    const trigger = wrapper.get(".kline-indicator-selector__trigger");
    Object.defineProperty(trigger.element, "getBoundingClientRect", {
      configurable: true,
      value: () => ({
        x: 0,
        y: 0,
        width: 64,
        height: 32,
        top: 0,
        right: 64,
        bottom: 32,
        left: 0,
        toJSON: () => ({}),
      }),
    });
    await trigger.trigger("click");
    expect(
      document.body.querySelector(".kline-indicator-selector__panel"),
    ).not.toBeNull();

    const volume = document.body.querySelector(
      'input[value="volume"]',
    ) as HTMLInputElement | null;
    expect(volume).not.toBeNull();
    volume?.dispatchEvent(new Event("change", { bubbles: true }));
    await nextTick();
    expect(window.localStorage.getItem("invalid-indicators")).toBe('["volume"]');
    volume?.dispatchEvent(new Event("change", { bubbles: true }));
    await nextTick();
    expect(window.localStorage.getItem("invalid-indicators")).toBe('["volume"]');

    document.body.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
    await nextTick();
    expect(
      document.body.querySelector(".kline-indicator-selector__panel"),
    ).toBeNull();
  });

  it("ignores legacy non-array indicator preferences instead of treating arbitrary JSON as a chart configuration", async () => {
    vi.stubGlobal("ResizeObserver", undefined);
    window.localStorage.setItem("legacy-indicators", JSON.stringify({ selected: ["rsi"] }));

    const Host = defineComponent({
      components: { KlineChart },
      setup() {
        provideThemeStore();
      },
      template: '<KlineChart :candles="[]" show-indicator-selector indicator-storage-key="legacy-indicators" />',
    });
    const wrapper = mount(Host, { attachTo: document.body });
    await nextTick();
    await nextTick();

    expect(wrapper.text()).toContain("K-line chart requires browser ResizeObserver support.");
    const trigger = wrapper.get(".kline-indicator-selector__trigger");
    Object.defineProperty(trigger.element, "getBoundingClientRect", {
      configurable: true,
      value: () => ({
        x: 0,
        y: 0,
        width: 64,
        height: 32,
        top: 0,
        right: 64,
        bottom: 32,
        left: 0,
        toJSON: () => ({}),
      }),
    });
    await trigger.trigger("click");
    expect(document.body.querySelector('input[value="volume"]')).not.toBeNull();
  });
});
