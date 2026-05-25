import { describe, expect, it } from "vitest";

import type { StrategyVisualNodeDocument } from "@jftrade/ui-contracts";

import {
  buildStrategyScriptFromVisualModel,
  buildStrategyVisualControlEdgeProperties,
  buildStrategyVisualDataEdgeProperties,
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

  it("round-trips split indicator getter inputs and branch edges", () => {
    const visualModel = {
      engine: "logic-flow" as const,
      version: 1,
      nodes: [
        {
          id: "on-init-root",
          type: "circle",
          x: 180,
          y: 120,
          text: "策略启动",
          properties: { blockKind: "onInit" },
        },
        {
          id: "on-kline-root",
          type: "circle",
          x: 180,
          y: 320,
          text: "K 线收盘",
          properties: { blockKind: "onKLineClosed" },
        },
        {
          id: "split-rsi-getter",
          type: "rect",
          x: 430,
          y: 320,
          text: "获取 RSI 14",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "rsi",
            period: 14,
          },
        },
        {
          id: "split-rsi-condition",
          type: "diamond",
          x: 700,
          y: 320,
          text: "RSI < 30",
          properties: {
            blockKind: "technicalIndicatorCondition",
            indicatorType: "rsi",
            conditionMode: "numeric",
            operator: "<",
            threshold: 30,
          },
        },
        {
          id: "split-rsi-buy",
          type: "rect",
          x: 970,
          y: 270,
          text: "下单",
          properties: {
            blockKind: "placeOrder",
            side: "BUY",
            orderType: "MARKET",
            quantityMode: "shares",
            quantityValue: 100,
          },
        },
        {
          id: "split-rsi-log",
          type: "rect",
          x: 970,
          y: 380,
          text: "输出日志",
          properties: {
            blockKind: "log",
            message: "not oversold",
          },
        },
      ],
      edges: [
        {
          id: "edge-split-getter",
          type: "polyline",
          sourceNodeId: "on-kline-root",
          targetNodeId: "split-rsi-getter",
          properties: buildStrategyVisualControlEdgeProperties(),
        },
        {
          id: "edge-split-condition",
          type: "polyline",
          sourceNodeId: "split-rsi-getter",
          targetNodeId: "split-rsi-condition",
          properties: buildStrategyVisualControlEdgeProperties(),
        },
        {
          id: "edge-split-data",
          type: "polyline",
          sourceNodeId: "split-rsi-getter",
          targetNodeId: "split-rsi-condition",
          properties: buildStrategyVisualDataEdgeProperties("primary"),
        },
        {
          id: "edge-split-true",
          type: "polyline",
          sourceNodeId: "split-rsi-condition",
          targetNodeId: "split-rsi-buy",
          properties: buildStrategyVisualControlEdgeProperties("true"),
        },
        {
          id: "edge-split-false",
          type: "polyline",
          sourceNodeId: "split-rsi-condition",
          targetNodeId: "split-rsi-log",
          properties: buildStrategyVisualControlEdgeProperties("false"),
        },
      ],
    };

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: "split-rsi-roundtrip",
      symbol: "00700",
      interval: "1m",
    });

    expect(script).toContain("@jftradeFlowInputPrimaryNodeId split-rsi-getter");

    const parsed = buildStrategyVisualModelFromScript(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    expect(readNonRootNodeSignatures(parsed.model.nodes)).toEqual(
      readNonRootNodeSignatures(visualModel.nodes),
    );
    expect(readEdgeSignatures(parsed.model.nodes, parsed.model.edges)).toEqual(
      readEdgeSignatures(visualModel.nodes, visualModel.edges),
    );
  });

  it("round-trips getter variable names from flow annotations", () => {
    const visualModel = {
      engine: "logic-flow" as const,
      version: 1,
      nodes: [
        {
          id: "on-kline-root",
          type: "circle",
          x: 180,
          y: 320,
          text: "K 线收盘",
          properties: { blockKind: "onKLineClosed" },
        },
        {
          id: "split-rsi-getter",
          type: "rect",
          x: 430,
          y: 320,
          text: "获取 RSI 14",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "rsi",
            period: 14,
            variableName: "oversoldRsi",
          },
        },
        {
          id: "split-rsi-condition",
          type: "diamond",
          x: 700,
          y: 320,
          text: "RSI < 30",
          properties: {
            blockKind: "technicalIndicatorCondition",
            indicatorType: "rsi",
            conditionMode: "numeric",
            operator: "<",
            threshold: 30,
            inputPrimaryNodeId: "split-rsi-getter",
          },
        },
      ],
      edges: [
        {
          id: "edge-split-getter",
          type: "polyline",
          sourceNodeId: "on-kline-root",
          targetNodeId: "split-rsi-getter",
          properties: buildStrategyVisualControlEdgeProperties(),
        },
        {
          id: "edge-split-condition",
          type: "polyline",
          sourceNodeId: "split-rsi-getter",
          targetNodeId: "split-rsi-condition",
          properties: buildStrategyVisualControlEdgeProperties(),
        },
      ],
    };

    const script = buildStrategyScriptFromVisualModel(visualModel, {
      name: "split-rsi-variable-name",
      symbol: "00700",
      interval: "1m",
    });

    expect(script).toContain("@jftradeFlowVariableName oversoldRsi");

    const parsed = buildStrategyVisualModelFromScript(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const getterNode = parsed.model.nodes.find((node) => node.id === "split-rsi-getter");
    expect(getterNode?.properties.variableName).toBe("oversoldRsi");
    const conditionNode = parsed.model.nodes.find((node) => node.id === "split-rsi-condition");
    expect(conditionNode?.properties.inputPrimaryNodeId).toBe("split-rsi-getter");
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
        (node) => isIndicatorAuthoringNode(node),
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
      (node) => isIndicatorAuthoringNode(node),
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
    expect(getStrategyBlockKind(roundTrippedNode)).toBe(getStrategyBlockKind(renamedNode));
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
        (node) => isIndicatorAuthoringNode(node),
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

  it("parses place-order quantity mode and entry position policy from generated code", () => {
    const script = `
/** @param {JFTradeKLineClosedContext} ctx */
function onKLineClosed(ctx) {
  /**
   * @jftradeFlowNodeId buy-node
   * @jftradeFlowBlockKind placeOrder
   * @jftradeFlowNodeText 下单
   */
  const pos = getPosition();
  if (pos && pos.quantity !== 0) {
    console.log("当前已有持仓（方向 " + pos.direction + "，数量 " + pos.quantity + "），按必须空仓策略跳过开多");
    return;
  }
  const orderPrice = ctx.kline.close;
  const availableCash = getAvailableCash();
  const targetAmount = availableCash * 25 / 100;
  const orderQty = targetAmount > 0 ? Math.floor(targetAmount / orderPrice) : 0;
  if (orderQty <= 0) {
    console.log("现金百分比计算所得数量为 0（可用资金 " + availableCash + " × 25% ÷ 价格 " + orderPrice + "），请调整百分比或确认账户资金充足");
    return;
  }
  console.log(\`下单 \${orderQty} 股 买入开多 (cashPercent)\`);
  placeOrder({ side: "BUY", orderType: "MARKET", quantity: orderQty });
}
`;

    const parsed = buildStrategyVisualModelFromScript(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const buyNode = parsed.model.nodes.find((node) => node.id === "buy-node");
    expect(getStrategyBlockKind(buyNode)).toBe("placeOrder");
    expect(buyNode?.properties.entryPositionPolicy).toBe("flatOnly");
    expect(buyNode?.properties.quantityMode).toBe("cashPercent");
    expect(buyNode?.properties.quantityValue).toBe(25);
  });

  it("parses current symbol position percent and limit price from generated code", () => {
    const script = `
/** @param {JFTradeKLineClosedContext} ctx */
function onKLineClosed(ctx) {
  /**
   * @jftradeFlowNodeId sell-node
   * @jftradeFlowBlockKind placeOrder
   * @jftradeFlowNodeText 下单
   */
  const pos = getPosition();
  const availablePositionQty = pos ? Math.floor(Math.abs(pos.availableQuantity) > 0 ? Math.abs(pos.availableQuantity) : Math.abs(pos.quantity)) : 0;
  if (!pos || pos.direction !== "LONG" || availablePositionQty <= 0) {
    console.log("无多头持仓可平，跳过卖出");
    return;
  }
  const orderPrice = 480.5;
  const currentPositionValue = pos ? Math.abs(pos.marketValue) : 0;
  const targetValue = currentPositionValue * 25 / 100;
  const rawOrderQty = targetValue > 0 ? Math.floor(targetValue / orderPrice) : 0;
  const orderQty = rawOrderQty > 0 ? Math.min(rawOrderQty, availablePositionQty || rawOrderQty) : (true && availablePositionQty > 0 ? 1 : 0);
  if (orderQty <= 0) {
    console.log("当前标的仓位百分比计算所得数量为 0，跳过下单");
    return;
  }
  console.log(\`下单 \${orderQty} 股 卖出平多 (symbolPositionPercent)\`);
  placeOrder({ side: "SELL", orderType: "LIMIT", limitPrice: 480.5, quantity: orderQty });
}
`;

    const parsed = buildStrategyVisualModelFromScript(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const sellNode = parsed.model.nodes.find((node) => node.id === "sell-node");
    expect(getStrategyBlockKind(sellNode)).toBe("placeOrder");
    expect(sellNode?.properties.quantityMode).toBe("symbolPositionPercent");
    expect(sellNode?.properties.quantityValue).toBe(25);
    expect(sellNode?.properties.limitPrice).toBe(480.5);
  });

  it("parses account position percent from generated code", () => {
    const script = `
/** @param {JFTradeKLineClosedContext} ctx */
function onKLineClosed(ctx) {
  /**
   * @jftradeFlowNodeId buy-node
   * @jftradeFlowBlockKind placeOrder
   * @jftradeFlowNodeText 下单
   */
  const pos = getPosition();
  const availablePositionQty = pos ? Math.floor(Math.abs(pos.availableQuantity) > 0 ? Math.abs(pos.availableQuantity) : Math.abs(pos.quantity)) : 0;
  if (pos && pos.direction === "LONG" && availablePositionQty > 0) {
    console.log("已有多头持仓 " + pos.quantity + " 股，按拦截同向加仓策略跳过开多");
    return;
  }
  const orderPrice = ctx.kline.close;
  const accountTotalValue = getTotalAccountValue();
  const targetAmount = accountTotalValue * 10 / 100;
  const rawOrderQty = targetAmount > 0 ? Math.floor(targetAmount / orderPrice) : 0;
  const orderQty = rawOrderQty > 0 ? Math.min(rawOrderQty, availablePositionQty || rawOrderQty) : (false && availablePositionQty > 0 ? 1 : 0);
  if (orderQty <= 0) {
    console.log("账户仓位百分比计算所得数量为 0（账户总资产 " + accountTotalValue + " × 10% ÷ 价格 " + orderPrice + "），请调整百分比或确认账户资产快照可用");
    return;
  }
  console.log(\`下单 \${orderQty} 股 买入开多 (accountPositionPercent)\`);
  placeOrder({ side: "BUY", orderType: "MARKET", quantity: orderQty });
}
`;

    const parsed = buildStrategyVisualModelFromScript(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const buyNode = parsed.model.nodes.find((node) => node.id === "buy-node");
    expect(getStrategyBlockKind(buyNode)).toBe("placeOrder");
    expect(buyNode?.properties.quantityMode).toBe("accountPositionPercent");
    expect(buyNode?.properties.quantityValue).toBe(10);
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
  edges: Array<{
    sourceNodeId: string;
    targetNodeId: string;
    properties?: Record<string, unknown>;
  }>,
): string[] {
  const nodesById = new Map(nodes.map((node) => [node.id, node] as const));

  return edges
    .map((edge) => {
      const source = nodesById.get(edge.sourceNodeId);
      const target = nodesById.get(edge.targetNodeId);
      if (source === undefined || target === undefined) {
        return null;
      }
      return [
        `${buildNodeSignature(source)}->${buildNodeSignature(target)}`,
        String(edge.properties?.role ?? "control"),
        String(edge.properties?.branch ?? ""),
        String(edge.properties?.slot ?? ""),
      ].join(":");
    })
    .filter((value): value is string => value !== null)
    .sort();
}

function buildNodeSignature(node: StrategyVisualNodeDocument): string {
  const kind = getStrategyBlockKind(node) ?? "unknown";
  const properties = node.properties ?? {};

  switch (kind) {
    case "getTechnicalIndicator":
      return [
        kind,
        String(properties.indicatorType ?? ""),
        String(properties.movingAverageType ?? ""),
        String(properties.windowSize ?? ""),
        String(properties.period ?? ""),
        String(properties.fastPeriod ?? ""),
        String(properties.slowPeriod ?? ""),
        String(properties.signalPeriod ?? ""),
        String(properties.m1 ?? ""),
        String(properties.m2 ?? ""),
        String(properties.multiplier ?? ""),
      ].join(":");
    case "technicalIndicatorCondition":
      return [
        kind,
        String(properties.indicatorType ?? ""),
        String(properties.conditionMode ?? ""),
        String(properties.operator ?? ""),
        String(properties.threshold ?? ""),
        String(properties.patternType ?? ""),
        String(properties.lookback ?? ""),
      ].join(":");
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

function isIndicatorAuthoringNode(node: StrategyVisualNodeDocument): boolean {
  const kind = getStrategyBlockKind(node);
  return kind === "technicalIndicator"
    || kind === "getTechnicalIndicator"
    || kind === "technicalIndicatorCondition";
}
