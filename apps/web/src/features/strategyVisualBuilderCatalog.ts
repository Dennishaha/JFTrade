import type { StrategyVisualNodeDocument } from "@jftrade/ui-contracts";

import {
  nextGetTechnicalIndicatorNodeText,
  nextTechnicalIndicatorConditionNodeText,
  nextTechnicalIndicatorNodeText,
  type TechnicalIndicatorConditionMode,
  type TechnicalIndicatorOperator,
  type TechnicalIndicatorPatternType,
  type TechnicalIndicatorType,
} from "./strategyVisualBuilderIndicatorBlock";

export type StrategyBlockKind =
  | "onInit"
  | "onKLineClosed"
  | "codeBlock"
  | "getTechnicalIndicator"
  | "technicalIndicatorCondition"
  | "technicalIndicator"
  | "ifCloseAbove"
  | "ifCloseBelow"
  | "log"
  | "notify"
  | "placeOrder";

export type {
  TechnicalIndicatorConditionMode,
  TechnicalIndicatorOperator,
  TechnicalIndicatorPatternType,
  TechnicalIndicatorType,
} from "./strategyVisualBuilderIndicatorBlock";

export interface StrategyBlockDefinition {
  kind: StrategyBlockKind;
  label: string;
  description: string;
  shape: "circle" | "diamond" | "rect";
  text: string;
  properties: Record<string, unknown>;
  accent: string;
  paletteVisible?: boolean;
}

const STRATEGY_BLOCK_CATALOG: StrategyBlockDefinition[] = [
  {
    kind: "onInit",
    label: "策略启动",
    description: "策略启动时执行一次，适合打印上下文或做预热。",
    shape: "circle",
    text: "策略启动",
    properties: { blockKind: "onInit" },
    accent: "#0f766e",
  },
  {
    kind: "onKLineClosed",
    label: "K 线收盘",
    description: "每次 K 线收盘触发，是当前 QuickJS 策略的核心入口。",
    shape: "circle",
    text: "K 线收盘",
    properties: { blockKind: "onKLineClosed" },
    accent: "#1d4ed8",
  },
  {
    kind: "getTechnicalIndicator",
    label: "指标数据",
    description: "加载一个技术指标结果，供后续判断节点或动作节点复用。",
    shape: "rect",
    text: nextGetTechnicalIndicatorNodeText({
      blockKind: "getTechnicalIndicator",
      indicatorType: "rsi",
      period: 14,
    }),
    properties: {
      blockKind: "getTechnicalIndicator",
      indicatorType: "rsi",
      period: 14,
    },
    accent: "#0f766e",
    paletteVisible: false,
  },
  {
    kind: "technicalIndicatorCondition",
    label: "指标条件判断",
    description: "基于已定义变量里的指标结果做数值或形态判断，并提供 true / false 两个后续分支。",
    shape: "diamond",
    text: nextTechnicalIndicatorConditionNodeText({
      blockKind: "technicalIndicatorCondition",
      indicatorType: "rsi",
      conditionMode: "numeric",
      operator: "<",
      threshold: 30,
    }),
    properties: {
      blockKind: "technicalIndicatorCondition",
      indicatorType: "rsi",
      conditionMode: "numeric",
      operator: "<",
      threshold: 30,
    },
    accent: "#ca8a04",
    paletteVisible: true,
  },
  {
    kind: "technicalIndicator",
    label: "技术指标（兼容）",
    description: "旧版合并式技术指标图块，仅保留给兼容解析和历史模型使用。",
    shape: "rect",
    text: nextTechnicalIndicatorNodeText({
      blockKind: "technicalIndicator",
      indicatorType: "rsi",
      conditionMode: "numeric",
      operator: "<",
      threshold: 30,
      period: 14,
    }),
    properties: {
      blockKind: "technicalIndicator",
      indicatorType: "rsi",
      period: 14,
      conditionMode: "numeric",
      operator: "<",
      threshold: 30,
    },
    accent: "#ca8a04",
    paletteVisible: false,
  },
  {
    kind: "codeBlock",
    label: "代码块",
    description: "承载当前无法映射成标准图块的 QuickJS 代码片段，保留在流程图里继续混编。",
    shape: "rect",
    text: "代码块",
    properties: {
      blockKind: "codeBlock",
      code: "console.log(\"补充自定义逻辑\");",
    },
    accent: "#475569",
  },
  {
    kind: "ifCloseAbove",
    label: "收盘价高于",
    description: "当 ctx.kline.close 高于阈值时执行后续动作。",
    shape: "diamond",
    text: "收盘价 > 阈值",
    properties: {
      blockKind: "ifCloseAbove",
      threshold: 520,
    },
    accent: "#d97706",
  },
  {
    kind: "ifCloseBelow",
    label: "收盘价低于",
    description: "当 ctx.kline.close 低于阈值时执行后续动作。",
    shape: "diamond",
    text: "收盘价 < 阈值",
    properties: {
      blockKind: "ifCloseBelow",
      threshold: 480,
    },
    accent: "#b45309",
  },
  {
    kind: "log",
    label: "输出日志",
    description: "调用 console.log，把上下文或信号写入运行日志。",
    shape: "rect",
    text: "输出日志",
    properties: {
      blockKind: "log",
      message: "观察到新的策略事件",
    },
    accent: "#475569",
  },
  {
    kind: "notify",
    label: "发送通知",
    description: "调用 notify，把策略信号发到控制台通知流。",
    shape: "rect",
    text: "发送通知",
    properties: {
      blockKind: "notify",
      message: "策略条件命中，准备处理后续动作",
    },
    accent: "#be123c",
  },
  {
    kind: "placeOrder",
    label: "下单",
    description: "向券商提交买入或卖出订单，支持固定股数、固定金额、仓位百分比或可用现金百分比四种数量模式。",
    shape: "rect",
    text: "下单",
    properties: {
      blockKind: "placeOrder",
      side: "BUY",
      orderType: "MARKET",
      entryPositionPolicy: "sameDirection",
      quantityMode: "shares",
      quantityValue: 100,
      limitPrice: 0,
      referenceCash: 100000,
    },
    accent: "#0f766e",
  },
];

