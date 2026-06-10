import type { StrategyVisualModelDocument } from "@/contracts";

import {
  buildStrategyVisualControlEdgeProperties,
  buildStrategyVisualDataEdgeProperties,
} from "./strategyVisualBuilderEdges";

export function createBollingerReversionStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "boll-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "boll-init-log", type: "rect", x: 450, y: 120, text: "输出日志", properties: { blockKind: "log", message: "布林带策略已初始化：${ctx.symbol || '00700'} ${ctx.interval || '1m'}" } },
      { id: "boll-kline-root", type: "circle", x: 180, y: 320, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "boll-getter", type: "rect", x: 470, y: 320, text: "获取布林带 20x2", properties: { blockKind: "getTechnicalIndicator", indicatorType: "bollinger", period: 20, multiplier: 2 } },
      { id: "boll-lower-node", type: "diamond", x: 760, y: 260, text: "布林带跌破下轨", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "bollinger", conditionMode: "pattern", patternType: "closeBelowLowerBand" } },
      { id: "boll-lower-buy", type: "rect", x: 1050, y: 210, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "boll-upper-node", type: "diamond", x: 760, y: 390, text: "布林带突破上轨", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "bollinger", conditionMode: "pattern", patternType: "closeAboveUpperBand" } },
      { id: "boll-upper-sell", type: "rect", x: 1050, y: 390, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-boll-init-log", type: "polyline", sourceNodeId: "boll-init-root", targetNodeId: "boll-init-log", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-boll-getter", type: "polyline", sourceNodeId: "boll-kline-root", targetNodeId: "boll-getter", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-boll-lower", type: "polyline", sourceNodeId: "boll-getter", targetNodeId: "boll-lower-node", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-boll-lower-input", type: "polyline", sourceNodeId: "boll-getter", targetNodeId: "boll-lower-node", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-boll-lower-buy", type: "polyline", sourceNodeId: "boll-lower-node", targetNodeId: "boll-lower-buy", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-boll-upper", type: "polyline", sourceNodeId: "boll-lower-node", targetNodeId: "boll-upper-node", properties: buildStrategyVisualControlEdgeProperties("false") },
      { id: "edge-boll-upper-input", type: "polyline", sourceNodeId: "boll-getter", targetNodeId: "boll-upper-node", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-boll-upper-sell", type: "polyline", sourceNodeId: "boll-upper-node", targetNodeId: "boll-upper-sell", properties: buildStrategyVisualControlEdgeProperties("true") },
    ],
  };
}
