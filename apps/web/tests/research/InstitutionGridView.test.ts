// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ fetch: vi.fn() }));
vi.mock("../../src/composables/productFeatures", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../src/composables/productFeatures")>();
  return { ...actual, fetchProductFeature: mocks.fetch };
});

import InstitutionGridView from "../../src/components/research/InstitutionGridView.vue";
import { flushPromises } from "../productTestUtils";

function featureResult(
  entries: Record<string, unknown>[],
  metadata: Record<string, unknown> = {},
  envelope: Record<string, unknown> = {},
) {
  return {
    provider: {
      brokerId: "futu",
      featureId: "research.institutions",
      capability: "available" as const,
      selectionReason: "explicit",
      resolvedAt: "2026-07-17T00:00:00Z",
      asOf: "2026-07-17T00:00:00Z",
    },
    asOf: "2026-07-17T00:00:00Z",
    entries,
    metadata,
    total: entries.length,
    ...envelope,
  };
}

const institutions = [
  {
    institutionId: 101,
    name: "Berkshire Hathaway",
    marketValue: 3.8e11,
    marketValueChange: 2.5e9,
    holdingCount: 42,
    holdingCountChange: 2,
    asOfDate: "2026-06-30",
  },
  {
    institutionId: 202,
    institutionName: "ARK Invest",
    marketValue: 1.2e10,
    marketValueChange: -1.3e8,
    holdingCount: 35,
    holdingCountChange: -1,
    disclosureDate: "2026-06-30",
  },
];

afterEach(() => mocks.fetch.mockReset());

async function mountView(
  data = institutions,
  props: Record<string, unknown> = {},
) {
  mocks.fetch.mockImplementation((path: string) => {
    const operation = new URLSearchParams(path.split("?")[1]).get("operation");
    if (operation === "list") return Promise.resolve(featureResult(data, { currency: "USD" }));
    if (operation === "holdings") {
      return Promise.resolve(
        featureResult([
          {
            instrumentId: "US.AAPL",
            market: "US",
            symbol: "AAPL",
            name: "Apple",
            productClass: "equity",
            holdingPct: 5.1,
            lastHoldingPct: 4.8,
            changeShares: 1.2e6,
            holdingValue: 8.8e9,
            portfolioPct: 41.2,
          },
        ]),
      );
    }
    if (operation === "holding_changes") {
      return Promise.resolve(
        featureResult([
          {
            instrumentId: "US.TSLA",
            market: "US",
            symbol: "TSLA",
            name: "Tesla",
            productClass: "equity",
            portfolioPct: 3.25,
            changeShares: -250_000,
            changePct: -12.5,
            holdingDate: "2026-06-30",
            source: "13F",
          },
        ]),
      );
    }
    return Promise.resolve(featureResult([]));
  });
  const wrapper = mount(InstitutionGridView, {
    props: { market: "US", brokerId: "futu", ...props },
  });
  await flushPromises();
  return wrapper;
}

