import { describe, expect, it } from "vitest";

import type { StrategyVisualModelDocument } from "@/types";

import {
  createStrategyPaletteItems,
  getStrategyBlockCatalog,
} from "@/features/strategy-builder";
import {
  assessPineBlockSupport,
  getVisualBlockCapabilities,
  getStrategyAuthoringTemplates,
  parsePineExpressionToVisualExpression,
  renderVisualExpressionToPine,
  summarizePineBlockSupport,
} from "@/features/strategy-builder";
import {
  buildStrategyPineFromVisualModel,
} from "@/features/strategy-builder";
import {
  buildStrategyVisualModelFromPine,
} from "@/features/strategy-builder";

function createLinearVisualModel(
  nodes: Array<{
    id: string;
    text: string;
    properties: Record<string, unknown>;
    type?: "rect" | "diamond";
  }>,
): StrategyVisualModelDocument {
  const rootId = "on-kline-root";
  const visualNodes = [
    {
      id: rootId,
      type: "circle",
      x: 120,
      y: 120,
      text: "K 线收盘",
      properties: { blockKind: "onKLineClosed" },
    },
    ...nodes.map((node, index) => ({
      id: node.id,
      type: node.type ?? "rect",
      x: 360 + index * 240,
      y: 120,
      text: node.text,
      properties: node.properties,
    })),
  ];
  return {
    engine: "logic-flow",
    version: 1,
    nodes: visualNodes,
    edges: nodes.map((node, index) => ({
      id: `edge-${index}`,
      type: "polyline",
      sourceNodeId: index === 0 ? rootId : nodes[index - 1].id,
      targetNodeId: node.id,
    })),
  };
}

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
  });

  it("parses positional order quantity and close metadata supported by the backend", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Order Metadata", overlay=true)
strategy.entry("Long", strategy.long, 5, comment="breakout", alert_message="go", disable_alert=true, when=close > open)
strategy.close("Long", 2, stop=low[1], comment="trim", alert_message="scale", immediately=true, disable_alert=false, when=bar_index > 10)
strategy.close_all(true, "flatten", "done", false)`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const orderNodes = parsed.model.nodes.filter((node) => node.properties?.blockKind === "placeOrder");
    expect(orderNodes).toHaveLength(3);
    expect(orderNodes[0]?.properties).toMatchObject({
      orderAction: "entry",
      quantityMode: "shares",
      quantityValue: 5,
      comment: "breakout",
      alert_message: "go",
      disable_alert: true,
      when: "close > open",
    });
    expect(orderNodes[1]?.properties).toMatchObject({
      orderAction: "close",
      quantityMode: "shares",
      quantityValue: 2,
      comment: "trim",
      alert_message: "scale",
      immediately: true,
      disable_alert: false,
      when: "bar_index > 10",
    });
    expect(orderNodes[1]?.properties.stopPriceExpressionAst).toMatchObject({ kind: "history" });
    expect(orderNodes[2]?.properties).toMatchObject({
      orderAction: "closeAll",
      immediately: true,
      comment: "flatten",
      alert_message: "done",
      disable_alert: false,
    });
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

  it("renders close stop metadata and close_all metadata in legacy visual builder", () => {
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
          id: "entry-order",
          type: "rect",
          x: 360,
          y: 120,
          text: "注释入场",
          properties: {
            blockKind: "placeOrder",
            orderAction: "entry",
            orderId: "Long",
            side: "BUY",
            orderType: "MARKET",
            quantityMode: "shares",
            quantityValue: 5,
            comment: "breakout",
            alert_message: "go",
            disable_alert: true,
            when: "close > open",
          },
        },
        {
          id: "close-order",
          type: "rect",
          x: 600,
          y: 120,
          text: "带 stop 平仓",
          properties: {
            blockKind: "placeOrder",
            orderAction: "close",
            orderId: "Long",
            side: "SELL",
            quantityMode: "shares",
            quantityValue: 2,
            stopPriceExpressionAst: { kind: "history", target: { kind: "source", source: "low" }, offset: 1 },
            comment: "trim",
            alert_message: "scale",
            immediately: true,
            disable_alert: false,
            when: "bar_index > 10",
          },
        },
        {
          id: "close-all",
          type: "rect",
          x: 840,
          y: 120,
          text: "全部平仓",
          properties: {
            blockKind: "placeOrder",
            orderAction: "closeAll",
            immediately: true,
            comment: "flatten",
            alert_message: "done",
            disable_alert: false,
          },
        },
      ],
      edges: [
        { id: "edge-root-entry", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "entry-order" },
        { id: "edge-entry-close", type: "polyline", sourceNodeId: "entry-order", targetNodeId: "close-order" },
        { id: "edge-close-closeall", type: "polyline", sourceNodeId: "close-order", targetNodeId: "close-all" },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Legacy Order Metadata" });

    expect(script).toContain('strategy.entry("Long", strategy.long, qty=5, comment="breakout", alert_message="go", disable_alert=true, when=close > open)');
    expect(script).toContain('strategy.close("Long", qty=2, stop=low[1], comment="trim", alert_message="scale", immediately=true, disable_alert=false, when=bar_index > 10)');
    expect(script).toContain('strategy.close_all(immediately=true, comment="flatten", alert_message="done", disable_alert=false)');
  });

  it("parses bare strategy.risk max rules into legacy visual risk blocks", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Risk Rules", overlay=true)
strategy.risk.max_drawdown(10, strategy.percent_of_equity, alert_message="dd")
strategy.risk.max_intraday_loss(500, strategy.cash, "day")
strategy.risk.max_intraday_filled_orders(3, alert_message="fills")
strategy.risk.max_position_size(12)
strategy.risk.max_cons_loss_days(2, "days")`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const riskNodes = parsed.model.nodes.filter((node) => node.properties?.blockKind === "riskRule");
    expect(riskNodes).toHaveLength(5);
    [
      { riskRuleType: "maxDrawdown", riskValue: 10, riskAmountType: "strategy.percent_of_equity", alert_message: "dd" },
      { riskRuleType: "maxIntradayLoss", riskValue: 500, riskAmountType: "strategy.cash", alert_message: "day" },
      { riskRuleType: "maxIntradayFilledOrders", riskCount: 3, alert_message: "fills" },
      { riskRuleType: "maxPositionSize", riskContracts: 12 },
      { riskRuleType: "maxConsLossDays", riskCount: 2, alert_message: "days" },
    ].forEach((expected, index) => expect(riskNodes[index]?.properties).toMatchObject(expected));
  });

  it("renders legacy visual risk blocks back to strategy.risk statements", () => {
    const model = createLinearVisualModel([
      {
        id: "risk-drawdown",
        text: "最大回撤",
        properties: {
          blockKind: "riskRule",
          riskRuleType: "maxDrawdown",
          riskValue: 10,
          riskAmountType: "strategy.percent_of_equity",
          alert_message: "dd",
        },
      },
      {
        id: "risk-loss",
        text: "日内亏损",
        properties: {
          blockKind: "riskRule",
          riskRuleType: "maxIntradayLoss",
          riskValue: 500,
          riskAmountType: "strategy.cash",
          alert_message: "day",
        },
      },
      {
        id: "risk-fills",
        text: "成交上限",
        properties: {
          blockKind: "riskRule",
          riskRuleType: "maxIntradayFilledOrders",
          riskCount: 3,
          alert_message: "fills",
        },
      },
      {
        id: "risk-size",
        text: "最大持仓",
        properties: {
          blockKind: "riskRule",
          riskRuleType: "maxPositionSize",
          riskContracts: 12,
        },
      },
      {
        id: "risk-loss-days",
        text: "连续亏损天数",
        properties: {
          blockKind: "riskRule",
          riskRuleType: "maxConsLossDays",
          riskCount: 2,
          alert_message: "days",
        },
      },
    ]);

    const script = buildStrategyPineFromVisualModel(model, { name: "Legacy Risk Rules" });

    expect(script).toContain('strategy.risk.max_drawdown(10, strategy.percent_of_equity, alert_message="dd")');
    expect(script).toContain('strategy.risk.max_intraday_loss(500, strategy.cash, alert_message="day")');
    expect(script).toContain('strategy.risk.max_intraday_filled_orders(3, alert_message="fills")');
    expect(script).toContain("strategy.risk.max_position_size(12)");
    expect(script).toContain('strategy.risk.max_cons_loss_days(2, alert_message="days")');
  });
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
    expect(script).toContain("if ta.rising(close, 3)");
    expect(script).toContain("if ta.barssince(close > 520) < 5");
    expect(script).toContain("if ta.valuewhen(close > 520, close, 0) > 500");

    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Series", overlay=true)
