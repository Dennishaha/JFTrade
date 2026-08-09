import { describe, expect, it } from "vitest";

import type { StrategyVisualModelDocument } from "@/types";

import {
  createStrategyPaletteItems,
  getStrategyBlockCatalog,
} from "@/features/strategy-builder";
import {
  assessPineBlockSupport,
  getVisualBlockCapabilities,
  getStrategyAuthoringTemplates,
  parsePineExpressionToVisualExpression,
  renderVisualExpressionToPine,
  summarizePineBlockSupport,
} from "@/features/strategy-builder";
import {
  buildStrategyPineFromVisualModel,
} from "@/features/strategy-builder";
import {
  buildStrategyVisualModelFromPine,
} from "@/features/strategy-builder";

function createLinearVisualModel(
  nodes: Array<{
    id: string;
    text: string;
    properties: Record<string, unknown>;
    type?: "rect" | "diamond";
  }>,
): StrategyVisualModelDocument {
  const rootId = "on-kline-root";
  const visualNodes = [
    {
      id: rootId,
      type: "circle",
      x: 120,
      y: 120,
      text: "K 线收盘",
      properties: { blockKind: "onKLineClosed" },
    },
    ...nodes.map((node, index) => ({
      id: node.id,
      type: node.type ?? "rect",
      x: 360 + index * 240,
      y: 120,
      text: node.text,
      properties: node.properties,
    })),
  ];
  return {
    engine: "logic-flow",
    version: 1,
    nodes: visualNodes,
    edges: nodes.map((node, index) => ({
      id: `edge-${index}`,
      type: "polyline",
      sourceNodeId: index === 0 ? rootId : nodes[index - 1].id,
      targetNodeId: node.id,
    })),
  };
}

