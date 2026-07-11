// @vitest-environment jsdom

import { mount, flushPromises } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, ref } from "vue";

import { queryClient } from "../src/composables/serverState";

const importMocks = vi.hoisted(() => ({
  listSources: vi.fn(),
  listSourceGroups: vi.fn(),
  listBindings: vi.fn(),
  deleteBinding: vi.fn(),
  preview: vi.fn(),
  commit: vi.fn(),
}));

vi.mock("../src/composables/useWatchlist", () => ({
  useWatchlistGroups: () => ({
    data: ref([
      { id: "g-default", name: "自选股", isDefault: true, revision: 1, itemCount: 2 },
      { id: "g-tech", name: "科技股", isDefault: false, revision: 1, itemCount: 1 },
    ]),
    refetch: vi.fn().mockResolvedValue(undefined),
  }),
}));

vi.mock("../src/composables/watchlistApi", () => ({
  listWatchlistSources: importMocks.listSources,
  listWatchlistSourceGroups: importMocks.listSourceGroups,
  listWatchlistBindings: importMocks.listBindings,
  deleteWatchlistBinding: importMocks.deleteBinding,
  previewWatchlistImport: importMocks.preview,
  commitWatchlistImport: importMocks.commit,
}));

import WatchlistImportDialog from "../src/components/domain/watchlist/WatchlistImportDialog.vue";

const DialogStub = defineComponent({
  props: { modelValue: Boolean },
  template: "<div v-if='modelValue' class='dialog-stub'><slot /></div>",
});

beforeEach(() => {
  importMocks.listBindings.mockResolvedValue([]);
  importMocks.deleteBinding.mockResolvedValue(undefined);
});

