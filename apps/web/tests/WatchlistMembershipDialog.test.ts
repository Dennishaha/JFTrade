// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick, ref } from "vue";

import { ApiClientError } from "../src/composables/apiClient";

const watchlistMocks = vi.hoisted(() => ({
  groupsData: null as unknown as ReturnType<typeof ref>,
  groupsError: null as unknown as ReturnType<typeof ref>,
  membershipData: null as unknown as ReturnType<typeof ref>,
  membershipError: null as unknown as ReturnType<typeof ref>,
  groupsRefetch: vi.fn().mockResolvedValue(undefined),
  membershipRefetch: vi.fn(),
  replace: vi.fn(),
}));

vi.mock("../src/composables/useWatchlist", () => ({
  useWatchlistGroups: () => ({
    data: watchlistMocks.groupsData,
    error: watchlistMocks.groupsError,
    isLoading: ref(false),
    refetch: watchlistMocks.groupsRefetch,
  }),
  useWatchlistMembership: () => ({
    query: {
      data: watchlistMocks.membershipData,
      error: watchlistMocks.membershipError,
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
  watchlistMocks.groupsError = ref(null);
  watchlistMocks.membershipData = ref({
    instrumentId: "US.AAPL",
    groupIds: ["g-default"],
    revision: 7,
  });
  watchlistMocks.membershipError = ref(null);
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

  it("refreshes on opening, prevents duplicate pending groups, and handles missing membership safely", async () => {
    const wrapper = mount(WatchlistMembershipDialog, {
      props: { modelValue: false, market: "US", symbol: "AAPL", name: "Apple" },
      global: { stubs: { "v-dialog": DialogStub } },
    });
    await wrapper.setProps({ modelValue: true });
    await nextTick();
    expect(watchlistMocks.groupsRefetch).toHaveBeenCalledOnce();
    expect(watchlistMocks.membershipRefetch).toHaveBeenCalledOnce();

    const newGroup = wrapper.get("#watchlist-new-group");
    await newGroup.setValue(" 自选股 ");
    await wrapper.findAll("button").find((button) => button.text() === "添加")!.trigger("click");
    expect(wrapper.text()).toContain("分组名称已存在");
    await newGroup.setValue(" 价值观察 ");
    await newGroup.trigger("keydown.enter");
    expect(wrapper.text()).toContain("价值观察");
    await wrapper.findAll(".watchlist-membership-dialog__chips button")[0]!.trigger("click");
    expect(wrapper.text()).not.toContain("价值观察");

    watchlistMocks.membershipData.value = null;
    await wrapper.get(".watchlist-membership-dialog__save").trigger("click");
    expect(wrapper.text()).toContain("自选状态尚未加载完成");
    await wrapper.get("button[aria-label='关闭']").trigger("click");
    expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([false]);
  });

  it("shows loading errors and a non-conflict save error without closing the editor", async () => {
    watchlistMocks.groupsError.value = new Error("分组服务不可用");
    watchlistMocks.replace.mockRejectedValueOnce(new Error("保存失败"));
    const wrapper = mount(WatchlistMembershipDialog, {
      props: { modelValue: true, market: "US", symbol: "AAPL", name: "Apple" },
      global: { stubs: { "v-dialog": DialogStub } },
    });
    await nextTick();
    expect(wrapper.text()).toContain("分组服务不可用");

    watchlistMocks.groupsError.value = null;
    await nextTick();
    await wrapper.get(".watchlist-membership-dialog__save").trigger("click");
    await nextTick();
    expect(wrapper.text()).toContain("保存失败");
    expect(wrapper.emitted("update:modelValue")).toBeUndefined();
  });
});
