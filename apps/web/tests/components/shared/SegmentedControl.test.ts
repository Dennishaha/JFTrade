// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import SegmentedControl from "@/components/shared/SegmentedControl.vue";

const items = [
  { value: "all", label: "全部", icon: "fa-solid fa-list", count: 4, testId: "all-segment" },
  { value: "active", label: "活跃" },
  { value: "disabled", label: "停用", disabled: true },
] as const;

describe("SegmentedControl", () => {
  it("uses pressed-button semantics instead of tab semantics", () => {
    const wrapper = mount(SegmentedControl, {
      props: { modelValue: "all", items, label: "状态筛选" },
    });

    expect(wrapper.get('[role="group"]').attributes("aria-label")).toBe("状态筛选");
    expect(wrapper.find('[role="tablist"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="all-segment"]').attributes("aria-pressed")).toBe("true");
    expect(wrapper.get('[data-testid="all-segment"] i').classes()).toContain("fa-list");
    expect(wrapper.get(".segmented-control__count").text()).toBe("4");
  });

  it("selects enabled choices by click and arrow keys", async () => {
    const wrapper = mount(SegmentedControl, {
      props: { modelValue: "all", items, label: "状态筛选" },
    });
    const buttons = wrapper.findAll("button");

    await buttons[1]?.trigger("click");
    await buttons[0]?.trigger("keydown", { key: "ArrowRight" });
    await buttons[0]?.trigger("keydown", { key: "ArrowLeft" });
    await buttons[0]?.trigger("keydown", { key: "Enter" });
    await buttons[2]?.trigger("keydown", { key: "ArrowRight" });
    await buttons[2]?.trigger("click");

    expect(wrapper.emitted("update:modelValue")).toEqual([
      ["active"],
      ["active"],
      ["active"],
    ]);
  });

  it("fails closed when keyboard targets leave the managed group", async () => {
    const wrapper = mount(SegmentedControl, {
      attachTo: document.body,
      props: { modelValue: "all", items, label: "状态筛选" },
    });
    const buttons = wrapper.findAll<HTMLButtonElement>("button");

    delete buttons[1]!.element.dataset.segmentValue;
    await buttons[0]!.trigger("keydown", { key: "ArrowRight" });
    expect(wrapper.emitted("update:modelValue")).toBeUndefined();

    const detached = buttons[0]!.element;
    detached.remove();
    detached.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }));
    expect(wrapper.emitted("update:modelValue")).toBeUndefined();
    wrapper.unmount();
  });
});
