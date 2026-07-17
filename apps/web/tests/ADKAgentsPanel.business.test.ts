// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick } from "vue";

import ADKAgentsPanel from "../src/components/adk-settings/ADKAgentsPanel.vue";
import type { ADKAgent, ADKToolDescriptor } from "../src/contracts";
import { dialogStub, selectStub } from "./helpers";

const singleSlotStub = {
  template: "<div><slot /></div>",
};

const safeButtonStub = defineComponent({
  emits: ["click"],
  template:
    "<button type='button' @click=\"$emit('click')\"><slot /></button>",
});

const checkboxStub = defineComponent({
  props: ["modelValue", "value"],
  emits: ["update:modelValue"],
  setup(props, { emit }) {
    return () =>
      h("label", [
        h("input", {
          type: "checkbox",
          checked: Array.isArray(props.modelValue)
            ? props.modelValue.includes(props.value)
            : Boolean(props.modelValue),
          onChange: (event: Event) => {
            const checked = (event.target as HTMLInputElement).checked;
            if (Array.isArray(props.modelValue)) {
              const next = new Set(props.modelValue as string[]);
              if (checked) next.add(String(props.value));
              else next.delete(String(props.value));
              emit("update:modelValue", [...next]);
              return;
            }
            emit("update:modelValue", checked);
          },
        }),
        h("span", String(props.value ?? "")),
      ]);
  },
});

const multiAwareSelectStub = defineComponent({
  props: ["modelValue", "items", "label", "multiple"],
  emits: ["update:modelValue"],
  setup(props, { emit }) {
    return () =>
      h("label", [
        props.label ? h("span", String(props.label)) : null,
        h(
          "select",
          {
            multiple: props.multiple,
            value: props.multiple ? undefined : props.modelValue,
            onChange: (event: Event) => {
              const target = event.target as HTMLSelectElement;
              if (props.multiple) {
                emit(
                  "update:modelValue",
                  Array.from(target.selectedOptions).map((option) => option.value),
                );
                return;
              }
              emit("update:modelValue", target.value);
            },
          },
          ((props.items as Array<string | { title?: string; value?: string }>) ?? []).map(
            (item, index) => {
              const value =
                typeof item === "string" ? item : item.value ?? String(index);
              const label =
                typeof item === "string" ? item : item.title ?? value;
              return h("option", { key: value, value }, label);
            },
          ),
        ),
      ]);
  },
});

