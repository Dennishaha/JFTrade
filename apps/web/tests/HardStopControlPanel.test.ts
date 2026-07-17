// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent } from "vue";
import { describe, expect, it } from "vitest";

import HardStopControlPanel from "../src/components/risk/HardStopControlPanel.vue";

const card = defineComponent({ template: "<section><slot /></section>" });
const cardText = defineComponent({ template: "<div><slot /></div>" });
const chip = defineComponent({ template: "<span><slot /></span>" });
const button = defineComponent({
  emits: ["click"],
  template: "<button type='button' @click='$emit(\"click\")'><slot /></button>",
});

function mountPanel(entries: unknown[] = []) {
  return mount(HardStopControlPanel, {
    props: { entries, loadingAction: "" } as never,
    global: {
      stubs: {
        "v-card": card,
        "v-card-text": cardText,
        "v-chip": chip,
        "v-btn": button,
      },
    },
  });
}

describe("hard-stop control panel", () => {
  it("normalizes manual hard-stop commands before emitting an activation request", async () => {
    const wrapper = mountPanel();
    await wrapper.get("[aria-label='硬停止账户 ID']").setValue("  REAL-01 ");
    await wrapper.get("[aria-label='硬停止范围']").setValue("SYMBOL");
    await wrapper.get("[aria-label='硬停止市场']").setValue(" us ");
    await wrapper.get("[aria-label='硬停止标的']").setValue(" aapl ");
    await wrapper.get("[aria-label='硬停止原因']").setValue("  operator review  ");
    await wrapper.get("button").trigger("click");

    expect(wrapper.emitted("activate")).toEqual([[
      {
        brokerId: "futu",
        tradingEnvironment: "REAL",
        accountId: "REAL-01",
        market: "US",
        symbol: "AAPL",
        hardStopScope: "SYMBOL",
        operatorId: "local",
        reason: "operator review",
      },
    ]]);
    expect(wrapper.text()).toContain("暂无活跃实盘硬停止。");
  });

  it("renders account, market, and symbol scopes and releases a selected hard stop", async () => {
    const wrapper = mountPanel([
      {
        id: "account-stop",
        brokerId: "futu",
        tradingEnvironment: "REAL",
        accountId: "REAL-01",
        market: null,
        symbol: null,
        operatorId: "risk",
        reason: "",
      },
      {
        id: "market-stop",
        brokerId: "futu",
        tradingEnvironment: "SIMULATE",
        accountId: "SIM-01",
        market: "US",
        symbol: null,
        operatorId: "risk",
        reason: "feed unstable",
      },
      {
        id: "symbol-stop",
        brokerId: "futu",
        tradingEnvironment: "REAL",
        accountId: "REAL-01",
        market: "HK",
        symbol: "HK.00700",
        operatorId: "risk",
        reason: "manual hold",
      },
    ]);

    expect(wrapper.text()).toContain("3 条生效");
    expect(wrapper.text()).toContain("账户");
    expect(wrapper.text()).toContain("市场 / 美股");
    expect(wrapper.text()).toContain("标的 / 港股 / HK.00700");
    expect(wrapper.text()).toContain("未填写原因");
    expect(wrapper.text()).toContain("模拟 / 美股");

    const releaseButtons = wrapper.findAll("button").filter((candidate) => candidate.text() === "解除硬停止");
    await releaseButtons[2]?.trigger("click");
    expect(wrapper.emitted("release")).toEqual([["symbol-stop"]]);
  });
});
