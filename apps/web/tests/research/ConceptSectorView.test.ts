// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ fetch: vi.fn(), fetchWithInit: vi.fn() }));

vi.mock("../../src/composables/productFeatures", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../src/composables/productFeatures")>();
  return { ...actual, fetchProductFeature: mocks.fetch };
});
vi.mock("../../src/composables/apiClient", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../src/composables/apiClient")>();
  return { ...actual, fetchEnvelopeWithInit: mocks.fetchWithInit };
});

import ConceptSectorView from "../../src/components/research/ConceptSectorView.vue";
import { flushPromises } from "../productTestUtils";

function featureResult(entries: Record<string, unknown>[]) {
  return {
    provider: {
      brokerId: "futu",
      featureId: "research.industry",
      capability: "available" as const,
      selectionReason: "explicit",
      resolvedAt: "2026-07-17T00:00:00Z",
      asOf: "2026-07-17T00:00:00Z",
    },
    asOf: "2026-07-17T00:00:00Z",
    entries,
  };
}

const plates = [
  { instrumentId: "US.BK100", market: "US", symbol: "BK100", name: "半导体", productClass: "plate" },
  { instrumentId: "US.BK200", market: "US", symbol: "BK200", name: "新能源", productClass: "plate" },
];
const stocks = [
  { instrumentId: "US.AAA", market: "US", symbol: "AAA", name: "甲", productClass: "equity" },
  { instrumentId: "US.BBB", market: "US", symbol: "BBB", name: "乙", productClass: "equity" },
];

afterEach(() => {
  mocks.fetch.mockReset();
  mocks.fetchWithInit.mockReset();
});

function mountView(market = "US") {
  mocks.fetch.mockImplementation((path: string) => {
    const params = new URLSearchParams(path.split("?")[1]);
    return Promise.resolve(
      featureResult(params.get("operation") === "plate_list" ? plates : stocks),
    );
  });
  mocks.fetchWithInit.mockImplementation((_path: string, init: RequestInit) => {
    const ids = JSON.parse(String(init.body)).instrumentIds as string[];
    return Promise.resolve(
      featureResult(
        ids.map((symbol, index) => ({
          symbol,
          lastPrice: index === 0 ? 10 : 20,
          previousClose: index === 0 ? 9 : 21,
          volume: index === 0 ? 1e6 : 2e6,
          turnover: index === 0 ? 1e7 : 4e7,
          observedAt: "2026-07-17T00:00:00Z",
        })),
      ),
    );
  });
  return mount(ConceptSectorView, {
    props: { market, brokerId: "futu" },
  });
}

