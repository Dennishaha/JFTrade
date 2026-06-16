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
    const wrapper = mount(ADKAgentsPanel, {
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
        agents: [],
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
        formatPermission: (mode: string) => mode,
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
          "v-select": selectStub,
          "v-switch": { template: "<label><slot /></label>" },
          "v-text-field": inputStub,
          "v-textarea": inputStub,
        },
      },
    });

    const newButton = wrapper.findAll("button").find((button) => button.text().includes("自定义新建"));
    expect(newButton).toBeTruthy();
    await newButton!.trigger("click");

    const text = wrapper.text();
    expect(text).toContain("单轮对话");
    expect(text).toContain("任务编排");
    expect(text).toContain("目标循环");
    expect(text).not.toContain("顺序执行");
    expect(text).not.toContain("并行分支");
  });
});