describe("InstitutionGridView", () => {
  it("renders canonical institution list fields without fake logos/holdings", async () => {
    const wrapper = await mountView();
    expect(wrapper.findAll(".institution-grid-view__card")).toHaveLength(2);
    expect(wrapper.text()).toContain("Berkshire Hathaway");
    expect(wrapper.text()).toContain("3800.00亿");
    expect(wrapper.text()).toContain("42");
    expect(wrapper.text()).toContain("披露 2026-06-30");
    expect(wrapper.text()).toContain("货币单位：USD");
    expect(wrapper.find(".institution-grid-view__logo").exists()).toBe(false);
    expect(wrapper.findAll(".institution-grid-view__mark")).toHaveLength(2);
  });

  it("filters list and lazily loads details using numeric institutionId", async () => {
    const wrapper = await mountView();
    await wrapper.get(".institution-grid-view__search").setValue("ark");
    expect(wrapper.findAll(".institution-grid-view__card")).toHaveLength(1);
    await wrapper.get(".institution-grid-view__card").trigger("click");
    await flushPromises();

    const calls = mocks.fetch.mock.calls.map(([path]) => String(path));
    expect(calls).toEqual(
      expect.arrayContaining([
        expect.stringContaining("operation=profile"),
        expect.stringContaining("operation=holdings"),
        expect.stringContaining("operation=distribution"),
        expect.stringContaining("institutionId=202"),
        expect.stringContaining("brokerId=futu"),
      ]),
    );
    expect(wrapper.text()).toContain("变动股数");
    expect(wrapper.text()).toContain("Apple");
    expect(wrapper.text()).toContain("41.20%");
    expect(wrapper.text()).toContain("5.10%");
    expect(wrapper.emitted("select")).toBeUndefined();
    await wrapper.get(".institution-grid-view__holdings tbody tr").trigger("click");
    expect(wrapper.emitted("select")?.[0]?.[0]).toMatchObject({
      instrumentId: "US.AAPL",
      productClass: "equity",
    });
  });

  it("shows an empty state", async () => {
    const wrapper = await mountView([]);
    expect(wrapper.text()).toContain("暂无数据");
  });

  it("selects an institution before requesting holding changes", async () => {
    const wrapper = await mountView(institutions, {
      operation: "holding_changes",
    });

    expect(wrapper.text()).toContain("请选择机构查看持仓变化");
    expect(
      mocks.fetch.mock.calls.some(([path]) =>
        String(path).includes("operation=holding_changes"),
      ),
    ).toBe(false);

    await wrapper.findAll(".institution-grid-view__card")[1]!.trigger("click");
    await flushPromises();

    const calls = mocks.fetch.mock.calls.map(([path]) => String(path));
    const holdingChangeCalls = calls.filter((path) =>
      path.includes("operation=holding_changes"),
    );
    expect(holdingChangeCalls).toHaveLength(1);
    expect(holdingChangeCalls[0]).toContain("institutionId=202");
    expect(holdingChangeCalls[0]).toContain("brokerId=futu");
    expect(calls.some((path) => path.includes("operation=profile"))).toBe(false);
    expect(calls.some((path) => path.includes("operation=holdings"))).toBe(false);
    expect(calls.some((path) => path.includes("operation=distribution"))).toBe(false);

    expect(wrapper.text()).toContain("Tesla");
    expect(wrapper.text()).toContain("+3.25%");
    expect(wrapper.text()).toContain("-25.00万");
    expect(wrapper.text()).toContain("-12.50%");
    expect(wrapper.text()).toContain("2026-06-30");
    expect(wrapper.text()).toContain("13F");

    await wrapper.get(".institution-grid-view__holdings tbody tr").trigger("click");
    expect(wrapper.emitted("select")?.[0]?.[0]).toMatchObject({
      instrumentId: "US.TSLA",
      productClass: "equity",
    });
  });

  it("paginates institution lists and selected holdings with cursors", async () => {
    mocks.fetch.mockImplementation((path: string) => {
      const params = new URLSearchParams(path.split("?")[1]);
      const operation = params.get("operation");
      const cursor = params.get("cursor");
      if (operation === "list") {
        return Promise.resolve(
          cursor === "institutions-next"
            ? featureResult([institutions[1]!], {}, { total: 2, hasMore: false })
            : featureResult([institutions[0]!], {}, {
                total: 2,
                nextCursor: "institutions-next",
                hasMore: true,
              }),
        );
      }
      if (operation === "holdings") {
        const entry = (symbol: string) => ({
          instrumentId: `US.${symbol}`,
          market: "US",
          symbol,
          name: symbol,
          productClass: "equity",
        });
        return Promise.resolve(
          cursor === "holdings-next"
            ? featureResult([entry("MSFT")], {}, { total: 2, hasMore: false })
            : featureResult([entry("AAPL")], {}, {
                total: 2,
                nextCursor: "holdings-next",
                hasMore: true,
              }),
        );
      }
      return Promise.resolve(featureResult([]));
    });
    const wrapper = mount(InstitutionGridView, { props: { market: "US" } });
    await flushPromises();
    await wrapper.get(".institution-grid-view__load-more").trigger("click");
    await vi.waitFor(() => {
      expect(wrapper.findAll(".institution-grid-view__card")).toHaveLength(2);
    });
    await wrapper.findAll(".institution-grid-view__card")[0]!.trigger("click");
    await vi.waitFor(() => {
      expect(wrapper.text()).toContain("加载更多持仓");
    });
    await wrapper
      .findAll(".institution-grid-view__load-more")
      .find((button) => button.text().includes("持仓"))!
      .trigger("click");
    await vi.waitFor(() => {
      expect(wrapper.findAll(".institution-grid-view__holdings tbody tr")).toHaveLength(2);
    });
  });

  it("uses HK defaults, filters unnamed cards, and clears or closes details", async () => {
    mocks.fetch.mockImplementation((path: string) => {
      const operation = new URLSearchParams(path.split("?")[1]).get("operation");
      if (operation === "list") {
        return Promise.resolve(
          featureResult([{ institutionId: 101, marketValue: null }]),
        );
      }
      return Promise.resolve(featureResult([]));
    });
    const wrapper = mount(InstitutionGridView, {
      props: { market: "HK" },
    });
    await flushPromises();
    expect(wrapper.text()).toContain("港股机构");
    expect(wrapper.text()).toContain("货币单位：HKD");
    expect(wrapper.get(".institution-grid-view__name").text()).toBe("--");
    expect(wrapper.text()).not.toContain("披露");

    await wrapper.get(".institution-grid-view__card").trigger("click");
    await flushPromises();
    expect(wrapper.find(".institution-grid-view__details").exists()).toBe(true);
    await wrapper
      .get(".institution-grid-view__details-head button")
      .trigger("click");
    expect(wrapper.find(".institution-grid-view__details").exists()).toBe(false);

    await wrapper.get(".institution-grid-view__search").setValue("missing");
    expect(wrapper.text()).toContain("暂无数据");
    await wrapper.setProps({ operation: "holding_changes" });
    expect(wrapper.text()).toContain("港股机构持仓变化");
    expect(wrapper.text()).toContain("请选择机构查看持仓变化");
  });

  it("shows list loading and request failures", async () => {
    let rejectRequest!: (reason: unknown) => void;
    mocks.fetch.mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          rejectRequest = reject;
        }),
    );
    const wrapper = mount(InstitutionGridView);
    expect(wrapper.text()).toContain("加载中");
    rejectRequest(new Error("机构列表失败"));
    await flushPromises();
    expect(wrapper.text()).toContain("机构列表失败");
  });

  it("renders detail errors plus holding-change warnings, empty rows, and pagination labels", async () => {
    let resolveMore!: (value: ReturnType<typeof featureResult>) => void;
    mocks.fetch.mockImplementation((path: string) => {
      const params = new URLSearchParams(path.split("?")[1]);
      const operation = params.get("operation");
      if (operation === "list") {
        return Promise.resolve(featureResult([institutions[0]!]));
      }
      if (operation === "holding_changes") {
        if (params.get("cursor")) {
          return new Promise((resolve) => {
            resolveMore = resolve;
          });
        }
        return Promise.resolve(
          featureResult([], {}, {
            warnings: ["数据延迟"],
            partialErrors: [
              { scope: "13F", code: "PARTIAL", message: "部分来源失败" },
            ],
            total: 0,
            nextCursor: "next",
            hasMore: true,
          }),
        );
      }
      return Promise.reject(new Error("详情失败"));
    });
    const wrapper = mount(InstitutionGridView, {
      props: { operation: "holding_changes" },
    });
    await flushPromises();
    await wrapper.get(".institution-grid-view__card").trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("数据延迟；部分来源失败");
    expect(wrapper.text()).toContain("暂无持仓变化");
    const more = wrapper
      .findAll(".institution-grid-view__load-more")
      .find((button) => button.text().includes("加载更多变化"))!;
    await more.trigger("click");
    expect(more.text()).toContain("正在加载变化");
    resolveMore(featureResult([]));
    await flushPromises();
  });

  it("shows profile errors and ignores non-quoteable holding rows", async () => {
    mocks.fetch.mockImplementation((path: string) => {
      const operation = new URLSearchParams(path.split("?")[1]).get("operation");
      if (operation === "list") {
        return Promise.resolve(featureResult([institutions[0]!]));
      }
      if (operation === "profile") return Promise.reject(new Error("资料失败"));
      if (operation === "holdings") {
        return Promise.resolve(
          featureResult([{ holdingDate: "2026-06-30", name: "现金" }]),
        );
      }
      return Promise.resolve(featureResult([]));
    });
    const wrapper = mount(InstitutionGridView);
    await flushPromises();
    await wrapper.get(".institution-grid-view__card").trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("资料失败");
    const row = wrapper.get(".institution-grid-view__holdings tbody tr");
    expect(row.classes()).not.toContain("is-quoteable");
    await row.trigger("click");
    expect(wrapper.emitted("select")).toBeUndefined();
    expect(row.text()).toContain("--");
  });
});
