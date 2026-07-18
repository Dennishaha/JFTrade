// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import MarketFeedStatus from "../src/components/domain/market-data/MarketFeedStatus.vue";

afterEach(() => {
  vi.useRealTimers();
});

describe("MarketFeedStatus", () => {
  it("does not occupy the header while the feed is healthy, loading, or connecting", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-04T00:00:10Z"));
    const wrapper = mount(MarketFeedStatus, {
      props: {
        connectionState: "connected",
        observedAt: "2026-07-04T00:00:05Z",
        transportMode: "push-stream",
        source: "bbgo:futu",
      },
    });

    expect(wrapper.find(".market-feed-issue-badge").exists()).toBe(false);

    await wrapper.setProps({
      connectionState: "connected",
      observedAt: null,
      loading: true,
    });
    expect(wrapper.find(".market-feed-issue-badge").exists()).toBe(false);

    await wrapper.setProps({ connectionState: "connecting", loading: false });
    expect(wrapper.find(".market-feed-issue-badge").exists()).toBe(false);
    wrapper.unmount();
  });

  it("shows one compact issue with detailed error, source, and update context", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-04T00:00:10Z"));
    const wrapper = mount(MarketFeedStatus, {
      props: {
        connectionState: "connected",
        observedAt: "2026-07-04T00:00:05Z",
        transportMode: "push-stream",
        source: "bbgo:futu",
        error: "网络断开",
      },
    });

    const issue = wrapper.get(".market-feed-issue-badge");
    expect(issue.text()).toContain("行情异常");
    expect(issue.attributes("data-issue")).toBe("error");
    expect(issue.attributes("title")).toContain("网络断开");
    expect(issue.attributes("title")).toContain("来源：bbgo:futu");
    expect(issue.attributes("title")).toContain(
      "更新时间：2026-07-04T00:00:05Z",
    );
    wrapper.unmount();
  });

  it("covers stale, cache, degraded, unavailable, and empty feed problems", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-04T00:02:00Z"));
    const wrapper = mount(MarketFeedStatus, {
      props: {
        connectionState: "connected",
        observedAt: "2026-07-04T00:00:00Z",
        transportMode: "push-stream",
      },
    });

    expect(wrapper.get(".market-feed-issue-badge").text()).toContain("数据陈旧");
    expect(wrapper.get(".market-feed-issue-badge").attributes("data-issue")).toBe("stale");

    await wrapper.setProps({
      observedAt: "2026-07-04T00:01:59Z",
      fromCache: true,
    });
    expect(wrapper.get(".market-feed-issue-badge").text()).toContain("缓存行情");
    expect(wrapper.get(".market-feed-issue-badge").attributes("data-issue")).toBe("cache");

    await wrapper.setProps({
      fromCache: false,
      transportMode: "snapshot-poll-fallback",
    });
    expect(wrapper.get(".market-feed-issue-badge").text()).toContain("行情已降级");
    expect(wrapper.get(".market-feed-issue-badge").attributes("data-issue")).toBe("degraded");
    expect(wrapper.get(".market-feed-issue-badge").attributes("title")).toContain(
      "已降级到轮询行情",
    );

    await wrapper.setProps({
      connectionState: "disconnected",
      observedAt: null,
      transportMode: "idle",
    });
    expect(wrapper.get(".market-feed-issue-badge").text()).toContain("行情不可用");
    expect(wrapper.get(".market-feed-issue-badge").attributes("data-issue")).toBe("unavailable");

    await wrapper.setProps({ connectionState: "connected" });
    expect(wrapper.get(".market-feed-issue-badge").text()).toContain("暂无行情数据");
    expect(wrapper.get(".market-feed-issue-badge").attributes("data-issue")).toBe("empty");
    wrapper.unmount();
  });

  it("describes unsupported feeds and second-level staleness", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-04T00:00:40Z"));
    const wrapper = mount(MarketFeedStatus, {
      props: {
        connectionState: "unsupported",
        observedAt: "2026-07-04T00:00:39Z",
        transportMode: "push-stream",
      },
    });

    expect(wrapper.get(".market-feed-issue-badge").attributes("title")).toContain(
      "不支持推送，使用快照行情",
    );

    await wrapper.setProps({
      connectionState: "connected",
      observedAt: "2026-07-04T00:00:09Z",
    });
    expect(wrapper.get(".market-feed-issue-badge").attributes("title")).toContain(
      "31秒 未更新",
    );
    wrapper.unmount();
  });

  it("keeps an empty diagnostic title when no issue is present", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-04T00:00:10Z"));
    const wrapper = mount(MarketFeedStatus, {
      props: {
        connectionState: "connected",
        observedAt: "2026-07-04T00:00:09Z",
        transportMode: "push-stream",
      },
    });

    expect((wrapper.vm as unknown as { issueTitle: string }).issueTitle).toBe("");
    wrapper.unmount();
  });
});
