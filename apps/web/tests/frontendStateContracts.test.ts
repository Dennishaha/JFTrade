// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick } from "vue";

import {
  provideCommandPaletteStore,
  useCommandPalette,
} from "../src/composables/useCommandPalette";
import { queryKeys } from "../src/composables/serverState";
import {
  provideThemeStore,
  useTheme,
} from "../src/composables/useTheme";
import { readLocalStorage } from "../src/composables/safeStorage";

afterEach(() => {
  vi.restoreAllMocks();
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
  document.documentElement.removeAttribute("data-vuetify-theme");
  document.documentElement.style.removeProperty("color-scheme");
  document.documentElement.classList.remove("dark");
});

describe("frontend state contracts", () => {
  it("uses stable cache keys for ADK and market detail resources", () => {
    expect(queryKeys.adk("sessions")).toEqual(["adk", "sessions"]);
    expect(queryKeys.marketData("HK", "00700", "1m")).toEqual([
      "marketData",
      "HK",
      "00700",
      "1m",
    ]);
  });

  it("treats a browser storage read failure as an unavailable preference", () => {
    vi.spyOn(window.localStorage, "getItem").mockImplementation(() => {
      throw new Error("storage blocked by policy");
    });

    expect(readLocalStorage("jftrade.theme")).toBeNull();
  });

  it("persists the selected theme and makes provider requirements explicit", async () => {
    window.localStorage.setItem("jftrade.theme", "light");
    let store: ReturnType<typeof provideThemeStore> | undefined;
    const Host = defineComponent({
      setup() {
        store = provideThemeStore();
        return () => h("div");
      },
    });
    const wrapper = mount(Host);

    expect(store?.theme.value).toBe("light");
    expect(document.documentElement.dataset.theme).toBe("light");
    store?.toggle();
    await nextTick();
    expect(store?.theme.value).toBe("dark");
    expect(window.localStorage.getItem("jftrade.theme")).toBe("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    wrapper.unmount();

    const warning = vi.spyOn(console, "warn").mockImplementation(() => {});
    expect(() => useTheme()).toThrow("Theme store not provided");
    expect(warning).toHaveBeenCalled();
  });

  it("replaces stale palette actions and rejects use outside its provider", () => {
    let store: ReturnType<typeof provideCommandPaletteStore> | undefined;
    const Provider = defineComponent({
      setup() {
        store = provideCommandPaletteStore();
        return () => h("div");
      },
    });
    const wrapper = mount(Provider);
    const removeFirst = store?.register({
      id: "nav.workspace",
      label: "旧工作台",
      group: "导航",
      run: () => {},
    });
    store?.register({
      id: "nav.workspace",
      label: "新工作台",
      group: "导航",
      run: () => {},
    });
    expect(store?.actions.value.map((action) => action.label)).toEqual(["新工作台"]);

    // The disposer returned for the current registration removes that command
    // when its owner is unmounted.
    const removeCurrent = store?.register({
      id: "nav.workspace",
      label: "新工作台",
      group: "导航",
      run: () => {},
    });
    removeCurrent?.();
    expect(store?.actions.value).toEqual([]);
    expect(removeFirst).toBeTypeOf("function");
    wrapper.unmount();

    const warning = vi.spyOn(console, "warn").mockImplementation(() => {});
    expect(() => useCommandPalette()).toThrow("Command palette store not provided");
    expect(warning).toHaveBeenCalled();
  });
});
