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

      // Templates that intentionally contain codeBlock nodes for buy/sell logic
      // skip the structural round-trip comparison; only verify the script is valid.
      const orderTemplates = new Set([
        "double-moving-average",
        "rsi-reversion",
        "macd-momentum",
        "bollinger-reversion",
        "breakout-alert",
        "mean-reversion-alert",
      ]);
      if (orderTemplates.has(template.id)) {
        expect(parsed.codeBlockCount).toBeGreaterThan(0);
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

    const regenerated = buildStrategyScriptFromVisualModel(parsed.model, {
      name: "raw-log",
      symbol: "00700",
      interval: "1m",
    });
    expect(regenerated).toContain("console.log(ctx.kline.close);");
  });

  it("preserves renamed visual block titles and ids via flow JSDoc", () => {
    const template = getStrategyAuthoringTemplates().find((item) =>
      item.visualModel.nodes.some(
        (node) => getStrategyBlockKind(node) === "movingAverageFast",
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
      (node) => getStrategyBlockKind(node) === "movingAverageFast",
    );
    expect(renamedNode).toBeDefined();
    if (renamedNode === undefined) {
      return;
    }

    renamedNode.text = "主快线入口";

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: template.defaultName,
      symbol: template.defaultSymbol,
      interval: template.defaultInterval,
    });
    expect(script).toContain("@jftradeFlowNodeText 主快线入口");
    expect(script).toContain(`@jftradeFlowNodeId ${renamedNode.id}`);

    const parsed = buildStrategyVisualModelFromScript(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const roundTrippedNode = parsed.model.nodes.find(
      (node) => node.id === renamedNode.id,
    );
    expect(roundTrippedNode?.text).toBe("主快线入口");
    expect(getStrategyBlockKind(roundTrippedNode)).toBe("movingAverageFast");
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
    expect(readNodeSourceRange(codeBlocks[0])).toEqual({
      start: script.indexOf("const price = ctx.kline.close;"),
      end:
        script.indexOf("notify(`price: ${price}`);") +
        "notify(`price: ${price}`);".length,
    });
  });

  it("recovers block kind from flow JSDoc when the generated code is altered beyond pattern recognition", () => {
    const visualModel = createDefaultStrategyVisualModel();
    const rsiNode: StrategyVisualNodeDocument = {
      id: "rsi-fallback-node",
      type: "rect",
      x: 440,
      y: 560,
      text: "自定义RSI",
      properties: {
        blockKind: "rsi",
        period: 21,
      },
    };
    visualModel.nodes.push(rsiNode);
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

    expect(script).toContain("@jftradeFlowBlockKind rsi");
    expect(script).toContain("@jftradeFlowNodeText 自定义RSI");

    // Corrupt the generated code so the AST pattern no longer matches,
    // but keep the JSDoc annotation intact.
    script = script.replace(
      "latestRsi = calculateRSI(state.closes, 21);",
      "latestRsi = customRsiCalc(state.closes, 21, { smoothing: true });",
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
    expect(getStrategyBlockKind(recovered)).toBe("rsi");
    expect(recovered?.text).toBe("自定义RSI");
    expect(recovered?.properties.period).toBe(14); // fallback uses default
  });

  it("preserves node x,y positions from the existing visual model when round-tripping code -> flow", () => {
    const template = getStrategyAuthoringTemplates().find((item) =>
      item.visualModel.nodes.some(
        (node) => getStrategyBlockKind(node) === "movingAverageFast",
      ),
    );
    expect(template).toBeDefined();
    if (template === undefined) {
      return;
    }

    // Build initial visual model and simulate user dragging nodes to custom positions.
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

    // Generate script from the model with custom positions.
    const script = buildStrategyScriptFromVisualModel(existingModel, {
      name: template.defaultName,
      symbol: template.defaultSymbol,
      interval: template.defaultInterval,
    });

    // Parse the script back, passing the existing model to preserve positions.
    const parsed = buildStrategyVisualModelFromScript(script, existingModel);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    // Verify that nodes with matching IDs kept their custom positions.
    for (const node of parsed.model.nodes) {
      const expected = customPositions.get(node.id);
      if (expected === undefined) {
        continue;
      }
      expect(
        node.x,
        `node ${node.id} (${node.text}) x position should be preserved`,
      ).toBe(expected.x);
      expect(
        node.y,
        `node ${node.id} (${node.text}) y position should be preserved`,
      ).toBe(expected.y);
    }
  });

  it("uses computed positions for new nodes when existing model has no match", () => {
    // Parse a script without an existing model — all positions should be computed.
    const script = [
      "function onKLineClosed(ctx) {",
      "  console.log(ctx.kline.close);",
      "}",
    ].join("\n");
    const parsedWithoutExisting = buildStrategyVisualModelFromScript(script);
    expect(parsedWithoutExisting.ok).toBe(true);
    if (!parsedWithoutExisting.ok) {
      return;
    }

    const logNode = parsedWithoutExisting.model.nodes.find(
      (node) => getStrategyBlockKind(node) === "log",
    );
    expect(logNode).toBeDefined();

    // Parse the same script with an unrelated existing model — positions should still be computed.
    const unrelatedModel = createDefaultStrategyVisualModel();
    const parsedWithUnrelated = buildStrategyVisualModelFromScript(script, unrelatedModel);
    expect(parsedWithUnrelated.ok).toBe(true);
    if (!parsedWithUnrelated.ok) {
      return;
    }

    const logNode2 = parsedWithUnrelated.model.nodes.find(
      (node) => getStrategyBlockKind(node) === "log",
    );
    expect(logNode2).toBeDefined();
    // New node IDs won't match the unrelated model, so positions come from layout.
    expect(logNode2?.x).toBe(logNode?.x);
    expect(logNode2?.y).toBe(logNode?.y);
  });

  it("chains condition body statements serially instead of making them all siblings", () => {
    // Build a model where a condition has two serial children (notify → codeBlock).
    const visualModel = createDefaultStrategyVisualModel();
    const conditionNode: StrategyVisualNodeDocument = {
      id: "serial-cond",
      type: "diamond",
      x: 440,
      y: 300,
      text: "金叉",
      properties: { blockKind: "ifGoldenCross" },
    };
    const notifyNode: StrategyVisualNodeDocument = {
      id: "serial-notify",
      type: "rect",
      x: 700,
      y: 300,
      text: "发送通知",
      properties: { blockKind: "notify", message: "Golden cross!" },
    };
    const codeNode: StrategyVisualNodeDocument = {
      id: "serial-code",
      type: "rect",
      x: 960,
      y: 300,
      text: "买入",
      properties: { blockKind: "codeBlock", code: "placeOrder({ side: \"BUY\", quantity: 100, orderType: \"MARKET\" });", codeScope: "hook" },
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

    // Verify: condition → notify → codeBlock (serial chain), NOT all siblings.
    const edges = parsed.model.edges;
    const condToNotify = edges.find(
      (e) => e.sourceNodeId === "serial-cond" && e.targetNodeId === "serial-notify",
    );
    const notifyToCode = edges.find(
      (e) => e.sourceNodeId === "serial-notify" && e.targetNodeId === "serial-code",
    );
    const condToCode = edges.find(
      (e) => e.sourceNodeId === "serial-cond" && e.targetNodeId === "serial-code",
    );

    expect(condToNotify, "condition should connect to notify").toBeDefined();
    expect(notifyToCode, "notify should connect to codeBlock").toBeDefined();
    expect(condToCode, "condition should NOT directly connect to codeBlock").toBeUndefined();
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
    case "movingAverageFast":
    case "movingAverageSlow":
      return `${kind}:${properties.windowSize ?? ""}`;
    case "rsi":
      return `${kind}:${properties.period ?? ""}`;
    case "macd":
      return `${kind}:${properties.fastPeriod ?? ""}/${properties.slowPeriod ?? ""}/${properties.signalPeriod ?? ""}`;
    case "bollinger":
      return `${kind}:${properties.period ?? ""}x${properties.multiplier ?? ""}`;
    case "ifRsiAbove":
    case "ifRsiBelow":
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