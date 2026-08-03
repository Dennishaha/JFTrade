// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import AppTabs from "@/components/shared/AppTabs.vue";

const items = [
  { value: "overview", label: "概览", icon: "fa-solid fa-chart-line", count: 3, testId: "overview-tab" },
  { value: "orders", label: "订单", surfaceId: "orders-surface" },
  { value: "disabled", label: "停用", disabled: true },
] as const;

describe("AppTabs", () => {
  it("renders tab semantics and item metadata", () => {
    const wrapper = mount(AppTabs, {
      props: { modelValue: "overview", items, label: "报告视图" },
    });

    expect(wrapper.get('[role="tablist"]').attributes("aria-label")).toBe("报告视图");
    const tabs = wrapper.findAll('[role="tab"]');
    expect(tabs[0]?.attributes()).toMatchObject({ "aria-selected": "true", tabindex: "0" });
    expect(tabs[1]?.attributes()).toMatchObject({
      "aria-selected": "false",
      tabindex: "-1",
      "data-capability-surface": "orders-surface",
    });
    expect(wrapper.get('[data-testid="overview-tab"]').text()).toContain("3");
    expect(wrapper.get('[data-testid="overview-tab"] i').classes()).toEqual(
      expect.arrayContaining(["fa-solid", "fa-chart-line"]),
    );
    expect(tabs[2]?.attributes("disabled")).toBeDefined();
  });

  it("selects enabled tabs and ignores disabled tabs", async () => {
    const wrapper = mount(AppTabs, {
      props: { modelValue: "overview", items, label: "报告视图" },
    });

    await wrapper.findAll('[role="tab"]')[1]?.trigger("click");
    await wrapper.findAll('[role="tab"]')[2]?.trigger("click");
    expect(wrapper.emitted("update:modelValue")).toEqual([["orders"]]);
  });

  it("cycles enabled tabs with every navigation key and ignores unrelated keys", async () => {
    const wrapper = mount(AppTabs, {
      attachTo: document.body,
      props: { modelValue: "overview", items, label: "报告视图" },
    });
    const tabs = wrapper.findAll('[role="tab"]');

    await tabs[0]?.trigger("keydown", { key: "ArrowRight" });
    await tabs[0]?.trigger("keydown", { key: "ArrowLeft" });
    await tabs[0]?.trigger("keydown", { key: "End" });
    await tabs[1]?.trigger("keydown", { key: "Home" });
    await tabs[0]?.trigger("keydown", { key: "Enter" });
    await tabs[2]?.trigger("keydown", { key: "ArrowRight" });

    expect(wrapper.emitted("update:modelValue")).toEqual([
      ["orders"],
      ["orders"],
      ["orders"],
      ["overview"],
    ]);
    wrapper.unmount();
  });

  it("fails closed when keyboard targets leave the managed tablist", async () => {
    const wrapper = mount(AppTabs, {
      attachTo: document.body,
      props: { modelValue: "overview", items, label: "报告视图" },
    });
    const tabs = wrapper.findAll<HTMLButtonElement>('[role="tab"]');

    delete tabs[1]!.element.dataset.tabValue;
    await tabs[0]!.trigger("keydown", { key: "ArrowRight" });
    expect(wrapper.emitted("update:modelValue")).toBeUndefined();

    const detached = tabs[0]!.element;
    detached.remove();
    detached.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }));
    expect(wrapper.emitted("update:modelValue")).toBeUndefined();
    wrapper.unmount();
  });
});
