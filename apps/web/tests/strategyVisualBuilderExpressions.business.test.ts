import { describe, expect, it } from "vitest";

import {
  binaryExpression,
  expressionToLegacyString,
  literalExpression,
  normalizeVisualExpression,
  parsePineExpressionToVisualExpression,
  renderVisualExpressionToPine,
  sourceExpression,
} from "../src/features/strategyVisualBuilderExpressions";

describe("strategyVisualBuilderExpressions business boundaries", () => {
  it("normalizes malformed payloads without inventing trading intent", () => {
    const fallback = sourceExpression("high");

    expect(binaryExpression(sourceExpression("open"), "and", literalExpression(true))).toEqual({
      kind: "binary",
      left: { kind: "source", source: "open" },
      operator: "and",
      right: { kind: "literal", value: true },
    });
    expect(expressionToLegacyString(binaryExpression(sourceExpression("open"), "and", literalExpression(true)), "close")).toBe("open and true");

    expect(normalizeVisualExpression({ kind: "literal", value: { broken: true } }, fallback)).toEqual(fallback);
    expect(normalizeVisualExpression({ kind: "unknown" }, fallback)).toEqual(fallback);
    expect(normalizeVisualExpression({
      kind: "unary",
      operator: "not",
      argument: { kind: "source", source: "bad-source" },
    })).toEqual({
      kind: "unary",
      operator: "not",
      argument: { kind: "source", source: "close" },
    });
    expect(normalizeVisualExpression({
      kind: "call",
      functionName: "unsupported.fn",
      args: [{ kind: "literal", value: 1 }, null, { kind: "source", source: "open" }, { kind: "source", source: "close" }, { kind: "source", source: "high" }],
    })).toEqual({
      kind: "call",
      functionName: "math.max",
      args: [
        { kind: "literal", value: 1 },
        { kind: "source", source: "close" },
        { kind: "source", source: "open" },
        { kind: "source", source: "close" },
      ],
    });

    expect(renderVisualExpressionToPine({
      kind: "literal",
      value: "say \"hi\" \\ now",
    })).toBe("\"say \\\"hi\\\" \\\\ now\"");
    expect(renderVisualExpressionToPine({
      kind: "unary",
      operator: "not",
      argument: { kind: "source", source: "close" },
    })).toBe("not close");
  });

  it("parses quoted, wrapped, unary, and aliased Pine expressions safely", () => {
    expect(parsePineExpressionToVisualExpression("")).toBeNull();
    expect(parsePineExpressionToVisualExpression("not close")).toEqual({
      kind: "unary",
      operator: "not",
      argument: { kind: "source", source: "close" },
    });
    expect(parsePineExpressionToVisualExpression("- close")).toEqual({
      kind: "unary",
      operator: "-",
      argument: { kind: "source", source: "close" },
    });
    expect(parsePineExpressionToVisualExpression("'signal'")).toEqual({
      kind: "literal",
      value: "signal",
    });
    expect(parsePineExpressionToVisualExpression("\"buy and hold\" == \"sell or wait\"")).toEqual({
      kind: "binary",
      operator: "==",
      left: { kind: "literal", value: "buy and hold" },
      right: { kind: "literal", value: "sell or wait" },
    });
    expect(parsePineExpressionToVisualExpression("candor == true")).toEqual({
      kind: "binary",
      operator: "==",
      left: { kind: "reference", name: "candor" },
      right: { kind: "literal", value: true },
    });
    expect(parsePineExpressionToVisualExpression("(close) and (open)")).toEqual({
      kind: "binary",
      operator: "and",
      left: { kind: "source", source: "close" },
      right: { kind: "source", source: "open" },
    });
    expect(parsePineExpressionToVisualExpression("(\"wrapped signal\")")).toEqual({
      kind: "literal",
      value: "wrapped signal",
    });
    expect(parsePineExpressionToVisualExpression("barssince(close > 1) > 3")).toEqual({
      kind: "binary",
      operator: ">",
      left: {
        kind: "call",
        functionName: "ta.barssince",
        args: [{
          kind: "binary",
          operator: ">",
          left: { kind: "source", source: "close" },
          right: { kind: "literal", value: 1 },
        }],
      },
      right: { kind: "literal", value: 3 },
    });
    expect(parsePineExpressionToVisualExpression("valuewhen(close > 1, \"a,b\", 0) > 1")).toEqual({
      kind: "binary",
      operator: ">",
      left: {
        kind: "call",
        functionName: "ta.valuewhen",
        args: [
          {
            kind: "binary",
            operator: ">",
            left: { kind: "source", source: "close" },
            right: { kind: "literal", value: 1 },
          },
          { kind: "literal", value: "a,b" },
          { kind: "literal", value: 0 },
        ],
      },
      right: { kind: "literal", value: 1 },
    });
    expect(parsePineExpressionToVisualExpression("math.max(\"x,y\", nz(close[1], 0))")).toEqual({
      kind: "call",
      functionName: "math.max",
      args: [
        { kind: "literal", value: "x,y" },
        {
          kind: "call",
          functionName: "nz",
          args: [
            {
              kind: "history",
              target: { kind: "source", source: "close" },
              offset: 1,
            },
            { kind: "literal", value: 0 },
          ],
        },
      ],
    });
    expect(parsePineExpressionToVisualExpression("custom.fn(close)")).toBeNull();
  });
});
