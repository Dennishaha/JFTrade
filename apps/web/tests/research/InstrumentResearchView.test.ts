// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ fetch: vi.fn() }));

vi.mock("../../src/composables/productFeatures", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../src/composables/productFeatures")>();
  return { ...actual, fetchProductFeature: mocks.fetch };
});

import InstrumentResearchView from "../../src/components/research/InstrumentResearchView.vue";
import { flushPromises } from "../productTestUtils";

function result(
  entries: Record<string, unknown>[],
  metadata: Record<string, unknown> = {},
) {
  return {
    provider: {
      brokerId: "futu",
      featureId: "research.instrument",
      capability: "available" as const,
      selectionReason: "explicit",
      resolvedAt: "2026-07-23T00:00:00Z",
      asOf: "2026-07-23T00:00:00Z",
    },
    asOf: "2026-07-23T00:00:00Z",
    entries,
    metadata,
  };
}

function mountView(
  operation:
    | "valuation"
    | "analyst"
    | "ownership"
    | "corporate_actions"
    | "short_interest",
) {
  return mount(InstrumentResearchView, {
    props: {
      instrumentId: "US.AAPL",
      brokerId: "futu",
      operation,
    },
    global: {
      stubs: {
        InstrumentSearchBox: {
          props: ["modelValue"],
          template: '<div class="instrument-search-stub" />',
        },
        CompactInstrumentNews: {
          template: '<div class="instrument-news-stub" />',
        },
      },
    },
  });
}

afterEach(() => {
  mocks.fetch.mockReset();
});

