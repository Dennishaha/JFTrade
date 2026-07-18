// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

const featureMocks = vi.hoisted(() => ({
  fetch: vi.fn(),
}));
const externalLinkMocks = vi.hoisted(() => ({
  handleExternalLinkClick: vi.fn((event: MouseEvent) => {
    event.preventDefault();
  }),
}));

vi.mock("../src/composables/productFeatures", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../src/composables/productFeatures")>();
  return { ...actual, fetchProductFeature: featureMocks.fetch };
});
vi.mock("../src/composables/externalLink", () => ({
  useExternalLink: () => externalLinkMocks,
}));

import NewsWorkspacePanel from "../src/components/product/NewsWorkspacePanel.vue";
import {
  flushPromises,
  productGlobalStubs,
  setupState,
} from "./productTestUtils";

const newsResult = {
  provider: {
    brokerId: "futu",
    securityFirm: "Moomoo US",
    featureId: "research.news",
    capability: "available" as const,
    selectionReason: "explicit",
    resolvedAt: "2026-07-17T12:00:00Z",
    asOf: "2026-07-17T12:00:00Z",
  },
  asOf: "2026-07-17T12:00:00Z",
  warnings: ["新闻权限提示"],
  partialErrors: [{ scope: "US.BAD", code: "DENIED", message: "无权限" }],
  entries: [
    {
      title: "<em>Apple</em> launches a new product",
      newsSubType: 1,
      source: "Market Wire",
      publishTime: "2026-07-17T11:17:00Z",
      summary: "A concise product update.",
      relatedSecurities: ["AAPL.US"],
      url: "https://example.com/apple",
    },
    {
      title: "Apple files an announcement",
      newsSubType: 2,
      source: "Exchange",
      publishTime: "2026-07-17T10:00:00Z",
    },
  ],
};

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  featureMocks.fetch.mockReset();
  externalLinkMocks.handleExternalLinkClick.mockClear();
});

describe("news workspace", () => {
  it("renders a readable news stream with category, search, links and refresh", async () => {
    vi.useFakeTimers();
    vi.setSystemTime("2026-07-17T12:00:00Z");
    featureMocks.fetch.mockResolvedValue(newsResult);
    const wrapper = mount(NewsWorkspacePanel, {
      props: {
        instrumentId: "US.AAPL",
        path: "/api/v1/market-data/news?market=US&code=US.AAPL",
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();

    expect(wrapper.text()).toContain("Apple launches a new product");
    expect(wrapper.text()).not.toContain("<em>");
    expect(wrapper.text()).toContain("Market Wire");
    expect(wrapper.text()).toContain("43分钟前");
    expect(wrapper.text()).toContain("新闻权限提示");
    expect(wrapper.text()).toContain("US.BAD · 无权限");
    expect(wrapper.get("a").attributes("href")).toBe(
      "https://example.com/apple",
    );
    await wrapper.get("a").trigger("click");
    expect(externalLinkMocks.handleExternalLinkClick).toHaveBeenCalledWith(
      expect.any(MouseEvent),
      "https://example.com/apple",
    );
    expect(wrapper.get(".news-workspace__filter").classes()).toContain(
      "product-compact-control",
    );
    expect(wrapper.get('button[title="刷新"]').classes()).toContain(
      "product-toolbar-refresh",
    );

    const state = setupState<{
      category: string;
      visibleItems: unknown[];
      load: (refresh?: boolean) => Promise<void>;
    }>(wrapper);
    state.category = "notice";
    await wrapper.vm.$nextTick();
    expect(state.visibleItems).toHaveLength(1);
    expect(wrapper.text()).toContain("Apple files an announcement");
    expect(wrapper.text()).not.toContain("launches a new product");

    state.category = "all";
    await wrapper.get("input").setValue("market wire");
    expect(state.visibleItems).toHaveLength(1);
    await wrapper.get(".news-workspace__related button").trigger("click");
    expect(wrapper.emitted("openInstrument")?.[0]).toEqual(["US.AAPL"]);

    await wrapper.get('button[title="刷新"]').trigger("click");
    await flushPromises();
    expect(featureMocks.fetch).toHaveBeenLastCalledWith(
      "/api/v1/market-data/news?market=US&code=US.AAPL&refresh=true",
    );
  });

  it("handles empty, failed and unresolved news paths", async () => {
    featureMocks.fetch.mockResolvedValueOnce({
      ...newsResult,
      warnings: [],
      partialErrors: [],
      entries: [],
    });
    const wrapper = mount(NewsWorkspacePanel, {
      props: { instrumentId: "US.AAPL", path: "/api/news" },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();
    expect(wrapper.text()).toContain("暂无匹配资讯");

    featureMocks.fetch.mockRejectedValueOnce(new Error("新闻加载失败"));
    await wrapper.setProps({ path: "/api/news-fail" });
    await flushPromises();
    expect(wrapper.text()).toContain("新闻加载失败");

    const callCount = featureMocks.fetch.mock.calls.length;
    await wrapper.setProps({ path: "" });
    await flushPromises();
    expect(featureMocks.fetch).toHaveBeenCalledTimes(callCount);
    const state = setupState<{ asOfLabel: string }>(wrapper);
    expect(state.asOfLabel).toBe("");
  });
});
