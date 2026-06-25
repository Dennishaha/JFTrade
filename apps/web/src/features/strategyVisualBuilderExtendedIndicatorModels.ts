import type { StrategyVisualModelDocument } from "@/contracts";

import {
  buildStrategyVisualControlEdgeProperties,
  buildStrategyVisualDataEdgeProperties,
} from "./strategyVisualBuilderEdges";

export function createKDJReversionStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "kdj-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "kdj-init-log", type: "rect", x: 450, y: 120, text: "输出日志", properties: { blockKind: "log", message: "KDJ 策略已初始化：${ctx.symbol || '00700'} ${ctx.interval || '1m'}" } },
      { id: "kdj-kline-root", type: "circle", x: 180, y: 320, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "kdj-getter", type: "rect", x: 470, y: 320, text: "获取 KDJ 9/3/3", properties: { blockKind: "getTechnicalIndicator", indicatorType: "kdj", period: 9, m1: 3, m2: 3 } },
      { id: "kdj-buy-signal", type: "diamond", x: 760, y: 260, text: "KDJ 金叉", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "kdj", conditionMode: "pattern", patternType: "goldenCross" } },
      { id: "kdj-buy-order", type: "rect", x: 1040, y: 210, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "kdj-sell-signal", type: "diamond", x: 760, y: 390, text: "KDJ 死叉", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "kdj", conditionMode: "pattern", patternType: "deathCross" } },
      { id: "kdj-sell-order", type: "rect", x: 1040, y: 390, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-kdj-init-log", type: "polyline", sourceNodeId: "kdj-init-root", targetNodeId: "kdj-init-log", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-kdj-getter", type: "polyline", sourceNodeId: "kdj-kline-root", targetNodeId: "kdj-getter", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-kdj-buy-signal", type: "polyline", sourceNodeId: "kdj-getter", targetNodeId: "kdj-buy-signal", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-kdj-buy-input", type: "polyline", sourceNodeId: "kdj-getter", targetNodeId: "kdj-buy-signal", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-kdj-buy-order", type: "polyline", sourceNodeId: "kdj-buy-signal", targetNodeId: "kdj-buy-order", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-kdj-sell-signal", type: "polyline", sourceNodeId: "kdj-buy-signal", targetNodeId: "kdj-sell-signal", properties: buildStrategyVisualControlEdgeProperties("false") },
      { id: "edge-kdj-sell-input", type: "polyline", sourceNodeId: "kdj-getter", targetNodeId: "kdj-sell-signal", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-kdj-sell-order", type: "polyline", sourceNodeId: "kdj-sell-signal", targetNodeId: "kdj-sell-order", properties: buildStrategyVisualControlEdgeProperties("true") },
    ],
  };
}

export function createATRVolatilityStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "atr-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "atr-init-log", type: "rect", x: 450, y: 120, text: "输出日志", properties: { blockKind: "log", message: "ATR 策略已初始化：${ctx.symbol || '00700'} ${ctx.interval || '1m'}" } },
      { id: "atr-kline-root", type: "circle", x: 180, y: 320, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "atr-getter", type: "rect", x: 470, y: 320, text: "获取 ATR 14", properties: { blockKind: "getTechnicalIndicator", indicatorType: "atr", period: 14 } },
      { id: "atr-high", type: "diamond", x: 740, y: 260, text: "ATR > 2", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "atr", conditionMode: "numeric", operator: ">", threshold: 2 } },
      { id: "atr-buy-order", type: "rect", x: 1010, y: 210, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "atr-low", type: "diamond", x: 740, y: 390, text: "ATR < 1", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "atr", conditionMode: "numeric", operator: "<", threshold: 1 } },
      { id: "atr-sell-order", type: "rect", x: 1010, y: 390, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-atr-init-log", type: "polyline", sourceNodeId: "atr-init-root", targetNodeId: "atr-init-log", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-atr-getter", type: "polyline", sourceNodeId: "atr-kline-root", targetNodeId: "atr-getter", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-atr-high", type: "polyline", sourceNodeId: "atr-getter", targetNodeId: "atr-high", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-atr-high-input", type: "polyline", sourceNodeId: "atr-getter", targetNodeId: "atr-high", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-atr-buy-order", type: "polyline", sourceNodeId: "atr-high", targetNodeId: "atr-buy-order", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-atr-low", type: "polyline", sourceNodeId: "atr-high", targetNodeId: "atr-low", properties: buildStrategyVisualControlEdgeProperties("false") },
      { id: "edge-atr-low-input", type: "polyline", sourceNodeId: "atr-getter", targetNodeId: "atr-low", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-atr-sell-order", type: "polyline", sourceNodeId: "atr-low", targetNodeId: "atr-sell-order", properties: buildStrategyVisualControlEdgeProperties("true") },
    ],
  };
}

