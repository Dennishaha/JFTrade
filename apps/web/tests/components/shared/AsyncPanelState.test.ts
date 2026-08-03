// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent } from "vue";
import { describe, expect, it } from "vitest";

import AsyncPanelState from "../../../src/components/shared/AsyncPanelState.vue";

const passthrough = defineComponent({
  inheritAttrs: false,
  template: `<div v-bind="$attrs"><slot /></div>`,
});

function mountPanel(
  props: Record<string, unknown> = {},
  slots: Record<string, string> = {},
) {
  return mount(AsyncPanelState, {
    props,
    slots,
    global: {
      stubs: {
        "v-alert": passthrough,
        "v-progress-linear": passthrough,
      },
    },
  });
}

describe("async panel state", () => {
  it("renders an indeterminate progress bar while loading", () => {
    const wrapper = mountPanel({ loading: true });
    const bars = wrapper.findAll("[indeterminate]");
    expect(bars).toHaveLength(1);
    expect(wrapper.text()).toBe("");
  });

  it("applies the progress class override to the progress bar", () => {
    const wrapper = mountPanel({
      loading: true,
      progressClass: "panel__progress",
    });
    expect(wrapper.find(".panel__progress").exists()).toBe(true);
  });

  it("renders the error message in a warning tonal alert", () => {
    const wrapper = mountPanel({ error: "加载失败" });
    const alerts = wrapper.findAll("[type='warning'][variant='tonal']");
    expect(alerts).toHaveLength(1);
    expect(alerts[0]!.text()).toContain("加载失败");
  });

  it("renders warnings with the default warning type", () => {
    const wrapper = mountPanel({ warnings: ["提示一", "提示二"] });
    const alerts = wrapper.findAll("[type='warning'][variant='tonal']");
    expect(alerts).toHaveLength(2);
    expect(wrapper.text()).toContain("提示一");
    expect(wrapper.text()).toContain("提示二");
  });

  it("allows switching warnings to the info type", () => {
    const wrapper = mountPanel({
      warnings: ["提示"],
      warningType: "info",
    });
    expect(wrapper.findAll("[type='info'][variant='tonal']")).toHaveLength(1);
  });

  it("renders partial errors with scope and message by default", () => {
    const wrapper = mountPanel({
      partialErrors: [{ scope: "US.AAPL", code: "DENIED", message: "无权限" }],
    });
    const alerts = wrapper.findAll("[type='warning'][variant='outlined']");
    expect(alerts).toHaveLength(1);
    expect(alerts[0]!.text()).toBe("US.AAPL · 无权限");
  });

  it("supports overriding the partial error rendering via slot", () => {
    const wrapper = mountPanel(
      {
        partialErrors: [{ scope: "US.AAPL", code: "DENIED", message: "无权限" }],
      },
      {
        "partial-error": `<template #partial-error="{ partialError }">{{ partialError.scope }} · {{ partialError.code }} · {{ partialError.message }}</template>`,
      },
    );
    expect(wrapper.text()).toContain("US.AAPL · DENIED · 无权限");
  });

  it("renders the default slot between the error alert and warnings", () => {
    const wrapper = mountPanel(
      { error: "出错了", warnings: ["警告"] },
      { default: `<p class="extra">附加内容</p>` },
    );
    const text = wrapper.text();
    expect(text.indexOf("出错了")).toBeLessThan(text.indexOf("附加内容"));
    expect(text.indexOf("附加内容")).toBeLessThan(text.indexOf("警告"));
  });

  it("renders nothing when idle", () => {
    const wrapper = mountPanel();
    expect(wrapper.text()).toBe("");
    expect(wrapper.findAll("[type]")).toHaveLength(0);
    expect(wrapper.findAll("[indeterminate]")).toHaveLength(0);
  });
});
