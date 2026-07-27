import { describe, expect, it } from "vitest";

import {
  buildPineSourceStructureIndex,
  buildWorkflowSnapshotFromSource,
  readSourceBlockField,
  sourceBlockEditableFields,
} from "../src/features/pineSourceStructureIndex";
import { readLogicalLine, splitSourceLines } from "../src/features/pineSourceStructureLines";
import {
  classifyIndexedRawCall,
  classifyIndexedRawDeclaration,
  classifyIndexedRawObject,
} from "../src/features/pineSourceStructureRules";

describe("Pine source structure lexer recovery", () => {
  it("keeps escaped quotes inside a multiline call instead of ending the logical line early", () => {
    const lines = splitSourceLines(String.raw`alert("price \"breakout\"",
  "review the order") // comment after the call`);
    const logical = readLogicalLine(lines, 0);

    expect(logical.nextIndex).toBe(2);
    expect(logical.line.endNumber).toBe(2);
    expect(logical.line.trimmed).toContain(String.raw`\"breakout\"`);
    expect(logical.line.trimmed).toContain("review the order");
  });

  it("classifies raw Pine forms that are deliberately not editable workflow instructions", () => {
    expect(classifyIndexedRawObject("ta.sma(close, 10)")).toBeNull();
    expect(classifyIndexedRawDeclaration("varip intraBar = 0")).toMatchObject({
      kind: "declaration",
      label: "类型声明 intraBar",
    });
    expect(classifyIndexedRawDeclaration("var CustomState state = na")).toMatchObject({
      kind: "declaration",
      label: "类型状态变量 state",
      detail: "CustomState = na",
    });
    expect(classifyIndexedRawCall("request.financial(syminfo.tickerid, \"TOTAL_REVENUE\", \"FY\")")).toMatchObject({
      kind: "request",
      label: "数据请求",
    });
  });

  it("keeps bar-close scaffolding out of the workflow while exposing raw and state edit contracts", () => {
    const source = `//@version=6
strategy("Stateful", overlay=true)
phase = close
barClosed = barstate.isconfirmed
if barClosed
    log.info("close confirmed")
import TradingView/ta/7 as ta7
`;
    const blocks = buildPineSourceStructureIndex(source);
    const stateBlock = blocks.find((block) => block.label === "phase");
    const importBlock = blocks.find((block) => block.kind === "library");

    expect(sourceBlockEditableFields(stateBlock!).map((field) => field.key)).toEqual([
      "name",
      "expression",
    ]);
    expect(sourceBlockEditableFields(importBlock!)).toEqual([]);

    const workflow = buildWorkflowSnapshotFromSource(source);
    expect(workflow.blocks.map((block) => block.kind)).toEqual([
      "series_assign",
      "log",
    ]);
    expect(workflow.blocks[1]?.params.message).toBe("close confirmed");
  });

  it("preserves a normal if/else workflow branch rather than treating it as bar-close scaffolding", () => {
    const workflow = buildWorkflowSnapshotFromSource(`//@version=6
strategy("Branch", overlay=true)
if close > 10
    log.info("bull")
else
    log.info("risk")`);

    expect(workflow.blocks).toHaveLength(1);
    expect(workflow.blocks[0]).toMatchObject({
      kind: "if",
      params: { condition: "close > 10" },
      thenBlocks: [{ kind: "log", params: { message: "bull" } }],
      elseBlocks: [{ kind: "log", params: { message: "risk" } }],
    });
  });

  it("classifies object operations and deliberately malformed raw declarations without making them editable workflow nodes", () => {
    expect(classifyIndexedRawObject("state.value := close")).toMatchObject({
      kind: "object",
      label: "对象字段更新 state.value",
    });
    expect(classifyIndexedRawObject("TradeState.new(close)")).toMatchObject({
      kind: "object",
      label: "对象构造 TradeState",
    });
    expect(classifyIndexedRawObject("state[1].value")).toMatchObject({
      kind: "object",
      label: "对象历史读取 state",
    });
    expect(classifyIndexedRawObject("state.update(close)")).toMatchObject({
      kind: "object",
      label: "对象方法 state",
    });
    expect(classifyIndexedRawObject("state.value")).toMatchObject({
      kind: "object",
      label: "对象字段读取 state",
    });
    expect(classifyIndexedRawObject("state.")).toBeNull();
    expect(classifyIndexedRawDeclaration("const threshold = 3")).toMatchObject({
      kind: "declaration",
      // The abbreviated form is deliberately treated as a typed declaration;
      // the editor must not promote an ambiguous raw form to executable state.
      label: "类型声明 threshold",
    });
    expect(classifyIndexedRawDeclaration("float threshold")).toMatchObject({
      kind: "declaration",
      label: "字段声明 threshold",
    });
  });

  it("serializes declaration scalars for the source editor", () => {
    const blocks = buildPineSourceStructureIndex(`//@version=6
strategy("Scalar", overlay=true, initial_capital=1000)`);
    const declaration = blocks.find((block) => block.kind === "strategy");

    expect(readSourceBlockField(declaration!, "overlay")).toBe("true");
    expect(readSourceBlockField(declaration!, "initialCapital")).toBe("1000");
  });
});
