// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";
import { defineComponent, nextTick, ref } from "vue";

const pageMocks = vi.hoisted(() => ({
  filters: null as unknown as { value: Record<string, unknown> },
  groupsQuery: null as unknown as Record<string, unknown>,
  itemsQuery: null as unknown as Record<string, unknown>,
  quoteIds: null as unknown as { value: string[] },
  quotesQuery: null as unknown as Record<string, unknown>,
  workspace: null as unknown as Record<string, unknown>,
}));

vi.mock("../src/composables/useWatchlist", () => ({
  useWatchlistGroups: () => pageMocks.groupsQuery,
  useWatchlistItems: (filters: unknown) => {
    pageMocks.filters = filters as typeof pageMocks.filters;
    return pageMocks.itemsQuery;
  },
  useWatchlistQuotes: (ids: unknown) => {
    pageMocks.quoteIds = ids as typeof pageMocks.quoteIds;
    return pageMocks.quotesQuery;
  },
}));

vi.mock("../src/composables/useWorkspaceLayout", () => ({
  useWorkspaceTradingPrefs: () => pageMocks.workspace,
}));

import WatchlistPage from "../src/pages/WatchlistPage.vue";

const item = {
  instrumentId: "US.AAPL",
  market: "US",
  symbol: "AAPL",
  name: "Apple",
  securityType: "EQUITY",
};

const tableStub = defineComponent({
  props: ["emptyText"],
  emits: ["select", "edit-membership", "visible-instrument-ids", "end-reached"],
  template: `
    <div class="watchlist-table-stub" :data-empty-text="emptyText">
      <button class="table-select" @click="$emit('select', { instrumentId: 'US.AAPL', market: 'US', symbol: 'AAPL', name: 'Apple' })">select</button>
      <button class="table-edit" @click="$emit('edit-membership', { instrumentId: 'US.AAPL', market: 'US', symbol: 'AAPL', name: 'Apple' })">edit</button>
      <button class="table-visible" @click="$emit('visible-instrument-ids', ['US.AAPL'])">visible</button>
      <button class="table-more" @click="$emit('end-reached')">more</button>
    </div>
  `,
});

const membershipStub = defineComponent({
  props: ["modelValue", "market", "symbol", "name"],
  template: "<div v-if='modelValue' class='membership-stub'>{{ market }} {{ symbol }} {{ name }}</div>",
});

function setupState() {
  const groups = ref([
    { id: "default", name: "自选股", revision: 1, itemCount: 1 },
    { id: "tech", name: "科技", revision: 2, itemCount: 1 },
  ]);
  const groupError = ref<unknown>(null);
  const itemError = ref<unknown>(null);
  const refetch = vi.fn();
  const fetchNextPage = vi.fn();
  pageMocks.groupsQuery = {
    data: groups,
    error: groupError,
  };
  pageMocks.itemsQuery = {
    data: ref({ pages: [{ items: [item, { ...item }] }] }),
    error: itemError,
    fetchNextPage,
    hasNextPage: ref(true),
    isFetchingNextPage: ref(false),
    isLoading: ref(false),
    refetch,
  };
  pageMocks.quotesQuery = {
    errorsByInstrument: ref(new Map()),
    quotesByInstrument: ref(new Map()),
  };
  const update = vi.fn();
  pageMocks.workspace = {
    prefs: ref({ market: "HK", symbol: "00700" }),
    update,
  };
  return { fetchNextPage, groupError, groups, itemError, refetch, update };
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
});

