import type {
  PineV6WorkflowBlock,
  PineV6WorkflowBlockKind,
  PineV6WorkflowDocument,
  PineV6WorkflowInput,
  PineV6WorkflowRuntimeBindingDraft,
} from "@/contracts";

import {
  PINE_V6_WORKFLOW_ENGINE,
  PINE_V6_BLOCK_KINDS,
  createDefaultPineV6Workflow,
  createWorkflowId,
} from "./pineV6Workflow";
import {
  orderLabelFromBlock,
  makeBlock,
  parseAssignmentLine,
  parseCollectionLine,
  parseInputLine,
  parseLogLine,
  parseOrderLine,
  parseRequestSecurityLine,
  parseStrategyDeclaration,
  parseVisualLine,
  readCallSummary,
  visualLabelFromBlock,
} from "./pineSourceStructureMatchers";
import {
  classifyIndexedRawCall,
  classifyIndexedRawDeclaration,
  classifyIndexedRawDefinition,
  classifyIndexedRawObject,
} from "./pineSourceStructureRules";
import {
  readLogicalLine,
  splitSourceLines,
  type SourceLine,
} from "./pineSourceStructureLines";
import {
  deleteSourceBlock as deleteSourceBlockOperation,
  duplicateSourceBlock as duplicateSourceBlockOperation,
  insertSourceBlock as insertSourceBlockOperation,
  moveSourceBlock as moveSourceBlockOperation,
  replaceSourceBlockKind as replaceSourceBlockKindOperation,
  type PineSourceOperationContext,
} from "./pineSourceStructureOperations";
import { renderBlockToSource, renderDefaultSourceBlock } from "./pineSourceStructureRender";
import { trimDetail } from "./pineSourceStructureText";

export { renderBlockToSource, renderDefaultSourceBlock };

export type PineSourceStructureKind =
  | "version"
  | "strategy"
  | "input"
  | "declaration"
  | "assignment"
  | "condition"
  | "branch"
  | "loop"
  | "switch"
  | "library"
  | "type"
  | "method"
  | "object"
  | "request"
  | "order"
  | "visual"
  | "alert"
  | "function"
  | "collection"
  | "log"
  | "runtime"
  | "comment"
  | "raw";

export type PineSourceBlockMatch =
  | { type: "raw" }
  | { type: "strategy"; declaration: Partial<PineV6WorkflowDocument["declaration"]> }
  | { type: "input"; input: PineV6WorkflowInput }
  | { type: "instruction"; block: PineV6WorkflowBlock };

export interface PineSourceBlock {
  id: string;
  kind: PineSourceStructureKind;
  label: string;
  detail: string;
  line: number;
  depth: number;
  sourceRange: { start: number; end: number };
  lineRange: { start: number; end: number };
  raw: string;
  children: PineSourceBlock[];
  match: PineSourceBlockMatch;
}

export type PineSourceStructureNode = PineSourceBlock;

export interface PineSourceEditResult {
  source: string;
  changed: boolean;
}

export function buildPineSourceStructureIndex(source: string): PineSourceStructureNode[] {
  return flattenSourceBlocks(parseSourceToBlocks(source));
}

export function parseSourceToBlocks(source: string): PineSourceBlock[] {
  const lines = splitSourceLines(source).filter((line) => line.trimmed !== "");
  const { blocks } = parseLinesAtIndent(lines, 0, -1);
  return blocks;
}

export function flattenSourceBlocks(blocks: PineSourceBlock[]): PineSourceBlock[] {
  return blocks.flatMap((block) => [block, ...flattenSourceBlocks(block.children)]);
}

export function replaceSourceRange(
  source: string,
  range: { start: number; end: number },
  nextText: string,
): string {
  return `${source.slice(0, range.start)}${nextText}${source.slice(range.end)}`;
}

const sourceOperationContext: PineSourceOperationContext = {
  parseSourceToBlocks,
  flattenSourceBlocks,
  replaceSourceRange,
};

