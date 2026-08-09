import { describe, expect, it } from "vitest";

import type { StrategyVisualNodeDocument } from "@/types";

import { normalizeMtfSeriesBlockProperties } from "@/features/strategy-builder/strategyVisualBuilderCatalog";
import { normalizeGetTechnicalIndicatorProperties } from "@/features/strategy-builder/strategyVisualBuilderIndicatorBlock";
import {
  formatNumber,
  formatPineValue,
  sanitizeMetadataValue,
} from "@/features/strategy-builder/strategyVisualBuilderPineFormat";
import {
  buildIndicatorExpression,
  readSyntheticKDJVariableName,
  wrapIndicatorTimeframe,
} from "@/features/strategy-builder/strategyVisualBuilderPineIndicatorExpressions";
import {
  buildMtfInnerExpression,
  buildStateUpdateStatement,
} from "@/features/strategy-builder/strategyVisualBuilderPineStatements";

describe("strategy visual builder pine render boundaries", () => {
  it("keeps readable metadata and numeric formatting for malformed values", () => {
    expect(sanitizeMetadataValue("first\nsecond", "fallback")).toBe(
      "first second",
    );
    expect(formatNumber(Number.NaN)).toBe("0");
    expect(formatPineValue(undefined)).toBe("0");
  });

  it("uses the raw indicator expression when it cannot be parsed to an AST", () => {
    const mtf = normalizeMtfSeriesBlockProperties({
      variableName: "mtf_custom",
      timeframe: "D",
      expressionType: "indicator",
      indicatorExpression: "custom.fn(close)",
    });
    expect(buildMtfInnerExpression(mtf)).toBe("custom.fn(close)");

    const node = {
      properties: {
        variableName: "armed",
        expression: "custom.fn(close)",
      },
    } as unknown as StrategyVisualNodeDocument;
    expect(buildStateUpdateStatement(node)).toBe("armed := custom.fn(close)");
  });

  it("renders KDJ names and skips request.security for non-wrappable indicators", () => {
    const normalized = normalizeGetTechnicalIndicatorProperties({
      indicatorType: "kdj",
      variableName: "kdj_fast",
    });
    expect(buildIndicatorExpression(normalized)).toBe("kdj_fast_j");
    expect(
      buildIndicatorExpression(
        normalizeGetTechnicalIndicatorProperties({ indicatorType: "kdj" }),
      ),
    ).toBe("kdj_j");
    expect(readSyntheticKDJVariableName("kdj", "k")).toBe("kdj_k");
    expect(
      wrapIndicatorTimeframe({ indicatorType: "williamsR" }, "ta.wpr(14)"),
    ).toBe("ta.wpr(14)");
  });
});
