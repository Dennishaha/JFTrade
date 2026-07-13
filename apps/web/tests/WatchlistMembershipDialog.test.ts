// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick, ref } from "vue";

import { ApiClientError } from "../src/composables/apiClient";

const watchlistMocks = vi.hoisted(() => ({
  groupsData: null as unknown as ReturnType<typeof ref>,
  membershipData: null as unknown as ReturnType<typeof ref>,
  groupsRefetch: vi.fn().mockResolvedValue(undefined),
  membershipRefetch: vi.fn(),
  replace: vi.fn(),
}));

vi.mock("../src/composables/useWatchlist", () => ({
  useWatchlistGroups: () => ({
    data: watchlistMocks.groupsData,
    isLoading: ref(false),
    refetch: watchlistMocks.groupsRefetch,
  }),
  useWatchlistMembership: () => ({
    query: {
      data: watchlistMocks.membershipData,
      isLoading: ref(false),
      refetch: watchlistMocks.membershipRefetch,
    },
    replaceMutation: {
      isPending: ref(false),
      mutateAsync: watchlistMocks.replace,
    },
  }),
}));

import WatchlistMembershipDialog from "../src/components/domain/watchlist/WatchlistMembershipDialog.vue";

const DialogStub = defineComponent({
  props: { modelValue: Boolean },
  template: "<div v-if='modelValue' class='dialog-stub'><slot /></div>",
});

beforeEach(() => {
  watchlistMocks.groupsData = ref([
    { id: "g-default", name: "自选股", isDefault: true, revision: 2, itemCount: 1 },
    { id: "g-growth", name: "成长", revision: 3, itemCount: 4 },
  ]);
  watchlistMocks.membershipData = ref({
    instrumentId: "US.AAPL",
    groupIds: ["g-default"],
    revision: 7,
  });
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("WatchlistMembershipDialog", () => {
  it("renders an A-share title with a bare code and exchange tag", () => {
    const wrapper = mount(WatchlistMembershipDialog, {
      props: {
        modelValue: true,
        market: "SH",
        symbol: "600519",
        name: "贵州茅台",
      },
      global: { stubs: { "v-dialog": DialogStub } },
    });

    const identity = wrapper.get("h2 .instrument-identity");
    expect(identity.text()).toContain("600519");
    expect(identity.text()).toContain("上证");
    expect(identity.text()).toContain("贵州茅台");
    expect(identity.text()).not.toContain("SH.600519");
    expect(identity.attributes("title")).toBe("SH.600519");
  });

  it("keeps multi-group and new-group choices across a 409 and requires confirmation again", async () => {
    watchlistMocks.membershipRefetch.mockImplementation(async () => {
      watchlistMocks.membershipData.value = {
        instrumentId: "US.AAPL",
        groupIds: ["g-default"],
        revision: 8,
      };
    });
    watchlistMocks.replace
      .mockRejectedValueOnce(
        new ApiClientError("revision conflict", "WATCHLIST_CONFLICT", 409),
      )
      .mockResolvedValueOnce({
        instrumentId: "US.AAPL",
        groupIds: ["g-default", "g-growth"],
        revision: 9,
      });

    const wrapper = mount(WatchlistMembershipDialog, {
      props: {
        modelValue: true,
        market: "US",
        symbol: "AAPL",
        name: "Apple",
      },
      global: { stubs: { "v-dialog": DialogStub } },
    });
    await nextTick();

    const groupCheckboxes = wrapper.findAll(
      ".watchlist-membership-dialog__group input",
    );
    expect((groupCheckboxes[0]?.element as HTMLInputElement).checked).toBe(true);
    await groupCheckboxes[1]!.setValue(true);
    await wrapper.get("#watchlist-new-group").setValue("AI 观察");
    await wrapper.get("#watchlist-new-group").trigger("keydown.enter");
    await wrapper.get(".watchlist-membership-dialog__save").trigger("click");

    expect(watchlistMocks.replace).toHaveBeenNthCalledWith(1, {
      groupIds: ["g-default", "g-growth"],
      newGroupNames: ["AI 观察"],
      expectedRevision: 7,
    });
    expect(wrapper.text()).toContain("已刷新最新版本");
    expect(wrapper.text()).toContain("AI 观察");
    expect(wrapper.get(".watchlist-membership-dialog__notice").classes()).toEqual(
      expect.arrayContaining(["tv-status--warning", "tv-status-surface"]),
    );
    expect(wrapper.emitted("update:modelValue")).toBeUndefined();

    await wrapper.get(".watchlist-membership-dialog__save").trigger("click");
    expect(watchlistMocks.replace).toHaveBeenNthCalledWith(2, {
      groupIds: ["g-default", "g-growth"],
      newGroupNames: ["AI 观察"],
      expectedRevision: 8,
    });
    expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([false]);
  });
});
