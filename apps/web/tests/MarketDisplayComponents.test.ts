// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { nextTick } from "vue";
import { describe, expect, it, vi } from "vitest";

import InstrumentIdentity from "../src/components/domain/market-data/InstrumentIdentity.vue";
import OrderBookDepthTable from "../src/components/domain/market-data/OrderBookDepthTable.vue";
import RuntimeHealthBadge from "../src/components/domain/runtime/RuntimeHealthBadge.vue";

describe("market and runtime display components", () => {
  it("maps runtime states to user-visible health tones and labels", () => {
    const healthy = mount(RuntimeHealthBadge, { props: { status: " connected " } });
    expect(healthy.attributes("data-status")).toBe("CONNECTED");
    expect(healthy.attributes("data-tone")).toBe("healthy");
    expect(healthy.text()).toBe("CONNECTED");

    const paused = mount(RuntimeHealthBadge, { props: { status: "PAUSED" } });
    expect(paused.attributes("data-tone")).toBe("warning");
    expect(paused.text()).toBe("已暂停");

    const failed = mount(RuntimeHealthBadge, { props: { status: "FAILED", label: "回测失败" } });
    expect(failed.attributes("data-tone")).toBe("error");
    expect(failed.text()).toBe("回测失败");

    const disabled = mount(RuntimeHealthBadge, { props: { status: "RUNNING", disabled: true } });
    expect(disabled.attributes("data-tone")).toBe("disabled");
    expect(disabled.attributes("aria-disabled")).toBe("true");

    const stopped = mount(RuntimeHealthBadge, { props: { status: "STOPPED" } });
    expect(stopped.text()).toBe("已停止");
  });

  it("renders canonical identities and copies the selected instrument ID", async () => {
    const wrapper = mount(InstrumentIdentity, {
      props: {
        market: "US",
        code: "AAPL",
        instrumentId: "US.AAPL",
        name: "Apple",
      },
    });
    expect(wrapper.text()).toContain("AAPL");
    expect(wrapper.text()).toContain("Apple");
    expect(wrapper.attributes("data-market")).toBe("US");
    expect(wrapper.attributes("data-category-market")).toBe("US");
    expect(wrapper.attributes("data-copy-value")).toBe("US.AAPL");

    const copied = vi.fn();
    const event = new Event("copy", { cancelable: true });
    Object.defineProperty(event, "clipboardData", { value: { setData: copied } });
    wrapper.element.dispatchEvent(event);
    await nextTick();
    expect(copied).toHaveBeenCalledWith("text/plain", "US.AAPL");
    expect(event.defaultPrevented).toBe(true);

    const unavailableClipboard = new Event("copy", { cancelable: true });
    wrapper.element.dispatchEvent(unavailableClipboard);
    expect(unavailableClipboard.defaultPrevented).toBe(false);

    const stacked = mount(InstrumentIdentity, {
      props: { market: "HK", code: "00700", layout: "stacked", name: "腾讯控股" },
    });
    expect(stacked.classes()).toContain("instrument-identity--stacked");
    expect(stacked.text()).toContain("港股");
    expect(stacked.text()).toContain("腾讯控股");

    const empty = mount(InstrumentIdentity, { props: { layout: "stacked" } });
    expect(empty.text()).toContain("未设置");
  });

  it("formats depth rows and distinguishes loading, failure, disabled, and empty states", () => {
    const normal = mount(OrderBookDepthTable, {
      props: {
        market: "US",
        levels: [
          { bidPrice: 0.12345, askPrice: 12.3456, bidSize: 1_500, askSize: 2_000_000 },
          { bidPrice: null, askPrice: null, bidSize: 3, askSize: 4 },
        ],
      },
    });
    expect(normal.attributes("data-state")).toBe("normal");
    expect(normal.text()).toContain("0.12");
    expect(normal.text()).toContain("12.35");
    expect(normal.text()).toContain("1.5K");
    expect(normal.text()).toContain("2.00M");
    expect(normal.text()).toContain("—");
    expect(normal.find(".tv-ob-depth-bar").attributes("style")).toContain("width: 100%");

    const loading = mount(OrderBookDepthTable, { props: { levels: [], loading: true } });
    expect(loading.attributes("data-state")).toBe("loading");
    const failure = mount(OrderBookDepthTable, { props: { levels: [], loading: true, error: "OpenD unavailable" } });
    expect(failure.attributes("data-state")).toBe("error");
    expect(failure.text()).toContain("OpenD unavailable");
    const disabled = mount(OrderBookDepthTable, { props: { levels: [], disabled: true } });
    expect(disabled.attributes("data-state")).toBe("disabled");
    const empty = mount(OrderBookDepthTable, { props: { levels: [] } });
    expect(empty.attributes("data-state")).toBe("empty");
  });
});
