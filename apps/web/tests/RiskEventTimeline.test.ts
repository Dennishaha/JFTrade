// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent } from "vue";
import { describe, expect, it } from "vitest";

import RiskEventTimeline from "../src/components/risk/RiskEventTimeline.vue";

const card = defineComponent({ template: "<section><slot /></section>" });
const cardText = defineComponent({ template: "<div><slot /></div>" });
const chip = defineComponent({
  props: ["color"],
  template: "<span :data-color='color'><slot /></span>",
});

function mountTimeline(riskEvents: unknown[], killSwitchEvents: unknown[]) {
  return mount(RiskEventTimeline, {
    props: { riskEvents, killSwitchEvents } as never,
    global: {
      stubs: {
        "v-card": card,
        "v-card-text": cardText,
        "v-chip": chip,
      },
    },
  });
}

describe("risk event timeline", () => {
  it("renders risk and kill-switch histories with their safety fallbacks", () => {
    const wrapper = mountTimeline(
      [{
        id: "risk-1",
        eventType: "rejected",
        action: "",
        createdAt: "2026-07-16T09:00:00Z",
        operatorId: null,
        reason: "",
        errorCode: "RISK_LIMIT",
      }],
      [{
        id: "kill-1",
        eventType: "activated",
        action: "人工熔断",
        createdAt: "2026-07-16T09:01:00Z",
        operatorId: "risk-operator",
        reason: "",
        errorCode: "",
      }],
    );

    expect(wrapper.text()).toContain("RISK_LIMIT");
    expect(wrapper.text()).toContain("system");
    expect(wrapper.text()).toContain("人工熔断");
    expect(wrapper.text()).toContain("暂无原因");
    expect(wrapper.find(".tv-status--error").exists()).toBe(true);
  });

  it("keeps each empty stream distinct", () => {
    const wrapper = mountTimeline([], []);
    expect(wrapper.text()).toContain("暂无运行时风控事件。");
    expect(wrapper.text()).toContain("暂无熔断事件。");
  });

  it("filters the timeline by event source", async () => {
    const wrapper = mountTimeline(
      [{
        id: "risk-1",
        eventType: "rejected",
        action: "拒绝下单",
        createdAt: "2026-07-16T09:00:00Z",
        operatorId: null,
        reason: "超出限额",
        errorCode: "",
      }],
      [{
        id: "kill-1",
        eventType: "activated",
        action: "人工熔断",
        createdAt: "2026-07-16T09:01:00Z",
        operatorId: "risk-operator",
        reason: "紧急处置",
        errorCode: "",
      }],
    );

    // 默认「全部」：两列都渲染。
    expect(wrapper.text()).toContain("拒绝下单");
    expect(wrapper.text()).toContain("人工熔断");

    const filterButton = (label: string) =>
      wrapper
        .findAll(".risk-events__filter-btn")
        .find((button) => button.text() === label)!;

    await filterButton("熔断").trigger("click");
    expect(wrapper.text()).not.toContain("拒绝下单");
    expect(wrapper.text()).toContain("人工熔断");

    await filterButton("配置与拒单").trigger("click");
    expect(wrapper.text()).toContain("拒绝下单");
    expect(wrapper.text()).not.toContain("人工熔断");

    await filterButton("全部").trigger("click");
    expect(wrapper.text()).toContain("拒绝下单");
    expect(wrapper.text()).toContain("人工熔断");
  });
});
