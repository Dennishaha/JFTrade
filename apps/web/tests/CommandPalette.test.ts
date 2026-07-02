// @vitest-environment jsdom

import { mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick } from "vue";

import { provideCommandPaletteStore } from "../src/composables/useCommandPalette";
import CommandPalette from "../src/layout/CommandPalette.vue";

const wrappers: VueWrapper[] = [];

function mountCommandPalette(input?: {
  actions?: Array<{
    id: string;
    label: string;
    group: string;
    hint?: string;
    keywords?: string[];
    run?: () => void;
  }>;
}) {
  const runById = new Map<string, ReturnType<typeof vi.fn>>();
  let paletteStore: ReturnType<typeof provideCommandPaletteStore> | undefined;

  const Host = defineComponent({
    setup() {
      paletteStore = provideCommandPaletteStore();
      for (const action of input?.actions ?? []) {
        const run = vi.fn();
        runById.set(action.id, run);
        paletteStore.register({
          ...action,
          run: action.run ?? run,
        });
      }
      return () => h(CommandPalette);
    },
  });

  const wrapper = mount(Host, {
    attachTo: document.body,
  });
  wrappers.push(wrapper);
  if (paletteStore == null) throw new Error("palette store not initialized");
  return {
    wrapper,
    paletteStore,
    runById,
  };
}

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) {
    wrapper.unmount();
  }
  document.body.innerHTML = "";
});

describe("CommandPalette", () => {
  it("filters actions, clamps the active row, and runs the selected action", async () => {
    const { wrapper, paletteStore, runById } = mountCommandPalette({
      actions: [
        {
          id: "nav.account",
          label: "打开我的账户",
          group: "导航",
          hint: "/account",
          keywords: ["portfolio", "broker"],
        },
        {
          id: "nav.backtest",
          label: "打开回测",
          group: "导航",
          hint: "/backtest",
          keywords: ["strategy"],
        },
        {
          id: "action.refresh",
          label: "刷新控制台状态",
          group: "操作",
          hint: "重新加载系统与存储概览",
          keywords: ["reload", "system"],
        },
      ],
    });

    paletteStore.show();
    await nextTick();
    await nextTick();

    const input = wrapper.get("input");
    expect(document.activeElement).toBe(input.element);
    await input.setValue("strategy");
    expect(wrapper.text()).toContain("打开回测");
    expect(wrapper.text()).not.toContain("打开我的账户");

    await input.setValue("");
    const panel = wrapper.get(".tv-palette");
    await panel.trigger("keydown", { key: "ArrowDown" });
    await panel.trigger("keydown", { key: "ArrowDown" });
    const rows = wrapper.findAll("li");
    expect(rows[2]?.classes()).toContain("is-active");
    await panel.trigger("keydown", { key: "ArrowUp" });
    expect(wrapper.findAll("li")[1]?.classes()).toContain("is-active");

    await input.setValue("reload");
    expect(wrapper.findAll("li")).toHaveLength(1);
    expect(wrapper.get("li").classes()).toContain("is-active");

    await panel.trigger("keydown", { key: "Enter" });
    expect(runById.get("action.refresh")).toHaveBeenCalledTimes(1);
    expect(wrapper.find(".tv-palette-backdrop").exists()).toBe(false);
  });

  it("supports mouse selection, empty states, and close gestures", async () => {
    const { wrapper, paletteStore, runById } = mountCommandPalette({
      actions: [
        {
          id: "nav.docs",
          label: "打开文档",
          group: "导航",
          hint: "/docs",
        },
        {
          id: "nav.system",
          label: "打开系统",
          group: "导航",
          hint: "/system",
        },
      ],
    });

    paletteStore.show();
    await nextTick();

    const rows = wrapper.findAll("li");
    await rows[1]!.trigger("mouseenter");
    expect(rows[1]!.classes()).toContain("is-active");
    await rows[1]!.trigger("mousedown");
    expect(runById.get("nav.system")).toHaveBeenCalledTimes(1);
    expect(wrapper.find(".tv-palette-backdrop").exists()).toBe(false);

    paletteStore.show();
    await nextTick();
    await wrapper.get("input").setValue("missing-command");
    expect(wrapper.text()).toContain("没有匹配项");

    const backdrop = wrapper.get(".tv-palette-backdrop");
    await backdrop.trigger("mousedown");
    expect(wrapper.find(".tv-palette-backdrop").exists()).toBe(false);

    paletteStore.show();
    await nextTick();
    await wrapper.get(".tv-palette").trigger("keydown", { key: "Escape" });
    expect(wrapper.find(".tv-palette-backdrop").exists()).toBe(false);
  });

  it("toggles from the global keyboard shortcut and removes the listener on unmount", async () => {
    const { wrapper } = mountCommandPalette({
      actions: [
        {
          id: "nav.workspace",
          label: "打开交易工作台",
          group: "导航",
          hint: "/workspace",
        },
      ],
    });

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "k", metaKey: true }));
    await nextTick();
    expect(wrapper.find(".tv-palette-backdrop").exists()).toBe(true);

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "K", ctrlKey: true }));
    await nextTick();
    expect(wrapper.find(".tv-palette-backdrop").exists()).toBe(false);

    wrapper.unmount();
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "k", metaKey: true }));
    await nextTick();
    expect(document.querySelector(".tv-palette-backdrop")).toBeNull();
  });
});
