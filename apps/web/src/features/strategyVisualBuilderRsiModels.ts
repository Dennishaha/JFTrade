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
        id: "rsi-oversold-notify",
        type: "rect",
        x: 950,
        y: 280,
        text: "发送通知",
        properties: {
          blockKind: "notify",
          message: "RSI oversold on ${ctx.symbol || '00700'} ${ctx.interval || '1m'} value=${latestRsi.toFixed(2)} close=${close.toFixed(2)}",
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
        id: "rsi-overbought-notify",
        type: "rect",
        x: 950,
        y: 430,
        text: "发送通知",
        properties: {
          blockKind: "notify",
          message: "RSI overbought on ${ctx.symbol || '00700'} ${ctx.interval || '1m'} value=${latestRsi.toFixed(2)} close=${close.toFixed(2)}",
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
        id: "edge-rsi-oversold-notify",
        type: "polyline",
        sourceNodeId: "rsi-oversold-node",
        targetNodeId: "rsi-oversold-notify",
      },
      {
        id: "edge-rsi-overbought-notify",
        type: "polyline",
        sourceNodeId: "rsi-overbought-node",
        targetNodeId: "rsi-overbought-notify",
      },
    ],
  };
}