if volume > 1000000
    if ta.rising(close, 3)
        if ta.barssince(close > 520) < 5
            if ta.valuewhen(close > 520, close, 0) > 500
                strategy.entry("Long", strategy.long, qty=1)
`);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }
    const seriesNodes = parsed.model.nodes.filter((node) => node.properties.blockKind === "seriesCondition");
    expect(seriesNodes.map((node) => node.properties.mode)).toEqual([
      "compare",
      "rising",
      "barssince",
      "valuewhen",
    ]);

    const legacyParsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Legacy Series", overlay=true)
if rising(close, 3)
    if barssince(close > 520) < 5
        if valuewhen(close > 520, close, 0) > 500
            strategy.entry("Long", strategy.long, qty=1)
`);
    expect(legacyParsed.ok).toBe(false);
    expect(legacyParsed.error).toContain("第 3 行无法同步为流程图：if rising(close, 3)");
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
        { id: "mtf-trend", type: "rect", x: 600, y: 280, text: "MTF trend", properties: { blockKind: "mtfSeries", variableName: "daily_trend", timeframe: "D", expressionType: "indicator", indicatorExpression: "ta.supertrend(3, 10)", mtfField: "direction" } },
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
    expect(script).toContain('daily_trend = request.security(syminfo.tickerid, "D", ta.supertrend(3, 10).direction)');
    expect(script).toContain("range_median = array.from(close, open, high).median()");
    expect(script).toContain("var armed = false");
    expect(script).toContain("armed := close > prev_close");

    const parsed = buildStrategyVisualModelFromPine(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }
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
