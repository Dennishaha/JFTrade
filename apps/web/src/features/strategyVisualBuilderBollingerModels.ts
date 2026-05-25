import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

export function createBollingerReversionStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "boll-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "boll-init-log", type: "rect", x: 450, y: 120, text: "输出日志", properties: { blockKind: "log", message: "Bollinger strategy initialized for ${ctx.symbol || '00700'} ${ctx.interval || '1m'}" } },
      { id: "boll-kline-root", type: "circle", x: 180, y: 320, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "boll-lower-node", type: "rect", x: 470, y: 260, text: "布林带 20x2 跌破下轨", properties: { blockKind: "technicalIndicator", indicatorType: "bollinger", conditionMode: "pattern", patternType: "closeBelowLowerBand", period: 20, multiplier: 2 } },
      { id: "boll-lower-buy", type: "rect", x: 810, y: 260, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "boll-upper-node", type: "rect", x: 470, y: 380, text: "布林带 20x2 突破上轨", properties: { blockKind: "technicalIndicator", indicatorType: "bollinger", conditionMode: "pattern", patternType: "closeAboveUpperBand", period: 20, multiplier: 2 } },
      { id: "boll-upper-sell", type: "rect", x: 810, y: 380, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-boll-init-log", type: "polyline", sourceNodeId: "boll-init-root", targetNodeId: "boll-init-log" },
      { id: "edge-boll-lower", type: "polyline", sourceNodeId: "boll-kline-root", targetNodeId: "boll-lower-node" },
      { id: "edge-boll-lower-buy", type: "polyline", sourceNodeId: "boll-lower-node", targetNodeId: "boll-lower-buy" },
      { id: "edge-boll-upper", type: "polyline", sourceNodeId: "boll-kline-root", targetNodeId: "boll-upper-node" },
      { id: "edge-boll-upper-sell", type: "polyline", sourceNodeId: "boll-upper-node", targetNodeId: "boll-upper-sell" },
    ],
  };
}
