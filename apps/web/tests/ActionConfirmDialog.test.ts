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

  it("shows a custom busy label when provided", () => {
    const wrapper = mount(ActionConfirmDialog, {
      props: {
        busy: true,
        busyLabel: "正在安排…",
        message: "安排后无法撤销。",
        open: true,
        title: "确认重建",
      },
    });

    expect(wrapper.get('[data-testid="action-confirm-submit"]').text()).toBe("正在安排…");
  });

  it("keeps the confirm button disabled until the typed confirmation matches", async () => {
    const wrapper = mount(ActionConfirmDialog, {
      props: {
        confirmationText: "COMPACT strategy",
        confirmLabel: "确认整理",
        message: "将执行 WAL checkpoint 和 VACUUM。",
        open: true,
        title: "整理 strategy",
      },
    });

    const submit = wrapper.get('[data-testid="action-confirm-submit"]');
    expect(submit.attributes("disabled")).toBeDefined();
    expect(wrapper.text()).toContain("COMPACT strategy");

    const input = wrapper.get('[data-testid="action-confirm-confirmation-input"]');
    await input.setValue("COMPACT other");
    expect(submit.attributes("disabled")).toBeDefined();
    await submit.trigger("click");
    expect(wrapper.emitted("confirm")).toBeUndefined();

    await input.setValue("COMPACT strategy");
    expect(wrapper.get('[data-testid="action-confirm-submit"]').attributes("disabled")).toBeUndefined();
    await wrapper.get('[data-testid="action-confirm-submit"]').trigger("click");
    expect(wrapper.emitted("confirm")).toEqual([["COMPACT strategy"]]);
  });

  it("submits the typed confirmation with the Enter key", async () => {
    const wrapper = mount(ActionConfirmDialog, {
      props: {
        confirmationText: "REBUILD ALL",
        message: "无法撤销。",
        open: true,
        title: "重建数据库",
      },
    });

    const input = wrapper.get('[data-testid="action-confirm-confirmation-input"]');
    await input.setValue("REBUILD ALL");
    await input.trigger("keydown", { key: "Enter" });
    expect(wrapper.emitted("confirm")).toEqual([["REBUILD ALL"]]);
  });

  it("resets the typed confirmation each time the dialog reopens", async () => {
    const wrapper = mount(ActionConfirmDialog, {
      props: {
        confirmationText: "COMPACT strategy",
        message: "将执行 WAL checkpoint 和 VACUUM。",
        open: true,
        title: "整理 strategy",
      },
    });

    await wrapper.get('[data-testid="action-confirm-confirmation-input"]').setValue("COMPACT strategy");
    await wrapper.setProps({ open: false });
    await wrapper.setProps({ open: true });

    const input = wrapper.get('[data-testid="action-confirm-confirmation-input"]');
    expect((input.element as HTMLInputElement).value).toBe("");
    expect(wrapper.get('[data-testid="action-confirm-submit"]').attributes("disabled")).toBeDefined();
  });
});
