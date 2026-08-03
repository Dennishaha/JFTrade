// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ActionConfirmationHost from "../../src/components/shared/ActionConfirmationHost.vue";
import { useActionConfirmation } from "@/composables/shared/useActionConfirmation";

function mountHost() {
  const controller = useActionConfirmation();
  const wrapper = mount(ActionConfirmationHost, {
    props: { controller },
  });
  return { controller, wrapper };
}

describe("useActionConfirmation", () => {
  it("resolves with an empty string when a plain confirmation is accepted", async () => {
    const { controller, wrapper } = mountHost();

    const pending = controller.requestConfirmation({
      title: "删除工作流",
      message: "删除工作流「复盘」？",
      confirmLabel: "删除",
    });
    await wrapper.vm.$nextTick();

    expect(controller.confirmationOpen.value).toBe(true);
    expect(wrapper.text()).toContain("删除工作流「复盘」？");
    expect(wrapper.get('[data-testid="action-confirm-submit"]').text()).toBe("删除");

    await wrapper.get('[data-testid="action-confirm-submit"]').trigger("click");

    await expect(pending).resolves.toBe("");
    expect(controller.confirmationOpen.value).toBe(false);
  });

  it("resolves with null when the confirmation is cancelled", async () => {
    const { controller, wrapper } = mountHost();

    const pending = controller.requestConfirmation({
      title: "删除预设",
      message: "删除预设“低估值”？",
    });
    await wrapper.vm.$nextTick();
    await wrapper.get('[data-testid="action-confirm-cancel"]').trigger("click");

    await expect(pending).resolves.toBeNull();
    expect(controller.pendingConfirmation.value).toBeNull();
  });

  it("passes the confirmation input through for typed confirmations", async () => {
    const { controller, wrapper } = mountHost();

    const pending = controller.requestConfirmation({
      title: "整理 strategy",
      message: "将执行 WAL checkpoint 和 VACUUM。",
      confirmationText: "COMPACT strategy",
    });
    await wrapper.vm.$nextTick();

    const submit = wrapper.get('[data-testid="action-confirm-submit"]');
    expect(submit.attributes("disabled")).toBeDefined();

    await wrapper.get('[data-testid="action-confirm-confirmation-input"]').setValue("COMPACT strategy");
    await wrapper.get('[data-testid="action-confirm-submit"]').trigger("click");

    await expect(pending).resolves.toBe("COMPACT strategy");
  });
});