export function buildWorkflowSnapshotFromSource(
  source: string,
  fallback: PineV6WorkflowDocument = createDefaultPineV6Workflow(),
): PineV6WorkflowDocument {
  const blocks = parseSourceToBlocks(source);
  const declaration = {
    ...fallback.declaration,
    ...firstStrategyDeclaration(blocks),
  };
  const inputs = flattenSourceBlocks(blocks)
    .flatMap((block) => block.match.type === "input" ? [block.match.input] : []);
  const workflowBlocks = collectWorkflowBlocks(blocks);
  return {
    engine: PINE_V6_WORKFLOW_ENGINE,
    version: 1,
    declaration,
    inputs,
    blocks: workflowBlocks,
    runtimeBindingDraft: fallback.runtimeBindingDraft,
  };
}

export function updateInstructionBlockParam(
  block: PineSourceBlock,
  key: string,
  value: unknown,
): PineSourceBlock {
  if (block.match.type === "strategy") {
    return {
      ...block,
      match: {
        type: "strategy",
        declaration: {
          ...block.match.declaration,
          [key]: parseDeclarationValue(key, value),
        },
      },
    };
  }
  if (block.match.type === "input") {
    return {
      ...block,
      match: {
        type: "input",
        input: {
          ...block.match.input,
          [key]: String(value),
        },
      },
    };
  }
  if (block.match.type === "instruction") {
    return {
      ...block,
      match: {
        type: "instruction",
        block: {
          ...block.match.block,
          params: {
            ...block.match.block.params,
            [key]: value,
          },
        },
      },
    };
  }
  return block;
}

