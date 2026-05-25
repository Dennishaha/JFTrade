import { describe, expect, it } from "vitest";

import type { StrategyVisualNodeDocument } from "@jftrade/ui-contracts";

import {
  buildStrategyScriptFromVisualModel,
  buildStrategyVisualModelFromScript,
  cloneStrategyVisualModel,
  createDefaultStrategyVisualModel,
  getStrategyAuthoringTemplates,
  getStrategyBlockKind,
} from "../src/features/strategyVisualBuilder";

describe("strategyVisualBuilderParser", () => {
  it("round-trips unified technical indicator blocks for RSI numeric conditions", () => {
    const visualModel = createDefaultStrategyVisualModel();
    visualModel.nodes.push({
      id: "unified-rsi-node",
      type: "rect",
      x: 430,
      y: 560,
      text: "RSI 14 > 70",
      properties: {
        blockKind: "technicalIndicator",
        indicatorType: "rsi",
        conditionMode: "numeric",
        operator: ">",
        threshold: 70,
        period: 14,
      },
    });
    visualModel.edges.push({
      id: "edge-unified-rsi-node",
      type: "polyline",
      sourceNodeId: "on-kline-root",
      targetNodeId: "unified-rsi-node",
    });

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: "unified-rsi",
      symbol: "00700",
      interval: "1m",
    });

    expect(script).toContain("@jftradeFlowBlockKind technicalIndicator");
    expect(script).toContain('latestRsi = ctx.indicators["rsi:14"] ?? null;');

    const parsed = buildStrategyVisualModelFromScript(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const indicatorNode = parsed.model.nodes.find((node) => node.id === "unified-rsi-node");
    expect(getStrategyBlockKind(indicatorNode)).toBe("technicalIndicator");
    expect(indicatorNode?.properties.indicatorType).toBe("rsi");
    expect(indicatorNode?.properties.period).toBe(14);
    expect(indicatorNode?.properties.operator).toBe(">");
    expect(indicatorNode?.properties.threshold).toBe(70);
  });

  it("round-trips built-in visual templates without degrading branches into code blocks", () => {
    for (const template of getStrategyAuthoringTemplates()) {
      const script = buildStrategyScriptFromVisualModel(template.visualModel, {
        name: template.defaultName,
        symbol: template.defaultSymbol,
        interval: template.defaultInterval,
      });
      const parsed = buildStrategyVisualModelFromScript(script);

      expect(parsed.ok, template.id).toBe(true);
      if (!parsed.ok) {
        continue;
      }

      expect(parsed.codeBlockCount, template.id).toBe(0);
      expect(readNonRootNodeSignatures(parsed.model.nodes), template.id).toEqual(
        readNonRootNodeSignatures(template.visualModel.nodes),
      );
      expect(readEdgeSignatures(parsed.model.nodes, parsed.model.edges), template.id).toEqual(
        readEdgeSignatures(template.visualModel.nodes, template.visualModel.edges),
      );
    }
  });

  it("parses a raw console.log expression back into a visual log block", () => {
    const script = [
      "function onKLineClosed(ctx) {",
      "  console.log(ctx.kline.close);",
      "}",
    ].join("\n");
    const parsed = buildStrategyVisualModelFromScript(script);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const logNode = parsed.model.nodes.find((node) => getStrategyBlockKind(node) === "log");
    expect(logNode).toBeDefined();
    expect(logNode?.properties.message).toBe("${ctx.kline.close}");
    expect(readNodeSourceRange(logNode)).toEqual({
      start: script.indexOf("console.log(ctx.kline.close);"),
      end: script.indexOf("console.log(ctx.kline.close);") + "console.log(ctx.kline.close);".length,
    });
  });

  it("preserves renamed visual block titles and ids via flow JSDoc", () => {
    const template = getStrategyAuthoringTemplates().find((item) =>
      item.visualModel.nodes.some(
        (node) => getStrategyBlockKind(node) === "technicalIndicator",
      ),
    );
    expect(template).toBeDefined();
    if (template === undefined) {
      return;
    }

    const visualModel = cloneStrategyVisualModel(template.visualModel);
    expect(visualModel).not.toBeNull();
    if (visualModel === null) {
      return;
    }

    const renamedNode = visualModel.nodes.find(
      (node) => getStrategyBlockKind(node) === "technicalIndicator",
    );
    expect(renamedNode).toBeDefined();
    if (renamedNode === undefined) {
      return;
    }

    renamedNode.text = "主信号入口";

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: template.defaultName,
      symbol: template.defaultSymbol,
      interval: template.defaultInterval,
    });
    expect(script).toContain("@jftradeFlowNodeText 主信号入口");
    expect(script).toContain(`@jftradeFlowNodeId ${renamedNode.id}`);

    const parsed = buildStrategyVisualModelFromScript(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const roundTrippedNode = parsed.model.nodes.find(
      (node) => node.id === renamedNode.id,
    );
    expect(roundTrippedNode?.text).toBe("主信号入口");
    expect(getStrategyBlockKind(roundTrippedNode)).toBe("technicalIndicator");
  });

  it("keeps a multi-statement code block as one renamed flow node", () => {
    const visualModel = createDefaultStrategyVisualModel();
    visualModel.nodes.push({
      id: "hook-code-block",
      type: "rect",
      x: 440,
      y: 560,
      text: "自定义执行段",
      properties: {
        blockKind: "codeBlock",
        codeScope: "hook",
        code: [
          "const price = ctx.kline.close;",
          "console.log(price);",
          "notify(`price: ${price}`);",
        ].join("\n"),
      },
    });
    visualModel.edges.push({
      id: "hook-code-edge",
      type: "polyline",
      sourceNodeId: "on-kline-root",
      targetNodeId: "hook-code-block",
    });

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: "mixed-code",
      symbol: "00700",
      interval: "1m",
    });
    expect(script).toContain("@jftradeFlowBlockKind codeBlock");

    const parsed = buildStrategyVisualModelFromScript(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const codeBlocks = parsed.model.nodes.filter(
      (node) => getStrategyBlockKind(node) === "codeBlock",
    );
    expect(codeBlocks).toHaveLength(1);
    expect(codeBlocks[0]?.id).toBe("hook-code-block");
    expect(codeBlocks[0]?.text).toBe("自定义执行段");
    expect(codeBlocks[0]?.properties.code).toBe([
      "const price = ctx.kline.close;",
      "console.log(price);",
      "notify(`price: ${price}`);",
    ].join("\n"));
  });

  it("recovers block kind from flow JSDoc when generated code is altered beyond pattern recognition", () => {
    const visualModel = createDefaultStrategyVisualModel();
    visualModel.nodes.push({
      id: "rsi-fallback-node",
      type: "rect",
      x: 440,
      y: 560,
      text: "自定义RSI",
      properties: {
        blockKind: "technicalIndicator",
        indicatorType: "rsi",
        conditionMode: "numeric",
        operator: "<",
        threshold: 30,
        period: 21,
      },
    });
    visualModel.edges.push({
      id: "rsi-fallback-edge",
      type: "polyline",
      sourceNodeId: "on-kline-root",
      targetNodeId: "rsi-fallback-node",
    });

    let script = buildStrategyScriptFromVisualModel(visualModel, {
      name: "fallback-test",
      symbol: "00700",
      interval: "1m",
    });

    expect(script).toContain("@jftradeFlowBlockKind technicalIndicator");
    expect(script).toContain("@jftradeFlowNodeText 自定义RSI");

    script = script.replace(
      'latestRsi = ctx.indicators["rsi:21"] ?? null;',
      "latestRsi = customRsiCalc(ctx.indicators, 21, { smoothing: true });",
    );

    const parsed = buildStrategyVisualModelFromScript(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const recovered = parsed.model.nodes.find(
      (node) => node.id === "rsi-fallback-node",
    );
    expect(recovered).toBeDefined();
    expect(getStrategyBlockKind(recovered)).toBe("technicalIndicator");
    expect(recovered?.text).toBe("自定义RSI");
    expect(recovered?.properties.period).toBe(14);
  });

  it("preserves node x,y positions from the existing visual model when round-tripping code -> flow", () => {
    const template = getStrategyAuthoringTemplates().find((item) =>
      item.visualModel.nodes.some(
        (node) => getStrategyBlockKind(node) === "technicalIndicator",
      ),
    );
    expect(template).toBeDefined();
    if (template === undefined) {
      return;
    }

    const existingModel = cloneStrategyVisualModel(template.visualModel);
    expect(existingModel).not.toBeNull();
    if (existingModel === null) {
      return;
    }

    const customPositions = new Map<string, { x: number; y: number }>();
    for (const node of existingModel.nodes) {
      const kind = getStrategyBlockKind(node);
      if (kind === "onInit" || kind === "onKLineClosed") {
        continue;
      }
      const customX = node.x + 350;
      const customY = node.y + 200;
      customPositions.set(node.id, { x: customX, y: customY });
      node.x = customX;
      node.y = customY;
    }

    const script = buildStrategyScriptFromVisualModel(existingModel, {
      name: template.defaultName,
      symbol: template.defaultSymbol,
      interval: template.defaultInterval,
    });

    const parsed = buildStrategyVisualModelFromScript(script, existingModel);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    for (const node of parsed.model.nodes) {
      const expected = customPositions.get(node.id);
      if (expected === undefined) {
        continue;
      }
      expect(node.x, `node ${node.id} x position should be preserved`).toBe(expected.x);
      expect(node.y, `node ${node.id} y position should be preserved`).toBe(expected.y);
    }
  });

  it("chains condition body statements serially instead of making them all siblings", () => {
    const visualModel = createDefaultStrategyVisualModel();
    const conditionNode: StrategyVisualNodeDocument = {
      id: "serial-cond",
      type: "rect",
      x: 440,
      y: 300,
      text: "RSI 14 < 30",
      properties: {
        blockKind: "technicalIndicator",
        indicatorType: "rsi",
        conditionMode: "numeric",
        operator: "<",
        threshold: 30,
        period: 14,
      },
    };
    const notifyNode: StrategyVisualNodeDocument = {
      id: "serial-notify",
      type: "rect",
      x: 700,
      y: 300,
      text: "发送通知",
      properties: { blockKind: "notify", message: "RSI hit" },
    };
    const codeNode: StrategyVisualNodeDocument = {
      id: "serial-code",
      type: "rect",
      x: 960,
      y: 300,
      text: "买入",
      properties: { blockKind: "codeBlock", code: 'placeOrder({ side: "BUY", quantity: 100, orderType: "MARKET" });', codeScope: "hook" },
    };

    visualModel.nodes.push(conditionNode, notifyNode, codeNode);
    visualModel.edges.push(
      { id: "e1", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "serial-cond" },
      { id: "e2", type: "polyline", sourceNodeId: "serial-cond", targetNodeId: "serial-notify" },
      { id: "e3", type: "polyline", sourceNodeId: "serial-notify", targetNodeId: "serial-code" },
    );

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: "serial-test",
      symbol: "00700",
      interval: "1m",
    });

    const parsed = buildStrategyVisualModelFromScript(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const edges = parsed.model.edges;
    const condToNotify = edges.find(
      (e) => e.sourceNodeId === "serial-cond" && e.targetNodeId === "serial-notify",
    );
    const notifyChildren = edges.filter((e) => e.sourceNodeId === "serial-notify");

    expect(condToNotify, "condition should connect to notify").toBeDefined();
    expect(notifyChildren.length, "notify should keep serial body flow").toBeGreaterThan(0);
  });
});

