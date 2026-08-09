import { describe, expect, it } from "vitest";

import type {
  StrategyVisualEdgeDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@/types";

import {
  buildStrategyVisualControlEdgeProperties,
  buildStrategyVisualDataEdgeProperties,
} from "@/features/strategy-builder";
import {
  getPatternOptions,
  getTechnicalIndicatorConditionModeOptions,
  getTechnicalIndicatorInputSlots,
  indicatorTimeframeLabel,
  indicatorTypeLabel,
  normalizeGetTechnicalIndicatorProperties,
  normalizeIndicatorTimeframe,
  normalizeTechnicalIndicatorConditionMode,
  normalizeTechnicalIndicatorConditionProperties,
  normalizeTechnicalIndicatorPatternType,
  nextGetTechnicalIndicatorNodeText,
  nextTechnicalIndicatorConditionNodeText,
  patternTypeLabel,
  supportsNumericCondition,
  supportsPatternCondition,
} from "@/features/strategy-builder";
import { buildStrategyPineFromVisualModel } from "@/features/strategy-builder";
import { buildStrategyVisualModelFromPine } from "@/features/strategy-builder";

describe("strategy visual builder business coverage", () => {
  it("normalizes advanced technical indicator blocks and operator-facing labels", () => {
    expect(
      getTechnicalIndicatorConditionModeOptions("rsi", true).map(
        (option) => option.value,
      ),
    ).toEqual(["none", "numeric", "pattern"]);
    expect(
      getTechnicalIndicatorConditionModeOptions("movingAverage").map(
        (option) => option.value,
      ),
    ).toEqual(["pattern"]);
    expect(getTechnicalIndicatorInputSlots("movingAverage")).toEqual([
      "fast",
      "slow",
    ]);
    expect(getPatternOptions("bollinger").map((option) => option.value)).toEqual([
      "closeAboveUpperBand",
      "closeBelowLowerBand",
    ]);
    expect(supportsNumericCondition("atr")).toBe(true);
    expect(supportsPatternCondition("atr")).toBe(false);
    expect(supportsPatternCondition("movingAverage")).toBe(true);

    expect(normalizeIndicatorTimeframe(" 120 ")).toBe("120");
    expect(normalizeIndicatorTimeframe(" 240 ")).toBe("240");
    expect(normalizeIndicatorTimeframe(" bad ")).toBe("");
    expect(indicatorTimeframeLabel("45")).toBe("45分钟");
    expect(indicatorTypeLabel("keltner")).toBe("Keltner 通道");
    expect(patternTypeLabel("closeBelowLowerBand")).toBe("跌破下轨");

    expect(normalizeTechnicalIndicatorConditionMode("none", "rsi")).toBe("none");
    expect(normalizeTechnicalIndicatorConditionMode("pattern", "atr")).toBe(
      "numeric",
    );
    expect(
      normalizeTechnicalIndicatorPatternType("movingAverage", "invalid"),
    ).toBe("goldenCross");
    expect(normalizeTechnicalIndicatorPatternType("rsi", "invalid")).toBe(
      "bottomDivergence",
    );
    expect(
      normalizeTechnicalIndicatorPatternType("bollinger", "invalid"),
    ).toBe("closeBelowLowerBand");
    expect(normalizeTechnicalIndicatorPatternType("atr", "invalid")).toBe(
      "goldenCross",
    );

    expect(
      normalizeGetTechnicalIndicatorProperties({
        indicatorType: "sar",
        start: "0.03",
        increment: "0.04",
        maximum: "0.5",
      }),
    ).toMatchObject({
      indicatorType: "sar",
      start: 0.03,
      increment: 0.04,
      maximum: 0.5,
    });
    expect(
      normalizeGetTechnicalIndicatorProperties({
        indicatorType: "linreg",
        source: "hlc3",
        period: "15",
        offset: "2",
      }),
    ).toMatchObject({
      indicatorType: "linreg",
      source: "hlc3",
      period: 15,
      offset: 2,
    });
    expect(
      normalizeGetTechnicalIndicatorProperties({
        indicatorType: "pivotHigh",
        source: "high",
        leftBars: "3",
        rightBars: "5",
      }),
    ).toMatchObject({
      indicatorType: "pivotHigh",
      source: "high",
      leftBars: 3,
      rightBars: 5,
    });
    expect(
      normalizeGetTechnicalIndicatorProperties({
        indicatorType: "alma",
        source: "ohlc4",
        period: "11",
        offset: "0.75",
        sigma: "4",
      }),
    ).toMatchObject({
      indicatorType: "alma",
      source: "ohlc4",
      period: 11,
      offset: 0.75,
      sigma: 4,
    });
    expect(
      normalizeGetTechnicalIndicatorProperties({
        indicatorType: "vwap",
        source: "hl2",
      }),
    ).toMatchObject({
      indicatorType: "vwap",
      source: "hl2",
    });

    expect(
      normalizeTechnicalIndicatorConditionProperties({
        indicatorType: "movingAverage",
        conditionMode: "none",
      }),
    ).toMatchObject({
      indicatorType: "movingAverage",
      conditionMode: "pattern",
    });
    expect(
      normalizeTechnicalIndicatorConditionProperties({
        indicatorType: "dmi",
        conditionMode: "numeric",
        operator: ">",
        threshold: "bad",
      }),
    ).toMatchObject({
      indicatorType: "dmi",
      operator: ">",
      threshold: 25,
    });
    expect(
      normalizeTechnicalIndicatorConditionProperties({
        indicatorType: "rsi",
        conditionMode: "pattern",
        patternType: "topDivergence",
        lookback: "8",
      }),
    ).toMatchObject({
      indicatorType: "rsi",
      patternType: "topDivergence",
      lookback: 8,
    });

    expect(
      nextGetTechnicalIndicatorNodeText({
        indicatorType: "vwap",
        source: "hl2",
      }),
    ).toBe("获取 VWAP hl2");
    expect(
      nextGetTechnicalIndicatorNodeText({
        indicatorType: "dmi",
        period: 14,
        adxSmoothing: 10,
      }),
    ).toBe("获取 DMI/ADX 14/10");
    expect(
      nextGetTechnicalIndicatorNodeText({
        indicatorType: "sar",
        start: 0.03,
        increment: 0.04,
        maximum: 0.5,
      }),
    ).toBe("获取 Parabolic SAR 0.03/0.04/0.50");
    expect(
      nextGetTechnicalIndicatorNodeText({
        indicatorType: "linreg",
        period: 9,
        offset: 2,
      }),
    ).toBe("获取 线性回归 9/2");
    expect(
      nextGetTechnicalIndicatorNodeText({
        indicatorType: "pivotHigh",
        leftBars: 3,
        rightBars: 4,
      }),
    ).toBe("获取 Pivot High 3/4");
    expect(
      nextGetTechnicalIndicatorNodeText({
        indicatorType: "keltner",
        period: 18,
        multiplier: 1.8,
      }),
    ).toBe("获取 Keltner 通道 18x1.80");
    expect(
      nextGetTechnicalIndicatorNodeText({
        indicatorType: "alma",
        period: 9,
        offset: 0.75,
        sigma: 4,
      }),
    ).toBe("获取 ALMA 9/0.75/4");
    expect(
      nextTechnicalIndicatorConditionNodeText({
        indicatorType: "rsi",
        conditionMode: "pattern",
        patternType: "topDivergence",
        lookback: 8,
      }),
    ).toBe("RSI 顶背离 (8)");
    expect(
      nextTechnicalIndicatorConditionNodeText({
        indicatorType: "bollinger",
        conditionMode: "pattern",
        patternType: "closeAboveUpperBand",
      }),
    ).toBe("布林带 突破上轨");
  });

  it("renders rootless inputs, rejects legacy blocks, and reports parser failures clearly", () => {
    const inputOnlyModel: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        createNode("input-int", "strategyInput", "参数 Period", {
          variableName: "period",
          inputType: "int",
          title: "Period",
          defaultValue: 20,
        }),
        createNode("input-float", "strategyInput", "参数 Threshold", {
          variableName: "threshold",
          inputType: "float",
          title: "Threshold",
          defaultValue: 1.5,
        }),
        createNode("input-source", "strategyInput", "参数 Baseline", {
          variableName: "baseline",
          inputType: "source",
          title: "Baseline",
          defaultValue: "hl2",
        }),
        createNode("input-timeframe", "strategyInput", "参数 MTF", {
          variableName: "higher_tf",
          inputType: "timeframe",
          title: "Higher TF",
          defaultValue: "60",
        }),
        createNode("input-time", "strategyInput", "参数 Reset", {
          variableName: "reset_time",
          inputType: "time",
          title: "Reset Time",
          defaultValue: "timestamp(2026, 1, 1)",
        }),
        createNode("input-color", "strategyInput", "参数 Theme", {
          variableName: "theme",
          inputType: "color",
          title: "Theme",
          defaultValue: "color.green",
        }),
      ],
      edges: [],
    };

    const inputScript = buildStrategyPineFromVisualModel(inputOnlyModel, {
      name: "  ",
    });

    expect(inputScript).toContain('strategy("未命名策略"');
    expect(inputScript).toContain('period = input.int(20, "Period")');
    expect(inputScript).toContain(
      'threshold = input.float(defval=1.5, title="Threshold")',
    );
    expect(inputScript).toContain('baseline = input.source(hl2, "Baseline")');
    expect(inputScript).toContain(
      'higher_tf = input.timeframe("60", "Higher TF")',
    );
    expect(inputScript).toContain(
      'reset_time = input.time(timestamp(2026, 1, 1), "Reset Time")',
    );
    expect(inputScript).toContain(
      'theme = input.color(color.green, "Theme")',
    );
    expect(inputScript).toContain('log.info("策略尚未配置入口图块")');

    expect(() =>
      buildStrategyPineFromVisualModel(
        {
          engine: "logic-flow",
          version: 1,
          nodes: [
            createRoot("root", "onKLineClosed", "K 线收盘"),
            createNode("legacy-block", "codeBlock", "旧版代码块", {}),
          ],
          edges: [controlEdge("root", "legacy-block")],
        },
        { name: "legacy" },
      ),
    ).toThrow(/旧流程图块 codeBlock 不再支持/);

    expect(buildStrategyVisualModelFromPine("")).toEqual({
      ok: false,
      error: "Pine 代码为空，无法转换回流程图。",
    });
    expect(
      buildStrategyVisualModelFromPine(
        "// @jftradeFlowBlockKind technicalIndicator\nindicator = ta.rsi(close, 14)\n",
      ),
    ).toMatchObject({
      ok: false,
      error: expect.stringContaining("旧 codeBlock / technicalIndicator"),
    });
    expect(
      buildStrategyVisualModelFromPine(
        "//@version=6\nstrategy(\"Unsupported\", overlay=true)\nplot(close)\n",
      ),
    ).toMatchObject({
      ok: false,
      error: expect.stringContaining("plot(close)"),
    });
  });
});

