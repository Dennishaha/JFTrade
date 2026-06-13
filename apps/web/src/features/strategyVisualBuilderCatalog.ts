import type { StrategyVisualNodeDocument } from "@/contracts";

import {
  nextGetTechnicalIndicatorNodeText,
  nextTechnicalIndicatorConditionNodeText,
  type TechnicalIndicatorConditionMode,
  type TechnicalIndicatorOperator,
  type TechnicalIndicatorPatternType,
  type TechnicalIndicatorType,
} from "./strategyVisualBuilderIndicatorBlock";

export type StrategyBlockKind =
  | "onInit"
  | "onKLineClosed"
  | "pineSnippet"
  | "getTechnicalIndicator"
  | "technicalIndicatorCondition"
  | "ifCloseAbove"
  | "ifCloseBelow"
  | "log"
  | "notify"
  | "placeOrder"
  | "stopLoss";

export type {
  TechnicalIndicatorConditionMode,
  TechnicalIndicatorOperator,
  TechnicalIndicatorPatternType,
  TechnicalIndicatorType,
} from "./strategyVisualBuilderIndicatorBlock";

export type StopLossDirection = "auto" | "long" | "short";

export type StopLossMode = "stopLoss" | "takeProfit" | "trailingStop";

export type StopLossTimeUnit = "bar" | "minute" | "hour" | "day" | "week" | "month";

export type StopLossWindowPolicy = "continuous" | "session";

export interface StopLossBlockProperties {
  blockKind: "stopLoss";
  mode?: StopLossMode;
  direction?: StopLossDirection;
  timeValue?: number;
  timeUnit?: StopLossTimeUnit;
  percentage?: number;
  windowPolicy?: StopLossWindowPolicy;
}

export const STOP_LOSS_MODE_OPTIONS: Array<{ value: StopLossMode; label: string }> = [
  { value: "stopLoss", label: "止损" },
  { value: "takeProfit", label: "止盈" },
  { value: "trailingStop", label: "追踪止损" },
];

export const STOP_LOSS_DIRECTION_OPTIONS: Array<{ value: StopLossDirection; label: string }> = [
  { value: "auto", label: "自动识别持仓方向" },
  { value: "long", label: "仅多头止损" },
  { value: "short", label: "仅空头止损" },
];

export const STOP_LOSS_TIME_UNIT_OPTIONS: Array<{ value: StopLossTimeUnit; label: string }> = [
  { value: "bar", label: "柱" },
  { value: "minute", label: "分钟" },
  { value: "hour", label: "小时" },
  { value: "day", label: "日" },
  { value: "week", label: "周" },
  { value: "month", label: "月" },
];

export const STOP_LOSS_WINDOW_POLICY_OPTIONS: Array<{ value: StopLossWindowPolicy; label: string }> = [
  { value: "continuous", label: "连续窗口" },
  { value: "session", label: "交易时段感知" },
];

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
    description: "每次 K 线收盘触发，是当前 Pine 策略的核心入口。",
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
    kind: "pineSnippet",
    label: "Pine 片段",
    description: "保留当前不能稳定映射成标准图块的 Pine v6 语句；保存时会原样写回 Pine。",
    shape: "rect",
    text: "Pine 片段",
    properties: {
      blockKind: "pineSnippet",
      code: "log.info(\"保留 Pine 片段\")",
    },
    accent: "#475569",
    paletteVisible: false,
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
    description: "向券商提交买入或卖出订单，支持固定股数、固定金额和账户权益百分比三种 Pine 对齐数量模式。",
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
    },
    accent: "#0f766e",
  },
  {
    kind: "stopLoss",
    label: "止损",
    description: "基于 Go 预计算的风险快照，对当前多空仓位执行止损、止盈或追踪止损平仓。",
    shape: "rect",
    text: "自动止损 1日 2%",
    properties: {
      blockKind: "stopLoss",
      mode: "stopLoss",
      direction: "auto",
      timeValue: 1,
      timeUnit: "day",
      percentage: 2,
      windowPolicy: "continuous",
    },
    accent: "#b91c1c",
  },
];

