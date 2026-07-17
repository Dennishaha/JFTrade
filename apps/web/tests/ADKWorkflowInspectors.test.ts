// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent } from "vue";
import { describe, expect, it } from "vitest";

import ADKWorkflowAgentInspector from "../src/components/adk-page/ADKWorkflowAgentInspector.vue";
import ADKWorkflowStartInspector from "../src/components/adk-page/ADKWorkflowStartInspector.vue";
import ADKWorkflowStudioInspector from "../src/components/adk-page/ADKWorkflowStudioInspector.vue";

const textFieldStub = defineComponent({
  props: ["label", "modelValue"],
  emits: ["update:modelValue"],
  template: "<label><span>{{ label }}</span><input :value='modelValue' @input='$emit(\"update:modelValue\", $event.target.value)' /></label>",
});

const textAreaStub = defineComponent({
  props: ["label", "modelValue"],
  emits: ["update:modelValue"],
  template: "<label><span>{{ label }}</span><textarea :value='modelValue' @input='$emit(\"update:modelValue\", $event.target.value)' /></label>",
});

const selectStub = defineComponent({
  props: ["label", "modelValue"],
  emits: ["update:modelValue"],
  template: "<label><span>{{ label }}</span><select :value='modelValue' @change='$emit(\"update:modelValue\", $event.target.value)'><option value='agent-a'>agent-a</option><option value='provider-a'>provider-a</option><option value='number'>number</option></select></label>",
});

const switchStub = defineComponent({
  props: ["label", "modelValue"],
  emits: ["update:modelValue"],
  template: "<label><span>{{ label }}</span><input type='checkbox' :checked='modelValue' @change='$emit(\"update:modelValue\", $event.target.checked)' /></label>",
});

const buttonStub = defineComponent({
  emits: ["click"],
  template: "<button type='button' @click='$emit(\"click\", $event)'><slot /></button>",
});

const nodeRunPreviewStub = defineComponent({
  template: "<div data-testid='node-run-preview' />",
});

function fieldInput(wrapper: ReturnType<typeof mount>, label: string, selector = "input") {
  const field = wrapper.findAll("label").find((candidate) =>
    candidate.text().includes(label),
  );
  if (field == null) throw new Error(`missing field: ${label}`);
  return field.get(selector);
}

function workflowForm(inputRows: unknown[] = []) {
  return {
    name: "工作流",
    description: "用于测试",
    status: "draft",
    tagsText: "coverage",
    agentId: "agent-a",
    inputRows,
  };
}

function inspectorProps(inspectorKind: string) {
  return {
    inspectorKind,
    workflowForm: workflowForm(),
    triggerForm: {},
    selectedTrigger: null,
    selectedNodeRun: null,
    selectedAgentNodeData: {},
    selectedLog: null,
    visibleLogs: [],
    selectedNodeId: "node-1",
    workflowStats: { total: 2, successRate: 1, avgMs: 12, recent: 1 },
    triggerRunSummary: null,
    schedulePreviewRuns: [],
    webhookEndpoint: "https://example.test/hook",
    webhookCurlSample: "curl example",
    latestMarketEvent: null,
    logTriggerOptions: [],
    logStatusFilter: "",
    logTriggerFilter: "",
    logKeywordFilter: "",
    logFromFilter: "",
    logToFilter: "",
    logLoading: false,
    triggerLoading: false,
    runningTrigger: false,
    saving: false,
    logPage: { items: [], total: 0, page: 1, pageSize: 10 },
    logPageSummary: "无日志",
    preservedInputCount: 0,
    preservedConfigCount: 0,
    agentOptions: [],
    providerOptions: [],
    inputVariableOptions: [],
    providerName: (value: string) => value,
    formatDateTime: (value: string) => value,
    runLink: () => "/runs/example",
  };
}

