import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

export function createMACDMomentumStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "macd-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "macd-init-log", type: "rect", x: 450, y: 120, text: "输出日志", properties: { blockKind: "log", message: "MACD strategy initialized for ${ctx.symbol || '00700'} ${ctx.interval || '1m'}" } },
      { id: "macd-kline-root", type: "circle", x: 180, y: 320, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "macd-bullish-node", type: "rect", x: 470, y: 260, text: "MACD 12/26/9 金叉", properties: { blockKind: "technicalIndicator", indicatorType: "macd", conditionMode: "pattern", patternType: "goldenCross", fastPeriod: 12, slowPeriod: 26, signalPeriod: 9 } },
      { id: "macd-bullish-buy", type: "rect", x: 800, y: 260, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "macd-bearish-node", type: "rect", x: 470, y: 380, text: "MACD 12/26/9 死叉", properties: { blockKind: "technicalIndicator", indicatorType: "macd", conditionMode: "pattern", patternType: "deathCross", fastPeriod: 12, slowPeriod: 26, signalPeriod: 9 } },
      { id: "macd-bearish-sell", type: "rect", x: 800, y: 380, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-macd-init-log", type: "polyline", sourceNodeId: "macd-init-root", targetNodeId: "macd-init-log" },
      { id: "edge-macd-bullish", type: "polyline", sourceNodeId: "macd-kline-root", targetNodeId: "macd-bullish-node" },
      { id: "edge-macd-bullish-buy", type: "polyline", sourceNodeId: "macd-bullish-node", targetNodeId: "macd-bullish-buy" },
      { id: "edge-macd-bearish", type: "polyline", sourceNodeId: "macd-kline-root", targetNodeId: "macd-bearish-node" },
      { id: "edge-macd-bearish-sell", type: "polyline", sourceNodeId: "macd-bearish-node", targetNodeId: "macd-bearish-sell" },
    ],
  };
}