export function normalizeStopLossMode(value: unknown): StopLossMode {
  return STOP_LOSS_MODE_OPTIONS.some((option) => option.value === value)
    ? (value as StopLossMode)
    : "stopLoss";
}

export function normalizeStopLossDirection(value: unknown): StopLossDirection {
  return value === "long" || value === "short" ? value : "auto";
}

export function normalizeStopLossTimeUnit(value: unknown): StopLossTimeUnit {
  return STOP_LOSS_TIME_UNIT_OPTIONS.some((option) => option.value === value)
    ? (value as StopLossTimeUnit)
    : "day";
}

export function normalizeStopLossWindowPolicy(value: unknown): StopLossWindowPolicy {
  return STOP_LOSS_WINDOW_POLICY_OPTIONS.some((option) => option.value === value)
    ? (value as StopLossWindowPolicy)
    : "continuous";
}

export function normalizeStopLossBlockProperties(
  properties: Record<string, unknown>,
): StopLossBlockProperties {
  return {
    blockKind: "stopLoss",
    mode: normalizeStopLossMode(properties.mode),
    direction: normalizeStopLossDirection(properties.direction),
    timeValue: normalizeStopLossInteger(properties.timeValue, 1),
    timeUnit: normalizeStopLossTimeUnit(properties.timeUnit),
    percentage: normalizeStopLossDecimal(properties.percentage, 2),
    windowPolicy: normalizeStopLossWindowPolicy(properties.windowPolicy),
  };
}

export function stopLossModeLabel(mode: StopLossMode): string {
  switch (mode) {
    case "takeProfit":
      return "止盈";
    case "trailingStop":
      return "追踪止损";
    case "stopLoss":
    default:
      return "止损";
  }
}

export function stopLossDirectionLabel(direction: StopLossDirection): string {
  switch (direction) {
    case "long":
      return "多头";
    case "short":
      return "空头";
    default:
      return "自动";
  }
}

export function stopLossTimeUnitLabel(unit: StopLossTimeUnit): string {
  switch (unit) {
    case "bar":
      return "柱";
    case "minute":
      return "分钟";
    case "hour":
      return "小时";
    case "week":
      return "周";
    case "month":
      return "月";
    case "day":
    default:
      return "日";
  }
}

export function stopLossWindowPolicyLabel(policy: StopLossWindowPolicy): string {
  return policy === "session" ? "交易时段感知" : "连续窗口";
}

export function stopLossRuleLabel(properties: StopLossBlockProperties): string {
  switch (properties.mode ?? "stopLoss") {
    case "takeProfit":
      return `顺向波动 >= ${properties.percentage ?? 2}%`;
    case "trailingStop":
      return `回撤 / 反弹 >= ${properties.percentage ?? 2}%`;
    case "stopLoss":
    default:
      return `反向波动 >= ${properties.percentage ?? 2}%`;
  }
}

export function nextStopLossNodeText(rawProperties: Record<string, unknown>): string {
  const properties = normalizeStopLossBlockProperties(rawProperties);
  return `${stopLossDirectionLabel(properties.direction ?? "auto")}${stopLossModeLabel(properties.mode ?? "stopLoss")} ${properties.timeValue ?? 1}${stopLossTimeUnitLabel(properties.timeUnit ?? "day")} ${properties.percentage ?? 2}%${properties.windowPolicy === "session" ? " 时段感知" : ""}`;
}

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

function normalizeStopLossInteger(value: unknown, fallback: number): number {
  if (typeof value === "number" && Number.isFinite(value)) {
    return Math.max(1, Math.round(value));
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) {
      return Math.max(1, Math.round(parsed));
    }
  }
  return fallback;
}

function normalizeStopLossDecimal(value: unknown, fallback: number): number {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }
  return fallback;
}
