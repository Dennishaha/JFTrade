import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

import {
  buildStrategyVisualControlEdgeProperties,
  buildStrategyVisualDataEdgeProperties,
} from "./strategyVisualBuilderEdges";

export function createRSIReversionStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "rsi-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "rsi-init-log", type: "rect", x: 450, y: 120, text: "输出日志", properties: { blockKind: "log", message: "RSI strategy initialized for ${ctx.symbol || '00700'} ${ctx.interval || '1m'}" } },
      { id: "rsi-kline-root", type: "circle", x: 180, y: 320, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "rsi-getter", type: "rect", x: 460, y: 320, text: "获取 RSI 14", properties: { blockKind: "getTechnicalIndicator", indicatorType: "rsi", period: 14 } },
      { id: "rsi-oversold-node", type: "diamond", x: 730, y: 260, text: "RSI < 30", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "rsi", conditionMode: "numeric", operator: "<", threshold: 30 } },
      { id: "rsi-oversold-buy", type: "rect", x: 1000, y: 210, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "rsi-overbought-node", type: "diamond", x: 730, y: 390, text: "RSI > 70", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "rsi", conditionMode: "numeric", operator: ">", threshold: 70 } },
      { id: "rsi-overbought-sell", type: "rect", x: 1000, y: 390, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-rsi-init-log", type: "polyline", sourceNodeId: "rsi-init-root", targetNodeId: "rsi-init-log", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-rsi-getter", type: "polyline", sourceNodeId: "rsi-kline-root", targetNodeId: "rsi-getter", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-rsi-oversold", type: "polyline", sourceNodeId: "rsi-getter", targetNodeId: "rsi-oversold-node", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-rsi-oversold-input", type: "polyline", sourceNodeId: "rsi-getter", targetNodeId: "rsi-oversold-node", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-rsi-oversold-buy", type: "polyline", sourceNodeId: "rsi-oversold-node", targetNodeId: "rsi-oversold-buy", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-rsi-overbought", type: "polyline", sourceNodeId: "rsi-oversold-node", targetNodeId: "rsi-overbought-node", properties: buildStrategyVisualControlEdgeProperties("false") },
      { id: "edge-rsi-overbought-input", type: "polyline", sourceNodeId: "rsi-getter", targetNodeId: "rsi-overbought-node", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-rsi-overbought-sell", type: "polyline", sourceNodeId: "rsi-overbought-node", targetNodeId: "rsi-overbought-sell", properties: buildStrategyVisualControlEdgeProperties("true") },
    ],
  };
}
