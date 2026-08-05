// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";

import LightweightChartHeader from "@/components/workspace/LightweightChartHeader.vue";

const mountedWrappers: Array<ReturnType<typeof mount>> = [];

function mountHeader(overrides: Record<string, unknown> = {}) {
  const wrapper = mount(LightweightChartHeader, {
    props: {
      variant: "workspace",
      market: "US",
      periods: [{ value: "1m", label: "1 分钟" }],
      selectedPeriod: "1m",
      loadingCapabilities: false,
      capabilitiesError: "",
      activeChartType: "standard",
      activeChartTypeLabel: "标准 K 线",
      tickPeriod: false,
      connectionState: "connected",
      observedAt: null,
      transportMode: null,
      source: "test",
      providerName: "测试",
      fromCache: false,
      loadingData: false,
      dataError: "",
      indicators: ["volume"],
      candleSessions: ["regular", "extended", "overnight"],
      supportedCandleSessions: ["regular", "extended", "overnight"],
      ...overrides,
    },
    global: {
      stubs: {
        KlineIndicatorSelector: true,
        MarketFeedStatus: true,
        LightweightChartTypeSelector: true,
      },
    },
  });
  mountedWrappers.push(wrapper);
  return wrapper;
}

function sessionOptions(): Array<HTMLInputElement> {
  return Array.from(
    document.querySelectorAll<HTMLInputElement>(
      ".lightweight-chart-session-selector__option input",
    ),
  );
}

function sessionMenu(): Element | null {
  return document.querySelector(".lightweight-chart-session-selector__menu");
}

afterEach(() => {
  for (const wrapper of mountedWrappers.splice(0)) wrapper.unmount();
});

