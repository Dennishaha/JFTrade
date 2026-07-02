import { describe, expect, it } from "vitest";

import {
  createStrategyPaletteItems,
  dayOfWeekLabel,
  getStrategyBlockCatalog,
  getStrategyBlockDefinition,
  getStrategyBlockKind,
  nextCollectionStatNodeText,
  nextDerivedSeriesNodeText,
  nextMtfSeriesNodeText,
  nextSeriesConditionNodeText,
  nextSessionFilterNodeText,
  nextStateUpdateNodeText,
  nextStateVariableNodeText,
  nextStopLossNodeText,
  nextStrategyInputNodeText,
  nextTimeFilterNodeText,
  normalizeCollectionStatBlockProperties,
  normalizeDerivedSeriesBlockProperties,
  normalizeMtfSeriesBlockProperties,
  normalizeSeriesConditionBlockProperties,
  normalizeSeriesConditionMode,
  normalizeSeriesConditionOperator,
  normalizeSeriesSource,
  normalizeSessionFilterBlockProperties,
  normalizeStateUpdateBlockProperties,
  normalizeStateVariableBlockProperties,
  normalizeStopLossBlockProperties,
  normalizeStopLossDirection,
  normalizeStopLossMode,
  normalizeStopLossTimeUnit,
  normalizeStopLossTrailingPriceMode,
  normalizeStopLossWindowPolicy,
  normalizeStrategyInputBlockProperties,
  normalizeTimeFilterBlockProperties,
  seriesSourceLabel,
  sessionScopeLabel,
  stopLossDirectionLabel,
  stopLossModeLabel,
  stopLossRuleLabel,
  stopLossTimeUnitLabel,
  stopLossWindowPolicyLabel,
} from "../src/features/strategyVisualBuilderCatalog";
import {
  buildAtrIndicatorKey,
  buildBollingerIndicatorKey,
  buildCciIndicatorKey,
  buildDivergenceIndicatorKey,
  buildHookPrelude,
  buildKdjIndicatorKey,
  buildMacdIndicatorKey,
  buildMovingAverageIndicatorKey,
  buildRsiIndicatorKey,
  buildScriptRuntimeBlocks,
  buildStopLossIndicatorKey,
  buildWilliamsRIndicatorKey,
  entryPositionPolicyLabel,
  entryPositionPolicyToSnakeCase,
  isQuantityModeAllowedForSide,
  normalizeDecimal,
  normalizeEntryPositionPolicy,
  normalizeMessage,
  normalizeOrderSide,
  normalizeOrderType,
  normalizePineOrderAction,
  normalizePineRiskAllowEntryDirection,
  normalizeQuantityMode,
  normalizeQuantityModeForSide,
  normalizeThreshold,
  normalizeWindowSize,
  orderSideForExchange,
  orderSideLabel,
  toConsoleLogArgument,
  toScriptMessage,
  type StrategyScriptRuntimeFlags,
} from "../src/features/strategyVisualBuilderScriptSupport";

const noRuntimeFlags: StrategyScriptRuntimeFlags = {
  usesMovingAverageRuntime: false,
  usesRSIRuntime: false,
  usesMACDRuntime: false,
  usesKDJRuntime: false,
  usesATRRuntime: false,
  usesCCIRuntime: false,
  usesWilliamsRRuntime: false,
  usesBollingerRuntime: false,
  usesSimpleMovingAverageHelper: false,
  usesSeriesStateRuntime: false,
  usesDivergenceRuntime: false,
};

