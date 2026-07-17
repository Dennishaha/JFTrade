import { describe, expect, it } from "vitest";

import type {
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "../src/contracts";
import { buildStrategyPineFromVisualModel } from "../src/features/strategyVisualBuilderPine";

function node(
  id: string,
  blockKind: string,
  properties: Record<string, unknown>,
  type: "circle" | "rect" | "diamond" = "rect",
): StrategyVisualNodeDocument {
  return {
    id,
    type,
    x: 120,
    y: 120,
    text: id,
    properties: { blockKind, ...properties },
  };
}

describe("strategy visual Pine property bindings", () => {
  it("renders moving-average crossings from persisted input references without data edges", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        node("root", "onKLineClosed", {}, "circle"),
        node("fast", "getTechnicalIndicator", {
          indicatorType: "movingAverage",
          movingAverageType: "EMA",
          period: 5,
          variableName: "fast_ma",
        }),
        node("slow", "getTechnicalIndicator", {
          indicatorType: "movingAverage",
          movingAverageType: "SMA",
          period: 20,
          variableName: "slow_ma",
        }),
        node("cross", "technicalIndicatorCondition", {
          indicatorType: "movingAverage",
          conditionMode: "pattern",
          patternType: "deathCross",
          inputFastNodeId: "fast",
          inputSlowNodeId: "slow",
        }, "diamond"),
        node("notify", "notify", { message: "trend reversed" }),
      ],
      edges: [
        { id: "root-fast", type: "polyline", sourceNodeId: "root", targetNodeId: "fast" },
        { id: "fast-slow", type: "polyline", sourceNodeId: "fast", targetNodeId: "slow" },
        { id: "slow-cross", type: "polyline", sourceNodeId: "slow", targetNodeId: "cross" },
        {
          id: "cross-notify",
          type: "polyline",
          sourceNodeId: "cross",
          targetNodeId: "notify",
          properties: { branch: "true" },
        },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, {
      name: "Persisted Binding",
    });

    expect(script).toContain("fast_ma = ta.ema(close, 5)");
    expect(script).toContain("slow_ma = ta.sma(close, 20)");
    expect(script).toContain("if ta.crossunder(fast_ma, slow_ma)");
    expect(script).toContain('alert("trend reversed")');
  });

  it("fails fast for a visual block that has no Pine v6 contract", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        node("root", "onKLineClosed", {}, "circle"),
        node("unknown", "not-a-pine-block", {}),
      ],
      edges: [
        { id: "root-unknown", type: "polyline", sourceNodeId: "root", targetNodeId: "unknown" },
      ],
    };

    expect(() => buildStrategyPineFromVisualModel(model, { name: "Invalid" })).toThrow(
      "不支持的流程图块",
    );
  });
});
