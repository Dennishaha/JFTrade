import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

import {
  createBollingerReversionStrategyVisualModel,
  createBreakoutStrategyVisualModel,
  createDefaultStrategyVisualModel,
  createDoubleMovingAverageStrategyVisualModel,
  createMACDMomentumStrategyVisualModel,
  createMeanReversionStrategyVisualModel,
  createRSIReversionStrategyVisualModel,
} from "./strategyVisualBuilderModels";
import {
  buildDoubleMovingAverageScript,
  buildStrategyScriptFromVisualModel,
  type StrategyScriptContext,
} from "./strategyVisualBuilderScript";

export interface StrategyAuthoringTemplate {
  id: string;
  label: string;
  description: string;
  mode: "visual" | "code";
  defaultId: string;
  defaultName: string;
  defaultVersion: string;
  defaultDescription: string;
  defaultSymbol: string;
  defaultInterval: string;
  visualModel: StrategyVisualModelDocument;
  syncVisualToCode: boolean;
  buildScript: (context: StrategyScriptContext) => string;
}

export function getStrategyAuthoringTemplates(): StrategyAuthoringTemplate[] {
  return [
    {
      id: "logic-flow-starter",
      label: "逻辑流起步骨架",
      description: "给一份可视化起步图，适合从拖拽块开始搭策略。",
      mode: "visual",
      defaultId: "js-logic-flow-starter",
      defaultName: "逻辑流起步骨架",
      defaultVersion: "0.1.0",
      defaultDescription: "用流程图拖拽块，快速搭一个可保存的 QuickJS 策略骨架。",
      defaultSymbol: "00700",
      defaultInterval: "1m",
      visualModel: createDefaultStrategyVisualModel(),
      syncVisualToCode: true,
      buildScript: (context) =>
        buildStrategyScriptFromVisualModel(createDefaultStrategyVisualModel(), context),
    },
    {
      id: "double-moving-average",
      label: "双均线系统",
      description: "经典快慢均线交叉模板，图和代码都使用同一套金叉/死叉流程。",
      mode: "visual",
      defaultId: "js-double-moving-average",
      defaultName: "双均线系统",
      defaultVersion: "0.1.0",
      defaultDescription: "双均线交叉示例，使用 5/20 周期均线产生金叉和死叉通知。",
      defaultSymbol: "00700",
      defaultInterval: "5m",
      visualModel: createDoubleMovingAverageStrategyVisualModel(),
      syncVisualToCode: true,
      buildScript: (context) => buildDoubleMovingAverageScript(context),
    },
    {
      id: "rsi-reversion",
      label: "RSI 反转观察",
      description: "经典 RSI 超买超卖模板，适合先做观察与通知，不直接耦合下单。",
      mode: "visual",
      defaultId: "js-rsi-reversion",
      defaultName: "RSI 反转观察",
      defaultVersion: "0.1.0",
      defaultDescription: "通过 RSI 14 观测超卖与超买区间，并发送视觉化告警。",
      defaultSymbol: "00700",
      defaultInterval: "1m",
      visualModel: createRSIReversionStrategyVisualModel(),
      syncVisualToCode: true,
      buildScript: (context) =>
        buildStrategyScriptFromVisualModel(createRSIReversionStrategyVisualModel(), context),
    },
    {
      id: "macd-momentum",
      label: "MACD 动能观察",
      description: "经典 MACD 动能模板，关注 diff 与 signal 的多空关系。",
      mode: "visual",
      defaultId: "js-macd-momentum",
      defaultName: "MACD 动能观察",
      defaultVersion: "0.1.0",
      defaultDescription: "通过 MACD 12/26/9 观察多空动能，并输出视觉化告警。",
      defaultSymbol: "00700",
      defaultInterval: "1m",
      visualModel: createMACDMomentumStrategyVisualModel(),
      syncVisualToCode: true,
      buildScript: (context) =>
        buildStrategyScriptFromVisualModel(createMACDMomentumStrategyVisualModel(), context),
    },
    {
      id: "bollinger-reversion",
      label: "布林带回归观察",
      description: "经典布林带模板，观察价格脱离通道上轨或下轨后的信号。",
      mode: "visual",
      defaultId: "js-bollinger-reversion",
      defaultName: "布林带回归观察",
      defaultVersion: "0.1.0",
      defaultDescription: "通过布林带 20x2 观察价格突破通道后的回归或延续信号。",
      defaultSymbol: "00700",
      defaultInterval: "1m",
      visualModel: createBollingerReversionStrategyVisualModel(),
      syncVisualToCode: true,
      buildScript: (context) =>
        buildStrategyScriptFromVisualModel(createBollingerReversionStrategyVisualModel(), context),
    },
    {
      id: "breakout-alert",
      label: "突破告警",
      description: "可视化突破模板，收盘价上穿阈值后直接触发通知。",
      mode: "visual",
      defaultId: "js-breakout-alert",
      defaultName: "突破告警",
      defaultVersion: "0.1.0",
      defaultDescription: "突破阈值后的最小告警模板，适合继续接入更复杂的趋势策略。",
      defaultSymbol: "00700",
      defaultInterval: "1m",
      visualModel: createBreakoutStrategyVisualModel(),
      syncVisualToCode: true,
      buildScript: (context) =>
        buildStrategyScriptFromVisualModel(createBreakoutStrategyVisualModel(), context),
    },
    {
      id: "mean-reversion-alert",
      label: "均值回归告警",
      description: "可视化回归模板，适合先做低吸观察与通知流。",
      mode: "visual",
      defaultId: "js-mean-reversion-alert",
      defaultName: "均值回归告警",
      defaultVersion: "0.1.0",
      defaultDescription: "收盘价跌破观察阈值时记录日志并发送通知。",
      defaultSymbol: "00700",
      defaultInterval: "1m",
      visualModel: createMeanReversionStrategyVisualModel(),
      syncVisualToCode: true,
      buildScript: (context) =>
        buildStrategyScriptFromVisualModel(createMeanReversionStrategyVisualModel(), context),
    },
  ];
}