export function getStrategyBlockCatalog(): StrategyBlockDefinition[] {
  return STRATEGY_BLOCK_CATALOG.map((block) => ({
    ...block,
    properties: { ...block.properties },
  }));
}

export function getStrategyBlockDefinition(
  kind: string | null | undefined,
): StrategyBlockDefinition | null {
  return STRATEGY_BLOCK_CATALOG.find((block) => block.kind === kind) ?? null;
}

export function getStrategyBlockKind(
  node: StrategyVisualNodeDocument | null | undefined,
): StrategyBlockKind | null {
  const value = node?.properties.blockKind;
  return typeof value === "string" ? (value as StrategyBlockKind) : null;
}

export function createStrategyPaletteItems(): Array<{
  type: string;
  text: string;
  label: string;
  icon: string;
  properties: Record<string, unknown>;
}> {
  return STRATEGY_BLOCK_CATALOG
    .filter((block) => block.paletteVisible !== false)
    .map((block) => ({
      type: block.shape,
      text: block.text,
      label: block.label,
      icon: buildPaletteIcon(block.accent, block.label.slice(0, 2)),
      properties: {
        ...block.properties,
      },
    }));
}

function buildPaletteIcon(fill: string, text: string): string {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="72" height="72" viewBox="0 0 72 72"><rect width="72" height="72" rx="18" fill="${fill}"/><text x="36" y="41" text-anchor="middle" font-size="22" font-family="Georgia, serif" fill="white">${escapeXml(text)}</text></svg>`;
  return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
}

function escapeXml(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&apos;");
}