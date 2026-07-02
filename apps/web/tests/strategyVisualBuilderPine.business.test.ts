import { describe, expect, it } from "vitest";

import type {
  StrategyVisualEdgeDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@/contracts";

import {
  buildStrategyVisualControlEdgeProperties,
  buildStrategyVisualDataEdgeProperties,
} from "../src/features/strategyVisualBuilderEdges";
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
} from "../src/features/strategyVisualBuilderIndicatorBlock";
import { buildStrategyPineFromVisualModel } from "../src/features/strategyVisualBuilderPine";
import { buildStrategyVisualModelFromPine } from "../src/features/strategyVisualBuilderPineParser";

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

  it("round-trips advanced trading flows across indicators, filters, orders, exits, and state blocks", () => {
    const nodes = [
      createRoot("init-root", "onInit", "初始化"),
      createRoot("kline-root", "onKLineClosed", "K 线收盘"),

      createNode("input-int", "strategyInput", "参数 Period", {
        variableName: "period",
        inputType: "int",
        title: "Period",
        defaultValue: 20,
      }),
      createNode("input-source", "strategyInput", "参数 Baseline", {
        variableName: "baseline",
        inputType: "source",
        title: "Baseline",
        defaultValue: "hl2",
      }),
      createNode("input-timeframe", "strategyInput", "参数 HigherTF", {
        variableName: "higher_tf",
        inputType: "timeframe",
        title: "Higher TF",
        defaultValue: "D",
      }),
      createNode("input-color", "strategyInput", "参数 Theme", {
        variableName: "theme",
        inputType: "color",
        title: "Theme",
        defaultValue: "#00FF00",
      }),

      createNode("state-var", "stateVariable", "状态 Armed", {
        variableName: "armed",
        valueType: "bool",
        initialValue: true,
      }),
      createNode("state-update", "stateUpdate", "更新状态", {
        variableName: "armed",
        expressionAst: {
          kind: "binary",
          left: { kind: "reference", name: "armed" },
          operator: "and",
          right: { kind: "literal", value: true },
        },
      }),

      createNode("derived-nz", "derivedSeries", "派生 NZ", {
        variableName: "safe_close",
        mode: "nz",
        source: "close",
        fallbackValue: 0,
      }),
      createNode("derived-max", "derivedSeries", "派生 MAX", {
        variableName: "range_high",
        mode: "math",
        mathFunction: "max",
        leftExpression: "high",
        rightExpression: "open",
      }),
      createNode("derived-abs", "derivedSeries", "派生 ABS", {
        variableName: "range_abs",
        mode: "math",
        mathFunction: "abs",
        leftExpression: "close",
      }),
      createNode("derived-arithmetic", "derivedSeries", "派生 SPREAD", {
        variableName: "spread",
        mode: "arithmetic",
        leftExpression: "close",
        operator: "-",
        rightExpression: "open",
      }),
      createNode("derived-cross", "derivedSeries", "派生 CROSS", {
        variableName: "cross_up",
        mode: "cross",
        crossFunction: "crossover",
        leftExpression: "close",
        rightExpression: "open",
      }),
      createNode("derived-history", "derivedSeries", "派生 HISTORY", {
        variableName: "prior_low",
        mode: "history",
        source: "low",
        historyOffset: 2,
      }),

      createNode("mtf-source", "mtfSeries", "MTF Source", {
        variableName: "daily_close",
        timeframe: "D",
        expressionType: "source",
        source: "close",
      }),
      createNode("mtf-history", "mtfSeries", "MTF History", {
        variableName: "daily_low_prev",
        timeframe: "W",
        expressionType: "history",
        source: "low",
        historyOffset: 2,
      }),
      createNode("mtf-indicator", "mtfSeries", "MTF MACD", {
        variableName: "weekly_macd",
        timeframe: "60",
        expressionType: "indicator",
        indicatorExpression: "ta.macd(close, 12, 26, 9)",
        mtfField: "histogram",
      }),

      createNode("collection-avg", "collectionStat", "集合统计 AVG", {
        variableName: "triple_avg",
        statFunction: "avg",
        sourceA: "close",
        sourceB: "open",
        sourceC: "high",
      }),
      createNode("collection-percentile", "collectionStat", "集合统计 PCTL", {
        variableName: "range_p90",
        statFunction: "percentile",
        sourceA: "low",
        sourceB: "close",
        sourceC: "high",
        percentile: 90,
      }),

      ...[
        {
          id: "ema-fast",
          text: "EMA Fast",
          properties: {
            indicatorType: "movingAverage",
            movingAverageType: "EMA",
            source: "close",
            windowSize: 9,
            timeframe: "60",
            variableName: "ema_fast",
          },
        },
        {
          id: "sma-slow",
          text: "SMA Slow",
          properties: {
            indicatorType: "movingAverage",
            movingAverageType: "SMA",
            source: "open",
            windowSize: 21,
            variableName: "sma_slow",
          },
        },
        {
          id: "smma-node",
          text: "SMMA",
          properties: {
            indicatorType: "movingAverage",
            movingAverageType: "SMMA",
            source: "hl2",
            windowSize: 14,
            variableName: "smma_value",
          },
        },
        {
          id: "lwma-node",
          text: "LWMA",
          properties: {
            indicatorType: "movingAverage",
            movingAverageType: "LWMA",
            source: "close",
            windowSize: 10,
            variableName: "lwma_value",
          },
        },
        {
          id: "hma-node",
          text: "HMA",
          properties: {
            indicatorType: "movingAverage",
            movingAverageType: "HMA",
            source: "close",
            windowSize: 16,
            variableName: "hma_value",
          },
        },
        {
          id: "vwma-node",
          text: "VWMA",
          properties: {
            indicatorType: "movingAverage",
            movingAverageType: "VWMA",
            source: "close",
            windowSize: 12,
            variableName: "vwma_value",
          },
        },
        {
          id: "macd-node",
          text: "MACD",
          properties: {
            indicatorType: "macd",
            fastPeriod: 12,
            slowPeriod: 26,
            signalPeriod: 9,
            timeframe: "D",
            variableName: "macd_signal",
          },
        },
        {
          id: "kdj-node",
          text: "KDJ",
          properties: {
            indicatorType: "kdj",
            period: 9,
            m1: 3,
            m2: 3,
            variableName: "kdj_signal",
          },
        },
        {
          id: "rsi-node",
          text: "RSI",
          properties: {
            indicatorType: "rsi",
            period: 7,
            timeframe: "15",
            variableName: "rsi_fast",
          },
        },
        {
          id: "boll-node",
          text: "Bollinger",
          properties: {
            indicatorType: "bollinger",
            period: 20,
            multiplier: 2,
            timeframe: "45",
            variableName: "boll_band",
          },
        },
        {
          id: "atr-node",
          text: "ATR",
          properties: {
            indicatorType: "atr",
            period: 14,
            timeframe: "30",
            variableName: "atr_value",
          },
        },
        {
          id: "cci-node",
          text: "CCI",
          properties: {
            indicatorType: "cci",
            source: "hlc3",
            period: 20,
            timeframe: "45",
            variableName: "cci_value",
          },
        },
        {
          id: "willr-node",
          text: "WilliamsR",
          properties: {
            indicatorType: "williamsR",
            period: 14,
            timeframe: "D",
            variableName: "wpr_value",
          },
        },
        {
          id: "stdev-node",
          text: "Stdev",
          properties: {
            indicatorType: "stdev",
            source: "close",
            period: 12,
            timeframe: "M",
            variableName: "stdev_value",
          },
        },
        {
          id: "variance-node",
          text: "Variance",
          properties: {
            indicatorType: "variance",
            source: "ohlc4",
            period: 10,
            timeframe: "120",
            variableName: "variance_value",
          },
        },
        {
          id: "highest-node",
          text: "Highest",
          properties: {
            indicatorType: "highest",
            source: "high",
            period: 30,
            timeframe: "5",
            variableName: "highest_value",
          },
        },
        {
          id: "lowest-node",
          text: "Lowest",
          properties: {
            indicatorType: "lowest",
            source: "low",
            period: 8,
            timeframe: "1",
            variableName: "lowest_value",
          },
        },
        {
          id: "sum-node",
          text: "Sum",
          properties: {
            indicatorType: "sum",
            source: "volume",
            period: 50,
            timeframe: "D",
            variableName: "sum_value",
          },
        },
        {
          id: "vwap-node",
          text: "VWAP",
          properties: {
            indicatorType: "vwap",
            source: "hlc3",
            timeframe: "D",
            variableName: "vwap_value",
          },
        },
        {
          id: "mfi-node",
          text: "MFI",
          properties: {
            indicatorType: "mfi",
            source: "hlc3",
            period: 14,
            timeframe: "60",
            variableName: "mfi_value",
          },
        },
        {
          id: "dmi-node",
          text: "DMI",
          properties: {
            indicatorType: "dmi",
            period: 14,
            adxSmoothing: 10,
            timeframe: "60",
            variableName: "dmi_strength",
          },
        },
        {
          id: "supertrend-node",
          text: "Supertrend",
          properties: {
            indicatorType: "supertrend",
            factor: 2.5,
            period: 11,
            timeframe: "240",
            variableName: "supertrend_signal",
          },
        },
        {
          id: "linreg-node",
          text: "LinReg",
          properties: {
            indicatorType: "linreg",
            source: "close",
            period: 9,
            offset: 2,
            timeframe: "D",
            variableName: "linreg_value",
          },
        },
        {
          id: "sar-node",
          text: "SAR",
          properties: {
            indicatorType: "sar",
            start: 0.03,
            increment: 0.04,
            maximum: 0.5,
            timeframe: "D",
            variableName: "sar_value",
          },
        },
        {
          id: "pivot-high-node",
          text: "Pivot High",
          properties: {
            indicatorType: "pivotHigh",
            source: "high",
            leftBars: 3,
            rightBars: 4,
            timeframe: "15",
            variableName: "pivot_high_value",
          },
        },
        {
          id: "pivot-low-node",
          text: "Pivot Low",
          properties: {
            indicatorType: "pivotLow",
            source: "low",
            leftBars: 2,
            rightBars: 5,
            timeframe: "15",
            variableName: "pivot_low_value",
          },
        },
        {
          id: "keltner-node",
          text: "Keltner",
          properties: {
            indicatorType: "keltner",
            source: "close",
            period: 18,
            multiplier: 1.8,
            timeframe: "W",
            variableName: "keltner_band",
          },
        },
        {
          id: "alma-node",
          text: "ALMA",
          properties: {
            indicatorType: "alma",
            source: "close",
            period: 9,
            offset: 0.75,
            sigma: 4,
            timeframe: "30",
            variableName: "alma_line",
          },
        },
      ].map((config) =>
        createNode(config.id, "getTechnicalIndicator", config.text, config.properties),
      ),

      createNode("time-after", "timeFilter", "开盘后", {
        mode: "after",
        startHour: 9,
        startMinute: 45,
      }, "diamond"),
      createNode("time-before", "timeFilter", "收盘前", {
        mode: "before",
        endHour: 15,
        endMinute: 30,
      }, "diamond"),
      createNode("time-day", "timeFilter", "周三过滤", {
        mode: "dayOfWeek",
        dayOfWeek: 4,
      }, "diamond"),
      createNode("time-between", "timeFilter", "交易时间", {
        mode: "between",
        startHour: 9,
        startMinute: 30,
        endHour: 16,
        endMinute: 0,
      }, "diamond"),
      createNode("between-log", "log", "命中日志", {
        message: "time matched",
      }),
      createNode("between-notify", "notify", "兜底通知", {
        message: "time missed",
      }),

      createNode("session-market", "sessionFilter", "常规时段", {
        scope: "market",
      }, "diamond"),
      createNode("session-pre", "sessionFilter", "盘前", {
        scope: "premarket",
      }, "diamond"),
      createNode("session-post", "sessionFilter", "盘后", {
        scope: "postmarket",
      }, "diamond"),

      createNode("close-above", "ifCloseAbove", "突破阈值", {
        threshold: 520,
      }, "diamond"),
      createNode("close-log", "log", "突破日志", {
        message: "close above",
      }),
      createNode("close-notify", "notify", "突破通知", {
        message: "close below",
      }),
      createNode("close-below", "ifCloseBelow", "跌破阈值", {
        threshold: 480,
      }, "diamond"),

      createNode("series-barssince", "seriesCondition", "Bars Since", {
        mode: "barssince",
        eventSource: "close",
        eventOperator: ">",
        eventThreshold: 520,
        length: 3,
      }, "diamond"),
      createNode("series-valuewhen", "seriesCondition", "Value When", {
        mode: "valuewhen",
        eventSource: "low",
        eventOperator: "<",
        eventThreshold: 5,
        valueSource: "high",
        occurrence: 1,
        operator: ">",
        threshold: 10,
      }, "diamond"),

      createNode("ma-cross", "technicalIndicatorCondition", "均线死叉", {
        indicatorType: "movingAverage",
        conditionMode: "pattern",
        patternType: "deathCross",
      }, "diamond"),
      createNode("macd-divergence", "technicalIndicatorCondition", "MACD 顶背离", {
        indicatorType: "macd",
        conditionMode: "pattern",
        patternType: "topDivergence",
        lookback: 7,
      }, "diamond"),
      createNode("rsi-divergence", "technicalIndicatorCondition", "RSI 底背离", {
        indicatorType: "rsi",
        conditionMode: "pattern",
        patternType: "bottomDivergence",
        lookback: 6,
      }, "diamond"),
      createNode("boll-break", "technicalIndicatorCondition", "突破布林上轨", {
        indicatorType: "bollinger",
        conditionMode: "pattern",
        patternType: "closeAboveUpperBand",
      }, "diamond"),
      createNode("macd-numeric", "technicalIndicatorCondition", "MACD 柱状图", {
        indicatorType: "macd",
        conditionMode: "numeric",
        operator: ">",
        threshold: 0,
        inputPrimaryNodeId: "macd-node",
      }, "diamond"),
      createNode("kdj-numeric", "technicalIndicatorCondition", "KDJ 阈值", {
        indicatorType: "kdj",
        conditionMode: "numeric",
        operator: "<",
        threshold: 20,
        inputPrimaryNodeId: "kdj-node",
      }, "diamond"),
      createNode("dmi-numeric", "technicalIndicatorCondition", "DMI 阈值", {
        indicatorType: "dmi",
        conditionMode: "numeric",
        operator: ">",
        threshold: 25,
      }, "diamond"),
      createNode("supertrend-numeric", "technicalIndicatorCondition", "Supertrend 方向", {
        indicatorType: "supertrend",
        conditionMode: "numeric",
        operator: ">",
        threshold: 0,
      }, "diamond"),
      createNode("keltner-numeric", "technicalIndicatorCondition", "Keltner 上轨", {
        indicatorType: "keltner",
        conditionMode: "numeric",
        operator: ">",
        threshold: 120,
      }, "diamond"),

      createNode("order-close-all", "placeOrder", "全部平仓", {
        orderAction: "closeAll",
        immediately: "true",
        comment: "flatten",
        alert_message: "flatten alert",
        disable_alert: "false",
      }),
      createNode("order-cancel", "placeOrder", "撤单", {
        orderAction: "cancel",
        orderId: "Long",
      }),
      createNode("order-cancel-all", "placeOrder", "全部撤单", {
        orderAction: "cancelAll",
      }),
      createNode("order-risk-allow", "placeOrder", "限制入场", {
        orderAction: "riskAllowEntryIn",
        riskAllowedDirection: "short",
      }),
      createNode("order-close-short", "placeOrder", "空头平仓", {
        orderAction: "close",
        side: "BUY_COVER",
        orderId: "Short",
        quantityMode: "amount",
        quantityValue: 1000,
        limitPriceExpressionAst: {
          kind: "source",
          source: "high",
        },
        stopPriceExpressionAst: {
          kind: "source",
          source: "low",
        },
        comment: "close short",
        alert_message: "close short alert",
        immediately: "true",
        disable_alert: "false",
        when: "barstate.isconfirmed",
      }),
      createNode("order-entry-long", "placeOrder", "多头入场", {
        orderAction: "entry",
        side: "BUY",
        orderId: "Long",
        quantityMode: "equityPercent",
        quantityValue: 12.5,
        orderType: "LIMIT",
        limitPriceExpressionAst: {
          kind: "source",
          source: "close",
        },
        stopPriceExpressionAst: {
          kind: "history",
          target: { kind: "source", source: "low" },
          offset: 1,
        },
        comment: "open long",
        alert_message: "open long alert",
        disable_alert: "true",
        when: "barstate.isconfirmed",
        entryPositionPolicy: "flatOnly",
      }),
      createNode("order-generic-short", "placeOrder", "通用做空单", {
        orderAction: "order",
        side: "SELL",
        orderId: "Short scalp",
        quantityMode: "shares",
        quantityValue: 3,
        orderType: "LIMIT",
        limitPriceExpressionAst: {
          kind: "source",
          source: "open",
        },
        stopPriceExpressionAst: {
          kind: "source",
          source: "high",
        },
        comment: "generic short",
        alert_message: "generic short alert",
        disable_alert: "false",
      }),

      createNode("risk-allow", "riskRule", "风控方向", {
        riskRuleType: "allowEntryIn",
        riskAllowedDirection: "long",
      }),
      createNode("risk-drawdown", "riskRule", "最大回撤", {
        riskRuleType: "maxDrawdown",
        riskValue: 8,
        riskAmountType: "strategy.cash",
        alert_message: "drawdown limit",
      }),
      createNode("risk-intraday-loss", "riskRule", "日内亏损", {
        riskRuleType: "maxIntradayLoss",
        riskValue: 5,
        riskAmountType: "strategy.percent_of_equity",
        alert_message: "intraday loss",
      }),
      createNode("risk-filled-orders", "riskRule", "成交笔数", {
        riskRuleType: "maxIntradayFilledOrders",
        riskCount: 4,
        alert_message: "filled orders",
      }),
      createNode("risk-loss-days", "riskRule", "连续亏损日", {
        riskRuleType: "maxConsLossDays",
        riskCount: 3,
        alert_message: "loss days",
      }),
      createNode("risk-position-size", "riskRule", "持仓上限", {
        riskRuleType: "maxPositionSize",
        riskContracts: 2,
      }),
      createNode("risk-fallback", "riskRule", "默认风控", {
        riskRuleType: "unsupported",
      }),

      createNode("exit-take-profit", "stopLoss", "止盈退出", {
        mode: "takeProfit",
        direction: "long",
        timeValue: 1,
        timeUnit: "bar",
        windowPolicy: "continuous",
        quantityPercentage: 50,
        takeProfitPriceExpressionAst: {
          kind: "source",
          source: "high",
        },
        comment_profit: "tp hit",
      }),
      createNode("exit-profit-ticks", "stopLoss", "止盈点数", {
        mode: "takeProfit",
        direction: "short",
        timeValue: 1,
        timeUnit: "bar",
        windowPolicy: "continuous",
        profitTicks: 60,
      }),
      createNode("exit-trailing", "stopLoss", "移动止损", {
        mode: "trailingStop",
        direction: "long",
        timeValue: 1,
        timeUnit: "bar",
        windowPolicy: "continuous",
        trailingPriceMode: "price",
        trailingPriceExpressionAst: {
          kind: "source",
          source: "close",
        },
        trailingOffsetExpressionAst: {
          kind: "literal",
          value: 1.5,
        },
      }),
      createNode("exit-bracket", "stopLoss", "Bracket Exit", {
        mode: "bracketExit",
        direction: "short",
        timeValue: 1,
        timeUnit: "bar",
        windowPolicy: "continuous",
        stopPriceExpressionAst: {
          kind: "source",
          source: "high",
        },
        takeProfitPriceExpressionAst: {
          kind: "source",
          source: "low",
        },
        quantityPercentage: 75,
      }),
      createNode("exit-stop-loss", "stopLoss", "止损点数", {
        mode: "stopLoss",
        direction: "short",
        timeValue: 1,
        timeUnit: "bar",
        windowPolicy: "continuous",
        lossTicks: 25,
        alert_loss: "loss hit",
      }),
    ];

    const klineSequence = [
      "state-var",
      "state-update",
      "derived-nz",
      "derived-max",
      "derived-abs",
      "derived-arithmetic",
      "derived-cross",
      "derived-history",
      "mtf-source",
      "mtf-history",
      "mtf-indicator",
      "collection-avg",
      "collection-percentile",
      "ema-fast",
      "sma-slow",
      "smma-node",
      "lwma-node",
      "hma-node",
      "vwma-node",
      "macd-node",
      "kdj-node",
      "rsi-node",
      "boll-node",
      "atr-node",
      "cci-node",
      "willr-node",
      "stdev-node",
      "variance-node",
      "highest-node",
      "lowest-node",
      "sum-node",
      "vwap-node",
      "mfi-node",
      "dmi-node",
      "supertrend-node",
      "linreg-node",
      "sar-node",
      "pivot-high-node",
      "pivot-low-node",
      "keltner-node",
      "alma-node",
      "time-after",
      "time-before",
      "time-day",
      "time-between",
      "session-market",
      "session-pre",
      "session-post",
      "close-above",
      "close-below",
      "series-barssince",
      "series-valuewhen",
      "ma-cross",
      "macd-divergence",
      "rsi-divergence",
      "boll-break",
      "macd-numeric",
      "kdj-numeric",
      "dmi-numeric",
      "supertrend-numeric",
      "keltner-numeric",
      "order-close-all",
      "order-cancel",
      "order-cancel-all",
      "order-risk-allow",
      "order-close-short",
      "order-entry-long",
      "order-generic-short",
      "risk-allow",
      "risk-drawdown",
      "risk-intraday-loss",
      "risk-filled-orders",
      "risk-loss-days",
      "risk-position-size",
      "risk-fallback",
      "exit-take-profit",
      "exit-profit-ticks",
      "exit-trailing",
      "exit-bracket",
      "exit-stop-loss",
    ];

    const edges = [
      ...klineSequence.map((id) => controlEdge("kline-root", id)),
      controlEdge("time-between", "between-log", "true"),
      controlEdge("time-between", "between-notify", "false"),
      controlEdge("close-above", "close-log", "true"),
      controlEdge("close-above", "close-notify", "false"),
      dataEdge("ema-fast", "ma-cross", "fast"),
      dataEdge("sma-slow", "ma-cross", "slow"),
      dataEdge("macd-node", "macd-divergence", "primary"),
      dataEdge("rsi-node", "rsi-divergence", "primary"),
      dataEdge("boll-node", "boll-break", "primary"),
      dataEdge("dmi-node", "dmi-numeric", "primary"),
      dataEdge("supertrend-node", "supertrend-numeric", "primary"),
      dataEdge("keltner-node", "keltner-numeric", "primary"),
    ];

    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes,
      edges,
    };

    const script = buildStrategyPineFromVisualModel(model, {
      name: "Advanced Strategy Flow",
    });

    expect(script).toContain("log.info(\"入口图块暂无动作\")");
    expect(script).toContain("safe_close = nz(close, 0)");
    expect(script).toContain("range_high = math.max(high, open)");
    expect(script).toContain("range_abs = math.abs(close)");
    expect(script).toContain("cross_up = ta.crossover(close, open)");
    expect(script).toContain(
      "weekly_macd = request.security(syminfo.tickerid, \"60\", ta.macd(close, 12, 26, 9).histogram)",
    );
    expect(script).toContain(
      "range_p90 = array.from(low, close, high).percentile_linear_interpolation(90)",
    );
    expect(script).toContain(
      "ema_fast = request.security(syminfo.tickerid, \"60\", ta.ema(close, 9))",
    );
    expect(script).toContain("smma_value = ta.rma(hl2, 14)");
    expect(script).toContain("lwma_value = ta.wma(close, 10)");
    expect(script).toContain("hma_value = ta.hma(close, 16)");
    expect(script).toContain("vwma_value = ta.vwma(close, 12)");
    expect(script).toContain("if ta.crossunder(ema_fast, sma_slow)");
    expect(script).toContain("if divergence_top(macd_signal, 7)");
    expect(script).toContain("if divergence_bottom(rsi_fast, 6)");
    expect(script).toContain("if close > boll_band.upper");
    expect(script).toContain("if macd_signal.histogram > 0");
    expect(script).toContain("if kdj_signal_j < 20");
    expect(script).toContain("if dmi_strength.adx > 25");
    expect(script).toContain("if supertrend_signal.direction > 0");
    expect(script).toContain("if keltner_band.upper > 120");
    expect(script).toContain("if (hour * 60 + minute) >= 585");
    expect(script).toContain("if (hour * 60 + minute) < 930");
    expect(script).toContain("if dayofweek == 4");
    expect(script).toContain("if session.ispremarket");
    expect(script).toContain("if session.ispostmarket");
    expect(script).toContain("strategy.close_all(");
    expect(script).toContain("strategy.cancel(\"Long\")");
    expect(script).toContain("strategy.cancel_all()");
    expect(script).toContain(
      "strategy.risk.allow_entry_in(strategy.direction.short)",
    );
    expect(script).toContain("strategy.close(\"Short\"");
    expect(script).toContain("// @entry_policy flat_only");
    expect(script).toContain("strategy.entry(\"Long\", strategy.long");
    expect(script).toContain("strategy.order(\"Short scalp\", strategy.short");
    expect(script).toContain(
      "strategy.risk.max_drawdown(8, strategy.cash, alert_message=\"drawdown limit\")",
    );
    expect(script).toContain(
      "strategy.risk.max_intraday_loss(5, strategy.percent_of_equity, alert_message=\"intraday loss\")",
    );
    expect(script).toContain(
      "strategy.risk.max_intraday_filled_orders(4, alert_message=\"filled orders\")",
    );
    expect(script).toContain(
      "strategy.risk.max_cons_loss_days(3, alert_message=\"loss days\")",
    );
    expect(script).toContain("strategy.risk.max_position_size(2)");
    expect(script).toContain(
      "strategy.risk.max_drawdown(10, strategy.percent_of_equity)",
    );
    expect(script).toContain(
      "strategy.exit(\"Long takeProfit\", \"Long\", limit=high, qty_percent=50, comment_profit=\"tp hit\")",
    );
    expect(script).toContain(
      "strategy.exit(\"Short takeProfit\", \"Short\", profit=60)",
    );
    expect(script).toContain("trail_price=close, trail_offset=1.5");
    expect(script).toContain("strategy.exit(\"Short bracketExit\", \"Short\", stop=high, limit=low, qty_percent=75)");
    expect(script).toContain(
      "strategy.exit(\"Short stopLoss\", \"Short\", loss=25, alert_loss=\"loss hit\")",
    );

    const parsed = buildStrategyVisualModelFromPine(script);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) {
      return;
    }

    const kinds = parsed.model.nodes.map((node) =>
      String(node.properties.blockKind),
    );
    expect(kinds).toEqual(
      expect.arrayContaining([
        "strategyInput",
        "stateVariable",
        "stateUpdate",
        "derivedSeries",
        "mtfSeries",
        "collectionStat",
        "getTechnicalIndicator",
        "timeFilter",
        "sessionFilter",
        "ifCloseAbove",
        "ifCloseBelow",
        "seriesCondition",
        "technicalIndicatorCondition",
        "placeOrder",
        "riskRule",
        "stopLoss",
      ]),
    );
    expect(
      parsed.model.edges.some((edge) => edge.properties?.role === "data"),
    ).toBe(true);
    expect(
      parsed.model.edges.some((edge) => edge.properties?.branch === "false"),
    ).toBe(true);

    expect(
      parsed.model.nodes.find(
        (node) =>
          node.properties.blockKind === "timeFilter"
          && node.properties.mode === "between",
      )?.properties,
    ).toMatchObject({
      blockKind: "timeFilter",
      mode: "between",
    });
    expect(
      parsed.model.nodes.find(
        (node) =>
          node.properties.blockKind === "technicalIndicatorCondition"
          && node.properties.patternType === "topDivergence",
      )?.properties,
    ).toMatchObject({
      blockKind: "technicalIndicatorCondition",
      patternType: "topDivergence",
      lookback: 7,
    });
    expect(
      parsed.model.nodes.find(
        (node) =>
          node.properties.blockKind === "placeOrder"
          && node.properties.orderAction === "closeAll",
      )?.properties,
    ).toMatchObject({
      blockKind: "placeOrder",
      orderAction: "closeAll",
      immediately: true,
    });
    expect(
      parsed.model.nodes.find(
        (node) =>
          node.properties.blockKind === "riskRule"
          && node.properties.riskRuleType === "maxDrawdown",
      )?.properties,
    ).toMatchObject({
      blockKind: "riskRule",
      riskRuleType: "maxDrawdown",
      riskAmountType: "strategy.cash",
    });
    expect(
      parsed.model.nodes.find(
        (node) =>
          node.properties.blockKind === "stopLoss"
          && node.properties.mode === "trailingStop",
      )?.properties,
    ).toMatchObject({
      blockKind: "stopLoss",
      mode: "trailingStop",
      trailingPriceMode: "price",
    });
  });

  it("renders fallback conditions for unsupported indicator inputs and timed exits", () => {
    const model: StrategyVisualModelDocument = {
      engine: "logic-flow",
      version: 1,
      nodes: [
        createRoot("root", "onKLineClosed", "K 线收盘"),
        createNode(
          "missing-ma-condition",
          "technicalIndicatorCondition",
          "缺失均线输入",
          {
            indicatorType: "movingAverage",
            conditionMode: "pattern",
            patternType: "goldenCross",
          },
          "diamond",
        ),
        createNode(
          "missing-rsi-condition",
          "technicalIndicatorCondition",
          "缺失 RSI 输入",
          {
            indicatorType: "rsi",
            conditionMode: "pattern",
            patternType: "topDivergence",
          },
          "diamond",
        ),
        createNode("timed-exit", "stopLoss", "窗口退出", {
          mode: "stopLoss",
          direction: "long",
          timeValue: 2,
          timeUnit: "day",
          windowPolicy: "session",
        }),
      ],
      edges: [
        controlEdge("root", "missing-ma-condition"),
        controlEdge("root", "missing-rsi-condition"),
        controlEdge("root", "timed-exit"),
      ],
    };

    const script = buildStrategyPineFromVisualModel(model, {
      name: "Fallback Flow",
    });

    expect(script).toContain("if false");
    expect(script).toContain(
      'runtime.error("JFTrade Pine 暂不支持带时间窗口或交易时段感知的自动退出图块")',
    );
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