export function createCCIReversionStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "cci-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "cci-init-log", type: "rect", x: 450, y: 120, text: "输出日志", properties: { blockKind: "log", message: "CCI 策略已初始化：${ctx.symbol || '00700'} ${ctx.interval || '1m'}" } },
      { id: "cci-kline-root", type: "circle", x: 180, y: 320, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "cci-getter", type: "rect", x: 470, y: 320, text: "获取 CCI 20", properties: { blockKind: "getTechnicalIndicator", indicatorType: "cci", period: 20 } },
      { id: "cci-buy-signal", type: "diamond", x: 740, y: 260, text: "CCI < -100", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "cci", conditionMode: "numeric", operator: "<", threshold: -100 } },
      { id: "cci-buy-order", type: "rect", x: 1010, y: 210, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "cci-sell-signal", type: "diamond", x: 740, y: 390, text: "CCI > 100", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "cci", conditionMode: "numeric", operator: ">", threshold: 100 } },
      { id: "cci-sell-order", type: "rect", x: 1010, y: 390, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-cci-init-log", type: "polyline", sourceNodeId: "cci-init-root", targetNodeId: "cci-init-log", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-cci-getter", type: "polyline", sourceNodeId: "cci-kline-root", targetNodeId: "cci-getter", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-cci-buy-signal", type: "polyline", sourceNodeId: "cci-getter", targetNodeId: "cci-buy-signal", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-cci-buy-input", type: "polyline", sourceNodeId: "cci-getter", targetNodeId: "cci-buy-signal", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-cci-buy-order", type: "polyline", sourceNodeId: "cci-buy-signal", targetNodeId: "cci-buy-order", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-cci-sell-signal", type: "polyline", sourceNodeId: "cci-buy-signal", targetNodeId: "cci-sell-signal", properties: buildStrategyVisualControlEdgeProperties("false") },
      { id: "edge-cci-sell-input", type: "polyline", sourceNodeId: "cci-getter", targetNodeId: "cci-sell-signal", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-cci-sell-order", type: "polyline", sourceNodeId: "cci-sell-signal", targetNodeId: "cci-sell-order", properties: buildStrategyVisualControlEdgeProperties("true") },
    ],
  };
}

