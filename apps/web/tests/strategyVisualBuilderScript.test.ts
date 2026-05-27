import { describe, expect, it } from "vitest";

import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

import {
  buildStrategyScriptFromVisualModel,
  buildStrategyVisualControlEdgeProperties,
  buildStrategyVisualDataEdgeProperties,
} from "../src/features/strategyVisualBuilder";

describe("strategyVisualBuilderScript", () => {
  it("renders separated indicator getter and condition nodes with true/false branches", () => {
    const visualModel: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        { id: "on-kline-root", type: "circle", x: 180, y: 120, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
        { id: "rsi-getter", type: "rect", x: 420, y: 120, text: "获取 RSI 14", properties: { blockKind: "getTechnicalIndicator", indicatorType: "rsi", period: 14 } },
        { id: "rsi-condition", type: "diamond", x: 680, y: 120, text: "RSI < 30", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "rsi", conditionMode: "numeric", operator: "<", threshold: 30 } },
        { id: "rsi-buy", type: "rect", x: 940, y: 70, text: "下单", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
        { id: "rsi-log", type: "rect", x: 940, y: 180, text: "输出日志", properties: { blockKind: "log", message: "RSI not oversold" } },
      ],
      edges: [
        { id: "edge-root-getter", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "rsi-getter", properties: buildStrategyVisualControlEdgeProperties() },
        { id: "edge-getter-condition", type: "polyline", sourceNodeId: "rsi-getter", targetNodeId: "rsi-condition", properties: buildStrategyVisualControlEdgeProperties() },
        { id: "edge-data-primary", type: "polyline", sourceNodeId: "rsi-getter", targetNodeId: "rsi-condition", properties: buildStrategyVisualDataEdgeProperties("primary") },
        { id: "edge-condition-true", type: "polyline", sourceNodeId: "rsi-condition", targetNodeId: "rsi-buy", properties: buildStrategyVisualControlEdgeProperties("true") },
        { id: "edge-condition-false", type: "polyline", sourceNodeId: "rsi-condition", targetNodeId: "rsi-log", properties: buildStrategyVisualControlEdgeProperties("false") },
      ],
    };

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: "split-rsi",
      symbol: "00700",
      interval: "1m",
    });

    expect(script).toContain("@jftradeFlowBlockKind getTechnicalIndicator");
    expect(script).toContain("@jftradeFlowBlockKind technicalIndicatorCondition");
    expect(script).toContain("const flow_rsi_getter = () => {");
    expect(script).toContain('indicator_rsi_getter_snapshot = ctx.indicators["rsi:14"] ?? null;');
    expect(script).toContain('indicator_rsi_getter_value = indicator_rsi_getter_snapshot;');
    expect(script).toContain("if (Number.isFinite(indicator_rsi_getter_value) && indicator_rsi_getter_value < 30) {");
    expect(script).toContain("} else {");
    expect(script).toContain('console.log("RSI not oversold");');
    expect(script).toContain('placeOrder({ side: "BUY", orderType: "MARKET", quantity: orderQty });');
  });

  it("renders moving-average condition nodes from fast and slow data edges", () => {
    const visualModel: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        { id: "on-kline-root", type: "circle", x: 180, y: 120, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
        { id: "fast-ma", type: "rect", x: 420, y: 70, text: "获取 MA 5", properties: { blockKind: "getTechnicalIndicator", indicatorType: "movingAverage", movingAverageType: "MA", windowSize: 5 } },
        { id: "slow-ma", type: "rect", x: 420, y: 170, text: "获取 MA 20", properties: { blockKind: "getTechnicalIndicator", indicatorType: "movingAverage", movingAverageType: "MA", windowSize: 20 } },
        { id: "ma-condition", type: "diamond", x: 700, y: 120, text: "双均线金叉", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "movingAverage", conditionMode: "pattern", patternType: "goldenCross" } },
        { id: "ma-buy", type: "rect", x: 960, y: 70, text: "下单", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
        { id: "ma-log", type: "rect", x: 960, y: 180, text: "输出日志", properties: { blockKind: "log", message: "均线未金叉" } },
      ],
      edges: [
        { id: "edge-root-fast", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "fast-ma", properties: buildStrategyVisualControlEdgeProperties() },
        { id: "edge-fast-slow", type: "polyline", sourceNodeId: "fast-ma", targetNodeId: "slow-ma", properties: buildStrategyVisualControlEdgeProperties() },
        { id: "edge-slow-condition", type: "polyline", sourceNodeId: "slow-ma", targetNodeId: "ma-condition", properties: buildStrategyVisualControlEdgeProperties() },
        { id: "edge-fast-data", type: "polyline", sourceNodeId: "fast-ma", targetNodeId: "ma-condition", properties: buildStrategyVisualDataEdgeProperties("fast") },
        { id: "edge-slow-data", type: "polyline", sourceNodeId: "slow-ma", targetNodeId: "ma-condition", properties: buildStrategyVisualDataEdgeProperties("slow") },
        { id: "edge-ma-true", type: "polyline", sourceNodeId: "ma-condition", targetNodeId: "ma-buy", properties: buildStrategyVisualControlEdgeProperties("true") },
        { id: "edge-ma-false", type: "polyline", sourceNodeId: "ma-condition", targetNodeId: "ma-log", properties: buildStrategyVisualControlEdgeProperties("false") },
      ],
    };

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: "split-ma",
      symbol: "00700",
      interval: "5m",
    });

    expect(script).toContain("const flow_fast_ma = () => {");
    expect(script).toContain('indicator_fast_ma_snapshot = ctx.indicators["ma:MA:5"] ?? null;');
    expect(script).toContain('indicator_slow_ma_snapshot = ctx.indicators["ma:MA:20"] ?? null;');
    expect(script).toContain("indicator_fast_ma_previous <= indicator_slow_ma_previous");
    expect(script).toContain("indicator_fast_ma_value > indicator_slow_ma_value");
    expect(script).toContain('console.log("均线未金叉");');
  });

  it("renders moving-average condition nodes from node input bindings without visible data edges", () => {
    const visualModel: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        { id: "on-kline-root", type: "circle", x: 180, y: 120, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
        { id: "fast-ma", type: "rect", x: 420, y: 70, text: "获取 双均线 EMA 5", properties: { blockKind: "getTechnicalIndicator", indicatorType: "movingAverage", movingAverageType: "EMA", windowSize: 5, variableName: "EMA5" } },
        { id: "slow-ma", type: "rect", x: 420, y: 170, text: "获取 双均线 EMA 20", properties: { blockKind: "getTechnicalIndicator", indicatorType: "movingAverage", movingAverageType: "EMA", windowSize: 20, variableName: "EMA20" } },
        { id: "ma-condition", type: "diamond", x: 700, y: 120, text: "双均线金叉", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "movingAverage", conditionMode: "pattern", patternType: "goldenCross", inputFastNodeId: "fast-ma", inputSlowNodeId: "slow-ma" } },
      ],
      edges: [
        { id: "edge-root-fast", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "fast-ma", properties: buildStrategyVisualControlEdgeProperties() },
        { id: "edge-fast-slow", type: "polyline", sourceNodeId: "fast-ma", targetNodeId: "slow-ma", properties: buildStrategyVisualControlEdgeProperties() },
        { id: "edge-slow-condition", type: "polyline", sourceNodeId: "slow-ma", targetNodeId: "ma-condition", properties: buildStrategyVisualControlEdgeProperties() },
      ],
    };

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: "split-ma-bindings",
      symbol: "00700",
      interval: "5m",
    });

    expect(script).toContain("@jftradeFlowVariableName EMA5");
    expect(script).toContain("@jftradeFlowInputFastNodeId fast-ma");
    expect(script).toContain("@jftradeFlowInputSlowNodeId slow-ma");
    expect(script).toContain("indicator_fast_ma_previous <= indicator_slow_ma_previous");
    expect(script).toContain("indicator_fast_ma_value > indicator_slow_ma_value");
  });

  it("renders variable-backed conditions even when getter nodes are not on the control path", () => {
    const visualModel: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        { id: "on-kline-root", type: "circle", x: 180, y: 120, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
        { id: "rsi-var", type: "rect", x: 420, y: 120, text: "获取 RSI 14", properties: { blockKind: "getTechnicalIndicator", indicatorType: "rsi", period: 14, variableName: "RSI14" } },
        { id: "rsi-condition", type: "diamond", x: 680, y: 120, text: "RSI < 30", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "rsi", conditionMode: "numeric", operator: "<", threshold: 30, inputPrimaryNodeId: "rsi-var" } },
      ],
      edges: [
        { id: "edge-root-condition", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "rsi-condition", properties: buildStrategyVisualControlEdgeProperties() },
      ],
    };

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: "hidden-variable-rsi",
      symbol: "00700",
      interval: "1m",
    });

    expect(script).toContain("const flow_rsi_var = () => {");
    expect(script).toContain('indicator_rsi_var_snapshot = ctx.indicators["rsi:14"] ?? null;');
    expect(script).toContain('indicator_rsi_var_value = indicator_rsi_var_snapshot;');
    expect(script).toContain("if (Number.isFinite(indicator_rsi_var_value) && indicator_rsi_var_value < 30) {");
  });

  it("keeps legacy position-percent exits compatible and prevents tail-position stalls", () => {
    const visualModel: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        { id: "on-kline-root", type: "circle", x: 180, y: 120, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
        { id: "sell-node", type: "rect", x: 420, y: 120, text: "下单", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "positionPercent", quantityValue: 50 } },
      ],
      edges: [
        { id: "edge-root-sell", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "sell-node", properties: buildStrategyVisualControlEdgeProperties() },
      ],
    };

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: "position-percent-tail-exit",
      symbol: "00700",
      interval: "5m",
    });

    expect(script).toContain("const currentPositionValue = pos ? Math.abs(pos.marketValue) : 0;");
    expect(script).toContain("const rawOrderQty = targetValue > 0 ? Math.floor(targetValue / orderPrice) : 0;");
    expect(script).toContain("const availablePositionQty = pos ? Math.floor(Math.abs(pos.availableQuantity) > 0 ? Math.abs(pos.availableQuantity) : Math.abs(pos.quantity)) : 0;");
    expect(script).toContain("const orderQty = rawOrderQty > 0 ? Math.min(rawOrderQty, availablePositionQty || rawOrderQty) : (true && availablePositionQty > 0 ? 1 : 0);");
  });

  it("renders configurable entry position guards for buy orders", () => {
    const visualModel: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        { id: "on-kline-root", type: "circle", x: 180, y: 120, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
        { id: "buy-node", type: "rect", x: 420, y: 120, text: "下单", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", entryPositionPolicy: "flatOnly", quantityMode: "shares", quantityValue: 100 } },
      ],
      edges: [
        { id: "edge-root-buy", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "buy-node", properties: buildStrategyVisualControlEdgeProperties() },
      ],
    };

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: "buy-entry-policy",
      symbol: "00700",
      interval: "5m",
    });

    expect(script).toContain("if (pos && pos.quantity !== 0) {");
    expect(script).toContain("按必须空仓策略跳过开多");
  });

  it("uses limit price and absolute position value for current symbol position percent orders", () => {
    const visualModel: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        { id: "on-kline-root", type: "circle", x: 180, y: 120, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
        { id: "cover-node", type: "rect", x: 420, y: 120, text: "下单", properties: { blockKind: "placeOrder", side: "BUY_COVER", orderType: "LIMIT", quantityMode: "symbolPositionPercent", quantityValue: 25, limitPrice: 480.5 } },
      ],
      edges: [
        { id: "edge-root-cover", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "cover-node", properties: buildStrategyVisualControlEdgeProperties() },
      ],
    };

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: "symbol-position-percent-limit",
      symbol: "00700",
      interval: "5m",
    });

    expect(script).toContain("const orderPrice = 480.5;");
    expect(script).toContain("const currentPositionValue = pos ? Math.abs(pos.marketValue) : 0;");
    expect(script).toContain("当前标的仓位百分比计算所得数量为 0，跳过下单");
    expect(script).toContain('placeOrder({ side: "BUY", orderType: "LIMIT", limitPrice: 480.5, quantity: orderQty });');
  });

  it("renders account position percent orders against total account value", () => {
    const visualModel: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        { id: "on-kline-root", type: "circle", x: 180, y: 120, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
        { id: "buy-node", type: "rect", x: 420, y: 120, text: "下单", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "accountPositionPercent", quantityValue: 10 } },
      ],
      edges: [
        { id: "edge-root-buy", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "buy-node", properties: buildStrategyVisualControlEdgeProperties() },
      ],
    };

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: "account-position-percent",
      symbol: "00700",
      interval: "5m",
    });

    expect(script).toContain("const accountTotalValue = getTotalAccountValue();");
    expect(script).toContain("const targetAmount = accountTotalValue * 10 / 100;");
    expect(script).toContain("账户仓位百分比计算所得数量为 0（账户总资产 ");
  });

  it("renders margin buying power and short selling power sizing for margin accounts", () => {
    const visualModel: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        { id: "on-kline-root", type: "circle", x: 180, y: 120, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
        { id: "buy-node", type: "rect", x: 420, y: 120, text: "下单", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "marginBuyingPowerPercent", quantityValue: 15 } },
        { id: "short-node", type: "rect", x: 700, y: 120, text: "下单", properties: { blockKind: "placeOrder", side: "SELL_SHORT", orderType: "MARKET", quantityMode: "shortSellingPowerPercent", quantityValue: 20 } },
      ],
      edges: [
        { id: "edge-root-buy", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "buy-node", properties: buildStrategyVisualControlEdgeProperties() },
        { id: "edge-buy-short", type: "polyline", sourceNodeId: "buy-node", targetNodeId: "short-node", properties: buildStrategyVisualControlEdgeProperties() },
      ],
    };

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: "margin-account-sizing",
      symbol: "00700",
      interval: "5m",
    });

    expect(script).toContain("const marginBuyingPower = getMarginBuyingPower();");
    expect(script).toContain("const targetAmount = marginBuyingPower * 15 / 100;");
    expect(script).toContain("融资可用百分比计算所得数量为 0");
    expect(script).toContain("const shortSellingPower = getShortSellingPower();");
    expect(script).toContain("const targetAmount = shortSellingPower * 20 / 100;");
    expect(script).toContain("融券可用百分比计算所得数量为 0");
  });

  it("renders time-unit moving averages and stop-loss snapshots from Go-computed keys", () => {
    const visualModel: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        { id: "on-kline-root", type: "circle", x: 180, y: 120, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
        { id: "day-ma", type: "rect", x: 420, y: 120, text: "获取 均线 EMA 5日", properties: { blockKind: "getTechnicalIndicator", indicatorType: "movingAverage", movingAverageType: "EMA", windowSize: 5, periodUnit: "day" } },
        { id: "stop-loss", type: "rect", x: 700, y: 120, text: "自动止损 1日 2%", properties: { blockKind: "stopLoss", direction: "auto", timeValue: 1, timeUnit: "day", percentage: 2 } },
      ],
      edges: [
        { id: "edge-root-ma", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "day-ma", properties: buildStrategyVisualControlEdgeProperties() },
        { id: "edge-ma-stop-loss", type: "polyline", sourceNodeId: "day-ma", targetNodeId: "stop-loss", properties: buildStrategyVisualControlEdgeProperties() },
      ],
    };

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: "time-unit-stop-loss",
      symbol: "00700",
      interval: "5m",
    });

    expect(script).toContain("const flow_day_ma = () => {");
    expect(script).toContain('indicator_day_ma_snapshot = ctx.indicators["ma:EMA:5:day"] ?? null;');
    expect(script).toContain('const risk_stop_loss_snapshot = ctx.indicators["sl:auto:1:day:2"] ?? null;');
    expect(script).toContain('placeOrder({ side: "SELL", orderType: "MARKET", quantity: risk_stop_loss_qty });');
    expect(script).toContain('placeOrder({ side: "BUY", orderType: "MARKET", quantity: risk_stop_loss_qty });');
  });

  it("renders take-profit and session-aware trailing-stop risk snapshots from Go-computed keys", () => {
    const visualModel: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        { id: "on-kline-root", type: "circle", x: 180, y: 120, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
        { id: "take-profit", type: "rect", x: 420, y: 120, text: "自动止盈 1日 4%", properties: { blockKind: "stopLoss", mode: "takeProfit", direction: "auto", timeValue: 1, timeUnit: "day", percentage: 4, windowPolicy: "continuous" } },
        { id: "trailing-stop", type: "rect", x: 700, y: 120, text: "自动追踪止损 2小时 3% 时段感知", properties: { blockKind: "stopLoss", mode: "trailingStop", direction: "auto", timeValue: 2, timeUnit: "hour", percentage: 3, windowPolicy: "session" } },
      ],
      edges: [
        { id: "edge-root-take-profit", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "take-profit", properties: buildStrategyVisualControlEdgeProperties() },
        { id: "edge-take-profit-trailing", type: "polyline", sourceNodeId: "take-profit", targetNodeId: "trailing-stop", properties: buildStrategyVisualControlEdgeProperties() },
      ],
    };

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: "risk-guard-modes",
      symbol: "00700",
      interval: "5m",
    });

    expect(script).toContain('ctx.indicators["risk:takeProfit:auto:1:day:4:continuous"]');
    expect(script).toContain('ctx.indicators["risk:trailingStop:auto:2:hour:3:session"]');
    expect(script).toContain('risk_take_profit_snapshot.triggerPercent');
    expect(script).toContain('risk_trailing_stop_snapshot.triggerPercent');
  });
});