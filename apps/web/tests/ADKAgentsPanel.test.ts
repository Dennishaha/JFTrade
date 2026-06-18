// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import ADKAgentsPanel from "../src/components/adk-settings/ADKAgentsPanel.vue";
import {
  buttonStub,
  dialogStub,
  inputStub,
  passthroughStub,
  selectStub,
} from "./helpers";

afterEach(() => {
  document.body.innerHTML = "";
});

describe("ADKAgentsPanel", () => {
  it("hides sequential and parallel from the agent default work mode selector", async () => {
    const wrapper = mountAgentsPanel();

    const newButton = wrapper.findAll("button").find((button) => button.text().includes("自定义新建"));
    expect(newButton).toBeTruthy();
    await newButton!.trigger("click");

    const text = wrapper.text();
    expect(text).toContain("对话");
    expect(text).toContain("任务");
    expect(text).toContain("目标");
    expect(text).not.toContain("顺序执行");
    expect(text).not.toContain("并行分支");
    expect(text).not.toContain("目标循环最大轮次");
  });

  it("shows the saved default mode on every agent card", () => {
    const wrapper = mountAgentsPanel({
      agents: [
        buildAgent("chat"),
        buildAgent("task"),
        buildAgent("loop"),
      ],
    });

    expect(wrapper.text()).toContain("默认：对话");
    expect(wrapper.text()).toContain("默认：任务");
    expect(wrapper.text()).toContain("默认：目标");
    expect(wrapper.text()).toContain("审批制");
  });

  it("shows all supported default modes directly in the edit dialog", async () => {
    const wrapper = mountAgentsPanel({ agents: [buildAgent("chat")] });

    await wrapper.findAll("button").find((button) =>
      button.text().includes("编辑"),
    )!.trigger("click");

    expect(wrapper.text()).toContain("默认工作模式");
    const modeValues = [
      ...new Set(
        wrapper.findAll("input[type='radio']").map((input) =>
          input.attributes("value"),
        ),
      ),
    ];
    expect(modeValues).toEqual(["chat", "task", "loop"]);
    expect(wrapper.text()).not.toContain("顺序执行");
    expect(wrapper.text()).not.toContain("并行分支");
  });

  it("shows loop iterations only while the goal mode is selected", async () => {
    const wrapper = mountAgentsPanel();
    await wrapper.findAll("button").find((button) =>
      button.text().includes("自定义新建"),
    )!.trigger("click");
    const form = wrapper.props("agentForm");

    expect(wrapper.text()).not.toContain("目标循环最大轮次");
    form.workMode = "loop";
    await wrapper.vm.$nextTick();
    expect(wrapper.text()).toContain("目标循环最大轮次");

    form.workMode = "task";
    await wrapper.vm.$nextTick();
    expect(wrapper.text()).not.toContain("目标循环最大轮次");
    expect(form.loopMaxIterations).toBe(5);
  });
});

function mountAgentsPanel(
  overrides: Partial<{
    agents: ReturnType<typeof buildAgent>[];
  }> = {},
) {
  return mount(ADKAgentsPanel, {
    attachTo: document.body,
    props: {
      agentForm: {
        id: "",
        name: "新 Agent",
        instruction: "",
        providerId: "provider",
        model: "",
        tools: [],
        skills: [],
        permissionMode: "approval",
        memoryEnabled: true,
        recentUserWindow: 6,
        workMode: "chat",
        loopMaxIterations: 5,
        status: "ENABLED",
      },
      agents: overrides.agents ?? [],
      agentTemplates: [],
      agentTemplateNotice: "",
      providerOptions: [{ title: "Provider", value: "provider" }],
      toolOptions: [],
      skillOptions: [],
      permissionModes: [{ title: "审批制", value: "approval" }],
      tools: [],
      toolCategoryFilter: "",
      toolCategoryOptions: [],
      toolRiskFilter: "",
      toolRiskOptions: [],
      formatPermission: (mode: string) => mode === "approval" ? "审批制" : mode,
      riskColor: () => "default",
      riskLabel: () => "默认",
      applyAgentTemplate: vi.fn(),
      saveAgent: vi.fn(),
      newAgentForm: vi.fn(),
      editAgent: vi.fn(),
      duplicateAgent: vi.fn(),
      deleteAgent: vi.fn(),
    },
    global: {
      stubs: {
        "v-btn": buttonStub,
        "v-card": passthroughStub,
        "v-card-actions": passthroughStub,
        "v-card-title": passthroughStub,
        "v-card-text": passthroughStub,
        "v-chip": { template: "<span><slot /></span>" },
        "v-checkbox": { template: "<label><slot /></label>" },
        "v-dialog": dialogStub,
        "v-radio": {
          props: ["label", "value"],
          template: "<label><input type='radio' :value='value' />{{ label }}</label>",
        },
        "v-radio-group": {
          props: ["label"],
          template: "<fieldset><legend>{{ label }}</legend><slot /></fieldset>",
        },
        "v-select": selectStub,
        "v-switch": { template: "<label><slot /></label>" },
        "v-text-field": {
          props: ["modelValue", "label"],
          template: "<label>{{ label }}<input :value='modelValue' /></label>",
        },
        "v-textarea": inputStub,
      },
    },
  });
}

function buildAgent(workMode: "chat" | "task" | "loop") {
  return {
    id: `agent-${workMode}`,
    name: `Agent ${workMode}`,
    instruction: "",
    providerId: "provider",
    model: "",
    tools: [],
    skills: [],
    permissionMode: "approval" as const,
    memoryEnabled: true,
    recentUserWindow: 6,
    workMode,
    loopMaxIterations: 5,
    status: "ENABLED",
    createdAt: "2026-06-18T00:00:00Z",
    updatedAt: "2026-06-18T00:00:00Z",
  };
}
