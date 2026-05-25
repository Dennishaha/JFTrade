import type {
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@jftrade/ui-contracts";

import {
  getStrategyBlockKind,
  type StrategyBlockKind,
} from "./strategyVisualBuilderCatalog";

export interface StrategyFlowNodeJsDoc {
  nodeId: string;
  blockKind: StrategyBlockKind;
  nodeText?: string;
  codeScope?: string;
}

const STRATEGY_FLOW_JSDOC_TAGS = {
  nodeId: "jftradeFlowNodeId",
  blockKind: "jftradeFlowBlockKind",
  nodeText: "jftradeFlowNodeText",
  codeScope: "jftradeFlowCodeScope",
} as const;

const STRATEGY_FLOW_JSDOC_TAG_NAMES = new Set<string>(
  Object.values(STRATEGY_FLOW_JSDOC_TAGS),
);

export function cloneStrategyVisualModel(
  model: StrategyVisualModelDocument | null | undefined,
): StrategyVisualModelDocument | null {
  if (model === null || model === undefined) {
    return null;
  }

  return {
    engine: model.engine,
    version: model.version,
    nodes: model.nodes.map((node) => ({
      ...node,
      properties: { ...node.properties },
    })),
    edges: model.edges.map((edge) => ({
      ...edge,
      properties:
        edge.properties === undefined ? undefined : { ...edge.properties },
    })),
  };
}

export function buildStrategyFlowNodeJsDoc(
  node: StrategyVisualNodeDocument,
  depth: number,
): string[] {
  const blockKind = getStrategyBlockKind(node);
  if (blockKind === null) {
    return [];
  }

  const indent = "  ".repeat(depth);
  const lines = [
    `${indent}/**`,
    `${indent} * @${STRATEGY_FLOW_JSDOC_TAGS.nodeId} ${sanitizeFlowTagValue(node.id)}`,
    `${indent} * @${STRATEGY_FLOW_JSDOC_TAGS.blockKind} ${blockKind}`,
  ];

  const nodeText = sanitizeFlowTagValue(node.text);
  if (nodeText !== "") {
    lines.push(
      `${indent} * @${STRATEGY_FLOW_JSDOC_TAGS.nodeText} ${nodeText}`,
    );
  }

  if (blockKind === "codeBlock") {
    const codeScope = sanitizeFlowTagValue(
      typeof node.properties.codeScope === "string"
        ? node.properties.codeScope
        : "hook",
    );
    if (codeScope !== "") {
      lines.push(
        `${indent} * @${STRATEGY_FLOW_JSDOC_TAGS.codeScope} ${codeScope}`,
      );
    }
  }

  lines.push(`${indent} */`);
  return lines;
}

export function parseStrategyFlowNodeJsDocComment(
  commentValue: string,
): StrategyFlowNodeJsDoc | null {
  const tags = new Map<string, string>();

  for (const line of commentValue.split(/\r?\n/)) {
    const normalizedLine = line.replace(/^\s*\*\s?/, "").trim();
    const match = normalizedLine.match(/^@(\S+)(?:\s+(.*))?$/);
    if (match === null) {
      continue;
    }

    const tagName = match[1];
    const rawValue = match[2] ?? "";
    if (tagName === undefined) {
      continue;
    }

    if (!STRATEGY_FLOW_JSDOC_TAG_NAMES.has(tagName)) {
      continue;
    }

    tags.set(tagName, rawValue.trim());
  }

  const nodeId = tags.get(STRATEGY_FLOW_JSDOC_TAGS.nodeId);
  const blockKind = tags.get(STRATEGY_FLOW_JSDOC_TAGS.blockKind);
  if (
    nodeId === undefined ||
    blockKind === undefined ||
    !isStrategyBlockKind(blockKind)
  ) {
    return null;
  }

  const nodeText = tags.get(STRATEGY_FLOW_JSDOC_TAGS.nodeText);
  const codeScope = tags.get(STRATEGY_FLOW_JSDOC_TAGS.codeScope);

  return {
    nodeId,
    blockKind,
    ...(nodeText !== undefined && nodeText !== "" ? { nodeText } : {}),
    ...(codeScope !== undefined && codeScope !== "" ? { codeScope } : {}),
  };
}

function sanitizeFlowTagValue(value: unknown): string {
  if (typeof value !== "string") {
    return "";
  }

  return value.replace(/\s+/g, " ").trim();
}

function isStrategyBlockKind(value: string): value is StrategyBlockKind {
  switch (value) {
    case "onInit":
    case "onKLineClosed":
    case "codeBlock":
    case "technicalIndicator":
    case "ifCloseAbove":
    case "ifCloseBelow":
    case "log":
    case "notify":
    case "placeOrder":
      return true;
    default:
      return false;
  }
}