import { describe, expect, it } from "vitest";

import {
  createDoubleMovingAverageStrategyVisualModel,
  fromStrategyCanvasGraphData,
  fromLogicFlowGraphData,
  getStrategyBlockKind,
  getVisualBlockCapability,
  summarizePineBlockSupport,
  toStrategyCanvasGraphData,
  toLogicFlowGraphData,
} from "../src/features/strategyVisualBuilder";
import {
  STRATEGY_LOGIC_FLOW_CARD_NODE_TYPE,
  STRATEGY_LOGIC_FLOW_CONDITION_NODE_TYPE,
} from "../src/features/strategyVisualBuilderNodePresentation";

describe("strategyVisualBuilderGraphData", () => {
  it("normalizes freshly created get-technical-indicator nodes from graph data", () => {
    const model = fromLogicFlowGraphData({
      nodes: [
        {
          id: "indicator-get-node",
          type: "rect",
          x: 420,
          y: 260,
          text: "",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "movingAverage",
          },
        },
      ],
      edges: [],
    });

    const node = model.nodes[0];
    expect(node).toBeDefined();
    expect(getStrategyBlockKind(node)).toBe("getTechnicalIndicator");
    expect(node?.text).toBe("获取 均线 MA 20");
    expect(node?.properties.indicatorType).toBe("movingAverage");
    expect(node?.properties.movingAverageType).toBe("MA");
    expect(node?.properties.windowSize).toBe(20);
  });

  it("normalizes freshly created technical-indicator-condition nodes from graph data", () => {
    const model = fromLogicFlowGraphData({
      nodes: [
        {
          id: "indicator-condition-node",
          type: "diamond",
          x: 420,
          y: 260,
          text: "",
          properties: {
            blockKind: "technicalIndicatorCondition",
            indicatorType: "atr",
            conditionMode: "pattern",
          },
        },
      ],
      edges: [],
    });

    const node = model.nodes[0];
    expect(node).toBeDefined();
    expect(getStrategyBlockKind(node)).toBe("technicalIndicatorCondition");
    expect(node?.text).toBe("ATR < 1");
    expect(node?.properties.indicatorType).toBe("atr");
    expect(node?.properties.conditionMode).toBe("numeric");
    expect(node?.properties.operator).toBe("<");
    expect(node?.properties.threshold).toBe(1);
  });

  it("fills missing defaults for non-indicator palette nodes", () => {
    const model = fromLogicFlowGraphData({
      nodes: [
        {
          id: "notify-node",
          type: "rect",
          x: 420,
          y: 260,
          text: "",
          properties: {
            blockKind: "notify",
          },
        },
      ],
      edges: [],
    });

    const node = model.nodes[0];
    expect(node?.text).toBe("发送通知");
    expect(node?.properties.message).toBe("策略条件命中，准备处理后续动作");
  });

  it("maps rect and diamond nodes to html display types while keeping model types stable", () => {
    const graphData = toLogicFlowGraphData({
      engine: "logic-flow",
      version: 1,
      nodes: [
        {
          id: "getter-node",
          type: "rect",
          x: 320,
          y: 220,
          text: "获取 RSI 14",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "rsi",
            period: 14,
          },
        },
        {
          id: "condition-node",
          type: "diamond",
          x: 620,
          y: 220,
          text: "RSI < 30",
          properties: {
            blockKind: "technicalIndicatorCondition",
            indicatorType: "rsi",
            conditionMode: "numeric",
            operator: "<",
            threshold: 30,
          },
        },
      ],
      edges: [],
    });

    expect(graphData.nodes?.[0]?.type).toBe(STRATEGY_LOGIC_FLOW_CARD_NODE_TYPE);
    expect(graphData.nodes?.[1]?.type).toBe(STRATEGY_LOGIC_FLOW_CONDITION_NODE_TYPE);

    const roundTripped = fromLogicFlowGraphData(graphData);
    expect(roundTripped.nodes[0]?.type).toBe("rect");
    expect(roundTripped.nodes[1]?.type).toBe("diamond");
  });

  it("hides data edges in graph data while restoring them from condition bindings", () => {
    const graphData = toLogicFlowGraphData({
      engine: "logic-flow",
      version: 1,
      nodes: [
        {
          id: "getter-node",
          type: "rect",
          x: 320,
          y: 220,
          text: "获取 RSI 14",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "rsi",
            period: 14,
            variableName: "RSI14",
          },
        },
        {
          id: "condition-node",
          type: "diamond",
          x: 620,
          y: 220,
          text: "RSI < 30",
          properties: {
            blockKind: "technicalIndicatorCondition",
            indicatorType: "rsi",
            conditionMode: "numeric",
            operator: "<",
            threshold: 30,
            inputPrimaryNodeId: "getter-node",
          },
        },
      ],
      edges: [
        {
          id: "edge-control",
          type: "polyline",
          sourceNodeId: "getter-node",
          targetNodeId: "condition-node",
          properties: { role: "control" },
        },
        {
          id: "edge-data",
          type: "polyline",
          sourceNodeId: "getter-node",
          targetNodeId: "condition-node",
          properties: { role: "data", slot: "primary" },
        },
      ],
    });

    expect(graphData.edges).toHaveLength(1);
    expect(graphData.edges?.[0]?.properties).toMatchObject({ role: "control" });

    const roundTripped = fromLogicFlowGraphData(graphData);
    expect(roundTripped.nodes[1]?.properties.inputPrimaryNodeId).toBe("getter-node");
    expect(roundTripped.edges).toHaveLength(2);
    expect(roundTripped.edges.some((edge) => edge.properties?.role === "data" && edge.properties?.slot === "primary")).toBe(true);
  });

  it("hides getter variables from canvas graph data while preserving them on merge", () => {
    const baseModel = {
      engine: "logic-flow" as const,
      version: 1,
      nodes: [
        {
          id: "getter-node",
          type: "rect",
          x: 320,
          y: 220,
          text: "获取 RSI 14",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "rsi",
            period: 14,
            variableName: "RSI14",
          },
        },
        {
          id: "condition-node",
          type: "diamond",
          x: 620,
          y: 220,
          text: "RSI < 30",
          properties: {
            blockKind: "technicalIndicatorCondition",
            indicatorType: "rsi",
            conditionMode: "numeric",
            operator: "<",
            threshold: 30,
            inputPrimaryNodeId: "getter-node",
          },
        },
      ],
      edges: [
        {
          id: "edge-control",
          type: "polyline",
          sourceNodeId: "getter-node",
          targetNodeId: "condition-node",
          properties: { role: "control" },
        },
      ],
    };

    const canvasGraphData = toStrategyCanvasGraphData(baseModel);

    expect(canvasGraphData.nodes).toHaveLength(1);
    expect(canvasGraphData.nodes?.[0]?.id).toBe("condition-node");
    expect(canvasGraphData.edges).toHaveLength(0);

    const mergedModel = fromStrategyCanvasGraphData(canvasGraphData, baseModel);
    expect(mergedModel.nodes.some((node) => node.id === "getter-node")).toBe(true);
    expect(mergedModel.nodes.some((node) => node.id === "condition-node")).toBe(true);
    expect(mergedModel.nodes.find((node) => node.id === "condition-node")?.properties.inputPrimaryNodeId).toBe("getter-node");
  });

  it("keeps runtime support assessments out of persisted graph data", () => {
    const model = {
      engine: "logic-flow" as const,
      version: 1,
      nodes: [
        {
          id: "risk-node",
          type: "rect",
          x: 320,
          y: 220,
          text: "自动止损 1日 2%",
          properties: {
            blockKind: "stopLoss",
            mode: "stopLoss",
            direction: "auto",
            timeValue: 1,
            timeUnit: "day",
            percentage: 2,
            windowPolicy: "continuous",
          },
        },
        {
          id: "collection-stat-node",
          type: "rect",
          x: 520,
          y: 220,
          text: "集合统计",
          properties: {
            blockKind: "collectionStat",
            variableName: "range_median",
            statFunction: "median",
            sourceA: "close",
            sourceB: "open",
            sourceC: "high",
          },
        },
      ],
      edges: [],
    };

    expect(summarizePineBlockSupport(model).unsupportedConfigCount).toBe(1);
    expect(getVisualBlockCapability("stopLoss")?.controlSchema.controlIds).toContain("windowPolicy");
    expect(getVisualBlockCapability("collectionStat")?.controlSchema.controlIds).toContain("statFunction");
    const graphData = toStrategyCanvasGraphData(model);
    expect(graphData.nodes?.[0]?.properties).not.toHaveProperty("pineSupport");
    expect(graphData.nodes?.[0]?.properties).not.toHaveProperty("controlSchema");
    expect(graphData.nodes?.[0]?.properties).not.toHaveProperty("pineRenderRule");

    const roundTripped = fromStrategyCanvasGraphData(graphData, model);
    expect(roundTripped.nodes[0]?.properties).not.toHaveProperty("pineSupport");
    expect(roundTripped.nodes[0]?.properties).not.toHaveProperty("controlSchema");
    expect(roundTripped.nodes[0]?.properties).not.toHaveProperty("pineRenderRule");
  });

  it("persists structured expression AST as block properties without runtime schema metadata", () => {
    const model = {
      engine: "logic-flow" as const,
      version: 1,
      nodes: [
        {
          id: "structured-condition",
          type: "diamond",
          x: 320,
          y: 220,
          text: "结构化条件",
          properties: {
            blockKind: "seriesCondition",
            mode: "compare",
            operator: ">",
            leftExpressionAst: { kind: "reference", name: "signal" },
            rightExpressionAst: { kind: "literal", value: 0 },
          },
        },
      ],
      edges: [],
    };

    const graphData = toLogicFlowGraphData(model);

    expect(graphData.nodes?.[0]?.properties?.leftExpressionAst).toMatchObject({ kind: "reference", name: "signal" });
    expect(graphData.nodes?.[0]?.properties).not.toHaveProperty("expressionSchema");
    expect(graphData.nodes?.[0]?.properties).not.toHaveProperty("expressionReferenceOptions");

    const roundTripped = fromLogicFlowGraphData(graphData);
    expect(roundTripped.nodes[0]?.properties.leftExpressionAst).toMatchObject({ kind: "reference", name: "signal" });
  });

  it("persists vNext expression block properties without capability metadata", () => {
    const model = {
      engine: "logic-flow" as const,
      version: 1,
      nodes: [
        {
          id: "limit-order",
          type: "rect",
          x: 320,
          y: 220,
          text: "表达式订单",
          properties: {
            blockKind: "placeOrder",
            orderType: "LIMIT",
            limitPriceExpressionAst: { kind: "call", functionName: "math.max", args: [{ kind: "source", source: "close" }, { kind: "source", source: "open" }] },
          },
        },
        {
          id: "collection-stat",
          type: "rect",
          x: 560,
          y: 220,
          text: "集合统计",
          properties: {
            blockKind: "collectionStat",
            sourceAExpressionAst: { kind: "reference", name: "daily_trend" },
          },
        },
        {
          id: "time-filter",
          type: "diamond",
          x: 800,
          y: 220,
          text: "",
          properties: {
            blockKind: "timeFilter",
            mode: "dayOfWeek",
            dayOfWeek: 6,
          },
        },
        {
          id: "session-filter",
          type: "diamond",
          x: 1040,
          y: 220,
          text: "",
          properties: {
            blockKind: "sessionFilter",
            scope: "premarket",
          },
        },
      ],
      edges: [],
    };

    const graphData = toLogicFlowGraphData(model);
    expect(graphData.nodes?.[0]?.properties?.limitPriceExpressionAst).toMatchObject({ kind: "call", functionName: "math.max" });
    expect(graphData.nodes?.[1]?.properties?.sourceAExpressionAst).toMatchObject({ kind: "reference", name: "daily_trend" });
    expect(graphData.nodes?.[0]?.properties).not.toHaveProperty("controlSchema");
    expect(graphData.nodes?.[0]?.properties).not.toHaveProperty("expressionSchema");

    const roundTripped = fromLogicFlowGraphData(graphData);
    expect(roundTripped.nodes.find((node) => node.id === "time-filter")?.text).toContain("星期过滤");
    expect(roundTripped.nodes.find((node) => node.id === "session-filter")?.text).toContain("盘前");
    expect(roundTripped.nodes.find((node) => node.id === "limit-order")?.properties.limitPriceExpressionAst).toMatchObject({ kind: "call" });
    expect(roundTripped.nodes.find((node) => node.id === "collection-stat")?.properties.sourceAExpressionAst).toMatchObject({ kind: "reference" });
  });

  it("bridges control flow across hidden template getter variables without persisting synthetic edges", () => {
    const templateModel = createDoubleMovingAverageStrategyVisualModel();

    const canvasGraphData = toStrategyCanvasGraphData(templateModel);

    expect(canvasGraphData.nodes?.some((node) => node.id === "dma-fast-ma")).toBe(false);
    expect(canvasGraphData.nodes?.some((node) => node.id === "dma-slow-ma")).toBe(false);
    expect(canvasGraphData.edges?.some(
      (edge) => edge.sourceNodeId === "dma-kline-root" && edge.targetNodeId === "dma-golden-cross",
    )).toBe(true);

    const mergedModel = fromStrategyCanvasGraphData(canvasGraphData, templateModel);
    expect(mergedModel.edges.some(
      (edge) => edge.sourceNodeId === "dma-kline-root" && edge.targetNodeId === "dma-golden-cross",
    )).toBe(false);
    expect(mergedModel.edges).toHaveLength(templateModel.edges.length);
  });

  it("drops hidden control chains when the visible bridge edge is removed from canvas", () => {
    const templateModel = createDoubleMovingAverageStrategyVisualModel();
    const canvasGraphData = toStrategyCanvasGraphData(templateModel);

    const editedCanvasGraphData = {
      ...canvasGraphData,
      edges: (canvasGraphData.edges ?? []).filter(
        (edge) => !(edge.sourceNodeId === "dma-kline-root" && edge.targetNodeId === "dma-golden-cross"),
      ),
    };

    const mergedModel = fromStrategyCanvasGraphData(editedCanvasGraphData, templateModel);

    expect(mergedModel.edges.some((edge) => edge.id === "edge-dma-fast-ma")).toBe(false);
    expect(mergedModel.edges.some((edge) => edge.id === "edge-dma-slow-ma")).toBe(false);
    expect(mergedModel.edges.some((edge) => edge.id === "edge-dma-golden-cross")).toBe(false);
    expect(mergedModel.edges.some((edge) => edge.id === "edge-dma-golden-fast")).toBe(true);
    expect(mergedModel.edges.some((edge) => edge.id === "edge-dma-golden-slow")).toBe(true);
  });
});
