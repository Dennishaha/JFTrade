import { describe, expect, it } from "vitest";

import { createStrategyPaletteItems } from "../src/features/strategyVisualBuilder";
import {
  STRATEGY_TECHNICAL_INDICATOR_SHORTCUT_CREATION_MODE,
  expandTechnicalIndicatorShortcutNode,
} from "../src/features/strategyVisualBuilderIndicatorShortcut";

describe("strategyVisualBuilderIndicatorShortcut", () => {
  it("exposes indicator condition entry while keeping getter blocks out of the palette", () => {
    const labels = createStrategyPaletteItems().map((item) => item.label);

    expect(labels).toContain("指标条件判断");
    expect(labels).not.toContain("指标数据");
    expect(labels).not.toContain("技术指标");
  });

  it("expands a scalar indicator shortcut into getter and condition nodes", () => {
    const expansion = expandTechnicalIndicatorShortcutNode({
      id: "shortcut-rsi",
      x: 420,
      y: 260,
      properties: {
        blockKind: "technicalIndicator",
        creationMode: STRATEGY_TECHNICAL_INDICATOR_SHORTCUT_CREATION_MODE,
        indicatorType: "rsi",
        conditionMode: "numeric",
        operator: "<",
        threshold: 30,
        period: 14,
      },
    });

    expect(expansion.focusNodeId).toBe("shortcut-rsi-condition");
    expect(expansion.nodes).toHaveLength(2);
    expect(expansion.edges).toHaveLength(1);

    expect(expansion.nodes[0]).toMatchObject({
      id: "shortcut-rsi-getter",
      type: "rect",
      x: 420,
      y: 260,
      text: "获取 RSI 14",
      properties: {
        blockKind: "getTechnicalIndicator",
        indicatorType: "rsi",
        period: 14,
        variableName: "RSI14",
      },
    });
    expect(expansion.nodes[1]).toMatchObject({
      id: "shortcut-rsi-condition",
      type: "diamond",
      x: 720,
      y: 260,
      text: "RSI < 30",
      properties: {
        blockKind: "technicalIndicatorCondition",
        indicatorType: "rsi",
        conditionMode: "numeric",
        operator: "<",
        threshold: 30,
        inputPrimaryNodeId: "shortcut-rsi-getter",
      },
    });
    expect(expansion.edges[0]?.properties).toMatchObject({ role: "control" });
  });

  it("expands a moving-average shortcut into two getters and one condition", () => {
    const expansion = expandTechnicalIndicatorShortcutNode({
      id: "shortcut-ma",
      x: 500,
      y: 300,
      properties: {
        blockKind: "technicalIndicator",
        creationMode: STRATEGY_TECHNICAL_INDICATOR_SHORTCUT_CREATION_MODE,
        indicatorType: "movingAverage",
        conditionMode: "pattern",
        patternType: "goldenCross",
        movingAverageType: "EMA",
        fastPeriod: 5,
        slowPeriod: 20,
      },
    });

    expect(expansion.focusNodeId).toBe("shortcut-ma-condition");
    expect(expansion.nodes).toHaveLength(3);
    expect(expansion.edges).toHaveLength(2);
    expect(expansion.nodes[0]).toMatchObject({
      id: "shortcut-ma-fast",
      text: "获取 均线 EMA 5日",
      properties: {
        blockKind: "getTechnicalIndicator",
        indicatorType: "movingAverage",
        movingAverageType: "EMA",
        windowSize: 5,
        periodUnit: "day",
        variableName: "EMA5",
      },
    });
    expect(expansion.nodes[1]).toMatchObject({
      id: "shortcut-ma-slow",
      text: "获取 均线 EMA 20日",
      properties: {
        blockKind: "getTechnicalIndicator",
        indicatorType: "movingAverage",
        movingAverageType: "EMA",
        windowSize: 20,
        periodUnit: "day",
        variableName: "EMA20",
      },
    });
    expect(expansion.nodes[2]).toMatchObject({
      id: "shortcut-ma-condition",
      text: "双均线 金叉",
      properties: {
        blockKind: "technicalIndicatorCondition",
        indicatorType: "movingAverage",
        conditionMode: "pattern",
        patternType: "goldenCross",
        inputFastNodeId: "shortcut-ma-fast",
        inputSlowNodeId: "shortcut-ma-slow",
      },
    });
    expect(expansion.edges[0]?.properties).toMatchObject({ role: "control" });
    expect(expansion.edges[1]?.properties).toMatchObject({ role: "control" });
  });
});