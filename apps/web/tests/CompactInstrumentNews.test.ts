// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  fetch: vi.fn(),
  externalClick: vi.fn(),
}));

vi.mock("../src/composables/productFeatures", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../src/composables/productFeatures")>();
  return { ...actual, fetchProductFeature: mocks.fetch };
});

vi.mock("../src/composables/externalLink", () => ({
  useExternalLink: () => ({ handleExternalLinkClick: mocks.externalClick }),
}));

import CompactInstrumentNews from "../src/components/domain/market-data/CompactInstrumentNews.vue";
import { flushPromises } from "./productTestUtils";

const target = {
  kind: "instrument" as const,
  instrumentId: "US.AAPL",
  name: "Apple",
  productClass: "equity" as const,
};

function result(entries: Record<string, unknown>[], extra = {}) {
  return {
    provider: {
      brokerId: "futu",
      featureId: "market.news",
      capability: "available" as const,
      selectionReason: "explicit",
      resolvedAt: "2026-07-23T00:00:00Z",
      asOf: "2026-07-23T00:00:00Z",
    },
    asOf: "2026-07-23T00:00:00Z",
    entries,
    ...extra,
  };
}

afterEach(() => {
  mocks.fetch.mockReset();
  mocks.externalClick.mockReset();
});

describe("CompactInstrumentNews", () => {
  it("does not query inactive, plate, incomplete, or unqualified targets", async () => {
    mocks.fetch.mockResolvedValue(result([]));
    const wrapper = mount(CompactInstrumentNews, {
      props: { target, active: false, brokerId: "", queryInstrumentId: "" },
    });
    expect(wrapper.text()).toContain("请选择支持资讯的数据源");
    await wrapper.setProps({ brokerId: "futu" });
    expect(wrapper.text()).toContain("暂未取得可查询的关联标的");
    await wrapper.setProps({ pending: true });
    expect(wrapper.text()).toContain("正在识别正股");
    await wrapper.setProps({
      pending: false,
      active: true,
      queryInstrumentId: "INVALID",
    });
    await flushPromises();
    expect(mocks.fetch).not.toHaveBeenCalled();

    await wrapper.setProps({
      target: { ...target, kind: "plate", productClass: "plate" },
      queryInstrumentId: "HK.BK1001",
    });
    await flushPromises();
    expect(mocks.fetch).not.toHaveBeenCalled();
  });

  it("renders, filters, opens, and refreshes rich news entries", async () => {
    mocks.fetch.mockResolvedValueOnce(
      result(
        [
          {
            title: "Apple 发布新品",
            newsType: "news",
            source: "OpenD",
            publishTime: "2026-07-23T01:30:00Z",
            summary: "新品摘要",
            url: "https://example.com/apple",
            imageUrl: "https://example.com/apple.png",
            relatedInstruments: ["US.MSFT"],
          },
          { title: "Apple 公告", newsType: "notice" },
          { title: "Apple 获评级", newsType: "rating" },
        ],
        {
          warnings: ["资讯延迟"],
          partialErrors: [
            { scope: "rating", code: "PARTIAL", message: "评级源失败" },
          ],
        },
      ),
    );
    const wrapper = mount(CompactInstrumentNews, {
      props: {
        target,
        active: true,
        brokerId: " Futu ",
        queryInstrumentId: " us.aapl ",
      },
    });
    await flushPromises();
    expect(mocks.fetch).toHaveBeenCalledWith(
      expect.stringMatching(
        /market=US&code=US(?:\.|%2E)AAPL&operation=search&pageSize=30.*brokerId=futu/,
      ),
    );
    expect(wrapper.text()).toContain("资讯延迟");
    expect(wrapper.text()).toContain("rating · 评级源失败");
    expect(wrapper.text()).toContain("新品摘要");
    expect(wrapper.get("img").attributes("alt")).toBe("Apple 发布新品");

    await wrapper.get("a").trigger("click");
    expect(mocks.externalClick).toHaveBeenCalledWith(
      expect.anything(),
      "https://example.com/apple",
    );
    await wrapper.get(".compact-instrument-news__related button").trigger("click");
    expect(wrapper.emitted("selectTarget")?.[0]?.[0]).toEqual({
      kind: "instrument",
      instrumentId: "US.MSFT",
      name: "",
      productClass: "unknown",
    });

    const noticeTab = wrapper
      .findAll('[role="tab"]')
      .find((button) => button.text() === "公告")!;
    await noticeTab.trigger("click");
    expect(wrapper.findAll(".compact-instrument-news__item")).toHaveLength(1);
    expect(wrapper.text()).toContain("Apple 公告");

    mocks.fetch.mockRejectedValueOnce(new Error("刷新失败原因"));
    await (
      wrapper.vm as unknown as { refresh: () => Promise<void> }
    ).refresh();
    await flushPromises();
    expect(wrapper.text()).toContain("刷新失败：刷新失败原因");
    expect(wrapper.text()).toContain("Apple 公告");
    expect(mocks.fetch).toHaveBeenLastCalledWith(
      expect.stringContaining("refresh=true"),
    );

    await wrapper.setProps({ queryInstrumentId: "US.MSFT" });
    expect(
      wrapper
        .findAll('[role="tab"]')
        .find((button) => button.text() === "全部")
        ?.attributes("aria-selected"),
    ).toBe("true");
  });

  it("shows initial loading, fallback errors, and empty results", async () => {
    let rejectRequest!: (reason: unknown) => void;
    mocks.fetch.mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          rejectRequest = reject;
        }),
    );
    const wrapper = mount(CompactInstrumentNews, {
      props: {
        target,
        active: true,
        brokerId: "futu",
        queryInstrumentId: "US.AAPL",
      },
    });
    expect(wrapper.text()).toContain("资讯加载中");
    rejectRequest(new Error(" "));
    await flushPromises();
    expect(wrapper.text()).toContain("相关资讯加载失败");

    mocks.fetch.mockResolvedValueOnce(result([]));
    await wrapper.setProps({ queryInstrumentId: "US.MSFT" });
    await flushPromises();
    expect(wrapper.text()).toContain("暂无匹配资讯");
  });
});
