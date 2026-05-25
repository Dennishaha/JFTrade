import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

import {
  buildStrategyVisualControlEdgeProperties,
  buildStrategyVisualDataEdgeProperties,
} from "./strategyVisualBuilderEdges";

export function createKDJReversionStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "kdj-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "kdj-init-log", type: "rect", x: 450, y: 120, text: "输出日志", properties: { blockKind: "log", message: "KDJ strategy initialized for ${ctx.symbol || '00700'} ${ctx.interval || '1m'}" } },
      { id: "kdj-kline-root", type: "circle", x: 180, y: 320, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "kdj-getter", type: "rect", x: 470, y: 320, text: "获取 KDJ 9/3/3", properties: { blockKind: "getTechnicalIndicator", indicatorType: "kdj", period: 9, m1: 3, m2: 3 } },
      { id: "kdj-buy-signal", type: "diamond", x: 760, y: 260, text: "KDJ 金叉", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "kdj", conditionMode: "pattern", patternType: "goldenCross" } },
      { id: "kdj-buy-order", type: "rect", x: 1040, y: 210, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "kdj-sell-signal", type: "diamond", x: 760, y: 390, text: "KDJ 死叉", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "kdj", conditionMode: "pattern", patternType: "deathCross" } },
      { id: "kdj-sell-order", type: "rect", x: 1040, y: 390, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-kdj-init-log", type: "polyline", sourceNodeId: "kdj-init-root", targetNodeId: "kdj-init-log", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-kdj-getter", type: "polyline", sourceNodeId: "kdj-kline-root", targetNodeId: "kdj-getter", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-kdj-buy-signal", type: "polyline", sourceNodeId: "kdj-getter", targetNodeId: "kdj-buy-signal", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-kdj-buy-input", type: "polyline", sourceNodeId: "kdj-getter", targetNodeId: "kdj-buy-signal", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-kdj-buy-order", type: "polyline", sourceNodeId: "kdj-buy-signal", targetNodeId: "kdj-buy-order", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-kdj-sell-signal", type: "polyline", sourceNodeId: "kdj-buy-signal", targetNodeId: "kdj-sell-signal", properties: buildStrategyVisualControlEdgeProperties("false") },
      { id: "edge-kdj-sell-input", type: "polyline", sourceNodeId: "kdj-getter", targetNodeId: "kdj-sell-signal", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-kdj-sell-order", type: "polyline", sourceNodeId: "kdj-sell-signal", targetNodeId: "kdj-sell-order", properties: buildStrategyVisualControlEdgeProperties("true") },
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
      { id: "atr-getter", type: "rect", x: 470, y: 320, text: "获取 ATR 14", properties: { blockKind: "getTechnicalIndicator", indicatorType: "atr", period: 14 } },
      { id: "atr-high", type: "diamond", x: 740, y: 260, text: "ATR > 2", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "atr", conditionMode: "numeric", operator: ">", threshold: 2 } },
      { id: "atr-buy-order", type: "rect", x: 1010, y: 210, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "atr-low", type: "diamond", x: 740, y: 390, text: "ATR < 1", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "atr", conditionMode: "numeric", operator: "<", threshold: 1 } },
      { id: "atr-sell-order", type: "rect", x: 1010, y: 390, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-atr-init-log", type: "polyline", sourceNodeId: "atr-init-root", targetNodeId: "atr-init-log", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-atr-getter", type: "polyline", sourceNodeId: "atr-kline-root", targetNodeId: "atr-getter", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-atr-high", type: "polyline", sourceNodeId: "atr-getter", targetNodeId: "atr-high", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-atr-high-input", type: "polyline", sourceNodeId: "atr-getter", targetNodeId: "atr-high", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-atr-buy-order", type: "polyline", sourceNodeId: "atr-high", targetNodeId: "atr-buy-order", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-atr-low", type: "polyline", sourceNodeId: "atr-high", targetNodeId: "atr-low", properties: buildStrategyVisualControlEdgeProperties("false") },
      { id: "edge-atr-low-input", type: "polyline", sourceNodeId: "atr-getter", targetNodeId: "atr-low", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-atr-sell-order", type: "polyline", sourceNodeId: "atr-low", targetNodeId: "atr-sell-order", properties: buildStrategyVisualControlEdgeProperties("true") },
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
      { id: "cci-getter", type: "rect", x: 470, y: 320, text: "获取 CCI 20", properties: { blockKind: "getTechnicalIndicator", indicatorType: "cci", period: 20 } },
      { id: "cci-buy-signal", type: "diamond", x: 740, y: 260, text: "CCI < -100", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "cci", conditionMode: "numeric", operator: "<", threshold: -100 } },
      { id: "cci-buy-order", type: "rect", x: 1010, y: 210, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "cci-sell-signal", type: "diamond", x: 740, y: 390, text: "CCI > 100", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "cci", conditionMode: "numeric", operator: ">", threshold: 100 } },
      { id: "cci-sell-order", type: "rect", x: 1010, y: 390, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-cci-init-log", type: "polyline", sourceNodeId: "cci-init-root", targetNodeId: "cci-init-log", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-cci-getter", type: "polyline", sourceNodeId: "cci-kline-root", targetNodeId: "cci-getter", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-cci-buy-signal", type: "polyline", sourceNodeId: "cci-getter", targetNodeId: "cci-buy-signal", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-cci-buy-input", type: "polyline", sourceNodeId: "cci-getter", targetNodeId: "cci-buy-signal", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-cci-buy-order", type: "polyline", sourceNodeId: "cci-buy-signal", targetNodeId: "cci-buy-order", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-cci-sell-signal", type: "polyline", sourceNodeId: "cci-buy-signal", targetNodeId: "cci-sell-signal", properties: buildStrategyVisualControlEdgeProperties("false") },
      { id: "edge-cci-sell-input", type: "polyline", sourceNodeId: "cci-getter", targetNodeId: "cci-sell-signal", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-cci-sell-order", type: "polyline", sourceNodeId: "cci-sell-signal", targetNodeId: "cci-sell-order", properties: buildStrategyVisualControlEdgeProperties("true") },
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
      { id: "wr-getter", type: "rect", x: 470, y: 320, text: "获取 Williams %R 14", properties: { blockKind: "getTechnicalIndicator", indicatorType: "williamsR", period: 14 } },
      { id: "wr-buy-signal", type: "diamond", x: 770, y: 260, text: "Williams %R < -80", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "williamsR", conditionMode: "numeric", operator: "<", threshold: -80 } },
      { id: "wr-buy-order", type: "rect", x: 1070, y: 210, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "wr-sell-signal", type: "diamond", x: 770, y: 390, text: "Williams %R > -20", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "williamsR", conditionMode: "numeric", operator: ">", threshold: -20 } },
      { id: "wr-sell-order", type: "rect", x: 1070, y: 390, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-wr-init-log", type: "polyline", sourceNodeId: "wr-init-root", targetNodeId: "wr-init-log", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-wr-getter", type: "polyline", sourceNodeId: "wr-kline-root", targetNodeId: "wr-getter", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-wr-buy-signal", type: "polyline", sourceNodeId: "wr-getter", targetNodeId: "wr-buy-signal", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-wr-buy-input", type: "polyline", sourceNodeId: "wr-getter", targetNodeId: "wr-buy-signal", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-wr-buy-order", type: "polyline", sourceNodeId: "wr-buy-signal", targetNodeId: "wr-buy-order", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-wr-sell-signal", type: "polyline", sourceNodeId: "wr-buy-signal", targetNodeId: "wr-sell-signal", properties: buildStrategyVisualControlEdgeProperties("false") },
      { id: "edge-wr-sell-input", type: "polyline", sourceNodeId: "wr-getter", targetNodeId: "wr-sell-signal", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-wr-sell-order", type: "polyline", sourceNodeId: "wr-sell-signal", targetNodeId: "wr-sell-order", properties: buildStrategyVisualControlEdgeProperties("true") },
    ],
  };
}
