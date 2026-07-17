import { describe, expect, it } from "vitest";

import { buildStrategyVisualModelFromPine } from "../src/features/strategyVisualBuilderPineParser";

describe("strategy visual Pine parser recovery", () => {
  it("recognizes complete KDJ and time-series conditions as structured strategy blocks", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Structured Signals", overlay=true)
kdj_highest = ta.highest(high, 9)
kdj_lowest = ta.lowest(low, 9)
kdj_rsv = kdj_highest == kdj_lowest ? 50 : ((close - kdj_lowest) / (kdj_highest - kdj_lowest)) * 100
var kdj_k = 50.0
var kdj_d = 50.0
kdj_k := ((2) * nz(kdj_k[1], 50) + kdj_rsv) / 3
kdj_d := ((2) * nz(kdj_d[1], 50) + kdj_k) / 3
kdj_j = 3 * kdj_k - 2 * kdj_d
if ta.rising(close, 3)
    log.info("rising")
if ta.barssince(close < 10) > 2
    log.info("recovered")
if ta.valuewhen(close < 10, low, 1) < 8
    alert("value window")`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;

    expect(
      parsed.model.nodes.find(
        (node) => node.properties.blockKind === "getTechnicalIndicator",
      )?.properties,
    ).toMatchObject({
      indicatorType: "kdj",
      variableName: "kdj",
      period: 9,
      m1: 3,
      m2: 3,
    });
    expect(
      parsed.model.nodes.filter(
        (node) => node.properties.blockKind === "seriesCondition",
      ).map((node) => node.properties.mode),
    ).toEqual(["rising", "barssince", "valuewhen"]);
  });

  it("does not turn a damaged KDJ sequence into a complete indicator block", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Damaged Signal", overlay=true)
kdj_highest = ta.highest(high, 9)
kdj_lowest = ta.lowest(low, 9)
kdj_rsv = kdj_highest == kdj_lowest ? 50 : ((close - kdj_lowest) / (kdj_highest - kdj_lowest)) * 100
var kdj_k = 50.0
var kdj_d = 50.0
kdj_k := ((1) * nz(kdj_k[1], 50) + kdj_rsv) / 3
kdj_d := ((2) * nz(kdj_d[1], 50) + kdj_k) / 3
kdj_j = 3 * kdj_k - 2 * kdj_d`);

    expect(parsed).toMatchObject({
      ok: false,
      error: expect.stringContaining("kdj_rsv"),
    });
  });

  it("falls back to a standalone indicator for incomplete compatibility blocks and rejects malformed flow syntax", () => {
    const incompleteKdj = buildStrategyVisualModelFromPine(`//@version=6
strategy("Incomplete KDJ", overlay=true)
kdj_highest = ta.highest(high, 9)`);
    expect(incompleteKdj.ok).toBe(true);
    if (incompleteKdj.ok) {
      expect(incompleteKdj.model.nodes.find((node) => node.properties.indicatorType === "highest")).toMatchObject({
        properties: { period: 9 },
      });
    }

    const malformedAssignment = buildStrategyVisualModelFromPine(`//@version=6
strategy("Malformed Assignment", overlay=true)
var = close`);
    expect(malformedAssignment).toMatchObject({
      ok: false,
      error: expect.stringContaining("var = close"),
    });

    const orphanedElse = buildStrategyVisualModelFromPine(`//@version=6
strategy("Orphaned Else", overlay=true)
if close > 10
  else
    alert("invalid")`);
    expect(orphanedElse).toMatchObject({
      ok: false,
      error: expect.stringContaining("缺少对应的 if 条件"),
    });
  });

  it("preserves a top-level statement after a condition whose body is absent", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Empty Body", overlay=true)
if close > 10
log.info("still reachable")`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    expect(parsed.model.nodes.find((node) => node.properties.message === "still reachable")).toBeDefined();
  });
});
