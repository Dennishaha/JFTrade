// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it, vi } from "vitest";

vi.mock("@vue-flow/core", async () => {
  const { defineComponent, h } = await import("vue");
  return {
    VueFlow: defineComponent({
      emits: ["connect", "node-click", "update:nodes", "update:edges"],
      setup(_, { emit, slots }) {
        return () => h("div", { class: "vue-flow-stub" }, [
        h("button", {
          type: "button",
          "data-testid": "update-nodes",
          onClick: () => emit("update:nodes", [{ id: "replacement" }]),
        }),
        h("button", {
          type: "button",
          "data-testid": "update-edges",
          onClick: () => emit("update:edges", [{ id: "replacement-edge" }]),
        }),
        h("button", {
          type: "button",
          "data-testid": "connect",
          onClick: () => emit("connect", { source: "start", target: "agent:primary" }),
        }),
        h("button", {
          type: "button",
          "data-testid": "node-click",
          onClick: () => emit("node-click", { node: { id: "agent:primary" } }),
        }),
        slots["node-start"]?.({
          data: { title: "开始", subtitle: "输入", status: "ENABLED" },
          selected: false,
        }),
        slots["node-trigger"]?.({
          id: "trigger:open",
          data: { title: "开盘", subtitle: "定时", status: "ENABLED" },
          selected: false,
        }),
        slots["node-agent"]?.({
          id: "agent:primary",
          data: { title: "投研", subtitle: "智能体", status: "ENABLED" },
          selected: false,
        }),
        slots["node-monitor"]?.({
          data: { title: "监控", subtitle: "日志", status: "ENABLED" },
          selected: false,
        }),
        ]);
      },
    }),
  };
});
vi.mock("@vue-flow/background", async () => {
  const { defineComponent, h } = await import("vue");
  return { Background: defineComponent({ setup: () => () => h("div") }) };
});
vi.mock("@vue-flow/controls", async () => {
  const { defineComponent, h } = await import("vue");
  return { Controls: defineComponent({ setup: () => () => h("div") }) };
});
vi.mock("@vue-flow/minimap", async () => {
  const { defineComponent, h } = await import("vue");
  return { MiniMap: defineComponent({ setup: () => () => h("div") }) };
});

import ADKWorkflowCanvas from "../src/components/adk-page/ADKWorkflowCanvas.vue";
import ADKWorkflowDebugPanel from "../src/components/adk-page/ADKWorkflowDebugPanel.vue";
import ADKWorkflowStudioSidebar from "../src/components/adk-page/ADKWorkflowStudioSidebar.vue";
import ADKWorkflowTriggerInspector from "../src/components/adk-page/ADKWorkflowTriggerInspector.vue";

const buttonStub = defineComponent({
  emits: ["click"],
  template: "<button type='button' @click='$emit(\"click\", $event)'><slot /></button>",
});
const textFieldStub = defineComponent({
  props: ["label", "modelValue"],
  emits: ["update:modelValue"],
  template: "<label><span>{{ label }}</span><input :data-field='label' :value='modelValue' @input='$emit(\"update:modelValue\", $event.target.value)' /></label>",
});
const selectStub = defineComponent({
  props: ["label", "modelValue"],
  emits: ["update:modelValue"],
  template: "<label><span>{{ label }}</span><select :data-field='label' :value='modelValue' @change='$emit(\"update:modelValue\", $event.target.value)'><option value='event'>event</option><option value='boolean'>boolean</option><option value='number'>number</option><option value='ENABLED'>ENABLED</option><option value='DISABLED'>DISABLED</option></select></label>",
});
const switchStub = defineComponent({
  props: ["label", "modelValue"],
  emits: ["update:modelValue"],
  template: "<label><span>{{ label }}</span><input :data-field='label' type='checkbox' :checked='modelValue' @change='$emit(\"update:modelValue\", $event.target.checked)' /></label>",
});

const sharedStubs = {
  "v-icon": { template: "<span><slot /></span>" },
  "v-btn": buttonStub,
  "v-text-field": textFieldStub,
  "v-select": selectStub,
  "v-switch": switchStub,
};

