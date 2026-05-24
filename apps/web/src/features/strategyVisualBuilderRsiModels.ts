import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

export function createRSIReversionStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      {
        id: "rsi-init-root",
        type: "circle",
        x: 180,
        y: 120,
        text: "策略启动",
        properties: {
          blockKind: "onInit",
        },
      },
      {
        id: "rsi-init-log",
        type: "rect",
        x: 450,
        y: 120,
        text: "输出日志",
        properties: {
          blockKind: "log",
          message: "RSI strategy initialized for ${ctx.symbol || '00700'} ${ctx.interval || '1m'}",
        },
      },
      {
        id: "rsi-kline-root",
        type: "circle",
        x: 180,
        y: 320,
        text: "K 线收盘",
        properties: {
          blockKind: "onKLineClosed",
        },
      },
      {
        id: "rsi-calc-node",
        type: "rect",
        x: 430,
        y: 320,
        text: "RSI 14",
        properties: {
          blockKind: "rsi",
          period: 14,
        },
      },
      {
        id: "rsi-log-node",
        type: "rect",
        x: 700,
        y: 220,
        text: "输出日志",
        properties: {
          blockKind: "log",
          message: "RSI=${latestRsi.toFixed(2)} close=${close.toFixed(2)}",
        },
      },
      {
        id: "rsi-oversold-node",
        type: "diamond",
        x: 700,
        y: 320,
        text: "RSI < 30",
        properties: {
          blockKind: "ifRsiBelow",
          threshold: 30,
        },
      },
      {
        id: "rsi-oversold-buy",
        type: "rect",
        x: 950,
        y: 320,
        text: "下单 · 买入开多 · 100 股",
        properties: {
          blockKind: "placeOrder",
          side: "BUY",
          orderType: "MARKET",
          quantityMode: "shares",
          quantityValue: 100,
        },
      },
      {
        id: "rsi-overbought-node",
        type: "diamond",
        x: 700,
        y: 430,
        text: "RSI > 70",
        properties: {
          blockKind: "ifRsiAbove",
          threshold: 70,
        },
      },
      {
        id: "rsi-overbought-sell",
        type: "rect",
        x: 950,
        y: 430,
        text: "下单 · 卖出平多 · 100 股",
        properties: {
          blockKind: "placeOrder",
          side: "SELL",
          orderType: "MARKET",
          quantityMode: "shares",
          quantityValue: 100,
        },
      },
    ],
    edges: [
      {
        id: "edge-rsi-init-log",
        type: "polyline",
        sourceNodeId: "rsi-init-root",
        targetNodeId: "rsi-init-log",
      },
      {
        id: "edge-rsi-calc",
        type: "polyline",
        sourceNodeId: "rsi-kline-root",
        targetNodeId: "rsi-calc-node",
      },
      {
        id: "edge-rsi-log",
        type: "polyline",
        sourceNodeId: "rsi-calc-node",
        targetNodeId: "rsi-log-node",
      },
      {
        id: "edge-rsi-oversold",
        type: "polyline",
        sourceNodeId: "rsi-log-node",
        targetNodeId: "rsi-oversold-node",
      },
      {
        id: "edge-rsi-overbought",
        type: "polyline",
        sourceNodeId: "rsi-log-node",
        targetNodeId: "rsi-overbought-node",
      },
      {
        id: "edge-rsi-oversold-buy",
        type: "polyline",
        sourceNodeId: "rsi-oversold-node",
        targetNodeId: "rsi-oversold-buy",
      },
      {
        id: "edge-rsi-overbought-sell",
        type: "polyline",
        sourceNodeId: "rsi-overbought-node",
        targetNodeId: "rsi-overbought-sell",
      },
    ],
  };
}