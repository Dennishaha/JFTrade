// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import BrokerUnlockDialog from "@/components/workspace/BrokerUnlockDialog.vue";

describe("BrokerUnlockDialog", () => {
  it("renders broker id and prompts for password when open", () => {
    const wrapper = mount(BrokerUnlockDialog, {
      props: {
        brokerId: "futu",
        modelValue: true,
      },
    });

    expect(wrapper.text()).toContain("FUTU");
    expect(wrapper.text()).toContain("解锁券商交易权限");
    expect(wrapper.find("input[type='password']").exists()).toBe(true);
  });

  it("disables the confirm button when the password field is empty", () => {
    const wrapper = mount(BrokerUnlockDialog, {
      props: {
        brokerId: "futu",
        modelValue: true,
      },
    });

    const confirmBtn = wrapper.find(".tv-broker-unlock__btn--confirm");
    expect(confirmBtn.attributes("disabled")).toBeDefined();
  });

  it("enables confirm button when password is typed and emits submit on click", async () => {
    const wrapper = mount(BrokerUnlockDialog, {
      props: {
        brokerId: "futu",
        modelValue: true,
      },
    });

    const input = wrapper.get("input[type='password']");
    await input.setValue("trading_secret_123");

    const confirmBtn = wrapper.get(".tv-broker-unlock__btn--confirm");
    expect(confirmBtn.attributes("disabled")).toBeUndefined();

    await confirmBtn.trigger("click");
    expect(wrapper.emitted("submit")).toBeDefined();
    expect(wrapper.emitted("submit")![0]).toEqual(["trading_secret_123"]);

    // Input must be cleared after submit to avoid retained credentials
    expect((input.element as HTMLInputElement).value).toBe("");
  });

  it("emits cancel and closes dialog when cancel button is clicked", async () => {
    const wrapper = mount(BrokerUnlockDialog, {
      props: {
        brokerId: "futu",
        modelValue: true,
      },
    });

    const cancelBtn = wrapper.get(".tv-broker-unlock__btn--cancel");
    await cancelBtn.trigger("click");

    expect(wrapper.emitted("cancel")).toBeDefined();
    expect(wrapper.emitted("update:modelValue")).toBeDefined();
    expect(wrapper.emitted("update:modelValue")![0]).toEqual([false]);
  });

  it("displays error message alert when error prop is provided", () => {
    const wrapper = mount(BrokerUnlockDialog, {
      props: {
        brokerId: "futu",
        error: "交易密码错误，请重新输入",
        modelValue: true,
      },
    });

    const errorAlert = wrapper.find(".tv-broker-unlock__error");
    expect(errorAlert.exists()).toBe(true);
    expect(errorAlert.text()).toContain("交易密码错误，请重新输入");
  });

  it("shows unlocking busy state and disables buttons while unlocking is in flight", () => {
    const wrapper = mount(BrokerUnlockDialog, {
      props: {
        brokerId: "futu",
        modelValue: true,
        unlocking: true,
      },
    });

    const confirmBtn = wrapper.get(".tv-broker-unlock__btn--confirm");
    expect(confirmBtn.text()).toContain("解锁中...");
    expect(confirmBtn.attributes("disabled")).toBeDefined();

    const cancelBtn = wrapper.get(".tv-broker-unlock__btn--cancel");
    expect(cancelBtn.attributes("disabled")).toBeDefined();
  });
});
