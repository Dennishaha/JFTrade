import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

import { buildStrategyPineFromVisualModel } from "../src/features/strategyVisualBuilderPine";
import { buildStrategyVisualModelFromPine } from "../src/features/strategyVisualBuilderPineParser";

interface SharedStructureCorpusCase {
  id: string;
  dimensions: string[];
  source: string;
  expectedStatementKinds: Record<string, number>;
  expectedVisualBlockKinds: Record<string, number>;
  expectedVisualSemantics: string[];
  expectedBranches: Record<"true" | "false", number>;
  expectedBranchTargets: Record<string, number>;
  expectedTradeSemantics: string[];
  expectedMaxIfDepth: number;
}

const corpus = JSON.parse(
  readFileSync(
    new URL("../../../tests/fixtures/pine-structure-corpus.json", import.meta.url),
    "utf8",
  ),
) as SharedStructureCorpusCase[];

describe("shared Pine structure corpus", () => {
  it("covers the stable reversible Pine business surface", () => {
    expect(new Set(corpus.map((corpusCase) => corpusCase.id)).size).toBe(corpus.length);
    const dimensions = new Set(corpus.flatMap((corpusCase) => corpusCase.dimensions));
    for (const dimension of [
      "input",
      "state",
      "mtf",
      "nested-if",
      "else",
      "notification",
      "risk-metadata",
    ]) {
      expect(dimensions, dimension).toContain(dimension);
    }

    const visualKinds = new Set(
      corpus.flatMap((corpusCase) => Object.keys(corpusCase.expectedVisualBlockKinds)),
    );
    for (const kind of [
      "strategyInput",
      "stateVariable",
      "stateUpdate",
      "derivedSeries",
      "mtfSeries",
      "collectionStat",
      "getTechnicalIndicator",
      "technicalIndicatorCondition",
      "seriesCondition",
      "timeFilter",
      "sessionFilter",
      "ifCloseAbove",
      "ifCloseBelow",
      "placeOrder",
      "riskRule",
      "stopLoss",
      "log",
      "notify",
    ]) {
      expect(visualKinds, kind).toContain(kind);
    }

    const statementKinds = new Set(
      corpus.flatMap((corpusCase) => Object.keys(corpusCase.expectedStatementKinds)),
    );
    for (const kind of ["let", "if", "log", "notify", "order", "exit", "cancel"]) {
      expect(statementKinds, kind).toContain(kind);
    }

    const semanticSignatures = new Set(
      corpus.flatMap((corpusCase) => corpusCase.expectedVisualSemantics),
    );
    for (const signature of [
      "collectionStat:median",
      "placeOrder:cancel",
      "riskRule:maxDrawdown",
      "sessionFilter:market",
      "stateUpdate",
      "stopLoss:bracketExit",
      "timeFilter:between",
    ]) {
      expect(semanticSignatures, signature).toContain(signature);
    }
  });

  for (const corpusCase of corpus) {
    it(`matches the visual parser structure for ${corpusCase.id}`, () => {
      const parsed = buildStrategyVisualModelFromPine(corpusCase.source);
      expect(parsed.ok, corpusCase.id).toBe(true);
      if (!parsed.ok) {
        return;
      }

      assertVisualModelMatchesCorpusCase(parsed.model, corpusCase);

      const roundTripSource = buildStrategyPineFromVisualModel(parsed.model, {
        name: `Shared corpus ${corpusCase.id}`,
      });
      const roundTrip = buildStrategyVisualModelFromPine(roundTripSource);
      expect(roundTrip.ok, `${corpusCase.id} roundtrip`).toBe(true);
      if (roundTrip.ok) {
        assertVisualModelMatchesCorpusCase(roundTrip.model, corpusCase);
      }
    });
  }
});

function assertVisualModelMatchesCorpusCase(
  model: Parameters<typeof summarizeVisualControlFlow>[0],
  corpusCase: SharedStructureCorpusCase,
): void {
  const actualStatementKinds: Record<string, number> = {};
  const actualVisualKinds: Record<string, number> = {};
  for (const node of model.nodes) {
    const visualKind = String(node.properties.blockKind);
    if (visualKind !== "onKLineClosed") {
      actualVisualKinds[visualKind] = (actualVisualKinds[visualKind] ?? 0) + 1;
    }
    const statementKind = statementKindForVisualNode(node.properties);
    if (statementKind !== null) {
      actualStatementKinds[statementKind] = (actualStatementKinds[statementKind] ?? 0) + 1;
    }
  }
  expect(actualStatementKinds).toEqual(corpusCase.expectedStatementKinds);
  expect(actualVisualKinds).toEqual(corpusCase.expectedVisualBlockKinds);
  expect(
    model.nodes
      .filter((node) => node.properties.blockKind !== "onKLineClosed")
      .map((node) => visualSemanticSignature(node.properties))
      .sort(),
  ).toEqual(corpusCase.expectedVisualSemantics);
  expect(summarizeVisualTradeSemantics(model)).toEqual(corpusCase.expectedTradeSemantics);

  const controlSummary = summarizeVisualControlFlow(model);
  expect(controlSummary.branches).toEqual(corpusCase.expectedBranches);
  expect(controlSummary.branchTargets).toEqual(corpusCase.expectedBranchTargets);
  expect(controlSummary.maxIfDepth).toBe(corpusCase.expectedMaxIfDepth);
}

