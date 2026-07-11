// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref } from "vue";

import { queryClient } from "../src/composables/serverState";
import { useWatchlistQuotes } from "../src/composables/useWatchlist";

afterEach(() => {
  queryClient.clear();
  vi.unstubAllGlobals();
});

describe("useWatchlistQuotes", () => {
  it("pauses while the page is hidden and fetches immediately when visible", async () => {
    let visibility: DocumentVisibilityState = "hidden";
    const original = Object.getOwnPropertyDescriptor(document, "visibilityState");
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => visibility,
    });
    const fetchMock = vi.fn(async () =>
      new Response(
        JSON.stringify({
          ok: true,
          data: { quotes: [], errors: [], observedAt: "2026-07-11T00:00:00Z" },
          timestamp: "2026-07-11T00:00:00Z",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);
    const Host = defineComponent({
      setup() {
        useWatchlistQuotes(ref(["US.AAPL"]), true);
        return () => h("div");
      },
    });
    const wrapper = mount(Host);
    try {
      await flushPromises();
      expect(fetchMock).not.toHaveBeenCalled();

      visibility = "visible";
      document.dispatchEvent(new Event("visibilitychange"));
      await flushPromises();
      await flushPromises();
      expect(fetchMock).toHaveBeenCalledTimes(1);
      expect(String(fetchMock.mock.calls[0]?.[0])).toContain(
        "/api/v1/watchlist/quotes/batch",
      );
    } finally {
      wrapper.unmount();
      if (original == null) {
        Reflect.deleteProperty(document, "visibilityState");
      } else {
        Object.defineProperty(document, "visibilityState", original);
      }
    }
  });
});
