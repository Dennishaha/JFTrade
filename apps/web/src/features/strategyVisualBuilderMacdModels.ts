import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

export function createMACDMomentumStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      {
        id: "macd-init-root",
        type: "circle",
        x: 180,
        y: 120,
        text: "策略启动",
        properties: {
          blockKind: "onInit",
        },
      },
      {
        id: "macd-init-log",
        type: "rect",
        x: 450,
        y: 120,
        text: "输出日志",
        properties: {
          blockKind: "log",
          message: "MACD strategy initialized for ${ctx.symbol || '00700'} ${ctx.interval || '1m'}",
        },
      },
      {
        id: "macd-kline-root",
        type: "circle",
        x: 180,
        y: 320,
        text: "K 线收盘",
        properties: {
          blockKind: "onKLineClosed",
        },
      },
      {
        id: "macd-calc-node",
        type: "rect",
        x: 430,
        y: 320,
        text: "MACD 12/26/9",
        properties: {
          blockKind: "macd",
          fastPeriod: 12,
          slowPeriod: 26,
          signalPeriod: 9,
        },
      },
      {
        id: "macd-log-node",
        type: "rect",
        x: 720,
        y: 220,
        text: "输出日志",
        properties: {
          blockKind: "log",
          message: "MACD diff=${latestMacdDiff.toFixed(2)} signal=${latestMacdSignal.toFixed(2)} hist=${latestMacdHistogram.toFixed(2)}",
        },
      },
      {
        id: "macd-bullish-node",
        type: "diamond",
        x: 720,
        y: 320,
        text: "MACD 多头",
        properties: {
          blockKind: "ifMacdBullish",
        },
      },
      {
        id: "macd-bullish-notify",
        type: "rect",
        x: 980,
        y: 280,
        text: "发送通知",
        properties: {
          blockKind: "notify",
          message: "MACD bullish on ${ctx.symbol || '00700'} ${ctx.interval || '1m'} diff=${latestMacdDiff.toFixed(2)} signal=${latestMacdSignal.toFixed(2)}",
        },
      },
      {
        id: "macd-bearish-node",
        type: "diamond",
        x: 720,
        y: 430,
        text: "MACD 空头",
        properties: {
          blockKind: "ifMacdBearish",
        },
      },
      {
        id: "macd-bearish-notify",
        type: "rect",
        x: 980,
        y: 430,
        text: "发送通知",
        properties: {
          blockKind: "notify",
          message: "MACD bearish on ${ctx.symbol || '00700'} ${ctx.interval || '1m'} diff=${latestMacdDiff.toFixed(2)} signal=${latestMacdSignal.toFixed(2)}",
        },
      },
    ],
    edges: [
      {
        id: "edge-macd-init-log",
        type: "polyline",
        sourceNodeId: "macd-init-root",
        targetNodeId: "macd-init-log",
      },
      {
        id: "edge-macd-calc",
        type: "polyline",
        sourceNodeId: "macd-kline-root",
        targetNodeId: "macd-calc-node",
      },
      {
        id: "edge-macd-log",
        type: "polyline",
        sourceNodeId: "macd-calc-node",
        targetNodeId: "macd-log-node",
      },
      {
        id: "edge-macd-bullish",
        type: "polyline",
        sourceNodeId: "macd-log-node",
        targetNodeId: "macd-bullish-node",
      },
      {
        id: "edge-macd-bearish",
        type: "polyline",
        sourceNodeId: "macd-log-node",
        targetNodeId: "macd-bearish-node",
      },
      {
        id: "edge-macd-bullish-notify",
        type: "polyline",
        sourceNodeId: "macd-bullish-node",
        targetNodeId: "macd-bullish-notify",
      },
      {
        id: "edge-macd-bearish-notify",
        type: "polyline",
        sourceNodeId: "macd-bearish-node",
        targetNodeId: "macd-bearish-notify",
      },
    ],
  };
}