describe("LightweightChartHeader candle sessions", () => {
  it("opens the popup, keeps unsupported options visible, and protects the last selection", async () => {
    const wrapper = mountHeader();
    await wrapper.get(".lightweight-chart-session-selector__trigger").trigger("click");
    const options = sessionOptions();
    expect(options).toHaveLength(3);
    expect(wrapper.get(".lightweight-chart-session-selector__summary").text()).toBe("全天");

    options[0]!.checked = false;
    options[0]!.dispatchEvent(new Event("change", { bubbles: true }));
    expect(wrapper.emitted("update:candle-sessions")?.at(-1)?.[0]).toEqual(["extended", "overnight"]);
    await wrapper.setProps({ candleSessions: ["overnight"] });
    expect(options[2]!.disabled).toBe(true);
  });

  it("disables sessions unavailable for the active provider", async () => {
    const wrapper = mountHeader({
      supportedCandleSessions: ["regular", "extended"],
      candleSessions: ["regular", "extended"],
    });
    await wrapper.get(".lightweight-chart-session-selector__trigger").trigger("click");
    const options = sessionOptions();
    expect(options[2]!.disabled).toBe(true);
    options[1]!.dispatchEvent(new Event("change", { bubbles: true }));
  });

  it("hides the selector for regular-only non-US markets", () => {
    const wrapper = mountHeader({
      market: "SH",
      supportedCandleSessions: ["regular"],
      candleSessions: ["regular"],
    });
    expect(wrapper.find(".lightweight-chart-session-selector").exists()).toBe(false);
  });

  it("keeps the selector visible for US when the adapter only supports regular", () => {
    const wrapper = mountHeader({
      supportedCandleSessions: ["regular"],
      candleSessions: ["regular"],
    });
    expect(wrapper.find(".lightweight-chart-session-selector").exists()).toBe(true);
  });

  it("adds a supported session, closes on outside pointer, and blocks empty-capability menus", async () => {
    const wrapper = mountHeader({
      candleSessions: ["regular"],
      supportedCandleSessions: ["regular", "extended"],
    });
    await wrapper.get(".lightweight-chart-session-selector__trigger").trigger("click");
    const options = sessionOptions();
    options[1]!.checked = true;
    options[1]!.dispatchEvent(new Event("change", { bubbles: true }));
    expect(wrapper.emitted("update:candle-sessions")?.at(-1)?.[0]).toEqual([
      "regular",
      "extended",
    ]);

    document.body.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
    await nextTick();
    expect(sessionMenu()).toBeNull();

    const blocked = mountHeader({
      loadingCapabilities: true,
      supportedCandleSessions: [],
    });
    expect(blocked.find(".lightweight-chart-session-selector").exists()).toBe(false);
    expect(sessionMenu()).toBeNull();

    const unavailable = mountHeader({
      loadingCapabilities: false,
      supportedCandleSessions: [],
    });
    expect(unavailable.find(".lightweight-chart-session-selector").exists()).toBe(false);
    expect(sessionMenu()).toBeNull();

    await wrapper.setProps({ candleSessions: ["regular"] });
    await wrapper.get(".lightweight-chart-session-selector__trigger").trigger("click");
    const singleOption = sessionOptions()[0]!;
    singleOption.checked = false;
    singleOption.dispatchEvent(new Event("change", { bubbles: true }));
    await nextTick();
    expect(wrapper.emitted("update:candle-sessions")?.at(-1)?.[0]).toEqual([
      "regular",
    ]);
  });

  it("keeps inside clicks open, restores focus on escape, and positions the popup", async () => {
    const wrapper = mountHeader();
    const trigger = wrapper.get(
      ".lightweight-chart-session-selector__trigger",
    );
    window.dispatchEvent(new Event("resize"));
    await trigger.trigger("click");
    const menu = sessionMenu() as HTMLElement;
    const triggerElement = trigger.element as HTMLElement;
    Object.defineProperty(triggerElement, "getBoundingClientRect", {
      configurable: true,
      value: () => ({ left: 4, top: 700, bottom: 720 }),
    });
    Object.defineProperty(menu, "getBoundingClientRect", {
      configurable: true,
      value: () => ({ width: 200, height: 100 }),
    });
    triggerElement.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
    menu.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
    window.dispatchEvent(new Event("resize"));
    await nextTick();
    expect(sessionMenu()).not.toBeNull();
    expect(menu.style.top).toBe("596px");
    expect(menu.style.left).toBe("8px");

    const focus = vi.spyOn(triggerElement, "focus");
    document.dispatchEvent(
      new KeyboardEvent("keydown", {
        key: "Escape",
        bubbles: true,
        cancelable: true,
      }),
    );
    await nextTick();
    expect(sessionMenu()).toBeNull();
    expect(focus).toHaveBeenCalledOnce();

    await trigger.trigger("click");
    const closeButton = sessionMenu()?.querySelector<HTMLButtonElement>(
      ".lightweight-chart-session-selector__close",
    );
    closeButton?.click();
    await nextTick();
    expect(sessionMenu()).toBeNull();
    await trigger.trigger("click");
    await trigger.trigger("click");
    expect(sessionMenu()).toBeNull();
  });

  it("renders compact summaries and honors period, retry, and refresh controls", async () => {
    const wrapper = mountHeader({
      candleSessions: ["extended", "overnight"],
      supportedCandleSessions: ["extended", "overnight"],
      capabilitiesError: "能力读取失败",
    });
    expect(
      wrapper
        .get(".lightweight-chart-head__primary-controls")
        .element.firstElementChild?.classList.contains(
          "lightweight-chart-session-selector",
        ),
    ).toBe(true);
    expect(wrapper.get(".lightweight-chart-session-selector__summary").text()).toBe(
      "盘前后+夜盘",
    );
    await wrapper.get(".lightweight-chart-head__capability-retry").trigger("click");
    await wrapper.get(".lightweight-chart-head__refresh").trigger("click");
    await wrapper.get("select[aria-label='K 线周期']").setValue("1m");
    expect(wrapper.emitted("retry")).toHaveLength(1);
    expect(wrapper.emitted("refresh")).toHaveLength(1);
    expect(wrapper.emitted("select-period")?.at(-1)?.[0]).toBe("1m");
  });
});