function statementKindForVisualNode(properties: Record<string, unknown>): string | null {
  switch (properties.blockKind) {
    case "onKLineClosed":
    case "riskRule":
      return null;
    case "strategyInput":
    case "stateVariable":
    case "stateUpdate":
    case "derivedSeries":
    case "getTechnicalIndicator":
    case "mtfSeries":
    case "collectionStat":
      return "let";
    case "technicalIndicatorCondition":
    case "seriesCondition":
    case "timeFilter":
    case "sessionFilter":
    case "ifCloseAbove":
    case "ifCloseBelow":
      return "if";
    case "placeOrder":
      return String(properties.pineOrderFunction).startsWith("strategy.cancel")
        ? "cancel"
        : "order";
    case "stopLoss":
      return "exit";
    case "log":
      return "log";
    case "notify":
      return "notify";
    default:
      throw new Error(`shared Pine corpus produced unmapped visual block: ${String(properties.blockKind)}`);
  }
}

function visualSemanticSignature(properties: Record<string, unknown>): string {
  const kind = String(properties.blockKind);
  switch (kind) {
    case "strategyInput":
      return `${kind}:${String(properties.inputType)}`;
    case "stateVariable":
      return `${kind}:${String(properties.valueType)}`;
    case "derivedSeries":
      return `${kind}:${String(properties.mode)}`;
    case "mtfSeries":
      return `${kind}:${String(properties.expressionType)}`;
    case "collectionStat":
      return `${kind}:${String(properties.statFunction)}`;
    case "getTechnicalIndicator":
      return properties.indicatorType === "movingAverage"
        ? `${kind}:movingAverage:${String(properties.movingAverageType)}`
        : `${kind}:${String(properties.indicatorType)}`;
    case "technicalIndicatorCondition":
      return properties.conditionMode === "pattern"
        ? `${kind}:pattern:${String(properties.patternType)}`
        : `${kind}:${String(properties.conditionMode)}`;
    case "seriesCondition":
      return `${kind}:${String(properties.mode)}`;
    case "timeFilter":
      return `${kind}:${String(properties.mode)}`;
    case "sessionFilter":
      return `${kind}:${String(properties.scope)}`;
    case "placeOrder":
      return `${kind}:${String(properties.orderAction)}`;
    case "riskRule":
      return `${kind}:${String(properties.riskRuleType)}`;
    case "stopLoss":
      return `${kind}:${String(properties.mode)}`;
    case "stateUpdate":
    case "ifCloseAbove":
    case "ifCloseBelow":
    case "log":
    case "notify":
      return kind;
    default:
      throw new Error(`shared Pine corpus has no semantic signature for ${kind}`);
  }
}

function summarizeVisualControlFlow(model: {
  nodes: Array<{ id: string; properties: Record<string, unknown> }>;
  edges: Array<{
    sourceNodeId: string;
    targetNodeId: string;
    properties?: Record<string, unknown>;
  }>;
}): {
  branches: Record<"true" | "false", number>;
  branchTargets: Record<string, number>;
  maxIfDepth: number;
} {
  const branches = { true: 0, false: 0 };
  const branchTargets: Record<string, number> = {};
  const nodesByID = new Map(model.nodes.map((node) => [node.id, node]));
  const conditionIDs = new Set(
    model.nodes
      .filter((node) => isVisualCondition(node.properties.blockKind))
      .map((node) => node.id),
  );
  const nestedConditionEdges = new Map<string, string[]>();
  for (const edge of model.edges) {
    const branch = edge.properties?.branch;
    if (branch === "true" || branch === "false") {
      branches[branch] += 1;
      const target = nodesByID.get(edge.targetNodeId);
      const targetKind = target === undefined ? null : statementKindForVisualNode(target.properties);
      if (targetKind === null) {
        throw new Error(`shared Pine corpus branch targets non-statement node ${edge.targetNodeId}`);
      }
      const signature = `${branch}:${targetKind}`;
      branchTargets[signature] = (branchTargets[signature] ?? 0) + 1;
      if (conditionIDs.has(edge.sourceNodeId) && conditionIDs.has(edge.targetNodeId)) {
        const children = nestedConditionEdges.get(edge.sourceNodeId) ?? [];
        children.push(edge.targetNodeId);
        nestedConditionEdges.set(edge.sourceNodeId, children);
      }
    }
  }

  const nestedTargets = new Set([...nestedConditionEdges.values()].flat());
  const roots = [...conditionIDs].filter((id) => !nestedTargets.has(id));
  const depthFrom = (id: string, seen: Set<string>): number => {
    if (seen.has(id)) {
      throw new Error(`shared Pine corpus produced a condition cycle at ${id}`);
    }
    const nextSeen = new Set(seen).add(id);
    return 1 + Math.max(
      0,
      ...(nestedConditionEdges.get(id) ?? []).map((child) => depthFrom(child, nextSeen)),
    );
  };
  return {
    branches,
    branchTargets,
    maxIfDepth: Math.max(0, ...roots.map((id) => depthFrom(id, new Set()))),
  };
}

