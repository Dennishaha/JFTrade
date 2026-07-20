// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, onMounted, onUnmounted, ref } from "vue";

const workspaceMocks = vi.hoisted(() => ({
  store: null as null | {
    prefs: ReturnType<typeof ref>;
    update: (patch: Record<string, unknown>) => void;
  },
  tradingPrefs: null as null | ReturnType<typeof ref>,
}));

vi.mock("../src/composables/useWorkspaceLayout", () => ({
  useWorkspaceViewState: () => workspaceMocks.store,
  useWorkspaceTradingPrefs: () => ({
    prefs: workspaceMocks.tradingPrefs,
  }),
}));

import WorkspacePage from "../src/pages/WorkspacePage.vue";
import { setupState } from "./productTestUtils";

const sidebarStub = defineComponent({
  props: { compact: Boolean },
  emits: ["close", "selected"],
  template: `
    <div class="sidebar-stub" :data-compact="compact ? 'yes' : 'no'">
      <button class="sidebar-close" @click="$emit('close')">close</button>
      <button class="sidebar-selected" @click="$emit('selected')">selected</button>
    </div>
  `,
});

const splitPaneStub = defineComponent({
  emits: ["resized"],
  template: `
    <div class="split-pane-stub">
      <button class="valid-resize" @click="$emit('resized', { panes: [{ size: 61 }, { size: 39 }] })">valid</button>
      <button class="invalid-resize" @click="$emit('resized', { panes: [{ size: 0 }, { size: 100 }] })">invalid</button>
      <slot />
    </div>
  `,
});

const stubs = {
  InstrumentOverviewPanel: { template: "<div class='overview-stub' />" },
  LightweightChart: { template: "<div class='chart-stub' />" },
  OrderBookPanel: { template: "<div class='book-stub' />" },
  OrderEntryPanel: { template: "<div class='entry-stub' />" },
  PositionsPanel: { template: "<div class='positions-stub' />" },
  SplitPane: splitPaneStub,
  SplitPaneItem: { template: "<div class='split-pane-item-stub'><slot /></div>" },
  WorkspaceProductPane: { template: "<div class='product-pane-stub' />" },
  PredictionContractWorkspacePanel: {
    props: ["instrumentId", "view"],
    template:
      "<div class='prediction-contract-stub' :data-instrument='instrumentId' :data-view='view' />",
  },
  WorkspaceWatchlistSidebar: sidebarStub,
  "v-icon": { template: "<span><slot /></span>" },
};

type MediaQueryController = {
  listeners: Set<(event: { matches: boolean }) => void>;
  mediaQuery: MediaQueryList;
};

function installMatchMedia(matches: boolean, modern = true): MediaQueryController {
  const listeners = new Set<(event: { matches: boolean }) => void>();
  const mediaQuery = {
    matches,
    media: "(max-width: 1180px)",
    addEventListener: modern
      ? (_event: string, listener: (event: { matches: boolean }) => void) => listeners.add(listener)
      : undefined,
    removeEventListener: modern
      ? (_event: string, listener: (event: { matches: boolean }) => void) => listeners.delete(listener)
      : undefined,
    addListener: (listener: (event: { matches: boolean }) => void) => listeners.add(listener),
    removeListener: (listener: (event: { matches: boolean }) => void) => listeners.delete(listener),
  } as unknown as MediaQueryList;
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: vi.fn(() => mediaQuery),
  });
  return { listeners, mediaQuery };
}

function createWorkspaceStore(open = true) {
  const prefs = ref({
    rightDockOpen: false,
    rightDockTab: "notifications",
    rightDockSize: 28,
    watchlistSidebarOpen: open,
    watchlistSidebarWidth: 280,
    watchlistGroupId: null,
    paneSizes: {
      main: [72, 28],
      leftColumn: [60, 40],
      bottom: [60, 40],
      rightColumn: [60, 40],
    },
  });
  const update = vi.fn((patch: Record<string, unknown>) => {
    const paneSizes = patch.paneSizes == null
      ? prefs.value.paneSizes
      : { ...prefs.value.paneSizes, ...(patch.paneSizes as object) };
    prefs.value = { ...prefs.value, ...patch, paneSizes };
  });
  return { prefs, update };
}