describe("ADKAgentsPanel business flows", () => {
  it("manages runtime tool transfer, filter emits, and save flow with real agent data", async () => {
    const saveAgent = vi.fn(async () => {});
    const editAgent = vi.fn();
    const agentForm = buildAgentForm({
      id: "agent-edit",
      tools: ["system.status", "legacy.unknown"],
    });
    const wrapper = mountAgentsPanel({
      agentForm,
      agents: [buildAgent({ id: "agent-edit", tools: ["system.status"] })],
      tools: [
        buildTool({
          name: "system.status",
          displayName: "系统状态",
          category: "system",
          riskLevel: "low",
        }),
        buildTool({
          name: "trading.submit_order",
          displayName: "提交订单",
          category: "trading",
          riskLevel: "high",
        }),
        buildTool({
          name: "research.screen",
          displayName: "条件筛选",
          category: "research",
          riskLevel: "medium",
        }),
      ],
      toolCategoryOptions: ["system", "trading", "research"],
      toolRiskOptions: ["low", "medium", "high"],
      editAgent,
      saveAgent,
    });

    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("编辑"))!
      .trigger("click");

    expect(editAgent).toHaveBeenCalledWith(
      expect.objectContaining({ id: "agent-edit" }),
    );
    expect(wrapper.text()).toContain("legacy.unknown");
    expect(wrapper.text()).toContain("已启用 2/3");

    const checkboxes = wrapper.findAll("input[type='checkbox']");
    await checkboxes.find((input) => input.element.nextSibling?.textContent === "trading.submit_order")!.setValue(true);
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "添加")!
      .trigger("click");
    expect(agentForm.tools).toEqual([
      "system.status",
      "legacy.unknown",
      "trading.submit_order",
    ]);

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "全部添加")!
      .trigger("click");
    expect(agentForm.tools).toEqual([
      "system.status",
      "legacy.unknown",
      "trading.submit_order",
      "research.screen",
    ]);

    const enabledCheckboxes = wrapper.findAll("input[type='checkbox']");
    await enabledCheckboxes.find((input) => input.element.nextSibling?.textContent === "legacy.unknown")!.setValue(true);
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "移除")!
      .trigger("click");
    expect(agentForm.tools).toEqual([
      "system.status",
      "trading.submit_order",
      "research.screen",
    ]);

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "全部移除")!
      .trigger("click");
    expect(agentForm.tools).toEqual([]);
    expect(wrapper.text()).toContain("空列表表示该智能体可使用全部运行时工具。");

    const categorySelect = wrapper
      .findAll("select")
      .find((select) => select.text().includes("trading"));
    const riskSelect = wrapper
      .findAll("select")
      .find((select) => select.text().includes("high"));
    await categorySelect!.setValue("trading");
    await riskSelect!.setValue("high");
    expect(wrapper.emitted("update:toolCategoryFilter")?.[0]).toEqual([
      "trading",
    ]);
    expect(wrapper.emitted("update:toolRiskFilter")?.[0]).toEqual(["high"]);

    await wrapper
      .findAll("button")
      .find((button) => button.text().trim() === "保存智能体")!
      .trigger("click");
    expect(saveAgent).toHaveBeenCalledOnce();
    expect(wrapper.find(".v-dialog-stub").exists()).toBe(false);
  });

  it("shows empty/template states and protects the primary default agent controls", async () => {
    const applyAgentTemplate = vi.fn();
    const duplicateAgent = vi.fn();
    const deleteAgent = vi.fn();
    const agentForm = buildAgentForm({ id: "jftrade-default", status: "ENABLED" });
    const newAgentForm = vi.fn();
    const wrapper = mountAgentsPanel({
      agentForm,
      agents: [
        buildAgent({
          id: "jftrade-default",
          name: "默认助手",
          builtin: true,
          memoryEnabled: false,
          tools: [],
        }),
      ],
      agentTemplates: [],
      applyAgentTemplate,
      duplicateAgent,
      deleteAgent,
      newAgentForm,
    });

    expect(wrapper.text()).toContain("记忆已关闭");
    expect(wrapper.findAll("button").some((button) => button.text() === "编辑")).toBe(false);
    expect(wrapper.findAll("button").some((button) => button.text() === "删除")).toBe(false);

    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("从模板新建"))!
      .trigger("click");
    expect(wrapper.text()).toContain("暂无可用模板。");
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "取消")!
      .trigger("click");
    expect(wrapper.text()).not.toContain("暂无可用模板。");

    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("自定义新建"))!
      .trigger("click");
    expect(wrapper.text()).toContain("编辑智能体");
    await wrapper.find("textarea").setValue("审计所有持久化变更");
    expect(agentForm.instruction).toBe("审计所有持久化变更");
    const switches = wrapper.findAll("input[type='checkbox']");
    await switches[switches.length - 2]!.setValue(false);
    expect(agentForm.memoryEnabled).toBe(false);
    expect(wrapper.text()).toContain("记忆");
    expect(switches[switches.length - 1]?.attributes("disabled")).toBeDefined();
    expect(agentForm.status).toBe("ENABLED");

    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("取消"))!
      .trigger("click");
    expect(wrapper.find(".v-dialog-stub").exists()).toBe(false);

    const emptyWrapper = mountAgentsPanel({
      agents: [],
      agentTemplates: [],
    });
    expect(emptyWrapper.text()).toContain("尚未创建任何智能体。");

    const templateWrapper = mountAgentsPanel({
      agentTemplates: [
        buildAgent({
          id: "template-1",
          name: "研究模板",
          tools: [],
          skills: ["research", "risk"],
        }),
      ],
      applyAgentTemplate,
    });
    await templateWrapper
      .findAll("button")
      .find((button) => button.text().includes("从模板新建"))!
      .trigger("click");
    await templateWrapper.find(".adk-agent-template-card").trigger("click");
    expect(applyAgentTemplate).toHaveBeenCalledWith(
      expect.objectContaining({ id: "template-1", name: "研究模板" }),
    );
  });

  it("edits work-mode, numeric, textarea, and switch fields for a normal agent", async () => {
    const agentForm = buildAgentForm({
      id: "agent-edit",
      workMode: "chat",
      recentUserWindow: 6,
      loopMaxIterations: 5,
      memoryEnabled: true,
      status: "ENABLED",
      skills: [],
    });
    const wrapper = mountAgentsPanel({
      agentForm,
      agents: [buildAgent({ id: "agent-edit", name: "执行助手" })],
    });

    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("编辑"))!
      .trigger("click");

    await wrapper.setProps({
      agentForm: {
        ...agentForm,
        workMode: "loop",
      },
    });
    expect(wrapper.text()).toContain("目标循环最大轮次");

    const labels = wrapper.findAll("label");
    await labels
      .find((label) => label.text().includes("保留最近用户消息条数"))!
      .find("input")
      .setValue("12");
    await labels
      .find((label) => label.text().includes("目标循环最大轮次"))!
      .find("input")
      .setValue("9");
    await wrapper.find("textarea").setValue("处理异常并记录审计日志");

    const switches = wrapper.findAll("input[type='checkbox']");
    await switches[switches.length - 2]!.setValue(false);
    await switches[switches.length - 1]!.setValue(false);

    const editedForm = wrapper.props("agentForm") as typeof agentForm;
    expect(editedForm.recentUserWindow).toBe(12);
    expect(editedForm.loopMaxIterations).toBe(9);
    expect(editedForm.instruction).toBe("处理异常并记录审计日志");
    expect(editedForm.memoryEnabled).toBe(false);
    expect(editedForm.status).toBe("DISABLED");
  });

  it("renders create-dialog header fields, updates skills, and closes from the header action", async () => {
    const agentForm = buildAgentForm({
      id: "",
      skills: [],
    });
    const wrapper = mountAgentsPanel(
      {
        agentForm,
      },
      {
        "v-select": multiAwareSelectStub,
      },
    );

    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("自定义新建"))!
      .trigger("click");

    expect(wrapper.text()).toContain("新建智能体");
    expect(wrapper.text()).toContain("名称");
    expect(wrapper.text()).toContain("模型服务");
    expect(wrapper.text()).toContain("覆盖模型（可选）");
    expect(wrapper.text()).toContain("默认审批等级");
    expect(wrapper.text()).toContain("默认工作模式");
    expect(wrapper.text()).toContain("启用技能");

    const skillSelect = wrapper
      .findAllComponents(multiAwareSelectStub)
      .find((component) => component.text().includes("启用技能"));
    expect(skillSelect).toBeDefined();
    skillSelect!.vm.$emit("update:modelValue", ["research", "risk"]);

    expect(agentForm.skills).toEqual(["research", "risk"]);

    const closeButton = wrapper.findAll("button").find((button) => button.text() === "");
    expect(closeButton).toBeDefined();
    await closeButton!.trigger("click");
    expect(wrapper.find(".v-dialog-stub").exists()).toBe(false);
  });

  it("keeps the default agent first while duplicating, deleting, and narrowing available tools", async () => {
    const duplicateAgent = vi.fn();
    const deleteAgent = vi.fn(async () => {});
    const wrapper = mountAgentsPanel({
      agents: [
        buildAgent({ id: "research-agent", name: "研究助手", tools: ["system.status"] }),
        buildAgent({ id: "jftrade-default", name: "默认助手", builtin: true }),
      ],
      tools: [
        buildTool({ name: "system.status", category: "system", riskLevel: "low" }),
        buildTool({ name: "trading.submit_order", category: "trading", riskLevel: "high" }),
        buildTool({ name: "research.screen", category: "research", riskLevel: "medium" }),
      ],
      agentForm: buildAgentForm({ id: "research-agent", tools: ["system.status"] }),
      duplicateAgent,
      deleteAgent,
    });

    const names = wrapper
      .findAll(".font-semibold.text-slate-900")
      .map((node) => node.text());
    expect(names.slice(-2)).toEqual(["默认助手", "研究助手"]);

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "复制")!
      .trigger("click");
    expect(duplicateAgent).toHaveBeenCalledWith(
      expect.objectContaining({ id: "jftrade-default" }),
    );
    expect(wrapper.text()).toContain("编辑智能体");

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "删除")!
      .trigger("click");
    expect(deleteAgent).toHaveBeenCalledWith("research-agent");

    await wrapper.setProps({ toolCategoryFilter: "trading", toolRiskFilter: "high" });
    expect(wrapper.text()).toContain("可用运行时工具");
    expect(wrapper.text()).toContain("trading.submit_order");
    expect(wrapper.text()).not.toContain("research.screen");
  });

  it("removes stale checked tool selections when the persisted agent or runtime filters change", async () => {
    const agentForm = buildAgentForm({
      id: "risk-agent",
      tools: ["system.status", "trading.submit_order"],
    });
    const wrapper = mountAgentsPanel({
      agentForm,
      tools: [
        buildTool({ name: "system.status", category: "system", riskLevel: "low" }),
        buildTool({ name: "trading.submit_order", category: "trading", riskLevel: "high" }),
        buildTool({ name: "research.screen", category: "research", riskLevel: "medium" }),
      ],
    });
    const setup = wrapper.vm.$.setupState as Record<string, unknown>;
    const write = (key: string, value: unknown) => {
      const current = setup[key];
      if (current !== null && typeof current === "object" && "value" in current) {
        (current as { value: unknown }).value = value;
        return;
      }
      setup[key] = value;
    };
    const read = <T>(value: unknown): T =>
      value !== null && typeof value === "object" && "value" in value
        ? (value as { value: T }).value
        : value as T;

    write("checkedAvailableTools", ["research.screen", "trading.submit_order"]);
    write("checkedEnabledTools", ["system.status", "trading.submit_order"]);
    await wrapper.setProps({
      agentForm: { ...agentForm, tools: ["system.status"] },
      toolCategoryFilter: "research",
      toolRiskFilter: "medium",
    });
    await nextTick();

    expect(read<string[]>(setup.checkedAvailableTools)).toEqual(["research.screen"]);
    expect(read<string[]>(setup.checkedEnabledTools)).toEqual(["system.status"]);
  });
});

