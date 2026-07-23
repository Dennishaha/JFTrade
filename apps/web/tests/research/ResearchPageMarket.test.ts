// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createPinia } from "pinia";
import { createMemoryHistory, createRouter } from "vue-router";
import { defineComponent, h, nextTick } from "vue";

import ResearchPage from "../../src/pages/ResearchPage.vue";
import { provideWorkspaceTradingPreferencesStore } from "../../src/composables/useWorkspaceLayout";
import { provideConsoleDataStore } from "../../src/composables/useConsoleData";
import { flushPromises, productGlobalStubs } from "../productTestUtils";

const viewStub = (name: string) =>
  defineComponent({
    name,
    props: { market: String, brokerId: String, operation: String },
    emits: ["select", "more"],
    template: `
      <div
        class="view-stub"
        :data-view="$options.name"
        :data-market="market"
        :data-broker-id="brokerId"
        :data-operation="operation"
      >
        <button
          class="emit-select"
          @click="$emit('select', {
            instrumentId: market === 'CN' ? 'SH.600000' : market + '.AAPL',
            market: market === 'CN' ? 'SH' : market,
            symbol: market === 'CN' ? '600000' : 'AAPL',
            name: '测试证券',
            productClass: 'equity',
            price: 10,
          })"
        >select</button>
        <button class="emit-more" @click="$emit('more', 'hot')">more</button>
      </div>
    `,
  });

const quoteDetailRailStub = defineComponent({
  name: "QuoteDetailRail",
  props: {
    target: { type: Object, default: null },
    entry: { type: Object, default: null },
    brokerId: String,
    drawer: Boolean,
    period: { type: String, default: "day" },
    tab: { type: String, default: "quote" },
  },
  emits: [
    "update:period",
    "update:tab",
    "select",
    "open-workspace",
    "close",
  ],
  template: `
    <div
      class="rail-stub"
      :data-target-id="target?.instrumentId ?? ''"
      :data-entry-name="entry?.name ?? ''"
      :data-broker-id="brokerId ?? ''"
      :data-drawer="String(drawer)"
      :data-period="period"
      :data-tab="tab"
    >
      <button class="rail-period-week" @click="$emit('update:period', 'week')">week</button>
      <button class="rail-tab-news" @click="$emit('update:tab', 'news')">news</button>
      <button
        class="rail-select-related"
        @click="$emit('select', {
          kind: 'instrument', instrumentId: 'US.NVDA',
          name: 'NVIDIA', productClass: 'equity',
        })"
      >related</button>
      <button class="rail-close" @click="$emit('close')">close</button>
      <button
        class="rail-open-workspace"
        @click="target && $emit('open-workspace', target)"
      >workspace</button>
    </div>
  `,
});

const featurePanelStub = defineComponent({
  name: "ProductFeaturePanel",
  props: { path: String },
  emits: ["openInstrument"],
  template: `
    <div class="feature-panel-stub" :data-path="path">
      <button
        class="feature-open"
        @click="$emit('openInstrument', 'US.MSFT', {
          instrumentId: 'US.MSFT', market: 'US', symbol: 'MSFT',
          name: 'Microsoft', productClass: 'equity', price: 500,
        })"
      >open</button>
    </div>
  `,
});

const researchStubs = {
  MarketHomeView: viewStub("MarketHomeView"),
  MarketRankingsView: viewStub("MarketRankingsView"),
  ConceptSectorView: viewStub("ConceptSectorView"),
  EarningsCalendarView: viewStub("EarningsCalendarView"),
  EconCalendarView: viewStub("EconCalendarView"),
  IpoCenterView: viewStub("IpoCenterView"),
  InstitutionGridView: viewStub("InstitutionGridView"),
  QuoteDetailRail: quoteDetailRailStub,
  ProductFeaturePanel: featurePanelStub,
  OptionResearchPanel: defineComponent({ template: `<div class="option-stub" />` }),
  PredictionResearchPanel: defineComponent({ template: `<div class="prediction-stub" />` }),
};

async function mountResearchPage(initialQuery = "section=market") {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/research", component: ResearchPage },
      { path: "/workspace", component: { template: "<div />" } },
    ],
  });
  await router.push(`/research?${initialQuery}`);
  await router.isReady();
  const Host = defineComponent({
    setup() {
      const trading = provideWorkspaceTradingPreferencesStore();
      provideConsoleDataStore(trading);
      return () => h(ResearchPage);
    },
  });
  const wrapper = mount(Host, {
    global: {
      plugins: [createPinia(), router],
      stubs: { ...productGlobalStubs, ...researchStubs },
    },
  });
  await flushPromises();
  return { wrapper, router, page: wrapper.getComponent(ResearchPage) };
}

afterEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("ResearchPage information architecture and quote rail", () => {
  it("shows only 总览 / 榜单 / 板块 under the nine research domains", async () => {
    const { page } = await mountResearchPage();
    expect(page.get(".view-stub").attributes("data-view")).toBe("MarketHomeView");
    expect(
      page.get(".research-page__market-views").findAll("button").map((item) => item.text()),
    ).toEqual(["总览", "榜单", "板块"]);
    expect(
      page.get(".research-page__navigation").findAll("button").map((item) => item.text()),
    ).toEqual([
      "市场", "筛选器", "衍生品", "日历", "宏观", "机构", "产业链", "个股研究", "预测市场",
    ]);
    expect(page.find(".research-page__capabilities").exists()).toBe(false);
    expect(page.get(".rail-stub").attributes("data-target-id")).toBe("");
  });

  it("places the complete research center and quote rail in sibling panes", async () => {
    const { page } = await mountResearchPage();
    const shell = page.get(".research-page__shell");
    const panes = shell.findAll(".splitpanes__pane");
    const center = page.get(".research-page__center");
    const rail = page.get(".research-page__market-rail");

    expect(panes).toHaveLength(2);
    expect(center.element.parentElement).toBe(panes[0]!.element);
    expect(rail.element.parentElement).toBe(panes[1]!.element);
    expect(center.find(".product-panel-toolbar").exists()).toBe(true);
    expect(center.find(".research-page__navigation").exists()).toBe(true);
    expect(center.find(".research-page__market-nav").exists()).toBe(true);
    expect(center.find(".research-page__body").exists()).toBe(true);
    expect(
      page.get(".research-page__body").find(".research-page__market-rail").exists(),
    ).toBe(false);
  });

  it("switches market views and uses the industry capability for sectors", async () => {
    const { page, router } = await mountResearchPage();
    await page
      .get(".research-page__market-views")
      .findAll("button")
      .find((button) => button.text() === "板块")!
      .trigger("click");
    await nextTick();
    expect(page.get(".view-stub").attributes("data-view")).toBe("ConceptSectorView");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.view).toBe("sectors");
    });
    expect(page.get(".broker-provider-tag-stub").attributes("data-feature-id")).toBe(
      "research.industry",
    );
  });

  it("keeps a selected capability domain instead of racing back to market", async () => {
    const { page, router } = await mountResearchPage();
    page.getComponent({ name: "VTabs" }).vm.$emit("update:modelValue", "calendar");
    await flushPromises();
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.section).toBe("calendar");
      expect(router.currentRoute.value.query.operation).toBe("earnings");
    });
    expect(page.get(".research-page__context strong").text()).toBe("日历");
    expect(page.get(".view-stub").attributes("data-view")).toBe(
      "EarningsCalendarView",
    );
  });

  it("redirects legacy market calendar links to their canonical domain", async () => {
    const { page, router } = await mountResearchPage("section=market&view=economy&mkt=HK");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.section).toBe("calendar");
      expect(router.currentRoute.value.query.operation).toBe("economic");
    });
    expect(router.currentRoute.value.query.view).toBeUndefined();
    expect(page.get(".view-stub").attributes("data-view")).toBe("EconCalendarView");
    expect(page.get(".view-stub").attributes("data-market")).toBe("HK");
    expect(page.find(".rail-stub").exists()).toBe(true);
  });

  it("redirects the old options section to derivatives", async () => {
    const { router } = await mountResearchPage("section=options&operation=warrant");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.section).toBe("derivatives");
    });
  });

  it("switches market and preserves concrete SH/SZ identities in the rail", async () => {
    const { page, router } = await mountResearchPage();
    await page
      .get(".research-page__market-codes")
      .findAll("button")
      .find((button) => button.text() === "沪深")!
      .trigger("click");
    await nextTick();
    expect(page.get(".view-stub").attributes("data-market")).toBe("CN");
    await page.get(".emit-select").trigger("click");
    await nextTick();
    expect(page.get(".rail-stub").attributes("data-target-id")).toBe("SH.600000");
    expect(page.get(".rail-stub").attributes("data-entry-name")).toBe("测试证券");
    expect(page.get(".broker-provider-tag-stub").attributes("data-market")).toBe("CN");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.mkt).toBe("CN");
      expect(router.currentRoute.value.query.quote).toBe("SH.600000");
      expect(router.currentRoute.value.query.quoteKind).toBe("instrument");
      expect(router.currentRoute.value.query.quoteClass).toBe("equity");
    });
  });

  it("restores the selected target, period, and tab from the research URL", async () => {
    const query = new URLSearchParams({
      section: "market",
      view: "rankings",
      mkt: "CN",
      quote: "SH.600519",
      quoteKind: "instrument",
      quoteClass: "equity",
      quoteName: "贵州茅台",
      quotePeriod: "week",
      quoteTab: "news",
    });
    const { page } = await mountResearchPage(query.toString());

    const rail = page.get(".rail-stub");
    expect(rail.attributes("data-target-id")).toBe("SH.600519");
    expect(rail.attributes("data-period")).toBe("week");
    expect(rail.attributes("data-tab")).toBe("news");
  });

  it("persists rail controls in the URL and carries a safe return path into workspace", async () => {
    const { page, router } = await mountResearchPage("section=market&mkt=US");
    await page.get(".emit-select").trigger("click");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.quote).toBe("US.AAPL");
    });

    await page.get(".rail-period-week").trigger("click");
    await page.get(".rail-tab-news").trigger("click");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.quotePeriod).toBe("week");
      expect(router.currentRoute.value.query.quoteTab).toBe("news");
    });

    await page.get(".rail-open-workspace").trigger("click");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.path).toBe("/workspace");
    });
    expect(router.currentRoute.value.query.tab).toBe("chart");
    expect(router.currentRoute.value.query.marketSegment).toBe("securities");
    expect(String(router.currentRoute.value.query.returnTo)).toContain(
      "/research?section=market",
    );
    expect(String(router.currentRoute.value.query.returnTo)).toContain(
      "quote=US.AAPL",
    );
    expect(String(router.currentRoute.value.query.returnTo)).toContain(
      "quotePeriod=week",
    );
  });

  it("clears stale rail data when the view changes", async () => {
    const { page } = await mountResearchPage();
    await page.get(".emit-select").trigger("click");
    expect(page.get(".rail-stub").attributes("data-target-id")).toBe("US.AAPL");
    await page
      .get(".research-page__market-views")
      .findAll("button")
      .find((button) => button.text() === "榜单")!
      .trigger("click");
    await nextTick();
    expect(page.get(".rail-stub").attributes("data-target-id")).toBe("");
  });

  it("opens quoteable ProductFeaturePanel securities in the shared rail", async () => {
    const { page, router } = await mountResearchPage("section=screens&operation=stock_v2");
    expect(page.find(".rail-stub").exists()).toBe(true);
    await page.get(".feature-open").trigger("click");
    await nextTick();
    expect(page.get(".rail-stub").attributes("data-target-id")).toBe("US.MSFT");
    expect(page.get(".rail-stub").attributes("data-entry-name")).toBe("Microsoft");
    expect(router.currentRoute.value.path).toBe("/research");
  });

  it("keeps news inside the individual-instrument research domain", async () => {
    const { page } = await mountResearchPage("section=instrument&operation=news");
    const path = page.get(".feature-panel-stub").attributes("data-path");
    expect(path).toContain("/api/v1/market-data/news?");
    expect(path).toContain("operation=search");
    expect(path).toContain("code=");
    expect(page.get(".broker-provider-tag-stub").attributes("data-feature-id")).toBe(
      "research.news",
    );
  });

  it("opens related institution holdings, while the institution view itself stays in place", async () => {
    const { page } = await mountResearchPage("section=institutions&operation=list&mkt=HK");
    expect(page.get(".view-stub").attributes("data-view")).toBe("InstitutionGridView");
    await page.get(".emit-select").trigger("click");
    expect(page.get(".rail-stub").attributes("data-target-id")).toBe("HK.AAPL");
  });

  it("routes holding changes through the institution selector", async () => {
    const { page } = await mountResearchPage(
      "section=institutions&operation=holding_changes&mkt=US",
    );
    const view = page.get(".view-stub");
    expect(view.attributes("data-view")).toBe("InstitutionGridView");
    expect(view.attributes("data-operation")).toBe("holding_changes");
    expect(page.find(".feature-panel-stub").exists()).toBe(false);
  });

  it("normalizes unavailable institution and calendar market combinations", async () => {
    const institution = await mountResearchPage("section=institutions&operation=list&mkt=CN");
    await vi.waitFor(() => {
      expect(institution.router.currentRoute.value.query.mkt).toBe("US");
    });
    expect(
      institution.page
        .get(".research-page__section-markets")
        .findAll("button")
        .map((button) => button.text()),
    ).toEqual(["美股", "港股"]);

    const calendar = await mountResearchPage("section=calendar&operation=dividends&mkt=CN");
    await vi.waitFor(() => {
      expect(calendar.router.currentRoute.value.query.operation).toBe("earnings");
    });
    expect(calendar.page.get(".view-stub").attributes("data-view")).toBe(
      "EarningsCalendarView",
    );
  });

  it("collapses and restores the shared rail", async () => {
    const { page } = await mountResearchPage("section=macro");
    expect(page.find(".splitpanes__splitter").exists()).toBe(true);
    await page.get(".research-page__rail-toggle").trigger("click");
    expect(page.find(".research-page__market-rail").exists()).toBe(false);
    expect(page.find(".rail-stub").exists()).toBe(false);
    expect(page.find(".splitpanes__splitter").exists()).toBe(false);
    await page.get(".research-page__rail-toggle").trigger("click");
    expect(page.find(".research-page__market-rail").exists()).toBe(true);
    expect(page.find(".rail-stub").exists()).toBe(true);
    expect(page.find(".splitpanes__splitter").exists()).toBe(true);
  });

  it("restores the persisted rail collapsed state on a later research mount", async () => {
    const first = await mountResearchPage("section=market");
    await first.page.get(".research-page__rail-toggle").trigger("click");
    expect(
      JSON.parse(
        window.sessionStorage.getItem("jftrade.research.view.v1") ?? "{}",
      ),
    ).toMatchObject({ railCollapsed: true, paneSizes: [72, 28] });
    first.wrapper.unmount();

    const second = await mountResearchPage("section=market");
    expect(second.page.find(".research-page__market-rail").exists()).toBe(false);
    second.wrapper.unmount();
  });

  it("covers direct navigation guards and all instrument-opening routes", async () => {
    const { page, router } = await mountResearchPage("section=market&mkt=US");
    const state = page.vm.$.setupState as unknown as {
      activeSection: string;
      activeOperation: string;
      activeMarketCode: string;
      selectSection: (value: unknown) => void;
      selectOperation: (value: unknown) => void;
      selectMarket: (value: unknown) => void;
      handleMarketPaneResized: (payload: unknown) => void;
      replacePathMarket: (path: string, market: string) => string;
      handleMarketMore: (operation: string) => void;
      rankingInitialOperation: string;
      selectQuotePeriod: (value: unknown) => void;
      selectQuoteTab: (value: unknown) => void;
      openWorkspaceInstrument: (
        instrumentId: string,
        segment?: "securities" | "derivatives" | "prediction",
        productClass?: string,
      ) => void;
      openQuoteTargetInWorkspace: (target: Record<string, unknown>) => void;
      inferredFeatureProductClass: (
        entry?: Record<string, unknown>,
      ) => string;
      openFeatureInstrument: (
        instrumentId: string,
        entry?: Record<string, unknown>,
      ) => void;
      openOptionResearchInstrument: (
        instrumentId: string,
        productClass: "option" | "equity",
      ) => void;
    };

    state.selectSection("invalid");
    state.selectOperation(state.activeOperation);
    state.selectMarket(state.activeMarketCode);
    state.handleMarketPaneResized({});
    state.handleMarketPaneResized({ panes: [{ size: 0 }, { size: 100 }] });
    state.handleMarketPaneResized({ panes: [{ size: 70 }, { size: 30 }] });
    expect(state.replacePathMarket("/api/plain", "HK")).toBe("/api/plain");
    expect(state.replacePathMarket("/api/data?market=US", "HK")).toBe(
      "/api/data?market=HK",
    );

    state.handleMarketMore("top_movers");
    await flushPromises();
    expect(state.rankingInitialOperation).toBe("top_gainers");
    state.selectQuotePeriod("invalid");
    state.selectQuoteTab("news");
    state.openWorkspaceInstrument("INVALID");
    state.openQuoteTargetInWorkspace({
      kind: "plate",
      instrumentId: "HK.BK1001",
      name: "板块",
      productClass: "plate",
    });
    expect(router.currentRoute.value.path).toBe("/research");
    expect(state.inferredFeatureProductClass({ securityType: " Fund " })).toBe(
      "fund",
    );
    expect(state.inferredFeatureProductClass({ type: "ETF" })).toBe("etf");
    expect(state.inferredFeatureProductClass()).toBe("unknown");

    const pushSpy = vi.spyOn(router, "push");
    state.openFeatureInstrument("US.OPT", {
      productClass: "option",
    });
    expect(pushSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        path: "/workspace",
        query: expect.objectContaining({ marketSegment: "derivatives" }),
      }),
    );

    await router.push("/research?section=market&mkt=US");
    state.openFeatureInstrument("US.UNKNOWN", {
      productClass: "future",
    });
    await nextTick();
    expect(router.currentRoute.value.path).toBe("/research");

    state.openOptionResearchInstrument("US.AAPL", "equity");
    await flushPromises();
    expect(page.get(".rail-stub").attributes("data-target-id")).toBe("US.AAPL");
    state.openOptionResearchInstrument("US.OPTION", "option");
    expect(pushSpy).toHaveBeenLastCalledWith(
      expect.objectContaining({
        path: "/workspace",
        query: expect.objectContaining({ marketSegment: "derivatives" }),
      }),
    );
  });

  it("selects related rail targets, forces plate tabs to quote, and closes the rail", async () => {
    const query = new URLSearchParams({
      section: "market",
      mkt: "HK",
      quote: "HK.BK1001",
      quoteKind: "plate",
      quoteClass: "plate",
      quoteTab: "news",
    });
    const { page, router } = await mountResearchPage(query.toString());
    expect(page.get(".rail-stub").attributes("data-tab")).toBe("quote");
    await page.get(".rail-select-related").trigger("click");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.quote).toBe("US.NVDA");
    });
    expect(page.get(".rail-stub").attributes("data-target-id")).toBe("US.NVDA");
    await page.get(".rail-close").trigger("click");
    expect(page.find(".rail-stub").exists()).toBe(false);
  });

  it("adds the current date to dividend calendar requests", async () => {
    const { page } = await mountResearchPage(
      "section=calendar&operation=dividends&mkt=HK",
    );
    const path = page.get(".feature-panel-stub").attributes("data-path");
    expect(path).toMatch(
      /operation=dividends.*(?:&|%26)date=\d{4}-\d{2}-\d{2}/,
    );
  });

  it("adapts the quote rail to narrow widths and cleans up observers", async () => {
    let mediaListener: ((event: { matches: boolean }) => void) | undefined;
    const removeMediaListener = vi.fn();
    Object.defineProperty(window, "matchMedia", {
      configurable: true,
      value: vi.fn(() => ({
        matches: true,
        addEventListener: (
          _name: string,
          listener: (event: { matches: boolean }) => void,
        ) => {
          mediaListener = listener;
        },
        removeEventListener: removeMediaListener,
      })),
    });
    const disconnect = vi.fn();
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(
          private readonly callback: (
            entries: Array<{ contentRect: { width: number } }>,
          ) => void,
        ) {}

        observe(): void {
          this.callback([{ contentRect: { width: 900 } }]);
        }

        disconnect(): void {
          disconnect();
        }
      },
    );
    const mounted = await mountResearchPage("section=market");
    await flushPromises();
    expect(mounted.page.find(".rail-stub").exists()).toBe(false);

    await mounted.page.get(".emit-select").trigger("click");
    await flushPromises();
    expect(mounted.page.get(".research-page__shell").classes()).toContain(
      "is-drawer",
    );
    expect(mounted.page.find(".research-page__rail-backdrop").exists()).toBe(
      true,
    );
    await mounted.page.get(".research-page__rail-backdrop").trigger("click");
    expect(mounted.page.find(".rail-stub").exists()).toBe(false);

    mediaListener?.({ matches: false });
    await mounted.page.get(".research-page__rail-toggle").trigger("click");
    expect(mounted.page.get(".research-page__shell").classes()).not.toContain(
      "is-drawer",
    );
    mounted.wrapper.unmount();
    expect(removeMediaListener).toHaveBeenCalled();
    expect(disconnect).toHaveBeenCalled();
    Reflect.deleteProperty(window, "matchMedia");
  });
});
