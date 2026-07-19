// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ActionConfirmDialog from "../src/components/shared/ActionConfirmDialog.vue";

describe("ActionConfirmDialog", () => {
  it("emits close and confirm from every available dismissal action", async () => {
    const wrapper = mount(ActionConfirmDialog, {
      props: {
        message: "该操作会撤销未完成订单。",
        open: true,
        title: "确认撤单",
      },
    });

    expect(wrapper.text()).toContain("确认");
    await wrapper.get('[aria-label="关闭确认弹窗"]').trigger("click");
    await wrapper.get(".action-confirm__actions button").trigger("click");
    await wrapper.get('[data-testid="action-confirm-submit"]').trigger("click");
    await wrapper.get('[role="dialog"]').trigger("click");

    expect(wrapper.emitted("close")).toHaveLength(3);
    expect(wrapper.emitted("confirm")).toHaveLength(1);
  });

  it("blocks dismissal while the confirmed action is busy", async () => {
    const wrapper = mount(ActionConfirmDialog, {
      props: {
        busy: true,
        confirmLabel: "确认删除",
        message: "删除后无法恢复。",
        open: true,
        title: "确认删除",
      },
    });

    expect(wrapper.text()).toContain("正在处理…");
    expect(
      wrapper
        .findAll("button")
        .every((button) => button.attributes("disabled") !== undefined),
    ).toBe(true);
    await wrapper.get('[role="dialog"]').trigger("click");
    expect(wrapper.emitted("close")).toBeUndefined();
    expect(wrapper.emitted("confirm")).toBeUndefined();
  });
});
