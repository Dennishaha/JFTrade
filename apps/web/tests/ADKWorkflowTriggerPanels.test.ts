// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent } from "vue";
import { describe, expect, it } from "vitest";

import ADKWorkflowEventTriggerPanel from "../src/components/adk-page/ADKWorkflowEventTriggerPanel.vue";
import ADKWorkflowMarketTriggerPanel from "../src/components/adk-page/ADKWorkflowMarketTriggerPanel.vue";
import ADKWorkflowScheduleTriggerPanel from "../src/components/adk-page/ADKWorkflowScheduleTriggerPanel.vue";
import { createTriggerForm } from "../src/features/adkWorkflowForms";

const textFieldStub = defineComponent({
  props: ["label", "modelValue"],
  emits: ["update:modelValue"],
  template:
    "<label><span>{{ label }}</span><input :value='modelValue' @input='$emit(\"update:modelValue\", $event.target.value)' /></label>",
});

const selectStub = defineComponent({
  props: ["label", "modelValue", "items"],
  emits: ["update:modelValue"],
  template:
    "<label><span>{{ label }}</span><select :value='modelValue' @change='$emit(\"update:modelValue\", $event.target.value)'><option value='weekly'>weekly</option><option value='custom'>custom</option><option value='>='>>=</option><option value='cross_down'>cross_down</option></select></label>",
});

function fieldInput(wrapper: ReturnType<typeof mount>, label: string, selector = "input") {
  const field = wrapper.findAll("label").find((candidate) => candidate.text().includes(label));
  if (field == null) throw new Error(`missing field: ${label}`);
  return field.get(selector);
}

describe("ADK workflow trigger panels", () => {
  it("edits event filters without losing optional matching fields", async () => {
    const triggerForm = createTriggerForm("event");
    const wrapper = mount(ADKWorkflowEventTriggerPanel, {
      props: { triggerForm },
      global: { stubs: { "v-text-field": textFieldStub } },
    });

    await fieldInput(wrapper, "来源").setValue("broker");
    await fieldInput(wrapper, "事件类型").setValue("order.updated");
    await fieldInput(wrapper, "实体标识").setValue("order-42");
    await fieldInput(wrapper, "分类").setValue("execution");
    await fieldInput(wrapper, "级别").setValue("warning");
    await fieldInput(wrapper, "冷却秒数").setValue("120");

    expect(triggerForm.event).toEqual({
      source: "broker",
      eventType: "order.updated",
      entityId: "order-42",
      category: "execution",
      level: "warning",
      cooldownSec: "120",
    });
  });

  it("renders market trigger health and persists selected threshold fields", async () => {
    const triggerForm = createTriggerForm("market");
    triggerForm.market.cooldownSec = "";
    const wrapper = mount(ADKWorkflowMarketTriggerPanel, {
      props: {
        triggerForm,
        selectedTrigger: {
          id: "trigger-market",
          workflowId: "workflow-1",
          type: "market",
          title: "AAPL price",
          status: "ENABLED",
          config: {},
          lastRunAt: "2026-07-16T10:00:00Z",
          lastError: "snapshot stale",
        },
        latestMarketEvent: { instrumentId: "US.AAPL", snapshot: { price: 220 } },
        formatDateTime: (value: string) => `at ${value}`,
      } as never,
      global: { stubs: { "v-text-field": textFieldStub, "v-select": selectStub } },
    });

    await fieldInput(wrapper, "标的列表").setValue("US.AAPL,US.MSFT");
    await fieldInput(wrapper, "指标路径").setValue("snapshot.changeRate");
    await fieldInput(wrapper, "比较符", "select").setValue(">=");
    await fieldInput(wrapper, "阈值").setValue("3");
    await fieldInput(wrapper, "触发边沿", "select").setValue("cross_down");
    await fieldInput(wrapper, "冷却秒数").setValue("45");

    expect(triggerForm.market).toEqual({
      instrumentIdsText: "US.AAPL,US.MSFT",
      snapshotPath: "snapshot.changeRate",
      operator: ">=",
      value: "3",
      edge: "cross_down",
      cooldownSec: "45",
    });
    expect(wrapper.text()).toContain("上次触发：at 2026-07-16T10:00:00Z");
    expect(wrapper.text()).toContain("最近错误：snapshot stale");
    expect(wrapper.text()).toContain('"instrumentId": "US.AAPL"');

    const defaultCooldown = mount(ADKWorkflowMarketTriggerPanel, {
      props: {
        triggerForm: createTriggerForm("market"),
        selectedTrigger: null,
        latestMarketEvent: null,
        formatDateTime: (value: string) => value,
      } as never,
      global: { stubs: { "v-text-field": textFieldStub, "v-select": selectStub } },
    });
    expect(defaultCooldown.text()).toContain("冷却时间：900 秒");
  });

  it("switches schedule-specific controls and displays the next runs", async () => {
    const triggerForm = createTriggerForm("schedule");
    const wrapper = mount(ADKWorkflowScheduleTriggerPanel, {
      props: {
        triggerForm,
        selectedTrigger: {
          id: "trigger-schedule",
          workflowId: "workflow-1",
          type: "schedule",
          title: "Daily report",
          status: "ENABLED",
          config: {},
          nextRunAt: "2026-07-17T01:00:00Z",
          lastError: "calendar unavailable",
        },
        schedulePreviewRuns: ["2026-07-17 09:00", "2026-07-18 09:00"],
        formatDateTime: (value: string) => `next ${value}`,
      } as never,
      global: { stubs: { "v-text-field": textFieldStub, "v-select": selectStub } },
    });

    await fieldInput(wrapper, "频率", "select").setValue("weekly");
    expect(triggerForm.schedule.frequency).toBe("weekly");
    expect(wrapper.text()).toContain("星期");

    await fieldInput(wrapper, "频率", "select").setValue("custom");
    expect(triggerForm.schedule.frequency).toBe("custom");
    expect(wrapper.text()).toContain("自定义定时表达式");
    await fieldInput(wrapper, "自定义定时表达式").setValue("0 9 * * 1-5");
    await fieldInput(wrapper, "时间").setValue("09:00");
    await fieldInput(wrapper, "时区").setValue("Asia/Shanghai");

    expect(triggerForm.schedule.customCron).toBe("0 9 * * 1-5");
    expect(wrapper.text()).toContain("2026-07-17 09:00");
    expect(wrapper.text()).toContain("下一次：next 2026-07-17T01:00:00Z");
    expect(wrapper.text()).toContain("最近错误：calendar unavailable");
  });
});
