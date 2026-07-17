// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import MarketFeedStatus from "../src/components/domain/market-data/MarketFeedStatus.vue";

afterEach(() => {
  vi.useRealTimers();
});

describe("MarketFeedStatus", () => {
  it("uses one vocabulary for freshness, connection, transport, and session", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-04T00:00:10Z"));
    const wrapper = mount(MarketFeedStatus, {
      props: {
        connectionState: "connected",
        observedAt: "2026-07-04T00:00:05Z",
        transportMode: "push-stream",
        session: "pre",
        source: "bbgo:futu",
        providerName: "Futu OpenD",
        providerCapabilities: "推送报价 / 历史K线 / 盘口",
      },
    });

    expect(wrapper.get("[data-state='live']").text()).toContain("新鲜 5秒");
    expect(wrapper.text()).toContain("流已连");
    expect(wrapper.text()).toContain("推送");
    expect(wrapper.text()).toContain("Futu OpenD");
    expect(wrapper.text()).toContain("盘前");
    expect(wrapper.attributes("title")).toContain("来源：bbgo:futu");
    expect(wrapper.attributes("title")).toContain("能力：推送报价 / 历史K线 / 盘口");

    await wrapper.setProps({
      connectionState: "disconnected",
      observedAt: "2026-07-03T23:58:00Z",
      comparisonObservedAt: "2026-07-03T23:59:00Z",
      comparisonLabel: "快照",
      transportMode: "snapshot-poll-fallback",
      fromCache: false,
    });

    expect(wrapper.get("[data-state='stale']").text()).toContain("陈旧 2分");
    expect(wrapper.text()).toContain("流中断");
    expect(wrapper.text()).toContain("轮询回退");
    expect(wrapper.text()).toContain("快照差 1分");
    wrapper.unmount();
  });

  it("shows explicit loading, empty, and error states", async () => {
    const wrapper = mount(MarketFeedStatus, {
      props: { connectionState: "connecting", loading: true },
    });
    expect(wrapper.get("[data-state='loading']").text()).toContain("加载中");

    await wrapper.setProps({ loading: false, emptyLabel: "暂无深度数据" });
    expect(wrapper.get("[data-state='empty']").text()).toContain("暂无深度数据");

    await wrapper.setProps({ error: "网络断开" });
    expect(wrapper.get("[data-state='error']").text()).toContain("网络断开");
    wrapper.unmount();
  });

  it("keeps degraded transport vocabulary explicit for unavailable push feeds", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-04T00:00:00Z"));
    const unsupported = mount(MarketFeedStatus, {
      props: {
        connectionState: "unsupported",
        observedAt: "2026-07-04T00:00:00Z",
        transportMode: "idle",
      },
    });

    expect(unsupported.get("[data-state='live']").text()).toContain("新鲜 刚刚");
    expect(unsupported.text()).toContain("无推送");
    expect(unsupported.text()).toContain("空闲");

    await unsupported.setProps({
      connectionState: "error",
      transportMode: "unrecognized-transport",
    });
    expect(unsupported.text()).toContain("流异常");
    expect(unsupported.text()).toContain("快照");
    unsupported.unmount();
  });
});
