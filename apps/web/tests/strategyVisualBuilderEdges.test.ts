import { describe, expect, it } from "vitest";

import type { StrategyVisualEdgeDocument } from "../src/contracts";
import {
  buildStrategyVisualControlEdgeProperties,
  buildStrategyVisualDataEdgeProperties,
  isStrategyVisualControlEdge,
  isStrategyVisualDataEdge,
  normalizeStrategyVisualEdgeProperties,
  readStrategyVisualEdgeBranch,
  readStrategyVisualEdgeInputSlot,
} from "../src/features/strategyVisualBuilderEdges";

function edge(properties?: Record<string, unknown>): StrategyVisualEdgeDocument {
  return {
    sourceNodeId: "entry",
    targetNodeId: "indicator",
    type: "default",
    properties,
  };
}

describe("strategy visual builder edge contracts", () => {
  it("keeps data edges only when their input slot is supported", () => {
    const valid = edge({ role: "data", slot: "fast" });
    const unsupported = edge({ role: "data", slot: "not-a-slot", branch: "true" });

    expect(isStrategyVisualDataEdge(valid)).toBe(true);
    expect(readStrategyVisualEdgeInputSlot(valid)).toBe("fast");
    expect(normalizeStrategyVisualEdgeProperties(valid.properties)).toEqual({
      role: "data",
      slot: "fast",
    });

    // A branch belongs to control flow and must not leak onto indicator inputs.
    expect(isStrategyVisualDataEdge(unsupported)).toBe(true);
    expect(readStrategyVisualEdgeInputSlot(unsupported)).toBeNull();
    expect(normalizeStrategyVisualEdgeProperties(unsupported.properties)).toEqual({
      role: "data",
    });
  });

  it("preserves only declared control-flow branches", () => {
    const conditional = edge({ role: "control", branch: "true" });
    const malformed = edge({ role: "control", branch: "otherwise" });

    expect(isStrategyVisualControlEdge(conditional)).toBe(true);
    expect(readStrategyVisualEdgeBranch(conditional)).toBe("true");
    expect(readStrategyVisualEdgeBranch(malformed)).toBeNull();
    expect(normalizeStrategyVisualEdgeProperties(malformed.properties)).toEqual({
      role: "control",
    });
    expect(buildStrategyVisualControlEdgeProperties()).toEqual({ role: "control" });
    expect(buildStrategyVisualControlEdgeProperties("false")).toEqual({
      role: "control",
      branch: "false",
    });
    expect(buildStrategyVisualDataEdgeProperties("slow")).toEqual({
      role: "data",
      slot: "slow",
    });
  });
});
