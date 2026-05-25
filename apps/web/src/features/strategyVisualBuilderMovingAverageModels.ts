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
        properties: { blockKind: "onInit" },
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
        y: 320,
        text: "K 线收盘",
        properties: { blockKind: "onKLineClosed" },
      },
      {
        id: "dma-golden-cross",
        type: "rect",
        x: 470,
        y: 260,
        text: "双均线 5/20 金叉",
        properties: {
          blockKind: "technicalIndicator",
          indicatorType: "movingAverage",
          conditionMode: "pattern",
          patternType: "goldenCross",
          fastPeriod: 5,
          slowPeriod: 20,
        },
      },
      {
        id: "dma-golden-buy",
        type: "rect",
        x: 780,
        y: 260,
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
        type: "rect",
        x: 470,
        y: 380,
        text: "双均线 5/20 死叉",
        properties: {
          blockKind: "technicalIndicator",
          indicatorType: "movingAverage",
          conditionMode: "pattern",
          patternType: "deathCross",
          fastPeriod: 5,
          slowPeriod: 20,
        },
      },
      {
        id: "dma-death-sell",
        type: "rect",
        x: 780,
        y: 380,
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
      { id: "edge-dma-init-log", type: "polyline", sourceNodeId: "dma-init-root", targetNodeId: "dma-init-log" },
      { id: "edge-dma-golden-cross", type: "polyline", sourceNodeId: "dma-kline-root", targetNodeId: "dma-golden-cross" },
      { id: "edge-dma-golden-buy", type: "polyline", sourceNodeId: "dma-golden-cross", targetNodeId: "dma-golden-buy" },
      { id: "edge-dma-death-cross", type: "polyline", sourceNodeId: "dma-kline-root", targetNodeId: "dma-death-cross" },
      { id: "edge-dma-death-sell", type: "polyline", sourceNodeId: "dma-death-cross", targetNodeId: "dma-death-sell" },
    ],
  };
}