export function sourceBlockEditableFields(block: PineSourceBlock): Array<{
  key: string;
  label: string;
  kind?: "select";
  options?: string[];
}> {
  if (block.match.type === "strategy") {
    return [
      { key: "title", label: "标题" },
      { key: "initialCapital", label: "初始资金" },
      { key: "pyramiding", label: "加仓层数" },
      { key: "defaultQtyValue", label: "默认仓位" },
    ];
  }
  if (block.match.type === "input") {
    return [
      { key: "name", label: "变量名" },
      { key: "type", label: "类型", kind: "select", options: ["int", "float", "bool", "string", "source", "time", "timeframe", "color"] },
      { key: "title", label: "标题" },
      { key: "defaultValue", label: "默认值" },
    ];
  }
  if (block.match.type !== "instruction") {
    return [];
  }
  switch (block.match.block.kind) {
    case "series_assign":
      return [{ key: "name", label: "变量名" }, { key: "expression", label: "表达式" }];
    case "var_state":
      return [{ key: "name", label: "变量名" }, { key: "initial", label: "初始值" }];
    case "if":
      return [{ key: "condition", label: "条件" }];
    case "request_security":
      return [{ key: "name", label: "变量名" }, { key: "symbol", label: "标的" }, { key: "timeframe", label: "周期" }, { key: "expression", label: "表达式" }];
    case "strategy_entry":
    case "strategy_order":
      return [
        { key: "id", label: "订单 ID" },
        { key: "direction", label: "方向", kind: "select", options: ["strategy.long", "strategy.short"] },
        { key: "qty", label: "数量" },
        { key: "qty_percent", label: "仓位百分比" },
        { key: "limit", label: "限价" },
        { key: "stop", label: "触发价" },
        { key: "oca_name", label: "OCA 名称" },
        { key: "oca_type", label: "OCA 类型" },
        { key: "comment", label: "备注" },
        { key: "alert_message", label: "提醒消息" },
        { key: "disable_alert", label: "禁用提醒" },
        { key: "when", label: "条件" },
      ];
    case "strategy_exit":
      return [
        { key: "id", label: "退出 ID" },
        { key: "from_entry", label: "来源入场" },
        { key: "qty", label: "数量" },
        { key: "qty_percent", label: "仓位百分比" },
        { key: "profit", label: "止盈点数" },
        { key: "limit", label: "止盈价" },
        { key: "loss", label: "止损点数" },
        { key: "stop", label: "止损/触发价" },
        { key: "trail_price", label: "追踪价格" },
        { key: "trail_points", label: "追踪点数" },
        { key: "trail_offset", label: "追踪偏移" },
        { key: "oca_name", label: "OCA 名称" },
        { key: "comment", label: "备注" },
        { key: "comment_profit", label: "止盈备注" },
        { key: "comment_loss", label: "止损备注" },
        { key: "comment_trailing", label: "追踪备注" },
        { key: "alert_message", label: "提醒消息" },
        { key: "alert_profit", label: "止盈提醒" },
        { key: "alert_loss", label: "止损提醒" },
        { key: "alert_trailing", label: "追踪提醒" },
        { key: "disable_alert", label: "禁用提醒" },
        { key: "when", label: "条件" },
      ];
    case "strategy_close":
      return [
        { key: "id", label: "入场 ID" },
        { key: "qty", label: "数量" },
        { key: "qty_percent", label: "仓位百分比" },
        { key: "limit", label: "限价" },
        { key: "stop", label: "止损/触发价" },
        { key: "comment", label: "备注" },
        { key: "alert_message", label: "提醒消息" },
        { key: "immediately", label: "立即平仓" },
        { key: "disable_alert", label: "禁用提醒" },
        { key: "when", label: "条件" },
      ];
    case "strategy_close_all":
      return [
        { key: "immediately", label: "立即平仓" },
        { key: "comment", label: "备注" },
        { key: "alert_message", label: "提醒消息" },
        { key: "disable_alert", label: "禁用提醒" },
      ];
    case "strategy_cancel":
      return [{ key: "id", label: "订单 ID" }];
    case "strategy_cancel_all":
      return [];
    case "strategy_risk_allow_entry_in":
      return [
        {
          key: "direction",
          label: "允许方向",
          kind: "select",
          options: ["strategy.direction.all", "strategy.direction.long", "strategy.direction.short"],
        },
      ];
    case "strategy_risk_max_drawdown":
    case "strategy_risk_max_intraday_loss":
      return [
        { key: "value", label: "阈值" },
        {
          key: "type",
          label: "类型",
          kind: "select",
          options: ["strategy.percent_of_equity", "strategy.cash"],
        },
        { key: "alert_message", label: "提醒消息" },
      ];
    case "strategy_risk_max_intraday_filled_orders":
    case "strategy_risk_max_cons_loss_days":
      return [
        { key: "count", label: "次数/天数" },
        { key: "alert_message", label: "提醒消息" },
      ];
    case "strategy_risk_max_position_size":
      return [{ key: "contracts", label: "合约数量" }];
    case "plot":
      return [{ key: "series", label: "序列" }, { key: "title", label: "标题" }, { key: "color", label: "颜色" }];
    case "alertcondition":
      return [{ key: "condition", label: "条件" }, { key: "title", label: "标题" }, { key: "message", label: "消息" }];
    case "log":
      return [{ key: "message", label: "消息" }];
    case "array_op":
      return [{ key: "name", label: "数组名" }, { key: "mode", label: "模式", kind: "select", options: ["new_float", "push", "median"] }, { key: "value", label: "数值" }, { key: "output", label: "输出" }];
    default:
      return [];
  }
}

export function readSourceBlockField(block: PineSourceBlock, key: string): string {
  if (block.match.type === "strategy") {
    const value = block.match.declaration[key as keyof PineV6WorkflowDocument["declaration"]];
    return value === undefined || value === null ? "" : String(value);
  }
  if (block.match.type === "input") {
    const value = block.match.input[key as keyof PineV6WorkflowInput];
    return value === undefined || value === null ? "" : String(value);
  }
  if (block.match.type === "instruction") {
    const value = block.match.block.params[key];
    return value === undefined || value === null ? "" : String(value);
  }
  return "";
}

