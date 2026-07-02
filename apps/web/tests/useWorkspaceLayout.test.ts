// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it } from "vitest";
import { defineComponent, h, nextTick } from "vue";

import {
  provideWorkspaceLayoutStore,
  useWorkspaceTradingPrefs,
  useWorkspaceViewState,
  type WorkspaceLayoutStore,
} from "../src/composables/useWorkspaceLayout";

const STORAGE_KEY = "jftrade.workspace.layout.v1";

afterEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
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
    expect(store.prefs.value.paneSizes.main).toEqual([72, 28]);

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
});
