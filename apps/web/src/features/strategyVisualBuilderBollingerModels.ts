import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

export function createBollingerReversionStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      {
        id: "boll-init-root",
        type: "circle",
        x: 180,
        y: 120,
        text: "策略启动",
        properties: {
          blockKind: "onInit",
        },
      },
      {
        id: "boll-init-log",
        type: "rect",
        x: 450,
        y: 120,
        text: "输出日志",
        properties: {
          blockKind: "log",
          message: "Bollinger strategy initialized for ${ctx.symbol || '00700'} ${ctx.interval || '1m'}",
        },
      },
      {
        id: "boll-kline-root",
        type: "circle",
        x: 180,
        y: 320,
        text: "K 线收盘",
        properties: {
          blockKind: "onKLineClosed",
        },
      },
      {
        id: "boll-calc-node",
        type: "rect",
        x: 430,
        y: 320,
        text: "布林带 20x2",
        properties: {
          blockKind: "bollinger",
          period: 20,
          multiplier: 2,
        },
      },
      {
        id: "boll-log-node",
        type: "rect",
        x: 730,
        y: 220,
        text: "输出日志",
        properties: {
          blockKind: "log",
          message: "BOLL mid=${latestBollingerMiddle.toFixed(2)} upper=${latestBollingerUpper.toFixed(2)} lower=${latestBollingerLower.toFixed(2)} close=${close.toFixed(2)}",
        },
      },
      {
        id: "boll-upper-node",
        type: "diamond",
        x: 730,
        y: 320,
        text: "收盘价 > 上轨",
        properties: {
          blockKind: "ifCloseAboveUpperBand",
        },
      },
      {
        id: "boll-upper-sell",
        type: "rect",
        x: 1000,
        y: 320,
        text: "下单 · 卖出平多 · 100 股",
        properties: {
          blockKind: "placeOrder",
          side: "SELL",
          orderType: "MARKET",
          quantityMode: "shares",
          quantityValue: 100,
        },
      },
      {
        id: "boll-lower-node",
        type: "diamond",
        x: 730,
        y: 430,
        text: "收盘价 < 下轨",
        properties: {
          blockKind: "ifCloseBelowLowerBand",
        },
      },
      {
        id: "boll-lower-buy",
        type: "rect",
        x: 1000,
        y: 430,
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
        id: "edge-boll-init-log",
        type: "polyline",
        sourceNodeId: "boll-init-root",
        targetNodeId: "boll-init-log",
      },
      {
        id: "edge-boll-calc",
        type: "polyline",
        sourceNodeId: "boll-kline-root",
        targetNodeId: "boll-calc-node",
      },
      {
        id: "edge-boll-log",
        type: "polyline",
        sourceNodeId: "boll-calc-node",
        targetNodeId: "boll-log-node",
      },
      {
        id: "edge-boll-upper",
        type: "polyline",
        sourceNodeId: "boll-log-node",
        targetNodeId: "boll-upper-node",
      },
      {
        id: "edge-boll-lower",
        type: "polyline",
        sourceNodeId: "boll-log-node",
        targetNodeId: "boll-lower-node",
      },
      {
        id: "edge-boll-upper-sell",
        type: "polyline",
        sourceNodeId: "boll-upper-node",
        targetNodeId: "boll-upper-sell",
      },
      {
        id: "edge-boll-lower-buy",
        type: "polyline",
        sourceNodeId: "boll-lower-node",
        targetNodeId: "boll-lower-buy",
      },
    ],
  };
}