export function insertSourceBlock(
  source: string,
  selectedBlockId: string | null,
  kind: PineV6WorkflowBlockKind,
): PineSourceEditResult {
  return insertSourceBlockOperation(sourceOperationContext, source, selectedBlockId, kind);
}

export function deleteSourceBlock(source: string, blockId: string): PineSourceEditResult {
  return deleteSourceBlockOperation(sourceOperationContext, source, blockId);
}

export function duplicateSourceBlock(source: string, blockId: string): PineSourceEditResult {
  return duplicateSourceBlockOperation(sourceOperationContext, source, blockId);
}

export function moveSourceBlock(source: string, blockId: string, direction: -1 | 1): PineSourceEditResult {
  return moveSourceBlockOperation(sourceOperationContext, source, blockId, direction);
}

export function replaceSourceBlockKind(
  source: string,
  blockId: string,
  kind: PineV6WorkflowBlockKind,
): PineSourceEditResult {
  return replaceSourceBlockKindOperation(sourceOperationContext, source, blockId, kind);
}

export function isPineV6WorkflowBlockKind(value: string): value is PineV6WorkflowBlockKind {
  return PINE_V6_BLOCK_KINDS.some((item) => item.kind === value);
}

function parseLinesAtIndent(
  lines: SourceLine[],
  startIndex: number,
  parentIndent: number,
): { blocks: PineSourceBlock[]; nextIndex: number } {
  const blocks: PineSourceBlock[] = [];
  let index = startIndex;
  while (index < lines.length) {
    const line = lines[index]!;
    if (line.indent <= parentIndent) {
      break;
    }
    const logicalLine = readLogicalLine(lines, index);
    const block = classifyLine(logicalLine.line);
    index = logicalLine.nextIndex;
    if (sourceBlockCanHaveChildren(block)) {
      const nested = parseLinesAtIndent(lines, index, line.indent);
      block.children = nested.blocks;
      if (nested.blocks.length > 0) {
        block.lineRange.end = nested.blocks[nested.blocks.length - 1]!.lineRange.end;
        block.sourceRange.end = nested.blocks[nested.blocks.length - 1]!.sourceRange.end;
      }
      index = nested.nextIndex;
      if (
        block.kind === "condition" &&
        index < lines.length &&
        lines[index]!.indent === line.indent &&
        (lines[index]!.trimmed === "else" || lines[index]!.trimmed.startsWith("else "))
      ) {
        const elseLine = lines[index]!;
        const elseBlock = classifyLine(elseLine);
        const elseNested = parseLinesAtIndent(lines, index + 1, elseLine.indent);
        elseBlock.children = elseNested.blocks;
        if (elseNested.blocks.length > 0) {
          elseBlock.lineRange.end = elseNested.blocks[elseNested.blocks.length - 1]!.lineRange.end;
          elseBlock.sourceRange.end = elseNested.blocks[elseNested.blocks.length - 1]!.sourceRange.end;
          block.lineRange.end = elseBlock.lineRange.end;
          block.sourceRange.end = elseBlock.sourceRange.end;
        }
        if (elseNested.blocks.length === 0) {
          block.lineRange.end = elseBlock.lineRange.end;
          block.sourceRange.end = elseBlock.sourceRange.end;
        }
        block.children.push(elseBlock);
        index = elseNested.nextIndex;
      }
    }
    blocks.push(block);
  }
  return { blocks, nextIndex: index };
}

