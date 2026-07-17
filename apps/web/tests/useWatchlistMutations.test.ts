// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick, ref, type Ref } from "vue";

const apiMocks = vi.hoisted(() => ({
  createGroup: vi.fn(),
  deleteGroup: vi.fn(),
  getMembership: vi.fn(),
  getQuotes: vi.fn(),
  listGroups: vi.fn(),
  listItems: vi.fn(),
  replaceMembership: vi.fn(),
  updateGroup: vi.fn(),
}));

vi.mock("../src/composables/watchlistApi", () => ({
  createWatchlistGroup: apiMocks.createGroup,
  deleteWatchlistGroup: apiMocks.deleteGroup,
  getWatchlistMembership: apiMocks.getMembership,
  getWatchlistQuotes: apiMocks.getQuotes,
  listWatchlistGroups: apiMocks.listGroups,
  listWatchlistItems: apiMocks.listItems,
  replaceWatchlistMembership: apiMocks.replaceMembership,
  updateWatchlistGroup: apiMocks.updateGroup,
}));

import { queryClient } from "../src/composables/serverState";
import {
  useCreateWatchlistGroup,
  useDeleteWatchlistGroup,
  useUpdateWatchlistGroup,
  useWatchlistItems,
  useWatchlistMembership,
  useWatchlistQuotes,
} from "../src/composables/useWatchlist";

let wrappers: ReturnType<typeof mount>[] = [];

beforeEach(() => {
  apiMocks.listItems.mockResolvedValue({ items: [{ instrumentId: "US.AAPL" }], nextCursor: "next-1" });
  apiMocks.getMembership.mockResolvedValue({ instrumentId: "US.AAPL", revision: 3, groups: [], groupIds: [] });
  apiMocks.getQuotes.mockResolvedValue({
    quotes: [{ instrumentId: "US.AAPL", price: 220 }],
    errors: [{ instrumentId: "US.MSFT", message: "snapshot unavailable" }],
    observedAt: "2026-07-16T00:00:00Z",
  });
  apiMocks.createGroup.mockResolvedValue({ id: "new", name: "新分组", revision: 1 });
  apiMocks.updateGroup.mockResolvedValue({ id: "g-1", name: "更新", revision: 2 });
  apiMocks.deleteGroup.mockResolvedValue(undefined);
  apiMocks.replaceMembership.mockResolvedValue({
    instrumentId: "US.AAPL",
    revision: 4,
    groups: [{ id: "g-1", name: "观察" }],
    groupIds: ["g-1"],
  });
});

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount();
  queryClient.clear();
  vi.clearAllMocks();
  vi.useRealTimers();
});

describe("watchlist composables", () => {
  it("normalizes list filters and invalidates the right caches after group and membership mutations", async () => {
    const filters = ref({ groupId: "g-1", query: "  Apple  ", market: "us", limit: 25 });
    const market = ref(" us ");
    const symbol = ref(" aapl ");
    let state: ReturnType<typeof createState> | null = null;
    const Host = defineComponent({
      setup() {
        state = createState(filters, market, symbol);
        return () => h("div");
      },
    });
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    wrappers.push(mount(Host));
    await flushPromises();
    await flushPromises();

    expect(apiMocks.listItems).toHaveBeenCalledWith({
      groupId: "g-1",
      query: "Apple",
      market: "US",
      limit: 25,
      cursor: null,
    });
    expect(apiMocks.getMembership).toHaveBeenCalledWith("US", "AAPL");
    await state!.items.fetchNextPage();
    expect(apiMocks.listItems).toHaveBeenLastCalledWith(expect.objectContaining({ cursor: "next-1" }));

    await state!.createMutation.mutateAsync("新分组");
    await state!.updateMutation.mutateAsync({ groupId: "g-1", name: "更新", expectedRevision: 1 });
    await state!.deleteMutation.mutateAsync("g-1");
    await state!.membership.replaceMutation.mutateAsync({
      groupIds: ["g-1"],
      newGroupNames: [],
      expectedRevision: 3,
    });
    expect(apiMocks.createGroup).toHaveBeenCalledWith("新分组");
    expect(apiMocks.updateGroup).toHaveBeenCalledWith("g-1", { name: "更新", expectedRevision: 1 });
    expect(apiMocks.deleteGroup).toHaveBeenCalledWith("g-1");
    expect(apiMocks.replaceMembership).toHaveBeenCalledWith("US", "AAPL", expect.objectContaining({ groupIds: ["g-1"] }));
    expect(invalidateSpy).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["watchlist", "groups"] }));
  });

  it("settles visible quote changes, exposes quote maps, and stops polling when no rows are visible", async () => {
    vi.useFakeTimers();
    const ids = ref([" us.aapl ", "US.AAPL"]);
    let quotes: ReturnType<typeof useWatchlistQuotes> | null = null;
    const Host = defineComponent({
      setup() {
        quotes = useWatchlistQuotes(ids, true);
        return () => h("div");
      },
    });
    wrappers.push(mount(Host));
    await flushPromises();
    expect(apiMocks.getQuotes).toHaveBeenCalledWith(["US.AAPL"]);
    expect(quotes!.quotesByInstrument.value.get("US.AAPL")?.price).toBe(220);
    expect(quotes!.errorsByInstrument.value.get("US.MSFT")?.message).toBe("snapshot unavailable");

    ids.value = ["US.MSFT"];
    await vi.advanceTimersByTimeAsync(120);
    await flushPromises();
    expect(apiMocks.getQuotes).toHaveBeenLastCalledWith(["US.MSFT"]);
    const callsBeforeEmpty = apiMocks.getQuotes.mock.calls.length;
    ids.value = [];
    await flushPromises();
    expect(apiMocks.getQuotes.mock.calls).toHaveLength(callsBeforeEmpty);
  });

  it("cancels pending visible-row settling when the table changes again or unmounts", async () => {
    vi.useFakeTimers();
    const ids = ref(["US.AAPL"]);
    const Host = defineComponent({
      setup() {
        useWatchlistQuotes(ids, true);
        return () => h("div");
      },
    });
    const wrapper = mount(Host);
    wrappers.push(wrapper);
    await flushPromises();
    const callsBeforeSettle = apiMocks.getQuotes.mock.calls.length;

    ids.value = ["US.MSFT"];
    await nextTick();
    ids.value = ["US.TSLA"];
    await nextTick();
    wrapper.unmount();
    await vi.advanceTimersByTimeAsync(200);

    expect(apiMocks.getQuotes.mock.calls).toHaveLength(callsBeforeSettle);
  });
});

function createState(
  filters: Ref<{ groupId: string; query: string; market: string; limit: number }>,
  market: Ref<string>,
  symbol: Ref<string>,
) {
  return {
    createMutation: useCreateWatchlistGroup(),
    deleteMutation: useDeleteWatchlistGroup(),
    items: useWatchlistItems(filters),
    membership: useWatchlistMembership(market, symbol),
    updateMutation: useUpdateWatchlistGroup(),
  };
}
