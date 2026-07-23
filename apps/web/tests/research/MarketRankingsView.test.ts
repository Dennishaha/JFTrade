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

import MarketRankingsView from "../../src/components/research/MarketRankingsView.vue";
import { flushPromises } from "../productTestUtils";

function featureResult(
  entries: Record<string, unknown>[],
  envelope: Record<string, unknown> = {},
) {
  return {
    provider: {
      brokerId: "futu",
      featureId: "research.rankings",
      capability: "available" as const,
      selectionReason: "explicit",
      resolvedAt: "2026-07-17T00:00:00Z",
      asOf: "2026-07-17T00:00:00Z",
    },
    asOf: "2026-07-17T00:00:00Z",
    entries,
    ...envelope,
  };
}

function installMocks(): void {
  mocks.fetch.mockImplementation((path: string) => {
    const params = new URLSearchParams(path.split("?")[1]);
    const operation = params.get("operation");
    const market = params.get("market") ?? "US";
    if (operation === "fund_catalog") {
      return Promise.resolve(featureResult([
        { instrumentId: `${market}.SPY`, market, symbol: "SPY", name: "股票 ETF", productClass: "fund" },
        { instrumentId: `${market}.MISC`, market, symbol: "MISC", name: "未知分类基金", productClass: "fund" },
      ]));
    }
    if (operation === "high_dividend_state") {
      const symbol = market === "SH" ? "600000" : "000001";
      return Promise.resolve(featureResult([
        { instrumentId: `${market}.${symbol}`, market, symbol, name: `${market}高股息`, productClass: "equity", dividendYieldTTM: market === "SH" ? 5 : 4 },
      ]));
    }
    return Promise.resolve(featureResult([
      { instrumentId: `${market}.AAPL`, market, symbol: "AAPL", name: "Apple", productClass: "equity", price: 200, changeRate: 2 },
    ]));
  });
  mocks.fetchWithInit.mockImplementation((_path: string, init: RequestInit) => {
    const ids = JSON.parse(String(init.body)).instrumentIds as string[];
    return Promise.resolve(featureResult(ids.map((symbol) => ({
      symbol,
      lastPrice: symbol.endsWith("SPY") ? 500 : 20,
      previousClose: symbol.endsWith("SPY") ? 490 : 20,
      turnover: symbol.endsWith("SPY") ? 1e9 : 1e6,
      fund: { assetClass: symbol.endsWith("SPY") ? "股票" : "Unknow" },
      observedAt: "2026-07-17T00:00:00Z",
    }))));
  });
}

afterEach(() => {
  mocks.fetch.mockReset();
  mocks.fetchWithInit.mockReset();
});