describe("strategyVisualBuilderPine", () => {
  it("rejects unsupported collection and visual Pine when converting to visual blocks", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Snippet Types", overlay=true)
values = array.from(close, open)
plot(close)
`);

    expect(parsed.ok).toBe(false);
    expect(parsed.error).toContain("第 3 行无法同步为流程图：values = array.from(close, open)");
  });

  it("rejects unsupported Pine lines instead of creating visual snippets", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Snippet", overlay=true)
plot(close)
`);

    expect(parsed.ok).toBe(false);
    expect(parsed.error).toContain("第 3 行无法同步为流程图：plot(close)");
  });

  it("rejects old codeBlock Pine annotations instead of converting them", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Legacy Annotation", overlay=true)
// @jftradeFlowNodeId legacy-code
// @jftradeFlowBlockKind codeBlock
// @jftradeFlowNodeText 旧代码块
plot(close)
`);

    expect(parsed.ok).toBe(false);
    expect(parsed.error).toContain("旧 codeBlock / technicalIndicator");
  });

  it("rejects legacy codeBlock visual models when generating Pine", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        {
          id: "on-kline-root",
          type: "circle",
          x: 120,
          y: 120,
          text: "K 线收盘",
          properties: { blockKind: "onKLineClosed" },
        },
        {
          id: "legacy-code",
          type: "rect",
          x: 360,
          y: 120,
          text: "旧代码块",
          properties: {
            blockKind: "codeBlock",
            code: "console.log('legacy')",
          },
        },
      ],
      edges: [
        {
          id: "edge-root-legacy",
          type: "polyline",
          sourceNodeId: "on-kline-root",
          targetNodeId: "legacy-code",
        },
      ],
    };

    expect(() => buildStrategyPineFromVisualModel(model, { name: "Legacy Code" }))
      .toThrow("旧流程图块 codeBlock 不再支持");
  });

  it("rejects legacy unified technicalIndicator visual models when generating Pine", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        {
          id: "on-kline-root",
          type: "circle",
          x: 120,
          y: 120,
          text: "K 线收盘",
          properties: { blockKind: "onKLineClosed" },
        },
        {
          id: "legacy-rsi",
          type: "rect",
          x: 360,
          y: 120,
          text: "RSI < 30",
          properties: {
            blockKind: "technicalIndicator",
            indicatorType: "rsi",
            conditionMode: "numeric",
            operator: "<",
            threshold: 30,
            period: 14,
          },
        },
      ],
      edges: [
        {
          id: "edge-root-rsi",
          type: "polyline",
          sourceNodeId: "on-kline-root",
          targetNodeId: "legacy-rsi",
        },
      ],
    };

    expect(() => buildStrategyPineFromVisualModel(model, { name: "Legacy Indicator" }))
      .toThrow("旧流程图块 technicalIndicator 不再支持");
  });

  it("does not expose legacy codeBlock or unified technicalIndicator in new palette paths", () => {
    const paletteKinds = createStrategyPaletteItems().map((item) => item.properties.blockKind);
    expect(paletteKinds).not.toContain("codeBlock");
    expect(paletteKinds).not.toContain("technicalIndicator");
  });

  it("rejects old technicalIndicator annotations instead of migrating them", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Legacy Indicator Annotation", overlay=true)
// @jftradeFlowNodeId old-rsi
// @jftradeFlowBlockKind technicalIndicator
// @jftradeFlowNodeText RSI
rsiValue = ta.rsi(close, 14)
// @jftradeFlowNodeId old-rsi-condition
// @jftradeFlowBlockKind technicalIndicator
// @jftradeFlowNodeText RSI < 30
if rsiValue < 30
    alert("buy")
`);

    expect(parsed.ok).toBe(false);
    expect(parsed.error).toContain("旧 codeBlock / technicalIndicator");
  });

  it("keeps built-in visual templates on standard Pine visual blocks", () => {
    for (const template of getStrategyAuthoringTemplates().filter((item) => item.mode === "visual")) {
      const blockKinds = template.visualModel.nodes.map((node) => node.properties.blockKind);
      expect(blockKinds, template.id).not.toContain("codeBlock");
      expect(blockKinds, template.id).not.toContain("technicalIndicator");
    }
  });

  it("registers Pine capabilities for every current visual block kind", () => {
    const catalogKinds = new Set(getStrategyBlockCatalog().map((block) => block.kind));
    const capabilities = getVisualBlockCapabilities();

    expect(new Set(capabilities.map((capability) => capability.kind))).toEqual(catalogKinds);
    for (const capability of capabilities) {
      expect(capability.controlSchema.controlIds.length, capability.kind).toBeGreaterThan(0);
      expect(capability.pineRenderRule.description, capability.kind).not.toBe("");
      expect(capability.pineParseRule.description, capability.kind).not.toBe("");
    }
    const placeOrder = capabilities.find((capability) => capability.kind === "placeOrder");
    expect(placeOrder?.controlSchema.controlIds).not.toContain("oca_name");
    expect(placeOrder?.controlSchema.controlIds).not.toContain("oca_type");
  });

  it("round-trips all visual templates without runtime guards", () => {
    for (const template of getStrategyAuthoringTemplates().filter((item) => item.mode === "visual")) {
      const script = template.buildScript({ name: template.label });
      expect(script, template.id).not.toContain("runtime.error");

      const parsed = buildStrategyVisualModelFromPine(script);
      expect(parsed.ok, template.id).toBe(true);
      if (!parsed.ok) {
        continue;
      }
    }
  });

  it("generates expanded visual templates with supported Pine indicator blocks", () => {
    const templates = new Map(
      getStrategyAuthoringTemplates().map((template) => [template.id, template]),
    );
    const mfiTemplate = templates.get("mfi-reversion");
    const mtfTemplate = templates.get("mtf-momentum");
    const supertrendTemplate = templates.get("supertrend-follow");
    const bracketTemplate = templates.get("bracket-exit-risk");

    expect(mfiTemplate).toBeDefined();
    expect(mtfTemplate).toBeDefined();
    expect(supertrendTemplate).toBeDefined();
    expect(bracketTemplate).toBeDefined();

    const mfiScript = mfiTemplate?.buildScript({ name: "MFI Template" }) ?? "";
    const mtfScript = mtfTemplate?.buildScript({ name: "MTF Template" }) ?? "";
    const supertrendScript = supertrendTemplate?.buildScript({ name: "Supertrend Template" }) ?? "";
    const bracketScript = bracketTemplate?.buildScript({ name: "Bracket Template" }) ?? "";

    expect(mfiScript).toContain("mfi_getter = ta.mfi(hlc3, 14)");
    expect(mtfScript).toContain('mtf_ema = request.security(syminfo.tickerid, "D", ta.ema(close, 20))');
    expect(supertrendScript).toContain("supertrend_getter = ta.supertrend(3, 10)");
    expect(supertrendScript).toContain("if supertrend_getter.direction > 0");
    expect(bracketScript).toContain('strategy.exit("Long bracketExit", "Long", stop=close * (1 - 2 / 100), limit=close * (1 + 4 / 100))');
    expect(`${mfiScript}\n${mtfScript}\n${supertrendScript}\n${bracketScript}`).not.toContain("runtime.error");
  });

  it("generates Pine timeframe moving averages and basic strategy exits", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        {
          id: "on-kline-root",
          type: "circle",
          x: 120,
          y: 120,
          text: "K 线收盘",
          properties: { blockKind: "onKLineClosed" },
        },
        {
          id: "daily-ma",
          type: "rect",
          x: 360,
          y: 120,
          text: "日线 EMA",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "movingAverage",
            movingAverageType: "EMA",
            windowSize: 5,
            timeframe: "D",
          },
        },
        {
          id: "exit-node",
          type: "rect",
          x: 600,
          y: 120,
          text: "1柱止损",
          properties: {
            blockKind: "stopLoss",
            mode: "stopLoss",
            direction: "long",
            timeValue: 1,
            timeUnit: "bar",
            percentage: 2,
            windowPolicy: "continuous",
          },
        },
      ],
      edges: [
        {
          id: "edge-root-ma",
          type: "polyline",
          sourceNodeId: "on-kline-root",
          targetNodeId: "daily-ma",
        },
        {
          id: "edge-ma-exit",
          type: "polyline",
          sourceNodeId: "daily-ma",
          targetNodeId: "exit-node",
        },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "MA Exit" });

    expect(script).toContain('daily_ma = request.security(syminfo.tickerid, "D", ta.ema(close, 5))');
    expect(script).toContain('strategy.exit("Long stopLoss", "Long", stop=close * (1 - 2 / 100))');

    const parsed = buildStrategyVisualModelFromPine(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const maNode = parsed.model.nodes.find((node) => node.id === "daily-ma");
    const exitNode = parsed.model.nodes.find((node) => node.id === "exit-node");
    expect(maNode?.properties.timeframe).toBe("D");
    expect(maNode?.properties.movingAverageType).toBe("EMA");
    expect(exitNode?.properties.mode).toBe("stopLoss");
    expect(exitNode?.properties.timeUnit).toBe("bar");
    expect(exitNode?.properties.percentage).toBe(2);
  });

  it("parses strategy.exit metadata into legacy stopLoss blocks", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Exit Metadata", overlay=true)
strategy.exit("Bracket", "Long", stop=98, limit=105, qty_percent=50, comment="generic", comment_profit="tp", comment_loss="sl", alert_message="base", alert_profit="ap", alert_loss="al", disable_alert=true, when=high > low)
strategy.exit("Trail", "Long", trail_points=10, trail_offset=5, comment="generic trail", comment_trailing="trail comment", alert_message="trail base", alert_trailing="trail alert", disable_alert=false, when=close > open)`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const stopNodes = parsed.model.nodes.filter((node) => node.properties?.blockKind === "stopLoss");
    expect(stopNodes).toHaveLength(2);
    [
      {
        mode: "bracketExit",
        quantityPercentage: 50,
        comment: "generic",
        comment_profit: "tp",
        comment_loss: "sl",
        alert_message: "base",
        alert_profit: "ap",
        alert_loss: "al",
        disable_alert: true,
        when: "high > low",
      },
      {
        mode: "trailingStop",
        comment: "generic trail",
        comment_trailing: "trail comment",
        alert_message: "trail base",
        alert_trailing: "trail alert",
        disable_alert: false,
        when: "close > open",
      },
    ].forEach((expected, index) => expect(stopNodes[index]?.properties).toMatchObject(expected));
  });

  it("parses strategy.exit with named or omitted from_entry into legacy stopLoss blocks", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Exit from entry variants", overlay=true)
strategy.exit("Named", from_entry="Short", limit=95)
strategy.exit("Auto", stop=98, qty_percent=25)`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const stopNodes = parsed.model.nodes.filter((node) => node.properties?.blockKind === "stopLoss");
    expect(stopNodes).toHaveLength(2);
    expect(stopNodes[0]?.properties).toMatchObject({
      direction: "short",
      mode: "takeProfit",
      takeProfitPriceExpressionAst: { kind: "literal", value: 95 },
    });
    expect(stopNodes[1]?.properties).toMatchObject({
      direction: "auto",
      fromEntryMode: "auto",
      mode: "stopLoss",
      quantityPercentage: 25,
      stopPriceExpressionAst: { kind: "literal", value: 98 },
    });

    const script = buildStrategyPineFromVisualModel(parsed.model, { name: "Exit from entry variants roundtrip" });
    expect(script).toContain('strategy.exit("Auto", stop=98, qty_percent=25)');
    expect(script).not.toContain('strategy.exit("Long stopLoss", "Long", stop=98');
    expect(script).not.toContain('strategy.exit("Short stopLoss", "Short", stop=98');
  });

  it("renders strategy.exit metadata from legacy stopLoss blocks", () => {
    const baseExit = {
      blockKind: "stopLoss",
      direction: "long",
      timeValue: 1,
      timeUnit: "bar",
      percentage: 2,
      windowPolicy: "continuous",
    };
    const model = createLinearVisualModel([
      {
        id: "bracket-exit",
        text: "Bracket",
        properties: {
          ...baseExit,
          mode: "bracketExit",
          takeProfitPercentage: 4,
          quantityPercentage: 50,
          comment: "generic",
          comment_profit: "tp",
          comment_loss: "sl",
          alert_message: "base",
          alert_profit: "ap",
          alert_loss: "al",
          disable_alert: true,
          when: "high > low",
        },
      },
      {
        id: "trail-exit",
        text: "Trail",
        properties: {
          ...baseExit,
          mode: "trailingStop",
          comment: "generic trail",
          comment_trailing: "trail comment",
          alert_message: "trail base",
          alert_trailing: "trail alert",
          disable_alert: false,
          when: "close > open",
        },
      },
    ]);

    const script = buildStrategyPineFromVisualModel(model, { name: "Legacy Exit Metadata" });

    expect(script).toContain('strategy.exit("Long bracketExit", "Long", stop=close * (1 - 2 / 100), limit=close * (1 + 4 / 100), qty_percent=50, comment="generic", comment_profit="tp", comment_loss="sl", alert_message="base", alert_profit="ap", alert_loss="al", disable_alert=true, when=high > low)');
    expect(script).toContain('strategy.exit("Long trailingStop", "Long", trail_points=close * 2 / 100, trail_offset=close * 2 / 100, comment="generic trail", comment_trailing="trail comment", alert_message="trail base", alert_trailing="trail alert", disable_alert=false, when=close > open)');
  });

  it("round-trips strategy.exit profit/loss tick semantics through legacy stopLoss blocks", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Exit points", overlay=true)
strategy.exit("Points", "Long", profit=50, loss=25, qty_percent=50)`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const exitNode = parsed.model.nodes.find((node) => node.properties?.blockKind === "stopLoss");
    expect(exitNode?.properties).toMatchObject({
      mode: "bracketExit",
      profitTicks: 50,
      lossTicks: 25,
      quantityPercentage: 50,
    });

    const script = buildStrategyPineFromVisualModel(parsed.model, { name: "Exit points roundtrip" });
    expect(script).toContain('strategy.exit("Points", "Long", loss=25, profit=50, qty_percent=50)');
  });

  it("round-trips strategy.exit trail_price semantics through legacy stopLoss blocks", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Trail price", overlay=true)
strategy.exit("Trail", "Long", trail_price=high[1], trail_offset=close * 2 / 100, comment_trailing="trail")`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const exitNode = parsed.model.nodes.find((node) => node.properties?.blockKind === "stopLoss");
    expect(exitNode?.properties).toMatchObject({
      mode: "trailingStop",
      trailingPriceMode: "price",
      trailingPriceExpressionAst: { kind: "history" },
      trailingOffsetExpressionAst: { kind: "binary" },
      comment_trailing: "trail",
    });

    const script = buildStrategyPineFromVisualModel(parsed.model, { name: "Trail price roundtrip" });
    expect(script).toContain('strategy.exit("Trail", "Long", trail_price=high[1], trail_offset=(close * 2) / 100, comment_trailing="trail")');
  });

  it("parses request.security timeframe indicators", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Parse MTF", overlay=true)
ema15 = request.security(syminfo.tickerid, "15", ta.ema(close, 9))`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }
    const emaNode = parsed.model.nodes.find((node) =>
      node.properties.blockKind === "getTechnicalIndicator"
      && node.properties.variableName === "ema15",
    );
    expect(emaNode?.properties.timeframe).toBe("15");
    expect(emaNode?.properties.movingAverageType).toBe("EMA");
    expect(emaNode?.properties.windowSize).toBe(9);
  });

  it("generates and parses bracket exit visual risk blocks", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        {
          id: "on-kline-root",
          type: "circle",
          x: 120,
          y: 120,
          text: "K 线收盘",
          properties: { blockKind: "onKLineClosed" },
        },
        {
          id: "bracket-exit",
          type: "rect",
          x: 360,
          y: 120,
          text: "止盈止损",
          properties: {
            blockKind: "stopLoss",
            mode: "bracketExit",
            direction: "long",
            timeValue: 1,
            timeUnit: "bar",
            percentage: 2,
            takeProfitPercentage: 4,
            windowPolicy: "continuous",
          },
        },
      ],
      edges: [
        { id: "edge-root-bracket", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "bracket-exit" },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Bracket Exit" });
    expect(script).toContain('strategy.exit("Long bracketExit", "Long", stop=close * (1 - 2 / 100), limit=close * (1 + 4 / 100))');
    expect(script).not.toContain("runtime.error");

    const parsed = buildStrategyVisualModelFromPine(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }
    expect(parsed.model.nodes.find((node) => node.id === "bracket-exit")?.properties).toMatchObject({
      blockKind: "stopLoss",
      mode: "bracketExit",
      direction: "long",
      percentage: 2,
      takeProfitPercentage: 4,
      timeUnit: "bar",
      windowPolicy: "continuous",
    });
  });

  it("parses TradingView Bollinger and Williams %R aliases", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Aliases", overlay=true)
basis = ta.bb(close, 20, 2)
wr = ta.wpr(14)
`);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const bollingerNode = parsed.model.nodes.find((node) => node.properties.variableName === "basis");
    const williamsNode = parsed.model.nodes.find((node) => node.properties.variableName === "wr");
    expect(bollingerNode?.properties.indicatorType).toBe("bollinger");
    expect(bollingerNode?.properties.period).toBe(20);
    expect(bollingerNode?.properties.multiplier).toBe(2);
    expect(williamsNode?.properties.indicatorType).toBe("williamsR");
    expect(williamsNode?.properties.period).toBe(14);
  });

  it("generates Williams %R runtime Pine instead of RSI", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        {
          id: "on-kline-root",
          type: "circle",
          x: 120,
          y: 120,
          text: "K 线收盘",
          properties: { blockKind: "onKLineClosed" },
        },
        {
          id: "williams",
          type: "rect",
          x: 360,
          y: 120,
          text: "Williams %R",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "williamsR",
            period: 14,
          },
        },
      ],
      edges: [
        {
          id: "edge-root-williams",
          type: "polyline",
          sourceNodeId: "on-kline-root",
          targetNodeId: "williams",
        },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "WPR" });
    expect(script).toContain("williams = ta.wpr(14)");
    expect(script).not.toContain("williams = ta.rsi");
  });

  it("generates runtime-aligned KDJ and Bollinger expressions and parses them back", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        {
          id: "on-kline-root",
          type: "circle",
          x: 120,
          y: 120,
          text: "K 线收盘",
          properties: { blockKind: "onKLineClosed" },
        },
        {
          id: "kdj-node",
          type: "rect",
          x: 360,
          y: 120,
          text: "KDJ",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "kdj",
            period: 9,
            m1: 3,
            m2: 3,
          },
        },
        {
          id: "boll-node",
          type: "rect",
          x: 600,
          y: 120,
          text: "布林带",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "bollinger",
            period: 20,
            multiplier: 2,
          },
        },
      ],
      edges: [
        {
          id: "edge-root-kdj",
          type: "polyline",
          sourceNodeId: "on-kline-root",
          targetNodeId: "kdj-node",
        },
        {
          id: "edge-kdj-boll",
          type: "polyline",
          sourceNodeId: "kdj-node",
          targetNodeId: "boll-node",
        },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Runtime Indicators" });
    expect(script).toContain("kdj_node_rsv = kdj_node_highest == kdj_node_lowest ? 50");
    expect(script).toContain("kdj_node_j = 3 * kdj_node_k - 2 * kdj_node_d");
    expect(script).toContain("boll_node = ta.bb(close, 20, 2)");
    expect(script).not.toContain("kdj_node = ta.rsi");
    expect(script).not.toContain("boll_node = ta.sma");

    const parsed = buildStrategyVisualModelFromPine(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }
    expect(parsed.model.nodes.find((node) => node.id === "kdj-node")?.properties).toMatchObject({
      indicatorType: "kdj",
      period: 9,
      m1: 3,
      m2: 3,
    });
    expect(parsed.model.nodes.find((node) => node.id === "boll-node")?.properties).toMatchObject({
      indicatorType: "bollinger",
      period: 20,
      multiplier: 2,
    });
  });

  it("parses native Pine indicator expressions into visual blocks", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Native Indicators", overlay=true)
signal = ta.macd(close, 12, 26, 9)
flow_highest = ta.highest(high, 9)
flow_lowest = ta.lowest(low, 9)
flow_rsv = flow_highest == flow_lowest ? 50 : ((close - flow_lowest) / (flow_highest - flow_lowest)) * 100
var flow_k = 50.0
var flow_d = 50.0
flow_k := ((2) * nz(flow_k[1], 50) + flow_rsv) / 3
flow_d := ((2) * nz(flow_d[1], 50) + flow_k) / 3
flow_j = 3 * flow_k - 2 * flow_d
band = ta.bb(close, 20, 2)
wr = ta.wpr(14)
`);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const byVariable = new Map(
      parsed.model.nodes.map((node) => [node.properties.variableName, node]),
    );
    expect(byVariable.get("signal")?.properties.indicatorType).toBe("macd");
    expect(byVariable.get("flow")?.properties.indicatorType).toBe("kdj");
    expect(byVariable.get("band")?.properties.indicatorType).toBe("bollinger");
    expect(byVariable.get("wr")?.properties.indicatorType).toBe("williamsR");
  });

  it("generates and parses the next Pine-supported indicator batch", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        {
          id: "on-kline-root",
          type: "circle",
          x: 120,
          y: 120,
          text: "K 线收盘",
          properties: { blockKind: "onKLineClosed" },
        },
        {
          id: "std-dev",
          type: "rect",
          x: 360,
          y: 120,
          text: "标准差",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "stdev",
            source: "close",
            period: 20,
          },
        },
        {
          id: "mfi-node",
          type: "rect",
          x: 600,
          y: 120,
          text: "MFI",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "mfi",
            source: "hlc3",
            period: 14,
          },
        },
        {
          id: "dmi-node",
          type: "rect",
          x: 840,
          y: 120,
          text: "DMI",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "dmi",
            period: 14,
            adxSmoothing: 14,
          },
        },
        {
          id: "trend-node",
          type: "rect",
          x: 1080,
          y: 120,
          text: "Supertrend",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "supertrend",
            factor: 3,
            period: 10,
          },
        },
        {
          id: "kc-node",
          type: "rect",
          x: 1320,
          y: 120,
          text: "Keltner",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "keltner",
            source: "close",
            period: 20,
            multiplier: 1.5,
          },
        },
        {
          id: "alma-node",
          type: "rect",
          x: 1560,
          y: 120,
          text: "ALMA",
          properties: {
            blockKind: "getTechnicalIndicator",
            indicatorType: "alma",
            source: "close",
            period: 20,
            offset: 0.85,
            sigma: 6,
          },
        },
      ],
      edges: [
        { id: "edge-root-std", type: "polyline", sourceNodeId: "on-kline-root", targetNodeId: "std-dev" },
        { id: "edge-std-mfi", type: "polyline", sourceNodeId: "std-dev", targetNodeId: "mfi-node" },
        { id: "edge-mfi-dmi", type: "polyline", sourceNodeId: "mfi-node", targetNodeId: "dmi-node" },
        { id: "edge-dmi-trend", type: "polyline", sourceNodeId: "dmi-node", targetNodeId: "trend-node" },
        { id: "edge-trend-kc", type: "polyline", sourceNodeId: "trend-node", targetNodeId: "kc-node" },
        { id: "edge-kc-alma", type: "polyline", sourceNodeId: "kc-node", targetNodeId: "alma-node" },
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, { name: "Expanded Indicators" });

    expect(script).toContain("std_dev = ta.stdev(close, 20)");
    expect(script).toContain("mfi_node = ta.mfi(hlc3, 14)");
    expect(script).toContain("dmi_node = ta.dmi(14, 14)");
    expect(script).toContain("trend_node = ta.supertrend(3, 10)");
    expect(script).toContain("kc_node = ta.kc(close, 20, 1.5, true)");
    expect(script).toContain("alma_node = ta.alma(close, 20, 0.85, 6)");

    const parsed = buildStrategyVisualModelFromPine(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    expect(parsed.model.nodes.find((node) => node.id === "std-dev")?.properties).toMatchObject({
      indicatorType: "stdev",
      source: "close",
      period: 20,
    });
    expect(parsed.model.nodes.find((node) => node.id === "mfi-node")?.properties).toMatchObject({
      indicatorType: "mfi",
      source: "hlc3",
      period: 14,
    });
    expect(parsed.model.nodes.find((node) => node.id === "dmi-node")?.properties).toMatchObject({
      indicatorType: "dmi",
      period: 14,
      adxSmoothing: 14,
    });
    expect(parsed.model.nodes.find((node) => node.id === "trend-node")?.properties).toMatchObject({
      indicatorType: "supertrend",
      factor: 3,
      period: 10,
    });
    expect(parsed.model.nodes.find((node) => node.id === "kc-node")?.properties).toMatchObject({
      indicatorType: "keltner",
      period: 20,
      multiplier: 1.5,
    });
    expect(parsed.model.nodes.find((node) => node.id === "alma-node")?.properties).toMatchObject({
      indicatorType: "alma",
      offset: 0.85,
      sigma: 6,
    });
  });

  it("parses object-field numeric indicator conditions", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Object Conditions", overlay=true)
trend = ta.supertrend(3, 10)
adx = ta.dmi(14, 14)
if trend.direction > 0
    if adx.adx > 25
        strategy.entry("Long", strategy.long, qty=1)
`);

    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const conditions = parsed.model.nodes.filter(
      (node) => node.properties.blockKind === "technicalIndicatorCondition",
    );
    expect(conditions).toHaveLength(2);
    expect(conditions[0]?.properties).toMatchObject({
      indicatorType: "supertrend",
      conditionMode: "numeric",
      operator: ">",
      threshold: 0,
    });
    expect(conditions[1]?.properties).toMatchObject({
      indicatorType: "dmi",
      conditionMode: "numeric",
      operator: ">",
      threshold: 25,
    });
  });

  it("reports Pine block support without persisting support state", () => {
    const supportedStop = {
      id: "supported-stop",
      type: "rect",
      x: 0,
      y: 0,
      text: "自动止损 1柱 2%",
      properties: {
        blockKind: "stopLoss",
        mode: "stopLoss",
        direction: "auto",
        timeValue: 1,
        timeUnit: "bar",
        percentage: 2,
        windowPolicy: "continuous",
      },
    };
    const unsupportedStop = {
      ...supportedStop,
      id: "unsupported-stop",
      properties: {
        ...supportedStop.properties,
        timeUnit: "day",
      },
    };
    const unknown = {
      ...supportedStop,
      id: "unknown",
      text: "未知图块",
      properties: {
        blockKind: "unknownBlock",
      },
    };
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [supportedStop, unsupportedStop, unknown],
      edges: [],
    };

    expect(assessPineBlockSupport(supportedStop).status).toBe("supported");
    expect(assessPineBlockSupport(unsupportedStop).status).toBe("unsupportedConfig");
    expect(assessPineBlockSupport(unknown).status).toBe("unsupportedConfig");
    expect(summarizePineBlockSupport(model)).toMatchObject({
      unsupportedConfigCount: 2,
      warningCount: 0,
    });
    expect(model.nodes.some((node) => "pineSupport" in node.properties)).toBe(false);
  });

  it("preserves source-aware moving averages", () => {
    const parsed = buildStrategyVisualModelFromPine(`//@version=6
strategy("Volume MA", overlay=true)
avgVol = ta.sma(volume, 20)
`);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const maNode = parsed.model.nodes.find((node) => node.properties.variableName === "avgVol");
    expect(maNode?.properties.indicatorType).toBe("movingAverage");
    expect(maNode?.properties.movingAverageType).toBe("SMA");
    expect(maNode?.properties.windowSize).toBe(20);
    expect(maNode?.properties.source).toBe("volume");

    const script = buildStrategyPineFromVisualModel(parsed.model, { name: "Volume MA" });
    expect(script).toContain("avgVol = ta.sma(volume, 20)");
  });
});
