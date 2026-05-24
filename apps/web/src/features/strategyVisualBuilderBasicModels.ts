import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

export function createDefaultStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      {
        id: "on-init-root",
        type: "circle",
        x: 180,
        y: 120,
        text: "策略启动",
        properties: {
          blockKind: "onInit",
        },
      },
      {
        id: "init-log",
        type: "rect",
        x: 420,
        y: 120,
        text: "输出日志",
        properties: {
          blockKind: "log",
          message: "策略启动，等待市场数据输入",
        },
      },
      {
        id: "on-kline-root",
        type: "circle",
        x: 180,
        y: 300,
        text: "K 线收盘",
        properties: {
          blockKind: "onKLineClosed",
        },
      },
      {
        id: "kline-log",
        type: "rect",
        x: 440,
        y: 300,
        text: "输出日志",
        properties: {
          blockKind: "log",
          message: "收盘价更新: ${ctx.kline.close}",
        },
      },
    ],
    edges: [
      {
        id: "edge-init-log",
        type: "polyline",
        sourceNodeId: "on-init-root",
        targetNodeId: "init-log",
      },
      {
        id: "edge-kline-log",
        type: "polyline",
        sourceNodeId: "on-kline-root",
        targetNodeId: "kline-log",
      },
    ],
  };
}

export function createBreakoutStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      {
        id: "breakout-root",
        type: "circle",
        x: 180,
        y: 220,
        text: "K 线收盘",
        properties: {
          blockKind: "onKLineClosed",
        },
      },
      {
        id: "breakout-condition",
        type: "diamond",
        x: 420,
        y: 220,
        text: "收盘价 > 阈值",
        properties: {
          blockKind: "ifCloseAbove",
          threshold: 520,
        },
      },
      {
        id: "breakout-buy",
        type: "rect",
        x: 680,
        y: 220,
        text: "下单 · 买入开多 · 100 股",
        properties: {
          blockKind: "placeOrder",
          side: "BUY",
          orderType: "MARKET",
          quantityMode: "shares",
          quantityValue: 100,
        },
      },
    ],
    edges: [
      {
        id: "edge-breakout-condition",
        type: "polyline",
        sourceNodeId: "breakout-root",
        targetNodeId: "breakout-condition",
      },
      {
        id: "edge-breakout-buy",
        type: "polyline",
        sourceNodeId: "breakout-condition",
        targetNodeId: "breakout-buy",
      },
    ],
  };
}

export function createMeanReversionStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      {
        id: "mean-revert-root",
        type: "circle",
        x: 180,
        y: 220,
        text: "K 线收盘",
        properties: {
          blockKind: "onKLineClosed",
        },
      },
      {
        id: "mean-revert-condition",
        type: "diamond",
        x: 420,
        y: 220,
        text: "收盘价 < 阈值",
        properties: {
          blockKind: "ifCloseBelow",
          threshold: 480,
        },
      },
      {
        id: "mean-revert-log",
        type: "rect",
        x: 680,
        y: 150,
        text: "输出日志",
        properties: {
          blockKind: "log",
          message: "均值回归关注区间，close=${ctx.kline.close}",
        },
      },
      {
        id: "mean-revert-buy",
        type: "rect",
        x: 680,
        y: 290,
        text: "下单 · 买入开多 · 100 股",
        properties: {
          blockKind: "placeOrder",
          side: "BUY",
          orderType: "MARKET",
          quantityMode: "shares",
          quantityValue: 100,
        },
      },
    ],
    edges: [
      {
        id: "edge-mean-revert-condition",
        type: "polyline",
        sourceNodeId: "mean-revert-root",
        targetNodeId: "mean-revert-condition",
      },
      {
        id: "edge-mean-revert-log",
        type: "polyline",
        sourceNodeId: "mean-revert-condition",
        targetNodeId: "mean-revert-log",
      },
      {
        id: "edge-mean-revert-buy",
        type: "polyline",
        sourceNodeId: "mean-revert-condition",
        targetNodeId: "mean-revert-buy",
      },
    ],
  };
}