function mountAgentsPanel(
  overrides: Partial<{
    agents: ADKAgent[];
    agentForm: ReturnType<typeof buildAgentForm>;
    agentTemplates: Array<Omit<ADKAgent, "createdAt" | "updatedAt">>;
    tools: ADKToolDescriptor[];
    toolCategoryFilter: string;
    toolCategoryOptions: string[];
    toolRiskFilter: string;
    toolRiskOptions: string[];
    applyAgentTemplate: ReturnType<typeof vi.fn>;
    saveAgent: ReturnType<typeof vi.fn>;
    newAgentForm: ReturnType<typeof vi.fn>;
    editAgent: ReturnType<typeof vi.fn>;
    duplicateAgent: ReturnType<typeof vi.fn>;
    deleteAgent: ReturnType<typeof vi.fn>;
  }> = {},
  stubs: Record<string, unknown> = {},
) {
  return mount(ADKAgentsPanel, {
    attachTo: document.body,
    props: {
      agentForm: overrides.agentForm ?? buildAgentForm(),
      agents: overrides.agents ?? [],
      agentTemplates: overrides.agentTemplates ?? [],
      agentTemplateNotice: "",
      providerOptions: [{ title: "OpenAI", value: "provider-1" }],
      toolOptions: [],
      skillOptions: [
        { title: "研究", value: "research" },
        { title: "风险", value: "risk" },
      ],
      permissionModes: [{ title: "审批制", value: "approval" }],
      tools: overrides.tools ?? [],
      toolCategoryFilter: overrides.toolCategoryFilter ?? "",
      toolCategoryOptions: overrides.toolCategoryOptions ?? [],
      toolRiskFilter: overrides.toolRiskFilter ?? "",
      toolRiskOptions: overrides.toolRiskOptions ?? [],
      formatPermission: (mode: string) =>
        mode === "approval" ? "审批制" : mode,
      riskColor: (risk?: string) => risk ?? "default",
      riskLabel: (risk?: string) =>
        risk === "high" ? "高风险" : risk === "medium" ? "中风险" : "低风险",
      applyAgentTemplate: overrides.applyAgentTemplate ?? vi.fn(),
      saveAgent: overrides.saveAgent ?? vi.fn(async () => {}),
      newAgentForm: overrides.newAgentForm ?? vi.fn(),
      editAgent: overrides.editAgent ?? vi.fn(),
      duplicateAgent: overrides.duplicateAgent ?? vi.fn(),
      deleteAgent: overrides.deleteAgent ?? vi.fn(),
    },
    global: {
      stubs: {
        "v-btn": safeButtonStub,
        "v-card": singleSlotStub,
        "v-card-actions": singleSlotStub,
        "v-card-title": singleSlotStub,
        "v-card-text": singleSlotStub,
        "v-checkbox": checkboxStub,
        "v-chip": { template: "<span><slot /></span>" },
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
          props: ["modelValue", "label", "disabled", "trueValue", "falseValue"],
          emits: ["update:modelValue"],
          template:
            "<label>{{ label }}<input type='checkbox' :disabled='disabled' :checked=\"modelValue === true || modelValue === trueValue\" @change=\"$emit('update:modelValue', $event.target.checked ? (trueValue ?? true) : (falseValue ?? false))\" /></label>",
        },
        "v-text-field": {
          props: ["modelValue", "label"],
          emits: ["update:modelValue"],
          template:
            "<label>{{ label }}<input :value='modelValue' @input=\"$emit('update:modelValue', $event.target.value)\" /></label>",
        },
        "v-textarea": {
          props: ["modelValue"],
          emits: ["update:modelValue"],
          template:
            "<textarea :value='modelValue' @input=\"$emit('update:modelValue', $event.target.value)\"></textarea>",
        },
        ...stubs,
      },
    },
  });
}

