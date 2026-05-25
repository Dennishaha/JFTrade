import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

export function createRSIReversionStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "rsi-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "rsi-init-log", type: "rect", x: 450, y: 120, text: "输出日志", properties: { blockKind: "log", message: "RSI strategy initialized for ${ctx.symbol || '00700'} ${ctx.interval || '1m'}" } },
      { id: "rsi-kline-root", type: "circle", x: 180, y: 320, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "rsi-oversold-node", type: "rect", x: 460, y: 260, text: "RSI 14 < 30", properties: { blockKind: "technicalIndicator", indicatorType: "rsi", conditionMode: "numeric", operator: "<", threshold: 30, period: 14 } },
      { id: "rsi-oversold-buy", type: "rect", x: 760, y: 260, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "rsi-overbought-node", type: "rect", x: 460, y: 380, text: "RSI 14 > 70", properties: { blockKind: "technicalIndicator", indicatorType: "rsi", conditionMode: "numeric", operator: ">", threshold: 70, period: 14 } },
      { id: "rsi-overbought-sell", type: "rect", x: 760, y: 380, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-rsi-init-log", type: "polyline", sourceNodeId: "rsi-init-root", targetNodeId: "rsi-init-log" },
      { id: "edge-rsi-oversold", type: "polyline", sourceNodeId: "rsi-kline-root", targetNodeId: "rsi-oversold-node" },
      { id: "edge-rsi-oversold-buy", type: "polyline", sourceNodeId: "rsi-oversold-node", targetNodeId: "rsi-oversold-buy" },
      { id: "edge-rsi-overbought", type: "polyline", sourceNodeId: "rsi-kline-root", targetNodeId: "rsi-overbought-node" },
      { id: "edge-rsi-overbought-sell", type: "polyline", sourceNodeId: "rsi-overbought-node", targetNodeId: "rsi-overbought-sell" },
    ],
  };
}
