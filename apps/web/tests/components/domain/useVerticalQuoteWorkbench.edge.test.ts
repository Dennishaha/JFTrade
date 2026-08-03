// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  fetchEnvelope: vi.fn(),
  getWatchlistMembership: vi.fn(),
}));

vi.mock("@/composables/shared/apiClient", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/composables/shared/apiClient")>();
  return {
    ...actual,
    fetchEnvelope: mocks.fetchEnvelope,
    apiGetPath: (_template: string, path: string) => mocks.fetchEnvelope(path),
  };
});

vi.mock("@/composables/watchlist/watchlistApi", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/composables/watchlist/watchlistApi")>();
  return {
    ...actual,
    getWatchlistMembership: mocks.getWatchlistMembership,
  };
});

import { useVerticalQuoteWorkbench } from "../../../src/components/domain/market-data/useVerticalQuoteWorkbench";
import { flushPromises } from "../../productTestUtils";

afterEach(() => {
  mocks.fetchEnvelope.mockReset();
  mocks.getWatchlistMembership.mockReset();
});

function mountHarness(input: Record<string, unknown>) {
  return mount(
    defineComponent({
      setup() {
        return useVerticalQuoteWorkbench(input);
      },
      template: `
        <div>
          <span class="name">{{ name }}</span>
          <span class="price-label">{{ priceLabel }}</span>
          <span class="price">{{ lastPrice }}</span>
          <span class="amount">{{ changeAmount }}</span>
          <span class="rate">{{ changeRate }}</span>
          <span class="status">{{ statusLine }}</span>
          <span v-for="item in metrics" :key="item.label">{{ item.label }}={{ item.value }}</span>
        </div>
      `,
    }),
  );
}

describe("useVerticalQuoteWorkbench edge behavior", () => {
  it("uses the explicit target and minimal quote seed while preserving default errors", async () => {
    mocks.fetchEnvelope.mockImplementation((path: string) => {
      if (path.includes("/snapshots/")) return Promise.reject(new Error(" "));
      if (path.includes("/securities/")) return Promise.reject("offline");
      return Promise.reject(new Error(`unexpected ${path}`));
    });
    mocks.getWatchlistMembership.mockRejectedValue(new Error("watchlist down"));
    const wrapper = mountHarness({
      target: {
        kind: "instrument",
        instrumentId: "US.EDGE",
        name: "Edge Corp",
        productClass: "equity",
      },
      seed: {
        name: "Edge Corp",
        lastPrice: 12.5,
        previousClosePrice: 0,
        changeRate: -3.5,
      },
      brokerId: " FUTU ",
      visible: false,
    });
    await flushPromises();
    expect(wrapper.get(".name").text()).toBe("Edge Corp");
    expect(wrapper.get(".price").text()).toBe("12.5");
    expect(wrapper.get(".amount").text()).toBe("12.5");
    expect(wrapper.get(".rate").text()).toBe("-3.5");
    expect(wrapper.get(".status").text()).toBe("");
    expect(wrapper.text()).toContain("最高=--");
    expect(wrapper.text()).toContain("最低=--");
    expect(wrapper.text()).toContain("成交额=--");

    const state = wrapper.vm.$.setupState as unknown as {
      snapshotError: string;
      securityError: string;
      favorite: boolean;
      directionClass: (value: number | null) => string;
      formatPrice: (value: number | null) => string;
      formatSigned: (value: number | null, suffix?: string) => string;
      handleWatchlistSaved: (membership: { groupIds: string[] }) => void;
    };
    expect(state.snapshotError).toBe("行情快照加载失败");
    expect(state.securityError).toBe("证券详情加载失败");
    expect(state.directionClass(null)).toBe("");
    expect(state.directionClass(0)).toBe("");
    expect(state.directionClass(-1)).toBe("tv-down");
    expect(state.formatPrice(null)).toBe("--");
    expect(state.formatSigned(null)).toBe("--");
    expect(state.formatSigned(0, "%")).toBe("0.00%");
    expect(state.formatSigned(2)).toBe("+2.00");
    expect(state.formatSigned(-2)).toBe("-2.00");
    state.handleWatchlistSaved({ groupIds: ["default"] });
    expect(state.favorite).toBe(true);
    state.handleWatchlistSaved({ groupIds: [] });
    expect(state.favorite).toBe(false);
    wrapper.unmount();
  });

  it("returns early for an unresolved target", async () => {
    const wrapper = mountHarness({
      target: null,
      seed: { name: "贵州茅台" },
      brokerId: "futu",
      visible: false,
    });
    await flushPromises();
    const state = wrapper.vm.$.setupState as unknown as {
      refresh: () => Promise<void>;
    };
    await state.refresh();
    expect(mocks.fetchEnvelope).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("keeps Yahoo after-hours pricing in its dedicated card", async () => {
    mocks.fetchEnvelope.mockImplementation((path: string) => {
      if (path.includes("/snapshots/")) {
        return Promise.resolve({
          request: { market: "US", symbol: "AAPL", instrumentId: "US.AAPL" },
          snapshot: {
            price: 211.5,
            bid: 211.5,
            ask: 211.5,
            previousClosePrice: 210,
            lastClosePrice: 200,
            volume: 0,
            turnover: 0,
            at: "2026-07-31T00:00:00Z",
            session: "after",
            extendedHours: true,
            extended: {
              afterMarket: {
                price: 211.5,
                quoteTime: "2026-07-31T00:00:00Z",
              },
            },
          },
          meta: {
            instrumentId: "US.AAPL",
            source: "yfinance",
            brokerId: "yfinance",
            resolvedAt: "2026-07-31T00:00:15Z",
            fromCache: false,
          },
        });
      }
      if (path.includes("/securities/")) {
        return Promise.resolve({
          request: { market: "US", symbol: "AAPL", instrumentId: "US.AAPL" },
          security: { name: "Apple" },
          meta: {
            instrumentId: "US.AAPL",
            source: "yfinance",
            brokerId: "yfinance",
            resolvedAt: "2026-07-31T00:00:15Z",
            fromCache: false,
          },
        });
      }
      return Promise.reject(new Error(`unexpected ${path}`));
    });
    mocks.getWatchlistMembership.mockResolvedValue({ groupIds: [] });

    const wrapper = mountHarness({
      target: {
        kind: "instrument",
        instrumentId: "US.AAPL",
        name: "Apple",
        productClass: "equity",
      },
      visible: false,
    });
    await flushPromises();

    expect(wrapper.get(".price-label").text()).toBe("最近常规收盘");
    expect(wrapper.get(".price").text()).toBe("210");
    expect(wrapper.get(".amount").text()).toBe("10");
    expect(wrapper.text()).toContain("昨收=200");
    expect(wrapper.get(".rate").text()).toBe("5");
    const state = wrapper.vm.$.setupState as unknown as {
      extendedCards: Array<{ key: string; quoteTime: string | null }>;
    };
    expect(state.extendedCards).toHaveLength(1);
    expect(state.extendedCards[0]).toMatchObject({
      key: "after",
      quoteTime: "2026-07-31T00:00:00Z",
    });
    wrapper.unmount();
  });
});