function readNodeSourceRange(
  node: StrategyVisualNodeDocument | undefined,
): { start: number; end: number } | null {
  const sourceRange = node?.properties.sourceRange;
  if (
    sourceRange === undefined ||
    sourceRange === null ||
    typeof sourceRange !== "object"
  ) {
    return null;
  }

  const start = Reflect.get(sourceRange, "start");
  const end = Reflect.get(sourceRange, "end");
  if (typeof start !== "number" || typeof end !== "number") {
    return null;
  }

  return { start, end };
}

function readNonRootNodeSignatures(nodes: StrategyVisualNodeDocument[]): string[] {
  return nodes
    .filter((node) => {
      const kind = getStrategyBlockKind(node);
      return kind !== "onInit" && kind !== "onKLineClosed";
    })
    .map((node) => buildNodeSignature(node))
    .sort();
}

function readEdgeSignatures(
  nodes: StrategyVisualNodeDocument[],
  edges: Array<{ sourceNodeId: string; targetNodeId: string }>,
): string[] {
  const nodesById = new Map(nodes.map((node) => [node.id, node] as const));

  return edges
    .map((edge) => {
      const source = nodesById.get(edge.sourceNodeId);
      const target = nodesById.get(edge.targetNodeId);
      if (source === undefined || target === undefined) {
        return null;
      }
      return `${buildNodeSignature(source)}->${buildNodeSignature(target)}`;
    })
    .filter((value): value is string => value !== null)
    .sort();
}

function buildNodeSignature(node: StrategyVisualNodeDocument): string {
  const kind = getStrategyBlockKind(node) ?? "unknown";
  const properties = node.properties ?? {};

  switch (kind) {
    case "technicalIndicator":
      return [
        kind,
        String(properties.indicatorType ?? ""),
        String(properties.conditionMode ?? ""),
        String(properties.operator ?? ""),
        String(properties.threshold ?? ""),
        String(properties.patternType ?? ""),
        String(properties.lookback ?? ""),
        String(properties.period ?? ""),
        String(properties.fastPeriod ?? ""),
        String(properties.slowPeriod ?? ""),
        String(properties.signalPeriod ?? ""),
        String(properties.m1 ?? ""),
        String(properties.m2 ?? ""),
        String(properties.multiplier ?? ""),
      ].join(":");
    case "ifCloseAbove":
    case "ifCloseBelow":
      return `${kind}:${properties.threshold ?? ""}`;
    case "log":
    case "notify":
    case "codeBlock":
      return `${kind}:${String(properties.message ?? properties.code ?? "")}`;
    default:
      return `${kind}:${node.text}`;
  }
}
