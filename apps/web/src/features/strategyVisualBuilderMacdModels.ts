import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

import {
  buildStrategyVisualControlEdgeProperties,
  buildStrategyVisualDataEdgeProperties,
} from "./strategyVisualBuilderEdges";

export function createMACDMomentumStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "macd-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "macd-init-log", type: "rect", x: 450, y: 120, text: "输出日志", properties: { blockKind: "log", message: "MACD 策略已初始化：${ctx.symbol || '00700'} ${ctx.interval || '1m'}" } },
      { id: "macd-kline-root", type: "circle", x: 180, y: 320, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "macd-getter", type: "rect", x: 470, y: 320, text: "获取 MACD 12/26/9", properties: { blockKind: "getTechnicalIndicator", indicatorType: "macd", fastPeriod: 12, slowPeriod: 26, signalPeriod: 9 } },
      { id: "macd-bullish-node", type: "diamond", x: 760, y: 260, text: "MACD 金叉", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "macd", conditionMode: "pattern", patternType: "goldenCross" } },
      { id: "macd-bullish-buy", type: "rect", x: 1030, y: 210, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "macd-bearish-node", type: "diamond", x: 760, y: 390, text: "MACD 死叉", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "macd", conditionMode: "pattern", patternType: "deathCross" } },
      { id: "macd-bearish-sell", type: "rect", x: 1030, y: 390, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-macd-init-log", type: "polyline", sourceNodeId: "macd-init-root", targetNodeId: "macd-init-log", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-macd-getter", type: "polyline", sourceNodeId: "macd-kline-root", targetNodeId: "macd-getter", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-macd-bullish", type: "polyline", sourceNodeId: "macd-getter", targetNodeId: "macd-bullish-node", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-macd-bullish-input", type: "polyline", sourceNodeId: "macd-getter", targetNodeId: "macd-bullish-node", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-macd-bullish-buy", type: "polyline", sourceNodeId: "macd-bullish-node", targetNodeId: "macd-bullish-buy", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-macd-bearish", type: "polyline", sourceNodeId: "macd-bullish-node", targetNodeId: "macd-bearish-node", properties: buildStrategyVisualControlEdgeProperties("false") },
      { id: "edge-macd-bearish-input", type: "polyline", sourceNodeId: "macd-getter", targetNodeId: "macd-bearish-node", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-macd-bearish-sell", type: "polyline", sourceNodeId: "macd-bearish-node", targetNodeId: "macd-bearish-sell", properties: buildStrategyVisualControlEdgeProperties("true") },
    ],
  };
}
