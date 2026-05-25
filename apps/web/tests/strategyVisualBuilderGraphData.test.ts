import { describe, expect, it } from "vitest";

import {
  createDoubleMovingAverageStrategyVisualModel,
  fromStrategyCanvasGraphData,
  fromLogicFlowGraphData,
  getStrategyBlockKind,
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

  it("normalizes freshly created technical indicator nodes from graph data", () => {
    const model = fromLogicFlowGraphData({
      nodes: [
        {
          id: "indicator-node",
          type: "rect",
          x: 420,
          y: 260,
          text: "",
          properties: {
            blockKind: "technicalIndicator",
          },
        },
      ],
      edges: [],
    });

    const node = model.nodes[0];
    expect(node).toBeDefined();
    expect(getStrategyBlockKind(node)).toBe("technicalIndicator");
    expect(node?.text).toBe("RSI 14 < 30");
    expect(node?.properties.indicatorType).toBe("rsi");
    expect(node?.properties.conditionMode).toBe("numeric");
    expect(node?.properties.operator).toBe("<");
    expect(node?.properties.threshold).toBe(30);
    expect(node?.properties.period).toBe(14);
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