// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ useResearchFeature: vi.fn() }));

vi.mock("../../src/composables/useResearchFeature", () => ({
  useResearchFeature: mocks.useResearchFeature,
}));

import InstrumentResearchView from "../../src/components/research/InstrumentResearchView.vue";

function state(entries: Record<string, unknown>[] = []) {
  return {
    entries: ref(entries),
    metadata: ref<Record<string, unknown>>({}),
    loading: ref(false),
    error: ref(""),
    asOf: ref("2026-07-24"),
    refresh: vi.fn(),
  };
}

function mountInstrument(
  feature: ReturnType<typeof state>,
  props: Record<string, unknown>,
) {
  mocks.useResearchFeature.mockImplementationOnce((source: unknown) => {
    if (source != null && typeof source === "object" && "value" in source) {
      void (source as { value: unknown }).value;
    }
    return feature;
  });
  return mount(InstrumentResearchView, {
    props: { instrumentId: "US.AAPL", ...props },
    global: {
      stubs: {
        InstrumentSearchBox: {
          name: "InstrumentSearchBox",
          props: ["modelValue"],
          emits: ["select", "update:modelValue"],
          template: "<div class='instrument-search-stub'>{{ modelValue }}</div>",
        },
        CompactInstrumentNews: {
          name: "CompactInstrumentNews",
          props: ["target", "queryInstrumentId"],
          emits: ["selectTarget"],
          template: "<div class='news-stub'>{{ queryInstrumentId }} · {{ target.instrumentId }}</div>",
        },
      },
    },
  });
}

beforeEach(() => mocks.useResearchFeature.mockReset());

