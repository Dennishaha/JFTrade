// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  useResearchFeature: vi.fn(),
}));

vi.mock("../../src/composables/useResearchFeature", () => ({
  useResearchFeature: mocks.useResearchFeature,
}));

import PredictionContractDataView from "../../src/components/research/PredictionContractDataView.vue";

function featureState(entries: Record<string, unknown>[] = []) {
  return {
    entries: ref(entries),
    loading: ref(false),
    error: ref(""),
    asOf: ref("2026-07-24 09:30:00"),
    refresh: vi.fn(),
  };
}

beforeEach(() => {
  mocks.useResearchFeature.mockReset();
});

describe("prediction contract market-data views", () => {
  it("renders a snapshot and refreshes the exact contract path", async () => {
    const feature = featureState([
      {
        contractStatus: "OPEN",
        lastPrice: 0.62,
        yesPrice: 0.61,
        yesAsk: 0.63,
        noPrice: 0.37,
        noAsk: 0.39,
        volume24h: 12_500,
        openInterest: 8_800,
      },
    ]);
    mocks.useResearchFeature.mockReturnValue(feature);

    const wrapper = mount(PredictionContractDataView, {
      props: { path: "/contracts/EC.HOME/snapshot", view: "snapshot" },
    });
    await flushPromises();

    expect(mocks.useResearchFeature).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain("合约快照");
    expect(wrapper.text()).toContain("OPEN");
    expect(wrapper.text()).toContain("0.62");
    expect(wrapper.text()).toContain("1.25万");
    expect(wrapper.text()).toContain("更新 2026-07-24 09:30:00");
    await wrapper.get("header button").trigger("click");
    expect(feature.refresh).toHaveBeenCalledOnce();
  });

  it("shows loading and current request failures before rendering data", async () => {
    const feature = featureState();
    feature.loading.value = true;
    feature.asOf.value = "";
    mocks.useResearchFeature.mockReturnValue(feature);
    const wrapper = mount(PredictionContractDataView, {
      props: { path: "/contracts/EC.HOME/depth", view: "depth" },
    });

    expect(wrapper.text()).toContain("加载中");
    feature.loading.value = false;
    feature.error.value = "盘口暂不可用";
    await flushPromises();
    expect(wrapper.text()).toContain("盘口暂不可用");
  });

  it("keeps direct depth rows and expands grouped YES/NO order books", async () => {
    const feature = featureState([
      { side: "YES 买盘", price: 0.6, volume: 1_000, orderCount: 3 },
      { predictionSide: "NO 卖盘", orderPrice: 0.4, size: 500, orders: 2 },
    ]);
    mocks.useResearchFeature.mockReturnValue(feature);
    const wrapper = mount(PredictionContractDataView, {
      props: { path: "/contracts/EC.HOME/depth", view: "depth" },
    });

    expect(wrapper.text()).toContain("YES 买盘");
    expect(wrapper.text()).toContain("NO 卖盘");
    expect(wrapper.text()).toContain("1,000");

    feature.entries.value = [
      {
        yesBids: [{ price: 0.59, quantity: 10 }],
        yesAsks: [{ price: 0.61, quantity: 11 }],
        noBids: [{ price: 0.39, quantity: 12 }],
        noAsks: [{ price: 0.41, quantity: 13 }],
        bids: [{ price: 0.58, quantity: 14 }],
        asks: [{ price: 0.62, quantity: 15 }],
      },
    ];
    await flushPromises();
    expect(wrapper.findAll("tbody tr")).toHaveLength(6);
    expect(wrapper.text()).toContain("买盘");
    expect(wrapper.text()).toContain("卖盘");
  });

  it("flattens nested candles and ticks while preserving contract identity", async () => {
    const feature = featureState([
      {
        code: { instrumentId: "US.EC.HOME" },
        preSide: "YES",
        klineList: [
          {
            timeKey: "2026-07-24 09:30",
            openPrice: 0.5,
            highPrice: 0.7,
            lowPrice: 0.4,
            closePrice: 0.6,
            volume: 2_500,
          },
        ],
      },
      {
        code: { code: "EC.AWAY" },
        predictionSide: "NO",
        klines: [
          { timestamp: "2026-07-24 09:35", open: 0.4, high: 0.5, low: 0.3, price: 0.45 },
        ],
      },
      { time: "2026-07-24 09:40", close: 0.55 },
      { code: "invalid-contract", klineList: [null, "bad"] },
    ]);
    mocks.useResearchFeature.mockReturnValue(feature);
    const wrapper = mount(PredictionContractDataView, {
      props: { path: "/contracts/EC.HOME/candles", view: "candles" },
    });

    expect(wrapper.findAll("tbody tr")).toHaveLength(4);
    expect(wrapper.text()).toContain("2026-07-24 09:30");
    expect(wrapper.text()).toContain("2,500");

    feature.entries.value = [
      {
        code: { instrumentId: "US.EC.HOME" },
        predictionSide: "YES",
        tickerList: [{ eventTime: "09:41", yesPrice: 0.63, noPrice: 0.37, quantity: 20 }],
      },
      { code: { code: "EC.AWAY" }, ticks: [{ dateTime: "09:42", side: "NO", volume: 30 }] },
    ];
    await wrapper.setProps({ path: "/contracts/EC.HOME/ticks", view: "ticks" });
    await flushPromises();
    expect(wrapper.text()).toContain("逐笔成交");
    expect(wrapper.text()).toContain("YES");
    expect(wrapper.text()).toContain("NO");
  });

  it("renders milestone aliases and an empty result", async () => {
    const feature = featureState([
      { startDate: "2026-07-01", title: "初选", endDate: "2026-07-02", type: "DONE" },
      { eventTime: "2026-07-03", name: "决赛", category: "SPORTS" },
      { date: "2026-07-04", description: "结算", status: "PENDING" },
    ]);
    mocks.useResearchFeature.mockReturnValue(feature);
    const wrapper = mount(PredictionContractDataView, {
      props: { path: "/contracts/EC.HOME/milestones", view: "milestones" },
    });

    expect(wrapper.text()).toContain("事件里程碑");
    expect(wrapper.text()).toContain("初选");
    expect(wrapper.text()).toContain("决赛");
    expect(wrapper.text()).toContain("结算");

    feature.entries.value = [];
    await flushPromises();
    expect(wrapper.text()).toContain("暂无合约数据");
  });
});
