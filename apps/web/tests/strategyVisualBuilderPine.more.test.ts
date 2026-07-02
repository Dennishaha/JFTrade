import { describe, expect, it } from "vitest";

import type { StrategyVisualModelDocument, StrategyVisualNodeDocument } from "@/contracts";

import { buildStrategyPineFromVisualModel } from "../src/features/strategyVisualBuilderPine";

function node(
  id: string,
  blockKind: string,
  text: string,
  properties: Record<string, unknown>,
  type: "circle" | "rect" | "diamond" = "rect",
): StrategyVisualNodeDocument {
  return {
    id,
    type,
    x: 120,
    y: 120,
    text,
    properties: { blockKind, ...properties },
  };
}

describe("strategyVisualBuilderPine additional business boundaries", () => {
  it("renders fallback Pine for empty models and input-driven control chains", () => {
    const empty = buildStrategyPineFromVisualModel(null, { name: "  " });
    expect(empty).toContain('strategy("未命名策略"');
    expect(empty).toContain('log.info("策略尚未配置入口图块")');

    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        node("root", "onKLineClosed", "K 线收盘", {}, "circle"),
        node("input-color", "strategyInput", "颜色", {
          variableName: "theme",
          inputType: "color",
          title: "Theme",
          defaultValue: "teal",
        }),
        node("state-weird", "stateVariable", "状态 Weird", {
          variableName: "weird",
          valueType: "number",
          initialValue: {},
        }),
        node("falling", "seriesCondition", "连续下跌", {
          mode: "falling",
          source: "close",
          length: 4,
        }, "diamond"),
        node("notify", "notify", "发提醒", { message: "downtrend" }),
      ],
      edges: [
        { id: "edge-root-input", type: "polyline", sourceNodeId: "root", targetNodeId: "input-color" },
        { id: "edge-input-state", type: "polyline", sourceNodeId: "input-color", targetNodeId: "state-weird" },
        { id: "edge-state-falling", type: "polyline", sourceNodeId: "state-weird", targetNodeId: "falling" },
        { id: "edge-falling-notify", type: "polyline", sourceNodeId: "falling", targetNodeId: "notify", properties: { branch: "true" } },
        { id: "edge-falling-missing", type: "polyline", sourceNodeId: "falling", targetNodeId: "missing-node", properties: { branch: "false" } },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Edge Cases" });

    expect(script).toContain('theme = input.color("teal", "Theme")');
    expect(script).toContain("var weird = 0");
    expect(script).toContain("if ta.falling(close, 4)");
    expect(script).toContain('alert("downtrend")');
  });

  it("renders indicator, order, risk, and protection fallbacks for partially bound blocks", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        node("root", "onKLineClosed", "K 线收盘", {}, "circle"),
        node("vwap-node", "getTechnicalIndicator", "VWAP", {
          indicatorType: "vwap",
          source: "hlc3",
          timeframe: "D",
          variableName: "vwapFlow",
        }),
        node("obv-node", "getTechnicalIndicator", "OBV", {
          indicatorType: "obv",
          timeframe: "D",
          variableName: "obvFlow",
        }),
        node("kdj-node", "getTechnicalIndicator", "KDJ", {
          indicatorType: "kdj",
          variableName: "1 bad",
        }),
        node("rsi-condition", "technicalIndicatorCondition", "RSI 阈值", {
          indicatorType: "rsi",
          conditionMode: "numeric",
          threshold: 50,
        }, "diamond"),
        node("macd-condition", "technicalIndicatorCondition", "MACD 背离", {
          indicatorType: "macd",
          conditionMode: "pattern",
          patternType: "topDivergence",
        }, "diamond"),
        node("kdj-condition", "technicalIndicatorCondition", "KDJ 背离", {
          indicatorType: "kdj",
          conditionMode: "pattern",
          patternType: "topDivergence",
          inputPrimaryNodeId: "kdj-node",
        }, "diamond"),
        node("boll-condition", "technicalIndicatorCondition", "布林突破", {
          indicatorType: "bollinger",
          conditionMode: "pattern",
          patternType: "closeAboveUpperBand",
        }, "diamond"),
        node("fallback-risk", "riskRule", "默认风控", {
          riskRuleType: "unsupported",
        }),
        node("close-node", "placeOrder", "平空", {
          side: "BUY_COVER",
          quantityMode: "shares",
          quantityValue: 3,
          immediately: "later",
          disable_alert: "nope",
        }),
        node("take-profit-short", "stopLoss", "空头止盈", {
          mode: "takeProfit",
          direction: "short",
          percentage: 3,
          quantityPercentage: 50,
          fromEntryMode: "explicit",
        }),
      ],
      edges: [
        { id: "edge-root-vwap", type: "polyline", sourceNodeId: "root", targetNodeId: "vwap-node" },
        { id: "edge-vwap-obv", type: "polyline", sourceNodeId: "vwap-node", targetNodeId: "obv-node" },
        { id: "edge-obv-kdj", type: "polyline", sourceNodeId: "obv-node", targetNodeId: "kdj-node" },
        { id: "edge-kdj-rsi", type: "polyline", sourceNodeId: "kdj-node", targetNodeId: "rsi-condition" },
        { id: "edge-root-rsi-data", type: "polyline", sourceNodeId: "root", targetNodeId: "rsi-condition", properties: { role: "data", slot: "primary" } },
        { id: "edge-rsi-macd", type: "polyline", sourceNodeId: "rsi-condition", targetNodeId: "macd-condition" },
        { id: "edge-macd-kdjc", type: "polyline", sourceNodeId: "macd-condition", targetNodeId: "kdj-condition" },
        { id: "edge-kdjc-boll", type: "polyline", sourceNodeId: "kdj-condition", targetNodeId: "boll-condition" },
        { id: "edge-boll-risk", type: "polyline", sourceNodeId: "boll-condition", targetNodeId: "fallback-risk" },
        { id: "edge-risk-close", type: "polyline", sourceNodeId: "fallback-risk", targetNodeId: "close-node" },
        { id: "edge-close-protect", type: "polyline", sourceNodeId: "close-node", targetNodeId: "take-profit-short" },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Indicator Edges" });

    expect(script).toContain("vwapFlow = ta.vwap(hlc3)");
    expect(script).toContain('obvFlow = request.security(syminfo.tickerid, "D", ta.obv)');
    expect(script).toContain("kdj_node_j = 3 * kdj_node_k - 2 * kdj_node_d");
    expect(script).toContain("if false");
    expect(script).toContain("strategy.risk.max_drawdown(10, strategy.percent_of_equity)");
    expect(script).toContain('strategy.close("Short", qty=3)');
    expect(script).not.toContain("immediately=");
    expect(script).not.toContain("disable_alert=");
    expect(script).toContain('strategy.exit("Short takeProfit", "Short", limit=close * (1 - 3 / 100), qty_percent=50)');
  });

  it("uses property-linked indicator inputs, ignores unsupported indicator timeframes, and breaks cyclic child chains", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        node("root", "onKLineClosed", "K 线收盘", {}, "circle"),
        node("relay-root", "onInit", "嵌套入口", {}, "circle"),
        node("wpr-node", "getTechnicalIndicator", "WPR", {
          indicatorType: "williamsR",
          period: 21,
          timeframe: "240",
          variableName: "wpr_flow",
        }),
        node("ma-fast", "getTechnicalIndicator", "EMA Fast", {
          indicatorType: "movingAverage",
          movingAverageType: "EMA",
          source: "close",
          windowSize: 9,
          variableName: "fast_ma",
        }),
        node("ma-slow", "getTechnicalIndicator", "EMA Slow", {
          indicatorType: "movingAverage",
          movingAverageType: "EMA",
          source: "close",
          windowSize: 21,
          variableName: "slow_ma",
        }),
        node("ma-condition", "technicalIndicatorCondition", "均线交叉", {
          indicatorType: "movingAverage",
          conditionMode: "pattern",
          patternType: "goldenCross",
          inputFastNodeId: "ma-fast",
          inputSlowNodeId: "ma-slow",
        }, "diamond"),
        node("kdj-condition", "technicalIndicatorCondition", "KDJ 缺失输入", {
          indicatorType: "kdj",
          conditionMode: "pattern",
          patternType: "goldenCross",
        }, "diamond"),
        node("notify", "notify", "发送提醒", { message: "cross ready" }),
      ],
      edges: [
        { id: "edge-root-relay", type: "polyline", sourceNodeId: "root", targetNodeId: "relay-root" },
        { id: "edge-relay-wpr", type: "polyline", sourceNodeId: "relay-root", targetNodeId: "wpr-node" },
        { id: "edge-wpr-ma", type: "polyline", sourceNodeId: "wpr-node", targetNodeId: "ma-condition" },
        {
          id: "edge-ma-true",
          type: "polyline",
          sourceNodeId: "ma-condition",
          targetNodeId: "notify",
          properties: { branch: "true" },
        },
        {
          id: "edge-ma-false",
          type: "polyline",
          sourceNodeId: "ma-condition",
          targetNodeId: "kdj-condition",
          properties: { branch: "false" },
        },
        { id: "edge-notify-self", type: "polyline", sourceNodeId: "notify", targetNodeId: "notify" },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Linked Inputs" });

    expect(script).toContain("wpr_flow = ta.wpr(21)");
    expect(script).not.toContain('request.security(syminfo.tickerid, "240", ta.wpr(21))');
    expect(script).toContain("fast_ma = ta.ema(close, 9)");
    expect(script).toContain("slow_ma = ta.ema(close, 21)");
    expect(script).toContain("if ta.crossover(fast_ma, slow_ma)");
    expect(script).toContain("if false");
    expect(script).toContain('alert("cross ready")');
  });
});
