// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  fetch: vi.fn(),
  fetchWithInit: vi.fn(),
}));

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

import MarketHomeView from "../../src/components/research/MarketHomeView.vue";
import { flushPromises } from "../productTestUtils";

function featureResult(entries: Record<string, unknown>[]) {
  return {
    provider: {
      brokerId: "futu",
      featureId: "research.test",
      capability: "available" as const,
      selectionReason: "explicit",
      resolvedAt: "2026-07-17T00:00:00Z",
      asOf: "2026-07-17T00:00:00Z",
    },
    asOf: "2026-07-17T00:00:00Z",
    entries,
  };
}

function canonicalInstrument(
  instrumentId: string,
  name: string,
  extra: Record<string, unknown> = {},
) {
  const [market, symbol] = instrumentId.split(".");
  return {
    instrumentId,
    market,
    symbol,
    name,
    productClass: "equity",
    ...extra,
  };
}

function mockData(): void {
  mocks.fetch.mockImplementation((path: string) => {
    const params = new URLSearchParams(path.split("?")[1]);
    const operation = params.get("operation") ?? "";
    if (operation === "top_movers") {
      return Promise.resolve(
        featureResult([
          canonicalInstrument(
            params.get("direction") === "down" ? "US.LOSE" : "US.GAIN",
            params.get("direction") === "down" ? "领跌公司" : "领涨公司",
            { changeRate: params.get("direction") === "down" ? -5 : 6, price: 10 },
          ),
        ]),
      );
    }
    if (operation === "hot") {
      return Promise.resolve(
        featureResult([
          canonicalInstrument("US.HOT", "热门公司", { averageHeat: 88 }),
        ]),
      );
    }
    if (operation === "heatmap") {
      return Promise.resolve(
        featureResult([
          {
            instrumentId: "US.BK100",
            market: "US",
            symbol: "BK100",
            name: "科技",
            productClass: "plate",
            marketValue: 100,
            changeRate: 1,
          },
        ]),
      );
    }
    return Promise.resolve(featureResult([]));
  });
  mocks.fetchWithInit.mockImplementation((_path: string, init: RequestInit) => {
    const ids = JSON.parse(String(init.body)).instrumentIds as string[];
    const entries = ids.map((instrumentId) => ({
      symbol: instrumentId,
      name: instrumentId === "US.HOT" ? "热门公司" : undefined,
      productClass:
        instrumentId.startsWith("HK.8") ||
        instrumentId.startsWith("SH.") ||
        instrumentId.startsWith("SZ.")
          ? "index"
          : "equity",
      lastPrice: instrumentId === "US.HOT" ? 30 : 100,
      previousClose: instrumentId === "US.HOT" ? 29 : 99,
      turnover: 1000,
      observedAt: "2026-07-17T00:00:00Z",
    }));
    return Promise.resolve(featureResult(entries));
  });
}

afterEach(() => {
  mocks.fetch.mockReset();
  mocks.fetchWithInit.mockReset();
});

