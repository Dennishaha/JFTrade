// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createPinia } from "pinia";
import { createMemoryHistory, createRouter } from "vue-router";
import { defineComponent, h, nextTick } from "vue";

import CompanyWorkspacePanel from "../src/components/product/CompanyWorkspacePanel.vue";
import WorkspaceProductPane from "../src/components/workspace/WorkspaceProductPane.vue";
import ResearchPage from "../src/pages/ResearchPage.vue";
import type {
  MarketSecurityDetails,
  MarketSecurityDetailsQueryResult,
} from "../src/contracts";
import {
  provideWorkspaceTradingPreferencesStore,
  type WorkspaceTradingPreferencesStore,
} from "../src/composables/useWorkspaceLayout";
import { provideConsoleDataStore } from "../src/composables/useConsoleData";
import type { MarketInstrumentReference } from "../src/composables/consoleDataSystemState";
import {
  flushPromises,
  productGlobalStubs,
  setupState,
} from "./productTestUtils";

const panelStub = defineComponent({
  name: "ProductFeaturePanel",
  props: {
    title: String,
    description: String,
    path: String,
    active: Boolean,
  },
  emits: ["openInstrument"],
  template: `
    <div>
      <button class="feature-panel" :data-path="path" @click="$emit('openInstrument', 'HK.00700')">{{ title }}</button>
      <slot name="controls" />
    </div>
  `,
});
const predictionStub = defineComponent({
  name: "PredictionResearchPanel",
  emits: ["openInstrument"],
  template: `<button class="prediction-panel" @click="$emit('openInstrument', 'US.EVENT')">prediction</button>`,
});
const optionResearchStub = defineComponent({
  name: "OptionResearchPanel",
  props: {
    market: String,
    operation: String,
    scope: String,
  },
  emits: ["openInstrument"],
  template: `<button class="option-research-panel" :data-scope="scope" :data-operation="operation" @click="$emit('openInstrument', 'US.BABA260724C80000', 'option')">option research</button>`,
});
const chartStub = defineComponent({
  name: "LightweightChart",
  template: "<div class='chart-panel'>chart</div>",
});
const optionStub = defineComponent({
  name: "OptionWorkspacePanel",
  props: {
    instrumentId: String,
    displayInstrumentId: String,
    underlyingPending: Boolean,
    market: String,
  },
  emits: ["openInstrument"],
  template: `<button class="option-panel" :data-underlying="instrumentId" :data-current="displayInstrumentId" @click="$emit('openInstrument', 'US.MSFT')">options</button>`,
});
const newsStub = defineComponent({
  name: "NewsWorkspacePanel",
  props: {
    instrumentId: String,
    path: String,
  },
  emits: ["openInstrument"],
  template: `<button class="news-panel" :data-path="path" :data-instrument="instrumentId" @click="$emit('openInstrument', 'US.MSFT')">news</button>`,
});

function marketReference(
  instrumentId: string,
  securityType: string,
): MarketInstrumentReference {
  const [market = "", symbol = ""] = instrumentId.split(".", 2);
  return {
    market,
    symbol,
    instrumentId,
    name: instrumentId,
    securityType,
    lotSize: null,
    exchange: null,
    status: null,
    source: "test",
    updatedAt: "2026-07-17T00:00:00Z",
  };
}

function securityDetailsResult(
  instrumentId: string,
  productClass: string,
  securityType: string,
): MarketSecurityDetailsQueryResult {
  const [market = "", symbol = ""] = instrumentId.split(".", 2);
  return {
    request: { market, symbol, instrumentId },
    security: {
      instrumentId,
      market,
      symbol,
      productClass,
      marketSegment:
        ["option", "warrant", "cbbc", "future"].includes(productClass)
          ? "derivatives"
          : "securities",
      securityType,
    } as MarketSecurityDetails,
    meta: {
      instrumentId,
      source: "test",
      resolvedAt: "2026-07-17T00:00:00Z",
      fromCache: false,
    },
  };
}

afterEach(() => {
  document.getElementById("workspace-provider-statusbar")?.remove();
  window.localStorage.clear();
  vi.restoreAllMocks();
});

