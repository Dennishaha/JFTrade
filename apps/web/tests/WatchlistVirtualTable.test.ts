// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import { nextTick } from "vue";

import type { WatchlistItem, WatchlistQuote } from "../src/contracts";
import WatchlistVirtualTable from "../src/components/domain/watchlist/WatchlistVirtualTable.vue";

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
});
