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
          "v-form": passthroughStub,
          "v-select": selectStub,
          "v-table": passthroughStub,
          "v-text-field": inputStub,
          "v-theme-provider": passthroughStub,
        },
      },
    });

    await wrapper.find("input").setValue("状态");
    const selects = wrapper.findAll("select");
    await selects[0]!.setValue("system");
    await selects[1]!.setValue("low");
    await wrapper.find(".adk-tool-row").trigger("click");

    expect(wrapper.emitted("update:toolSearchQuery")?.[0]).toEqual(["状态"]);
    expect(wrapper.emitted("update:toolCategoryFilter")?.[0]).toEqual(["system"]);
    expect(wrapper.emitted("update:toolRiskFilter")?.[0]).toEqual(["low"]);
    expect(openToolDetail).toHaveBeenCalledWith("system.status");
    expect(wrapper.text()).toContain("内部读取");
    expect(wrapper.text()).not.toContain("read_internal");
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
          requiredSkills: ["jftrade-strategy-research", "jftrade-strategy-publish"],
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
          "v-form": passthroughStub,
          "v-select": selectStub,
          "v-table": passthroughStub,
          "v-text-field": inputStub,
          "v-theme-provider": passthroughStub,
        },
      },
    });

    expect(wrapper.text()).toContain("系统状态");
    expect(wrapper.text()).toContain("内部读取");
    expect(wrapper.text()).not.toContain("read_internal");
    expect(wrapper.text()).toContain("审批制");
    expect(wrapper.text()).toContain("沙盒自动");
    expect(wrapper.text()).toContain("高度自动");
    expect(wrapper.text()).toContain("无额外审批模式限制");
    expect(wrapper.text()).toContain("未提供");
    expect(wrapper.text()).toContain("需加载 Skill");
    expect(wrapper.text()).toContain("当前 invocation 必须先加载以下任一 Skill：");
    expect(requiredSkillTagTexts(wrapper)).toEqual([
      "jftrade-workflow-management",
      "jftrade-strategy-research",
      "jftrade-strategy-publish",
    ]);
    expect(wrapper.text()).toContain("下一条用户消息需要重新加载");
    expect(wrapper.text()).toContain('"scope": {');

    const closeButton = wrapper
      .findAll("button")
      .find((button) => button.text().includes("关闭"));
    expect(closeButton).toBeTruthy();
    closeButton?.trigger("click");
    expect(closeToolDetail).toHaveBeenCalled();
  });

  it("renders an any-of Skill requirement when only requiredSkills is provided", async () => {
    const wrapper = mount(ADKToolsPanel, {
      props: {
        tools: [buildTool()],
        filteredTools: [
          buildTool({
            requiredSkills: [
              "jftrade-strategy-research",
              "jftrade-strategy-publish",
            ],
          }),
        ],
        selectedTool: buildTool({
          requiredSkills: [
            " jftrade-strategy-research ",
            "jftrade-strategy-research",
            "",
            "jftrade-strategy-publish",
          ],
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
          "v-form": passthroughStub,
          "v-select": selectStub,
          "v-table": passthroughStub,
          "v-text-field": inputStub,
          "v-theme-provider": passthroughStub,
        },
      },
    });

    expect(wrapper.text()).toContain("需加载 Skill");
    expect(wrapper.text()).toContain("当前 invocation 必须先加载以下任一 Skill：");
    expect(requiredSkillTagTexts(wrapper)).toEqual([
      "jftrade-strategy-research",
      "jftrade-strategy-publish",
    ]);

    await wrapper.setProps({
      filteredTools: [buildTool({ requiredSkill: "jftrade-workflow-management" })],
      selectedTool: buildTool({ requiredSkill: "jftrade-workflow-management" }),
    });
    expect(wrapper.text()).toContain("jftrade-workflow-management");
    expect(wrapper.text()).toContain("当前 invocation 必须先加载：");
    expect(wrapper.text()).not.toContain("当前 invocation 必须先加载以下任一 Skill：");
    expect(requiredSkillTagTexts(wrapper)).toEqual([
      "jftrade-workflow-management",
    ]);
  });
});

function requiredSkillTagTexts(wrapper: ReturnType<typeof mount>): string[] {
  return Array.from(
    new Set(
      wrapper
        .findAll('[data-testid="required-skill-tag"]')
        .map((tag) => tag.text()),
    ),
  );
}

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