function summarizeVisualTradeSemantics(model: {
  nodes: Array<{ properties: Record<string, unknown> }>;
}): string[] {
  const semantics: string[] = [];
  for (const { properties } of model.nodes) {
    if (properties.blockKind === "placeOrder") {
      const action = String(properties.orderAction);
      if (action === "cancel" || action === "cancelAll") {
        semantics.push(`cancel:${action === "cancelAll" ? "*" : tradeField(properties.orderId)}`);
        continue;
      }
      const intent = visualOrderIntent(action);
      semantics.push(
        `order:${intent}:${tradeField(properties.orderId)}:${visualOrderAction(intent, properties.side)}:` +
          `${visualQuantityMode(intent, properties.quantityMode)}:${tradeField(properties.quantityValue)}:` +
          `limit=${hasVisualOrderExpression(properties, "limit")}:` +
          `stop=${hasVisualOrderExpression(properties, "stop")}:` +
          `immediate=${properties.immediately === true}`,
      );
    } else if (properties.blockKind === "stopLoss") {
      const mode = String(properties.mode);
      semantics.push(
        `exit:${tradeField(properties.fromEntryId)}:${tradeField(properties.direction)}:` +
          `symbol_position_percent:${tradeField(properties.quantityPercentage)}:` +
          `stop=${properties.stopPriceExpressionAst !== undefined}:` +
          `limit=${properties.takeProfitPriceExpressionAst !== undefined}:` +
          `profit=${properties.profitTicks !== undefined}:loss=${properties.lossTicks !== undefined}:` +
          `trailPoints=${mode === "trailingStop" && properties.trailingPriceMode !== "price"}:` +
          `trailPrice=${mode === "trailingStop" && properties.trailingPriceMode === "price"}:` +
          `trailOffset=${mode === "trailingStop" && properties.trailingOffsetExpressionAst !== undefined}`,
      );
    } else if (properties.blockKind === "riskRule") {
      if (properties.riskRuleType === "allowEntryIn") {
        semantics.push(`risk:allowEntryIn:${tradeField(properties.riskAllowedDirection)}`);
      } else if (properties.riskRuleType === "maxDrawdown") {
        semantics.push(
          `risk:maxDrawdown:${tradeField(properties.riskValue)}:` +
            `${String(properties.riskAmountType).replace("strategy.", "")}:` +
            `${tradeField(properties.alert_message)}`,
        );
      }
    }
  }
  return semantics.sort();
}

function visualOrderIntent(action: string): "entry" | "net" | "close" | "flatten" {
  if (action === "order") return "net";
  if (action === "close") return "close";
  if (action === "closeAll") return "flatten";
  return "entry";
}

function visualOrderAction(intent: string, side: unknown): string {
  if (intent === "flatten") return "-";
  if (side === "SELL_SHORT") return "short";
  if (side === "BUY_COVER") return "cover";
  return side === "SELL" ? "sell" : "buy";
}

function visualQuantityMode(intent: string, mode: unknown): string {
  if (intent === "flatten") return "symbol_position_percent";
  if (mode !== "equityPercent") return tradeField(mode);
  return intent === "entry" || intent === "net"
    ? "account_position_percent"
    : "symbol_position_percent";
}

function hasVisualOrderExpression(
  properties: Record<string, unknown>,
  kind: "limit" | "stop",
): boolean {
  const numericValue = Number(properties[`${kind}Price`]);
  return properties[`${kind}PriceExpressionAst`] !== undefined ||
    (Number.isFinite(numericValue) && numericValue !== 0);
}

function tradeField(value: unknown): string {
  const normalized = value === undefined || value === null ? "" : String(value).trim();
  return normalized === "" ? "-" : normalized;
}

function isVisualCondition(value: unknown): boolean {
  switch (value) {
    case "technicalIndicatorCondition":
    case "seriesCondition":
    case "timeFilter":
    case "sessionFilter":
    case "ifCloseAbove":
    case "ifCloseBelow":
      return true;
    default:
      return false;
  }
}