describe("MarketRankingsView", () => {
  it("keeps pre/after/overnight in US and removes the unsupported US dividend rank", async () => {
    installMocks();
    const wrapper = mount(MarketRankingsView, { props: { market: "US", brokerId: "futu" } });
    await flushPromises();
    const labels = wrapper
      .findAll(".market-rankings-view__operations button")
      .map((button) => button.text());
    expect(labels).toEqual(["领涨", "领跌", "热门", "盘前", "盘后", "夜盘"]);
    expect(labels).not.toContain("高股息");
    await wrapper
      .findAll(".market-rankings-view__operations button")
      .find((button) => button.text() === "盘后")!
      .trigger("click");
    await vi.waitFor(() => {
      expect(mocks.fetch).toHaveBeenCalledWith(expect.stringContaining("operation=after_hours"));
    });
  });

  it("does not issue the HK-only high-dividend request for logical CN", async () => {
    installMocks();
    const wrapper = mount(MarketRankingsView, { props: { market: "CN" } });
    await flushPromises();
    expect(wrapper.findAll(".market-rankings-view__operations button")).toHaveLength(0);
    const paths = mocks.fetch.mock.calls.map(([path]) => String(path));
    expect(paths.some((path) => path.includes("operation=high_dividend_state"))).toBe(false);
    expect(paths.some((path) => /pre_market|after_hours|overnight/.test(path))).toBe(false);
    expect(wrapper.text()).toContain("不展示错配数据");
  });

  it("offers the dedicated high-dividend state for its real HK result set", async () => {
    installMocks();
    const wrapper = mount(MarketRankingsView, { props: { market: "HK" } });
    await flushPromises();
    const button = wrapper
      .findAll(".market-rankings-view__operations button")
      .find((item) => item.text() === "高股息")!;
    await button.trigger("click");
    await vi.waitFor(() => {
      expect(mocks.fetch).toHaveBeenCalledWith(expect.stringMatching(
        /market=HK.*operation=high_dividend_state/,
      ));
    });
  });

  it("builds ETF ranks and asset classes only from catalog plus real snapshots", async () => {
    installMocks();
    const wrapper = mount(MarketRankingsView, { props: { market: "US", brokerId: "futu" } });
    await flushPromises();
    await wrapper
      .findAll(".market-rankings-view__toolbar button")
      .find((button) => button.text() === "ETF / 基金")!
      .trigger("click");
    await vi.waitFor(() => {
      expect(wrapper.text()).toContain("股票");
      expect(wrapper.text()).toContain("未分类");
    });
    expect(wrapper.text()).not.toContain("加密货币");
    expect(wrapper.text()).not.toContain("Unknow");
    expect(mocks.fetch).toHaveBeenCalledWith(
      expect.stringMatching(/operation=fund_catalog.*brokerId=futu/),
    );
    expect(mocks.fetchWithInit).toHaveBeenCalledWith(
      expect.stringContaining("brokerId=futu"),
      expect.objectContaining({ method: "POST" }),
    );
    const firstRow = wrapper.get(".rank-list-panel__table tbody tr");
    await firstRow.trigger("click");
    expect(wrapper.emitted("select")?.[0]?.[0]).toMatchObject({
      instrumentId: "US.SPY",
      productClass: "fund",
    });
  });

  it("supports direct fund mode, asset toggles, and gainers/losers ranks", async () => {
    installMocks();
    const wrapper = mount(MarketRankingsView, {
      props: {
        market: "US",
        brokerId: "futu",
        initialOperation: "fund_catalog",
      },
    });
    await flushPromises();
    expect(wrapper.text()).toContain("ETF / 基金活跃榜");

    await vi.waitFor(() => {
      expect(
        wrapper
          .findAll(".market-rankings-view__asset-map button")
          .some((button) => button.text().includes("股票")),
      ).toBe(true);
    });
    const findAssetButton = () =>
      wrapper
        .findAll(".market-rankings-view__asset-map button")
        .find((button) => button.text().includes("股票"))!;
    const assetButton = findAssetButton();
    await assetButton.trigger("click");
    expect(findAssetButton().classes()).toContain("is-active");
    await findAssetButton().trigger("click");
    expect(findAssetButton().classes()).not.toContain("is-active");

    const findFundButton = (label: string) =>
      wrapper
        .findAll(".market-rankings-view__toolbar > .tv-seg button")
        .find((button) => button.text() === label)!;
    await findFundButton("领涨").trigger("click");
    expect(wrapper.text()).toContain("ETF / 基金领涨榜");
    await findFundButton("领跌").trigger("click");
    expect(wrapper.text()).toContain("ETF / 基金领跌榜");
    expect(
      wrapper.get(".rank-list-panel__sortable").attributes("aria-sort"),
    ).toBe("ascending");
  });

  it("renders hot and losers stock presentations and falls back for unknown markets", async () => {
    installMocks();
    const wrapper = mount(MarketRankingsView, {
      props: { market: "SG", initialOperation: "" },
    });
    await flushPromises();
    const operations = wrapper.findAll(".market-rankings-view__operations button");
    await operations.find((button) => button.text() === "热门")!.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("热门榜");
    await operations.find((button) => button.text() === "领跌")!.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("领跌");
    expect(
      wrapper.get(".rank-list-panel__sortable").attributes("aria-sort"),
    ).toBe("ascending");
  });

  it("shows feature warnings, partial failures, snapshot failures, and feature errors", async () => {
    mocks.fetch.mockResolvedValue(
      featureResult(
        [{ instrumentId: "US.AAPL", name: "Apple", changeRate: 1 }],
        {
          warnings: ["服务降级"],
          partialErrors: [
            { scope: "US", code: "PARTIAL", message: "部分失败" },
          ],
        },
      ),
    );
    mocks.fetchWithInit.mockRejectedValue(new Error("行情失败"));
    const wrapper = mount(MarketRankingsView, { props: { market: "US" } });
    await vi.waitFor(() => {
      expect(wrapper.text()).toContain("行情补充失败：行情失败");
    });
    expect(wrapper.text()).toContain("服务降级");
    expect(wrapper.text()).toContain("US · 部分失败");

    mocks.fetch.mockReset();
    mocks.fetchWithInit.mockReset();
    mocks.fetch.mockRejectedValue(new Error("榜单失败"));
    const failed = mount(MarketRankingsView, { props: { market: "US" } });
    await flushPromises();
    expect(failed.text()).toContain("榜单失败");
  });

  it("loads additional ranking pages and exposes the loading state", async () => {
    let resolveMore: ((value: unknown) => void) | undefined;
    mocks.fetch
      .mockResolvedValueOnce(
        featureResult([{ instrumentId: "US.A", changeRate: 1 }], {
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
    const wrapper = mount(MarketRankingsView, { props: { market: "US" } });
    await flushPromises();
    const button = wrapper.get(".market-rankings-view__more");
    await button.trigger("click");
    expect(button.attributes("disabled")).toBeDefined();
    expect(button.text()).toContain("加载中");
    resolveMore?.(
      featureResult([{ instrumentId: "US.B", changeRate: 2 }], {
        total: 2,
        hasMore: false,
      }),
    );
    await flushPromises();
    expect(wrapper.find(".market-rankings-view__more").exists()).toBe(false);
  });
});
