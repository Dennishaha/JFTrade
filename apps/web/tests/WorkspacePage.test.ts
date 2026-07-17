// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, ref } from "vue";

const workspaceMocks = vi.hoisted(() => ({
  store: null as null | {
    prefs: ReturnType<typeof ref>;
    update: (patch: Record<string, unknown>) => void;
  },
}));

vi.mock("../src/composables/useWorkspaceLayout", () => ({
  useWorkspaceViewState: () => workspaceMocks.store,
}));

import WorkspacePage from "../src/pages/WorkspacePage.vue";

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
    await wrapper.find(".valid-resize").trigger("click");
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

    expect(wrapper.find(".tv-workspace__compact-stack").exists()).toBe(true);
    expect(wrapper.find(".sidebar-stub").attributes("data-compact")).toBe("yes");
    await wrapper.find(".sidebar-selected").trigger("click");
    expect(store.prefs.value.watchlistSidebarOpen).toBe(false);
    await wrapper.get("button[aria-label='显示自选栏']").trigger("click");
    expect(store.prefs.value.watchlistSidebarOpen).toBe(true);
    await wrapper.get("button[aria-label='关闭自选栏']").trigger("click");
    expect(store.prefs.value.watchlistSidebarOpen).toBe(false);

    for (const listener of controller.listeners) listener({ matches: false });
    await wrapper.vm.$nextTick();
    expect(wrapper.find(".tv-workspace__desktop-shell").exists()).toBe(true);
    wrapper.unmount();
    expect(controller.listeners).toHaveLength(0);
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
