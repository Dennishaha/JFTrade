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