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

  it("retains the latest successful quote when the next batch only returns an error", async () => {
    let requestCount = 0;
    const fetchMock = vi.fn(async () => {
      requestCount += 1;
      const data = requestCount === 1
        ? {
            quotes: [{ instrumentId: "US.AAPL", price: 220 }],
            errors: [],
            observedAt: "2026-07-11T00:00:00Z",
          }
        : {
            quotes: [],
            errors: [{
              instrumentId: "US.AAPL",
              code: "SNAPSHOT_RATE_LIMITED",
              message: "行情额度暂时不足",
            }],
            observedAt: "2026-07-11T00:00:03Z",
          };
      return new Response(
        JSON.stringify({ ok: true, data, timestamp: data.observedAt }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    let query!: ReturnType<typeof useWatchlistQuotes>;
    const Host = defineComponent({
      setup() {
        query = useWatchlistQuotes(ref(["US.AAPL"]), true);
        return () => h("div");
      },
    });
    const wrapper = mount(Host);
    try {
      await flushPromises();
      expect(query.quotesByInstrument.value.get("US.AAPL")?.price).toBe(220);

      await query.refetch();
      await flushPromises();

      expect(query.quotesByInstrument.value.get("US.AAPL")?.price).toBe(220);
      expect(query.errorsByInstrument.value.get("US.AAPL")?.code).toBe(
        "SNAPSHOT_RATE_LIMITED",
      );
    } finally {
      wrapper.unmount();
    }
  });
});
