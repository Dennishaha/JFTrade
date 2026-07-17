// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent, ref } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ThemeMode } from "../src/composables/useTheme";
import {
  type UIColorPreferencesStore,
  provideUIColorPreferencesStore,
  useUIColorPreferences,
} from "../src/composables/useUIColorPreferences";
import { createResponse, flushRequests } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
  document.documentElement.removeAttribute("style");
});

describe("useUIColorPreferences", () => {
  it("keeps a user's in-flight color edits while hydrating and persists the final valid palette", async () => {
    let resolveInitialFetch: ((response: Response) => void) | undefined;
    const fetchMock = vi.fn()
      .mockImplementationOnce(() => new Promise<Response>((resolve) => {
        resolveInitialFetch = resolve;
      }))
      .mockImplementation(async () => createResponse({ appearance: {} }));
    vi.stubGlobal("fetch", fetchMock);

    let store: UIColorPreferencesStore | undefined;
    const Host = defineComponent({
      setup() {
        store = provideUIColorPreferencesStore(ref<ThemeMode>("light"));
        return {};
      },
      template: "<div />",
    });
    const wrapper = mount(Host);

    store!.setRiseIsRed(true);
    store!.reset();
    store!.update({ upColor: "not-a-valid-color" });

    expect(store!.prefs.value).toEqual({
      upColor: "#15803d",
      downColor: "#b91c1c",
    });
    resolveInitialFetch!(createResponse({
      appearance: { upColor: "#123456", downColor: "#abcdef" },
    }));
    await flushRequests();

    expect(store!.resolved.value).toEqual({
      upColor: "#15803d",
      downColor: "#b91c1c",
    });
    expect(document.documentElement.style.getPropertyValue("--tv-up")).toBe("#15803d");
    expect(fetchMock).toHaveBeenLastCalledWith(
      "/api/v1/settings/ui",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({
          appearance: { upColor: "#15803d", downColor: "#b91c1c" },
        }),
      }),
    );

    wrapper.unmount();
  });

  it("fails loudly when a consumer is mounted outside the UI-color provider", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});

    expect(() => useUIColorPreferences()).toThrow(
      "UI color preferences store not provided.",
    );

    warn.mockRestore();
  });
});