describe("product navigation surfaces", () => {
  it("exposes every company research operation through the shared panel", async () => {
    const wrapper = mount(CompanyWorkspacePanel, {
      props: { instrumentId: "US.AAPL", market: "US" },
      global: {
        stubs: { ...productGlobalStubs, ProductFeaturePanel: panelStub },
      },
    });
    const state = setupState<{
      section: string;
      operation: string;
      activeSection: { operations: Array<{ value: string }> };
      path: string;
    }>(wrapper);
    expect(wrapper.find(".company-workspace__toolbar").exists()).toBe(false);
    expect(wrapper.find(".company-workspace__operation").exists()).toBe(true);

    const cases: Record<string, string[]> = {
      overview: [
        "profile",
        "executives",
        "executive_background",
        "operational_efficiency",
        "top_brokers",
      ],
      financials: [
        "statements",
        "revenue_breakdown",
        "earnings_price_move",
        "earnings_price_history",
      ],
      valuation: ["detail", "constituents"],
      analyst: ["consensus", "ratings", "morningstar", "changes"],
      ownership: [
        "overview",
        "changes",
        "holders",
        "institutional",
        "insider_holders",
        "insider_transactions",
        "management_changes",
      ],
      actions: ["dividends", "buybacks", "splits", "code_changes"],
      short: ["daily_volume", "short_interest"],
      news: ["search"],
    };
    for (const [section, operations] of Object.entries(cases)) {
      state.section = section;
      await nextTick();
      expect(state.operation).toBe(operations[0]);
      expect(state.activeSection.operations.map((item) => item.value)).toEqual(
        operations,
      );
      for (const operation of operations) {
        state.operation = operation;
        await nextTick();
        expect(state.path).toContain(`operation=${operation}`);
      }
    }
    expect(state.path).toContain("/api/v1/market-data/news");
    wrapper.getComponent({ name: "VTabs" }).vm.$emit(
      "update:modelValue",
      "financials",
    );
    await nextTick();
    await wrapper.get("select").setValue("revenue_breakdown");
    expect(state.path).toContain("operation=revenue_breakdown");

    await wrapper.get(".feature-panel").trigger("click");
    expect(wrapper.emitted("openInstrument")?.[0]).toEqual(["HK.00700"]);

    await wrapper.setProps({ instrumentId: "" });
    expect(state.path).toBe("");
  });

  it("routes all research sections and opens results in the workspace", async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/research", component: ResearchPage },
        { path: "/workspace", component: { template: "<div />" } },
      ],
    });
    await router.push("/research?section=macro");
    await router.isReady();
    let trading: WorkspaceTradingPreferencesStore | null = null;
    const Host = defineComponent({
      setup() {
        trading = provideWorkspaceTradingPreferencesStore();
        provideConsoleDataStore(trading);
        return () => h(ResearchPage);
      },
    });
    const wrapper = mount(Host, {
      global: {
        plugins: [createPinia(), router],
        stubs: {
          ...productGlobalStubs,
          ProductFeaturePanel: panelStub,
          PredictionResearchPanel: predictionStub,
          OptionResearchPanel: optionResearchStub,
        },
      },
    });
    const page = wrapper.getComponent(ResearchPage);
    const state = setupState<{
      activeSection: string;
      activeOperation: string;
      activePath: string;
      openInstrument: (value: string) => void;
    }>(page);

    expect(state.activeSection).toBe("macro");
    expect(page.findAll(".broker-provider-tag-stub")).toHaveLength(1);
    expect(
      page.get(".broker-provider-tag-stub").attributes("data-feature-id"),
    ).toBe("research.macro");
    expect(page.get(".broker-provider-tag-stub").attributes("data-market")).toBe(
      "US",
    );
    state.activeSection = "screens";
    await nextTick();
    state.activeOperation = "warrant";
    await nextTick();
    expect(
      page.get(".broker-provider-tag-stub").attributes("data-feature-id"),
    ).toBe("derivatives.warrants");
    expect(page.get(".broker-provider-tag-stub").attributes("data-market")).toBe(
      "HK",
    );
    state.activeSection = "macro";
    await nextTick();
    page.getComponent({ name: "VTabs" }).vm.$emit(
      "update:modelValue",
      "institutions",
    );
    await nextTick();
    await page.get("select").setValue("ark_transactions");
    expect(state.activeOperation).toBe("ark_transactions");
    const sections = [
      "market",
      "screens",
      "options",
      "calendar",
      "macro",
      "institutions",
      "industries",
    ];
    for (const section of sections) {
      await router.replace({ path: "/research", query: { section } });
      await nextTick();
      expect(state.activePath).toContain("/api/v1/");
      expect(router.currentRoute.value.query.section).toBe(section);
      if (section === "options") {
        expect(page.get(".option-research-panel").attributes("data-scope")).toBe(
          "market",
        );
      }
    }

    state.activeSection = "prediction";
    await nextTick();
    await router.isReady();
    expect(wrapper.find(".prediction-panel").exists()).toBe(true);
    await wrapper.get(".prediction-panel").trigger("click");
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(router.currentRoute.value.path).toBe("/workspace");
    if (trading == null) throw new Error("trading preferences were not provided");
    expect(trading.prefs.value).toMatchObject({ market: "US", symbol: "EVENT" });

    await router.push("/research?section=invalid");
    await nextTick();
    expect(state.activeSection).toBe("market");
    state.openInstrument("INVALID");
  });

  it("keeps workspace product tabs broker-neutral and reuses trading preferences", async () => {
    const providerTarget = document.createElement("span");
    providerTarget.id = "workspace-provider-statusbar";
    document.body.appendChild(providerTarget);
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/workspace", component: WorkspaceProductPane }],
    });
    await router.push("/workspace?tab=warrants");
    await router.isReady();
    let trading: WorkspaceTradingPreferencesStore | null = null;
    let consoleData: ReturnType<typeof provideConsoleDataStore> | null = null;
    const Host = defineComponent({
      setup() {
        trading = provideWorkspaceTradingPreferencesStore();
        trading.update({ market: "US", symbol: "AAPL" });
        consoleData = provideConsoleDataStore(trading);
        consoleData.marketInstrumentReferences.value = [
          marketReference("US.AAPL", "Eqty"),
        ];
        return () => h(WorkspaceProductPane);
      },
    });
    const wrapper = mount(Host, {
      global: {
        plugins: [createPinia(), router],
        stubs: {
          ...productGlobalStubs,
          ProductFeaturePanel: panelStub,
          LightweightChart: chartStub,
          OptionWorkspacePanel: optionStub,
          NewsWorkspacePanel: newsStub,
          CompanyWorkspacePanel: panelStub,
        },
      },
    });
    const pane = wrapper.getComponent(WorkspaceProductPane);
    const state = setupState<{
      activeTab: string;
      instrumentID: string;
      featurePath: string;
      tabs: Array<{ value: string; label: string }>;
      openInstrument: (value: string) => void;
    }>(pane);
    await flushPromises();
    expect(pane.findAll(".broker-provider-tag-stub")).toHaveLength(0);
    expect(providerTarget.querySelectorAll(".broker-provider-tag-stub")).toHaveLength(1);
    expect(
      providerTarget
        .querySelector(".broker-provider-tag-stub")
        ?.getAttribute("data-feature-id"),
    ).toBe("market.candles");
    expect(
      providerTarget
        .querySelector(".broker-provider-tag-stub")
        ?.getAttribute("data-market"),
    ).toBe("US");
    expect(
      providerTarget
        .querySelector(".broker-provider-tag-stub")
        ?.getAttribute("data-menu-location"),
    ).toBe(
      "top end",
    );
    expect(state.activeTab).toBe("chart");
    expect(state.instrumentID).toBe("US.AAPL");
    expect(state.featurePath).toBe("");
    expect(state.tabs.map((tab) => tab.value)).not.toContain("warrants");
    expect(state.tabs.map((tab) => tab.value)).not.toContain("futures");
    state.activeTab = "warrants";
    expect(state.featurePath).toBe("");
    state.activeTab = "chart";
    pane.getComponent({ name: "VTabs" }).vm.$emit(
      "update:modelValue",
      "futures",
    );
    expect(state.activeTab).toBe("chart");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.tab).toBe("chart");
    });

    if (trading == null || consoleData == null) {
      throw new Error("workspace stores were not provided");
    }
    trading.update({ market: "HK", symbol: "00700" });
    consoleData.marketInstrumentReferences.value = [
      marketReference("HK.00700", "Eqty"),
    ];
    await router.replace("/workspace?tab=warrants");
    await flushPromises();
    expect(state.activeTab).toBe("warrants");
    expect(state.featurePath).toContain("/market-data/warrants");
    expect(
      providerTarget
        .querySelector(".broker-provider-tag-stub")
        ?.getAttribute("data-feature-id"),
    ).toBe("derivatives.warrants");
    expect(
      providerTarget
        .querySelector(".broker-provider-tag-stub")
        ?.getAttribute("data-market"),
    ).toBe(
      "HK",
    );
    expect(state.tabs.find((tab) => tab.value === "warrants")?.label).toBe(
      "轮证",
    );

    pane.getComponent({ name: "VTabs" }).vm.$emit(
      "update:modelValue",
      "news",
    );
    await nextTick();
    expect(state.activeTab).toBe("news");
    await router.replace("/workspace?tab=news");
    await nextTick();

    for (const [tab, fragment] of [
      ["news", "/news"],
      ["company", "/research/instruments"],
      ["options", "/options/chains"],
    ]) {
      state.activeTab = tab;
      await nextTick();
      expect(state.featurePath).toContain(fragment);
    }
    state.activeTab = "chart";
    await nextTick();
    expect(state.featurePath).toBe("");
    expect(wrapper.find(".chart-panel").exists()).toBe(true);

    trading.update({ market: "HK", symbol: "HSIMAIN" });
    consoleData.marketInstrumentReferences.value = [
      marketReference("HK.HSIMAIN", "Future"),
    ];
    await router.replace("/workspace?tab=company");
    await flushPromises();
    expect(state.activeTab).toBe("chart");
    expect(state.tabs.map((tab) => tab.value)).toEqual(["chart", "news"]);
    expect(wrapper.text()).not.toContain("期货");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.tab).toBe("chart");
    });

    await router.replace("/workspace?tab=futures");
    await flushPromises();
    expect(state.activeTab).toBe("chart");
    expect(state.featurePath).not.toContain("/market-data/futures");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.tab).toBe("chart");
    });

    trading.update({ market: "HK", symbol: "00700" });
    consoleData.marketInstrumentReferences.value = [
      marketReference("HK.00700", "Eqty"),
      marketReference("US.AAPL", "Eqty"),
    ];
    await router.replace("/workspace?tab=warrants");
    await flushPromises();
    expect(state.activeTab).toBe("warrants");
    trading.update({ market: "US", symbol: "AAPL" });
    await flushPromises();
    expect(state.activeTab).toBe("chart");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.tab).toBe("chart");
    });

    state.openInstrument("HK.00700");
    await nextTick();
    expect(trading.prefs.value).toMatchObject({ market: "HK", symbol: "00700" });
    expect(state.activeTab).toBe("chart");
    state.openInstrument("INVALID");

    await router.replace("/workspace?tab=bad");
    await nextTick();
    expect(state.activeTab).toBe("chart");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.tab).toBe("chart");
    });

    trading.update({ market: "US", symbol: "AAPL260117C00200000" });
    consoleData.selectWorkspaceInstrument({
      market: "US",
      symbol: "AAPL260117C00200000",
    });
    consoleData.marketInstrumentReferences.value = [
      marketReference("US.AAPL260117C00200000", "Drvt"),
    ];
    const optionDetails = securityDetailsResult(
      "US.AAPL260117C00200000",
      "option",
      "Drvt",
    );
    optionDetails.security!.option = {
      owner: {
        instrumentId: "US.AAPL",
        market: "US",
        symbol: "AAPL",
      },
    } as MarketSecurityDetails["option"];
    consoleData.marketSecurityDetails.value = optionDetails;
    consoleData.lastDataRefreshedAt.value = 1;
    await router.replace("/workspace?tab=options");
    await flushPromises();
    expect(state.activeTab).toBe("options");
    expect(state.featurePath).toContain(
      "/options/chains/US.AAPL?pageSize=50",
    );
    expect(wrapper.get(".option-panel").attributes("data-underlying")).toBe(
      "US.AAPL",
    );
    expect(wrapper.get(".option-panel").attributes("data-current")).toBe(
      "US.AAPL260117C00200000",
    );

    await router.replace("/workspace?tab=news");
    await flushPromises();
    expect(state.activeTab).toBe("news");
    expect(state.featurePath).toContain(
      "/market-data/news?market=US&code=US.AAPL&pageSize=30",
    );
    expect(state.featurePath).not.toContain("AAPL260117C00200000");
    expect(wrapper.get(".news-panel").attributes("data-path")).toContain(
      "code=US.AAPL",
    );

    await router.replace("/workspace?tab=company");
    await flushPromises();
    expect(state.activeTab).toBe("company");
    expect(state.featurePath).toContain(
      "/research/instruments/US.AAPL?pageSize=50",
    );
    expect(state.featurePath).not.toContain("AAPL260117C00200000");
  });

  it("defers restricted tabs until product identity resolves", async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/workspace", component: WorkspaceProductPane }],
    });
    await router.push("/workspace?tab=warrants");
    await router.isReady();
    let trading: WorkspaceTradingPreferencesStore | null = null;
    let consoleData: ReturnType<typeof provideConsoleDataStore> | null = null;
    const Host = defineComponent({
      setup() {
        trading = provideWorkspaceTradingPreferencesStore();
        trading.update({ market: "HK", symbol: "00700" });
        consoleData = provideConsoleDataStore(trading);
        consoleData.marketInstrumentReferences.value = [];
        consoleData.marketSecurityDetails.value = null;
        consoleData.lastDataRefreshedAt.value = 0;
        return () => h(WorkspaceProductPane);
      },
    });
    const wrapper = mount(Host, {
      global: {
        plugins: [createPinia(), router],
        stubs: {
          ...productGlobalStubs,
          ProductFeaturePanel: panelStub,
          LightweightChart: chartStub,
          OptionWorkspacePanel: optionStub,
          NewsWorkspacePanel: newsStub,
          CompanyWorkspacePanel: panelStub,
        },
      },
    });
    const state = setupState<{
      activeTab: string;
      featurePath: string;
      productIdentityPending: boolean;
    }>(wrapper.getComponent(WorkspaceProductPane));
    expect(state.productIdentityPending).toBe(true);
    expect(state.activeTab).toBe("chart");
    expect(state.featurePath).toBe("");
    expect(router.currentRoute.value.query.tab).toBe("warrants");

    if (trading == null || consoleData == null) {
      throw new Error("workspace stores were not provided");
    }
    consoleData.marketSecurityDetails.value = securityDetailsResult(
      "HK.00700",
      "equity",
      "Eqty",
    );
    consoleData.lastDataRefreshedAt.value = 1;
    await flushPromises();
    expect(state.productIdentityPending).toBe(false);
    expect(state.activeTab).toBe("warrants");
    expect(state.featurePath).toContain("/market-data/warrants");

    trading.update({ market: "HK", symbol: "HSIMAIN" });
    await flushPromises();
    expect(state.productIdentityPending).toBe(true);
    expect(state.activeTab).toBe("chart");
    expect(state.featurePath).toBe("");
    expect(router.currentRoute.value.query.tab).toBe("warrants");

    consoleData.selectWorkspaceInstrument({
      market: "HK",
      symbol: "HSIMAIN",
    });
    consoleData.marketSecurityDetails.value = securityDetailsResult(
      "HK.HSIMAIN",
      "future",
      "Future",
    );
    consoleData.lastDataRefreshedAt.value = 1;
    await flushPromises();
    expect(state.productIdentityPending).toBe(false);
    expect(state.activeTab).toBe("chart");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.tab).toBe("chart");
    });

    consoleData.selectWorkspaceInstrument({ market: "HK", symbol: "UNKNOWN" });
    await router.replace("/workspace?tab=options");
    consoleData.lastDataRefreshedAt.value = 1;
    await flushPromises();
    expect(state.productIdentityPending).toBe(false);
    expect(state.activeTab).toBe("chart");
    await vi.waitFor(() => {
      expect(router.currentRoute.value.query.tab).toBe("chart");
    });

    consoleData.selectWorkspaceInstrument({
      market: "US",
      symbol: "OPTION-PENDING",
    });
    trading.update({ market: "US", symbol: "OPTION-PENDING" });
    consoleData.marketInstrumentReferences.value = [
      marketReference("US.OPTION-PENDING", "Drvt"),
    ];
    consoleData.marketSecurityDetails.value = null;
    consoleData.lastDataRefreshedAt.value = 0;
    await router.replace("/workspace?tab=options");
    await flushPromises();
    expect(state.activeTab).toBe("options");
    expect(state.featurePath).toBe("");
    expect(router.currentRoute.value.query.tab).toBe("options");
    expect(wrapper.get(".option-panel").attributes("data-underlying")).toBe("");

    await router.replace("/workspace?tab=news");
    await flushPromises();
    expect(state.activeTab).toBe("news");
    expect(state.featurePath).toBe("");
    expect(wrapper.get(".news-panel").attributes("data-path")).toBe("");
  });
});
