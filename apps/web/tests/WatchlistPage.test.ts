// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";
import { defineComponent, h } from "vue";

import WatchlistPage from "../src/pages/WatchlistPage.vue";
import { queryClient } from "../src/composables/serverState";
import { provideWorkspaceLayoutStore } from "../src/composables/useWorkspaceLayout";

function ok(data: unknown): Response {
  return new Response(
    JSON.stringify({ ok: true, data, timestamp: "2026-07-11T00:00:00Z" }),
    { status: 200, headers: { "Content-Type": "application/json" } },
  );
}

afterEach(() => {
  queryClient.clear();
  window.localStorage.clear();
  vi.unstubAllGlobals();
});

describe("WatchlistPage", () => {
  it("renders local group tabs, filters by group, and opens group management", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith("/api/v1/watchlist/groups")) {
        return ok({
          groups: [
            { groupId: "default", name: "自选股", isDefault: true, protected: true, revision: 1, itemCount: 1 },
            { groupId: "tech", name: "科技", revision: 1, itemCount: 1 },
          ],
        });
      }
      if (url.includes("/api/v1/watchlist/items")) {
        return ok({
          items: [
            {
              instrumentId: "US.AAPL",
              market: "US",
              symbol: "AAPL",
              name: "Apple",
              type: "SecurityType_Eqty",
              groups: [{ groupId: "tech", name: "科技" }],
            },
          ],
        });
      }
      if (url.endsWith("/api/v1/watchlist/quotes/batch")) {
        return ok({ quotes: [], errors: [], observedAt: "2026-07-11T00:00:00Z" });
      }
      throw new Error(`unexpected request: ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/watchlist", component: WatchlistPage },
        { path: "/workspace", component: { template: "<div />" } },
      ],
    });
    await router.push("/watchlist");
    await router.isReady();
    const Host = defineComponent({
      setup() {
        provideWorkspaceLayoutStore();
        return () => h(WatchlistPage);
      },
    });
    const wrapper = mount(Host, {
      global: {
        plugins: [router],
        stubs: {
          WatchlistGroupManagerDialog: {
            props: ["modelValue"],
            template: "<div v-if='modelValue' data-testid='group-manager-stub' />",
          },
          WatchlistImportDialog: { template: "<div />" },
          WatchlistMembershipDialog: { template: "<div />" },
          "v-icon": { template: "<span><slot /></span>" },
        },
      },
    });
    await flushPromises();
    await flushPromises();

    expect(wrapper.get(".tv-state-dot").classes()).toContain("tv-status--success");
    const tabs = wrapper.findAll('[role="tab"]');
    expect(tabs.map((tab) => tab.text())).toEqual([
      "全部",
      expect.stringContaining("自选股"),
      expect.stringContaining("科技"),
    ]);
    await tabs[2]!.trigger("click");
    await flushPromises();
    await tabs[0]!.trigger("click");
    await flushPromises();
    await tabs[2]!.trigger("click");
    await flushPromises();
    const itemRequests = fetchMock.mock.calls
      .map((call) => String(call[0]))
      .filter((url) => url.includes("/api/v1/watchlist/items"));
    expect(itemRequests.filter((url) => url.includes("groupId=tech"))).toHaveLength(2);
    expect(itemRequests.filter((url) => !url.includes("groupId="))).toHaveLength(2);

    const marketOptions = wrapper
      .findAll(".watchlist-page__market-filter option")
      .map((option) => option.attributes("value"));
    expect(marketOptions).toEqual(
      expect.arrayContaining(["HK", "US", "SH", "SZ", "SG", "JP", "AU", "MY", "CA"]),
    );
    await wrapper
      .get(".watchlist-page__header-actions button")
      .trigger("click");
    expect(wrapper.find('[data-testid="group-manager-stub"]').exists()).toBe(true);
  });
});
