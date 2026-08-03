// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import EmptyState from "../../../src/components/shared/EmptyState.vue";

function mountState(
  props: Record<string, unknown> = {},
  options: { slot?: string; class?: string } = {},
) {
  return mount(EmptyState, {
    props,
    attrs: options.class ? { class: options.class } : {},
    slots: options.slot ? { default: options.slot } : {},
  });
}

describe("empty state panel", () => {
  it("shows the default loading label while loading", () => {
    const wrapper = mountState({ loading: true });
    expect(wrapper.text()).toBe("加载中…");
  });

  it("supports a custom loading label", () => {
    const wrapper = mountState({
      loading: true,
      loadingLabel: "派息日历加载中…",
    });
    expect(wrapper.text()).toBe("派息日历加载中…");
  });

  it("shows the error message when present", () => {
    const wrapper = mountState({ error: "上游请求失败" });
    expect(wrapper.text()).toBe("上游请求失败");
  });

  it("shows the default empty label when empty", () => {
    const wrapper = mountState({ empty: true });
    expect(wrapper.text()).toBe("暂无数据");
  });

  it("supports a custom empty label", () => {
    const wrapper = mountState({
      empty: true,
      emptyLabel: "当前 OpenD 未返回港股通相关板块",
    });
    expect(wrapper.text()).toBe("当前 OpenD 未返回港股通相关板块");
  });

  it("renders slot content when no state is active", () => {
    const wrapper = mountState({}, { slot: `<table class="data"></table>` });
    expect(wrapper.find("table.data").exists()).toBe(true);
    expect(wrapper.text()).not.toContain("加载中");
    expect(wrapper.text()).not.toContain("暂无数据");
  });

  it("prioritizes loading over error and empty states", () => {
    const wrapper = mountState({
      loading: true,
      error: "失败",
      empty: true,
    });
    expect(wrapper.text()).toBe("加载中…");
  });

  it("prioritizes error over the empty state", () => {
    const wrapper = mountState({ error: "失败", empty: true });
    expect(wrapper.text()).toBe("失败");
  });

  it("keeps caller classes and applies variant styles on the status div", () => {
    const wrapper = mountState(
      { loading: true, bordered: true, grow: true, minHeight: 96 },
      { class: "view__status" },
    );
    const status = wrapper.find(".view__status");
    expect(status.exists()).toBe(true);
    expect(status.classes()).toContain("empty-state");
    expect(status.classes()).toContain("empty-state--bordered");
    expect(status.classes()).toContain("empty-state--grow");
    expect(status.attributes("style")).toContain("min-height: 96px");
  });

  it("uses a 120px min height by default without variant classes", () => {
    const wrapper = mountState({ empty: true });
    const status = wrapper.find(".empty-state");
    expect(status.attributes("style")).toContain("min-height: 120px");
    expect(status.classes()).not.toContain("empty-state--bordered");
    expect(status.classes()).not.toContain("empty-state--grow");
  });
});
