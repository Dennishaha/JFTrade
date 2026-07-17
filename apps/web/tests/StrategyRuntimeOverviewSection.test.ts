// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import StrategyRuntimeOverviewSection from "../src/components/strategy-runtime/StrategyRuntimeOverviewSection.vue";

describe("strategy runtime overview", () => {
  it("summarizes active strategy execution and real-trade protections", () => {
    const wrapper = mount(StrategyRuntimeOverviewSection, {
      props: {
        activeStrategyCount: 3,
        selectedStrategy: {
          definition: { name: "趋势跟踪" },
          binding: { runtimeRisk: { maxOpenPositions: 2 } },
        },
        selectedStrategyRuntimeLabel: "实盘执行",
        systemStatus: {
          defaultTradingEnvironment: "REAL",
          realTradingEnabled: true,
          realTradingKillSwitch: { active: true },
        },
        formatStrategyRuntimeRiskSummary: (value: unknown) =>
          value == null ? "未配置" : "最多 2 个开仓",
      } as never,
    });

    expect(wrapper.text()).toContain("3 个运行中");
    expect(wrapper.text()).toContain("REAL");
    expect(wrapper.text()).toContain("趋势跟踪");
    expect(wrapper.get("[data-testid='strategy-runtime-mode']").text()).toBe("实盘执行");
    expect(wrapper.text()).toContain("已开启");
    expect(wrapper.text()).toContain("已启用");
    expect(wrapper.get("[data-testid='strategy-overview-runtime-risk']").text()).toBe("最多 2 个开仓");
  });

  it("shows safe defaults when no strategy is selected and the kill switch is inactive", () => {
    const wrapper = mount(StrategyRuntimeOverviewSection, {
      props: {
        activeStrategyCount: 0,
        selectedStrategy: null,
        selectedStrategyRuntimeLabel: "未运行",
        systemStatus: {
          defaultTradingEnvironment: "SIMULATE",
          realTradingEnabled: false,
          realTradingKillSwitch: { active: false },
        },
        formatStrategyRuntimeRiskSummary: () => "未配置",
      } as never,
    });

    expect(wrapper.text()).toContain("暂无");
    expect(wrapper.text()).toContain("已关闭");
    expect(wrapper.text()).toContain("未启用");
    expect(wrapper.text()).toContain("未配置");
  });
});