function classifyLine(line: SourceLine): PineSourceBlock {
  const text = line.trimmed;
  const base = {
    id: `line-${line.number}`,
    line: line.number,
    depth: line.indent,
    sourceRange: { start: line.startOffset, end: line.endOffset },
    lineRange: { start: line.number, end: line.endNumber },
    raw: line.text,
    children: [] as PineSourceBlock[],
  };

  if (text.startsWith("//@version=")) {
    return sourceBlock(base, "version", "Pine 版本", text.replace("//@", ""), { type: "raw" });
  }
  if (text.startsWith("//")) {
    return sourceBlock(base, "comment", "注释", trimDetail(text.slice(2).trim(), text), { type: "raw" });
  }
  if (/^strategy\s*\(/.test(text)) {
    return sourceBlock(base, "strategy", "策略声明", readCallSummary(text, "strategy"), {
      type: "strategy",
      declaration: parseStrategyDeclaration(text),
    });
  }
  const input = parseInputLine(text);
  if (input !== null) {
    return sourceBlock(base, "input", `输入参数 ${input.name}`, `${input.type} = ${input.defaultValue}`, { type: "input", input });
  }
  if (/^if\s+/.test(text)) {
    return sourceBlock(base, "condition", "条件分支", text.replace(/^if\s+/, ""), {
      type: "instruction",
      block: makeBlock("if", { condition: text.replace(/^if\s+/, "") }),
    });
  }
  if (text === "else" || text.startsWith("else ")) {
    return sourceBlock(base, "branch", "否则分支", text, { type: "raw" });
  }
  if (/^for\s+/.test(text)) {
    return sourceBlock(base, "loop", "循环结构", text, { type: "raw" });
  }
  if (/^while\s+/.test(text)) {
    return sourceBlock(base, "loop", "循环结构", text, { type: "raw" });
  }
  if (/^switch(?:\s+.*)?$/.test(text)) {
    return sourceBlock(base, "switch", "条件选择", text, { type: "raw" });
  }
  const indexedDefinition = classifyIndexedRawDefinition(text);
  if (indexedDefinition !== null) {
    return sourceBlock(base, indexedDefinition.kind, indexedDefinition.label, indexedDefinition.detail, { type: "raw" });
  }
  const requestBlock = parseRequestSecurityLine(text);
  if (requestBlock !== null) {
    return sourceBlock(base, "request", `跨周期 ${readParamString(requestBlock, "name")}`, readParamString(requestBlock, "expression"), {
      type: "instruction",
      block: requestBlock,
    });
  }
  const orderBlock = parseOrderLine(text);
  if (orderBlock !== null) {
    return sourceBlock(base, "order", orderLabelFromBlock(orderBlock.kind), readParamString(orderBlock, "id"), {
      type: "instruction",
      block: orderBlock,
    });
  }
  const visualBlock = parseVisualLine(text);
  if (visualBlock !== null) {
    return sourceBlock(base, "visual", visualLabelFromBlock(visualBlock.kind), text, {
      type: "instruction",
      block: visualBlock,
    });
  }
  const logBlock = parseLogLine(text);
  if (logBlock !== null) {
    return sourceBlock(base, "log", "运行日志", readParamString(logBlock, "message"), {
      type: "instruction",
      block: logBlock,
    });
  }
  const collectionBlock = parseCollectionLine(text);
  if (collectionBlock !== null) {
    return sourceBlock(base, "collection", "集合操作", text, { type: "instruction", block: collectionBlock });
  }
  const indexedRawCall = classifyIndexedRawCall(text);
  if (indexedRawCall !== null) {
    return sourceBlock(base, indexedRawCall.kind, indexedRawCall.label, indexedRawCall.detail, { type: "raw" });
  }
  const indexedObject = classifyIndexedRawObject(text);
  if (indexedObject !== null) {
    return sourceBlock(base, indexedObject.kind, indexedObject.label, indexedObject.detail, { type: "raw" });
  }
  const indexedDeclaration = classifyIndexedRawDeclaration(text);
  if (indexedDeclaration !== null) {
    return sourceBlock(base, indexedDeclaration.kind, indexedDeclaration.label, indexedDeclaration.detail, { type: "raw" });
  }
  const functionMatch = text.match(/^(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)\s*=>\s*(.*)$/);
  if (functionMatch) {
    return sourceBlock(base, "function", `${text.startsWith("export ") ? "导出函数" : "函数"} ${functionMatch[1]}`, trimDetail(functionMatch[3] ?? functionMatch[2] ?? "", text), { type: "raw" });
  }
  const assignmentBlock = parseAssignmentLine(text);
  if (assignmentBlock !== null) {
    return sourceBlock(base, assignmentBlock.kind === "var_state" ? "assignment" : "assignment", readParamString(assignmentBlock, "name"), readParamString(assignmentBlock, assignmentBlock.kind === "var_state" ? "initial" : "expression"), {
      type: "instruction",
      block: assignmentBlock,
    });
  }
  return sourceBlock(base, "raw", "Raw Pine", text, { type: "raw" });
}

function sourceBlock(
  base: Omit<PineSourceBlock, "kind" | "label" | "detail" | "match">,
  kind: PineSourceStructureKind,
  label: string,
  detail: string,
  match: PineSourceBlockMatch,
): PineSourceBlock {
  return {
    ...base,
    kind,
    label,
    detail: trimDetail(detail, base.raw.trim()),
    match,
  };
}

function sourceBlockCanHaveChildren(block: PineSourceBlock): boolean {
  return block.kind === "condition" ||
    block.kind === "branch" ||
    block.kind === "loop" ||
    block.kind === "switch" ||
    block.kind === "type" ||
    block.kind === "method" ||
    block.kind === "function";
}

function collectWorkflowBlocks(blocks: PineSourceBlock[]): PineV6WorkflowBlock[] {
  const result: PineV6WorkflowBlock[] = [];
  for (const block of blocks) {
    if (block.kind === "version" || block.kind === "strategy" || block.kind === "input" || block.kind === "comment") {
      result.push(...collectWorkflowBlocks(block.children));
      continue;
    }
    if (block.match.type === "instruction") {
      if (block.match.block.kind === "series_assign" && readParamString(block.match.block, "name") === "barClosed") {
        result.push(...collectWorkflowBlocks(block.children));
        continue;
      }
      if (block.match.block.kind === "if" && readParamString(block.match.block, "condition") === "barClosed") {
        result.push(...collectWorkflowBlocks(block.children));
        continue;
      }
      const cloned = cloneInstructionBlock(block.match.block);
      if (cloned.kind === "if") {
        cloned.thenBlocks = collectWorkflowBlocks(block.children.filter((child) => child.kind !== "branch"));
        const elseBranch = block.children.find((child) => child.kind === "branch");
        cloned.elseBlocks = elseBranch === undefined ? [] : collectWorkflowBlocks(elseBranch.children);
      }
      result.push(cloned);
    }
  }
  return result;
}

function cloneInstructionBlock(block: PineV6WorkflowBlock): PineV6WorkflowBlock {
  return {
    ...block,
    id: createWorkflowId("block"),
    params: { ...block.params },
    ...(block.thenBlocks === undefined ? {} : { thenBlocks: block.thenBlocks.map(cloneInstructionBlock) }),
    ...(block.elseBlocks === undefined ? {} : { elseBlocks: block.elseBlocks.map(cloneInstructionBlock) }),
  };
}

function firstStrategyDeclaration(blocks: PineSourceBlock[]): Partial<PineV6WorkflowDocument["declaration"]> {
  const match = flattenSourceBlocks(blocks).find((block) => block.match.type === "strategy")?.match;
  return match?.type === "strategy" ? match.declaration : {};
}

function parseDeclarationValue(key: string, value: unknown): unknown {
  if (key === "initialCapital" || key === "pyramiding" || key === "defaultQtyValue") {
    const numeric = Number(value);
    return Number.isFinite(numeric) ? numeric : null;
  }
  if (key === "overlay" || key === "calcOnEveryTick" || key === "processOrdersOnClose") {
    return value === true || value === "true";
  }
  return value;
}

function readParamString(block: PineV6WorkflowBlock, key: string): string {
  return readString(block.params[key]);
}

function readString(value: unknown): string {
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  return typeof value === "string" ? value.trim() : "";
}
