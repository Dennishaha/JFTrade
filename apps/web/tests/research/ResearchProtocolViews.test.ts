// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ fetch: vi.fn() }));

vi.mock("../../src/composables/productFeatures", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../src/composables/productFeatures")>();
  return { ...actual, fetchProductFeature: mocks.fetch };
});

import DerivativeScreenView from "../../src/components/research/DerivativeScreenView.vue";
import IndustryChainView from "../../src/components/research/IndustryChainView.vue";
import MacroResearchView from "../../src/components/research/MacroResearchView.vue";

function featureResult(
  entries: Record<string, unknown>[],
  metadata: Record<string, unknown> = {},
) {
  return {
    provider: {
      brokerId: "futu",
      featureId: "research.protocol",
      capability: "available" as const,
      selectionReason: "explicit",
      resolvedAt: "2026-07-23T00:00:00Z",
      asOf: "2026-07-23T00:00:00Z",
    },
    asOf: "2026-07-23T00:00:00Z",
    entries,
    metadata,
    total: entries.length,
  };
}

afterEach(() => {
  mocks.fetch.mockReset();
});

describe("research protocol-specific views", () => {
  it("maps option screen enums, YYYYMMDD dates, IV, and real protocol identities", async () => {
    mocks.fetch.mockResolvedValue(
      featureResult([
        {
          security: { market: "US", code: "AAPL270117C00200000" },
          optionName: "AAPL 2027-01-17 200 Call",
          strikeDate: 20270117,
          optionType: 1,
          impliedVolatility: 27.5,
          IV: 99,
          underlyingInfo: { stockID: 123456789, price: 200 },
          underlyingName: "不存在的旧标的名称",
          underlyingSecurity: { market: "US", code: "LEGACY" },
        },
        {
          security: { market: "US", code: "AAPL270118P00190000" },
          strikeDate: "20270118",
          optionType: 2,
          iv: 31.25,
          underlyingInfo: { stockId: "987654321" },
        },
      ]),
    );
    const wrapper = mount(DerivativeScreenView, {
      props: { operation: "option_screen", brokerId: "futu" },
    });
    await flushPromises();

    expect(wrapper.text()).toContain("看涨");
    expect(wrapper.text()).toContain("看跌");
    expect(wrapper.text()).toContain("2027-01-17");
    expect(wrapper.text()).toContain("2027-01-18");
    expect(wrapper.text()).toContain("27.5%");
    expect(wrapper.text()).toContain("31.25%");
    expect(wrapper.text()).not.toContain("99%");
    expect(wrapper.text()).toContain("Stock ID 123456789");
    expect(wrapper.text()).toContain("Stock ID 987654321");
    expect(wrapper.text()).toContain("US.AAPL270118P00190000");
    expect(wrapper.text()).not.toContain("不存在的旧标的名称");
    expect(wrapper.text()).not.toContain("US.LEGACY");
  });

  it("maps warrant protocol enums, timestamps, and security identity", async () => {
    mocks.fetch.mockResolvedValue(
      featureResult([
        {
          security: { market: "HK", code: "60001" },
          ownerSecurity: { market: "HK", code: "00700" },
          name: "腾讯法兴牛证",
          ownerName: "腾讯控股",
          warrantType: 3,
          maturityDate: 1784851200,
          currentPrice: 0.125,
        },
      ]),
    );
    const wrapper = mount(DerivativeScreenView, {
      props: { operation: "warrant", brokerId: "futu" },
    });
    await flushPromises();

    expect(wrapper.text()).toContain("牛证");
    expect(wrapper.text()).toContain("2026");
    expect(wrapper.text()).toContain("腾讯法兴牛证");
    await wrapper.get("tbody tr").trigger("click");
    expect(wrapper.emitted("select")?.[0]?.[0]).toMatchObject({
      instrumentId: "HK.60001",
      productClass: "cbbc",
    });
  });

  it("renders macro unit enums as business units instead of raw numbers", async () => {
    mocks.fetch.mockImplementation((path: string) => {
      const operation = new URLSearchParams(path.split("?")[1]).get(
        "operation",
      );
      if (operation === "indicators") {
        return Promise.resolve(
          featureResult([
            {
              categoryName: "就业",
              indicatorList: [
                { indicatorId: 101, name: "失业率" },
              ],
            },
          ]),
        );
      }
      if (operation === "indicator_history") {
        return Promise.resolve(
          featureResult([
            { dataTime: "2026-07", value: 4.5, unitType: 1 },
            { dataTime: "2026-06", value: 102.3, unitType: 3 },
          ]),
        );
      }
      return Promise.resolve(featureResult([]));
    });
    const wrapper = mount(MacroResearchView, {
      props: { operation: "indicators", brokerId: "futu" },
    });
    await flushPromises();

    expect(wrapper.text()).toContain("4.5%");
    expect(wrapper.text()).toContain("102.3（指数）");
    expect(wrapper.text()).not.toContain("4.51");
  });

  it("maps industrial-chain type enums to their Chinese semantics", async () => {
    let chainPage = 0;
    mocks.fetch.mockImplementation((path: string) => {
      const operation = new URLSearchParams(path.split("?")[1]).get(
        "operation",
      );
      if (operation === "chains" && chainPage++ === 0) {
        return Promise.resolve({
          ...featureResult([
            {
              chainId: 9,
              name: "人工智能",
              chainType: 3,
              stocksNum: 12,
            },
          ]),
          nextCursor: "chain-page-2",
          hasMore: true,
        });
      }
      return Promise.resolve(
        operation === "chains"
          ? featureResult([
              {
                chainId: 10,
                name: "机器人",
                chainType: 1,
                stocksNum: 8,
              },
            ])
          : featureResult([]),
      );
    });
    const wrapper = mount(IndustryChainView, {
      props: { market: "US", brokerId: "futu" },
    });
    await flushPromises();

    expect(wrapper.text()).toContain("上中下游型");
    expect(wrapper.text()).not.toContain(">3 ·");
    expect(String(mocks.fetch.mock.calls[0]?.[0])).toContain("pageSize=50");

    await wrapper.get(".industry-chain__load-more").trigger("click");
    await flushPromises();

    expect(wrapper.text()).toContain("机器人");
    expect(
      mocks.fetch.mock.calls.some(([path]) =>
        String(path).includes("cursor=chain-page-2"),
      ),
    ).toBe(true);
  });
});
