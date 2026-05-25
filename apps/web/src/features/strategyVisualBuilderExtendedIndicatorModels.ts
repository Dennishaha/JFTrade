import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

export function createKDJReversionStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "kdj-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "kdj-init-log", type: "rect", x: 450, y: 120, text: "输出日志", properties: { blockKind: "log", message: "KDJ strategy initialized for ${ctx.symbol || '00700'} ${ctx.interval || '1m'}" } },
      { id: "kdj-kline-root", type: "circle", x: 180, y: 320, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "kdj-buy-signal", type: "rect", x: 470, y: 260, text: "KDJ 9/3/3 金叉", properties: { blockKind: "technicalIndicator", indicatorType: "kdj", conditionMode: "pattern", patternType: "goldenCross", period: 9, m1: 3, m2: 3 } },
      { id: "kdj-buy-order", type: "rect", x: 780, y: 260, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "kdj-sell-signal", type: "rect", x: 470, y: 380, text: "KDJ 9/3/3 死叉", properties: { blockKind: "technicalIndicator", indicatorType: "kdj", conditionMode: "pattern", patternType: "deathCross", period: 9, m1: 3, m2: 3 } },
      { id: "kdj-sell-order", type: "rect", x: 780, y: 380, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-kdj-init-log", type: "polyline", sourceNodeId: "kdj-init-root", targetNodeId: "kdj-init-log" },
      { id: "edge-kdj-buy-signal", type: "polyline", sourceNodeId: "kdj-kline-root", targetNodeId: "kdj-buy-signal" },
      { id: "edge-kdj-buy-order", type: "polyline", sourceNodeId: "kdj-buy-signal", targetNodeId: "kdj-buy-order" },
      { id: "edge-kdj-sell-signal", type: "polyline", sourceNodeId: "kdj-kline-root", targetNodeId: "kdj-sell-signal" },
      { id: "edge-kdj-sell-order", type: "polyline", sourceNodeId: "kdj-sell-signal", targetNodeId: "kdj-sell-order" },
    ],
  };
}

export function createATRVolatilityStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "atr-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "atr-init-log", type: "rect", x: 450, y: 120, text: "输出日志", properties: { blockKind: "log", message: "ATR strategy initialized for ${ctx.symbol || '00700'} ${ctx.interval || '1m'}" } },
      { id: "atr-kline-root", type: "circle", x: 180, y: 320, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "atr-high", type: "rect", x: 470, y: 260, text: "ATR 14 > 2", properties: { blockKind: "technicalIndicator", indicatorType: "atr", conditionMode: "numeric", operator: ">", threshold: 2, period: 14 } },
      { id: "atr-buy-order", type: "rect", x: 760, y: 260, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "atr-low", type: "rect", x: 470, y: 380, text: "ATR 14 < 1", properties: { blockKind: "technicalIndicator", indicatorType: "atr", conditionMode: "numeric", operator: "<", threshold: 1, period: 14 } },
      { id: "atr-sell-order", type: "rect", x: 760, y: 380, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-atr-init-log", type: "polyline", sourceNodeId: "atr-init-root", targetNodeId: "atr-init-log" },
      { id: "edge-atr-high", type: "polyline", sourceNodeId: "atr-kline-root", targetNodeId: "atr-high" },
      { id: "edge-atr-buy-order", type: "polyline", sourceNodeId: "atr-high", targetNodeId: "atr-buy-order" },
      { id: "edge-atr-low", type: "polyline", sourceNodeId: "atr-kline-root", targetNodeId: "atr-low" },
      { id: "edge-atr-sell-order", type: "polyline", sourceNodeId: "atr-low", targetNodeId: "atr-sell-order" },
    ],
  };
}

export function createCCIReversionStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "cci-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "cci-init-log", type: "rect", x: 450, y: 120, text: "输出日志", properties: { blockKind: "log", message: "CCI strategy initialized for ${ctx.symbol || '00700'} ${ctx.interval || '1m'}" } },
      { id: "cci-kline-root", type: "circle", x: 180, y: 320, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "cci-buy-signal", type: "rect", x: 470, y: 260, text: "CCI 20 < -100", properties: { blockKind: "technicalIndicator", indicatorType: "cci", conditionMode: "numeric", operator: "<", threshold: -100, period: 20 } },
      { id: "cci-buy-order", type: "rect", x: 760, y: 260, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "cci-sell-signal", type: "rect", x: 470, y: 380, text: "CCI 20 > 100", properties: { blockKind: "technicalIndicator", indicatorType: "cci", conditionMode: "numeric", operator: ">", threshold: 100, period: 20 } },
      { id: "cci-sell-order", type: "rect", x: 760, y: 380, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-cci-init-log", type: "polyline", sourceNodeId: "cci-init-root", targetNodeId: "cci-init-log" },
      { id: "edge-cci-buy-signal", type: "polyline", sourceNodeId: "cci-kline-root", targetNodeId: "cci-buy-signal" },
      { id: "edge-cci-buy-order", type: "polyline", sourceNodeId: "cci-buy-signal", targetNodeId: "cci-buy-order" },
      { id: "edge-cci-sell-signal", type: "polyline", sourceNodeId: "cci-kline-root", targetNodeId: "cci-sell-signal" },
      { id: "edge-cci-sell-order", type: "polyline", sourceNodeId: "cci-sell-signal", targetNodeId: "cci-sell-order" },
    ],
  };
}

export function createWilliamsRReversionStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "wr-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "wr-init-log", type: "rect", x: 450, y: 120, text: "输出日志", properties: { blockKind: "log", message: "Williams %R strategy initialized for ${ctx.symbol || '00700'} ${ctx.interval || '1m'}" } },
      { id: "wr-kline-root", type: "circle", x: 180, y: 320, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "wr-buy-signal", type: "rect", x: 470, y: 260, text: "Williams %R 14 < -80", properties: { blockKind: "technicalIndicator", indicatorType: "williamsR", conditionMode: "numeric", operator: "<", threshold: -80, period: 14 } },
      { id: "wr-buy-order", type: "rect", x: 790, y: 260, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "wr-sell-signal", type: "rect", x: 470, y: 380, text: "Williams %R 14 > -20", properties: { blockKind: "technicalIndicator", indicatorType: "williamsR", conditionMode: "numeric", operator: ">", threshold: -20, period: 14 } },
      { id: "wr-sell-order", type: "rect", x: 790, y: 380, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-wr-init-log", type: "polyline", sourceNodeId: "wr-init-root", targetNodeId: "wr-init-log" },
      { id: "edge-wr-buy-signal", type: "polyline", sourceNodeId: "wr-kline-root", targetNodeId: "wr-buy-signal" },
      { id: "edge-wr-buy-order", type: "polyline", sourceNodeId: "wr-buy-signal", targetNodeId: "wr-buy-order" },
      { id: "edge-wr-sell-signal", type: "polyline", sourceNodeId: "wr-kline-root", targetNodeId: "wr-sell-signal" },
      { id: "edge-wr-sell-order", type: "polyline", sourceNodeId: "wr-sell-signal", targetNodeId: "wr-sell-order" },
    ],
  };
}
