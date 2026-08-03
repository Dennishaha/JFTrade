// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick, ref } from "vue";

import KlineIndicatorSelector from "@/components/domain/market-data/KlineIndicatorSelector.vue";
import type { KlineIndicatorKey } from "../../../src/charting/kline";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  window.localStorage.clear();
  document.body.innerHTML = "";
});

function mountSelector(storageKey = "chart-indicators") {
  const indicators = ref<KlineIndicatorKey[]>(["volume"]);
  const Host = defineComponent({
    components: { KlineIndicatorSelector },
    setup() {
      return { indicators, storageKey };
    },
    template: `
      <KlineIndicatorSelector
        v-model="indicators"
        :storage-key="storageKey"
      />
    `,
  });
  return { indicators, wrapper: mount(Host, { attachTo: document.body }) };
}

describe("KlineIndicatorSelector", () => {
  it("renders compact grouped controls and keeps rapid selections", async () => {
    const { indicators, wrapper } = mountSelector();
    await nextTick();

    const trigger = wrapper.get(".kline-indicator-selector__trigger");
    expect(trigger.text()).toContain("指标");
    expect(trigger.text()).toContain("1");
    await trigger.trigger("click");

    const panel = document.body.querySelector(
      ".kline-indicator-selector__panel",
    );
    expect(panel).not.toBeNull();
    expect(panel?.querySelectorAll(".kline-indicator-selector__group")).toHaveLength(3);
    expect(panel?.textContent).toContain("MA");
    expect(panel?.textContent).toContain("EMA");
    expect(panel?.textContent).toContain("副图");
    expect(panel?.textContent).not.toContain("勾选后立即叠加");

    const ma5 = panel?.querySelector('input[value="ma5"]');
    const ema5 = panel?.querySelector('input[value="ema5"]');
    ma5?.dispatchEvent(new Event("change", { bubbles: true }));
    ema5?.dispatchEvent(new Event("change", { bubbles: true }));
    await nextTick();

    expect(indicators.value).toEqual(["volume", "ma5", "ema5"]);
    expect(window.localStorage.getItem("chart-indicators")).toBe(
      '["volume","ma5","ema5"]',
    );
    expect(trigger.text()).toContain("3");
    wrapper.unmount();
  });

  it("restores preferences, clamps the popup, and closes via Escape or outside click", async () => {
    window.localStorage.setItem(
      "stored-indicators",
      JSON.stringify(["ma10", "macd"]),
    );
    const { indicators, wrapper } = mountSelector("stored-indicators");
    await nextTick();
    expect(indicators.value).toEqual(["macd", "ma10"]);

    Object.defineProperty(window, "innerWidth", {
      configurable: true,
      value: 320,
    });
    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 240,
    });
    const trigger = wrapper.get(".kline-indicator-selector__trigger");
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
      function () {
        if (this.classList.contains("kline-indicator-selector__panel")) {
          return {
            x: 0,
            y: 0,
            width: 304,
            height: 210,
            top: 0,
            right: 304,
            bottom: 210,
            left: 0,
            toJSON: () => ({}),
          };
        }
        return {
          x: 280,
          y: 205,
          width: 40,
          height: 26,
          top: 205,
          right: 320,
          bottom: 231,
          left: 280,
          toJSON: () => ({}),
        };
      },
    );
    await trigger.trigger("click");

    const panel = document.body.querySelector(
      ".kline-indicator-selector__panel",
    ) as HTMLElement | null;
    expect(panel).not.toBeNull();
    expect(panel?.style.left).toBe("8px");
    expect(panel?.style.top).toBe("22px");

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await nextTick();
    expect(
      document.body.querySelector(".kline-indicator-selector__panel"),
    ).toBeNull();

    await trigger.trigger("click");
    document.body.dispatchEvent(
      new PointerEvent("pointerdown", { bubbles: true }),
    );
    await nextTick();
    expect(
      document.body.querySelector(".kline-indicator-selector__panel"),
    ).toBeNull();
    wrapper.unmount();
  });

  it("keeps working when browser storage is unavailable", async () => {
    const getItem = vi.fn(() => {
      throw new DOMException("storage blocked");
    });
    const setItem = vi.fn(() => {
      throw new DOMException("storage blocked");
    });
    vi.stubGlobal("localStorage", {
      getItem,
      setItem,
      removeItem: vi.fn(),
      clear: vi.fn(),
      key: vi.fn(),
      length: 0,
    });
    const { indicators, wrapper } = mountSelector("blocked-storage");
    await nextTick();

    await wrapper.get(".kline-indicator-selector__trigger").trigger("click");
    document.body
      .querySelector<HTMLInputElement>('input[value="ma5"]')
      ?.dispatchEvent(new Event("change", { bubbles: true }));
    await nextTick();

    expect(indicators.value).toEqual(["volume", "ma5"]);
    expect(getItem).toHaveBeenCalled();
    expect(setItem).toHaveBeenCalled();
    wrapper.unmount();
  });

  it("honors the disabled state and keeps the controlled value without storage", async () => {
    const wrapper = mount(KlineIndicatorSelector, {
      attachTo: document.body,
      props: {
        modelValue: ["macd"],
        defaultIndicators: ["volume"],
        disabled: true,
      },
    });
    await nextTick();

    const trigger = wrapper.get(".kline-indicator-selector__trigger");
    expect(trigger.text()).toContain("1");
    expect(wrapper.emitted("update:modelValue")).toBeUndefined();
    await trigger.trigger("click");
    expect(
      document.body.querySelector(".kline-indicator-selector__panel"),
    ).toBeNull();

    await wrapper.setProps({ disabled: false, modelValue: ["macd", "ma5"] });
    expect(trigger.text()).toContain("2");
    await wrapper.setProps({ modelValue: ["macd", "ma5"] });
    wrapper.unmount();
  });

  it("positions the panel above or below the trigger and ignores inside events", async () => {
    Object.defineProperty(window, "innerWidth", {
      configurable: true,
      value: 600,
    });
    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 420,
    });
    let triggerTop = 20;
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
      function () {
        if (this.classList.contains("kline-indicator-selector__panel")) {
          return {
            x: 0,
            y: 0,
            width: 300,
            height: 100,
            top: 0,
            right: 300,
            bottom: 100,
            left: 0,
            toJSON: () => ({}),
          };
        }
        return {
          x: 24,
          y: triggerTop,
          width: 64,
          height: 26,
          top: triggerTop,
          right: 88,
          bottom: triggerTop + 26,
          left: 24,
          toJSON: () => ({}),
        };
      },
    );

    const { wrapper } = mountSelector("");
    await nextTick();
    const trigger = wrapper.get(".kline-indicator-selector__trigger");
    await trigger.trigger("click");

    let panel = document.body.querySelector(
      ".kline-indicator-selector__panel",
    ) as HTMLElement;
    expect(panel.style.left).toBe("24px");
    expect(panel.style.top).toBe("50px");

    trigger.element.dispatchEvent(
      new PointerEvent("pointerdown", { bubbles: true }),
    );
    panel.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));
    await nextTick();
    expect(
      document.body.querySelector(".kline-indicator-selector__panel"),
    ).not.toBeNull();

    triggerTop = 360;
    window.dispatchEvent(new Event("resize"));
    await nextTick();
    panel = document.body.querySelector(
      ".kline-indicator-selector__panel",
    ) as HTMLElement;
    expect(panel.style.top).toBe("256px");

    await trigger.trigger("click");
    expect(
      document.body.querySelector(".kline-indicator-selector__panel"),
    ).toBeNull();
    wrapper.unmount();
  });
});