describe("WatchlistPage business interactions", () => {
  it("updates filters, clears deleted group selection, and wires table actions to their business destinations", async () => {
    const state = setupState();
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/watchlist", component: WatchlistPage },
        { path: "/workspace", component: { template: "<div />" } },
      ],
    });
    await router.push("/watchlist");
    await router.isReady();
    const pushSpy = vi.spyOn(router, "push");
    const wrapper = mount(WatchlistPage, {
      global: {
        plugins: [router],
        stubs: {
          WatchlistGroupManagerDialog: { props: ["modelValue"], template: "<div v-if='modelValue' class='manager-stub' />" },
          WatchlistImportDialog: { props: ["modelValue"], template: "<div v-if='modelValue' class='import-stub' />" },
          WatchlistMembershipDialog: membershipStub,
          WatchlistVirtualTable: tableStub,
          "v-icon": { template: "<span><slot /></span>" },
        },
      },
    });

    await wrapper.findAll('[role="tab"]')[2]!.trigger("click");
    expect(wrapper.text()).toContain("科技 · 已加载 1");
    state.groups.value = [state.groups.value[0]!];
    await nextTick();
    expect(wrapper.text()).toContain("全部 · 已加载 1");

    const search = wrapper.get('input[aria-label="搜索自选股名称或代码"]');
    await search.setValue("  AAPL  ");
    await vi.advanceTimersByTimeAsync(220);
    expect(pageMocks.filters.value).toMatchObject({ query: "AAPL", groupId: null });
    await wrapper.get(".watchlist-page__market-filter select").setValue("US");
    expect(pageMocks.filters.value).toMatchObject({ market: "US" });
    await wrapper.get('button[aria-label="清除搜索"]').trigger("click");
    await vi.advanceTimersByTimeAsync(220);
    expect(pageMocks.filters.value).toMatchObject({ query: "" });

    await wrapper.find(".table-visible").trigger("click");
    expect(pageMocks.quoteIds.value).toEqual(["US.AAPL"]);
    await wrapper.find(".table-more").trigger("click");
    expect(state.fetchNextPage).toHaveBeenCalledOnce();
    await wrapper.find(".table-edit").trigger("click");
    expect(wrapper.find(".membership-stub").text()).toContain("US AAPL Apple");
    await wrapper.find(".table-select").trigger("click");
    expect(state.update).toHaveBeenCalledWith({ market: "US", symbol: "AAPL" });
    expect(pushSpy).toHaveBeenCalledWith("/workspace");

    await wrapper.findAll(".watchlist-page__header-actions button")[0]!.trigger("click");
    await wrapper.findAll(".watchlist-page__header-actions button")[1]!.trigger("click");
    expect(wrapper.find(".manager-stub").exists()).toBe(true);
    expect(wrapper.find(".import-stub").exists()).toBe(true);
  });

  it("shows the real query error and lets users request a retry", async () => {
    const state = setupState();
    state.groupError.value = new Error("分组读取失败");
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/watchlist", component: WatchlistPage }],
    });
    await router.push("/watchlist");
    await router.isReady();
    const wrapper = mount(WatchlistPage, {
      global: {
        plugins: [router],
        stubs: {
          WatchlistGroupManagerDialog: { template: "<div />" },
          WatchlistImportDialog: { template: "<div />" },
          WatchlistMembershipDialog: { template: "<div />" },
          WatchlistVirtualTable: tableStub,
          "v-icon": { template: "<span />" },
        },
      },
    });
    await nextTick();
    expect(wrapper.text()).toContain("分组读取失败");
    await wrapper.find(".watchlist-page__error button").trigger("click");
    expect(state.refetch).toHaveBeenCalledOnce();
  });

  it("does not queue duplicate page fetches and clears a pending search when leaving the page", async () => {
    const state = setupState();
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/watchlist", component: WatchlistPage }],
    });
    await router.push("/watchlist");
    await router.isReady();
    const wrapper = mount(WatchlistPage, {
      global: {
        plugins: [router],
        stubs: {
          WatchlistGroupManagerDialog: { template: "<div />" },
          WatchlistImportDialog: { template: "<div />" },
          WatchlistMembershipDialog: membershipStub,
          WatchlistVirtualTable: tableStub,
          "v-icon": { template: "<span />" },
        },
      },
    });

    (pageMocks.itemsQuery.hasNextPage as { value: boolean }).value = false;
    await wrapper.find(".table-more").trigger("click");
    (pageMocks.itemsQuery.hasNextPage as { value: boolean }).value = true;
    (pageMocks.itemsQuery.isFetchingNextPage as { value: boolean }).value = true;
    await wrapper.find(".table-more").trigger("click");
    expect(state.fetchNextPage).not.toHaveBeenCalled();

    const search = wrapper.get('input[aria-label="搜索自选股名称或代码"]');
    await search.setValue("TSLA");
    wrapper.unmount();
    await vi.advanceTimersByTimeAsync(220);
    expect(pageMocks.filters.value).not.toMatchObject({ query: "TSLA" });
  });
});
