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
    props: {
      market: String,
      brokerId: String,
      operation: String,
      instrumentId: String,
      institutionId: String,
      seriesCode: String,
      eventCode: String,
      contractCode: String,
      contractView: String,
    },
    emits: [
      "select",
      "open",
      "more",
      "update:institutionId",
      "update:seriesCode",
      "update:eventCode",
      "update:contractCode",
      "update:contractView",
    ],
    template: `
      <div
        class="view-stub"
        :data-view="$options.name"
        :data-market="market"
        :data-broker-id="brokerId"
        :data-operation="operation"
        :data-institution-id="institutionId"
        :data-series-code="seriesCode"
        :data-event-code="eventCode"
        :data-contract-code="contractCode"
        :data-contract-view="contractView"
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
        <button class="emit-institution" @click="$emit('update:institutionId', '202')">institution</button>
        <button class="clear-institution" @click="$emit('update:institutionId', '')">clear institution</button>
        <button class="emit-series" @click="$emit('update:seriesCode', 'SERIES.2')">series</button>
        <button class="emit-event" @click="$emit('update:eventCode', 'EVENT.2')">event</button>
        <button class="emit-contract" @click="$emit('update:contractCode', 'EC.2')">contract</button>
        <button class="emit-contract-view" @click="$emit('update:contractView', 'depth')">contract view</button>
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
  DividendCalendarView: viewStub("DividendCalendarView"),
  InstitutionGridView: viewStub("InstitutionGridView"),
  StockScreenerView: viewStub("StockScreenerView"),
  MacroResearchView: viewStub("MacroResearchView"),
  ArkResearchView: viewStub("ArkResearchView"),
  IndustryChainView: viewStub("IndustryChainView"),
  InstrumentResearchView: viewStub("InstrumentResearchView"),
  DerivativeScreenView: viewStub("DerivativeScreenView"),
  QuoteDetailRail: quoteDetailRailStub,
  ProductFeaturePanel: featurePanelStub,
  OptionResearchPanel: defineComponent({ template: `<div class="option-stub" />` }),
  PredictionResearchPanel: viewStub("PredictionResearchPanel"),
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
      page.get(".research-page__tabs").findAll("button").map((item) => item.text()),
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
    expect(center.find(".product-panel-toolbar").exists()).toBe(false);
    const navigation = center.get(".research-page__navigation");
    const navigationActions = navigation.get(
      ".research-page__navigation-actions",
    );
    expect(navigation.find(".broker-provider-tag-stub").exists()).toBe(true);
    expect(navigationActions.element.children).toHaveLength(2);
    expect(navigationActions.element.children[0]?.classList).toContain(
      "broker-provider-tag-stub",
    );
    expect(navigationActions.element.children[1]?.classList).toContain(
      "research-page__rail-toggle",
    );
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
    expect(
      page.get(".research-page__section-operations .is-active").text(),
    ).toBe("财报日历");
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
    expect(page.find(".rail-stub").exists()).toBe(false);
    expect(page.get(".research-page__rail-toggle").attributes("title")).toBe(
      "展开行情详情",
    );
  });

  it("opens quoteable stock-screen results in the shared rail", async () => {
    const { page, router } = await mountResearchPage("section=screens&operation=stock_v2");
    expect(page.find(".rail-stub").exists()).toBe(true);
    expect(page.get(".view-stub").attributes("data-view")).toBe(
      "StockScreenerView",
    );
    await page.get(".emit-select").trigger("click");
    await nextTick();
    expect(page.get(".rail-stub").attributes("data-target-id")).toBe("US.AAPL");
    expect(page.get(".rail-stub").attributes("data-entry-name")).toBe("测试证券");
    expect(router.currentRoute.value.path).toBe("/research");
  });

  it("keeps news inside the individual-instrument research domain", async () => {
    const { page } = await mountResearchPage("section=instrument&operation=news");
    const view = page.get(".view-stub");
    expect(view.attributes("data-view")).toBe("InstrumentResearchView");
    expect(view.attributes("data-operation")).toBe("news");
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

  it("drives institution details from the URL and restores them through history", async () => {
    const { page, router } = await mountResearchPage(
      "section=institutions&operation=list&mkt=US&institutionId=101",
    );
    expect(page.get(".view-stub").attributes("data-institution-id")).toBe("101");

    await page.get(".emit-institution").trigger("click");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.institutionId).toBe("202");
    });

    await router.push({
      query: {
        ...router.currentRoute.value.query,
        institutionId: "303",
      },
    });
    expect(page.get(".view-stub").attributes("data-institution-id")).toBe("303");
    router.back();
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.institutionId).toBe("202");
      expect(page.get(".view-stub").attributes("data-institution-id")).toBe(
        "202",
      );
    });

    await page.get(".clear-institution").trigger("click");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.institutionId).toBeUndefined();
    });
  });

  it("drives prediction contract and view state from the URL", async () => {
    const { page, router } = await mountResearchPage(
      "section=prediction&operation=categories&seriesCode=SERIES.1&eventCode=EVENT.1&contractCode=EC.1&contractView=ticks",
    );
    const view = page.get(".view-stub");
    expect(view.attributes("data-view")).toBe("PredictionResearchPanel");
    expect(view.attributes("data-series-code")).toBe("SERIES.1");
    expect(view.attributes("data-event-code")).toBe("EVENT.1");
    expect(view.attributes("data-contract-code")).toBe("EC.1");
    expect(view.attributes("data-contract-view")).toBe("ticks");

    await page.get(".emit-contract-view").trigger("click");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.contractView).toBe("depth");
    });
    await page.get(".emit-contract").trigger("click");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.contractCode).toBe("EC.2");
      expect(router.currentRoute.value.query.contractView).toBe("snapshot");
    });

    await router.push({
      query: {
        ...router.currentRoute.value.query,
        contractCode: "EC.3",
        contractView: "milestones",
      },
    });
    expect(page.get(".view-stub").attributes("data-contract-code")).toBe("EC.3");
    router.back();
    await vi.waitFor(() => {
      expect(page.get(".view-stub").attributes("data-contract-code")).toBe(
        "EC.2",
      );
      expect(page.get(".view-stub").attributes("data-contract-view")).toBe(
        "snapshot",
      );
    });
  });

  it("cleans incompatible deep-link context on section and operation changes", async () => {
    const prediction = await mountResearchPage(
      "section=prediction&operation=categories&instrumentId=US.AAPL&institutionId=101&seriesCode=SERIES.1&eventCode=EVENT.1&contractCode=EC.1&contractView=depth",
    );
    prediction.page
      .getComponent({ name: "VTabs" })
      .vm.$emit("update:modelValue", "calendar");
    await vi.waitFor(() => {
      expect(prediction.router.currentRoute.value.query.section).toBe(
        "calendar",
      );
    });
    expect(prediction.router.currentRoute.value.query).not.toHaveProperty(
      "instrumentId",
    );
    expect(prediction.router.currentRoute.value.query).not.toHaveProperty(
      "institutionId",
    );
    expect(prediction.router.currentRoute.value.query).not.toHaveProperty(
      "seriesCode",
    );
    expect(prediction.router.currentRoute.value.query).not.toHaveProperty(
      "eventCode",
    );
    expect(prediction.router.currentRoute.value.query).not.toHaveProperty(
      "contractCode",
    );
    expect(prediction.router.currentRoute.value.query).not.toHaveProperty(
      "contractView",
    );

    const institution = await mountResearchPage(
      "section=institutions&operation=list&mkt=US&institutionId=101",
    );
    await institution.page
      .get(".research-page__section-operations")
      .findAll("button")
      .find((button) => button.text() === "持仓变化")!
      .trigger("click");
    await vi.waitFor(() => {
      expect(institution.router.currentRoute.value.query.operation).toBe(
        "holding_changes",
      );
      expect(institution.router.currentRoute.value.query.institutionId).toBe(
        "101",
      );
    });
    await institution.page
      .get(".research-page__section-operations")
      .findAll("button")
      .find((button) => button.text() === "ARK 持仓")!
      .trigger("click");
    await vi.waitFor(() => {
      expect(institution.router.currentRoute.value.query.operation).toBe(
        "ark_fund_holdings",
      );
      expect(
        institution.router.currentRoute.value.query.institutionId,
      ).toBeUndefined();
    });
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
    expect(page.findAll(".research-page__rail-toggle")).toHaveLength(1);
    let railToggle = page.get(".research-page__rail-toggle");
    expect(railToggle.text()).toBe("");
    expect(
      railToggle.get(".research-page__rail-toggle-icon").attributes(
        "data-direction",
      ),
    ).toBe("right");
    expect(railToggle.attributes("title")).toBe("收起行情详情");
    expect(railToggle.attributes("aria-label")).toBe("收起行情详情");
    await railToggle.trigger("click");
    expect(page.find(".research-page__market-rail").exists()).toBe(false);
    expect(page.find(".rail-stub").exists()).toBe(false);
    expect(page.find(".splitpanes__splitter").exists()).toBe(false);
    expect(page.findAll(".research-page__rail-toggle")).toHaveLength(1);
    railToggle = page.get(".research-page__rail-toggle");
    expect(railToggle.text()).toBe("");
    expect(
      railToggle.get(".research-page__rail-toggle-icon").attributes(
        "data-direction",
      ),
    ).toBe("left");
    expect(railToggle.attributes("title")).toBe("展开行情详情");
    expect(railToggle.attributes("aria-label")).toBe("展开行情详情");
    await railToggle.trigger("click");
    expect(page.find(".research-page__market-rail").exists()).toBe(true);
    expect(page.find(".rail-stub").exists()).toBe(true);
    expect(page.find(".splitpanes__splitter").exists()).toBe(true);
    expect(page.findAll(".research-page__rail-toggle")).toHaveLength(1);
    expect(
      page
        .get(".research-page__rail-toggle-icon")
        .attributes("data-direction"),
    ).toBe("right");
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
    expect(
      second.page
        .get(".research-page__rail-toggle-icon")
        .attributes("data-direction"),
    ).toBe("left");
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
      selectResearchEntry: (entry: unknown) => void;
      openResearchEntry: (
        entry: unknown,
        productClassHint?: "option" | "equity" | "unknown",
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
    state.selectResearchEntry(null);
    state.selectResearchEntry({
      instrumentId: "US.MSFT",
      market: "US",
      symbol: "MSFT",
      productClass: "equity",
    });
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.quote).toBe("US.MSFT");
    });
    expect(page.get(".rail-stub").attributes("data-target-id")).toBe("US.MSFT");

    const pushSpy = vi.spyOn(router, "push");
    state.openResearchEntry(
      { instrumentId: "US.OPT", productClass: "option" },
      "option",
    );
    expect(pushSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        path: "/workspace",
        query: expect.objectContaining({ marketSegment: "derivatives" }),
      }),
    );

    await router.push("/research?section=market&mkt=US");
    state.openResearchEntry({});
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

  it("routes dividend research to the dedicated calendar view", async () => {
    const { page } = await mountResearchPage(
      "section=calendar&operation=dividends&mkt=HK",
    );
    const view = page.get(".view-stub");
    expect(view.attributes("data-view")).toBe("DividendCalendarView");
    expect(view.attributes("data-market")).toBe("HK");
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
    expect(
      mounted.page
        .get(".research-page__navigation-actions")
        .find(".broker-provider-tag-stub")
        .exists(),
    ).toBe(true);
    expect(
      mounted.page
        .get(".research-page__rail-toggle-icon")
        .attributes("data-direction"),
    ).toBe("left");

    await mounted.page.get(".emit-select").trigger("click");
    await flushPromises();
    expect(mounted.page.get(".research-page__shell").classes()).toContain(
      "is-drawer",
    );
    expect(mounted.page.find(".research-page__rail-backdrop").exists()).toBe(
      true,
    );
    expect(
      mounted.page
        .get(".research-page__rail-toggle-icon")
        .attributes("data-direction"),
    ).toBe("right");
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
