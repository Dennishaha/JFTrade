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
    expect(wrapper.find("[data-color='error']").exists()).toBe(true);
  });

  it("keeps each empty stream distinct", () => {
    const wrapper = mountTimeline([], []);
    expect(wrapper.text()).toContain("暂无运行时风控事件。");
    expect(wrapper.text()).toContain("暂无熔断事件。");
  });
});
