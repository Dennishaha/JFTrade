import type { StrategyVisualModelDocument } from "@/contracts";

import { buildStrategyVisualControlEdgeProperties } from "./strategyVisualBuilderEdges";

export function createParameterizedMtfTrendStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "param-mtf-length", type: "rect", x: 180, y: 120, text: "参数 trend_len = 20", properties: { blockKind: "strategyInput", variableName: "trend_len", inputType: "int", title: "Trend Length", defaultValue: 20 } },
      { id: "param-mtf-root", type: "circle", x: 180, y: 300, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "param-mtf-series", type: "rect", x: 500, y: 300, text: "MTF daily_trend · D", properties: { blockKind: "mtfSeries", variableName: "daily_trend", timeframe: "D", expressionType: "indicator", indicatorExpression: "ta.ema(close, trend_len)" } },
      { id: "param-mtf-cross", type: "rect", x: 820, y: 300, text: "派生 price_cross · cross", properties: { blockKind: "derivedSeries", variableName: "price_cross", mode: "cross", crossFunction: "crossover", leftExpression: "close", rightExpression: "daily_trend" } },
      { id: "param-mtf-order", type: "rect", x: 1140, y: 300, text: "下单 · 买入开多 · 10%", properties: { blockKind: "placeOrder", orderAction: "entry", orderId: "Long", side: "BUY", orderType: "MARKET", quantityMode: "equityPercent", quantityValue: 10 } },
    ],
    edges: [
      { id: "edge-param-mtf-series", type: "polyline", sourceNodeId: "param-mtf-root", targetNodeId: "param-mtf-series", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-param-mtf-cross", type: "polyline", sourceNodeId: "param-mtf-series", targetNodeId: "param-mtf-cross", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-param-mtf-order", type: "polyline", sourceNodeId: "param-mtf-cross", targetNodeId: "param-mtf-order", properties: buildStrategyVisualControlEdgeProperties() },
    ],
  };
}

export function createCooldownStateStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "cooldown-root", type: "circle", x: 180, y: 260, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "cooldown-state", type: "rect", x: 480, y: 260, text: "状态 cooldown = 0", properties: { blockKind: "stateVariable", variableName: "cooldown", valueType: "number", initialValue: 0 } },
      { id: "cooldown-update", type: "rect", x: 780, y: 260, text: "更新状态 cooldown", properties: { blockKind: "stateUpdate", variableName: "cooldown", expression: "math.max(cooldown - 1, 0)" } },
      { id: "cooldown-price", type: "diamond", x: 1080, y: 260, text: "收盘价 > 520", properties: { blockKind: "ifCloseAbove", threshold: 520 } },
      { id: "cooldown-order", type: "rect", x: 1380, y: 220, text: "下单 · 买入开多 · 5%", properties: { blockKind: "placeOrder", orderAction: "entry", orderId: "Long", side: "BUY", orderType: "MARKET", quantityMode: "equityPercent", quantityValue: 5 } },
      { id: "cooldown-reset", type: "rect", x: 1680, y: 220, text: "更新状态 cooldown", properties: { blockKind: "stateUpdate", variableName: "cooldown", expression: "5" } },
    ],
    edges: [
      { id: "edge-cooldown-state", type: "polyline", sourceNodeId: "cooldown-root", targetNodeId: "cooldown-state", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-cooldown-update", type: "polyline", sourceNodeId: "cooldown-state", targetNodeId: "cooldown-update", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-cooldown-price", type: "polyline", sourceNodeId: "cooldown-update", targetNodeId: "cooldown-price", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-cooldown-order", type: "polyline", sourceNodeId: "cooldown-price", targetNodeId: "cooldown-order", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-cooldown-reset", type: "polyline", sourceNodeId: "cooldown-order", targetNodeId: "cooldown-reset", properties: buildStrategyVisualControlEdgeProperties() },
    ],
  };
}

