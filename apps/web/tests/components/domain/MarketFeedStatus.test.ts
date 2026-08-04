// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import MarketFeedStatus from "../../../src/components/domain/market-data/MarketFeedStatus.vue";

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
    expect(wrapper.get(".market-feed-issue-badge").text()).toContain("推送回退");
    expect(wrapper.get(".market-feed-issue-badge").attributes("data-issue")).toBe("degraded");
    expect(wrapper.get(".market-feed-issue-badge").attributes("title")).toContain(
      "快照轮询（推送回退）",
    );

    await wrapper.setProps({ transportMode: "snapshot-poll-delayed" });
    expect(wrapper.find(".market-feed-issue-badge").exists()).toBe(false);

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

  it("shows a Yahoo snapshot as an expected HTTP query", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-04T00:00:10Z"));
    const wrapper = mount(MarketFeedStatus, {
      props: {
        connectionState: "connected",
        observedAt: "2026-07-04T00:00:05Z",
        source: "yfinance",
      },
    });

    const badge = wrapper.get(".market-feed-provider-badge");
    expect(badge.attributes("data-state")).toBe("live");
    expect(badge.attributes("data-quality")).toBe("degraded");
    expect(badge.text()).toContain("Yahoo");
    expect(badge.attributes("title")).toContain("连接方式：HTTP 定时查询");
    expect(badge.attributes("title")).toContain(
      "数据质量：非实时快照，时效以供应商返回为准",
    );
    expect(badge.attributes("title")).not.toContain("降级");
    wrapper.unmount();
  });

  it("recognizes AKShare upstream sources as expected delayed HTTP queries", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-04T00:00:10Z"));
    const wrapper = mount(MarketFeedStatus, {
      props: {
        connectionState: "connected",
        observedAt: "2026-07-04T00:00:05Z",
        source: "akshare:eastmoney",
      },
    });

    const badge = wrapper.get(".market-feed-provider-badge");
    expect(badge.text()).toContain("AKShare");
    expect(badge.attributes("data-quality")).toBe("degraded");
    expect(badge.attributes("title")).toContain("连接方式：HTTP 定时查询");
    expect(badge.attributes("title")).toContain(
      "数据质量：非实时快照，时效以供应商返回为准",
    );
    wrapper.unmount();
  });

  it("shows a Futu push feed with the provider name in the normal badge", () => {
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

    const badge = wrapper.get(".market-feed-provider-badge");
    expect(badge.text()).toContain("Futu OpenD");
    expect(badge.attributes("data-state")).toBe("live");
    expect(badge.attributes("title")).toContain("供应商：Futu OpenD");
    expect(badge.attributes("title")).toContain("连接方式：实时推送");
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
