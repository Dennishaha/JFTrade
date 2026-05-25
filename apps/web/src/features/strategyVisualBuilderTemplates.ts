import type { StrategyVisualModelDocument } from "@jftrade/ui-contracts";

import {
  createATRVolatilityStrategyVisualModel,
  createBollingerReversionStrategyVisualModel,
  createBreakoutStrategyVisualModel,
  createCCIReversionStrategyVisualModel,
  createDefaultStrategyVisualModel,
  createDoubleMovingAverageStrategyVisualModel,
  createKDJReversionStrategyVisualModel,
  createMACDMomentumStrategyVisualModel,
  createMeanReversionStrategyVisualModel,
  createRSIReversionStrategyVisualModel,
  createWilliamsRReversionStrategyVisualModel,
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
      defaultDescription: "双均线交叉示例，使用 5/20 周期均线，金叉买入、死叉卖出，含持仓与重复买入检查。",
      defaultSymbol: "00700",
      defaultInterval: "5m",
      visualModel: createDoubleMovingAverageStrategyVisualModel(),
      syncVisualToCode: true,
      buildScript: (context) => buildDoubleMovingAverageScript(context),
    },
    {
      id: "rsi-reversion",
      label: "RSI 反转交易",
      description: "RSI 超买超卖模板，超卖买入、超买卖出，含持仓与重复买入检查。",
      mode: "visual",
      defaultId: "js-rsi-reversion",
      defaultName: "RSI 反转交易",
      defaultVersion: "0.1.0",
      defaultDescription: "通过 RSI 14 在超卖区间买入、超买区间卖出，自动管理持仓。",
      defaultSymbol: "00700",
      defaultInterval: "1m",
      visualModel: createRSIReversionStrategyVisualModel(),
      syncVisualToCode: true,
      buildScript: (context) =>
        buildStrategyScriptFromVisualModel(createRSIReversionStrategyVisualModel(), context),
    },
    {
      id: "macd-momentum",
      label: "MACD 动能交易",
      description: "MACD 动能模板，多头买入、空头卖出，含持仓与重复买入检查。",
      mode: "visual",
      defaultId: "js-macd-momentum",
      defaultName: "MACD 动能交易",
      defaultVersion: "0.1.0",
      defaultDescription: "通过 MACD 12/26/9 金叉买入、死叉卖出，自动管理持仓。",
      defaultSymbol: "00700",
      defaultInterval: "1m",
      visualModel: createMACDMomentumStrategyVisualModel(),
      syncVisualToCode: true,
      buildScript: (context) =>
        buildStrategyScriptFromVisualModel(createMACDMomentumStrategyVisualModel(), context),
    },
    {
      id: "kdj-reversion",
      label: "KDJ 交叉交易",
      description: "KDJ 金叉/死叉模板，适合短周期动量反转。",
      mode: "visual",
      defaultId: "js-kdj-reversion",
      defaultName: "KDJ 交叉交易",
      defaultVersion: "0.1.0",
      defaultDescription: "通过 KDJ 9/3/3 金叉买入、死叉卖出，自动管理持仓。",
      defaultSymbol: "00700",
      defaultInterval: "1m",
      visualModel: createKDJReversionStrategyVisualModel(),
      syncVisualToCode: true,
      buildScript: (context) =>
        buildStrategyScriptFromVisualModel(createKDJReversionStrategyVisualModel(), context),
    },
    {
      id: "bollinger-reversion",
      label: "布林带回归交易",
      description: "布林带模板，下轨买入、上轨卖出，含持仓与重复买入检查。",
      mode: "visual",
      defaultId: "js-bollinger-reversion",
      defaultName: "布林带回归交易",
      defaultVersion: "0.1.0",
      defaultDescription: "通过布林带 20x2 在下轨买入、上轨卖出，自动管理持仓。",
      defaultSymbol: "00700",
      defaultInterval: "1m",
      visualModel: createBollingerReversionStrategyVisualModel(),
      syncVisualToCode: true,
      buildScript: (context) =>
        buildStrategyScriptFromVisualModel(createBollingerReversionStrategyVisualModel(), context),
    },
    {
      id: "atr-volatility",
      label: "ATR 波动率过滤",
      description: "ATR 高低阈值模板，适合作为波动率开关或风险过滤。",
      mode: "visual",
      defaultId: "js-atr-volatility",
      defaultName: "ATR 波动率过滤",
      defaultVersion: "0.1.0",
      defaultDescription: "通过 ATR 14 判断波动率状态，波动升高买入、回落卖出。",
      defaultSymbol: "00700",
      defaultInterval: "5m",
      visualModel: createATRVolatilityStrategyVisualModel(),
      syncVisualToCode: true,
      buildScript: (context) =>
        buildStrategyScriptFromVisualModel(createATRVolatilityStrategyVisualModel(), context),
    },
    {
      id: "cci-reversion",
      label: "CCI 反转交易",
      description: "CCI 超买超卖模板，适合顺势回撤与区间反转。",
      mode: "visual",
      defaultId: "js-cci-reversion",
      defaultName: "CCI 反转交易",
      defaultVersion: "0.1.0",
      defaultDescription: "通过 CCI 20 在低于 -100 买入、高于 100 卖出。",
      defaultSymbol: "00700",
      defaultInterval: "5m",
      visualModel: createCCIReversionStrategyVisualModel(),
      syncVisualToCode: true,
      buildScript: (context) =>
        buildStrategyScriptFromVisualModel(createCCIReversionStrategyVisualModel(), context),
    },
    {
      id: "williamsr-reversion",
      label: "Williams %R 反转交易",
      description: "Williams %R 超买超卖模板，适合高频回归场景。",
      mode: "visual",
      defaultId: "js-williamsr-reversion",
      defaultName: "Williams %R 反转交易",
      defaultVersion: "0.1.0",
      defaultDescription: "通过 Williams %R 14 在超卖区买入、超买区卖出。",
      defaultSymbol: "00700",
      defaultInterval: "5m",
      visualModel: createWilliamsRReversionStrategyVisualModel(),
      syncVisualToCode: true,
      buildScript: (context) =>
        buildStrategyScriptFromVisualModel(createWilliamsRReversionStrategyVisualModel(), context),
    },
    {
      id: "breakout-alert",
      label: "突破交易",
      description: "突破模板，收盘价上穿阈值后买入，含重复买入检查。",
      mode: "visual",
      defaultId: "js-breakout-alert",
      defaultName: "突破交易",
      defaultVersion: "0.1.0",
      defaultDescription: "收盘价突破阈值后买入，含持仓检查防止重复下单。",
      defaultSymbol: "00700",
      defaultInterval: "1m",
      visualModel: createBreakoutStrategyVisualModel(),
      syncVisualToCode: true,
      buildScript: (context) =>
        buildStrategyScriptFromVisualModel(createBreakoutStrategyVisualModel(), context),
    },
    {
      id: "mean-reversion-alert",
      label: "均值回归交易",
      description: "均值回归模板，收盘价跌破阈值后低吸买入，含重复买入检查。",
      mode: "visual",
      defaultId: "js-mean-reversion-alert",
      defaultName: "均值回归交易",
      defaultVersion: "0.1.0",
      defaultDescription: "收盘价跌破阈值时低吸买入，含持仓检查防止重复下单。",
      defaultSymbol: "00700",
      defaultInterval: "1m",
      visualModel: createMeanReversionStrategyVisualModel(),
      syncVisualToCode: true,
      buildScript: (context) =>
        buildStrategyScriptFromVisualModel(createMeanReversionStrategyVisualModel(), context),
    },
  ];
}