describe("ConceptSectorView", () => {
  it("uses plate_list then exact plate_members instrumentId", async () => {
    const wrapper = mountView();
    await flushPromises();

    const rows = wrapper.findAll(".concept-sector-view__plates tbody tr");
    expect(rows).toHaveLength(2);
    expect(rows[0]!.classes()).toContain("is-selected");
    await vi.waitFor(() => expect(wrapper.text()).toContain("AAA"));
    const calls = mocks.fetch.mock.calls.map(([path]) => String(path));
    expect(calls).toEqual(
      expect.arrayContaining([
        expect.stringContaining("operation=plate_list"),
        expect.stringContaining("plateType=concept"),
        expect.stringContaining("operation=plate_members"),
        expect.stringContaining("instrumentId=US.BK100"),
        expect.stringContaining("brokerId=futu"),
      ]),
    );
    expect(calls.some((path) => path.includes("operation=plate_stocks"))).toBe(false);
  });

  it("switches plate, opens the plate quote target source and refetches members", async () => {
    const wrapper = mountView();
    await flushPromises();
    await wrapper.findAll(".concept-sector-view__plates tbody tr")[1]!.trigger("click");
    await flushPromises();

    expect(wrapper.emitted("select")?.[0]?.[0]).toMatchObject({
      instrumentId: "US.BK200",
      productClass: "plate",
    });
    expect(mocks.fetch).toHaveBeenCalledWith(
      expect.stringContaining("instrumentId=US.BK200"),
    );
  });

  it("enriches member quotes and emits exact SH/SZ/US instrument rows", async () => {
    const wrapper = mountView();
    await flushPromises();
    await vi.waitFor(() => {
      expect(wrapper.findAll(".concept-sector-view__stocks tbody tr")).toHaveLength(2);
      expect(wrapper.text()).toContain("+11.11%");
    });
    const memberRows = wrapper.findAll(".concept-sector-view__stocks tbody tr");
    await memberRows[0]!.trigger("click");
    expect(wrapper.emitted("select")?.at(-1)?.[0]).toMatchObject({
      instrumentId: "US.AAA",
    });
  });

  it("offers a real-name-only 港股通 filter without inventing IDs", async () => {
    mocks.fetch.mockImplementation((path: string) => {
      const params = new URLSearchParams(path.split("?")[1]);
      const rows =
        params.get("operation") === "plate_list"
          ? [
              { instrumentId: "HK.BK001", market: "HK", symbol: "BK001", name: "港股通（沪）", productClass: "plate" },
              { instrumentId: "HK.BK002", market: "HK", symbol: "BK002", name: "恒生行业", productClass: "plate" },
            ]
          : [];
      return Promise.resolve(featureResult(rows));
    });
    mocks.fetchWithInit.mockResolvedValue(featureResult([]));
    const wrapper = mount(ConceptSectorView, { props: { market: "HK" } });
    await flushPromises();
    await wrapper.get(".concept-sector-view__connect").trigger("click");
    await flushPromises();
    expect(wrapper.findAll(".concept-sector-view__plates tbody tr")).toHaveLength(1);
    expect(wrapper.text()).toContain("港股通（沪）");
  });

  it("covers plate type changes, snapshot warnings, and empty connect filters", async () => {
    mocks.fetch.mockImplementation((path: string) => {
      const operation = new URLSearchParams(path.split("?")[1]).get("operation");
      return Promise.resolve(
        featureResult(operation === "plate_list" ? plates : []),
      );
    });
    mocks.fetchWithInit.mockRejectedValue(new Error("snapshot offline"));
    const wrapper = mount(ConceptSectorView, {
      props: { market: "HK", brokerId: "futu" },
    });
    await flushPromises();
    await vi.waitFor(() => {
      expect(wrapper.text()).toContain("行情补充失败：snapshot offline");
    });

    const typeButtons = wrapper.findAll(".concept-sector-view__types button");
    await typeButtons.find((button) => button.text() === "行业")!.trigger("click");
    await typeButtons.find((button) => button.text() === "地域")!.trigger("click");
    expect(mocks.fetch).toHaveBeenCalledWith(
      expect.stringContaining("plateType=region"),
    );

    await wrapper.get(".concept-sector-view__connect").trigger("click");
    expect(wrapper.text()).toContain("当前 OpenD 未返回港股通相关板块");
  });

  it("handles invalid plate identities and sorts missing member metrics", async () => {
    mocks.fetch.mockImplementation((path: string) => {
      const operation = new URLSearchParams(path.split("?")[1]).get("operation");
      if (operation === "plate_list") {
        return Promise.resolve(
          featureResult([
            { instrumentId: "US.BK100", name: "" },
            { instrumentId: "BROKEN", name: "无成员路径" },
          ]),
        );
      }
      return Promise.resolve(
        featureResult([
          { instrumentId: "US.NULL", symbol: "", name: "", price: null },
          { instrumentId: "US.LOW", symbol: "LOW", name: "低", price: 5 },
          { instrumentId: "US.HIGH", symbol: "HIGH", name: "高", price: 10 },
        ]),
      );
    });
    mocks.fetchWithInit.mockResolvedValue(featureResult([]));
    const wrapper = mount(ConceptSectorView);
    await flushPromises();
    await vi.waitFor(() => {
      expect(
        wrapper.findAll(".concept-sector-view__stocks tbody tr"),
      ).toHaveLength(3);
    });

    const priceHeader = wrapper
      .findAll(".concept-sector-view__sortable")
      .find((cell) => cell.text().includes("最新价"))!;
    await priceHeader.trigger("click");
    expect(priceHeader.text()).toContain("↓");
    await priceHeader.trigger("click");
    expect(priceHeader.text()).toContain("↑");
    const names = wrapper
      .findAll(".concept-sector-view__stocks tbody tr")
      .map((row) => row.text());
    expect(names[0]).toContain("低");
    expect(names.at(-1)).toContain("--");

    await wrapper.findAll(".concept-sector-view__plates tbody tr")[1]!.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("暂无数据");
  });

  it("shows plate request failures", async () => {
    mocks.fetch.mockRejectedValue(new Error("板块失败"));
    mocks.fetchWithInit.mockResolvedValue(featureResult([]));
    const wrapper = mount(ConceptSectorView);
    await flushPromises();
    expect(wrapper.text()).toContain("板块失败");
  });
});
