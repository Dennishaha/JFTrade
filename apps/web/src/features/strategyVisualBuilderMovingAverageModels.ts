import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

export function createDoubleMovingAverageStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      {
        id: "dma-init-root",
        type: "circle",
        x: 180,
        y: 120,
        text: "策略启动",
        properties: {
          blockKind: "onInit",
        },
      },
      {
        id: "dma-init-log",
        type: "rect",
        x: 450,
        y: 120,
        text: "输出日志",
        properties: {
          blockKind: "log",
          message: "double moving average initialized for ${ctx.symbol || '00700'} ${ctx.interval || '5m'}",
        },
      },
      {
        id: "dma-kline-root",
        type: "circle",
        x: 180,
        y: 300,
        text: "K 线收盘",
        properties: {
          blockKind: "onKLineClosed",
        },
      },
      {
        id: "dma-fast-average",
        type: "rect",
        x: 420,
        y: 300,
        text: "快均线 5",
        properties: {
          blockKind: "movingAverageFast",
          windowSize: 5,
        },
      },
      {
        id: "dma-slow-average",
        type: "rect",
        x: 670,
        y: 300,
        text: "慢均线 20",
        properties: {
          blockKind: "movingAverageSlow",
          windowSize: 20,
        },
      },
      {
        id: "dma-averages-log",
        type: "rect",
        x: 930,
        y: 220,
        text: "输出日志",
        properties: {
          blockKind: "log",
          message: "fast=${fastAverage.toFixed(2)} slow=${slowAverage.toFixed(2)} close=${close.toFixed(2)}",
        },
      },
      {
        id: "dma-golden-cross",
        type: "diamond",
        x: 930,
        y: 320,
        text: "金叉",
        properties: {
          blockKind: "ifGoldenCross",
        },
      },
      {
        id: "dma-golden-buy",
        type: "rect",
        x: 1180,
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
        id: "dma-death-cross",
        type: "diamond",
        x: 930,
        y: 420,
        text: "死叉",
        properties: {
          blockKind: "ifDeathCross",
        },
      },
      {
        id: "dma-death-sell",
        type: "rect",
        x: 1180,
        y: 420,
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
        id: "edge-dma-init-log",
        type: "polyline",
        sourceNodeId: "dma-init-root",
        targetNodeId: "dma-init-log",
      },
      {
        id: "edge-dma-fast-average",
        type: "polyline",
        sourceNodeId: "dma-kline-root",
        targetNodeId: "dma-fast-average",
      },
      {
        id: "edge-dma-slow-average",
        type: "polyline",
        sourceNodeId: "dma-fast-average",
        targetNodeId: "dma-slow-average",
      },
      {
        id: "edge-dma-averages-log",
        type: "polyline",
        sourceNodeId: "dma-slow-average",
        targetNodeId: "dma-averages-log",
      },
      {
        id: "edge-dma-golden-cross",
        type: "polyline",
        sourceNodeId: "dma-averages-log",
        targetNodeId: "dma-golden-cross",
      },
      {
        id: "edge-dma-death-cross",
        type: "polyline",
        sourceNodeId: "dma-averages-log",
        targetNodeId: "dma-death-cross",
      },
      {
        id: "edge-dma-golden-buy",
        type: "polyline",
        sourceNodeId: "dma-golden-cross",
        targetNodeId: "dma-golden-buy",
      },
      {
        id: "edge-dma-death-sell",
        type: "polyline",
        sourceNodeId: "dma-death-cross",
        targetNodeId: "dma-death-sell",
      },
    ],
  };
}