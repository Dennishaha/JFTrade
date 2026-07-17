// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent } from "vue";
import { describe, expect, it } from "vitest";

import StrategyRuntimeRiskSection from "../src/components/risk/StrategyRuntimeRiskSection.vue";

const buttonStub = defineComponent({
  emits: ["click"],
  template: "<button type='button' @click='$emit(\"click\", $event)'><slot /></button>",
});
const cardStub = defineComponent({ template: "<section><slot /></section>" });
const cardTextStub = defineComponent({ template: "<div><slot /></div>" });
const alertStub = defineComponent({ template: "<div role='alert'><slot /></div>" });
const chipStub = defineComponent({ template: "<span><slot /></span>" });
const emptyStateStub = defineComponent({ props: ["text"], template: "<p>{{ text }}</p>" });

function instance(id: string, status: string, name: string) {
  return {
    id,
    definition: { name, strategyId: id, version: "1.0.0" },
    status,
  };
}

describe("StrategyRuntimeRiskSection", () => {
  it("renders status, exposes risk summaries, and forwards control actions", async () => {
    const wrapper = mount(StrategyRuntimeRiskSection, {
      props: {
        error: "运行时暂不可达",
        instances: [
          instance("running", "RUNNING", "趋势策略"),
          instance("paused", "PAUSED", "均值回归"),
          instance("stopped", "STOPPED", "网格策略"),
          instance("unknown", "", "未知策略"),
        ],
        isUpdating: (id: string) => id === "paused",
        runtimeRiskForInstance: (id: string) => id === "running"
          ? { mode: "enforce", closeOnly: true, maxOrderQuantity: 100, maxOrderNotional: null, dailyMaxOrders: null, pauseOnReject: true }
          : { mode: "monitor", closeOnly: false, maxOrderQuantity: null, maxOrderNotional: null, dailyMaxOrders: 8, pauseOnReject: false },
      } as never,
      global: {
        stubs: {
          "v-card": cardStub,
          "v-card-text": cardTextStub,
          "v-alert": alertStub,
          "v-btn": buttonStub,
          "v-chip": chipStub,
          "v-empty-state": emptyStateStub,
        },
      },
    });

    expect(wrapper.get("[role='alert']").text()).toContain("运行时暂不可达");
    expect(wrapper.text()).toContain("运行中");
    expect(wrapper.text()).toContain("已暂停");
    expect(wrapper.text()).toContain("已停止");
    expect(wrapper.text()).toContain("未知");
    expect(wrapper.text()).toContain("执行 / 仅平仓 / 单笔数量 <= 100 / 拒单后暂停");
    expect(wrapper.text()).toContain("观察 / 日订单 <= 8");

    await wrapper.get("button").trigger("click");
    const selects = wrapper.findAll("select");
    expect(selects[1]?.attributes("disabled")).toBeDefined();
    await selects[0]?.setValue("monitor");
    expect(wrapper.emitted("refresh")).toHaveLength(1);
    expect(wrapper.emitted("updateMode")?.[0]).toEqual(["running", "monitor"]);
  });

  it("uses the explicit no-instance state", () => {
    const wrapper = mount(StrategyRuntimeRiskSection, {
      props: {
        error: "",
        instances: [],
        isUpdating: () => false,
        runtimeRiskForInstance: () => ({ mode: "off" }),
      } as never,
      global: {
        stubs: {
          "v-card": cardStub,
          "v-card-text": cardTextStub,
          "v-alert": alertStub,
          "v-btn": buttonStub,
          "v-chip": chipStub,
          "v-empty-state": emptyStateStub,
        },
      },
    });

    expect(wrapper.text()).toContain("当前没有策略实例");
    expect(wrapper.find("[role='alert']").exists()).toBe(false);
  });
});
