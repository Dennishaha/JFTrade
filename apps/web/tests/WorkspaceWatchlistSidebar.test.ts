// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";
import { defineComponent, h, nextTick } from "vue";

import WorkspaceWatchlistSidebar from "../src/components/domain/watchlist/WorkspaceWatchlistSidebar.vue";
import { queryClient } from "../src/composables/serverState";
import {
  provideWorkspaceLayoutStore,
  type WorkspaceLayoutStore,
} from "../src/composables/useWorkspaceLayout";

function ok(data: unknown): Response {
  return new Response(
    JSON.stringify({ ok: true, data, timestamp: "2026-07-11T00:00:00Z" }),
    { status: 200, headers: { "Content-Type": "application/json" } },
  );
}

afterEach(() => {
  queryClient.clear();
  window.localStorage.clear();
  window.sessionStorage.clear();
  vi.unstubAllGlobals();
});

describe("WorkspaceWatchlistSidebar", () => {
  it("polls quotes for the virtual visible window and selects the workspace instrument", async () => {
    const allItems = Array.from({ length: 2_000 }, (_, index) => {
      const symbol = String(index).padStart(5, "0");
      return {
        instrumentId: `HK.${symbol}`,
        market: "HK",
        symbol,
        name: `标的 ${index}`,
        type: "EQUITY",
        groups: [{ groupId: "g-default", name: "自选股" }],
      };
    });
    const quoteRequestBodies: Array<{ instrumentIds: string[] }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
        const url = String(input);
        if (url.endsWith("/api/v1/watchlist/groups")) {
          return ok({ groups: [{ groupId: "g-default", name: "自选股", isDefault: true, revision: 1, itemCount: 2_000 }] });
        }
        if (url.includes("/api/v1/watchlist/items")) {
          return ok({ items: allItems });
        }
        if (url.endsWith("/api/v1/watchlist/quotes/batch")) {
          const request = JSON.parse(String(init?.body)) as { instrumentIds: string[] };
          quoteRequestBodies.push(request);
          return ok({
            quotes: request.instrumentIds.map((instrumentId) => ({ instrumentId, price: 10 })),
            errors: [],
            observedAt: "2026-07-11T00:00:00Z",
          });
        }
        throw new Error(`unexpected request: ${url}`);
      }),
    );

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/workspace", component: { template: "<div />" } },
        { path: "/watchlist", component: { template: "<div />" } },
      ],
    });
    await router.push("/workspace");
    await router.isReady();
    let store: WorkspaceLayoutStore | null = null;
    const Host = defineComponent({
      setup() {
        store = provideWorkspaceLayoutStore();
        return () => h(WorkspaceWatchlistSidebar);
      },
    });
    const wrapper = mount(Host, {
      global: {
        plugins: [router],
        stubs: { "v-icon": { template: "<span><slot /></span>" } },
      },
    });
    await flushPromises();
    await nextTick();
    await flushPromises();

    expect(quoteRequestBodies.length).toBeGreaterThan(0);
    expect(quoteRequestBodies[0]!.instrumentIds.length).toBeGreaterThan(0);
    expect(quoteRequestBodies[0]!.instrumentIds.length).toBeLessThan(30);
    expect(quoteRequestBodies[0]!.instrumentIds).toContain("HK.00000");

    await wrapper.get(".watchlist-table__row").trigger("click");
    await nextTick();
    if (store == null) throw new Error("workspace store was not provided");
    expect(store.prefs.value.market).toBe("HK");
    expect(store.prefs.value.symbol).toBe("00000");
  });
});