afterEach(() => {
  queryClient.clear();
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

describe("WatchlistImportDialog", () => {
  it("does not request remote groups when every source is unavailable", async () => {
    importMocks.listSources.mockResolvedValue([
      {
        id: "futu:default",
        displayName: "富途 OpenD",
        available: false,
        message: "OpenD 未连接",
      },
    ]);

    const wrapper = mount(WatchlistImportDialog, {
      props: { modelValue: true },
      global: { stubs: { "v-dialog": DialogStub } },
    });
    await flushPromises();

    expect(importMocks.listSourceGroups).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("当前券商连接不可用");
  });

  it("disables ambiguous groups and keeps local-only deletion opt-in", async () => {
    importMocks.listSources.mockResolvedValue([
      { id: "futu:default", displayName: "富途 OpenD", available: true },
    ]);
    importMocks.listSourceGroups.mockResolvedValue([
      { remoteGroupId: "duplicate", name: "科技", type: "CUSTOM", ambiguous: true },
      { remoteGroupId: "system-all", name: "全部", type: "SYSTEM", system: true, ambiguous: false },
    ]);
    importMocks.preview.mockResolvedValue({
      id: "preview-1",
      previewId: "preview-1",
      sourceId: "futu:default",
      remoteGroupName: "全部",
      added: [{ instrumentId: "HK.00700" }],
      unchanged: [],
      localOnly: [{ instrumentId: "HK.00005", name: "汇丰控股" }],
    });
    importMocks.commit.mockResolvedValue({
      id: "run-1",
      sourceId: "futu:default",
      status: "completed",
    });

    const wrapper = mount(WatchlistImportDialog, {
      props: { modelValue: true },
      global: { stubs: { "v-dialog": DialogStub } },
    });
    await flushPromises();

    const remoteOptions = wrapper.findAll(
      ".watchlist-import-dialog__form-grid select:nth-of-type(1) option",
    );
    const ambiguousOption = wrapper.findAll("option").find((option) =>
      option.text().includes("重名"),
    );
    expect(ambiguousOption?.attributes("disabled")).toBeDefined();
    expect(wrapper.text()).toContain("系统组");
    expect(remoteOptions.length).toBeGreaterThanOrEqual(0);

    await wrapper.get(".watchlist-import-dialog__primary").trigger("click");
    await flushPromises();
    const localOnlyCheckbox = wrapper.get(
      ".watchlist-import-dialog__local-only input",
    ).element as HTMLInputElement;
    expect(localOnlyCheckbox.checked).toBe(false);

    await wrapper.get(".watchlist-import-dialog__primary").trigger("click");
    await flushPromises();
    expect(importMocks.commit).toHaveBeenCalledWith("preview-1", {
      deleteLocalOnlyInstrumentIds: [],
    });
    expect(wrapper.get(".watchlist-import-dialog__state").classes()).toEqual(
      expect.arrayContaining(["tv-status--success", "tv-status-surface"]),
    );
  });

  it("keeps the committed result after creating a local group", async () => {
    importMocks.listSources.mockResolvedValue([
      { id: "futu:default", displayName: "富途 OpenD", available: true },
    ]);
    importMocks.listSourceGroups.mockResolvedValue([
      { remoteGroupId: "remote-growth", name: "成长股", type: "CUSTOM", ambiguous: false },
    ]);
    importMocks.preview.mockResolvedValue({
      id: "preview-new-group",
      previewId: "preview-new-group",
      sourceId: "futu:default",
      remoteGroupName: "成长股",
      added: [{ instrumentId: "US.NVDA" }],
      unchanged: [],
      localOnly: [],
    });
    importMocks.commit.mockResolvedValue({
      id: "run-new-group",
      sourceId: "futu:default",
      status: "completed",
      localGroupId: "g-growth",
    });

    const wrapper = mount(WatchlistImportDialog, {
      props: { modelValue: true },
      global: { stubs: { "v-dialog": DialogStub } },
    });
    await flushPromises();

    const localGroupSelect = wrapper.findAll(
      ".watchlist-import-dialog__form-grid select",
    )[2]!;
    await localGroupSelect.setValue("");
    await wrapper.get(".watchlist-import-dialog__form-grid input").setValue("成长股");
    await wrapper.get(".watchlist-import-dialog__primary").trigger("click");
    await flushPromises();
    expect(importMocks.preview).toHaveBeenCalledWith(
      expect.objectContaining({
        sourceId: "futu:default",
        remoteGroupId: "remote-growth",
        newGroupName: "成长股",
      }),
    );

    await wrapper.get(".watchlist-import-dialog__primary").trigger("click");
    await flushPromises();
    await flushPromises();

    expect(wrapper.text()).toContain("导入已完成，本地自选已更新。");
    expect(wrapper.text()).toContain("US.NVDA");
    expect(wrapper.get("footer .tv-btn-ghost").text()).toBe("关闭");

    await localGroupSelect.setValue("g-default");
    expect(wrapper.text()).not.toContain("导入已完成，本地自选已更新。");
    expect(wrapper.text()).not.toContain("US.NVDA");
  });

  it("restores the bound local group for repeat imports", async () => {
    importMocks.listSources.mockResolvedValue([
      { id: "futu:default", displayName: "富途 OpenD", available: true },
    ]);
    importMocks.listSourceGroups.mockResolvedValue([
      { remoteGroupId: "remote-tech", name: "科技", type: "CUSTOM", ambiguous: false },
    ]);
    importMocks.listBindings.mockResolvedValue([
      {
        id: "binding-1",
        sourceId: "futu:default",
        remoteGroupId: "remote-tech",
        remoteGroupName: "科技",
        localGroupId: "g-tech",
      },
    ]);
    importMocks.preview.mockResolvedValue({
      id: "preview-repeat",
      previewId: "preview-repeat",
      sourceId: "futu:default",
      remoteGroupName: "科技",
      localGroupId: "g-tech",
      added: [],
      unchanged: [{ instrumentId: "US.AAPL" }],
      localOnly: [],
    });

    const wrapper = mount(WatchlistImportDialog, {
      props: { modelValue: true },
      global: { stubs: { "v-dialog": DialogStub } },
    });
    await flushPromises();

    const selects = wrapper.findAll(".watchlist-import-dialog__form-grid select");
    expect((selects[2]!.element as HTMLSelectElement).value).toBe("g-tech");
    expect(wrapper.text()).toContain("已绑定分组");
    expect(wrapper.text()).toContain("科技股");
    await wrapper.get(".watchlist-import-dialog__primary").trigger("click");
    await flushPromises();
    expect(importMocks.preview).toHaveBeenCalledWith(
      expect.objectContaining({
        sourceId: "futu:default",
        remoteGroupId: "remote-tech",
        localGroupId: "g-tech",
      }),
    );

    vi.spyOn(window, "confirm").mockReturnValue(true);
    await wrapper.get(".watchlist-import-dialog__bindings button").trigger("click");
    await flushPromises();
    expect(importMocks.deleteBinding).toHaveBeenCalledWith("binding-1");
  });
});
