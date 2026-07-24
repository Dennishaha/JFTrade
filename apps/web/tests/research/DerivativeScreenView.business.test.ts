// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ useResearchFeature: vi.fn() }));

vi.mock("../../src/composables/useResearchFeature", () => ({
  useResearchFeature: mocks.useResearchFeature,
}));

import DerivativeScreenView from "../../src/components/research/DerivativeScreenView.vue";

function state(entries: Record<string, unknown>[] = []) {
  return {
    entries: ref(entries),
    loading: ref(false),
    error: ref(""),
    asOf: ref("2026-07-24"),
    hasMore: ref(false),
    loadingMore: ref(false),
    refresh: vi.fn(),
    loadMore: vi.fn(),
  };
}

function mountScreen(feature: ReturnType<typeof state>, props: Record<string, unknown>) {
  mocks.useResearchFeature.mockImplementationOnce((source: unknown) => {
    if (source != null && typeof source === "object" && "value" in source) {
      void (source as { value: unknown }).value;
    }
    return feature;
  });
  return mount(DerivativeScreenView, { props });
}

beforeEach(() => mocks.useResearchFeature.mockReset());

describe("derivative screen business variants", () => {
  it("normalizes option identities, directions, dates, Greeks, and row actions", async () => {
    const feature = state([
      {
        optionName: "AAPL Call",
        security: { instrumentId: "US.AAPL270117C00200000" },
        underlyingInfo: { stockID: 123 },
        optionType: 1,
        strikeDate: 20270117,
        strikePrice: 200,
        price: 12.5,
        changeRate: 3.2,
        volume: 1_200,
        openInterest: 5_000,
        impliedVolatility: 22.5,
        delta: 0.55,
      },
      {
        name: "AAPL Put",
        security: { market: "US", code: "AAPL270118P00190000" },
        underlyingInfo: { stockId: "456" },
        optionType: 2,
        strikeDate: "20270118",
        currentPrice: 9,
        changeRate: -1,
        IV: 30,
        Greeks: { delta: -0.45 },
      },
      { security: { code: "CALL" }, optionType: "call", strikeDate: "20270230", greeks: { delta: 0.1 } },
      { instrumentId: "US.PUT", optionType: "put", strikeDate: "1784851200" },
      { security: [], optionType: 9, strikeDate: "not-a-date" },
      { security: null, optionType: "custom", strikeDate: -1 },
      { optionType: "", strikeDate: null },
    ]);
    const wrapper = mountScreen(feature, { operation: "option_screen", brokerId: "futu" });

    expect(wrapper.text()).toContain("期权筛选");
    expect(wrapper.text()).toContain("看涨");
    expect(wrapper.text()).toContain("看跌");
    expect(wrapper.text()).toContain("类型 9");
    expect(wrapper.text()).toContain("custom");
    expect(wrapper.text()).toContain("2027-01-17");
    expect(wrapper.text()).toContain("20270230");
    expect(wrapper.text()).toContain("Stock ID 123");
    expect(wrapper.text()).toContain("0.55");
    expect(wrapper.text()).toContain("-0.45");

    const rows = wrapper.findAll("tbody tr");
    await rows[0]!.trigger("click");
    await rows[0]!.trigger("dblclick");
    expect(wrapper.emitted("select")?.[0]?.[0]).toMatchObject({
      instrumentId: "US.AAPL270117C00200000",
      productClass: "option",
    });
    expect(wrapper.emitted("open")?.[0]?.[0]).toMatchObject({ productClass: "option" });

    await wrapper.get(".derivative-screen__toolbar input").setValue("stock id 456");
    expect(wrapper.findAll("tbody tr")).toHaveLength(1);
    await wrapper.get(".derivative-screen__toolbar input").setValue("");
    await wrapper.get(".derivative-screen__toolbar button").trigger("click");
    expect(feature.refresh).toHaveBeenCalledOnce();
  });

  it("maps all warrant types, owner identities, and CBBC classification", async () => {
    const feature = state([
      { name: "认购证", security: { market: "HK", code: "10001" }, ownerName: "腾讯", warrantType: 1, maturityTime: 1784851200 },
      { security: { code: "10002" }, owner: { market: "HK", symbol: "00700" }, warrantType: 2, maturityDate: "1784851200" },
      { security: { instrumentId: "HK.60001" }, ownerSecurity: { instrumentId: "HK.00700" }, warrantType: 3 },
      { instrumentId: "HK.60002", ownerSecurity: { code: "09988" }, warrantType: 4 },
      { name: "界内证", warrantType: 5 },
      { name: "未来类型", warrantType: 9 },
      { name: "Bull", warrantType: "bull" },
      { name: "Bear", warrantType: "bear" },
      { name: "普通轮证", warrantType: "warrant" },
      { name: "未知轮证" },
    ]);
    const wrapper = mountScreen(feature, { operation: "warrant" });

    expect(wrapper.text()).toContain("港股轮证筛选");
    for (const label of ["认购", "认沽", "牛证", "熊证", "界内证", "类型 9", "bull", "bear", "warrant"]) {
      expect(wrapper.text()).toContain(label);
    }
    const rows = wrapper.findAll("tbody tr");
    await rows[2]!.trigger("click");
    expect(wrapper.emitted("select")?.at(-1)?.[0]).toMatchObject({ productClass: "cbbc" });
    await rows[6]!.trigger("click");
    expect(wrapper.emitted("select")?.at(-1)?.[0]).toMatchObject({ productClass: "cbbc" });
    await rows[8]!.trigger("click");
    expect(wrapper.emitted("select")?.at(-1)?.[0]).toMatchObject({ productClass: "warrant" });

    await wrapper.get(".derivative-screen__toolbar input").setValue("00700");
    expect(wrapper.findAll("tbody tr").length).toBeGreaterThan(0);
  });

  it("shows request boundaries and load-more lifecycle", async () => {
    const feature = state();
    feature.loading.value = true;
    feature.asOf.value = "";
    feature.hasMore.value = true;
    const wrapper = mountScreen(feature, { operation: "warrant" });
    expect(wrapper.text()).toContain("加载中");

    feature.loading.value = false;
    feature.error.value = "轮证筛选失败";
    await flushPromises();
    expect(wrapper.text()).toContain("轮证筛选失败");
    feature.error.value = "";
    await flushPromises();
    expect(wrapper.text()).toContain("暂无符合条件的轮证");
    await wrapper.get(".derivative-screen__more").trigger("click");
    expect(feature.loadMore).toHaveBeenCalledOnce();
    feature.loadingMore.value = true;
    await flushPromises();
    expect(wrapper.get(".derivative-screen__more").attributes("disabled")).toBeDefined();
    expect(wrapper.get(".derivative-screen__more").text()).toContain("加载中");
  });
});
