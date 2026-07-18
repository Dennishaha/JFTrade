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
  queryClient.setDefaultOptions({});
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
    let itemRequests = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
        const url = String(input);
        if (url.endsWith("/api/v1/watchlist/groups")) {
          return ok({ groups: [
            { groupId: "g-default", name: "自选股", isDefault: true, revision: 1, itemCount: 2_000 },
            { groupId: "g-growth", name: "成长", isDefault: false, revision: 1, itemCount: 1 },
          ] });
        }
        if (url.includes("/api/v1/watchlist/items")) {
          itemRequests += 1;
          const cursor = new URL(url, "http://localhost").searchParams.get("cursor");
          return ok({
            items: cursor == null ? [...allItems, allItems[0]!] : [],
            nextCursor: cursor == null ? "page-2" : "",
          });
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
        stubs: {
          "v-icon": { template: "<span><slot /></span>" },
          WatchlistMembershipDialog: {
            props: ["market", "symbol", "name"],
            template: "<div data-testid='membership-dialog'>{{ market }}.{{ symbol }} {{ name }}</div>",
          },
        },
      },
    });
    await flushPromises();
    await nextTick();
    await flushPromises();

    const header = wrapper.get(".workspace-watchlist__header");
    expect(header.text()).toContain("自选股");
    expect(header.text()).not.toContain("可见行情实时刷新");
    expect(quoteRequestBodies.length).toBeGreaterThan(0);
    expect(quoteRequestBodies[0]!.instrumentIds.length).toBeGreaterThan(0);
    expect(quoteRequestBodies[0]!.instrumentIds.length).toBeLessThan(30);
    expect(quoteRequestBodies[0]!.instrumentIds).toContain("HK.00000");

    const viewport = wrapper.get<HTMLElement>("[data-testid='watchlist-virtual-viewport']");
    viewport.element.scrollTop = 999_999;
    await viewport.trigger("scroll");
    await flushPromises();
    expect(itemRequests).toBeGreaterThanOrEqual(2);
    viewport.element.scrollTop = 0;
    await viewport.trigger("scroll");
    await nextTick();

    await wrapper.get(".watchlist-table__row").trigger("click");
    await nextTick();
    if (store == null) throw new Error("workspace store was not provided");
    expect(store.prefs.value.market).toBe("HK");
    expect(store.prefs.value.symbol).toBe("00000");

    await wrapper.get(".watchlist-table__star").trigger("click");
    await nextTick();
    expect(wrapper.get("[data-testid='membership-dialog']").text()).toContain("HK.00000");
    await wrapper.get("[aria-label='选择自选分组']").setValue("g-growth");
    await nextTick();
    expect(store.prefs.value.watchlistGroupId).toBe("g-growth");
  });

  it("recovers from an unavailable group, exposes retry, and keeps navigation controls usable", async () => {
    queryClient.setDefaultOptions({ queries: { retry: false } });
    let itemRequests = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request) => {
        const url = String(input);
        if (url.endsWith("/api/v1/watchlist/groups")) {
          return ok({ groups: [{ groupId: "g-default", name: "自选股", isDefault: true, revision: 1, itemCount: 0 }] });
        }
        if (url.includes("/api/v1/watchlist/items")) {
          itemRequests += 1;
          throw new Error("自选读取失败");
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
        store.update({ watchlistGroupId: "stale-group" });
        return () => h(WorkspaceWatchlistSidebar, { compact: true });
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
    await nextTick();
    await flushPromises();

    if (store == null) throw new Error("workspace store was not provided");
    expect(store.prefs.value.watchlistGroupId).toBe("g-default");
    expect(wrapper.text()).toContain("自选读取失败");
    const retry = wrapper.findAll("button").find((button) => button.text() === "重试");
    if (retry == null) throw new Error("missing retry control");
    await retry.trigger("click");
    await flushPromises();
    expect(itemRequests).toBeGreaterThanOrEqual(2);

    const sidebar = wrapper.findComponent(WorkspaceWatchlistSidebar);
    await sidebar.find("header button").trigger("click");
    expect(sidebar.emitted("close")).toHaveLength(1);
    const openPage = sidebar.findAll("button").find((button) =>
      button.attributes("aria-label") === "打开完整自选页",
    );
    if (openPage == null) throw new Error("missing full-page control");
    await openPage.trigger("click");
    await flushPromises();
    expect(router.currentRoute.value.path).toBe("/watchlist");
  });
});
