// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";

import type { WatchlistItem, WatchlistQuote } from "../src/contracts";
import WatchlistVirtualTable from "../src/components/domain/watchlist/WatchlistVirtualTable.vue";

afterEach(() => {
  vi.unstubAllGlobals();
});

function item(index: number): WatchlistItem {
  const symbol = String(index).padStart(5, "0");
  return {
    instrumentId: `HK.${symbol}`,
    market: "HK",
    symbol,
    name: `标的 ${index}`,
    securityType: "EQUITY",
  };
}

describe("WatchlistVirtualTable", () => {
  it("renders only the visible window and reports only visible quote ids", async () => {
    const items = Array.from({ length: 2_000 }, (_, index) => item(index));
    const wrapper = mount(WatchlistVirtualTable, {
      props: { items },
      global: {
        stubs: { "v-icon": { template: "<span><slot /></span>" } },
      },
    });
    await nextTick();

    const renderedRows = wrapper.findAll(".watchlist-table__row");
    expect(renderedRows.length).toBeGreaterThan(0);
    expect(renderedRows.length).toBeLessThan(30);
    const emitted = wrapper.emitted("visible-instrument-ids") ?? [];
    const latestIds = emitted.at(-1)?.[0] as string[];
    expect(latestIds.length).toBe(renderedRows.length);
    expect(latestIds.length).toBeLessThan(30);
    expect(wrapper.get(".watchlist-table__spacer").attributes("style")).toContain(
      "104000px",
    );
  });

  it("shows partial quote errors without hiding healthy rows", () => {
    const items = [item(1), item(2)];
    const quotes = new Map<string, WatchlistQuote>([
      ["HK.00001", { instrumentId: "HK.00001", price: 12.3, previousClose: 12 }],
    ]);
    const quoteErrors = new Map([
      ["HK.00002", { instrumentId: "HK.00002", code: "NO_PERMISSION", message: "无行情权限" }],
    ]);
    const wrapper = mount(WatchlistVirtualTable, {
      props: { items, quotes, quoteErrors },
      global: {
        stubs: { "v-icon": { template: "<span><slot /></span>" } },
      },
    });

    expect(wrapper.text()).toContain("12.300");
    expect(wrapper.text()).toContain("不可用");
    expect(wrapper.find('[title="无行情权限"]').exists()).toBe(true);
  });

  it("keeps the latest successful quote visible when a refresh is temporarily unavailable", () => {
    const watched = item(1);
    const wrapper = mount(WatchlistVirtualTable, {
      props: {
        items: [watched],
        quotes: new Map([
          [watched.instrumentId, {
            instrumentId: watched.instrumentId,
            price: 12.3,
            previousClose: 12,
          }],
        ]),
        quoteErrors: new Map([
          [watched.instrumentId, {
            instrumentId: watched.instrumentId,
            code: "SNAPSHOT_RATE_LIMITED",
            message: "行情额度暂时不足",
          }],
        ]),
      },
      global: {
        stubs: { "v-icon": { template: "<span><slot /></span>" } },
      },
    });

    expect(wrapper.text()).toContain("12.300");
    expect(wrapper.text()).not.toContain("不可用");
    expect(wrapper.get(".watchlist-table__quote-stale").attributes("title")).toBe(
      "数据暂未更新：行情额度暂时不足",
    );
  });

  it("uses snapshot metadata while a locally starred instrument is being enriched", () => {
    const localOnly = { ...item(3), name: undefined, securityType: undefined };
    const quotes = new Map<string, WatchlistQuote>([
      [
        localOnly.instrumentId,
        {
          instrumentId: localOnly.instrumentId,
          name: "快照名称",
          securityType: "SecurityType_Eqty",
          price: 8,
        },
      ],
    ]);
    const wrapper = mount(WatchlistVirtualTable, {
      props: { items: [localOnly], quotes },
      global: {
        stubs: { "v-icon": { template: "<span><slot /></span>" } },
      },
    });

    expect(wrapper.text()).toContain("快照名称");
    expect(wrapper.text()).toContain("SecurityType_Eqty");
  });

  it("groups Shanghai instruments under A shares while keeping the exchange tag", () => {
    const wrapper = mount(WatchlistVirtualTable, {
      props: {
        items: [
          {
            instrumentId: "SH.600519",
            market: "SH",
            symbol: "600519",
            name: "贵州茅台",
            securityType: "EQUITY",
          },
        ],
      },
      global: {
        stubs: { "v-icon": { template: "<span><slot /></span>" } },
      },
    });

    expect(wrapper.text()).toContain("贵州茅台");
    expect(wrapper.text()).toContain("600519");
    expect(wrapper.text()).toContain("上证");
    expect(wrapper.text()).toContain("沪深 · EQUITY");
    expect(wrapper.text()).not.toContain("SH.600519");
    const identity = wrapper.get(".instrument-identity--stacked");
    expect(identity.get(".instrument-identity__primary").text()).toContain(
      "上证贵州茅台",
    );
    expect(identity.get(".instrument-identity__secondary").text()).toBe(
      "600519",
    );
  });

  it("shows a leaf-market tag, name, and bare trading code on two lines", () => {
    const wrapper = mount(WatchlistVirtualTable, {
      props: {
        items: [
          {
            instrumentId: "US.AAPL",
            market: "US",
            symbol: "AAPL",
            name: "Apple",
            securityType: "EQUITY",
          },
        ],
      },
      global: {
        stubs: { "v-icon": { template: "<span><slot /></span>" } },
      },
    });

    const identity = wrapper.get(".instrument-identity--stacked");
    expect(identity.get(".instrument-identity__exchange-tag").text()).toBe(
      "美股",
    );
    expect(identity.get(".instrument-identity__title").text()).toBe("Apple");
    expect(identity.get(".instrument-identity__secondary").text()).toBe(
      "AAPL",
    );
    expect(identity.attributes("title")).toBe("US.AAPL");
  });

  it("keeps the header aligned during horizontal scrolling and exposes grid semantics", async () => {
    const wrapper = mount(WatchlistVirtualTable, {
      props: { items: [item(1)] },
      global: {
        stubs: { "v-icon": { template: "<span><slot /></span>" } },
      },
    });
    const viewport = wrapper.get<HTMLElement>(
      '[data-testid="watchlist-virtual-viewport"]',
    );
    viewport.element.scrollLeft = 120;
    await viewport.trigger("scroll");

    expect(wrapper.get('[role="grid"]').attributes("aria-rowcount")).toBe("2");
    expect(wrapper.get(".watchlist-table__header").attributes("style")).toContain(
      "translateX(-120px)",
    );
    expect(wrapper.get(".watchlist-table__row").attributes("role")).toBe("row");
  });

  it("supports compact keyboard selection, membership editing, and near-end paging", async () => {
    const observed: HTMLElement[] = [];
    const disconnect = vi.fn();
    vi.stubGlobal("ResizeObserver", class {
      observe(element: HTMLElement) { observed.push(element); }
      disconnect = disconnect;
    });
    const watched = {
      ...item(7),
      sources: [
        { sourceId: "primary", sourceName: "行情主源" },
        { sourceId: "fallback", sourceName: "行情主源" },
        { sourceId: "backup", sourceName: "备用源" },
      ],
    };
    const wrapper = mount(WatchlistVirtualTable, {
      props: {
        items: [watched],
        compact: true,
        activeInstrumentId: watched.instrumentId,
        hasMore: true,
        quotes: new Map([
          [
            watched.instrumentId,
            {
              instrumentId: watched.instrumentId,
              price: 1000,
              previousClose: 0.1,
              change: -0.01,
              changePercent: -0.1,
              session: "REGULAR",
              updateTime: "2026-07-16T01:02:03Z",
            },
          ],
        ]),
      },
      global: { stubs: { "v-icon": { template: "<span><slot /></span>" } } },
    });
    await nextTick();

    const row = wrapper.get(".watchlist-table__row");
    expect(row.classes()).toContain("is-active");
    expect(row.attributes("style")).toContain("46px");
    expect(wrapper.text()).toContain("1000.00");
    expect(wrapper.text()).toContain("-0.10%");
    await row.trigger("click");
    await row.trigger("keydown", { key: "Enter" });
    await row.trigger("keydown", { key: " " });
    await wrapper.get(".watchlist-table__star").trigger("click");
    expect(wrapper.emitted("select")).toHaveLength(3);
    expect(wrapper.emitted("edit-membership")?.[0]).toEqual([watched]);
    expect(wrapper.emitted("end-reached")).toHaveLength(1);
    expect(observed).toHaveLength(1);
    wrapper.unmount();
    expect(disconnect).toHaveBeenCalledTimes(1);
  });

  it("distinguishes the loading and empty states while preserving a loading-more marker", async () => {
    const wrapper = mount(WatchlistVirtualTable, {
      props: { items: [], loading: true, emptyText: "没有收藏" },
      global: { stubs: { "v-icon": { template: "<span><slot /></span>" } } },
    });
    expect(wrapper.find(".watchlist-table__spinner").exists()).toBe(true);
    expect(wrapper.text()).toContain("正在加载自选股");

    await wrapper.setProps({ loading: false, loadingMore: true });
    expect(wrapper.text()).toContain("没有收藏");
    expect(wrapper.text()).toContain("点击图表标题旁的星号即可加入分组");
    expect(wrapper.text()).toContain("加载更多");
  });
});