export function createPartialExitStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "partial-root", type: "circle", x: 180, y: 260, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "partial-breakout", type: "diamond", x: 480, y: 260, text: "收盘价 > 520", properties: { blockKind: "ifCloseAbove", threshold: 520 } },
      { id: "partial-entry", type: "rect", x: 780, y: 220, text: "下单 · 买入开多 · 10%", properties: { blockKind: "placeOrder", orderAction: "entry", orderId: "Long", side: "BUY", orderType: "MARKET", quantityMode: "equityPercent", quantityValue: 10 } },
      { id: "partial-exit", type: "rect", x: 1080, y: 220, text: "多头止盈止损 1柱 2%", properties: { blockKind: "stopLoss", mode: "bracketExit", direction: "long", timeValue: 1, timeUnit: "bar", percentage: 2, takeProfitPercentage: 4, quantityPercentage: 50, windowPolicy: "continuous" } },
    ],
    edges: [
      { id: "edge-partial-breakout", type: "polyline", sourceNodeId: "partial-root", targetNodeId: "partial-breakout", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-partial-entry", type: "polyline", sourceNodeId: "partial-breakout", targetNodeId: "partial-entry", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-partial-exit", type: "polyline", sourceNodeId: "partial-entry", targetNodeId: "partial-exit", properties: buildStrategyVisualControlEdgeProperties() },
    ],
  };
}

export function createCollectionStatFilterStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "collection-root", type: "circle", x: 180, y: 260, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "collection-stat", type: "rect", x: 480, y: 260, text: "集合统计 range_median · median", properties: { blockKind: "collectionStat", variableName: "range_median", statFunction: "median", sourceA: "close", sourceB: "open", sourceC: "high" } },
      { id: "collection-signal", type: "rect", x: 780, y: 260, text: "派生 range_signal · math", properties: { blockKind: "derivedSeries", variableName: "range_signal", mode: "math", mathFunction: "max", leftExpression: "range_median", rightExpression: "low" } },
      { id: "collection-filter", type: "diamond", x: 1080, y: 260, text: "收盘价 > 520", properties: { blockKind: "ifCloseAbove", threshold: 520 } },
      { id: "collection-order", type: "rect", x: 1380, y: 220, text: "下单 · 买入开多 · 5%", properties: { blockKind: "placeOrder", orderAction: "entry", orderId: "Long", side: "BUY", orderType: "MARKET", quantityMode: "equityPercent", quantityValue: 5 } },
    ],
    edges: [
      { id: "edge-collection-stat", type: "polyline", sourceNodeId: "collection-root", targetNodeId: "collection-stat", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-collection-signal", type: "polyline", sourceNodeId: "collection-stat", targetNodeId: "collection-signal", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-collection-filter", type: "polyline", sourceNodeId: "collection-signal", targetNodeId: "collection-filter", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-collection-order", type: "polyline", sourceNodeId: "collection-filter", targetNodeId: "collection-order", properties: buildStrategyVisualControlEdgeProperties("true") },
    ],
  };
}

export function createTimeSessionMtfRiskStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "time-session-length", type: "rect", x: 180, y: 120, text: "参数 trend_len = 20", properties: { blockKind: "strategyInput", variableName: "trend_len", inputType: "int", title: "Trend Length", defaultValue: 20 } },
      { id: "time-session-root", type: "circle", x: 180, y: 300, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "time-session-time", type: "diamond", x: 480, y: 300, text: "时间过滤 · 09:30-16:00", properties: { blockKind: "timeFilter", mode: "between", startHour: 9, startMinute: 30, endHour: 16, endMinute: 0 } },
      { id: "time-session-session", type: "diamond", x: 780, y: 300, text: "时段过滤 · 常规交易时段", properties: { blockKind: "sessionFilter", scope: "market" } },
      {
        id: "time-session-mtf",
        type: "rect",
        x: 1080,
        y: 300,
        text: "MTF daily_trend · D",
        properties: {
          blockKind: "mtfSeries",
          variableName: "daily_trend",
          timeframe: "D",
          expressionType: "indicator",
          indicatorExpression: "ta.ema(close, trend_len)",
          indicatorExpressionAst: {
            kind: "call",
            functionName: "ta.ema",
            args: [{ kind: "source", source: "close" }, { kind: "reference", name: "trend_len" }],
          },
        },
      },
      { id: "time-session-filter", type: "diamond", x: 1380, y: 300, text: "Close > daily_trend", properties: { blockKind: "seriesCondition", mode: "compare", operator: ">", leftExpressionAst: { kind: "source", source: "close" }, rightExpressionAst: { kind: "reference", name: "daily_trend" } } },
      { id: "time-session-order", type: "rect", x: 1680, y: 260, text: "下单 · 买入开多 · 10%", properties: { blockKind: "placeOrder", orderAction: "entry", orderId: "Long", side: "BUY", orderType: "MARKET", quantityMode: "equityPercent", quantityValue: 10 } },
      { id: "time-session-exit", type: "rect", x: 1980, y: 260, text: "多头止盈止损 1柱", properties: { blockKind: "stopLoss", mode: "bracketExit", direction: "long", timeValue: 1, timeUnit: "bar", percentage: 2, takeProfitPercentage: 4, quantityPercentage: 50, windowPolicy: "continuous" } },
    ],
    edges: [
      { id: "edge-time-session-time", type: "polyline", sourceNodeId: "time-session-root", targetNodeId: "time-session-time", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-time-session-session", type: "polyline", sourceNodeId: "time-session-time", targetNodeId: "time-session-session", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-time-session-mtf", type: "polyline", sourceNodeId: "time-session-session", targetNodeId: "time-session-mtf", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-time-session-filter", type: "polyline", sourceNodeId: "time-session-mtf", targetNodeId: "time-session-filter", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-time-session-order", type: "polyline", sourceNodeId: "time-session-filter", targetNodeId: "time-session-order", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-time-session-exit", type: "polyline", sourceNodeId: "time-session-order", targetNodeId: "time-session-exit", properties: buildStrategyVisualControlEdgeProperties() },
    ],
  };
}

export function createStructuredLimitExitStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "structured-limit-root", type: "circle", x: 180, y: 260, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "structured-limit-condition", type: "diamond", x: 480, y: 260, text: "Close > Open", properties: { blockKind: "seriesCondition", mode: "compare", operator: ">", leftExpressionAst: { kind: "source", source: "close" }, rightExpressionAst: { kind: "source", source: "open" } } },
      {
        id: "structured-limit-order",
        type: "rect",
        x: 780,
        y: 220,
        text: "表达式限价开仓",
        properties: {
          blockKind: "placeOrder",
          orderAction: "entry",
          orderId: "Long",
          side: "BUY",
          orderType: "LIMIT",
          quantityMode: "equityPercent",
          quantityValue: 10,
          limitPriceExpressionAst: { kind: "call", functionName: "math.max", args: [{ kind: "source", source: "close" }, { kind: "source", source: "open" }] },
          stopPriceExpressionAst: { kind: "history", target: { kind: "source", source: "low" }, offset: 1 },
        },
      },
      {
        id: "structured-limit-exit",
        type: "rect",
        x: 1080,
        y: 220,
        text: "表达式部分退出",
        properties: {
          blockKind: "stopLoss",
          mode: "bracketExit",
          direction: "long",
          timeValue: 1,
          timeUnit: "bar",
          windowPolicy: "continuous",
          quantityPercentage: 50,
          stopPriceExpressionAst: { kind: "history", target: { kind: "source", source: "low" }, offset: 1 },
          takeProfitPriceExpressionAst: { kind: "history", target: { kind: "source", source: "high" }, offset: 1 },
        },
      },
    ],
    edges: [
      { id: "edge-structured-limit-condition", type: "polyline", sourceNodeId: "structured-limit-root", targetNodeId: "structured-limit-condition", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-structured-limit-order", type: "polyline", sourceNodeId: "structured-limit-condition", targetNodeId: "structured-limit-order", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-structured-limit-exit", type: "polyline", sourceNodeId: "structured-limit-order", targetNodeId: "structured-limit-exit", properties: buildStrategyVisualControlEdgeProperties() },
    ],
  };
}
