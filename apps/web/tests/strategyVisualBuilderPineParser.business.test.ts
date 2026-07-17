import { describe, expect, it } from "vitest";

import type { StrategyVisualModelDocument } from "@/contracts";

import { buildStrategyVisualModelFromPine } from "../src/features/strategyVisualBuilderPineParser";

describe("strategyVisualBuilderPineParser business boundaries", () => {
  it("reuses persisted layout while deduplicating annotated node ids", () => {
    const existingModel: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        {
          id: "on-kline-root",
          type: "circle",
          x: 960,
          y: 540,
          text: "已有根节点",
          properties: { blockKind: "onKLineClosed" },
        },
        {
          id: "shared-node",
          type: "rect",
          x: 320,
          y: 240,
          text: "沿用节点",
          properties: { blockKind: "log" },
        },
      ],
      edges: [],
    };

    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Existing Layout", overlay=true)
// @jftradeFlowNodeId shared-node
// @jftradeFlowBlockKind log
log.info("first")
// @jftradeFlowNodeId shared-node
// @jftradeFlowBlockKind notify
alert("second")`, existingModel);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    expect(parsed.model.nodes[0]).toMatchObject({
      id: "on-kline-root",
      x: 960,
      y: 540,
      text: "已有根节点",
    });
    expect(parsed.model.nodes.find((node) => node.id === "shared-node")).toMatchObject({
      x: 320,
      y: 240,
      text: "沿用节点",
      properties: { message: "first" },
    });
    expect(parsed.model.nodes.find((node) => node.id === "shared-node-2")).toMatchObject({
      properties: { blockKind: "notify", message: "second" },
    });
  });

  it("parses richer Pine input, state, condition, risk, and indicator shapes", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Business Parser", overlay=true)
threshold = input.float(defval=1.25, title="Threshold")
var phase = 'armed'
atr_value = ta.atr(14)
cci_value = ta.cci(hlc3, 20)
obv_value = ta.obv(close)
strategy.risk.allow_entry_in(strategy.direction.all)
if session_ispremarket
    log.info("premarket")
if ta.barssince(close > 10) < 3
    log.info("recent move")
if ta.valuewhen(close > 10, high, 1) > 12
    alert("valuewhen")`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    expect(parsed.model.nodes.find((node) => node.properties.blockKind === "strategyInput")?.properties).toMatchObject({
      inputType: "float",
      title: "Threshold",
      defaultValue: 1.25,
      variableName: "threshold",
    });
    expect(parsed.model.nodes.find((node) => node.properties.blockKind === "stateVariable")?.properties).toMatchObject({
      valueType: "string",
      initialValue: "armed",
    });
    expect(parsed.model.nodes.find((node) => node.text === "获取 ATR 14")?.properties).toMatchObject({
      indicatorType: "atr",
      period: 14,
    });
    expect(parsed.model.nodes.find((node) => node.text === "获取 CCI 20")?.properties).toMatchObject({
      indicatorType: "cci",
      source: "hlc3",
      period: 20,
    });
    expect(parsed.model.nodes.find((node) => node.properties.indicatorType === "obv")?.properties).toMatchObject({
      indicatorType: "obv",
      source: "close",
    });
    expect(parsed.model.nodes.find((node) => node.properties.blockKind === "riskRule")?.properties).toMatchObject({
      riskRuleType: "allowEntryIn",
      riskAllowedDirection: "all",
    });
    expect(parsed.model.nodes.find((node) => node.properties.blockKind === "sessionFilter")?.properties).toMatchObject({
      scope: "premarket",
    });
    expect(parsed.model.nodes.find((node) => node.properties.mode === "barssince")?.properties).toMatchObject({
      mode: "barssince",
      operator: "<",
      length: 3,
    });
    expect(parsed.model.nodes.find((node) => node.properties.mode === "valuewhen")?.properties).toMatchObject({
      mode: "valuewhen",
      threshold: 12,
      occurrence: 1,
      operator: ">",
    });
  });

  it("falls back to raw Pine string contents when a quoted literal is not valid JSON", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Literal Fallback", overlay=true)
source_input = input.source(defval=custom_feed, title="Broken \\q title")`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    expect(
      parsed.model.nodes.find((node) => node.properties.blockKind === "strategyInput")?.properties,
    ).toMatchObject({
      inputType: "source",
      variableName: "source_input",
      title: "Broken \\q title",
      defaultValue: "custom_feed",
    });
  });

  it("preserves the synthetic root and real order quantity semantics", () => {
    const rootOnly = buildStrategyVisualModelFromPine(`//@version=6
strategy("Metadata Only", overlay=true)`);
    expect(rootOnly.ok).toBe(true);
    if (rootOnly.ok) {
      expect(rootOnly.model.nodes).toMatchObject([
        {
          id: "on-kline-root",
          properties: { blockKind: "onKLineClosed" },
        },
      ]);
    }

    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Order Shapes", overlay=true)
strategy.entry("Long", strategy.long)
strategy.entry("Scaled", strategy.long, qty=(strategy.equity * 25 / 100) / close)`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const orders = parsed.model.nodes.filter(
      (node) => node.properties.blockKind === "placeOrder",
    );
    expect(orders).toHaveLength(2);
    expect(orders[0]?.properties).toMatchObject({
      quantityMode: "shares",
      quantityValue: 100,
    });
    expect(orders[1]?.properties).toMatchObject({
      quantityMode: "equityPercent",
      quantityValue: 25,
    });
  });

  it("rejects incomplete strategy exits that do not carry a real protective rule", () => {
    expect(
      buildStrategyVisualModelFromPine(`//@version=6
strategy("Incomplete Exit", overlay=true)
strategy.exit("Close", "Long")`),
    ).toMatchObject({
      ok: false,
      error: expect.stringContaining('strategy.exit("Close", "Long")'),
    });
  });

  it("maps annotated close conditions to decision diamonds and keeps both control branches", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Annotated Decisions", overlay=true)
// @jftradeFlowNodeId take-profit-check
// @jftradeFlowBlockKind ifCloseAbove
// @jftradeFlowNodeText 达到止盈价
if close > 120
    alert("take profit")
else
    log.info("hold")
// @jftradeFlowNodeId stop-loss-check
// @jftradeFlowBlockKind ifCloseBelow
if close < 90
    alert("stop loss")`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const takeProfit = parsed.model.nodes.find((node) => node.id === "take-profit-check");
    const stopLoss = parsed.model.nodes.find((node) => node.id === "stop-loss-check");
    expect(takeProfit).toMatchObject({
      type: "diamond",
      text: "达到止盈价",
      properties: { blockKind: "ifCloseAbove", threshold: 120 },
    });
    expect(stopLoss).toMatchObject({
      type: "diamond",
      properties: { blockKind: "ifCloseBelow", threshold: 90 },
    });
    expect(
      parsed.model.edges.filter((edge) => edge.sourceNodeId === "take-profit-check"),
    ).toEqual(expect.arrayContaining([
      expect.objectContaining({ properties: expect.objectContaining({ branch: "true" }) }),
      expect.objectContaining({ properties: expect.objectContaining({ branch: "false" }) }),
    ]));
  });

  it("rejects a malformed expanded KDJ compatibility block instead of silently assigning aliases", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Broken KDJ", overlay=true)
kdj_highest = ta.highest(high, 9)
kdj_lowest = ta.lowest(low, 9)
kdj_rsv = kdj_highest == kdj_lowest ? 50 : ((close - kdj_lowest) / (kdj_highest - kdj_lowest)) * 100
var kdj_k = 50.0
var kdj_d = 50.0
kdj_k := ((2) * nz(kdj_k[1], 50) + kdj_rsv) / 3
kdj_d := ((2) * nz(kdj_d[1], 50) + kdj_k) / 4
kdj_j = 3 * kdj_k - 2 * kdj_d`);

    expect(parsed).toMatchObject({
      ok: false,
      error: expect.stringContaining("kdj_rsv = kdj_highest"),
    });
  });

  it("rejects unsupported Pine forms with explicit, business-facing parser errors", () => {
    const cases = [
      {
        name: "request security symbol mismatch",
        script: `//@version=6
strategy("Bad Security", overlay=true)
ema_bad = request.security("AAPL", "D", ta.ema(close, 9))`,
        message: 'ema_bad = request.security("AAPL", "D", ta.ema(close, 9))',
      },
      {
        name: "request security unsupported timeframe",
        script: `//@version=6
strategy("Bad Timeframe", overlay=true)
ema_bad = request.security(syminfo.tickerid, "2H", ta.ema(close, 9))`,
        message: 'ema_bad = request.security(syminfo.tickerid, "2H", ta.ema(close, 9))',
      },
      {
        name: "request security unsupported indicator",
        script: `//@version=6
strategy("Bad Indicator", overlay=true)
vwap_bad = request.security(syminfo.tickerid, "D", ta.vwap(close))`,
        message: 'vwap_bad = request.security(syminfo.tickerid, "D", ta.vwap(close))',
      },
      {
        name: "request security invalid field split",
        script: `//@version=6
strategy("Bad Field", overlay=true)
daily_bad = request.security(syminfo.tickerid, "D", custom.field)`,
        message: 'daily_bad = request.security(syminfo.tickerid, "D", custom.field)',
      },
      {
        name: "collection too many args",
        script: `//@version=6
strategy("Bad Collection", overlay=true)
range_bad = array.from(close, open, high, low).avg()`,
        message: "range_bad = array.from(close, open, high, low).avg()",
      },
      {
        name: "collection invalid expression",
        script: `//@version=6
strategy("Bad Collection Expr", overlay=true)
range_bad = array.from(close, open >, high).avg()`,
        message: "range_bad = array.from(close, open >, high).avg()",
      },
      {
        name: "unsupported risk line under place order annotation",
        script: `//@version=6
strategy("Bad Risk", overlay=true)
// @jftradeFlowNodeId forced-place-order
// @jftradeFlowBlockKind placeOrder
strategy.risk.max_drawdown(10, strategy.cash)`,
        message: "strategy.risk.max_drawdown(10, strategy.cash)",
      },
      {
        name: "missing aliases for crossover condition",
        script: `//@version=6
strategy("Bad Cross", overlay=true)
if ta.crossover(fast, slow)
    log.info("x")`,
        message: "if ta.crossover(fast, slow)",
      },
      {
        name: "missing aliases for divergence condition",
        script: `//@version=6
strategy("Bad Divergence", overlay=true)
if divergence_top(rsi_alias, 5)
    log.info("x")`,
        message: "if divergence_top(rsi_alias, 5)",
      },
      {
        name: "missing aliases for bollinger condition",
        script: `//@version=6
strategy("Bad Bollinger", overlay=true)
if close > bands.upper
    log.info("x")`,
        message: "if close > bands.upper",
        ok: true,
      },
      {
        name: "unsupported indicator expression",
        script: `//@version=6
strategy("Bad Indicator Expr", overlay=true)
custom_signal = marketSentiment`,
        message: "custom_signal = marketSentiment",
      },
      {
        name: "exit without arguments",
        script: `//@version=6
strategy("Bad Exit", overlay=true)
strategy.exit(`,
        message: "strategy.exit(",
      },
      {
        name: "exit invalid stop expression",
        script: `//@version=6
strategy("Bad Exit Stop", overlay=true)
strategy.exit("Exit", stop=close > )`,
        message: 'strategy.exit("Exit", stop=close > )',
      },
      {
        name: "exit invalid trailing expression",
        script: `//@version=6
strategy("Bad Exit Trail", overlay=true)
strategy.exit("Exit", trail_points=close > , trail_offset=1)`,
        message: 'strategy.exit("Exit", trail_points=close > , trail_offset=1)',
      },
    ];

    for (const testCase of cases) {
      const parsed = buildStrategyVisualModelFromPine(testCase.script);
      if (testCase.ok === true) {
        expect(parsed.ok, testCase.name).toBe(true);
        if (!parsed.ok) {
          continue;
        }
        expect(parsed.model.nodes.find((node) => node.properties.blockKind === "seriesCondition")?.properties).toMatchObject({
          mode: "compare",
        });
        continue;
      }
      expect(parsed, testCase.name).toMatchObject({
        ok: false,
        error: expect.stringContaining(testCase.message),
      });
    }
  });
});
