import { describe, expect, it } from "vitest";

import type { StrategyVisualModelDocument } from "@/contracts";

import { createStrategyPaletteItems } from "../src/features/strategyVisualBuilderCatalog";
import {
  expandTechnicalIndicatorShortcutNode,
  STRATEGY_TECHNICAL_INDICATOR_SHORTCUT_CREATION_MODE,
} from "../src/features/strategyVisualBuilderIndicatorShortcut";
import {
  getStrategyAuthoringTemplates,
} from "../src/features/strategyVisualBuilder";
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

  it("parses Pine qty_percent, strategy.order, and close_all order forms", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Order Compatibility", overlay=true)
strategy.entry("Long", strategy.long, qty_percent=10)
strategy.order("Net", strategy.short, qty=5)
strategy.close("Long", qty_percent=50)
strategy.close_all()`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const orderNodes = parsed.model.nodes.filter((node) => node.properties?.blockKind === "placeOrder");
    expect(orderNodes).toHaveLength(4);
    expect(orderNodes[0]?.properties.quantityMode).toBe("equityPercent");
    expect(orderNodes[0]?.properties.quantityValue).toBe(10);
    expect(orderNodes[1]?.properties.side).toBe("SELL");
    expect(orderNodes[1]?.properties.quantityMode).toBe("shares");
    expect(orderNodes[1]?.properties.quantityValue).toBe(5);
    expect(orderNodes[2]?.properties.quantityMode).toBe("equityPercent");
    expect(orderNodes[2]?.properties.quantityValue).toBe(50);
    expect(orderNodes[3]?.properties.pineOrderFunction).toBe("strategy.close_all");
    expect(parsed.codeBlockCount).toBe(0);
  });

  it("keeps unsupported Pine lines as Pine snippet nodes", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Snippet", overlay=true)
plot(close)
`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const snippet = parsed.model.nodes.find((node) => node.properties?.blockKind === "pineSnippet");
    expect(snippet?.properties.code).toBe("plot(close)");
    expect(parsed.model.nodes.some((node) => node.properties?.blockKind === "codeBlock")).toBe(false);
    expect(parsed.codeBlockCount).toBe(1);

    const script = buildStrategyPineFromVisualModel(parsed.model, { name: "Snippet" });
    expect(script).toContain("plot(close)");
    expect(script).not.toContain("代码块已废弃");
  });

  it("converts old codeBlock Pine annotations to Pine snippet fallbacks", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Legacy Annotation", overlay=true)
// @jftradeFlowNodeId legacy-code
// @jftradeFlowBlockKind codeBlock
// @jftradeFlowNodeText 旧代码块
plot(close)
`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const node = parsed.model.nodes.find((item) => item.id === "legacy-code");
    expect(node?.properties.blockKind).toBe("pineSnippet");
    expect(node?.properties.code).toBe("plot(close)");
    expect(parsed.model.nodes.some((item) => item.properties?.blockKind === "codeBlock")).toBe(false);
    expect(parsed.codeBlockCount).toBe(1);
  });

  it("keeps legacy codeBlock visual models read-only instead of writing custom code", () => {
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
          id: "legacy-code",
          type: "rect",
          x: 360,
          y: 120,
          text: "旧代码块",
          properties: {
            blockKind: "codeBlock",
            code: "console.log('legacy')",
          },
        },
      ],
      edges: [
        {
          id: "edge-root-legacy",
          type: "polyline",
          sourceNodeId: "on-kline-root",
          targetNodeId: "legacy-code",
        },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Legacy Code" });
    expect(script).toContain("代码块已废弃，请改用标准 Pine 图块");
    expect(script).not.toContain("console.log('legacy')");
  });

  it("does not expose legacy codeBlock or unified technicalIndicator in new palette paths", () => {
    const paletteKinds = createStrategyPaletteItems().map((item) => item.properties.blockKind);
    expect(paletteKinds).not.toContain("codeBlock");
    expect(paletteKinds).not.toContain("technicalIndicator");

    const expansion = expandTechnicalIndicatorShortcutNode({
      id: "rsi-shortcut",
      x: 120,
      y: 120,
      properties: {
        blockKind: "technicalIndicator",
        creationMode: STRATEGY_TECHNICAL_INDICATOR_SHORTCUT_CREATION_MODE,
        indicatorType: "rsi",
        conditionMode: "numeric",
        operator: "<",
        threshold: 30,
        period: 14,
      },
    });
    expect(expansion.nodes.map((node) => node.properties.blockKind)).toEqual([
      "getTechnicalIndicator",
      "technicalIndicatorCondition",
    ]);
  });

  it("keeps built-in visual templates on standard Pine visual blocks", () => {
    for (const template of getStrategyAuthoringTemplates().filter((item) => item.mode === "visual")) {
      const blockKinds = template.visualModel.nodes.map((node) => node.properties.blockKind);
      expect(blockKinds, template.id).not.toContain("codeBlock");
      expect(blockKinds, template.id).not.toContain("technicalIndicator");
    }
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

  it("parses TradingView Bollinger and Williams %R aliases", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Aliases", overlay=true)
basis = ta.bb(close, 20, 2)
wr = ta.wpr(14)
`);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const bollingerNode = parsed.model.nodes.find((node) => node.properties.variableName === "basis");
    const williamsNode = parsed.model.nodes.find((node) => node.properties.variableName === "wr");
    expect(bollingerNode?.properties.indicatorType).toBe("bollinger");
    expect(bollingerNode?.properties.period).toBe(20);
    expect(bollingerNode?.properties.multiplier).toBe(2);
    expect(williamsNode?.properties.indicatorType).toBe("williamsR");
    expect(williamsNode?.properties.period).toBe(14);
  });

  it("generates Williams %R Pine instead of RSI", () => {
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
          id: "williams",
          type: "rect",
          x: 360,
          y: 120,
          text: "Williams %R",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "williamsR",
            period: 14,
          },
        },
      ],
      edges: [
        {
          id: "edge-root-williams",
          type: "polyline",
          sourceNodeId: "on-kline-root",
          targetNodeId: "williams",
        },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "WPR" });
    expect(script).toContain("williams = ta.wpr(14)");
    expect(script).not.toContain("williams = ta.rsi");
  });

  it("preserves source-aware moving averages", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Volume MA", overlay=true)
avgVol = ta.sma(volume, 20)
`);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const maNode = parsed.model.nodes.find((node) => node.properties.variableName === "avgVol");
    expect(maNode?.properties.indicatorType).toBe("movingAverage");
    expect(maNode?.properties.movingAverageType).toBe("SMA");
    expect(maNode?.properties.windowSize).toBe(20);
    expect(maNode?.properties.source).toBe("volume");

    const script = buildStrategyPineFromVisualModel(parsed.model, { name: "Volume MA" });
    expect(script).toContain("avgVol = ta.sma(volume, 20)");
  });
});
