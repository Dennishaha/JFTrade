// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { defineComponent } from "vue";

import ADKSkillsPanel from "../src/components/adk-settings/ADKSkillsPanel.vue";

const safeButtonStub = defineComponent({
  props: ["disabled", "title"],
  emits: ["click"],
  template:
    "<button type='button' :disabled='disabled' :title='title' @click=\"$emit('click')\"><slot /></button>",
});

describe("ADKSkillsPanel", () => {
  it("submits install requests and emits the live skill url draft", async () => {
    const installSkill = vi.fn();
    const wrapper = mount(ADKSkillsPanel, {
      props: {
        skillUrl: "",
        skills: [],
        isInternalSkill: () => false,
        installSkill,
        uninstallSkill: vi.fn(),
      },
      global: {
        stubs: {
          "v-alert": { template: "<div><slot /></div>" },
          "v-btn": safeButtonStub,
          "v-card": { template: "<div><slot /></div>" },
          "v-card-text": { template: "<div><slot /></div>" },
          "v-card-title": { template: "<div><slot /></div>" },
          "v-text-field": {
            props: ["modelValue"],
            emits: ["update:modelValue"],
            template:
              "<input :value='modelValue' @input=\"$emit('update:modelValue', $event.target.value)\" />",
          },
        },
      },
    });

    expect(wrapper.find("button").attributes("disabled")).toBeDefined();
    await wrapper.find("input").setValue("https://skills.example/alpha");
    expect(wrapper.emitted("update:skillUrl")?.[0]).toEqual([
      "https://skills.example/alpha",
    ]);

    await wrapper.find("form").trigger("submit");
    expect(installSkill).toHaveBeenCalledOnce();
  });

  it("marks builtin skills as non-removable and only uninstalls external skills", async () => {
    const uninstallSkill = vi.fn();
    const wrapper = mount(ADKSkillsPanel, {
      props: {
        skillUrl: "https://skills.example/beta",
        skills: [
          {
            id: "builtin",
            displayName: "内置技能",
            description: "系统自带",
            source: "builtin",
            version: "1",
            installPath: "",
            createdAt: "2026-07-01T00:00:00Z",
            updatedAt: "2026-07-01T00:00:00Z",
          },
          {
            id: "external",
            displayName: "外部技能",
            description: "仓库安装",
            source: "https://skills.example/external",
            version: "2",
            installPath: "",
            createdAt: "2026-07-01T00:00:00Z",
            updatedAt: "2026-07-01T00:00:00Z",
          },
        ],
        isInternalSkill: (skill: { id: string }) => skill.id === "builtin",
        installSkill: vi.fn(),
        uninstallSkill,
      },
      global: {
        stubs: {
          "v-alert": { template: "<div><slot /></div>" },
          "v-btn": safeButtonStub,
          "v-card": { template: "<div><slot /></div>" },
          "v-card-text": { template: "<div><slot /></div>" },
          "v-card-title": { template: "<div><slot /></div>" },
          "v-chip": { template: "<span><slot /></span>" },
          "v-text-field": {
            props: ["modelValue"],
            emits: ["update:modelValue"],
            template:
              "<input :value='modelValue' @input=\"$emit('update:modelValue', $event.target.value)\" />",
          },
        },
      },
    });

    expect(wrapper.text()).toContain("内部来源");
    expect(wrapper.text()).toContain("外部");
    expect(wrapper.text()).toContain("v2");
    const buttons = wrapper.findAll("button").filter((button) =>
      button.text().includes("卸载") || button.text().includes("不可卸载"),
    );
    expect(buttons[0]!.attributes("disabled")).toBeDefined();
    expect(buttons[0]!.attributes("title")).toContain("不允许卸载");

    await buttons[1]!.trigger("click");
    expect(uninstallSkill).toHaveBeenCalledWith(
      expect.objectContaining({ id: "external" }),
    );
  });
});