export function createWilliamsRReversionStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "wr-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "wr-init-log", type: "rect", x: 450, y: 120, text: "输出日志", properties: { blockKind: "log", message: "Williams %R 策略已初始化：${ctx.symbol || '00700'} ${ctx.interval || '1m'}" } },
      { id: "wr-kline-root", type: "circle", x: 180, y: 320, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "wr-getter", type: "rect", x: 470, y: 320, text: "获取 Williams %R 14", properties: { blockKind: "getTechnicalIndicator", indicatorType: "williamsR", period: 14 } },
      { id: "wr-buy-signal", type: "diamond", x: 770, y: 260, text: "Williams %R < -80", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "williamsR", conditionMode: "numeric", operator: "<", threshold: -80 } },
      { id: "wr-buy-order", type: "rect", x: 1070, y: 210, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "wr-sell-signal", type: "diamond", x: 770, y: 390, text: "Williams %R > -20", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "williamsR", conditionMode: "numeric", operator: ">", threshold: -20 } },
      { id: "wr-sell-order", type: "rect", x: 1070, y: 390, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-wr-init-log", type: "polyline", sourceNodeId: "wr-init-root", targetNodeId: "wr-init-log", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-wr-getter", type: "polyline", sourceNodeId: "wr-kline-root", targetNodeId: "wr-getter", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-wr-buy-signal", type: "polyline", sourceNodeId: "wr-getter", targetNodeId: "wr-buy-signal", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-wr-buy-input", type: "polyline", sourceNodeId: "wr-getter", targetNodeId: "wr-buy-signal", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-wr-buy-order", type: "polyline", sourceNodeId: "wr-buy-signal", targetNodeId: "wr-buy-order", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-wr-sell-signal", type: "polyline", sourceNodeId: "wr-buy-signal", targetNodeId: "wr-sell-signal", properties: buildStrategyVisualControlEdgeProperties("false") },
      { id: "edge-wr-sell-input", type: "polyline", sourceNodeId: "wr-getter", targetNodeId: "wr-sell-signal", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-wr-sell-order", type: "polyline", sourceNodeId: "wr-sell-signal", targetNodeId: "wr-sell-order", properties: buildStrategyVisualControlEdgeProperties("true") },
    ],
  };
}

export function createMFIReversionStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "mfi-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "mfi-init-log", type: "rect", x: 450, y: 120, text: "输出日志", properties: { blockKind: "log", message: "MFI 策略已初始化：${ctx.symbol || '00700'} ${ctx.interval || '5m'}" } },
      { id: "mfi-kline-root", type: "circle", x: 180, y: 320, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "mfi-getter", type: "rect", x: 470, y: 320, text: "获取 MFI 14", properties: { blockKind: "getTechnicalIndicator", indicatorType: "mfi", source: "hlc3", period: 14 } },
      { id: "mfi-buy-signal", type: "diamond", x: 740, y: 260, text: "MFI < 20", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "mfi", conditionMode: "numeric", operator: "<", threshold: 20 } },
      { id: "mfi-buy-order", type: "rect", x: 1010, y: 210, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "mfi-stop", type: "rect", x: 1280, y: 210, text: "自动止损 1柱 2%", properties: { blockKind: "stopLoss", mode: "stopLoss", direction: "long", timeValue: 1, timeUnit: "bar", percentage: 2, windowPolicy: "continuous" } },
      { id: "mfi-sell-signal", type: "diamond", x: 740, y: 390, text: "MFI > 80", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "mfi", conditionMode: "numeric", operator: ">", threshold: 80 } },
      { id: "mfi-sell-order", type: "rect", x: 1010, y: 390, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-mfi-init-log", type: "polyline", sourceNodeId: "mfi-init-root", targetNodeId: "mfi-init-log", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-mfi-getter", type: "polyline", sourceNodeId: "mfi-kline-root", targetNodeId: "mfi-getter", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-mfi-buy-signal", type: "polyline", sourceNodeId: "mfi-getter", targetNodeId: "mfi-buy-signal", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-mfi-buy-input", type: "polyline", sourceNodeId: "mfi-getter", targetNodeId: "mfi-buy-signal", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-mfi-buy-order", type: "polyline", sourceNodeId: "mfi-buy-signal", targetNodeId: "mfi-buy-order", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-mfi-stop", type: "polyline", sourceNodeId: "mfi-buy-order", targetNodeId: "mfi-stop", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-mfi-sell-signal", type: "polyline", sourceNodeId: "mfi-buy-signal", targetNodeId: "mfi-sell-signal", properties: buildStrategyVisualControlEdgeProperties("false") },
      { id: "edge-mfi-sell-input", type: "polyline", sourceNodeId: "mfi-getter", targetNodeId: "mfi-sell-signal", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-mfi-sell-order", type: "polyline", sourceNodeId: "mfi-sell-signal", targetNodeId: "mfi-sell-order", properties: buildStrategyVisualControlEdgeProperties("true") },
    ],
  };
}

