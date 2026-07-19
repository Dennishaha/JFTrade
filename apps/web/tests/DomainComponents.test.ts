// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import MarketStatusBadge from "../src/components/domain/market-data/MarketStatusBadge.vue";
import OrderBookDepthTable from "../src/components/domain/market-data/OrderBookDepthTable.vue";
import RuntimeHealthBadge from "../src/components/domain/runtime/RuntimeHealthBadge.vue";
import DenseMetricStrip from "../src/components/domain/shared/DenseMetricStrip.vue";
import StrategyInstanceCard from "../src/components/domain/strategy/StrategyInstanceCard.vue";
import type { StrategyInstanceCardModel } from "../src/components/domain/strategy/strategyInstanceCard";

describe("JFTrade domain components", () => {
  it("renders market status and dense metrics from props", () => {
    const status = mount(MarketStatusBadge, { props: { state: "live" } });
    expect(status.text()).toBe("实时");
    expect(status.attributes("data-state")).toBe("live");

    const disabled = mount(MarketStatusBadge, { props: { state: "disabled" } });
    expect(disabled.attributes("aria-disabled")).toBe("true");

    const metrics = mount(DenseMetricStrip, {
      props: {
        items: [
          { key: "price", label: "最新价", value: "100.20", tone: "positive" },
          { key: "volume", label: "成交量", value: "1.2M" },
        ],
      },
    });
    expect(metrics.text()).toContain("最新价");
    expect(metrics.text()).toContain("100.20");
    expect(metrics.findAll(".dense-metric-strip__item")).toHaveLength(2);
  });

  it.each([
    { props: { levels: [], loading: true }, state: "loading", text: "正在加载盘口" },
    { props: { levels: [], error: "行情权限不足" }, state: "error", text: "行情权限不足" },
    { props: { levels: [], disabled: true }, state: "disabled", text: "盘口不可用" },
    { props: { levels: [] }, state: "empty", text: "暂无深度数据" },
  ])("renders the order book $state state", ({ props, state, text }) => {
    const wrapper = mount(OrderBookDepthTable, { props });
    expect(wrapper.get(`[data-state="${state}"]`).text()).toContain(text);
  });

  it("renders normal order book levels with domain formatting", () => {
    const wrapper = mount(OrderBookDepthTable, {
      props: {
        market: "US",
        levels: [{ bidPrice: 100.12, askPrice: 100.18, bidSize: 1_200, askSize: 2_500 }],
      },
    });
    expect(wrapper.get('[data-state="normal"]').text()).toContain("100.12");
    expect(wrapper.get('[data-testid="depth-bid-size-col"]').text()).toContain("1.2K");
    expect(wrapper.get('[data-testid="depth-ask-size-col"]').text()).toContain("2.5K");
  });

  it("maps runtime health states without accessing runtime state", () => {
    const running = mount(RuntimeHealthBadge, { props: { status: "RUNNING" } });
    expect(running.text()).toBe("运行中");
    expect(running.attributes("data-tone")).toBe("healthy");

    const failed = mount(RuntimeHealthBadge, { props: { status: "FAILED" } });
    expect(failed.text()).toBe("执行失败");
    expect(failed.attributes("data-tone")).toBe("error");
  });

  it("emits selection for a normal strategy card and blocks a disabled card", async () => {
    const wrapper = mount(StrategyInstanceCard, { props: { model: buildStrategyCardModel() } });
    await wrapper.get("button").trigger("click");
    expect(wrapper.emitted("select")).toEqual([["strategy-1"]]);
    expect(wrapper.text()).toContain("均线策略");
    expect(wrapper.text()).toContain("600519");
    expect(wrapper.text()).toContain("上证");
    expect(wrapper.text()).toContain("000001");
    expect(wrapper.text()).toContain("深证");
    expect(wrapper.text()).not.toContain("SH.600519");
    expect(wrapper.text()).not.toContain("SZ.000001");
    expect(wrapper.get('[data-instrument-id="SH.600519"]').attributes("title")).toBe(
      "SH.600519",
    );

    await wrapper.setProps({ model: buildStrategyCardModel({ disabled: true }) });
    await wrapper.get("button").trigger("click");
    expect(wrapper.emitted("select")).toEqual([["strategy-1"]]);
    expect(wrapper.get("button").attributes("disabled")).toBeDefined();
  });
});

function buildStrategyCardModel(
  patch: Partial<StrategyInstanceCardModel> = {},
): StrategyInstanceCardModel {
  return {
    id: "strategy-1",
    name: "均线策略",
    status: "RUNNING",
    statusLabel: "运行中",
    selected: true,
    definitionStale: false,
    definitionSyncSummary: "",
    symbols: "SH.600519, SZ.000001",
    interval: "1m",
    brokerAccountSummary: "富途模拟账户",
    currentBrokerAccount: true,
    createdAt: "2026-06-30 09:30:00",
    createdAtTooltip: "2026-06-30 09:30:00",
    runtimeLabel: "PineTS worker",
    sourceFormatLabel: "Pine Script",
    eligibilityLabel: "可启动",
    startable: true,
    executionModeLabel: "仅通知",
    notifyOnly: true,
    ...patch,
  };
}