beforeEach(() => {
  workspaceMocks.store = createWorkspaceStore();
  workspaceMocks.tradingPrefs = ref({
    market: "HK",
    symbol: "00700",
    marketSegment: "securities",
    productClass: "equity",
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("WorkspacePage", () => {
  it("persists valid pane and sidebar changes while rejecting invalid pane payloads", async () => {
    installMatchMedia(false);
    const store = workspaceMocks.store!;
    const wrapper = mount(WorkspacePage, { global: { stubs } });

    expect(wrapper.find(".tv-workspace__desktop-shell").exists()).toBe(true);
    for (const button of wrapper.findAll(".valid-resize")) {
      await button.trigger("click");
    }
    expect(store.update).toHaveBeenCalledWith({ paneSizes: { main: [61, 39] } });
    const callsAfterValidResize = store.update.mock.calls.length;
    await wrapper.find(".invalid-resize").trigger("click");
    expect(store.update.mock.calls).toHaveLength(callsAfterValidResize);

    const resizer = wrapper.get("[aria-label='调整自选栏宽度']");
    await resizer.trigger("keydown", { key: "ArrowLeft" });
    expect(store.prefs.value.watchlistSidebarWidth).toBe(270);
    await resizer.trigger("keydown", { key: "End" });
    expect(store.prefs.value.watchlistSidebarWidth).toBe(420);
    await resizer.trigger("keydown", { key: "Home" });
    expect(store.prefs.value.watchlistSidebarWidth).toBe(220);
    await resizer.trigger("keydown", { key: "Unrelated" });
    expect(store.prefs.value.watchlistSidebarWidth).toBe(220);

    const pointerDown = new Event("pointerdown", { bubbles: true, cancelable: true });
    Object.defineProperties(pointerDown, {
      clientX: { value: 100 },
      pointerId: { value: 7 },
    });
    resizer.element.dispatchEvent(pointerDown);
    await wrapper.vm.$nextTick();
    window.dispatchEvent(new MouseEvent("pointermove", { clientX: 720 }));
    expect(store.prefs.value.watchlistSidebarWidth).toBe(420);

    await wrapper.find(".sidebar-close").trigger("click");
    expect(store.prefs.value.watchlistSidebarOpen).toBe(false);
    await wrapper.get("button[aria-label='显示自选栏']").trigger("click");
    expect(store.prefs.value.watchlistSidebarOpen).toBe(true);
    await wrapper.find(".sidebar-selected").trigger("click");
    expect(store.prefs.value.watchlistSidebarOpen).toBe(true);
  });

  it("uses the compact drawer interactions and legacy media-query listeners", async () => {
    const controller = installMatchMedia(true, false);
    const store = workspaceMocks.store!;
    const wrapper = mount(WorkspacePage, { global: { stubs } });
    await wrapper.vm.$nextTick();
    const state = setupState<{
      startWatchlistResize: (event: PointerEvent) => void;
      handleWatchlistResizeMove: (event: PointerEvent) => void;
    }>(wrapper);

    expect(wrapper.get(".tv-workspace__desktop-shell").classes()).toContain(
      "is-compact",
    );
    expect(wrapper.find(".sidebar-stub").attributes("data-compact")).toBe("yes");
    state.startWatchlistResize(new PointerEvent("pointerdown"));
    state.handleWatchlistResizeMove(new PointerEvent("pointermove"));
    await wrapper.find(".sidebar-close").trigger("click");
    expect(store.prefs.value.watchlistSidebarOpen).toBe(false);
    await wrapper.get("button[aria-label='显示自选栏']").trigger("click");
    await wrapper.find(".sidebar-selected").trigger("click");
    expect(store.prefs.value.watchlistSidebarOpen).toBe(false);
    await wrapper.get("button[aria-label='显示自选栏']").trigger("click");
    expect(store.prefs.value.watchlistSidebarOpen).toBe(true);
    await wrapper.get("button[aria-label='关闭自选栏']").trigger("click");
    expect(store.prefs.value.watchlistSidebarOpen).toBe(false);

    for (const listener of controller.listeners) listener({ matches: false });
    await wrapper.vm.$nextTick();
    expect(wrapper.find(".tv-workspace__desktop-shell").exists()).toBe(true);
    expect(wrapper.get(".tv-workspace__desktop-shell").classes()).not.toContain(
      "is-compact",
    );
    wrapper.unmount();
    expect(controller.listeners).toHaveLength(0);
  });

  it("preserves the product pane instance across compact layout changes", async () => {
    const controller = installMatchMedia(false, true);
    const mounted = vi.fn();
    const unmounted = vi.fn();
    const ProductPane = defineComponent({
      setup() {
        onMounted(mounted);
        onUnmounted(unmounted);
        return () => h("div", { class: "product-pane-lifecycle-stub" });
      },
    });
    const wrapper = mount(WorkspacePage, {
      global: {
        stubs: {
          ...stubs,
          WorkspaceProductPane: ProductPane,
        },
      },
    });
    await wrapper.vm.$nextTick();
    expect(mounted).toHaveBeenCalledTimes(1);
    expect(unmounted).not.toHaveBeenCalled();

    for (const listener of controller.listeners) listener({ matches: true });
    await wrapper.vm.$nextTick();
    expect(wrapper.get(".tv-workspace__desktop-shell").classes()).toContain(
      "is-compact",
    );
    expect(mounted).toHaveBeenCalledTimes(1);
    expect(unmounted).not.toHaveBeenCalled();

    for (const listener of controller.listeners) listener({ matches: false });
    await wrapper.vm.$nextTick();
    expect(mounted).toHaveBeenCalledTimes(1);
    expect(unmounted).not.toHaveBeenCalled();

    wrapper.unmount();
    expect(unmounted).toHaveBeenCalledTimes(1);
  });

  it("renders prediction contract identity in place of stock overview and book", async () => {
    installMatchMedia(false);
    workspaceMocks.tradingPrefs!.value = {
      market: "US",
      symbol: "EC.HOME",
      marketSegment: "prediction",
      productClass: "event_contract",
    };
    const wrapper = mount(WorkspacePage, { global: { stubs } });
    await wrapper.vm.$nextTick();

    expect(wrapper.find(".overview-stub").exists()).toBe(false);
    expect(wrapper.find(".book-stub").exists()).toBe(false);
    const panels = wrapper.findAll(".prediction-contract-stub");
    expect(panels).toHaveLength(2);
    expect(panels[0]?.attributes("data-instrument")).toBe("US.EC.HOME");
    expect(panels.map((panel) => panel.attributes("data-view"))).toEqual([
      "contract",
      "depth",
    ]);
  });

  it("keeps a drag active across unrelated pointer-up events and unregisters modern listeners on unmount", async () => {
    const controller = installMatchMedia(false, true);
    const store = workspaceMocks.store!;
    const wrapper = mount(WorkspacePage, { global: { stubs } });
    const resizer = wrapper.get("[aria-label='调整自选栏宽度']");

    window.dispatchEvent(new MouseEvent("pointermove", { clientX: 600 }));
    expect(store.update).not.toHaveBeenCalled();

    const pointerDown = new Event("pointerdown", { bubbles: true, cancelable: true });
    Object.defineProperties(pointerDown, {
      clientX: { value: 100 },
      pointerId: { value: 7 },
    });
    resizer.element.dispatchEvent(pointerDown);
    const unrelatedPointerUp = new Event("pointerup");
    Object.defineProperty(unrelatedPointerUp, "pointerId", { value: 8 });
    window.dispatchEvent(unrelatedPointerUp);
    window.dispatchEvent(new MouseEvent("pointermove", { clientX: 180 }));
    expect(store.prefs.value.watchlistSidebarWidth).toBe(360);

    wrapper.unmount();
    expect(controller.listeners).toHaveLength(0);
  });
});
