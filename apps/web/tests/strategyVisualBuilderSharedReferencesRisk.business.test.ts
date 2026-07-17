import { describe, expect, it } from "vitest";

import type {
  StrategyVisualEdgeDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@/contracts";

import {
  readTechnicalIndicatorConditionInputBindings,
  reconcileStrategyVisualModelIndicatorBindings,
  resolveStrategyIndicatorGetterLabel,
  suggestStrategyIndicatorVariableName,
  listStrategyIndicatorGetterOptions,
} from "../src/features/strategyVisualBuilderIndicatorReferences";
import {
  nextRiskRuleNodeText,
  normalizeRiskAmountType,
  normalizeRiskRuleBlockProperties,
  normalizeRiskRuleDirection,
  normalizeRiskRuleType,
  riskAmountTypeLabel,
  riskRuleTypeLabel,
} from "../src/features/strategyVisualBuilderRiskBlock";
import {
  buildStrategyFlowNodeAnnotation,
  buildStrategyFlowNodeJsDoc,
  cloneStrategyVisualModel,
  parseStrategyFlowNodeAnnotationLines,
  parseStrategyFlowNodeJsDocComment,
} from "../src/features/strategyVisualBuilderShared";

function createNode(
  id: string,
  blockKind: string,
  text: string,
  properties: Record<string, unknown>,
): StrategyVisualNodeDocument {
  return {
    id,
    type: blockKind === "technicalIndicatorCondition" ? "diamond" : "rect",
    x: 120,
    y: 120,
    text,
    properties: { blockKind, ...properties },
  };
}

function createEdge(
  id: string,
  sourceNodeId: string,
  targetNodeId: string,
  properties?: Record<string, unknown>,
): StrategyVisualEdgeDocument {
  return {
    id,
    type: "polyline",
    sourceNodeId,
    targetNodeId,
    ...(properties === undefined ? {} : { properties }),
  };
}

describe("strategy visual builder shared/reference/risk boundaries", () => {
  it("clones and round-trips flow annotations without leaking legacy or malformed tags", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [createNode("  fast-node  ", "getTechnicalIndicator", "  EMA Fast  ", { period: 9 })],
      edges: [createEdge("edge-1", "fast-node", "condition-1", { role: "data", slot: "primary" })],
    };

    const cloned = cloneStrategyVisualModel(model);
    expect(cloned).not.toBe(model);
    expect(cloned?.nodes[0]).not.toBe(model.nodes[0]);
    expect(cloned?.edges[0]).not.toBe(model.edges[0]);

    const jsDoc = buildStrategyFlowNodeJsDoc(model.nodes[0]!, 1, {
      variableName: "  ema_fast  ",
      inputPrimaryNodeId: "  primary-node  ",
      inputFastNodeId: " fast-id ",
      inputSlowNodeId: " slow-id ",
    });
    expect(jsDoc.join("\n")).toContain("@jftradeFlowNodeId fast-node");
    expect(jsDoc.join("\n")).toContain("@jftradeFlowVariableName ema_fast");
    expect(jsDoc.join("\n")).toContain("@jftradeFlowInputSlowNodeId slow-id");

    const annotation = buildStrategyFlowNodeAnnotation(model.nodes[0]!, 0, {
      variableName: " ema_fast ",
      inputPrimaryNodeId: " primary-node ",
    });
    expect(annotation).toEqual([
      "// @jftradeFlowNodeId fast-node",
      "// @jftradeFlowBlockKind getTechnicalIndicator",
      "// @jftradeFlowNodeText EMA Fast",
      "// @jftradeFlowVariableName ema_fast",
      "// @jftradeFlowInputPrimaryNodeId primary-node",
    ]);

    expect(parseStrategyFlowNodeJsDocComment(`/**
 * @jftradeFlowNodeId fast-node
 * @jftradeFlowBlockKind getTechnicalIndicator
 * @jftradeFlowNodeText EMA Fast
 * @jftradeFlowCodeScope on_bar_closed
 * @jftradeFlowVariableName ema_fast
 * @jftradeFlowInputPrimaryNodeId primary-node
 * @unknown ignored
 */`)).toEqual({
      nodeId: "fast-node",
      blockKind: "getTechnicalIndicator",
      nodeText: "EMA Fast",
      codeScope: "on_bar_closed",
      variableName: "ema_fast",
      inputPrimaryNodeId: "primary-node",
    });

    expect(parseStrategyFlowNodeAnnotationLines([
      "# @jftradeFlowNodeId cond-1",
      "# @jftradeFlowBlockKind technicalIndicatorCondition",
      "# @jftradeFlowNodeText RSI > 50",
      "# @jftradeFlowInputPrimaryNodeId rsi-node",
    ])).toEqual({
      nodeId: "cond-1",
      blockKind: "technicalIndicatorCondition",
      nodeText: "RSI > 50",
      inputPrimaryNodeId: "rsi-node",
    });

    const unsupportedNode: StrategyVisualNodeDocument = {
      id: "legacy",
      type: "rect",
      x: 0,
      y: 0,
      text: "Legacy",
      properties: {},
    };
    expect(buildStrategyFlowNodeJsDoc(unsupportedNode, 0)).toEqual([]);
    expect(buildStrategyFlowNodeAnnotation(unsupportedNode, 0)).toEqual([]);
    expect(parseStrategyFlowNodeJsDocComment("/** @jftradeFlowBlockKind legacy */")).toBeNull();
  });

  it("reconciles indicator bindings from nodes and data edges without inventing incompatible links", () => {
    const fast = createNode("fast-ma", "getTechnicalIndicator", "获取 均线", {
      indicatorType: "movingAverage",
      movingAverageType: "EMA",
      windowSize: 9,
      variableName: " fast ema ",
      sourceRange: { start: 10, end: 20 },
    });
    const slow = createNode("slow-ma", "getTechnicalIndicator", "", {
      indicatorType: "movingAverage",
      movingAverageType: "SMA",
      windowSize: 21,
    });
    const rsi = createNode("rsi-node", "getTechnicalIndicator", "", {
      indicatorType: "rsi",
      period: 14,
      variableName: "rsi_14",
    });
    const movingAverageCondition = createNode("ma-condition", "technicalIndicatorCondition", "均线金叉", {
      indicatorType: "movingAverage",
      conditionMode: "pattern",
      patternType: "goldenCross",
      inputSlowNodeId: "slow-ma",
      inputPrimaryNodeId: "rsi-node",
    });
    const rsiCondition = createNode("rsi-condition", "technicalIndicatorCondition", "", {
      indicatorType: "rsi",
      conditionMode: "numeric",
      operator: ">",
      threshold: 50,
    });
    const audit = createNode("audit-log", "log", "审计", { message: "keep visible edges only" });

    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [fast, slow, rsi, movingAverageCondition, rsiCondition, audit],
      edges: [
        createEdge("edge-fast-existing", "fast-ma", "ma-condition", { role: "data", slot: "fast" }),
        createEdge("edge-rsi-fallback", "rsi-node", "ma-condition", { role: "data", slot: "primary" }),
        createEdge("edge-audit-data", "fast-ma", "audit-log", { role: "data", slot: "primary" }),
        {
          type: "polyline",
          sourceNodeId: "fast-ma",
          targetNodeId: "rsi-condition",
          properties: { role: "data", slot: "primary" },
        },
        createEdge("edge-rsi-primary", "rsi-node", "rsi-condition", { role: "data", slot: "primary" }),
        createEdge("edge-control", "fast-ma", "rsi-condition"),
        { id: "edge-invalid", type: "polyline", sourceNodeId: "", targetNodeId: "rsi-condition" },
      ],
    };

    expect(suggestStrategyIndicatorVariableName({ indicatorType: "movingAverage", movingAverageType: "EMA", windowSize: 20 })).toBe("EMA20");
    expect(suggestStrategyIndicatorVariableName({ indicatorType: "macd", fastPeriod: 12, slowPeriod: 26, signalPeriod: 9 })).toBe("MACD12_26_9");
    expect(suggestStrategyIndicatorVariableName({ indicatorType: "kdj", period: 9, m1: 3, m2: 3 })).toBe("KDJ9_3_3");
    expect(suggestStrategyIndicatorVariableName({ indicatorType: "bollinger", period: 20, multiplier: 2 })).toBe("BOLL20x2");
    expect(suggestStrategyIndicatorVariableName({ indicatorType: "atr", period: 14 })).toBe("ATR14");
    expect(resolveStrategyIndicatorGetterLabel(rsi)).toContain("rsi_14");
    expect(listStrategyIndicatorGetterOptions(model).map((option) => option.value)).toEqual([
      "fast-ma",
      "slow-ma",
      "rsi-node",
    ]);
    expect(listStrategyIndicatorGetterOptions(model, "movingAverage").map((option) => option.value)).toEqual(["fast-ma", "slow-ma"]);
    expect(listStrategyIndicatorGetterOptions(null)).toEqual([]);
    expect(readTechnicalIndicatorConditionInputBindings({
      indicatorType: "movingAverage",
      inputFastNodeId: "fast-ma",
      inputSlowNodeId: "slow-ma",
    })).toEqual({
      fast: "fast-ma",
      slow: "slow-ma",
    });

    const reconciled = reconcileStrategyVisualModelIndicatorBindings(model);
    const nextFast = reconciled.nodes.find((node) => node.id === "fast-ma")!;
    const nextSlow = reconciled.nodes.find((node) => node.id === "slow-ma")!;
    const nextCondition = reconciled.nodes.find((node) => node.id === "ma-condition")!;
    const nextRsiCondition = reconciled.nodes.find((node) => node.id === "rsi-condition")!;

    expect(nextFast.properties).toMatchObject({
      indicatorType: "movingAverage",
      movingAverageType: "EMA",
      variableName: "fast ema",
      sourceRange: { start: 10, end: 20 },
    });
    expect(nextSlow.text).not.toBe("");
    expect(nextCondition.properties).toMatchObject({
      indicatorType: "movingAverage",
      inputFastNodeId: "fast-ma",
      inputSlowNodeId: "slow-ma",
    });
    expect(nextCondition.properties).not.toHaveProperty("inputPrimaryNodeId");
    expect(nextRsiCondition.properties).toMatchObject({
      indicatorType: "rsi",
      inputPrimaryNodeId: "rsi-node",
    });
    expect(reconciled.edges.find((edge) => edge.id === "edge-control")).toMatchObject({
      sourceNodeId: "fast-ma",
      targetNodeId: "rsi-condition",
    });
    expect(reconciled.edges.find((edge) => edge.id === "edge-fast-existing")).toMatchObject({
      sourceNodeId: "fast-ma",
      targetNodeId: "ma-condition",
      properties: expect.objectContaining({ role: "data", slot: "fast" }),
    });
    expect(reconciled.edges.find((edge) => edge.sourceNodeId === "slow-ma" && edge.targetNodeId === "ma-condition")).toMatchObject({
      properties: expect.objectContaining({ role: "data", slot: "slow" }),
    });
    expect(reconciled.edges.find((edge) => edge.sourceNodeId === "rsi-node" && edge.targetNodeId === "ma-condition" && edge.properties?.slot === "primary")).toBeUndefined();
  });

  it("normalizes risk-rule blocks to backend-safe values and operator-readable labels", () => {
    expect(normalizeRiskRuleType("allowEntryIn")).toBe("allowEntryIn");
    expect(normalizeRiskRuleType("bad")).toBe("maxDrawdown");
    expect(normalizeRiskAmountType("strategy.cash")).toBe("strategy.cash");
    expect(normalizeRiskAmountType("bad")).toBe("strategy.percent_of_equity");
    expect(normalizeRiskRuleDirection("long")).toBe("long");
    expect(normalizeRiskRuleDirection("short")).toBe("short");
    expect(normalizeRiskRuleDirection("bad")).toBe("all");

    expect(normalizeRiskRuleBlockProperties({
      riskRuleType: "allowEntryIn",
      riskAllowedDirection: "short",
      riskValue: "bad",
      riskAmountType: "strategy.cash",
      riskCount: "0",
      riskContracts: "2.5",
      alert_message: "  stop now  ",
    })).toEqual({
      blockKind: "riskRule",
      riskRuleType: "allowEntryIn",
      riskAllowedDirection: "short",
      riskValue: 10,
      riskAmountType: "strategy.cash",
      riskCount: 1,
      riskContracts: 2.5,
      alert_message: "stop now",
    });

    expect(riskRuleTypeLabel("maxIntradayLoss")).toBe("日内最大亏损");
    expect(riskAmountTypeLabel("strategy.cash")).toBe("现金金额");
    expect(nextRiskRuleNodeText({ riskRuleType: "allowEntryIn", riskAllowedDirection: "all" })).toBe("允许入场 · 多空都可");
    expect(nextRiskRuleNodeText({ riskRuleType: "allowEntryIn", riskAllowedDirection: "long" })).toBe("允许入场 · 仅多头");
    expect(nextRiskRuleNodeText({ riskRuleType: "maxPositionSize", riskContracts: 5 })).toBe("最大持仓 · 5");
    expect(nextRiskRuleNodeText({ riskRuleType: "maxIntradayFilledOrders", riskCount: 4 })).toBe("日内成交上限 · 4");
    expect(nextRiskRuleNodeText({ riskRuleType: "maxConsLossDays", riskCount: 2 })).toBe("连续亏损天数 · 2");
    expect(nextRiskRuleNodeText({ riskRuleType: "maxIntradayLoss", riskValue: 6, riskAmountType: "strategy.cash" })).toBe("日内最大亏损 · 6 现金");
    expect(nextRiskRuleNodeText({ riskRuleType: "maxDrawdown", riskValue: 12, riskAmountType: "strategy.percent_of_equity" })).toBe("最大回撤 · 12%");
  });
});