describe("InstrumentResearchView", () => {
  it("renders every nested valuation-detail collection with protocol labels", async () => {
    mocks.fetch.mockResolvedValue(
      result([
        {
          valuationType: "ValuationType_PE",
          trend: {
            currentValue: 24.5,
            averageValue: 20,
            avgMinus1Stddev: 15,
            avgPlus1Stddev: 25,
            valuationPercentile: 72.3,
            forwardValue: 21.8,
            historicalItems: [
              {
                timeStr: "2026-07-22",
                value: 23.8,
                plateValue: 19.2,
              },
            ],
          },
          marketDistribution: {
            sections: [{ start: 30, end: 0, number: 12 }],
            total: 500,
            ranking: 80,
            averageValue: 19.6,
            medianValue: 18.4,
          },
          plateDistribution: {
            plate: { market: 11, code: "BK100" },
            plateName: "软件服务",
            plateAverageValue: 22.1,
            plateRanking: 5,
            plateStockItemCount: 42,
            stockItems: [
              {
                security: { market: 11, code: "MSFT" },
                name: "Microsoft",
                value: 31.2,
                marketCap: 2_500_000_000_000,
              },
            ],
          },
          profitGrowthRate: {
            financialTtmMultiple: 1.7,
            marketCapMultiple: 2.1,
            yearCount: 5,
            conclusionDetailed: "盈利增速低于市值增速",
            profitData: [
              {
                periodStr: "2025/FY",
                reportDateStr: "2026-01-31",
                marketCapMultiple: 2.1,
                financeDataMultiple: 1.7,
              },
            ],
          },
        },
      ]),
    );

    const wrapper = mountView("valuation");
    await flushPromises();

    expect(mocks.fetch).toHaveBeenCalledWith(
      expect.stringContaining(
        "/api/v1/research/valuation/US.AAPL?operation=detail",
      ),
    );
    expect(wrapper.text()).toContain("估值趋势");
    expect(wrapper.text()).toContain("PE");
    expect(wrapper.text()).toContain("2026-07-22");
    expect(wrapper.text()).toContain("30 以上");
    expect(wrapper.text()).toContain("US.MSFT");
    expect(wrapper.text()).toContain("Microsoft");
    expect(wrapper.text()).toContain("2025/FY");
    expect(wrapper.text()).toContain("盈利增速低于市值增速");
  });

  it("treats analyst consensus values as percentages and shows all rating bands", async () => {
    mocks.fetch.mockResolvedValue(
      result([
        {
          lowest: 180,
          average: 220,
          highest: 260,
          rating: "ResearchRatingType_StrongBuy",
          total: 18,
          strongBuy: 10,
          buy: 55,
          hold: 20,
          underperform: 8,
          sell: 7,
          updateTimeStr: "2026-07-22",
        },
      ]),
    );

    const wrapper = mountView("analyst");
    await flushPromises();

    expect(wrapper.text()).toContain("18 位分析师");
    expect(wrapper.text()).toContain("强力推荐");
    expect(wrapper.text()).toContain("跑输大盘");
    expect(wrapper.text()).toContain("55%");
    const buyRow = wrapper
      .findAll(".instrument-research__rating-row")
      .find((row) => row.text().startsWith("买入"))!;
    expect(buyRow.get("i b").attributes("style")).toContain("width: 55%");
  });

  it("flattens shareholders overview groups with their own static dates and holderPct", async () => {
    mocks.fetch.mockResolvedValue(
      result(
        [
          {
            staticDateStr: "2026-06-30",
            itemList: [
              { name: "机构", holderPct: 68.5 },
              { name: "个人", holderPct: 31.5 },
            ],
          },
        ],
        {
          mainHolderInfoList: [
            {
              staticDateStr: "2026-03-31",
              itemList: [
                { name: "Vanguard", holderPct: 8.25, holderId: 101 },
              ],
            },
          ],
        },
      ),
    );

    const wrapper = mountView("ownership");
    await flushPromises();

    const rows = wrapper.findAll("tbody tr").map((row) => row.text());
    expect(rows).toEqual(
      expect.arrayContaining([
        expect.stringContaining("主要股东2026-03-31Vanguard8.25%"),
        expect.stringContaining("持股类型2026-06-30机构68.5%"),
        expect.stringContaining("持股类型2026-06-30个人31.5%"),
      ]),
    );
  });

  it("keeps every dividend date, plan, process and fiscal year in separate columns", async () => {
    mocks.fetch.mockResolvedValue(
      result([
        {
          pubDate: "2026/03/01",
          statement: "末期息 5.3 港元",
          process: "方案实施",
          recordDate: "2026/05/20",
          exDate: "2026/05/19",
          dividendPayableDate: "2026/06/01",
          fiscalYear: "2025",
        },
      ]),
    );

    const wrapper = mountView("corporate_actions");
    await flushPromises();

    expect(
      wrapper.findAll("thead th").map((header) => header.text()),
    ).toEqual([
      "公告日",
      "分配方案",
      "进度",
      "登记日",
      "除权除息日",
      "派息日",
      "财政年度",
    ]);
    expect(wrapper.get("tbody tr").text()).toContain("末期息 5.3 港元");
    expect(wrapper.get("tbody tr").text()).toContain("2026/06/01");
    expect(wrapper.get("tbody tr").text()).toContain("2025");
  });

  it("renders the US daily-short-volume fields without calling the daily average days-to-cover", async () => {
    mocks.fetch.mockResolvedValue(
      result([
        {
          timestampStr: "2026-07-22",
          totalSharesShort: 1_000_000,
          nasdaqSharesShort: 600_000,
          nyseSharesShort: 400_000,
          shortPercent: 32.5,
          volume: 3_000_000,
          closePrice: 210,
          lastClosePrice: 208,
          dailyTradeAvgRatio: 18.6,
        },
      ]),
    );

    const wrapper = mountView("short_interest");
    await flushPromises();

    const headers = wrapper.findAll("thead th").map((header) => header.text());
    expect(headers).toEqual([
      "日期",
      "卖空总股数",
      "NASDAQ",
      "NYSE",
      "卖空占比",
      "成交量",
      "收盘价",
      "昨收价",
      "20 日日均成交比例",
    ]);
    expect(wrapper.text()).toContain("18.6%");
    expect(wrapper.text()).not.toContain("回补天数");
  });

  it("renders HK volume and turnover fields plus aggregate short-interest metadata", async () => {
    mocks.fetch.mockResolvedValue(
      result(
        [
          {
            timestampStr: "2026-07-22",
            sharesTraded: 5_000_000,
            turnover: 105_000_000,
            shortSellSharesTraded: 900_000,
            shortSellTurnover: 18_000_000,
            openPrice: 20.5,
            closePrice: 21,
            lastClosePrice: 20,
            dailyTradeAvgRatio: 19.2,
          },
        ],
        {
          aggregatedShort: 12_000_000,
          aggregatedShortRatio: 2.4,
          newTimeStr: "2026-07-22",
        },
      ),
    );

    const wrapper = mountView("short_interest");
    await flushPromises();

    const headers = wrapper.findAll("thead th").map((header) => header.text());
    expect(headers).toContain("做空成交量");
    expect(headers).toContain("成交额");
    expect(headers).toContain("做空成交额");
    expect(headers).toContain("开盘价");
    expect(headers).not.toContain("NASDAQ");
    expect(wrapper.text()).toContain("未平仓股数");
    expect(wrapper.text()).toContain("1200.00万");
    expect(wrapper.text()).toContain("占流通股比例");
    expect(wrapper.text()).toContain("2.4%");
  });
});
