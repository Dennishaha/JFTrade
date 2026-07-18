// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

const featureMocks = vi.hoisted(() => ({
  fetch: vi.fn(),
}));

vi.mock("../src/composables/productFeatures", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../src/composables/productFeatures")>();
  return { ...actual, fetchProductFeature: featureMocks.fetch };
});

import ProductFeaturePanel from "../src/components/product/ProductFeaturePanel.vue";
import {
  resetBrokerProviderSelectionForTests,
  useBrokerProviderSelection,
} from "../src/composables/brokerProviderSelection";
import {
  featureEntryTitle,
  instrumentIDFromFeatureEntry,
} from "../src/composables/productFeatures";
import {
  flushPromises,
  productGlobalStubs,
  setupState,
} from "./productTestUtils";

const result = {
  provider: {
    brokerId: "futu",
    securityFirm: "Moomoo US",
    featureId: "research.test",
    capability: "available" as const,
    selectionReason: "explicit",
    resolvedAt: "2026-07-17T00:00:00Z",
    asOf: "2026-07-17T00:00:00Z",
  },
  asOf: "2026-07-17T00:00:00Z",
  nextCursor: "next",
  warnings: ["限频提示"],
  partialErrors: [{ scope: "US.BAD", code: "DENIED", message: "无权限" }],
  entries: [
    {
      name: "Apple",
      customFirst: "custom",
      instrumentId: "US.AAPL",
      lastPrice: 201.123456,
      active: true,
      nullable: null,
      tags: ["AI", "硬件"],
      detail: { sector: "Technology" },
    },
    {
      title: "腾讯",
      security: { market: "HK", code: "00700" },
      lastPrice: 500,
      active: false,
    },
  ],
};

afterEach(() => {
  vi.restoreAllMocks();
  featureMocks.fetch.mockReset();
  resetBrokerProviderSelectionForTests();
});

describe("product feature normalization and panel", () => {
  it("normalizes entry identities, titles, API envelopes, filtering and refresh", async () => {
    featureMocks.fetch.mockResolvedValue(result);
    const wrapper = mount(ProductFeaturePanel, {
      props: { title: "研究", description: "统一服务", path: "/api/data?x=1" },
      slots: {
        controls:
          '<select class="test-data-view"><option>数据视图</option></select>',
      },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();

    expect(featureMocks.fetch).toHaveBeenCalledWith("/api/data?x=1");
    const toolbar = wrapper.get(".product-panel-toolbar");
    expect(toolbar.find(".test-data-view").exists()).toBe(true);
    expect(toolbar.find(".product-feature-panel__filter").exists()).toBe(true);
    expect(toolbar.get(".product-feature-panel__filter").classes()).toContain(
      "product-compact-control",
    );
    expect(toolbar.get('button[title="刷新"]').classes()).toContain(
      "product-toolbar-refresh",
    );
    expect(toolbar.find(".broker-provider-tag-stub").exists()).toBe(false);
    expect(wrapper.text()).toContain("201.1235");
    expect(wrapper.text()).toContain("限频提示");
    expect(wrapper.text()).toContain("US.BAD · DENIED · 无权限");
    expect(wrapper.text()).toContain("还有下一页");
    const state = setupState<{
      formatCell: (value: unknown) => string;
      load: (refresh?: boolean) => Promise<void>;
    }>(wrapper);
    expect(state.formatCell(["AI", "硬件"])).toBe("2 项");
    expect(state.formatCell({ sector: "Technology" })).toBe("查看详情");
    expect(state.formatCell("text")).toBe("text");

    await wrapper.get("input").setValue("tencent-does-not-match");
    expect(wrapper.text()).not.toContain("Apple");
    await wrapper.get("input").setValue("apple");
    expect(wrapper.text()).toContain("Apple");

    await wrapper.get('button[title="刷新"]').trigger("click");
    await flushPromises();
    expect(featureMocks.fetch).toHaveBeenLastCalledWith(
      "/api/data?x=1&refresh=true",
    );

    const workspaceButtons = wrapper
      .findAll("button")
      .filter((button) => button.text() === "工作区");
    await workspaceButtons[0]!.trigger("click");
    expect(wrapper.emitted("openInstrument")?.[0]).toEqual(["US.AAPL"]);

    expect(instrumentIDFromFeatureEntry({ code: "US.MSFT" })).toBe("US.MSFT");
    expect(instrumentIDFromFeatureEntry({ securityCode: "HK.09988" })).toBe(
      "HK.09988",
    );
    expect(instrumentIDFromFeatureEntry({ stockCode: "SH.600000" })).toBe(
      "SH.600000",
    );
    expect(instrumentIDFromFeatureEntry({ contractCode: "US.EC.TEST" })).toBe(
      "US.EC.TEST",
    );
    expect(instrumentIDFromFeatureEntry({ code: "AAPL" })).toBeNull();
    expect(instrumentIDFromFeatureEntry({ security: null })).toBeNull();
    expect(
      instrumentIDFromFeatureEntry({ security: { market: "US" } }),
    ).toBeNull();
    expect(featureEntryTitle({ eventName: "Event" }, 0)).toBe("Event");
    expect(featureEntryTitle({ seriesName: "Series" }, 0)).toBe("Series");
    expect(featureEntryTitle({ code: "AAPL" }, 0)).toBe("AAPL");
    expect(featureEntryTitle({}, 2)).toBe("结果 3");
  });

  it("handles inactive, empty, and failed loads without retaining stale data", async () => {
    featureMocks.fetch.mockResolvedValueOnce({
      ...result,
      provider: { ...result.provider, capability: "degraded" as const },
      nextCursor: undefined,
      entries: [],
    });
    const wrapper = mount(ProductFeaturePanel, {
      props: { title: "空数据", path: "/api/empty", active: false },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();
    expect(featureMocks.fetch).not.toHaveBeenCalled();

    await wrapper.setProps({ active: true });
    await flushPromises();
    expect(wrapper.text()).toContain("当前账户与权限下暂无数据");
    expect(wrapper.find(".broker-provider-tag-stub").exists()).toBe(false);
    expect(wrapper.text()).not.toContain("还有下一页");

    featureMocks.fetch.mockRejectedValueOnce(new Error("上游失败"));
    await wrapper.setProps({ path: "/api/fail" });
    await flushPromises();
    expect(wrapper.text()).toContain("上游失败");

    featureMocks.fetch.mockRejectedValueOnce("字符串失败");
    const state = setupState<{ load: (refresh?: boolean) => Promise<void> }>(
      wrapper,
    );
    await state.load();
    expect(wrapper.text()).toContain("字符串失败");

    await wrapper.setProps({ path: "" });
    await state.load(true);
  });

  it("re-routes the active request when the shared provider changes", async () => {
    featureMocks.fetch.mockResolvedValue(result);
    const wrapper = mount(ProductFeaturePanel, {
      props: { title: "研究", path: "/api/data?x=1" },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();
    expect(featureMocks.fetch).toHaveBeenLastCalledWith("/api/data?x=1");

    useBrokerProviderSelection().selectBrokerProvider("alpha");
    await flushPromises();
    expect(featureMocks.fetch).toHaveBeenLastCalledWith(
      "/api/data?x=1&brokerId=alpha",
    );
    wrapper.unmount();
  });
});