describe("ADK workflow inspectors", () => {
  it("selects each inspector and forwards its business actions", async () => {
    const start = defineComponent({
      emits: ["refreshNodeData", "addInputRow", "removeInputRow"],
      template: "<div data-kind='start'><button @click='$emit(\"refreshNodeData\")'>refresh</button><button @click='$emit(\"addInputRow\")'>add</button><button @click='$emit(\"removeInputRow\", 2)'>remove</button></div>",
    });
    const agent = defineComponent({
      emits: ["refreshNodeData", "insertPromptVariable", "updateAgentNodeData"],
      template: "<div data-kind='agent'><button @click='$emit(\"refreshNodeData\")'>refresh</button><button @click='$emit(\"insertPromptVariable\", \"{{symbol}}\")'>variable</button><button @click='$emit(\"updateAgentNodeData\", { key: \"model\", value: \"gpt\" })'>update</button></div>",
    });
    const trigger = defineComponent({
      emits: ["refreshNodeData", "runSelectedTrigger", "removeSelectedTrigger"],
      template: "<div data-kind='trigger'><button @click='$emit(\"refreshNodeData\")'>refresh</button><button @click='$emit(\"runSelectedTrigger\")'>run</button><button @click='$emit(\"removeSelectedTrigger\")'>remove</button></div>",
    });
    const monitor = defineComponent({
      emits: ["refreshLogs", "selectLog", "selectNode", "copyResultMarkdown", "previousLogPage", "nextLogPage", "update:logStatusFilter", "update:logTriggerFilter", "update:logKeywordFilter", "update:logFromFilter", "update:logToFilter"],
      template: "<div data-kind='monitor'><button @click='$emit(\"refreshLogs\")'>refresh</button><button @click='$emit(\"selectLog\", \"log-1\")'>log</button><button @click='$emit(\"selectNode\", \"node-2\")'>node</button><button @click='$emit(\"copyResultMarkdown\")'>copy</button><button @click='$emit(\"previousLogPage\")'>previous</button><button @click='$emit(\"nextLogPage\")'>next</button><button @click='$emit(\"update:logStatusFilter\", \"failed\")'>status</button><button @click='$emit(\"update:logTriggerFilter\", \"trigger-1\")'>trigger</button><button @click='$emit(\"update:logKeywordFilter\", \"needle\")'>keyword</button><button @click='$emit(\"update:logFromFilter\", \"2026-01-01\")'>from</button><button @click='$emit(\"update:logToFilter\", \"2026-01-02\")'>to</button></div>",
    });
    const stubs = {
      ADKWorkflowStartInspector: start,
      ADKWorkflowAgentInspector: agent,
      ADKWorkflowTriggerInspector: trigger,
      ADKWorkflowMonitorPanel: monitor,
    };

    const startWrapper = mount(ADKWorkflowStudioInspector, {
      props: inspectorProps("start") as never,
      global: { stubs },
    });
    expect(startWrapper.get("[data-kind='start']").exists()).toBe(true);
    for (const button of startWrapper.findAll("button")) await button.trigger("click");
    expect(startWrapper.emitted("refreshNodeData")).toHaveLength(1);
    expect(startWrapper.emitted("addInputRow")).toHaveLength(1);
    expect(startWrapper.emitted("removeInputRow")?.[0]).toEqual([2]);

    const agentWrapper = mount(ADKWorkflowStudioInspector, {
      props: inspectorProps("agent") as never,
      global: { stubs },
    });
    for (const button of agentWrapper.findAll("button")) await button.trigger("click");
    expect(agentWrapper.emitted("insertPromptVariable")?.[0]).toEqual(["{{symbol}}"]);
    expect(agentWrapper.emitted("updateAgentNodeData")?.[0]).toEqual([
      { key: "model", value: "gpt" },
    ]);

    const triggerWrapper = mount(ADKWorkflowStudioInspector, {
      props: inspectorProps("trigger") as never,
      global: { stubs },
    });
    for (const button of triggerWrapper.findAll("button")) await button.trigger("click");
    expect(triggerWrapper.emitted("runSelectedTrigger")).toHaveLength(1);
    expect(triggerWrapper.emitted("removeSelectedTrigger")).toHaveLength(1);

    const monitorWrapper = mount(ADKWorkflowStudioInspector, {
      props: inspectorProps("monitor") as never,
      global: { stubs },
    });
    for (const button of monitorWrapper.findAll("button")) await button.trigger("click");
    expect(monitorWrapper.emitted("selectLog")?.[0]).toEqual(["log-1"]);
    expect(monitorWrapper.emitted("selectNode")?.[0]).toEqual(["node-2"]);
    expect(monitorWrapper.emitted("update:logToFilter")?.[0]).toEqual(["2026-01-02"]);
  });

  it("edits start-node fields and preserves its supported input types", async () => {
    const form = workflowForm([
      { key: "enabled", type: "boolean", booleanValue: true, value: "" },
      { key: "limit", type: "number", booleanValue: false, value: "12" },
    ]);
    const wrapper = mount(ADKWorkflowStartInspector, {
      props: { workflowForm: form, selectedNodeRun: null, preservedInputCount: 2 } as never,
      global: {
        stubs: {
          "v-text-field": textFieldStub,
          "v-select": selectStub,
          "v-switch": switchStub,
          "v-btn": buttonStub,
          ADKWorkflowNodeRunPreview: nodeRunPreviewStub,
        },
      },
    });

    await fieldInput(wrapper, "名称").setValue("改名工作流");
    await fieldInput(wrapper, "描述").setValue("新的说明");
    await fieldInput(wrapper, "状态", "select").setValue("agent-a");
    await fieldInput(wrapper, "标签").setValue("runtime,coverage");
    await fieldInput(wrapper, "参数名").setValue("enabledNow");
    await fieldInput(wrapper, "默认开启").setValue(false);
    await fieldInput(wrapper, "默认数字").setValue("24");
    await fieldInput(wrapper, "类型", "select").setValue("number");
    await wrapper.findAll("button")[0]?.trigger("click");
    await wrapper.findAll("button")[1]?.trigger("click");
    expect(form.name).toBe("改名工作流");
    expect(form.description).toBe("新的说明");
    expect(form.tagsText).toBe("runtime,coverage");
    expect(wrapper.emitted("refreshNodeData")).toHaveLength(3);
    expect(wrapper.emitted("addInputRow")).toHaveLength(1);
    expect(wrapper.emitted("removeInputRow")?.[0]).toEqual([0]);
    expect(wrapper.text()).toContain("已保留 2 个复杂输入字段");
    expect(wrapper.get("[data-testid='node-run-preview']").exists()).toBe(true);

    const empty = mount(ADKWorkflowStartInspector, {
      props: { workflowForm: workflowForm(), selectedNodeRun: null, preservedInputCount: 0 } as never,
      global: { stubs: { "v-text-field": textFieldStub, "v-select": selectStub, "v-switch": switchStub, "v-btn": buttonStub, ADKWorkflowNodeRunPreview: nodeRunPreviewStub } },
    });
    expect(empty.text()).toContain("暂无输入项");
  });

  it("updates agent-node overrides and inserts prompt variables without losing line breaks", async () => {
    const nodeData = {
      title: "执行器",
      agentId: "agent-a",
      providerId: "provider-a",
      model: "model-a",
      permissionMode: "",
      objectiveTemplate: "完成任务",
      promptTemplate: "先读取数据",
    };
    const props = {
      workflowForm: workflowForm(),
      selectedNodeRun: null,
      selectedNodeId: "agent-node",
      selectedAgentNodeData: nodeData,
      agentOptions: [{ title: "Agent A", value: "agent-a" }],
      providerOptions: [{ title: "Provider A", value: "provider-a" }],
      inputVariableOptions: [{ title: "标的", value: "{{symbol}}" }],
      providerName: (value: string) => `服务：${value}`,
    };
    const wrapper = mount(ADKWorkflowAgentInspector, {
      props: props as never,
      global: {
        stubs: {
          "v-text-field": textFieldStub,
          "v-textarea": textAreaStub,
          "v-select": selectStub,
          "v-btn": buttonStub,
          ADKWorkflowNodeRunPreview: nodeRunPreviewStub,
        },
      },
    });

    await fieldInput(wrapper, "节点标题").setValue("改名节点");
    await fieldInput(wrapper, "智能体", "select").setValue("agent-a");
    await wrapper.get("button").trigger("click");
    expect(wrapper.emitted("refreshNodeData")).toHaveLength(2);
    expect(wrapper.emitted("updateAgentNodeData")).toContainEqual([
      { key: "title", value: "改名节点" },
    ]);
    expect(wrapper.emitted("updateAgentNodeData")).toContainEqual([
      { key: "promptTemplate", value: "先读取数据\n{{symbol}}" },
    ]);
    expect(wrapper.text()).toContain("服务：provider-a");

    const emptyPrompt = mount(ADKWorkflowAgentInspector, {
      props: { ...props, selectedAgentNodeData: { ...nodeData, promptTemplate: "" } } as never,
      global: { stubs: { "v-text-field": textFieldStub, "v-textarea": textAreaStub, "v-select": selectStub, "v-btn": buttonStub, ADKWorkflowNodeRunPreview: nodeRunPreviewStub } },
    });
    await emptyPrompt.get("button").trigger("click");
    expect(emptyPrompt.emitted("updateAgentNodeData")).toContainEqual([
      { key: "promptTemplate", value: "{{symbol}}" },
    ]);
  });
});
