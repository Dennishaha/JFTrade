import type { StrategyVisualEdgeDocument } from "@/contracts";

import type { TechnicalIndicatorInputSlot } from "./strategyVisualBuilderIndicatorBlock";

export type StrategyVisualEdgeRole = "control" | "data";
export type StrategyVisualEdgeBranch = "true" | "false";

export function readStrategyVisualEdgeRole(
  edge: StrategyVisualEdgeDocument,
): StrategyVisualEdgeRole {
  return edge.properties?.role === "data" ? "data" : "control";
}

export function isStrategyVisualDataEdge(
  edge: StrategyVisualEdgeDocument,
): boolean {
  return readStrategyVisualEdgeRole(edge) === "data";
}

export function isStrategyVisualControlEdge(
  edge: StrategyVisualEdgeDocument,
): boolean {
  return readStrategyVisualEdgeRole(edge) === "control";
}

export function readStrategyVisualEdgeBranch(
  edge: StrategyVisualEdgeDocument,
): StrategyVisualEdgeBranch | null {
  const branch = edge.properties?.branch;
  return branch === "true" || branch === "false" ? branch : null;
}

export function readStrategyVisualEdgeInputSlot(
  edge: StrategyVisualEdgeDocument,
): TechnicalIndicatorInputSlot | null {
  const slot = edge.properties?.slot;
  return slot === "primary" || slot === "fast" || slot === "slow"
    ? slot
    : null;
}

export function normalizeStrategyVisualEdgeProperties(
  properties: Record<string, unknown> | undefined,
): Record<string, unknown> | undefined {
  if (properties === undefined) {
    return undefined;
  }

  const role = properties.role === "data" ? "data" : "control";
  if (role === "data") {
    const slot = properties.slot;
    return {
      role,
      ...(slot === "primary" || slot === "fast" || slot === "slow"
        ? { slot }
        : {}),
    };
  }

  const branch = properties.branch;
  return {
    role,
    ...(branch === "true" || branch === "false" ? { branch } : {}),
  };
}

export function buildStrategyVisualControlEdgeProperties(
  branch?: StrategyVisualEdgeBranch,
): Record<string, unknown> {
  return branch === undefined
    ? { role: "control" }
    : { role: "control", branch };
}

export function buildStrategyVisualDataEdgeProperties(
  slot: TechnicalIndicatorInputSlot,
): Record<string, unknown> {
  return {
    role: "data",
    slot,
  };
}