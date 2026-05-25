import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

import {
  buildStrategyVisualControlEdgeProperties,
  buildStrategyVisualDataEdgeProperties,
} from "./strategyVisualBuilderEdges";

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
        id: "dma-fast-ma",
        type: "rect",
        x: 450,
        y: 260,
        text: "获取 MA 5",
        properties: {
          blockKind: "getTechnicalIndicator",
          indicatorType: "movingAverage",
          movingAverageType: "MA",
          windowSize: 5,
        },
      },
      {
        id: "dma-slow-ma",
        type: "rect",
        x: 450,
        y: 380,
        text: "获取 MA 20",
        properties: {
          blockKind: "getTechnicalIndicator",
          indicatorType: "movingAverage",
          movingAverageType: "MA",
          windowSize: 20,
        },
      },
      {
        id: "dma-golden-cross",
        type: "diamond",
        x: 750,
        y: 260,
        text: "双均线金叉",
        properties: {
          blockKind: "technicalIndicatorCondition",
          indicatorType: "movingAverage",
          conditionMode: "pattern",
          patternType: "goldenCross",
        },
      },
      {
        id: "dma-golden-buy",
        type: "rect",
        x: 1030,
        y: 200,
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
        x: 750,
        y: 380,
        text: "双均线死叉",
        properties: {
          blockKind: "technicalIndicatorCondition",
          indicatorType: "movingAverage",
          conditionMode: "pattern",
          patternType: "deathCross",
        },
      },
      {
        id: "dma-death-sell",
        type: "rect",
        x: 1030,
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
      { id: "edge-dma-init-log", type: "polyline", sourceNodeId: "dma-init-root", targetNodeId: "dma-init-log", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-dma-fast-ma", type: "polyline", sourceNodeId: "dma-kline-root", targetNodeId: "dma-fast-ma", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-dma-slow-ma", type: "polyline", sourceNodeId: "dma-fast-ma", targetNodeId: "dma-slow-ma", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-dma-golden-cross", type: "polyline", sourceNodeId: "dma-slow-ma", targetNodeId: "dma-golden-cross", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-dma-golden-fast", type: "polyline", sourceNodeId: "dma-fast-ma", targetNodeId: "dma-golden-cross", properties: buildStrategyVisualDataEdgeProperties("fast") },
      { id: "edge-dma-golden-slow", type: "polyline", sourceNodeId: "dma-slow-ma", targetNodeId: "dma-golden-cross", properties: buildStrategyVisualDataEdgeProperties("slow") },
      { id: "edge-dma-golden-buy", type: "polyline", sourceNodeId: "dma-golden-cross", targetNodeId: "dma-golden-buy", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-dma-death-cross", type: "polyline", sourceNodeId: "dma-golden-cross", targetNodeId: "dma-death-cross", properties: buildStrategyVisualControlEdgeProperties("false") },
      { id: "edge-dma-death-fast", type: "polyline", sourceNodeId: "dma-fast-ma", targetNodeId: "dma-death-cross", properties: buildStrategyVisualDataEdgeProperties("fast") },
      { id: "edge-dma-death-slow", type: "polyline", sourceNodeId: "dma-slow-ma", targetNodeId: "dma-death-cross", properties: buildStrategyVisualDataEdgeProperties("slow") },
      { id: "edge-dma-death-sell", type: "polyline", sourceNodeId: "dma-death-cross", targetNodeId: "dma-death-sell", properties: buildStrategyVisualControlEdgeProperties("true") },
    ],
  };
}
