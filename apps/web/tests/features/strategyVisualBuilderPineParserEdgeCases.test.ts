import { describe, expect, it } from "vitest";

import type { StrategyVisualNodeDocument } from "@/types";

import {
  parseCollectionStatExpression,
  parseDerivedSeriesExpression,
  parseInputDefaultValue,
  parseMtfSeriesExpression,
  parseSeriesCondition,
  parseStrategyInputExpression,
} from "@/features/strategy-builder/strategyVisualBuilderPineParserExpressions";
import {
  addControlEdge,
  addDataEdge,
  addNode,
  ensureUniqueNodeId,
  hasEdge,
  type ParseState,
} from "@/features/strategy-builder/strategyVisualBuilderPineParserModel";

describe("strategy visual builder pine parser edge cases", () => {
  it("parses series conditions with empty call arguments and compare operators", () => {
    const barssince = parseSeriesCondition("ta.barssince() > 3");
    expect(barssince).toMatchObject({
      mode: "barssince",
      operator: ">",
      length: 3,
    });
    expect(barssince?.eventExpressionAst).toMatchObject({
      kind: "source",
      source: "close",
    });

    const valuewhen = parseSeriesCondition("ta.valuewhen() > 3");
    expect(valuewhen).toMatchObject({
      mode: "valuewhen",
      operator: ">",
      threshold: 3,
      occurrence: 0,
    });
    expect(valuewhen?.eventExpressionAst).toMatchObject({
      kind: "source",
      source: "close",
    });
    expect(valuewhen?.valueExpressionAst).toMatchObject({
      kind: "source",
      source: "close",
    });

    expect(parseSeriesCondition("close < open")).toMatchObject({
      mode: "compare",
      operator: "<",
    });
    expect(parseSeriesCondition("close == open")).toMatchObject({
      mode: "compare",
      operator: ">",
    });
  });

  it("applies input defaults when positional and named arguments are absent", () => {
    expect(parseStrategyInputExpression("input.int()")).toMatchObject({
      inputType: "int",
      title: "Input",
      defaultValue: 0,
    });
    expect(parseInputDefaultValue("source", "")).toBe("close");
    expect(parseInputDefaultValue("timeframe", "")).toBe("D");
    expect(parseInputDefaultValue("color", "")).toBe("color.green");
    expect(parseInputDefaultValue("time", "")).toBe("timestamp(2026, 1, 1)");
  });

  it("keeps source fallbacks when derived expressions cannot be parsed", () => {
    expect(parseDerivedSeriesExpression("math.abs()")).toMatchObject({
      mode: "math",
      mathFunction: "abs",
      leftExpression: "close",
      rightExpression: "open",
    });
    expect(
      parseDerivedSeriesExpression("math.max(custom.fn(close), open)"),
    ).toMatchObject({
      mode: "math",
      leftExpression: "custom.fn(close)",
      rightExpression: "open",
    });
    expect(
      parseDerivedSeriesExpression("ta.cross(custom.fn(close), open)"),
    ).toMatchObject({
      mode: "cross",
      leftExpression: "custom.fn(close)",
      rightExpression: "open",
    });
    expect(
      parseDerivedSeriesExpression("ta.cross(close, !open)"),
    ).toMatchObject({
      mode: "cross",
      leftExpression: "close",
      rightExpression: "!open",
    });
  });

  it("falls back to default timeframe and sources for sparse request.security and array calls", () => {
    expect(
      parseMtfSeriesExpression(
        "request.security(syminfo.tickerid, , close)",
      ),
    ).toMatchObject({
      timeframe: "D",
      expressionType: "source",
      source: "close",
    });
    expect(parseCollectionStatExpression("array.from(close).median()")).toMatchObject({
      statFunction: "median",
      sourceA: "close",
      sourceB: "open",
      sourceC: "high",
    });
  });

  it("deduplicates nodes and edges while keeping stable ids", () => {
    const state = createState();
    const node = createNode("shared");
    addNode(state, node);
    addNode(state, node);
    expect(state.nodes).toHaveLength(1);

    addControlEdge(state, "shared", "shared");
    addControlEdge(state, "shared", "target", "true");
    addControlEdge(state, "shared", "target", "true");
    expect(state.edges).toHaveLength(1);

    addDataEdge(state, "shared", "shared", "primary");
    addDataEdge(state, "shared", "target", "primary");
    addDataEdge(state, "shared", "target", "primary");
    expect(state.edges).toHaveLength(2);

    expect(hasEdge(state, "shared", "target", "primary", "data")).toBe(true);
    expect(hasEdge(state, "shared", "target", "true", "control")).toBe(true);

    state.nodeIds.add("pine-node");
    expect(ensureUniqueNodeId(state, "unique-node")).toBe("unique-node");
    expect(ensureUniqueNodeId(state, "   ")).toBe("pine-node-2");
  });
});

function createState(): ParseState {
  return {
    entries: [],
    index: 0,
    nodes: [],
    edges: [],
    nodeIds: new Set<string>(),
    existingNodeById: new Map<string, StrategyVisualNodeDocument>(),
    aliasByName: new Map(),
    sequence: 0,
    error: null,
  };
}

function createNode(id: string): StrategyVisualNodeDocument {
  return {
    id,
    type: "rect",
    x: 0,
    y: 0,
    text: id,
    properties: {},
  };
}
