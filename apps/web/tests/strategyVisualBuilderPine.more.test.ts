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

  it("renders executable advanced indicator, order, and protection policies without silently relaxing them", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        node("root", "onKLineClosed", "K 线收盘", {}, "circle"),
        node("alma", "getTechnicalIndicator", "ALMA", {
          indicatorType: "alma",
          source: "ohlc4",
          period: 34,
          offset: 0.9,
          sigma: 5,
          timeframe: "D",
          variableName: "alma_daily",
        }),
        node("dmi", "getTechnicalIndicator", "DMI", {
          indicatorType: "dmi",
          period: 10,
          adxSmoothing: 7,
          timeframe: "D",
          variableName: "dmi_signal",
        }),
        node("kdj", "getTechnicalIndicator", "KDJ", {
          indicatorType: "kdj",
          variableName: "kdj_fast",
          period: 7,
          m1: 2,
          m2: 4,
        }),
        node("kdj-threshold", "technicalIndicatorCondition", "KDJ 超买", {
          indicatorType: "kdj",
          conditionMode: "numeric",
          operator: ">",
          threshold: 68,
          inputPrimaryNodeId: "kdj",
        }, "diamond"),
        node("recent-pullback", "seriesCondition", "最近回撤", {
          mode: "valuewhen",
          eventSource: "high",
          eventOperator: "<",
          eventThreshold: 120,
          valueSource: "low",
          occurrence: 2,
          operator: ">=",
          threshold: 4,
        }, "diamond"),
        node("flatten", "placeOrder", "收盘", {
          orderAction: "closeAll",
          immediately: true,
          comment: " risk-off\nflatten ",
          alert_message: "flattened",
          disable_alert: "false",
        }),
        node("cancel", "placeOrder", "取消挂单", {
          orderAction: "cancel",
          orderId: "  staged long  ",
        }),
        node("risk", "riskRule", "连续亏损", {
          riskRuleType: "maxConsLossDays",
          riskCount: 2,
          alert_message: "pause strategy",
        }),
        node("windowed-protect", "stopLoss", "时段止损", {
          mode: "stopLoss",
          windowPolicy: "marketSession",
          timeUnit: "day",
          timeValue: 1,
        }),
      ],
      edges: [
        { id: "root-alma", type: "polyline", sourceNodeId: "root", targetNodeId: "alma" },
        { id: "alma-dmi", type: "polyline", sourceNodeId: "alma", targetNodeId: "dmi" },
        { id: "dmi-kdj", type: "polyline", sourceNodeId: "dmi", targetNodeId: "kdj" },
        { id: "kdj-condition", type: "polyline", sourceNodeId: "kdj", targetNodeId: "kdj-threshold" },
        {
          id: "kdj-data",
          type: "polyline",
          sourceNodeId: "kdj",
          targetNodeId: "kdj-threshold",
          properties: { role: "data", slot: "primary" },
        },
        {
          id: "condition-true",
          type: "polyline",
          sourceNodeId: "kdj-threshold",
          targetNodeId: "recent-pullback",
          properties: { branch: "true" },
        },
        {
          id: "condition-false",
          type: "polyline",
          sourceNodeId: "kdj-threshold",
          targetNodeId: "cancel",
          properties: { branch: "false" },
        },
        {
          id: "pullback-true",
          type: "polyline",
          sourceNodeId: "recent-pullback",
          targetNodeId: "flatten",
          properties: { branch: "true" },
        },
        {
          id: "flatten-risk",
          type: "polyline",
          sourceNodeId: "flatten",
          targetNodeId: "risk",
        },
        {
          id: "risk-protect",
          type: "polyline",
          sourceNodeId: "risk",
          targetNodeId: "windowed-protect",
        },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Advanced Policy" });

    expect(script).toContain('alma_daily = request.security(syminfo.tickerid, "D", ta.alma(ohlc4, 34, 0.9, 5))');
    expect(script).toContain("dmi_signal = ta.dmi(10, 7)");
    expect(script).not.toContain('request.security(syminfo.tickerid, "D", ta.dmi');
    expect(script).toContain("kdj_fast_j = 3 * kdj_fast_k - 2 * kdj_fast_d");
    expect(script).toContain("if kdj_fast_j > 68");
    expect(script).toContain("ta.valuewhen(high < 120, low, 2) > 4");
    expect(script).toContain('strategy.close_all(immediately=true, comment="risk-off flatten", alert_message="flattened", disable_alert=false)');
    expect(script).toContain('strategy.cancel("staged long")');
    expect(script).toContain('strategy.risk.max_cons_loss_days(2, alert_message="pause strategy")');
    expect(script).toContain('runtime.error("JFTrade Pine 暂不支持带时间窗口或交易时段感知的自动退出图块")');

    const legacyModel: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        node("legacy-root", "onKLineClosed", "K 线收盘", {}, "circle"),
        node("legacy-code", "codeBlock", "遗留代码", {}),
      ],
      edges: [{ id: "legacy-edge", type: "polyline", sourceNodeId: "legacy-root", targetNodeId: "legacy-code" }],
    };
    expect(() => buildStrategyPineFromVisualModel(legacyModel, { name: "Legacy" })).toThrow(
      "旧流程图块 codeBlock 不再支持",
    );
  });

  it("keeps malformed persisted wiring inert while still rendering valid reachable actions once", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        node("root", "onKLineClosed", "K 线收盘", {}, "circle"),
        node("cycle-log", "log", "循环日志", { message: "only once" }),
        node("rsi", "getTechnicalIndicator", "RSI", {
          indicatorType: "rsi",
          period: 14,
          variableName: "rsi_14",
        }),
        node("unbound-check", "technicalIndicatorCondition", "缺少指标引用", {
          indicatorType: "rsi",
          conditionMode: "numeric",
          operator: ">",
          threshold: 70,
          inputPrimaryNodeId: "missing-indicator",
        }, "diamond"),
        node("notify", "notify", "风险提示", { message: "review exposure" }),
      ],
      edges: [
        { id: "root-cycle", type: "polyline", sourceNodeId: "root", targetNodeId: "cycle-log" },
        { id: "cycle-root", type: "polyline", sourceNodeId: "cycle-log", targetNodeId: "root" },
        { id: "root-rsi", type: "polyline", sourceNodeId: "root", targetNodeId: "rsi" },
        { id: "rsi-check", type: "polyline", sourceNodeId: "rsi", targetNodeId: "unbound-check" },
        {
          id: "root-invalid-data",
          type: "polyline",
          sourceNodeId: "root",
          targetNodeId: "unbound-check",
          properties: { role: "data", slot: "primary" },
        },
        {
          id: "check-notify",
          type: "polyline",
          sourceNodeId: "unbound-check",
          targetNodeId: "notify",
          properties: { branch: "true" },
        },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Persisted Graph Repair" });

    expect(script.match(/log\.info\("only once"\)/g)).toHaveLength(1);
    expect(script).toContain("rsi_14 = ta.rsi(close, 14)");
    expect(script).toContain("if false");
    expect(script).toContain('alert("review exposure")');
  });
});