describe("strategy visual builder business boundaries", () => {
  it("builds runtime state only for closed-candle hooks and every enabled indicator family", () => {
    expect(buildHookPrelude("onInit", noRuntimeFlags)).toEqual([]);
    expect(buildHookPrelude("onKLineClosed", noRuntimeFlags)).toEqual([]);

    const lines = buildHookPrelude("onKLineClosed", {
      ...noRuntimeFlags,
      usesSeriesStateRuntime: true,
      usesMovingAverageRuntime: true,
      usesRSIRuntime: true,
      usesMACDRuntime: true,
      usesKDJRuntime: true,
      usesATRRuntime: true,
      usesCCIRuntime: true,
      usesWilliamsRRuntime: true,
      usesBollingerRuntime: true,
      usesDivergenceRuntime: true,
    });
    const source = lines.join("\n");
    expect(source).toContain("ctx.kline.close");
    expect(source).toContain("fastAverageSnapshot");
    expect(source).toContain("latestRsi");
    expect(source).toContain("latestMacdHistogram");
    expect(source).toContain("previousDValue");
    expect(source).toContain("latestAtr");
    expect(source).toContain("latestCci");
    expect(source).toContain("latestWilliamsR");
    expect(source).toContain("latestBollingerUpper");
    expect(source).toContain("divergenceSignal");
    expect(buildScriptRuntimeBlocks(noRuntimeFlags)).toEqual([]);
  });

  it("uses stable indicator cache keys across chart timeframes and risk modes", () => {
    expect(buildMovingAverageIndicatorKey(20)).toBe("ma:MA:20");
    expect(buildMovingAverageIndicatorKey(20, "EMA", "1")).toBe("ma:EMA:20:minute");
    expect(buildMovingAverageIndicatorKey(20, "EMA", "60")).toBe("ma:EMA:20:hour");
    expect(buildMovingAverageIndicatorKey(20, "EMA", "D")).toBe("ma:EMA:20:day");
    expect(buildMovingAverageIndicatorKey(20, "EMA", "W")).toBe("ma:EMA:20:week");
    expect(buildMovingAverageIndicatorKey(20, "EMA", "M")).toBe("ma:EMA:20:month");
    expect(buildMovingAverageIndicatorKey(20, "EMA", "15")).toBe("ma:EMA:20:15m");
    expect(buildStopLossIndicatorKey("long", 3, "bar", 2)).toBe("sl:long:3:bar:2");
    expect(buildStopLossIndicatorKey("auto", 2, "day", 4, "trailingStop", "session"))
      .toBe("risk:trailingStop:auto:2:day:4:session");
    expect(buildRsiIndicatorKey(14)).toBe("rsi:14");
    expect(buildMacdIndicatorKey(12, 26, 9)).toBe("macd:12:26:9");
    expect(buildBollingerIndicatorKey(20, 2)).toBe("bollinger:20:2");
    expect(buildKdjIndicatorKey(9, 3, 3)).toBe("kdj:9:3:3");
    expect(buildAtrIndicatorKey(14)).toBe("atr:14");
    expect(buildCciIndicatorKey(20)).toBe("cci:20");
    expect(buildWilliamsRIndicatorKey(14)).toBe("williamsr:14");
    expect(buildDivergenceIndicatorKey("macd", [12, 26, 9], "top", 50))
      .toBe("divergence:macd:12:26:9:top:50");
  });

  it("normalizes user-authored order and numeric values without changing trading intent", () => {
    expect(normalizeMessage("  signal  ", "fallback")).toBe("signal");
    expect(normalizeMessage(" ", "fallback")).toBe("fallback");
    expect(normalizeThreshold(2.5, 1)).toBe(2.5);
    expect(normalizeThreshold("3.5", 1)).toBe(3.5);
    expect(normalizeThreshold("bad", 1)).toBe(1);
    expect(normalizeWindowSize(2.6, 10)).toBe(3);
    expect(normalizeWindowSize("0", 10)).toBe(1);
    expect(normalizeWindowSize(null, 10)).toBe(10);
    expect(normalizeDecimal(2.25, 1)).toBe(2.25);
    expect(normalizeDecimal("4.5", 1)).toBe(4.5);
    expect(normalizeDecimal({}, 1)).toBe(1);

    for (const side of ["BUY", "SELL", "SELL_SHORT", "BUY_COVER"] as const) {
      expect(normalizeOrderSide(side)).toBe(side);
      expect(orderSideLabel(side)).not.toBe("");
    }
    expect(normalizeOrderSide("invalid")).toBe("BUY");
    expect(orderSideForExchange("SELL_SHORT")).toBe("SELL");
    expect(orderSideForExchange("BUY_COVER")).toBe("BUY");
    expect(normalizeOrderType("LIMIT")).toBe("LIMIT");
    expect(normalizeOrderType("STOP")).toBe("MARKET");

    for (const action of ["entry", "order", "close", "closeAll", "cancel", "cancelAll", "riskAllowEntryIn"] as const) {
      expect(normalizePineOrderAction(action)).toBe(action);
    }
    expect(normalizePineOrderAction("invalid")).toBe("entry");
    expect(normalizePineRiskAllowEntryDirection("long")).toBe("long");
    expect(normalizePineRiskAllowEntryDirection("short")).toBe("short");
    expect(normalizePineRiskAllowEntryDirection("invalid")).toBe("all");

    expect(normalizeEntryPositionPolicy("flatOnly")).toBe("flatOnly");
    expect(normalizeEntryPositionPolicy("allow")).toBe("allow");
    expect(normalizeEntryPositionPolicy("invalid")).toBe("sameDirection");
    expect(entryPositionPolicyLabel("flatOnly")).toBe("必须空仓");
    expect(entryPositionPolicyLabel("allow")).toBe("允许加仓");
    expect(entryPositionPolicyLabel("sameDirection")).toBe("拦截同向加仓");
    expect(entryPositionPolicyToSnakeCase("flatOnly")).toBe("flat_only");
    expect(entryPositionPolicyToSnakeCase("allow")).toBe("allow");
    expect(entryPositionPolicyToSnakeCase("sameDirection")).toBe("same_direction");

    expect(normalizeQuantityMode("shares")).toBe("shares");
    expect(normalizeQuantityMode("amount")).toBe("amount");
    expect(normalizeQuantityMode("equityPercent")).toBe("equityPercent");
    expect(normalizeQuantityMode("accountPositionPercent")).toBe("equityPercent");
    expect(normalizeQuantityMode("invalid")).toBe("shares");
    expect(isQuantityModeAllowedForSide("amount", "SELL_SHORT")).toBe(true);
    expect(normalizeQuantityModeForSide("amount", "SELL_SHORT")).toBe("amount");
  });

  it("escapes notification templates while preserving pure log expressions", () => {
    expect(toScriptMessage("plain message")).toBe('"plain message"');
    expect(toScriptMessage("price `${close}`")).toBe("`price \\`${close}\\``");
    expect(toConsoleLogArgument("${ctx.kline.close}")).toBe("ctx.kline.close");
    expect(toConsoleLogArgument("${   }")).toBe("`${   }`");
    expect(toConsoleLogArgument("signal ${close}")).toBe("`signal ${close}`");
  });

  it("normalizes every structured visual block and produces operator-readable summaries", () => {
    expect(normalizeSeriesSource("high")).toBe("high");
    expect(normalizeSeriesSource("bad", "open")).toBe("open");
    expect(normalizeSeriesConditionMode("rising")).toBe("rising");
    expect(normalizeSeriesConditionMode("bad")).toBe("compare");
    expect(normalizeSeriesConditionOperator("<")).toBe("<");
    expect(normalizeSeriesConditionOperator("=")).toBe(">");

    expect(normalizeSeriesConditionBlockProperties({
      mode: "valuewhen", source: "high", operator: "<", threshold: "10.5", length: "4",
      eventSource: "low", eventOperator: "<", eventThreshold: 8, valueSource: "open", occurrence: 2,
    })).toMatchObject({ mode: "valuewhen", source: "high", threshold: 10.5, length: 4, occurrence: 2 });
    expect(normalizeStrategyInputBlockProperties({ variableName: " Period ", inputType: "float", title: " Length ", defaultValue: "2.5" }))
      .toMatchObject({ variableName: "Period", inputType: "float", title: "Length", defaultValue: 2.5 });
    expect(normalizeStrategyInputBlockProperties({ inputType: "bad", defaultValue: "bad" }))
      .toMatchObject({ inputType: "int", defaultValue: 20 });
    expect(normalizeDerivedSeriesBlockProperties({ mode: "arithmetic", variableName: "spread", operator: "/", leftExpression: "high", rightExpression: "low" }))
      .toMatchObject({ mode: "arithmetic", variableName: "spread", operator: "/" });
    expect(normalizeDerivedSeriesBlockProperties({ mode: "bad", mathFunction: "bad", crossFunction: "bad" }))
      .toMatchObject({ mode: "history", mathFunction: "max", crossFunction: "crossover" });
    expect(normalizeMtfSeriesBlockProperties({ variableName: "daily_close", timeframe: "D", expressionType: "indicator", indicatorExpression: "ta.ema(close, 20)", mtfField: "histogram" }))
      .toMatchObject({ variableName: "daily_close", timeframe: "D", expressionType: "indicator", mtfField: "histogram" });
    expect(normalizeStateVariableBlockProperties({ variableName: "armed", valueType: "string", initialValue: 5 }))
      .toMatchObject({ valueType: "string", initialValue: "" });
    expect(normalizeStateUpdateBlockProperties({ variableName: "armed", expression: "close > open" }))
      .toMatchObject({ variableName: "armed", expression: "close > open" });
    expect(normalizeCollectionStatBlockProperties({ statFunction: "percentile", sourceA: "low", sourceB: "high", sourceC: "close", percentile: 120 }))
      .toMatchObject({ statFunction: "percentile", percentile: 100 });
    expect(normalizeTimeFilterBlockProperties({ mode: "dayOfWeek", startHour: 30, startMinute: -2, endHour: 12, endMinute: 90, dayOfWeek: 9 }))
      .toMatchObject({ mode: "dayOfWeek", startHour: 23, startMinute: 0, endHour: 12, endMinute: 59, dayOfWeek: 7 });
    expect(normalizeSessionFilterBlockProperties({ scope: "premarket" })).toEqual({ blockKind: "sessionFilter", scope: "premarket" });

    expect(nextStrategyInputNodeText({ variableName: "period", inputType: "int", defaultValue: 20 })).toContain("period = 20");
    expect(nextDerivedSeriesNodeText({ variableName: "spread", mode: "arithmetic" })).toBe("派生 spread · arithmetic");
    expect(nextMtfSeriesNodeText({ variableName: "daily", timeframe: "D" })).toBe("MTF daily · D");
    expect(nextStateVariableNodeText({ variableName: "armed", valueType: "bool", initialValue: true })).toContain("armed = true");
    expect(nextStateUpdateNodeText({ variableName: "armed" })).toBe("更新状态 armed");
    expect(nextCollectionStatNodeText({ variableName: "avg", statFunction: "avg" })).toBe("集合统计 avg · avg");
    expect(nextTimeFilterNodeText({ mode: "dayOfWeek", dayOfWeek: 6 })).toBe("星期过滤 · 周五");
    expect(nextTimeFilterNodeText({ mode: "between", startHour: 9, startMinute: 30, endHour: 16, endMinute: 0 })).toContain("09:30-16:00");
    expect(nextSessionFilterNodeText({ scope: "postmarket" })).toBe("时段过滤 · 盘后");
    expect(nextSeriesConditionNodeText({ mode: "rising", source: "close", length: 3 })).toContain("连续上升");
    expect(nextSeriesConditionNodeText({ mode: "falling", source: "close", length: 3 })).toContain("连续下降");
    expect(nextSeriesConditionNodeText({ mode: "barssince" })).toContain("距 Close > 520");
    expect(nextSeriesConditionNodeText({ mode: "valuewhen" })).toContain("@");
    expect(nextSeriesConditionNodeText({ mode: "compare", source: "low", operator: "<", threshold: 5 })).toBe("Low < 5");
  });

  it("preserves advanced exit options, labels and independent palette copies", () => {
    const normalized = normalizeStopLossBlockProperties({
      mode: "bracketExit",
      direction: "short",
      timeValue: "3",
      timeUnit: "day",
      percentage: "2.5",
      takeProfitPercentage: 5,
      profitTicks: 20,
      lossTicks: 10,
      quantityPercentage: 50,
      windowPolicy: "session",
      trailingPriceMode: "price",
      fromEntryMode: "auto",
      stopPriceExpressionAst: { kind: "source", source: "low" },
      takeProfitPriceExpressionAst: { kind: "source", source: "high" },
      trailingPriceExpressionAst: { kind: "source", source: "close" },
      trailingOffsetExpressionAst: { kind: "literal", value: 2 },
      comment: " exit ",
      comment_profit: " profit ",
      comment_loss: " loss ",
      comment_trailing: " trail ",
      alert_message: " alert ",
      alert_profit: " alert profit ",
      alert_loss: " alert loss ",
      alert_trailing: " alert trail ",
      disable_alert: true,
      when: " close > open ",
    });
    expect(normalized).toMatchObject({
      mode: "bracketExit", direction: "short", timeValue: 3, timeUnit: "day", percentage: 2.5,
      takeProfitPercentage: 5, profitTicks: 20, lossTicks: 10, windowPolicy: "session",
      trailingPriceMode: "price", fromEntryMode: "auto", comment: "exit", disable_alert: true,
    });
    expect(normalizeStopLossMode("bad")).toBe("stopLoss");
    expect(normalizeStopLossDirection("long")).toBe("long");
    expect(normalizeStopLossDirection("bad")).toBe("auto");
    expect(normalizeStopLossTimeUnit("month")).toBe("month");
    expect(normalizeStopLossTimeUnit("bad")).toBe("bar");
    expect(normalizeStopLossWindowPolicy("session")).toBe("session");
    expect(normalizeStopLossWindowPolicy("bad")).toBe("continuous");
    expect(normalizeStopLossTrailingPriceMode("price")).toBe("price");
    expect(normalizeStopLossTrailingPriceMode("bad")).toBe("points");

    expect(stopLossModeLabel("takeProfit")).toBe("止盈");
    expect(stopLossModeLabel("trailingStop")).toBe("追踪止损");
    expect(stopLossModeLabel("bracketExit")).toBe("止盈止损");
    expect(stopLossModeLabel("stopLoss")).toBe("止损");
    expect(stopLossDirectionLabel("long")).toBe("多头");
    expect(stopLossDirectionLabel("short")).toBe("空头");
    expect(stopLossDirectionLabel("auto")).toBe("自动");
    for (const unit of ["bar", "minute", "hour", "day", "week", "month"] as const) {
      expect(stopLossTimeUnitLabel(unit)).not.toBe("");
    }
    expect(stopLossWindowPolicyLabel("session")).toBe("交易时段感知");
    expect(stopLossWindowPolicyLabel("continuous")).toBe("连续窗口");
    expect(stopLossRuleLabel({ blockKind: "stopLoss", mode: "takeProfit", percentage: 4 })).toContain("顺向");
    expect(stopLossRuleLabel({ blockKind: "stopLoss", mode: "trailingStop", percentage: 3 })).toContain("回撤");
    expect(stopLossRuleLabel({ blockKind: "stopLoss", mode: "bracketExit", percentage: 2, takeProfitPercentage: 5 })).toContain("或顺向");
    expect(stopLossRuleLabel({ blockKind: "stopLoss", mode: "stopLoss", percentage: 2 })).toContain("反向波动");
    expect(nextStopLossNodeText(normalized)).toContain("空头止盈止损 3日 2.5% 时段感知");

    expect([1, 2, 3, 4, 5, 6, 7].map(dayOfWeekLabel)).toEqual(["周日", "周一", "周二", "周三", "周四", "周五", "周六"]);
    expect(sessionScopeLabel("premarket")).toBe("盘前");
    expect(sessionScopeLabel("postmarket")).toBe("盘后");
    expect(sessionScopeLabel("market")).toBe("常规交易时段");
    expect(seriesSourceLabel("ohlc4")).toBe("OHLC4");

    const catalog = getStrategyBlockCatalog();
    const copy = getStrategyBlockCatalog();
    catalog[0]!.properties.changed = true;
    expect(copy[0]!.properties.changed).toBeUndefined();
    expect(getStrategyBlockDefinition("stopLoss")?.kind).toBe("stopLoss");
    expect(getStrategyBlockDefinition("missing")).toBeNull();
    expect(getStrategyBlockKind({ id: "n", type: "rect", x: 0, y: 0, text: "", properties: { blockKind: "log" } })).toBe("log");
    expect(getStrategyBlockKind(null)).toBeNull();
    const palette = createStrategyPaletteItems();
    expect(palette.length).toBeGreaterThan(10);
    expect(palette[0]!.icon).toMatch(/^data:image\/svg\+xml/);
  });
});
