// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";

import ADKToolsPanel from "../src/components/adk-settings/ADKToolsPanel.vue";
import {
  buttonStub,
  dialogStub,
  inputStub,
  passthroughStub,
  selectStub,
} from "./helpers";

describe("ADKToolsPanel", () => {
  it("emits filter updates and opens tool detail from the list", async () => {
    const openToolDetail = vi.fn();

    const wrapper = mount(ADKToolsPanel, {
      props: {
        tools: [buildTool()],
        filteredTools: [buildTool({ requiredSkill: "jftrade-workflow-management" })],
        selectedTool: null,
        toolCategoryFilter: "",
        toolCategoryOptions: ["system"],
        toolRiskFilter: "",
        toolRiskOptions: ["low"],
        toolSearchQuery: "",
        toolDetailDialogOpen: false,
        preview: previewJSON,
        formatPermissionMode: formatPermissionMode,
        riskColor: () => "success",
        riskLabel: () => "低风险",
        openToolDetail,
        closeToolDetail: vi.fn(),
      },
      global: {
        stubs: {
          "v-btn": buttonStub,
          "v-card": { template: "<section><slot /></section>" },
          "v-card-actions": passthroughStub,
          "v-card-title": { template: "<div><slot /></div>" },
          "v-card-text": { template: "<div><slot /></div>" },
          "v-chip": { template: "<span><slot /></span>" },
          "v-dialog": dialogStub,
          "v-select": selectStub,
          "v-text-field": inputStub,
        },
      },
    });

    await wrapper.find("input").setValue("状态");
    const selects = wrapper.findAll("select");
    await selects[0]!.setValue("system");
    await selects[1]!.setValue("low");
    await wrapper.find(".adk-tool-card").trigger("click");

    expect(wrapper.emitted("update:toolSearchQuery")?.[0]).toEqual(["状态"]);
    expect(wrapper.emitted("update:toolCategoryFilter")?.[0]).toEqual(["system"]);
    expect(wrapper.emitted("update:toolRiskFilter")?.[0]).toEqual(["low"]);
    expect(openToolDetail).toHaveBeenCalledWith("system.status");
  });

  it("renders the tool definition dialog with fallbacks", () => {
    const closeToolDetail = vi.fn();

    const wrapper = mount(ADKToolsPanel, {
      props: {
        tools: [buildTool()],
        filteredTools: [buildTool({ requiredSkill: "jftrade-workflow-management" })],
        selectedTool: buildTool({
          outputSummary: "",
          requiresApprovalIn: [],
          requiredSkill: "jftrade-workflow-management",
          inputSchema: {
            type: "object",
            properties: {
              scope: { type: "string" },
            },
          },
        }),
        toolCategoryFilter: "",
        toolCategoryOptions: ["system"],
        toolRiskFilter: "",
        toolRiskOptions: ["low"],
        toolSearchQuery: "",
        toolDetailDialogOpen: true,
        preview: previewJSON,
        formatPermissionMode: formatPermissionMode,
        riskColor: () => "success",
        riskLabel: () => "低风险",
        openToolDetail: vi.fn(),
        closeToolDetail,
      },
      global: {
        stubs: {
          "v-btn": buttonStub,
          "v-card": { template: "<section><slot /></section>" },
          "v-card-actions": passthroughStub,
          "v-card-title": { template: "<div><slot /></div>" },
          "v-card-text": { template: "<div><slot /></div>" },
          "v-chip": { template: "<span><slot /></span>" },
          "v-dialog": dialogStub,
          "v-select": selectStub,
          "v-text-field": inputStub,
        },
      },
    });

    expect(wrapper.text()).toContain("系统状态");
    expect(wrapper.text()).toContain("审批制");
    expect(wrapper.text()).toContain("沙盒自动");
    expect(wrapper.text()).toContain("高度自动");
    expect(wrapper.text()).toContain("无额外审批模式限制");
    expect(wrapper.text()).toContain("未提供");
    expect(wrapper.text()).toContain("需加载 Skill");
    expect(wrapper.text()).toContain("jftrade-workflow-management");
    expect(wrapper.text()).toContain("下一条用户消息需要重新加载");
    expect(wrapper.text()).toContain('"scope": {');

    const closeButton = wrapper
      .findAll("button")
      .find((button) => button.text().includes("关闭"));
    expect(closeButton).toBeTruthy();
    closeButton?.trigger("click");
    expect(closeToolDetail).toHaveBeenCalled();
  });
});

function buildTool(overrides: Record<string, unknown> = {}) {
  return {
    name: "system.status",
    displayName: "系统状态",
    description: "读取系统状态摘要。",
    category: "system",
    permission: "read_internal",
    allowedModes: ["approval", "less_approval", "all"],
    requiresApprovalIn: ["approval"],
    inputSchema: { type: "object", properties: {} },
    outputSummary: "系统健康摘要",
    riskLevel: "low",
    ...overrides,
  };
}

function previewJSON(value: unknown): string {
  return JSON.stringify(value, null, 2);
}

function formatPermissionMode(mode: string): string {
  switch (mode) {
    case "less_approval":
      return "沙盒自动";
    case "all":
      return "高度自动";
    default:
      return "审批制";
  }
}
