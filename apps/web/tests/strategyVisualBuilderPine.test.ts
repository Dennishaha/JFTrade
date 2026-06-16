import { describe, expect, it } from "vitest";

import type { StrategyVisualModelDocument } from "@/contracts";

import {
  createStrategyPaletteItems,
  getStrategyBlockCatalog,
} from "../src/features/strategyVisualBuilderCatalog";
import {
  assessPineBlockSupport,
  getVisualBlockCapabilities,
  getStrategyAuthoringTemplates,
  parsePineExpressionToVisualExpression,
  renderVisualExpressionToPine,
  summarizePineBlockSupport,
} from "../src/features/strategyVisualBuilder";
import {
  buildStrategyPineFromVisualModel,
} from "../src/features/strategyVisualBuilderPine";
import {
  buildStrategyVisualModelFromPine,
} from "../src/features/strategyVisualBuilderPineParser";

describe("strategyVisualBuilderPine", () => {
  it("renders and parses safe visual expression AST nodes", () => {
    const expression = {
      kind: "binary" as const,
      left: {
        kind: "history" as const,
        target: {
          kind: "field" as const,
          target: { kind: "reference" as const, name: "macd_fast" },
          field: "histogram",
        },
        offset: 1,
      },
      operator: ">" as const,
      right: { kind: "literal" as const, value: 0 },
    };

    const scriptExpression = renderVisualExpressionToPine(expression);

    expect(scriptExpression).toBe("macd_fast.histogram[1] > 0");
    expect(parsePineExpressionToVisualExpression(scriptExpression)).toMatchObject({
      kind: "binary",
      left: { kind: "history" },
      right: { kind: "literal", value: 0 },
    });
  });

  it("uses structured expression AST for series condition, derived series, and state update blocks", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        { id: "root", type: "circle", x: 120, y: 120, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
        {
          id: "series-condition",
          type: "diamond",
          x: 360,
          y: 120,
          text: "结构化条件",
          properties: {
            blockKind: "seriesCondition",
            mode: "compare",
            operator: ">",
            leftExpressionAst: { kind: "field", target: { kind: "reference", name: "trend" }, field: "histogram" },
            rightExpressionAst: { kind: "literal", value: 0 },
          },
        },
        {
          id: "derived",
          type: "rect",
          x: 620,
          y: 120,
          text: "派生 spread",
          properties: {
            blockKind: "derivedSeries",
            variableName: "spread",
            mode: "arithmetic",
            operator: "-",
            leftExpressionAst: { kind: "reference", name: "trend" },
            rightExpressionAst: { kind: "source", source: "close" },
          },
        },
        {
          id: "state-update",
          type: "rect",
          x: 880,
          y: 120,
          text: "更新 cooldown",
          properties: {
            blockKind: "stateUpdate",
            variableName: "cooldown",
            expressionAst: {
              kind: "call",
              functionName: "math.max",
              args: [
                { kind: "binary", left: { kind: "reference", name: "cooldown" }, operator: "-", right: { kind: "literal", value: 1 } },
                { kind: "literal", value: 0 },
              ],
            },
          },
        },
      ],
      edges: [
        { id: "edge-root-condition", type: "polyline", sourceNodeId: "root", targetNodeId: "series-condition" },
        { id: "edge-condition-derived", type: "polyline", sourceNodeId: "series-condition", targetNodeId: "derived", properties: { branch: "true" } },
        { id: "edge-derived-state", type: "polyline", sourceNodeId: "derived", targetNodeId: "state-update" },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Structured Expressions" });

    expect(script).toContain("if trend.histogram > 0");
    expect(script).toContain("spread = (trend - close)");
    expect(script).toContain("cooldown := math.max(cooldown - 1, 0)");

    const parsed = buildStrategyVisualModelFromPine(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }
    expect(parsed.model.nodes.find((node) => node.properties.blockKind === "seriesCondition")?.properties.leftExpressionAst).toMatchObject({ kind: "field" });
    expect(parsed.model.nodes.find((node) => node.properties.blockKind === "derivedSeries")?.properties.leftExpressionAst).toMatchObject({ kind: "reference" });
    expect(parsed.model.nodes.find((node) => node.properties.blockKind === "stateUpdate")?.properties.expressionAst).toMatchObject({ kind: "call", functionName: "math.max" });
  });

  it("round-trips structured order, exit, MTF, collection, time, and session blocks", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        { id: "root", type: "circle", x: 120, y: 120, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
        {
          id: "time-filter",
          type: "diamond",
          x: 360,
          y: 120,
          text: "时间过滤",
          properties: { blockKind: "timeFilter", mode: "between", startHour: 9, startMinute: 30, endHour: 16, endMinute: 0 },
        },
        {
          id: "session-filter",
          type: "diamond",
          x: 600,
          y: 120,
          text: "交易时段过滤",
          properties: { blockKind: "sessionFilter", scope: "market" },
        },
        {
          id: "mtf-trend",
          type: "rect",
          x: 840,
          y: 120,
          text: "MTF trend",
          properties: {
            blockKind: "mtfSeries",
            variableName: "daily_trend",
            timeframe: "D",
            expressionType: "indicator",
            indicatorExpression: "ta.ema(close, 20)",
            indicatorExpressionAst: {
              kind: "call",
              functionName: "ta.ema",
              args: [{ kind: "source", source: "close" }, { kind: "literal", value: 20 }],
            },
          },
        },
        {
          id: "collection-stat",
          type: "rect",
          x: 1080,
          y: 120,
          text: "集合统计",
          properties: {
            blockKind: "collectionStat",
            variableName: "range_avg",
            statFunction: "avg",
            sourceAExpressionAst: { kind: "source", source: "close" },
            sourceBExpressionAst: { kind: "history", target: { kind: "source", source: "high" }, offset: 1 },
            sourceCExpressionAst: { kind: "call", functionName: "math.max", args: [{ kind: "source", source: "open" }, { kind: "source", source: "low" }] },
          },
        },
        {
          id: "limit-order",
          type: "rect",
          x: 1320,
          y: 120,
          text: "表达式限价单",
          properties: {
            blockKind: "placeOrder",
            orderAction: "entry",
            orderId: "Long",
            side: "BUY",
            orderType: "LIMIT",
            quantityMode: "equityPercent",
            quantityValue: 10,
            limitPriceExpressionAst: { kind: "call", functionName: "math.max", args: [{ kind: "source", source: "close" }, { kind: "source", source: "open" }] },
            stopPriceExpressionAst: { kind: "history", target: { kind: "source", source: "low" }, offset: 1 },
          },
        },
        {
          id: "expr-exit",
          type: "rect",
          x: 1560,
          y: 120,
          text: "表达式退出",
          properties: {
            blockKind: "stopLoss",
            mode: "bracketExit",
            direction: "long",
            timeValue: 1,
            timeUnit: "bar",
            windowPolicy: "continuous",
            quantityPercentage: 50,
            stopPriceExpressionAst: { kind: "history", target: { kind: "source", source: "low" }, offset: 1 },
            takeProfitPriceExpressionAst: { kind: "history", target: { kind: "source", source: "high" }, offset: 1 },
          },
        },
      ],
      edges: [
        { id: "edge-root-time", type: "polyline", sourceNodeId: "root", targetNodeId: "time-filter" },
        { id: "edge-time-session", type: "polyline", sourceNodeId: "time-filter", targetNodeId: "session-filter", properties: { branch: "true" } },
        { id: "edge-session-mtf", type: "polyline", sourceNodeId: "session-filter", targetNodeId: "mtf-trend", properties: { branch: "true" } },
        { id: "edge-mtf-collection", type: "polyline", sourceNodeId: "mtf-trend", targetNodeId: "collection-stat" },
        { id: "edge-collection-order", type: "polyline", sourceNodeId: "collection-stat", targetNodeId: "limit-order" },
        { id: "edge-order-exit", type: "polyline", sourceNodeId: "limit-order", targetNodeId: "expr-exit" },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "VNext Structured" });

    expect(script).toContain("if (hour * 60 + minute) >= 570 and (hour * 60 + minute) < 960");
    expect(script).toContain("if session.ismarket");
    expect(script).toContain('daily_trend = request.security(syminfo.tickerid, "D", ta.ema(close, 20))');
    expect(script).toContain("range_avg = array.from(close, high[1], math.max(open, low)).avg()");
    expect(script).toContain('strategy.entry("Long", strategy.long, qty_percent=10, limit=math.max(close, open), stop=low[1])');
    expect(script).toContain('strategy.exit("Long bracketExit", "Long", stop=low[1], limit=high[1], qty_percent=50)');

    const parsed = buildStrategyVisualModelFromPine(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }
    expect(parsed.pineSnippetCount).toBe(0);
    expect(parsed.model.nodes.find((node) => node.properties.blockKind === "timeFilter")?.properties.mode).toBe("between");
    expect(parsed.model.nodes.find((node) => node.properties.blockKind === "sessionFilter")?.properties.scope).toBe("market");
    expect(parsed.model.nodes.find((node) => node.properties.blockKind === "mtfSeries")?.properties.indicatorExpressionAst).toMatchObject({ kind: "call", functionName: "ta.ema" });
    expect(parsed.model.nodes.find((node) => node.properties.blockKind === "collectionStat")?.properties.sourceBExpressionAst).toMatchObject({ kind: "history" });
    expect(parsed.model.nodes.find((node) => node.id === "limit-order")?.properties).toMatchObject({
      orderType: "LIMIT",
      limitPriceExpressionAst: { kind: "call", functionName: "math.max" },
      stopPriceExpressionAst: { kind: "history" },
    });
    expect(parsed.model.nodes.find((node) => node.id === "expr-exit")?.properties).toMatchObject({
      mode: "bracketExit",
      quantityPercentage: 50,
      stopPriceExpressionAst: { kind: "history" },
      takeProfitPriceExpressionAst: { kind: "history" },
    });
  });

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
    expect(script).toContain('strategy.entry("Short", strategy.short, qty_percent=25)');

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
    expect(parsed.pineSnippetCount).toBe(0);
  });

  it("generates and parses expanded Pine order actions", () => {
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
          id: "net-order",
          type: "rect",
          x: 360,
          y: 120,
          text: "净额挂单",
          properties: {
            blockKind: "placeOrder",
            orderAction: "order",
            orderId: "Breakout",
            side: "BUY",
            orderType: "LIMIT",
            quantityMode: "equityPercent",
            quantityValue: 10,
            limitPrice: 105,
            stopPrice: 102,
          },
        },
        {
          id: "risk-direction",
          type: "rect",
          x: 600,
          y: 120,
          text: "仅允许多头",
          properties: {
            blockKind: "placeOrder",
            orderAction: "riskAllowEntryIn",
            riskAllowedDirection: "long",
          },
        },
        {
          id: "cancel-order",
          type: "rect",
          x: 840,
          y: 120,
          text: "撤销挂单",
          properties: {
            blockKind: "placeOrder",
            orderAction: "cancel",
            orderId: "Breakout",
          },
        },
        {
          id: "close-all",
          type: "rect",
          x: 1080,
          y: 120,
          text: "全部平仓",
          properties: {
            blockKind: "placeOrder",
            orderAction: "closeAll",
          },
        },
      ],
      edges: [
        { id: "edge-root-net", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "net-order" },
        { id: "edge-net-risk", type: "polyline", sourceNodeId: "net-order", targetNodeId: "risk-direction" },
        { id: "edge-risk-cancel", type: "polyline", sourceNodeId: "risk-direction", targetNodeId: "cancel-order" },
        { id: "edge-cancel-close", type: "polyline", sourceNodeId: "cancel-order", targetNodeId: "close-all" },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Orders" });

    expect(script).toContain('strategy.order("Breakout", strategy.long, qty_percent=10, limit=105, stop=102)');
    expect(script).toContain("strategy.risk.allow_entry_in(strategy.direction.long)");
    expect(script).toContain('strategy.cancel("Breakout")');
    expect(script).toContain("strategy.close_all()");

    const parsed = buildStrategyVisualModelFromPine(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    expect(parsed.pineSnippetCount).toBe(0);
    expect(parsed.model.nodes.find((node) => node.id === "net-order")?.properties).toMatchObject({
      orderAction: "order",
      orderId: "Breakout",
      quantityMode: "equityPercent",
      quantityValue: 10,
      limitPrice: 105,
      stopPrice: 102,
    });
    expect(parsed.model.nodes.find((node) => node.id === "risk-direction")?.properties).toMatchObject({
      orderAction: "riskAllowEntryIn",
      riskAllowedDirection: "long",
    });
    expect(parsed.model.nodes.find((node) => node.id === "cancel-order")?.properties).toMatchObject({
      orderAction: "cancel",
      orderId: "Breakout",
    });
    expect(parsed.model.nodes.find((node) => node.id === "close-all")?.properties.orderAction).toBe("closeAll");
  });

  it("generates and parses series condition blocks", () => {
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
          id: "volume-filter",
          type: "diamond",
          x: 360,
          y: 120,
          text: "Volume > 1000000",
          properties: {
            blockKind: "seriesCondition",
            mode: "compare",
            source: "volume",
            operator: ">",
            threshold: 1000000,
          },
        },
        {
          id: "close-rising",
          type: "diamond",
          x: 600,
          y: 120,
          text: "Close rising",
          properties: {
            blockKind: "seriesCondition",
            mode: "rising",
            source: "close",
            length: 3,
          },
        },
        {
          id: "recent-breakout",
          type: "diamond",
          x: 840,
          y: 120,
          text: "Recent breakout",
          properties: {
            blockKind: "seriesCondition",
            mode: "barssince",
            eventSource: "close",
            eventOperator: ">",
            eventThreshold: 520,
            length: 5,
          },
        },
        {
          id: "last-breakout-close",
          type: "diamond",
          x: 1080,
          y: 120,
          text: "Last breakout close",
          properties: {
            blockKind: "seriesCondition",
            mode: "valuewhen",
            eventSource: "close",
            eventOperator: ">",
            eventThreshold: 520,
            valueSource: "close",
            occurrence: 0,
            operator: ">",
            threshold: 500,
          },
        },
      ],
      edges: [
        { id: "edge-root-volume", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "volume-filter" },
        { id: "edge-volume-rising", type: "polyline", sourceNodeId: "volume-filter", targetNodeId: "close-rising", properties: { branch: "true" } },
        { id: "edge-rising-bars", type: "polyline", sourceNodeId: "close-rising", targetNodeId: "recent-breakout", properties: { branch: "true" } },
        { id: "edge-bars-value", type: "polyline", sourceNodeId: "recent-breakout", targetNodeId: "last-breakout-close", properties: { branch: "true" } },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Series Conditions" });
    expect(script).toContain("if volume > 1000000");
    expect(script).toContain("if rising(close, 3)");
    expect(script).toContain("if barssince(close > 520) < 5");
    expect(script).toContain("if valuewhen(close > 520, close, 0) > 500");

    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Series", overlay=true)
if volume > 1000000
    if rising(close, 3)
        if barssince(close > 520) < 5
            if valuewhen(close > 520, close, 0) > 500
                strategy.entry("Long", strategy.long, qty=1)
`);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }
    expect(parsed.pineSnippetCount).toBe(0);
    const seriesNodes = parsed.model.nodes.filter((node) => node.properties.blockKind === "seriesCondition");
    expect(seriesNodes.map((node) => node.properties.mode)).toEqual([
      "compare",
      "rising",
      "barssince",
      "valuewhen",
    ]);
  });

  it("generates and parses strategy input, derived series, MTF series, and state blocks", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        { id: "length-input", type: "rect", x: 120, y: 80, text: "参数 length = 20", properties: { blockKind: "strategyInput", variableName: "length", inputType: "int", title: "Length", defaultValue: 20 } },
        { id: "on-kline-root", type: "circle", x: 120, y: 180, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
        { id: "previous-close", type: "rect", x: 360, y: 180, text: "派生 prev_close", properties: { blockKind: "derivedSeries", variableName: "prev_close", mode: "history", source: "close", historyOffset: 1 } },
        { id: "mtf-close", type: "rect", x: 600, y: 180, text: "MTF close", properties: { blockKind: "mtfSeries", variableName: "daily_close", timeframe: "D", expressionType: "history", source: "close", historyOffset: 1 } },
        { id: "mtf-trend", type: "rect", x: 600, y: 280, text: "MTF trend", properties: { blockKind: "mtfSeries", variableName: "daily_trend", timeframe: "D", expressionType: "indicator", indicatorExpression: "supertrend(3, 10)", mtfField: "direction" } },
        { id: "range-stat", type: "rect", x: 720, y: 280, text: "集合统计", properties: { blockKind: "collectionStat", variableName: "range_median", statFunction: "median", sourceA: "close", sourceB: "open", sourceC: "high" } },
        { id: "armed-state", type: "rect", x: 840, y: 180, text: "状态 armed", properties: { blockKind: "stateVariable", variableName: "armed", valueType: "bool", initialValue: false } },
        { id: "armed-update", type: "rect", x: 1080, y: 180, text: "更新 armed", properties: { blockKind: "stateUpdate", variableName: "armed", expression: "close > prev_close" } },
      ],
      edges: [
        { id: "edge-root-derived", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "previous-close" },
        { id: "edge-derived-mtf", type: "polyline", sourceNodeId: "previous-close", targetNodeId: "mtf-close" },
        { id: "edge-mtf-trend", type: "polyline", sourceNodeId: "mtf-close", targetNodeId: "mtf-trend" },
        { id: "edge-trend-stat", type: "polyline", sourceNodeId: "mtf-trend", targetNodeId: "range-stat" },
        { id: "edge-stat-state", type: "polyline", sourceNodeId: "range-stat", targetNodeId: "armed-state" },
        { id: "edge-state-update", type: "polyline", sourceNodeId: "armed-state", targetNodeId: "armed-update" },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Inputs and State" });

    expect(script).toContain('length = input.int(20, "Length")');
    expect(script).toContain("prev_close = close[1]");
    expect(script).toContain('daily_close = request.security(syminfo.tickerid, "D", close[1])');
    expect(script).toContain('daily_trend = request.security(syminfo.tickerid, "D", supertrend(3, 10).direction)');
    expect(script).toContain("range_median = array.from(close, open, high).median()");
    expect(script).toContain("var armed = false");
    expect(script).toContain("armed := close > prev_close");

    const parsed = buildStrategyVisualModelFromPine(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }
    expect(parsed.pineSnippetCount).toBe(0);
    expect(parsed.model.nodes.some((node) => node.properties.blockKind === "strategyInput")).toBe(true);
    expect(parsed.model.nodes.some((node) => node.properties.blockKind === "derivedSeries")).toBe(true);
    expect(parsed.model.nodes.some((node) => node.properties.blockKind === "mtfSeries")).toBe(true);
    expect(parsed.model.nodes.some((node) => node.properties.blockKind === "collectionStat")).toBe(true);
    expect(parsed.model.nodes.some((node) => node.properties.blockKind === "stateVariable")).toBe(true);
    expect(parsed.model.nodes.some((node) => node.properties.blockKind === "stateUpdate")).toBe(true);
  });

  it("generates and parses partial close and partial exit quantity percentages", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        { id: "on-kline-root", type: "circle", x: 120, y: 120, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
        { id: "partial-close", type: "rect", x: 360, y: 120, text: "部分平仓", properties: { blockKind: "placeOrder", orderAction: "close", orderId: "Long", side: "SELL", quantityMode: "equityPercent", quantityValue: 50 } },
        { id: "partial-exit", type: "rect", x: 600, y: 120, text: "部分止盈止损", properties: { blockKind: "stopLoss", mode: "bracketExit", direction: "long", timeValue: 1, timeUnit: "bar", percentage: 2, takeProfitPercentage: 4, quantityPercentage: 50, windowPolicy: "continuous" } },
      ],
      edges: [
        { id: "edge-root-close", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "partial-close" },
        { id: "edge-close-exit", type: "polyline", sourceNodeId: "partial-close", targetNodeId: "partial-exit" },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Partial Exit" });

    expect(script).toContain('strategy.close("Long", qty_percent=50)');
    expect(script).toContain("qty_percent=50");

    const parsed = buildStrategyVisualModelFromPine(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }
    const closeNode = parsed.model.nodes.find((node) => node.properties.orderAction === "close");
    const exitNode = parsed.model.nodes.find((node) => node.properties.blockKind === "stopLoss");
    expect(closeNode?.properties).toMatchObject({ quantityMode: "equityPercent", quantityValue: 50 });
    expect(exitNode?.properties).toMatchObject({ mode: "bracketExit", quantityPercentage: 50 });
  });

  it("keeps unsupported collection and visual Pine as typed snippet nodes", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Snippet Types", overlay=true)
values = array.from(close, open)
plot(close)
`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }
    const snippets = parsed.model.nodes.filter((node) => node.properties.blockKind === "pineSnippet");
    expect(snippets.map((node) => node.properties.snippetSource)).toEqual([
      "advancedCollection",
      "visualOnly",
    ]);
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
    expect(snippet?.properties.snippetSource).toBe("visualOnly");
    expect(parsed.model.nodes.some((node) => node.properties?.blockKind === "codeBlock")).toBe(false);
    expect(parsed.pineSnippetCount).toBe(1);

    const script = buildStrategyPineFromVisualModel(parsed.model, { name: "Snippet" });
    expect(script).toContain("plot(close)");
    expect(script).not.toContain("代码块已废弃");
  });

  it("rejects old codeBlock Pine annotations instead of converting them", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Legacy Annotation", overlay=true)
// @jftradeFlowNodeId legacy-code
// @jftradeFlowBlockKind codeBlock
// @jftradeFlowNodeText 旧代码块
plot(close)
`);

    expect(parsed.ok).toBe(false);
    expect(parsed.error).toContain("旧 codeBlock / technicalIndicator");
  });

  it("omits legacy codeBlock visual models when generating Pine", () => {
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
    expect(script).not.toContain("@jftradeFlowBlockKind codeBlock");
    expect(script).not.toContain("console.log('legacy')");
  });

  it("omits legacy unified technicalIndicator visual models when generating Pine", () => {
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
          id: "legacy-rsi",
          type: "rect",
          x: 360,
          y: 120,
          text: "RSI < 30",
          properties: {
            blockKind: "technicalIndicator",
            indicatorType: "rsi",
            conditionMode: "numeric",
            operator: "<",
            threshold: 30,
            period: 14,
          },
        },
      ],
      edges: [
        {
          id: "edge-root-rsi",
          type: "polyline",
          sourceNodeId: "on-kline-root",
          targetNodeId: "legacy-rsi",
        },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Legacy Indicator" });
    expect(script).not.toContain("legacy_rsi = ta.rsi");
    expect(script).not.toContain("if legacy_rsi < 30");
    expect(script).not.toMatch(/@jftradeFlowBlockKind technicalIndicator\s*$/m);
  });

  it("does not expose legacy codeBlock or unified technicalIndicator in new palette paths", () => {
    const paletteKinds = createStrategyPaletteItems().map((item) => item.properties.blockKind);
    expect(paletteKinds).not.toContain("codeBlock");
    expect(paletteKinds).not.toContain("technicalIndicator");
  });

  it("rejects old technicalIndicator annotations instead of migrating them", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Legacy Indicator Annotation", overlay=true)
// @jftradeFlowNodeId old-rsi
// @jftradeFlowBlockKind technicalIndicator
// @jftradeFlowNodeText RSI
rsiValue = ta.rsi(close, 14)
// @jftradeFlowNodeId old-rsi-condition
// @jftradeFlowBlockKind technicalIndicator
// @jftradeFlowNodeText RSI < 30
if rsiValue < 30
    alert("buy")
`);

    expect(parsed.ok).toBe(false);
    expect(parsed.error).toContain("旧 codeBlock / technicalIndicator");
  });

  it("keeps built-in visual templates on standard Pine visual blocks", () => {
    for (const template of getStrategyAuthoringTemplates().filter((item) => item.mode === "visual")) {
      const blockKinds = template.visualModel.nodes.map((node) => node.properties.blockKind);
      expect(blockKinds, template.id).not.toContain("codeBlock");
      expect(blockKinds, template.id).not.toContain("technicalIndicator");
    }
  });

  it("registers Pine capabilities for every current visual block kind", () => {
    const catalogKinds = new Set(getStrategyBlockCatalog().map((block) => block.kind));
    const capabilities = getVisualBlockCapabilities();

    expect(new Set(capabilities.map((capability) => capability.kind))).toEqual(catalogKinds);
    for (const capability of capabilities) {
      expect(capability.controlSchema.controlIds.length, capability.kind).toBeGreaterThan(0);
      expect(capability.pineRenderRule.description, capability.kind).not.toBe("");
      expect(capability.pineParseRule.description, capability.kind).not.toBe("");
    }
  });

  it("round-trips all visual templates without runtime guards or unexpected snippets", () => {
    for (const template of getStrategyAuthoringTemplates().filter((item) => item.mode === "visual")) {
      const script = template.buildScript({ name: template.label });
      expect(script, template.id).not.toContain("runtime.error");

      const parsed = buildStrategyVisualModelFromPine(script);
      expect(parsed.ok, template.id).toBe(true);
      if (!parsed.ok) {
        continue;
      }
      const snippetNodes = parsed.model.nodes.filter((node) => node.properties.blockKind === "pineSnippet");
      expect(snippetNodes, template.id).toHaveLength(0);
    }
  });

  it("generates expanded visual templates with supported Pine indicator blocks", () => {
    const templates = new Map(
      getStrategyAuthoringTemplates().map((template) => [template.id, template]),
    );
    const mfiTemplate = templates.get("mfi-reversion");
    const mtfTemplate = templates.get("mtf-momentum");
    const supertrendTemplate = templates.get("supertrend-follow");
    const bracketTemplate = templates.get("bracket-exit-risk");

    expect(mfiTemplate).toBeDefined();
    expect(mtfTemplate).toBeDefined();
    expect(supertrendTemplate).toBeDefined();
    expect(bracketTemplate).toBeDefined();

    const mfiScript = mfiTemplate?.buildScript({ name: "MFI Template" }) ?? "";
    const mtfScript = mtfTemplate?.buildScript({ name: "MTF Template" }) ?? "";
    const supertrendScript = supertrendTemplate?.buildScript({ name: "Supertrend Template" }) ?? "";
    const bracketScript = bracketTemplate?.buildScript({ name: "Bracket Template" }) ?? "";

    expect(mfiScript).toContain("mfi_getter = mfi(hlc3, 14)");
    expect(mtfScript).toContain('mtf_ema = request.security(syminfo.tickerid, "D", ta.ema(close, 20))');
    expect(supertrendScript).toContain("supertrend_getter = supertrend(3, 10)");
    expect(supertrendScript).toContain("if supertrend_getter.direction > 0");
    expect(bracketScript).toContain('strategy.exit("Long bracketExit", "Long", stop=close * (1 - 2 / 100), limit=close * (1 + 4 / 100))');
    expect(`${mfiScript}\n${mtfScript}\n${supertrendScript}\n${bracketScript}`).not.toContain("runtime.error");
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

  it("generates and parses bracket exit visual risk blocks", () => {
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
          id: "bracket-exit",
          type: "rect",
          x: 360,
          y: 120,
          text: "止盈止损",
          properties: {
            blockKind: "stopLoss",
            mode: "bracketExit",
            direction: "long",
            timeValue: 1,
            timeUnit: "bar",
            percentage: 2,
            takeProfitPercentage: 4,
            windowPolicy: "continuous",
          },
        },
      ],
      edges: [
        { id: "edge-root-bracket", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "bracket-exit" },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Bracket Exit" });
    expect(script).toContain('strategy.exit("Long bracketExit", "Long", stop=close * (1 - 2 / 100), limit=close * (1 + 4 / 100))');
    expect(script).not.toContain("runtime.error");

    const parsed = buildStrategyVisualModelFromPine(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }
    expect(parsed.model.nodes.find((node) => node.id === "bracket-exit")?.properties).toMatchObject({
      blockKind: "stopLoss",
      mode: "bracketExit",
      direction: "long",
      percentage: 2,
      takeProfitPercentage: 4,
      timeUnit: "bar",
      windowPolicy: "continuous",
    });
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

  it("generates Williams %R runtime Pine instead of RSI", () => {
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
    expect(script).toContain("williams = williams_r(14)");
    expect(script).not.toContain("williams = ta.rsi");
  });

  it("generates runtime-aligned KDJ and Bollinger expressions and parses them back", () => {
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
          id: "kdj-node",
          type: "rect",
          x: 360,
          y: 120,
          text: "KDJ",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "kdj",
            period: 9,
            m1: 3,
            m2: 3,
          },
        },
        {
          id: "boll-node",
          type: "rect",
          x: 600,
          y: 120,
          text: "布林带",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "bollinger",
            period: 20,
            multiplier: 2,
          },
        },
      ],
      edges: [
        {
          id: "edge-root-kdj",
          type: "polyline",
          sourceNodeId: "on-kline-root",
          targetNodeId: "kdj-node",
        },
        {
          id: "edge-kdj-boll",
          type: "polyline",
          sourceNodeId: "kdj-node",
          targetNodeId: "boll-node",
        },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Runtime Indicators" });
    expect(script).toContain("kdj_node = kdj(9, 3, 3)");
    expect(script).toContain("boll_node = bollinger(20, 2)");
    expect(script).not.toContain("kdj_node = ta.rsi");
    expect(script).not.toContain("boll_node = ta.sma");

    const parsed = buildStrategyVisualModelFromPine(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }
    expect(parsed.pineSnippetCount).toBe(0);
    expect(parsed.model.nodes.find((node) => node.id === "kdj-node")?.properties).toMatchObject({
      indicatorType: "kdj",
      period: 9,
      m1: 3,
      m2: 3,
    });
    expect(parsed.model.nodes.find((node) => node.id === "boll-node")?.properties).toMatchObject({
      indicatorType: "bollinger",
      period: 20,
      multiplier: 2,
    });
  });

  it("parses internal Pine indicator expressions into visual blocks", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Internal Indicators", overlay=true)
signal = macd(12, 26, 9)
flow = kdj(9, 3, 3)
band = bollinger(20, 2)
wr = williams_r(14)
`);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const byVariable = new Map(
      parsed.model.nodes.map((node) => [node.properties.variableName, node]),
    );
    expect(byVariable.get("signal")?.properties.indicatorType).toBe("macd");
    expect(byVariable.get("flow")?.properties.indicatorType).toBe("kdj");
    expect(byVariable.get("band")?.properties.indicatorType).toBe("bollinger");
    expect(byVariable.get("wr")?.properties.indicatorType).toBe("williamsR");
    expect(parsed.pineSnippetCount).toBe(0);
  });

  it("generates and parses the next Pine-supported indicator batch", () => {
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
          id: "std-dev",
          type: "rect",
          x: 360,
          y: 120,
          text: "标准差",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "stdev",
            source: "close",
            period: 20,
          },
        },
        {
          id: "mfi-node",
          type: "rect",
          x: 600,
          y: 120,
          text: "MFI",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "mfi",
            source: "hlc3",
            period: 14,
          },
        },
        {
          id: "dmi-node",
          type: "rect",
          x: 840,
          y: 120,
          text: "DMI",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "dmi",
            period: 14,
            adxSmoothing: 14,
          },
        },
        {
          id: "trend-node",
          type: "rect",
          x: 1080,
          y: 120,
          text: "Supertrend",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "supertrend",
            factor: 3,
            period: 10,
          },
        },
        {
          id: "kc-node",
          type: "rect",
          x: 1320,
          y: 120,
          text: "Keltner",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "keltner",
            source: "close",
            period: 20,
            multiplier: 1.5,
          },
        },
        {
          id: "alma-node",
          type: "rect",
          x: 1560,
          y: 120,
          text: "ALMA",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "alma",
            source: "close",
            period: 20,
            offset: 0.85,
            sigma: 6,
          },
        },
      ],
      edges: [
        { id: "edge-root-std", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "std-dev" },
        { id: "edge-std-mfi", type: "polyline", sourceNodeId: "std-dev", targetNodeId: "mfi-node" },
        { id: "edge-mfi-dmi", type: "polyline", sourceNodeId: "mfi-node", targetNodeId: "dmi-node" },
        { id: "edge-dmi-trend", type: "polyline", sourceNodeId: "dmi-node", targetNodeId: "trend-node" },
        { id: "edge-trend-kc", type: "polyline", sourceNodeId: "trend-node", targetNodeId: "kc-node" },
        { id: "edge-kc-alma", type: "polyline", sourceNodeId: "kc-node", targetNodeId: "alma-node" },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Expanded Indicators" });

    expect(script).toContain("std_dev = stdev(close, 20)");
    expect(script).toContain("mfi_node = mfi(hlc3, 14)");
    expect(script).toContain("dmi_node = dmi(14, 14)");
    expect(script).toContain("trend_node = supertrend(3, 10)");
    expect(script).toContain("kc_node = kc(close, 20, 1.5, true)");
    expect(script).toContain("alma_node = alma(close, 20, 0.85, 6)");

    const parsed = buildStrategyVisualModelFromPine(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    expect(parsed.pineSnippetCount).toBe(0);
    expect(parsed.model.nodes.find((node) => node.id === "std-dev")?.properties).toMatchObject({
      indicatorType: "stdev",
      source: "close",
      period: 20,
    });
    expect(parsed.model.nodes.find((node) => node.id === "mfi-node")?.properties).toMatchObject({
      indicatorType: "mfi",
      source: "hlc3",
      period: 14,
    });
    expect(parsed.model.nodes.find((node) => node.id === "dmi-node")?.properties).toMatchObject({
      indicatorType: "dmi",
      period: 14,
      adxSmoothing: 14,
    });
    expect(parsed.model.nodes.find((node) => node.id === "trend-node")?.properties).toMatchObject({
      indicatorType: "supertrend",
      factor: 3,
      period: 10,
    });
    expect(parsed.model.nodes.find((node) => node.id === "kc-node")?.properties).toMatchObject({
      indicatorType: "keltner",
      period: 20,
      multiplier: 1.5,
    });
    expect(parsed.model.nodes.find((node) => node.id === "alma-node")?.properties).toMatchObject({
      indicatorType: "alma",
      offset: 0.85,
      sigma: 6,
    });
  });

  it("parses object-field numeric indicator conditions", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Object Conditions", overlay=true)
trend = supertrend(3, 10)
adx = dmi(14, 14)
if trend.direction > 0
    if adx.adx > 25
        strategy.entry("Long", strategy.long, qty=1)
`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const conditions = parsed.model.nodes.filter(
      (node) => node.properties.blockKind === "technicalIndicatorCondition",
    );
    expect(conditions).toHaveLength(2);
    expect(conditions[0]?.properties).toMatchObject({
      indicatorType: "supertrend",
      conditionMode: "numeric",
      operator: ">",
      threshold: 0,
    });
    expect(conditions[1]?.properties).toMatchObject({
      indicatorType: "dmi",
      conditionMode: "numeric",
      operator: ">",
      threshold: 25,
    });
    expect(parsed.pineSnippetCount).toBe(0);
  });

  it("reports Pine block support without persisting support state", () => {
    const supportedStop = {
      id: "supported-stop",
      type: "rect",
      x: 0,
      y: 0,
      text: "自动止损 1柱 2%",
      properties: {
        blockKind: "stopLoss",
        mode: "stopLoss",
        direction: "auto",
        timeValue: 1,
        timeUnit: "bar",
        percentage: 2,
        windowPolicy: "continuous",
      },
    };
    const unsupportedStop = {
      ...supportedStop,
      id: "unsupported-stop",
      properties: {
        ...supportedStop.properties,
        timeUnit: "day",
      },
    };
    const snippet = {
      ...supportedStop,
      id: "snippet",
      text: "Pine 片段",
      properties: {
        blockKind: "pineSnippet",
        code: "plot(close)",
      },
    };
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [supportedStop, unsupportedStop, snippet],
      edges: [],
    };

    expect(assessPineBlockSupport(supportedStop).status).toBe("supported");
    expect(assessPineBlockSupport(unsupportedStop).status).toBe("unsupportedConfig");
    expect(assessPineBlockSupport(snippet).status).toBe("snippetOnly");
    expect(summarizePineBlockSupport(model)).toMatchObject({
      unsupportedConfigCount: 1,
      snippetOnlyCount: 1,
    });
    expect(model.nodes.some((node) => "pineSupport" in node.properties)).toBe(false);
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
