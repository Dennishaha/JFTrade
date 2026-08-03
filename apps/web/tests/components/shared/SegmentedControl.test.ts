// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import SegmentedControl from "@/components/shared/SegmentedControl.vue";

const items = [
  { value: "all", label: "全部", testId: "all-segment" },
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
  });

  it("selects enabled choices by click and arrow keys", async () => {
    const wrapper = mount(SegmentedControl, {
      props: { modelValue: "all", items, label: "状态筛选" },
    });
    const buttons = wrapper.findAll("button");

    await buttons[1]?.trigger("click");
    await buttons[0]?.trigger("keydown", { key: "ArrowLeft" });
    await buttons[2]?.trigger("click");

    expect(wrapper.emitted("update:modelValue")).toEqual([["active"], ["active"]]);
  });
});
