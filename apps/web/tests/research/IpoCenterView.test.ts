// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ fetch: vi.fn(), fetchWithInit: vi.fn() }));
vi.mock("../../src/composables/productFeatures", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../src/composables/productFeatures")>();
  return { ...actual, fetchProductFeature: mocks.fetch };
});
vi.mock("../../src/composables/apiClient", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../src/composables/apiClient")>();
  return { ...actual, fetchEnvelopeWithInit: mocks.fetchWithInit };
});

import IpoCenterView from "../../src/components/research/IpoCenterView.vue";
import { flushPromises } from "../productTestUtils";

function featureResult(
  entries: Record<string, unknown>[],
  envelope: Record<string, unknown> = {},
) {
  return {
    provider: { brokerId: "futu", featureId: "research.calendar", capability: "available" as const, selectionReason: "explicit", resolvedAt: "2026-07-17T00:00:00Z", asOf: "2026-07-17T00:00:00Z" },
    asOf: "2026-07-17T00:00:00Z",
    entries,
    ...envelope,
  };
}

const entries = [
  { instrumentId: "US.NEW1", market: "US", symbol: "NEW1", name: "待上市甲", productClass: "equity", status: "pending", listingDate: "2099-08-01", issuePriceMin: 12, issuePriceMax: 14, issueVolume: 2e7 },
  { instrumentId: "US.IPO2", market: "US", symbol: "IPO2", name: "已上市乙", productClass: "equity", status: "listed", listingDate: "2026-01-01" },
];

afterEach(() => {
  mocks.fetch.mockReset();
  mocks.fetchWithInit.mockReset();
});

async function mountView(data = entries) {
  mocks.fetch.mockResolvedValue(featureResult(data));
  mocks.fetchWithInit.mockResolvedValue(
    featureResult([{ symbol: "US.IPO2", name: "已上市乙", lastPrice: 30, previousClose: 25, observedAt: "2026-07-17T00:00:00Z" }]),
  );
  const wrapper = mount(IpoCenterView, { props: { brokerId: "futu" } });
  await flushPromises();
  return wrapper;
}

describe("IpoCenterView", () => {
  it("renders canonical pending/listed IPO fields and snapshot quotes", async () => {
    const wrapper = await mountView();
    const panels = wrapper.findAll(".ipo-center-view__panel");
    expect(panels[0]!.findAll("tbody tr")).toHaveLength(1);
    expect(panels[0]!.text()).toContain("NEW1");
    expect(panels[0]!.text()).toContain("12 ~ 14");
    expect(panels[0]!.text()).toContain("2000.00万股");
    expect(panels[1]!.findAll("tbody tr")).toHaveLength(1);
    await vi.waitFor(() => {
      expect(panels[1]!.text()).toContain("30");
      expect(panels[1]!.text()).toContain("+20.00%");
    });
    expect(wrapper.text()).not.toContain("首日涨幅");
  });

  it("opens only listed securities and keeps pending events non-interactive", async () => {
    const wrapper = await mountView();
    await wrapper.findAll(".ipo-center-view__panel")[0]!.get("tbody tr").trigger("click");
    expect(wrapper.emitted("select")).toBeUndefined();
    await wrapper.findAll(".ipo-center-view__panel")[1]!.get("tbody tr").trigger("click");
    expect(wrapper.emitted("select")?.[0]?.[0]).toMatchObject({ instrumentId: "US.IPO2" });
  });

  it("supports preserved date fields while canonical rollout is mixed", async () => {
    const wrapper = await mountView([
      { instrumentId: "US.OLD", symbol: "OLD", name: "兼容上市", status: "", eventDate: "2020-01-01" },
    ]);
    expect(wrapper.findAll(".ipo-center-view__panel")[1]!.text()).toContain("兼容上市");
  });

  it("renders empty and failed feature states", async () => {
    const empty = await mountView([]);
    expect(empty.findAll(".ipo-center-view__status")).toHaveLength(2);

    mocks.fetch.mockReset();
    mocks.fetchWithInit.mockReset();
    mocks.fetch.mockRejectedValue(new Error("IPO 上游失败"));
    const failed = mount(IpoCenterView);
    await flushPromises();
    expect(failed.text()).toContain("IPO 上游失败");
  });

  it("formats single prices, small and large volumes, and missing identities", async () => {
    const wrapper = await mountView([
      {
        status: "pending",
        issuePriceMin: 8,
        issueVolume: 999,
      },
      {
        name: "亿级发行",
        status: "upcoming",
        issuePriceMax: 9,
        issueVolume: 2e8,
      },
      {
        name: "无价格",
        status: "申购",
      },
    ]);
    const pending = wrapper.findAll(".ipo-center-view__panel")[0]!;
    expect(pending.text()).toContain("8");
    expect(pending.text()).toContain("999");
    expect(pending.text()).toContain("2.00亿股");
    expect(pending.text()).toContain("--");
  });

  it("shows snapshot failures and emits both more actions", async () => {
    mocks.fetch.mockResolvedValue(featureResult([entries[1]!]));
    mocks.fetchWithInit.mockRejectedValue(new Error("行情失败"));
    const wrapper = mount(IpoCenterView);
    await vi.waitFor(() => {
      expect(wrapper.text()).toContain("上市后行情补充失败：行情失败");
    });

    const more = wrapper.findAll(".ipo-center-view__more");
    await more[0]!.trigger("click");
    await more[1]!.trigger("click");
    expect(wrapper.emitted("more")).toEqual([["pending"], ["listed"]]);
  });

  it("loads the next IPO page and disables the button while pending", async () => {
    let resolveMore: ((value: unknown) => void) | undefined;
    mocks.fetch
      .mockResolvedValueOnce(
        featureResult([entries[0]!], {
          total: 2,
          hasMore: true,
          nextCursor: "next",
        }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveMore = resolve;
          }),
      );
    mocks.fetchWithInit.mockResolvedValue(featureResult([]));
    const wrapper = mount(IpoCenterView);
    await flushPromises();
    const button = wrapper.get(".ipo-center-view__load-more");
    await button.trigger("click");
    expect(button.attributes("disabled")).toBeDefined();
    expect(button.text()).toContain("加载中");
    resolveMore?.(featureResult([entries[1]!], { total: 2, hasMore: false }));
    await flushPromises();
    expect(wrapper.find(".ipo-center-view__load-more").exists()).toBe(false);
  });
});
