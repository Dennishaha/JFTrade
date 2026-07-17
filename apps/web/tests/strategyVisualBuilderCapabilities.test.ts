import { describe, expect, it } from "vitest";

import type { StrategyVisualNodeDocument } from "../src/contracts";
import {
  assessPineBlockSupport,
  getVisualBlockCapabilities,
  getVisualBlockCapability,
  summarizePineBlockSupport,
} from "../src/features/strategyVisualBuilderCapabilities";

function node(kind: string, properties: Record<string, unknown> = {}): StrategyVisualNodeDocument {
  return {
    id: `${kind}-node`,
    type: "rect",
    x: 0,
    y: 0,
    text: kind,
    properties: { blockKind: kind, ...properties },
  };
}

describe("strategy visual-builder capabilities", () => {
  it("returns the catalog and rejects absent or unknown capability kinds", () => {
    const capabilities = getVisualBlockCapabilities();
    expect(capabilities.length).toBeGreaterThan(10);
    expect(getVisualBlockCapability(null)).toBeNull();
    expect(getVisualBlockCapability(undefined)).toBeNull();
    expect(getVisualBlockCapability("not-a-block" as never)).toBeNull();
    expect(assessPineBlockSupport(null)).toMatchObject({
      status: "unsupportedConfig",
      label: "未知图块",
    });
    expect(assessPineBlockSupport(node("unknown"))).toMatchObject({
      status: "unsupportedConfig",
      label: "未知图块",
    });
  });

  it("describes native warning, indicator, condition, risk, and stop-loss support", () => {
    expect(assessPineBlockSupport(node("onInit"))).toMatchObject({
      status: "warning",
      label: "闭盘执行",
    });
    expect(assessPineBlockSupport(node("getTechnicalIndicator", { indicatorType: "rsi" }))).toMatchObject({
      status: "supported",
      label: "指标可运行",
    });
    expect(assessPineBlockSupport(node("technicalIndicatorCondition", {
      indicatorType: "macd",
      conditionMode: "pattern",
    }))).toMatchObject({
      status: "supported",
      label: "条件可运行",
    });
    expect(assessPineBlockSupport(node("riskRule", { riskRuleType: "allowEntryIn" }))).toMatchObject({
      status: "supported",
      label: "方向风控可运行",
    });
    expect(assessPineBlockSupport(node("riskRule", { riskRuleType: "maxDrawdown" }))).toMatchObject({
      status: "supported",
      label: "阈值风控可运行",
    });
    expect(assessPineBlockSupport(node("stopLoss", {
      windowPolicy: "continuous",
      timeUnit: "bar",
      timeValue: 1,
    })).status).toBe("supported");
    expect(assessPineBlockSupport(node("stopLoss", {
      windowPolicy: "fixed",
      timeUnit: "day",
      timeValue: 2,
    }))).toMatchObject({
      status: "unsupportedConfig",
      label: "配置不支持",
    });
  });

  it("summarizes warnings and unsupported configurations across a model", () => {
    const summary = summarizePineBlockSupport({
      engine: "pine",
      version: 6,
      nodes: [node("onInit"), node("stopLoss", { windowPolicy: "session" }), node("log")],
      edges: [],
    });
    expect(summary).toEqual({ unsupportedConfigCount: 1, warningCount: 1 });
  });
});