function createRoot(
  id: string,
  blockKind: "onInit" | "onKLineClosed",
  text: string,
): StrategyVisualNodeDocument {
  return {
    id,
    type: "circle",
    x: 0,
    y: 0,
    text,
    properties: { blockKind },
  };
}

function createNode(
  id: string,
  blockKind: string,
  text: string,
  properties: Record<string, unknown>,
  type: StrategyVisualNodeDocument["type"] = "rect",
): StrategyVisualNodeDocument {
  return {
    id,
    type,
    x: 0,
    y: 0,
    text,
    properties: {
      blockKind,
      ...properties,
    },
  };
}

function controlEdge(
  sourceNodeId: string,
  targetNodeId: string,
  branch?: "true" | "false",
): StrategyVisualEdgeDocument {
  return {
    id: `edge-${sourceNodeId}-${targetNodeId}-${branch ?? "control"}`,
    type: "polyline",
    sourceNodeId,
    targetNodeId,
    properties: buildStrategyVisualControlEdgeProperties(branch),
  };
}

function dataEdge(
  sourceNodeId: string,
  targetNodeId: string,
  slot: "primary" | "fast" | "slow",
): StrategyVisualEdgeDocument {
  return {
    id: `edge-${sourceNodeId}-${targetNodeId}-${slot}`,
    type: "polyline",
    sourceNodeId,
    targetNodeId,
    properties: buildStrategyVisualDataEdgeProperties(slot),
  };
}
