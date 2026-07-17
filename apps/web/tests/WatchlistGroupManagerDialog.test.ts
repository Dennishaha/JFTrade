// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, ref } from "vue";

const groupMocks = vi.hoisted(() => ({
  create: vi.fn(),
  delete: vi.fn(),
  groups: [] as Array<{
    id: string;
    name: string;
    isDefault?: boolean;
    protected?: boolean;
    revision: number;
    itemCount?: number;
  }>,
  refetch: vi.fn(),
  update: vi.fn(),
}));

vi.mock("../src/composables/useWatchlist", () => ({
  useWatchlistGroups: () => ({
    data: ref(groupMocks.groups),
    refetch: groupMocks.refetch,
  }),
  useCreateWatchlistGroup: () => ({
    isPending: ref(false),
    mutateAsync: groupMocks.create,
  }),
  useUpdateWatchlistGroup: () => ({
    mutateAsync: groupMocks.update,
  }),
  useDeleteWatchlistGroup: () => ({
    mutateAsync: groupMocks.delete,
  }),
}));

import WatchlistGroupManagerDialog from "../src/components/domain/watchlist/WatchlistGroupManagerDialog.vue";

const DialogStub = defineComponent({
  props: { modelValue: Boolean },
  template: "<div v-if='modelValue' class='dialog-stub'><slot /></div>",
});

function articleFor(wrapper: ReturnType<typeof mount>, groupName: string) {
  const article = wrapper.findAll("article").find((candidate) => candidate.text().includes(groupName));
  if (article == null) throw new Error(`missing group article: ${groupName}`);
  return article;
}

beforeEach(() => {
  groupMocks.groups = [
    { id: "default", name: "默认自选", isDefault: true, protected: true, revision: 1, itemCount: 2 },
    { id: "tech", name: "科技观察", revision: 4, itemCount: 3 },
  ];
  groupMocks.create.mockResolvedValue(undefined);
  groupMocks.update.mockResolvedValue(undefined);
  groupMocks.delete.mockResolvedValue(undefined);
  groupMocks.refetch.mockResolvedValue(undefined);
});

afterEach(() => {
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

describe("WatchlistGroupManagerDialog", () => {
  it("refreshes on open, creates a trimmed local group, and allows closing when idle", async () => {
    const wrapper = mount(WatchlistGroupManagerDialog, {
      props: { modelValue: false },
      global: { stubs: { "v-dialog": DialogStub } },
    });

    await wrapper.setProps({ modelValue: true });
    await flushPromises();
    expect(groupMocks.refetch).toHaveBeenCalledOnce();

    const nameInput = wrapper.get("input[aria-label='新分组名称']");
    await nameInput.setValue("  高股息  ");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(groupMocks.create).toHaveBeenCalledWith("高股息");
    expect((nameInput.element as HTMLInputElement).value).toBe("");

    await wrapper.get("button[aria-label='关闭']").trigger("click");
    expect(wrapper.emitted("update:modelValue")).toEqual([[false]]);
  });

  it("handles rename validation, successful rename, and mutation errors", async () => {
    const wrapper = mount(WatchlistGroupManagerDialog, {
      props: { modelValue: true },
      global: { stubs: { "v-dialog": DialogStub } },
    });

    const techArticle = articleFor(wrapper, "科技观察");
    await techArticle.findAll("button").find((button) => button.text() === "重命名")!.trigger("click");
    const renameInput = techArticle.get("input[aria-label='重命名分组 科技观察']");
    await renameInput.setValue(" ");
    await renameInput.trigger("keydown.enter");
    expect(techArticle.find("input").exists()).toBe(false);
    expect(groupMocks.update).not.toHaveBeenCalled();

    await techArticle.findAll("button").find((button) => button.text() === "重命名")!.trigger("click");
    await techArticle.get("input").setValue("  AI 基础设施 ");
    await techArticle.findAll("button").find((button) => button.text() === "保存")!.trigger("click");
    await flushPromises();
    expect(groupMocks.update).toHaveBeenCalledWith({
      groupId: "tech",
      name: "AI 基础设施",
      expectedRevision: 4,
    });

    groupMocks.update.mockRejectedValueOnce(new Error("版本冲突"));
    await techArticle.findAll("button").find((button) => button.text() === "重命名")!.trigger("click");
    await techArticle.get("input").setValue("冲突名称");
    await techArticle.findAll("button").find((button) => button.text() === "保存")!.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("版本冲突");
  });

  it("keeps protected groups safe and requires confirmation before deleting local groups", async () => {
    const wrapper = mount(WatchlistGroupManagerDialog, {
      props: { modelValue: true },
      global: { stubs: { "v-dialog": DialogStub } },
    });
    const defaultArticle = articleFor(wrapper, "默认自选");
    const protectedDelete = defaultArticle.findAll("button").find((button) => button.text() === "删除")!;
    expect(protectedDelete.attributes("disabled")).toBeDefined();

    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValueOnce(false).mockReturnValue(true);
    const techArticle = articleFor(wrapper, "科技观察");
    const deleteButton = techArticle.findAll("button").find((button) => button.text() === "删除")!;
    await deleteButton.trigger("click");
    await flushPromises();
    expect(groupMocks.delete).not.toHaveBeenCalled();

    await deleteButton.trigger("click");
    await flushPromises();
    expect(confirmSpy).toHaveBeenCalledTimes(2);
    expect(groupMocks.delete).toHaveBeenCalledWith("tech");

    groupMocks.delete.mockRejectedValueOnce(new Error("分组仍有同步任务"));
    await deleteButton.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("分组仍有同步任务");
  });

  it("does not create blank groups and keeps an actionable fallback after a non-Error failure", async () => {
    groupMocks.create.mockRejectedValueOnce("service unavailable");
    const wrapper = mount(WatchlistGroupManagerDialog, {
      props: { modelValue: true },
      global: { stubs: { "v-dialog": DialogStub } },
    });

    await wrapper.get("form").trigger("submit");
    expect(groupMocks.create).not.toHaveBeenCalled();

    const input = wrapper.get("input[aria-label='新分组名称']");
    await input.setValue("临时观察");
    await wrapper.get("form").trigger("submit");
    await flushPromises();

    expect(groupMocks.create).toHaveBeenCalledWith("临时观察");
    expect(wrapper.text()).toContain("创建分组失败");
  });
});