describe("instrument research protocol boundaries", () => {
  it("groups profile aliases, links URLs, validates search candidates, and shows status", async () => {
    const feature = state([
      { fieldType: "normal", name: "官网", value: "https://apple.example" },
      { type: "TITLE", content: "公司资料" },
      { fieldName: "CEO", content: "Tim Cook" },
      { fieldType: "title", name: "", value: "" },
      { name: "", value: "" },
      { name: "只有名称" },
      { value: "只有内容" },
    ]);
    const wrapper = mountInstrument(feature, { operation: "profile", brokerId: "futu" });
    expect(wrapper.text()).toContain("基本资料");
    expect(wrapper.text()).toContain("公司资料");
    expect(wrapper.text()).toContain("其他资料");
    expect(wrapper.get(".instrument-research__profile a").attributes("href")).toBe("https://apple.example");
    expect(wrapper.text()).toContain("只有名称");
    expect(wrapper.text()).toContain("只有内容");

    const search = wrapper.getComponent({ name: "InstrumentSearchBox" });
    search.vm.$emit("select", { selectable: false, instrumentId: "US.BAD" });
    search.vm.$emit("select", { selectable: true, instrumentId: "" });
    search.vm.$emit("select", { selectable: true, instrumentId: "us.msft" });
    expect(wrapper.emitted("update:instrumentId")?.at(-1)).toEqual(["US.MSFT"]);
    await wrapper.setProps({ instrumentId: "HK.00700" });
    expect(wrapper.get(".instrument-search-stub").text()).toContain("HK.00700");
    await wrapper.get(".instrument-research__toolbar button").trigger("click");
    expect(feature.refresh).toHaveBeenCalledOnce();

    feature.loading.value = true;
    await flushPromises();
    expect(wrapper.text()).toContain("研究数据加载中");
    feature.loading.value = false;
    feature.error.value = "公司资料失败";
    await flushPromises();
    expect(wrapper.text()).toContain("公司资料失败");
    feature.error.value = "";
    feature.entries.value = [];
    await flushPromises();
    expect(wrapper.text()).toContain("暂无公司资料");
  });

  it("builds financial columns from all structure and item aliases", async () => {
    const feature = state([
      {
        periodText: "2026 Q2",
        currencyCode: "USD",
        itemList: [
          { fieldId: 1, data: 1_200_000, yoy: 12.5, qoq: 2 },
          { id: "2", value: "N/A", yoy: "bad" },
          { fieldId: "", data: 9 },
          null,
        ],
      },
      { reportDate: "2026-03-31", currency: "HKD", items: [{ id: 1, value: 900_000, yoy: -2 }] },
    ]);
    feature.metadata.value = {
      structureList: [
        { fieldId: 1, displayName: "营收" },
        { id: 2, fieldName: "利润" },
        { id: "", name: "无效" },
        null,
        ...Array.from({ length: 15 }, (_, index) => ({ id: index + 3, name: `字段${index}` })),
      ],
    };
    const wrapper = mountInstrument(feature, { operation: "financials" });
    expect(wrapper.text()).toContain("2026 Q2");
    expect(wrapper.text()).toContain("营收");
    expect(wrapper.text()).toContain("利润");
    expect(wrapper.text()).toContain("120.00万 (+12.50%)");
    expect(wrapper.text()).toContain("N/A");
    expect(wrapper.findAll("thead th")).toHaveLength(15);

    feature.metadata.value = { fields: [{ id: "cash", name: "现金" }] };
    feature.entries.value = [{ period: "FY", items: [{ id: "cash", value: 10 }] }];
    await flushPromises();
    expect(wrapper.text()).toContain("现金");
    feature.metadata.value = { statementStructureList: [{ id: "debt", name: "负债" }] };
    await flushPromises();
    expect(wrapper.text()).toContain("负债");
  });

  it("covers valuation labels, interval edges, market codes, and empty data", async () => {
    const markets = [1, 11, 21, 22, 31, 41, 51, 61, 71, 81, 91, 101, 999];
    const feature = state([
      {
        valuationType: 9,
        trend: {
          currentValue: 20,
          valuationPercentile: null,
          historicalItems: [{ time: "2026-01", value: 10, plateValue: 12 }],
        },
        marketDistribution: {
          total: 100,
          sections: [
            { start: null, end: 5, number: 1 },
            { start: 10, end: 0, number: 2 },
            { start: 5, end: 10, number: 3 },
          ],
        },
        plateDistribution: {
          plate: { market: "HK", code: "BK100" },
          stockItems: markets.map((market, index) => ({
            name: `证券${index}`,
            security: { market, code: `C${index}` },
            value: index,
            marketCap: 10_000 + index,
          })).concat([{ name: "无证券", security: null }]),
        },
        profitGrowthRate: {
          financialTtmMultiple: null,
          conclusionDetailed: "增长稳定",
          profitData: [
            { periodStr: "FY2025", marketCapMultiple: null, financeDataMultiple: 2 },
            { financialYear: "2024", financialQuarter: "Q4", reportDate: "2025-03", marketCapMultiple: 3, financeDataMultiple: null },
          ],
        },
      },
    ]);
    const wrapper = mountInstrument(feature, { operation: "valuation" });
    expect(wrapper.text()).toContain("估值趋势");
    expect(wrapper.text()).toContain("估值");
    expect(wrapper.text()).toContain("10 以上");
    expect(wrapper.text()).toContain("5 – 10");
    expect(wrapper.text()).toContain("HK.C0");
    expect(wrapper.text()).toContain("US.C1");
    expect(wrapper.text()).toContain("CC.C10");
    expect(wrapper.text()).toContain("增长稳定");
    expect(wrapper.text()).toContain("2024/Q4");

    for (const valuationType of [1, 2, 3, "ValuationType_pb", "PS", "other"]) {
      feature.entries.value = [{ ...feature.entries.value[0], valuationType }];
      await flushPromises();
    }
    feature.entries.value = [{ trend: [], marketDistribution: null, plateDistribution: "bad", profitGrowthRate: null }];
    await flushPromises();
    expect(wrapper.text()).toContain("暂无估值数据");
  });

  it("maps analyst rating numbers and names while clamping distribution bars", async () => {
    const feature = state([
      {
        rating: 1,
        total: 0,
        lowest: 100,
        average: 120,
        highest: 140,
        strongBuy: 120,
        buy: -5,
        hold: 0,
        underperform: 10,
        sell: 20,
        updateTime: "today",
      },
    ]);
    const wrapper = mountInstrument(feature, { operation: "analyst" });
    expect(wrapper.text()).toContain("卖出");
    expect(wrapper.text()).toContain("0 位分析师");
    const bars = wrapper.findAll(".instrument-research__rating-row b");
    expect(bars[0]!.attributes("style")).toContain("100%");
    expect(bars[1]!.attributes("style")).toContain("0%");

    for (const rating of [2, 3, 4, 5, 9, "ResearchRatingType_StrongBuy", "underperform", "unknown", null]) {
      feature.entries.value = [{ ...feature.entries.value[0], rating }];
      await flushPromises();
    }
    feature.entries.value = [{}];
    await flushPromises();
    expect(wrapper.findAll(".instrument-research__rating-row")).toHaveLength(0);
    expect(wrapper.text()).not.toContain("位分析师");
  });

  it("falls back between ownership metadata groups and entry groups", async () => {
    const feature = state([
      { staticDate: "2026-01", itemList: [{ holderId: 1, name: "主要 A", holdingRatio: 12 }] },
      { staticDateStr: "2026-02", itemList: [{ holderId: 0, name: "基金", percentage: 30 }, null] },
      { itemList: "bad" },
    ]);
    const wrapper = mountInstrument(feature, { operation: "ownership" });
    expect(wrapper.text()).toContain("主要股东");
    expect(wrapper.text()).toContain("持股类型");
    expect(wrapper.text()).toContain("主要 A");
    expect(wrapper.text()).toContain("基金");

    feature.metadata.value = {
      mainHolderInfoList: [{ staticDateStr: "2026-03", itemList: [{ name: "Meta A", holderPct: 8 }] }],
      holderTypeInfoList: [{ staticDateStr: "2026-04", itemList: [{ name: "Meta B", ratio: 9 }] }],
    };
    await flushPromises();
    expect(wrapper.text()).toContain("Meta A");
    expect(wrapper.text()).toContain("Meta B");
    expect(wrapper.text()).not.toContain("主要 A");
  });

  it("emits corporate actions and news target changes", async () => {
    const feature = state([
      { pubDate: "2026-01", statement: "每股派息", process: "已实施", recordDate: "2026-02", exDate: "2026-03", dividendPayableDate: "2026-04", fiscalYear: "2025" },
    ]);
    const actions = mountInstrument(feature, { operation: "corporate_actions" });
    const row = actions.get("tbody tr");
    await row.trigger("click");
    await row.trigger("dblclick");
    expect(actions.emitted("select")?.[0]?.[0]).toMatchObject({ process: "已实施" });
    expect(actions.emitted("open")?.[0]?.[0]).toMatchObject({ fiscalYear: "2025" });

    const news = mountInstrument(state(), { operation: "news", instrumentId: "US.NVDA" });
    expect(news.text()).toContain("US.NVDA · US.NVDA");
    expect(news.find(".instrument-research__toolbar button").exists()).toBe(false);
    news.getComponent({ name: "CompactInstrumentNews" }).vm.$emit("selectTarget", {
      instrumentId: "US.AMD",
    });
    expect(news.emitted("update:instrumentId")?.at(-1)).toEqual(["US.AMD"]);
  });
});