export function createSupertrendStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "supertrend-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "supertrend-init-log", type: "rect", x: 500, y: 120, text: "输出日志", properties: { blockKind: "log", message: "Supertrend 策略已初始化：${ctx.symbol || '00700'} ${ctx.interval || '5m'}" } },
      { id: "supertrend-kline-root", type: "circle", x: 180, y: 330, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "supertrend-getter", type: "rect", x: 500, y: 330, text: "获取 Supertrend 3/10", properties: { blockKind: "getTechnicalIndicator", indicatorType: "supertrend", factor: 3, period: 10 } },
      { id: "supertrend-buy-signal", type: "diamond", x: 830, y: 270, text: "Supertrend > 0", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "supertrend", conditionMode: "numeric", operator: ">", threshold: 0 } },
      { id: "supertrend-buy-order", type: "rect", x: 1160, y: 220, text: "下单 · 买入开多 · 100 股", properties: { blockKind: "placeOrder", side: "BUY", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
      { id: "supertrend-stop", type: "rect", x: 1470, y: 220, text: "自动止损 1柱 3%", properties: { blockKind: "stopLoss", mode: "stopLoss", direction: "long", timeValue: 1, timeUnit: "bar", percentage: 3, windowPolicy: "continuous" } },
      { id: "supertrend-sell-signal", type: "diamond", x: 830, y: 410, text: "Supertrend < 0", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "supertrend", conditionMode: "numeric", operator: "<", threshold: 0 } },
      { id: "supertrend-sell-order", type: "rect", x: 1160, y: 410, text: "下单 · 卖出平多 · 100 股", properties: { blockKind: "placeOrder", side: "SELL", orderType: "MARKET", quantityMode: "shares", quantityValue: 100 } },
    ],
    edges: [
      { id: "edge-supertrend-init-log", type: "polyline", sourceNodeId: "supertrend-init-root", targetNodeId: "supertrend-init-log", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-supertrend-getter", type: "polyline", sourceNodeId: "supertrend-kline-root", targetNodeId: "supertrend-getter", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-supertrend-buy-signal", type: "polyline", sourceNodeId: "supertrend-getter", targetNodeId: "supertrend-buy-signal", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-supertrend-buy-input", type: "polyline", sourceNodeId: "supertrend-getter", targetNodeId: "supertrend-buy-signal", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-supertrend-buy-order", type: "polyline", sourceNodeId: "supertrend-buy-signal", targetNodeId: "supertrend-buy-order", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-supertrend-stop", type: "polyline", sourceNodeId: "supertrend-buy-order", targetNodeId: "supertrend-stop", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-supertrend-sell-signal", type: "polyline", sourceNodeId: "supertrend-buy-signal", targetNodeId: "supertrend-sell-signal", properties: buildStrategyVisualControlEdgeProperties("false") },
      { id: "edge-supertrend-sell-input", type: "polyline", sourceNodeId: "supertrend-getter", targetNodeId: "supertrend-sell-signal", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-supertrend-sell-order", type: "polyline", sourceNodeId: "supertrend-sell-signal", targetNodeId: "supertrend-sell-order", properties: buildStrategyVisualControlEdgeProperties("true") },
    ],
  };
}