describe("ADK workflow interaction boundaries", () => {
  it("propagates canvas model changes, graph events, and every selectable node", async () => {
    const wrapper = mount(ADKWorkflowCanvas, {
      props: {
        nodes: [],
        edges: [],
        selectedNodeId: "",
      },
      global: { stubs: sharedStubs },
    });

    await wrapper.get("[data-testid='update-nodes']").trigger("click");
    await wrapper.get("[data-testid='update-edges']").trigger("click");
    await wrapper.get("[data-testid='connect']").trigger("click");
    await wrapper.get("[data-testid='node-click']").trigger("click");
    for (const selector of [
      ".adk-flow-node.is-start",
      ".adk-flow-node.is-trigger",
      ".adk-flow-node.is-agent",
      ".adk-flow-node.is-monitor",
    ]) {
      await wrapper.get(selector).trigger("click");
    }

    expect(wrapper.emitted("update:nodes")?.[0]).toEqual([[{ id: "replacement" }]]);
    expect(wrapper.emitted("update:edges")?.[0]).toEqual([[{ id: "replacement-edge" }]]);
    expect(wrapper.emitted("connect")?.[0]).toEqual([{ source: "start", target: "agent:primary" }]);
    expect(wrapper.emitted("nodeClick")?.[0]).toEqual([{ node: { id: "agent:primary" } }]);
    expect(wrapper.emitted("selectNode")).toEqual([
      ["start"],
      ["trigger:open"],
      ["agent:primary"],
      ["monitor"],
    ]);
  });

  it("edits typed debug inputs without confusing boolean and text payloads", async () => {
    const rows = [
      { key: "symbol", type: "string", value: "US.AAPL", booleanValue: false },
      { key: "dryRun", type: "boolean", value: "", booleanValue: false },
      { key: "limit", type: "number", value: "10", booleanValue: false },
    ];
    const wrapper = mount(ADKWorkflowDebugPanel, {
      props: { inputRows: rows, running: false },
      global: { stubs: sharedStubs },
    });

    await wrapper.findAll("input[data-field='参数名']")[0]?.setValue("ticker");
    await wrapper.findAll("select[data-field='类型']")[0]?.setValue("number");
    await wrapper.findAll("input[data-field='调试数字']")[0]?.setValue("US.MSFT");
    await wrapper.get("input[data-field='开启']").setValue(true);
    await wrapper.get("input[data-field='调试数字']").setValue("25");

    const buttons = wrapper.findAll("button");
    await buttons[0]?.trigger("click");
    await buttons[1]?.trigger("click");
    await buttons[2]?.trigger("click");

    expect(rows).toEqual([
      { key: "ticker", type: "number", value: "25", booleanValue: false },
      { key: "dryRun", type: "boolean", value: "", booleanValue: true },
      { key: "limit", type: "number", value: "10", booleanValue: false },
    ]);
    expect(wrapper.emitted("addInput")).toHaveLength(1);
    expect(wrapper.emitted("run")).toHaveLength(1);
    expect(wrapper.emitted("removeInput")?.[0]).toEqual([0]);
  });

  it("refreshes trigger data when type, status, or title changes", async () => {
    const triggerForm = {
      id: "",
      type: "schedule",
      status: "ENABLED",
      title: "开盘计划",
    };
    const wrapper = mount(ADKWorkflowTriggerInspector, {
      props: {
        triggerForm,
        selectedTrigger: null,
        selectedNodeRun: null,
        triggerRunSummary: null,
        schedulePreviewRuns: [],
        webhookEndpoint: "",
        webhookCurlSample: "",
        latestMarketEvent: null,
        triggerLoading: false,
        runningTrigger: false,
        saving: false,
        preservedConfigCount: 0,
        formatDateTime: (value: string) => value,
      } as never,
      global: {
        stubs: {
          ...sharedStubs,
          ADKWorkflowScheduleTriggerPanel: true,
          ADKWorkflowWebhookTriggerPanel: true,
          ADKWorkflowEventTriggerPanel: true,
          ADKWorkflowMarketTriggerPanel: true,
          ADKWorkflowNodeRunPreview: true,
        },
      },
    });

    const selects = wrapper.findAll("select");
    await selects[0]?.setValue("event");
    await selects[1]?.setValue("DISABLED");
    await wrapper.get("input[data-field='标题']").setValue("盘中异动");
    await wrapper.findAll("button")[0]?.trigger("click");
    await wrapper.findAll("button")[1]?.trigger("click");

    expect(triggerForm).toEqual({
      id: "",
      type: "event",
      status: "DISABLED",
      title: "盘中异动",
    });
    expect(wrapper.emitted("refreshNodeData")).toHaveLength(3);
    expect(wrapper.emitted("runSelectedTrigger")).toHaveLength(1);
    expect(wrapper.emitted("removeSelectedTrigger")).toHaveLength(1);
  });

  it("emits sidebar filter, template-picker, and pager commands", async () => {
    const workflow = {
      id: "workflow-1",
      name: "开盘复盘",
      agentId: "agent-1",
      workMode: "loop",
      status: "ENABLED",
    };
    const wrapper = mount(ADKWorkflowStudioSidebar, {
      props: {
        workflows: [workflow],
        selectedWorkflowId: "workflow-1",
        templates: [],
        showTemplatePicker: false,
        search: "",
        statusFilter: "",
        statusOptions: [],
        loading: false,
        page: { offset: 20, hasMore: true },
        pageSummary: "21-40 / 50",
        agentName: () => "投研智能体",
        workModeLabel: () => "循环",
        workflowTone: () => "is-success",
        statusLabel: () => "已启用",
      } as never,
      global: { stubs: sharedStubs },
    });

    await wrapper.findAll("button")[0]?.trigger("click");
    await wrapper.get("input").setValue("复盘");
    await wrapper.get("select").setValue("ENABLED");
    const pagerButtons = wrapper.findAll("button").filter((button) =>
      ["上一页", "下一页"].includes(button.text()),
    );
    await pagerButtons[0]?.trigger("click");
    await pagerButtons[1]?.trigger("click");

    expect(wrapper.emitted("update:showTemplatePicker")?.[0]).toEqual([true]);
    expect(wrapper.emitted("update:search")?.[0]).toEqual(["复盘"]);
    expect(wrapper.emitted("update:statusFilter")?.[0]).toEqual(["ENABLED"]);
    expect(wrapper.emitted("previous")).toHaveLength(1);
    expect(wrapper.emitted("next")).toHaveLength(1);
  });
});
