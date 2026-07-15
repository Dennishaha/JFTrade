// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick } from "vue";

vi.mock("../src/components/workspace/LightweightChart.vue", () => ({
  default: defineComponent({ template: "<div />" }),
}));

import WorkspacePage from "../src/pages/WorkspacePage.vue";
import {
  provideWorkspaceLayoutStore,
  type WorkspaceLayoutStore,
} from "../src/composables/useWorkspaceLayout";

afterEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("Workspace watchlist layout", () => {
  it("uses an overlay drawer on compact screens and closes it after selection", async () => {
    vi.stubGlobal(
      "matchMedia",
      vi.fn(() =>
        ({
          matches: true,
          media: "(max-width: 1180px)",
          onchange: null,
          addEventListener: vi.fn(),
          removeEventListener: vi.fn(),
          addListener: vi.fn(),
          removeListener: vi.fn(),
          dispatchEvent: vi.fn(() => true),
        }) as MediaQueryList),
    );
    let store: WorkspaceLayoutStore | null = null;
    const Host = defineComponent({
      setup() {
        store = provideWorkspaceLayoutStore();
        return () => h(WorkspacePage);
      },
    });
    const wrapper = mount(Host, {
      global: {
        stubs: {
          WorkspaceWatchlistSidebar: {
            emits: ["selected", "close"],
            template:
              "<button data-testid='sidebar-select' @click=\"$emit('selected', { instrumentId: 'US.AAPL' })\">select</button>",
          },
          PositionsPanel: { template: "<div />" },
          OrderEntryPanel: { template: "<div />" },
          InstrumentOverviewPanel: { template: "<div />" },
          OrderBookPanel: { template: "<div />" },
          "v-icon": { template: "<span><slot /></span>" },
        },
      },
    });
    await nextTick();

    expect(wrapper.find(".tv-workspace__watchlist-drawer").exists()).toBe(true);
    await wrapper.get('[data-testid="sidebar-select"]').trigger("click");
    await nextTick();

    if (store == null) throw new Error("workspace store was not provided");
    expect(store.prefs.value.watchlistSidebarOpen).toBe(false);
    expect(wrapper.find(".tv-workspace__watchlist-drawer").exists()).toBe(false);
    expect(wrapper.find(".tv-workspace__watchlist-open").exists()).toBe(true);
  });

  it("supports keyboard resizing within the persisted desktop width bounds", async () => {
    vi.stubGlobal(
      "matchMedia",
      vi.fn(() =>
        ({
          matches: false,
          media: "(max-width: 1180px)",
          onchange: null,
          addEventListener: vi.fn(),
          removeEventListener: vi.fn(),
          addListener: vi.fn(),
          removeListener: vi.fn(),
          dispatchEvent: vi.fn(() => true),
        }) as MediaQueryList),
    );
    let store: WorkspaceLayoutStore | null = null;
    const Host = defineComponent({
      setup() {
        store = provideWorkspaceLayoutStore();
        return () => h(WorkspacePage);
      },
    });
    const wrapper = mount(Host, {
      global: {
        stubs: {
          WorkspaceWatchlistSidebar: { template: "<div />" },
          PositionsPanel: { template: "<div />" },
          OrderEntryPanel: { template: "<div />" },
          InstrumentOverviewPanel: { template: "<div />" },
          OrderBookPanel: { template: "<div />" },
          SplitPane: { template: "<div><slot /></div>" },
          SplitPaneItem: { template: "<div><slot /></div>" },
          "v-icon": { template: "<span><slot /></span>" },
        },
      },
    });
    await nextTick();

    const resizer = wrapper.get('[role="separator"]');
    expect(resizer.classes()).toContain("tv-resizer--vertical");
    await resizer.trigger("keydown", { key: "ArrowRight" });
    if (store == null) throw new Error("workspace store was not provided");
    expect(store.prefs.value.watchlistSidebarWidth).toBe(290);
    await resizer.trigger("keydown", { key: "End" });
    expect(store.prefs.value.watchlistSidebarWidth).toBe(420);
    await resizer.trigger("keydown", { key: "ArrowRight" });
    expect(store.prefs.value.watchlistSidebarWidth).toBe(420);
    expect(resizer.attributes("aria-valuenow")).toBe("420");
  });
});
