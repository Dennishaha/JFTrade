// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { unref } from "vue";
import { describe, expect, it } from "vitest";

import BacktestReportProperties from "../src/components/backtest/BacktestReportProperties.vue";
import type {
  BacktestRun,
  BacktestRunResult,
} from "../src/composables/useBacktestRuns";

function runWithResult(result?: BacktestRunResult): BacktestRun {
  return {
    id: "run-properties",
    status: result ? "failed" : "running",
    request: {
      definitionId: "strategy-1",
      symbol: "US.AAPL",
      interval: "5m",
      chartType: "standard",
      startTime: "2026-06-01T00:00:00Z",
      endTime: "2026-06-02T00:00:00Z",
      initialBalance: 100_000,
    },
    ...(result ? { result } : {}),
    createdAt: "2026-06-02T00:00:00Z",
    updatedAt: "2026-06-02T00:01:00Z",
  };
}

function diagnosticResult(): BacktestRunResult {
  return {
    symbol: "US.AAPL",
    interval: "5m",
    startTime: "2026-06-01T00:00:00Z",
    endTime: "2026-06-02T00:00:00Z",
    finalBalance: 99_000,
    pnl: -1_000,
    totalTrades: 0,
    winRate: 0,
    runtimeErrors: ["strategy execution timed out"],
    runtimeErrorCounts: { "strategy execution timed out": 1 },
    warnings: ["one order intent was ignored"],
    logs: ["loaded 10 closed candles"],
    error: "strategy execution failed",
  };
}

describe("BacktestReportProperties", () => {
  it("keeps collection projections empty while a run has no result", () => {
    const wrapper = mount(BacktestReportProperties, {
      props: { run: runWithResult() },
      global: { stubs: { "v-icon": true } },
    });
    const setup = wrapper.vm.$.setupState as Record<string, unknown>;

    expect(wrapper.text()).toContain("暂无属性");
    expect(unref(setup.visibleRuntimeErrors)).toEqual([]);
    expect(unref(setup.visibleWarnings)).toEqual([]);
    expect(unref(setup.visibleLogs)).toEqual([]);
    expect(unref(setup.hiddenRuntimeErrors)).toBe(0);
    expect(unref(setup.hiddenWarnings)).toBe(0);
    expect(unref(setup.hiddenLogs)).toBe(0);
  });

  it("renders compact diagnostics without a false truncation notice", () => {
    const wrapper = mount(BacktestReportProperties, {
      props: { run: runWithResult(diagnosticResult()) },
      global: { stubs: { "v-icon": true } },
    });
    const text = wrapper.text();

    expect(text).toContain("价格口径");
    expect(text).toContain("券商 market_preset");
    expect(text).toContain("市场 market_preset");
    expect(text).toContain("运行时错误 (1)");
    expect(text).toContain("strategy execution timed out");
    expect(text).toContain("回测警告 (1)");
    expect(text).toContain("loaded 10 closed candles");
    expect(text).toContain("strategy execution failed");
    expect(text).not.toContain("另有");
    expect(text).not.toContain("x1");
  });
});
