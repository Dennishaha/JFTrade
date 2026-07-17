// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick } from "vue";

import {
  provideWorkspaceLayoutStore,
  useWorkspaceTradingPrefs,
  useWorkspaceViewState,
  type WorkspaceLayoutStore,
} from "../src/composables/useWorkspaceLayout";

const STORAGE_KEY = "jftrade.workspace.layout.v1";
const VIEW_STORAGE_KEY = "jftrade.workspace.view.v1";
const TRADING_STORAGE_KEY = "jftrade.workspace.trading.v1";

afterEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
  vi.restoreAllMocks();
});

function mountLayoutStore() {
  let store: WorkspaceLayoutStore | null = null;
  const Host = defineComponent({
    setup() {
      store = provideWorkspaceLayoutStore();
      return () => h("div");
    },
  });

  const wrapper = mount(Host);
  if (store == null) {
    throw new Error("Workspace layout store was not provided.");
  }
  return { store, wrapper };
}

describe("useWorkspaceLayout", () => {
  it("prefers session storage over local storage", () => {
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ market: "US", symbol: "AAPL" }),
    );
    window.sessionStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ market: "HK", symbol: "00700" }),
    );

    const { store, wrapper } = mountLayoutStore();

    expect(store.prefs.value.market).toBe("HK");
    expect(store.prefs.value.symbol).toBe("00700");

    wrapper.unmount();
  });

  it("falls back to local storage when session storage is empty", () => {
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        market: "US",
        symbol: "AAPL",
        paneSizes: {
          main: [70, 30],
          leftColumn: [65, 35],
          bottom: [55, 45],
          rightColumn: [50, 50],
        },
      }),
    );

    const { store, wrapper } = mountLayoutStore();

    expect(store.prefs.value.market).toBe("US");
    expect(store.prefs.value.symbol).toBe("AAPL");
    expect(store.prefs.value.paneSizes.main).toEqual([70, 30]);

    wrapper.unmount();
  });

  it("writes updates to session and local storage", async () => {
    const { store, wrapper } = mountLayoutStore();

    store.update({
      paneSizes: {
        bottom: [55, 45],
      },
    });
    await nextTick();

    expect(
      JSON.parse(window.sessionStorage.getItem(STORAGE_KEY) ?? "{}").paneSizes
        .bottom,
    ).toEqual([55, 45]);
    expect(
      JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? "{}").paneSizes
        .bottom,
    ).toEqual([55, 45]);

    wrapper.unmount();
  });

  it("falls back to default pane sizes for invalid stored values", () => {
    window.sessionStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        paneSizes: {
          main: [101, -1],
          leftColumn: ["bad", 40],
          bottom: [10, 10],
          rightColumn: [45, 55],
        },
      }),
    );

    const { store, wrapper } = mountLayoutStore();

    expect(store.prefs.value.paneSizes.main).toEqual([72, 28]);
    expect(store.prefs.value.paneSizes.leftColumn).toEqual([60, 40]);
    expect(store.prefs.value.paneSizes.bottom).toEqual([60, 40]);
    expect(store.prefs.value.paneSizes.rightColumn).toEqual([45, 55]);

    wrapper.unmount();
  });

  it("resets both view and trading preferences back to defaults", async () => {
    const { store, wrapper } = mountLayoutStore();

    store.update({
      market: "US",
      symbol: "AAPL",
      period: "5m",
      rightDockOpen: true,
      paneSizes: {
        main: [65, 35],
      },
    });
    await nextTick();

    store.reset();
    await nextTick();

    expect(store.prefs.value.market).toBe("HK");
    expect(store.prefs.value.symbol).toBe("00700");
    expect(store.prefs.value.period).toBe("1m");
    expect(store.prefs.value.rightDockOpen).toBe(false);
    expect(store.prefs.value.watchlistSidebarOpen).toBe(true);
    expect(store.prefs.value.watchlistSidebarWidth).toBe(280);
    expect(store.prefs.value.watchlistGroupId).toBeNull();
    expect(store.prefs.value.paneSizes.main).toEqual([72, 28]);

    wrapper.unmount();
  });

  it("persists and clamps the workspace watchlist view state", async () => {
    const { store, wrapper } = mountLayoutStore();

    store.update({
      watchlistSidebarOpen: false,
      watchlistSidebarWidth: 999,
      watchlistGroupId: "group-growth",
    });
    await nextTick();

    expect(store.prefs.value.watchlistSidebarOpen).toBe(false);
    expect(store.prefs.value.watchlistSidebarWidth).toBe(420);
    expect(store.prefs.value.watchlistGroupId).toBe("group-growth");
    expect(
      JSON.parse(
        window.localStorage.getItem("jftrade.workspace.view.v1") ?? "{}",
      ),
    ).toMatchObject({
      watchlistSidebarOpen: false,
      watchlistSidebarWidth: 420,
      watchlistGroupId: "group-growth",
    });

    wrapper.unmount();
  });

  it("throws clear errors when view or trading stores are missing", () => {
    const MissingViewHost = defineComponent({
      setup() {
        useWorkspaceViewState();
        return () => h("div");
      },
    });
    const MissingTradingHost = defineComponent({
      setup() {
        useWorkspaceTradingPrefs();
        return () => h("div");
      },
    });

    expect(() => mount(MissingViewHost)).toThrow(
      "Workspace view state store not provided.",
    );
    expect(() => mount(MissingTradingHost)).toThrow(
      "Workspace trading preferences store not provided.",
    );
  });

  it("normalizes the split view and trading records stored under their current keys", () => {
    window.sessionStorage.setItem(
      VIEW_STORAGE_KEY,
      JSON.stringify({
        rightDockOpen: true,
        rightDockTab: "ai",
        rightDockSize: 99,
        watchlistSidebarOpen: false,
        watchlistSidebarWidth: 199,
        watchlistGroupId: " growth ",
        paneSizes: {
          main: [64, 36],
          leftColumn: [0, 100],
          bottom: [60, 40],
          rightColumn: [50, 50],
        },
      }),
    );
    window.sessionStorage.setItem(
      TRADING_STORAGE_KEY,
      JSON.stringify({
        market: " us ",
        symbol: " aapl ",
        period: "60MIN",
        selectedBrokerAccountKey: " account-a ",
        favoriteBrokerAccountKeys: [" account-a ", "", "account-a", 3, "account-b"],
      }),
    );

    const { store, wrapper } = mountLayoutStore();
    expect(store.prefs.value.rightDockTab).toBe("ai");
    expect(store.prefs.value.rightDockSize).toBe(48);
    expect(store.prefs.value.watchlistSidebarWidth).toBe(220);
    expect(store.prefs.value.watchlistGroupId).toBe("growth");
    expect(store.prefs.value.paneSizes.leftColumn).toEqual([60, 40]);
    expect(store.prefs.value.market).toBe("US");
    expect(store.prefs.value.symbol).toBe("AAPL");
    expect(store.prefs.value.period).toBe("1h");
    expect(store.prefs.value.selectedBrokerAccountKey).toBe("account-a");
    expect(store.prefs.value.favoriteBrokerAccountKeys).toEqual(["account-a", "account-b"]);
    wrapper.unmount();
  });

  it("falls back through malformed current records to a valid legacy layout", () => {
    window.sessionStorage.setItem(VIEW_STORAGE_KEY, "{");
    window.sessionStorage.setItem(TRADING_STORAGE_KEY, "{");
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        market: "US",
        symbol: "MSFT",
        period: "15m",
        rightDockTab: "unexpected",
        rightDockSize: 10,
        watchlistSidebarWidth: 500,
        watchlistGroupId: "  ",
      }),
    );

    const { store, wrapper } = mountLayoutStore();
    expect(store.prefs.value.market).toBe("US");
    expect(store.prefs.value.symbol).toBe("MSFT");
    expect(store.prefs.value.period).toBe("15m");
    expect(store.prefs.value.rightDockTab).toBe("notifications");
    expect(store.prefs.value.rightDockSize).toBe(18);
    expect(store.prefs.value.watchlistSidebarWidth).toBe(420);
    expect(store.prefs.value.watchlistGroupId).toBeNull();
    wrapper.unmount();
  });

  it("normalizes unavailable optional preferences without losing the rest of a valid workspace record", () => {
    window.sessionStorage.setItem(
      VIEW_STORAGE_KEY,
      JSON.stringify({
        rightDockSize: "not-a-number",
        watchlistSidebarWidth: "not-a-number",
      }),
    );
    window.sessionStorage.setItem(
      TRADING_STORAGE_KEY,
      JSON.stringify({
        market: "US",
        symbol: "AAPL",
        period: "5m",
        favoriteBrokerAccountKeys: null,
      }),
    );

    const { store, wrapper } = mountLayoutStore();

    expect(store.prefs.value.rightDockSize).toBe(28);
    expect(store.prefs.value.watchlistSidebarWidth).toBe(280);
    expect(store.prefs.value.favoriteBrokerAccountKeys).toEqual([]);
    expect(store.prefs.value.symbol).toBe("AAPL");
    wrapper.unmount();
  });

  it("continues with in-memory preferences when browser storage becomes unavailable", async () => {
    vi.spyOn(window.sessionStorage, "getItem").mockImplementation(() => {
      throw new DOMException("storage blocked");
    });
    vi.spyOn(window.sessionStorage, "setItem").mockImplementation(() => {
      throw new DOMException("storage full");
    });

    const { store, wrapper } = mountLayoutStore();
    store.update({ symbol: "NVDA", market: "US" });
    await nextTick();

    expect(store.prefs.value).toMatchObject({ symbol: "NVDA", market: "US" });
    wrapper.unmount();
  });
});
