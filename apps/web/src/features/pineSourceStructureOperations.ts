import type { PineV6WorkflowBlockKind } from "@/contracts";

import type { PineSourceBlock, PineSourceEditResult } from "./pineSourceStructureIndex";
import { renderDefaultSourceBlock } from "./pineSourceStructureRender";

export interface PineSourceOperationContext {
  parseSourceToBlocks(source: string): PineSourceBlock[];
  flattenSourceBlocks(blocks: PineSourceBlock[]): PineSourceBlock[];
  replaceSourceRange(source: string, range: { start: number; end: number }, nextText: string): string;
}

export function insertSourceBlock(
  context: PineSourceOperationContext,
  source: string,
  selectedBlockId: string | null,
  kind: PineV6WorkflowBlockKind,
): PineSourceEditResult {
  const roots = context.parseSourceToBlocks(source);
  const selected = selectedBlockId === null ? null : findBlockById(roots, selectedBlockId);
  const located = selectedBlockId === null ? null : findBlockWithSiblings(roots, selectedBlockId);
  const scope = selected === null ? findBarClosedScope(context, roots) : resolveInsertionScope(selected, located);
  const indent = scope.indent;
  const insertionOffset = afterLineEndOffset(source, scope.insertOffset);
  const text = renderDefaultSourceBlock(kind, indent);
  const prefix = source.length === 0 || insertionOffset === 0 || source.slice(0, insertionOffset).endsWith("\n") ? "" : "\n";
  const suffix = insertionOffset >= source.length || source.slice(insertionOffset - 1, insertionOffset) === "\n" ? "\n" : "";
  return {
    source: `${source.slice(0, insertionOffset)}${prefix}${text}${suffix}${source.slice(insertionOffset)}`,
    changed: true,
  };
}

export function deleteSourceBlock(
  context: PineSourceOperationContext,
  source: string,
  blockId: string,
): PineSourceEditResult {
  const block = findBlockById(context.parseSourceToBlocks(source), blockId);
  if (block === null) {
    return { source, changed: false };
  }
  return {
    source: context.replaceSourceRange(source, editableSourceRange(source, block), ""),
    changed: true,
  };
}

export function duplicateSourceBlock(
  context: PineSourceOperationContext,
  source: string,
  blockId: string,
): PineSourceEditResult {
  const block = findBlockById(context.parseSourceToBlocks(source), blockId);
  if (block === null) {
    return { source, changed: false };
  }
  const range = editableSourceRange(source, block);
  const text = source.slice(range.start, range.end);
  return {
    source: `${source.slice(0, range.end)}${text}${source.slice(range.end)}`,
    changed: true,
  };
}

export function moveSourceBlock(
  context: PineSourceOperationContext,
  source: string,
  blockId: string,
  direction: -1 | 1,
): PineSourceEditResult {
  const tree = context.parseSourceToBlocks(source);
  const located = findBlockWithSiblings(tree, blockId);
  if (located === null) {
    return { source, changed: false };
  }
  const targetIndex = located.index + direction;
  const target = located.siblings[targetIndex];
  if (target === undefined) {
    return { source, changed: false };
  }
  const blockRange = editableSourceRange(source, located.block);
  const targetRange = editableSourceRange(source, target);
  if (direction < 0) {
    return {
      source: `${source.slice(0, targetRange.start)}${source.slice(blockRange.start, blockRange.end)}${source.slice(targetRange.end, blockRange.start)}${source.slice(targetRange.start, targetRange.end)}${source.slice(blockRange.end)}`,
      changed: true,
    };
  }
  return {
    source: `${source.slice(0, blockRange.start)}${source.slice(targetRange.start, targetRange.end)}${source.slice(blockRange.end, targetRange.start)}${source.slice(blockRange.start, blockRange.end)}${source.slice(targetRange.end)}`,
    changed: true,
  };
}

export function replaceSourceBlockKind(
  context: PineSourceOperationContext,
  source: string,
  blockId: string,
  kind: PineV6WorkflowBlockKind,
): PineSourceEditResult {
  const block = findBlockById(context.parseSourceToBlocks(source), blockId);
  if (block === null) {
    return { source, changed: false };
  }
  const range = editableSourceRange(source, block);
  const replacement = `${renderDefaultSourceBlock(kind, block.depth)}${source.slice(range.end - 1, range.end) === "\n" ? "\n" : ""}`;
  return {
    source: context.replaceSourceRange(source, range, replacement),
    changed: true,
  };
}

function findBlockById(blocks: PineSourceBlock[], blockId: string): PineSourceBlock | null {
  for (const block of blocks) {
    if (block.id === blockId) {
      return block;
    }
    const child = findBlockById(block.children, blockId);
    if (child !== null) {
      return child;
    }
  }
  return null;
}

function findBlockWithSiblings(
  blocks: PineSourceBlock[],
  blockId: string,
): { block: PineSourceBlock; siblings: PineSourceBlock[]; index: number } | null {
  const index = blocks.findIndex((block) => block.id === blockId);
  if (index >= 0) {
    return { block: blocks[index]!, siblings: blocks, index };
  }
  for (const block of blocks) {
    const found = findBlockWithSiblings(block.children, blockId);
    if (found !== null) {
      return found;
    }
  }
  return null;
}

function editableSourceRange(source: string, block: PineSourceBlock): { start: number; end: number } {
  const nextNewline = source.indexOf("\n", block.sourceRange.end);
  const end = nextNewline === block.sourceRange.end ? nextNewline + 1 : block.sourceRange.end;
  return { start: block.sourceRange.start, end: Math.min(source.length, end) };
}

function afterLineEndOffset(source: string, offset: number): number {
  return source.slice(offset, offset + 1) === "\n" ? offset + 1 : offset;
}

function findBarClosedScope(
  context: PineSourceOperationContext,
  blocks: PineSourceBlock[],
): { indent: number; insertOffset: number } {
  const barClosed = context.flattenSourceBlocks(blocks).find((block) =>
    block.match.type === "instruction" &&
    block.match.block.kind === "if" &&
    readParamString(block.match.block.params.condition) === "barClosed"
  );
  if (barClosed !== undefined) {
    return blockChildScope(barClosed);
  }
  const last = blocks[blocks.length - 1];
  return { indent: 0, insertOffset: last === undefined ? 0 : editableSourceRangeFromBlockEnd(last) };
}

function resolveInsertionScope(
  block: PineSourceBlock,
  located: { block: PineSourceBlock; siblings: PineSourceBlock[]; index: number } | null,
): { indent: number; insertOffset: number } {
  if (sourceBlockCanHaveChildren(block)) {
    return blockChildScope(block);
  }
  const lastSibling = located?.siblings[located.siblings.length - 1] ?? block;
  return { indent: block.depth, insertOffset: editableSourceRangeFromBlockEnd(lastSibling) };
}

function blockChildScope(block: PineSourceBlock): { indent: number; insertOffset: number } {
  const elseBranch = block.kind === "condition" ? block.children.find((child) => child.kind === "branch") : undefined;
  const children = elseBranch === undefined ? block.children : block.children.filter((child) => child.kind !== "branch");
  const lastChild = children[children.length - 1];
  return {
    indent: block.depth + 1,
    insertOffset: lastChild === undefined ? block.sourceRange.end : editableSourceRangeFromBlockEnd(lastChild),
  };
}

function editableSourceRangeFromBlockEnd(block: PineSourceBlock): number {
  return block.sourceRange.end;
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

function readParamString(value: unknown): string {
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  return typeof value === "string" ? value.trim() : "";
}
