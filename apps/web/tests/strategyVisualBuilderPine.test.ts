import { describe, expect, it } from "vitest";

import type { StrategyVisualModelDocument } from "@/contracts";

import {
  buildStrategyPineFromVisualModel,
} from "../src/features/strategyVisualBuilderPine";
import {
  buildStrategyVisualModelFromPine,
} from "../src/features/strategyVisualBuilderPineParser";

describe("strategyVisualBuilderPine", () => {
  it("generates Pine-aligned quantity expressions and parses them back", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        {
          id: "on-kline-root",
          type: "circle",
          x: 120,
          y: 120,
          text: "K 线收盘",
          properties: { blockKind: "onKLineClosed" },
        },
        {
          id: "amount-order",
          type: "rect",
          x: 360,
          y: 120,
          text: "固定金额买入",
          properties: {
            blockKind: "placeOrder",
            side: "BUY",
            orderType: "LIMIT",
            quantityMode: "amount",
            quantityValue: 5000,
            limitPrice: 101.5,
          },
        },
        {
          id: "equity-order",
          type: "rect",
          x: 600,
          y: 120,
          text: "权益百分比开空",
          properties: {
            blockKind: "placeOrder",
            side: "SELL_SHORT",
            orderType: "MARKET",
            quantityMode: "equityPercent",
            quantityValue: 25,
          },
        },
      ],
      edges: [
        {
          id: "edge-root-amount",
          type: "polyline",
          sourceNodeId: "on-kline-root",
          targetNodeId: "amount-order",
        },
        {
          id: "edge-amount-equity",
          type: "polyline",
          sourceNodeId: "amount-order",
          targetNodeId: "equity-order",
        },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Sizing" });

    expect(script).toContain('strategy.entry("Long", strategy.long, qty=(5000 / close), limit=101.5)');
    expect(script).toContain('strategy.entry("Short", strategy.short, qty=((strategy.equity * 25 / 100) / close))');
    expect(script).not.toContain("qty_percent");

    const parsed = buildStrategyVisualModelFromPine(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const amountNode = parsed.model.nodes.find((node) => node.id === "amount-order");
    const equityNode = parsed.model.nodes.find((node) => node.id === "equity-order");
    expect(amountNode?.properties.quantityMode).toBe("amount");
    expect(amountNode?.properties.quantityValue).toBe(5000);
    expect(amountNode?.properties.orderType).toBe("LIMIT");
    expect(amountNode?.properties.limitPrice).toBe(101.5);
    expect(equityNode?.properties.quantityMode).toBe("equityPercent");
    expect(equityNode?.properties.quantityValue).toBe(25);
  });

  it("generates Pine timeframe moving averages and basic strategy exits", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        {
          id: "on-kline-root",
          type: "circle",
          x: 120,
          y: 120,
          text: "K 线收盘",
          properties: { blockKind: "onKLineClosed" },
        },
        {
          id: "daily-ma",
          type: "rect",
          x: 360,
          y: 120,
          text: "日线 EMA",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "movingAverage",
            movingAverageType: "EMA",
            windowSize: 5,
            periodUnit: "day",
          },
        },
        {
          id: "exit-node",
          type: "rect",
          x: 600,
          y: 120,
          text: "1柱止损",
          properties: {
            blockKind: "stopLoss",
            mode: "stopLoss",
            direction: "long",
            timeValue: 1,
            timeUnit: "bar",
            percentage: 2,
            windowPolicy: "continuous",
          },
        },
      ],
      edges: [
        {
          id: "edge-root-ma",
          type: "polyline",
          sourceNodeId: "on-kline-root",
          targetNodeId: "daily-ma",
        },
        {
          id: "edge-ma-exit",
          type: "polyline",
          sourceNodeId: "daily-ma",
          targetNodeId: "exit-node",
        },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "MA Exit" });

    expect(script).toContain('daily_ma = request.security(syminfo.tickerid, "D", ta.ema(close, 5))');
    expect(script).toContain('strategy.exit("Long stopLoss", "Long", stop=close * (1 - 2 / 100))');

    const parsed = buildStrategyVisualModelFromPine(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const maNode = parsed.model.nodes.find((node) => node.id === "daily-ma");
    const exitNode = parsed.model.nodes.find((node) => node.id === "exit-node");
    expect(maNode?.properties.periodUnit).toBe("day");
    expect(maNode?.properties.movingAverageType).toBe("EMA");
    expect(exitNode?.properties.mode).toBe("stopLoss");
    expect(exitNode?.properties.timeUnit).toBe("bar");
    expect(exitNode?.properties.percentage).toBe(2);
  });
});
