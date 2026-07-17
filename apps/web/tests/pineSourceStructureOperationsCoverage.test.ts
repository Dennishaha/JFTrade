import { describe, expect, it } from "vitest";

import {
  buildPineSourceStructureIndex,
  deleteSourceBlock,
  duplicateSourceBlock,
  insertSourceBlock,
  moveSourceBlock,
  replaceSourceBlockKind,
} from "../src/features/pineSourceStructureIndex";

describe("Pine source structure edit boundaries", () => {
  it("keeps a source unchanged when an edit names a block that no longer exists", () => {
    const source = `strategy("Boundary", overlay=true)\nplot(close)\n`;

    expect(deleteSourceBlock(source, "removed")).toEqual({ source, changed: false });
    expect(duplicateSourceBlock(source, "removed")).toEqual({ source, changed: false });
    expect(moveSourceBlock(source, "removed", 1)).toEqual({ source, changed: false });
    expect(replaceSourceBlockKind(source, "removed", "strategy_entry")).toEqual({
      source,
      changed: false,
    });
  });

  it("does not move the first or last statement beyond its sibling scope", () => {
    const source = `if close > open\n    fast = close\n    slow = open\n`;
    const nodes = buildPineSourceStructureIndex(source);
    const fast = nodes.find((node) => node.label === "fast")!;
    const slow = nodes.find((node) => node.label === "slow")!;

    expect(moveSourceBlock(source, fast.id, -1)).toEqual({ source, changed: false });
    expect(moveSourceBlock(source, slow.id, 1)).toEqual({ source, changed: false });
  });

  it("moves a block down, preserves its newline, and replaces scalar condition kinds", () => {
    const source = `if close > open\n    fast = close\n    slow = open\n`;
    const nodes = buildPineSourceStructureIndex(source);
    const fast = nodes.find((node) => node.label === "fast")!;
    const condition = nodes.find((node) => node.kind === "condition")!;

    const moved = moveSourceBlock(source, fast.id, 1);
    expect(moved.changed).toBe(true);
    expect(moved.source).toContain("    slow = open\n    fast = close\n");

    const replaced = replaceSourceBlockKind(source, condition.id, "switch");
    expect(replaced.changed).toBe(true);
    expect(replaced.source).toContain("switch\n");
    expect(replaced.source.endsWith("\n")).toBe(true);
  });

  it("inserts at an empty source and accepts primitive bar-closed parameters", () => {
    const inserted = insertSourceBlock("", null, "log");
    expect(inserted).toEqual({
      source: 'log.info("Pine v6 工作流")\n',
      changed: true,
    });

    const numericCondition = `if 1\n    closeValue = close\n`;
    const condition = buildPineSourceStructureIndex(numericCondition).find(
      (node) => node.kind === "condition",
    )!;
    const selected = insertSourceBlock(numericCondition, condition.id, "plot");
    expect(selected.source).toContain("    plot(close");
  });
});
