// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent } from "vue";
import { describe, expect, it } from "vitest";

import AiAssistantPanel from "../src/layout/AiAssistantPanel.vue";

describe("AI assistant dock panel", () => {
  it("uses the mobile workspace shell variant inside the right dock", () => {
    const shell = defineComponent({
      props: ["layout"],
      template: "<div data-testid='workspace-shell'>{{ layout }}</div>",
    });
    const wrapper = mount(AiAssistantPanel, {
      global: { stubs: { ADKWorkspaceShell: shell } },
    });

    expect(wrapper.get("[data-testid='workspace-shell']").text()).toBe("mobile");
  });
});