describe("MarketHomeView", () => {
  it("shows the real US index capability limitation and uses real ranking operations", async () => {
    mockData();
    const wrapper = mount(MarketHomeView, {
      props: { market: "US", brokerId: "futu" },
    });
    await flushPromises();

    expect(wrapper.findAll(".market-home-view__index-card")).toHaveLength(0);
    expect(wrapper.text()).toContain("美股指数快照暂不可用");
    expect(wrapper.text()).toContain("不会用 ETF 代理或伪造指数卡片");
    expect(wrapper.text()).toContain("领涨榜");
    expect(wrapper.text()).toContain("领跌榜");
    expect(wrapper.text()).toContain("热度榜");
    expect(wrapper.text()).not.toContain("高股息");
    expect(wrapper.text()).toContain("热门公司");
    expect(wrapper.text()).toContain("88.00");

    const paths = mocks.fetch.mock.calls.map(([path]) => String(path));
    expect(paths.some((path) => path.includes("operation=rise_fall_distribution"))).toBe(false);
    expect(paths).toEqual(
      expect.arrayContaining([
        expect.stringContaining("operation=top_movers"),
        expect.stringContaining("direction=up"),
        expect.stringContaining("direction=down"),
        expect.stringContaining("operation=hot"),
        expect.stringContaining("operation=heatmap"),
      ]),
    );
    expect(mocks.fetchWithInit).toHaveBeenCalledWith(
      expect.stringContaining("brokerId=futu"),
      expect.objectContaining({ method: "POST" }),
    );
    for (const [, init] of mocks.fetchWithInit.mock.calls) {
      expect(String((init as RequestInit).body)).not.toMatch(/US\.(DJI|SPX|IXIC)/);
    }
  });

  it("emits quoteable canonical entries and routes more to rankings", async () => {
    mockData();
    const wrapper = mount(MarketHomeView, { props: { market: "US" } });
    await flushPromises();

    const hotPanel = wrapper
      .findAll(".rank-list-panel")
      .find((panel) => panel.text().includes("热度榜"))!;
    await hotPanel.get("tbody tr").trigger("click");
    expect(wrapper.emitted("select")?.[0]?.[0]).toMatchObject({
      instrumentId: "US.HOT",
      productClass: "equity",
    });
    await wrapper.findAll(".market-home-view__more")[2]!.trigger("click");
    expect(wrapper.emitted("more")).toContainEqual(["hot"]);
  });

  it("does not call unsupported movers/hot for CN", async () => {
    mockData();
    const wrapper = mount(MarketHomeView, { props: { market: "CN" } });
    await flushPromises();
    const paths = mocks.fetch.mock.calls.map(([path]) => String(path));
    expect(paths.some((path) => path.includes("operation=top_movers"))).toBe(false);
    expect(paths.some((path) => path.includes("operation=hot"))).toBe(false);
    expect(paths.some((path) => path.includes("operation=high_dividend_state"))).toBe(false);
    expect(wrapper.text()).toContain("仅支持美股、港股");
    expect(wrapper.text()).toContain("不展示错配数据");
  });

  it("uses the dedicated high-dividend protocol only for its real HK result set", async () => {
    mockData();
    const wrapper = mount(MarketHomeView, { props: { market: "HK" } });
    await flushPromises();
    const paths = mocks.fetch.mock.calls.map(([path]) => String(path));
    expect(paths).toEqual(expect.arrayContaining([
      expect.stringContaining("market=HK"),
      expect.stringContaining("operation=high_dividend_state"),
    ]));
    expect(wrapper.text()).toContain("高股息");
  });

  it("opens HK benchmarks, routes every ranking shortcut, and changes heatmap types", async () => {
    mockData();
    const wrapper = mount(MarketHomeView, {
      props: { market: "HK", brokerId: "futu" },
    });
    await flushPromises();
    await vi.waitFor(() => {
      expect(wrapper.findAll(".market-home-view__index-card")).toHaveLength(3);
    });
    await wrapper.get(".market-home-view__index-card").trigger("click");
    expect(wrapper.emitted("select")?.at(-1)?.[0]).toMatchObject({
      instrumentId: "HK.800000",
      productClass: "index",
    });

    for (const button of wrapper.findAll(".market-home-view__more")) {
      await button.trigger("click");
    }
    expect(wrapper.emitted("more")).toEqual([
      ["top_gainers"],
      ["top_losers"],
      ["hot"],
      ["high_dividend_state"],
    ]);

    const heatmapButtons = wrapper.findAll(
      ".market-home-view__heatmap-head button",
    );
    await heatmapButtons.find((button) => button.text() === "概念")!.trigger("click");
    await heatmapButtons.find((button) => button.text() === "主题")!.trigger("click");
    await flushPromises();
    expect(mocks.fetch).toHaveBeenCalledWith(
      expect.stringContaining("plateType=theme"),
    );
  });

  it("surfaces benchmark snapshot failures for supported markets", async () => {
    mockData();
    mocks.fetchWithInit.mockRejectedValue(new Error("指数快照权限不足"));
    const wrapper = mount(MarketHomeView, { props: { market: "HK" } });
    await vi.waitFor(() => {
      expect(wrapper.text()).toContain("指数快照权限不足");
    });
  });
});
