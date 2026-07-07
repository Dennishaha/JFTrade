// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import ADKAgentsPanel from "../src/components/adk-settings/ADKAgentsPanel.vue";
import type { ADKAgent } from "../src/contracts";
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
  it("opens the new-agent dialog from the custom create button", async () => {
    const newAgentForm = vi.fn();
    const wrapper = mountAgentsPanel({ newAgentForm });

    expect(wrapper.find(".v-dialog-stub").exists()).toBe(false);
    await wrapper.findAll("button").find((button) => button.text().includes("自定义新建"))!.trigger("click");

    expect(newAgentForm).toHaveBeenCalled();
    expect(wrapper.text()).toContain("新建智能体");
    expect(wrapper.text()).toContain("保存智能体");
  });

  it("opens templates and applies the selected template", async () => {
    const applyAgentTemplate = vi.fn();
    const template = {
      ...buildAgent("loop"),
      id: "template-strategy",
      name: "策略模板",
      tools: [],
      skills: ["jftrade-market"],
    };
    const wrapper = mountAgentsPanel({
      agentTemplates: [template],
      applyAgentTemplate,
    });

    await wrapper.findAll("button").find((button) => button.text().includes("从模板新建"))!.trigger("click");
    expect(wrapper.text()).toContain("选择智能体模板");
    expect(wrapper.text()).toContain("策略模板");

    await wrapper.findAll("button").find((button) => button.text().includes("策略模板"))!.trigger("click");

    expect(applyAgentTemplate).toHaveBeenCalledWith(template);
    expect(wrapper.text()).toContain("新建智能体");
  });

  it("hides sequential and parallel from the agent default work mode selector", async () => {
    const wrapper = mountAgentsPanel();

    const newButton = wrapper.findAll("button").find((button) => button.text().includes("自定义新建"));
    expect(newButton).toBeTruthy();
    await newButton!.trigger("click");

    const text = wrapper.text();
    expect(text).toContain("对话");
    expect(text).toContain("目标");
    expect(text).not.toContain("任务");
    expect(text).not.toContain("顺序执行");
    expect(text).not.toContain("并行分支");
    expect(text).not.toContain("目标循环最大轮次");
  });

  it("shows the saved default mode on every agent card", () => {
    const wrapper = mountAgentsPanel({
      agents: [
        buildAgent("chat"),
        buildAgent("loop"),
        buildAgent("loop"),
      ],
    });

    expect(wrapper.text()).toContain("默认：对话");
    expect(wrapper.text()).toContain("默认：目标");
    expect(wrapper.text()).not.toContain("默认：任务");
    expect(wrapper.text()).toContain("审批制");
  });

  it("marks builtin agents, shows all tools, and hides delete", () => {
    const wrapper = mountAgentsPanel({
      agents: [
        {
          ...buildAgent("chat"),
          id: "jftrade-default",
          name: "默认助手",
          builtin: true,
          tools: [],
        },
      ],
    });

    expect(wrapper.text()).toContain("系统默认");
    expect(wrapper.text()).toContain("全部工具");
    expect(wrapper.findAll("button").some((button) => button.text().includes("删除"))).toBe(false);
    expect(wrapper.findAll("button").some((button) => button.text().includes("编辑"))).toBe(false);
  });

  it("sorts the primary default agent first", () => {
    const wrapper = mountAgentsPanel({
      agents: [
        {
          ...buildAgent("loop"),
          id: "custom-agent",
          name: "Custom Agent",
        },
        {
          ...buildAgent("chat"),
          id: "jftrade-default",
          name: "默认助手",
          builtin: true,
          status: "ENABLED",
        },
      ],
    });

    const text = wrapper.text();
    expect(text.indexOf("默认助手")).toBeGreaterThanOrEqual(0);
    expect(text.indexOf("默认助手")).toBeLessThan(text.indexOf("Custom Agent"));
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
    expect(modeValues).toEqual(["chat", "loop"]);
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

    form.workMode = "chat";
    await wrapper.vm.$nextTick();
    expect(wrapper.text()).not.toContain("目标循环最大轮次");
    expect(form.loopMaxIterations).toBe(5);
  });
});

function mountAgentsPanel(
  overrides: Partial<{
    agents: ADKAgent[];
    agentForm: Partial<ADKAgent>;
    agentTemplates: Array<Omit<ADKAgent, "createdAt" | "updatedAt">>;
    applyAgentTemplate: ReturnType<typeof vi.fn>;
    newAgentForm: ReturnType<typeof vi.fn>;
  }> = {},
) {
  const agentForm = {
    id: "",
    name: "新 Agent",
    instruction: "",
    providerId: "provider",
    model: "",
    tools: [],
    skills: [],
    permissionMode: "approval" as const,
    memoryEnabled: true,
    recentUserWindow: 6,
    workMode: "chat" as const,
    loopMaxIterations: 5,
    status: "ENABLED" as const,
    ...overrides.agentForm,
  };
  return mount(ADKAgentsPanel, {
    attachTo: document.body,
    props: {
      agentForm,
      agents: overrides.agents ?? [],
      agentTemplates: overrides.agentTemplates ?? [],
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
      applyAgentTemplate: overrides.applyAgentTemplate ?? vi.fn(),
      saveAgent: vi.fn(),
      newAgentForm: overrides.newAgentForm ?? vi.fn(),
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
        "v-switch": {
          props: ["modelValue", "label", "disabled"],
          template: "<label>{{ label }}<input type='checkbox' :data-switch-label='label' :checked=\"modelValue === true || modelValue === 'ENABLED'\" :disabled='disabled' /></label>",
        },
        "v-text-field": {
          props: ["modelValue", "label"],
          template: "<label>{{ label }}<input :value='modelValue' /></label>",
        },
        "v-textarea": inputStub,
      },
    },
  });
}

function buildAgent(workMode: "chat" | "loop"): ADKAgent {
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