function buildAgentForm(overrides: Partial<ADKAgent> = {}) {
  return {
    id: "",
    name: "新 Agent",
    instruction: "",
    providerId: "provider-1",
    model: "",
    tools: [] as string[],
    skills: [] as string[],
    permissionMode: "approval" as const,
    memoryEnabled: true,
    recentUserWindow: 6,
    workMode: "chat" as const,
    loopMaxIterations: 5,
    status: "ENABLED" as const,
    ...overrides,
  };
}

function buildAgent(overrides: Partial<ADKAgent> = {}): ADKAgent {
  return {
    id: "agent-1",
    name: "交易助手",
    instruction: "",
    providerId: "provider-1",
    model: "",
    tools: [],
    skills: [],
    permissionMode: "approval",
    memoryEnabled: true,
    recentUserWindow: 6,
    workMode: "chat",
    loopMaxIterations: 5,
    status: "ENABLED",
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
    ...overrides,
  };
}

function buildTool(overrides: Partial<ADKToolDescriptor> = {}): ADKToolDescriptor {
  return {
    name: "system.status",
    displayName: "系统状态",
    description: "读取状态。",
    category: "system",
    permission: "system_read",
    riskLevel: "low",
    allowedModes: ["approval", "less_approval", "all"],
    requiresApprovalIn: [],
    ...overrides,
  };
}