export function createMTFMomentumStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "mtf-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "mtf-init-log", type: "rect", x: 500, y: 120, text: "输出日志", properties: { blockKind: "log", message: "MTF 动能策略已初始化：${ctx.symbol || '00700'} ${ctx.interval || '5m'}" } },
      { id: "mtf-kline-root", type: "circle", x: 180, y: 330, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "mtf-ema", type: "rect", x: 500, y: 290, text: "获取 EMA 20日", properties: { blockKind: "getTechnicalIndicator", indicatorType: "movingAverage", movingAverageType: "EMA", source: "close", windowSize: 20, timeframe: "D" } },
      { id: "mtf-macd", type: "rect", x: 500, y: 420, text: "获取 MACD 12/26/9", properties: { blockKind: "getTechnicalIndicator", indicatorType: "macd", fastPeriod: 12, slowPeriod: 26, signalPeriod: 9 } },
      { id: "mtf-macd-cross", type: "diamond", x: 820, y: 330, text: "MACD 金叉", properties: { blockKind: "technicalIndicatorCondition", indicatorType: "macd", conditionMode: "pattern", patternType: "goldenCross" } },
      { id: "mtf-buy-order", type: "rect", x: 1130, y: 330, text: "下单 · 买入开多 · 10%", properties: { blockKind: "placeOrder", orderAction: "entry", orderId: "Long", side: "BUY", orderType: "MARKET", quantityMode: "equityPercent", quantityValue: 10 } },
    ],
    edges: [
      { id: "edge-mtf-init-log", type: "polyline", sourceNodeId: "mtf-init-root", targetNodeId: "mtf-init-log", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-mtf-ema", type: "polyline", sourceNodeId: "mtf-kline-root", targetNodeId: "mtf-ema", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-mtf-macd", type: "polyline", sourceNodeId: "mtf-ema", targetNodeId: "mtf-macd", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-mtf-macd-cross", type: "polyline", sourceNodeId: "mtf-macd", targetNodeId: "mtf-macd-cross", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-mtf-macd-input", type: "polyline", sourceNodeId: "mtf-macd", targetNodeId: "mtf-macd-cross", properties: buildStrategyVisualDataEdgeProperties("primary") },
      { id: "edge-mtf-buy-order", type: "polyline", sourceNodeId: "mtf-macd-cross", targetNodeId: "mtf-buy-order", properties: buildStrategyVisualControlEdgeProperties("true") },
    ],
  };
}

export function createBracketExitStrategyVisualModel(): StrategyVisualModelDocument {
  return {
    engine: "logic-flow",
    version: 1,
    nodes: [
      { id: "bracket-init-root", type: "circle", x: 180, y: 120, text: "策略启动", properties: { blockKind: "onInit" } },
      { id: "bracket-init-log", type: "rect", x: 500, y: 120, text: "输出日志", properties: { blockKind: "log", message: "Bracket 风控策略已初始化：${ctx.symbol || '00700'} ${ctx.interval || '5m'}" } },
      { id: "bracket-kline-root", type: "circle", x: 180, y: 320, text: "K 线收盘", properties: { blockKind: "onKLineClosed" } },
      { id: "bracket-breakout", type: "diamond", x: 500, y: 320, text: "收盘价 > 520", properties: { blockKind: "ifCloseAbove", threshold: 520 } },
      { id: "bracket-entry", type: "rect", x: 800, y: 260, text: "下单 · 买入开多 · 10%", properties: { blockKind: "placeOrder", orderAction: "entry", orderId: "Long", side: "BUY", orderType: "MARKET", quantityMode: "equityPercent", quantityValue: 10 } },
      { id: "bracket-exit", type: "rect", x: 1100, y: 260, text: "多头止盈止损 1柱 2%", properties: { blockKind: "stopLoss", mode: "bracketExit", direction: "long", timeValue: 1, timeUnit: "bar", percentage: 2, takeProfitPercentage: 4, windowPolicy: "continuous" } },
    ],
    edges: [
      { id: "edge-bracket-init-log", type: "polyline", sourceNodeId: "bracket-init-root", targetNodeId: "bracket-init-log", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-bracket-breakout", type: "polyline", sourceNodeId: "bracket-kline-root", targetNodeId: "bracket-breakout", properties: buildStrategyVisualControlEdgeProperties() },
      { id: "edge-bracket-entry", type: "polyline", sourceNodeId: "bracket-breakout", targetNodeId: "bracket-entry", properties: buildStrategyVisualControlEdgeProperties("true") },
      { id: "edge-bracket-exit", type: "polyline", sourceNodeId: "bracket-entry", targetNodeId: "bracket-exit", properties: buildStrategyVisualControlEdgeProperties() },
    ],
  };
}
