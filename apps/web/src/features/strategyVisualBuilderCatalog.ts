import type { StrategyVisualNodeDocument } from "@jftrade/ui-contracts";

export type StrategyBlockKind =
  | "onInit"
  | "onKLineClosed"
  | "codeBlock"
  | "movingAverageFast"
  | "movingAverageSlow"
  | "ifGoldenCross"
  | "ifDeathCross"
  | "rsi"
  | "ifRsiAbove"
  | "ifRsiBelow"
  | "macd"
  | "ifMacdBullish"
  | "ifMacdBearish"
  | "bollinger"
  | "ifCloseAboveUpperBand"
  | "ifCloseBelowLowerBand"
  | "ifCloseAbove"
  | "ifCloseBelow"
  | "log"
  | "notify";

export interface StrategyBlockDefinition {
  kind: StrategyBlockKind;
  label: string;
  description: string;
  shape: "circle" | "diamond" | "rect";
  text: string;
  properties: Record<string, unknown>;
  accent: string;
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
    kind: "movingAverageFast",
    label: "快均线",
    description: "计算快线均值，默认是 5 根 K 线。",
    shape: "rect",
    text: "快均线 5",
    properties: {
      blockKind: "movingAverageFast",
      windowSize: 5,
    },
    accent: "#0891b2",
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
    kind: "movingAverageSlow",
    label: "慢均线",
    description: "计算慢线均值，并同步更新上一根均线状态。",
    shape: "rect",
    text: "慢均线 20",
    properties: {
      blockKind: "movingAverageSlow",
      windowSize: 20,
    },
    accent: "#7c3aed",
  },
  {
    kind: "ifGoldenCross",
    label: "金叉",
    description: "快线从下向上穿过慢线时执行后续动作。",
    shape: "diamond",
    text: "金叉",
    properties: {
      blockKind: "ifGoldenCross",
    },
    accent: "#ca8a04",
  },
  {
    kind: "ifDeathCross",
    label: "死叉",
    description: "快线从上向下跌破慢线时执行后续动作。",
    shape: "diamond",
    text: "死叉",
    properties: {
      blockKind: "ifDeathCross",
    },
    accent: "#dc2626",
  },
  {
    kind: "rsi",
    label: "RSI",
    description: "计算相对强弱指数，适合和超买超卖条件块组合。",
    shape: "rect",
    text: "RSI 14",
    properties: {
      blockKind: "rsi",
      period: 14,
    },
    accent: "#0f766e",
  },
  {
    kind: "ifRsiAbove",
    label: "RSI 高于",
    description: "当 RSI 高于阈值时执行后续动作，常用于超买告警。",
    shape: "diamond",
    text: "RSI > 70",
    properties: {
      blockKind: "ifRsiAbove",
      threshold: 70,
    },
    accent: "#ea580c",
  },
  {
    kind: "ifRsiBelow",
    label: "RSI 低于",
    description: "当 RSI 低于阈值时执行后续动作，常用于超卖反转观察。",
    shape: "diamond",
    text: "RSI < 30",
    properties: {
      blockKind: "ifRsiBelow",
      threshold: 30,
    },
    accent: "#2563eb",
  },
  {
    kind: "macd",
    label: "MACD",
    description: "计算 MACD diff、signal 和 histogram，适合做趋势动能观察。",
    shape: "rect",
    text: "MACD 12/26/9",
    properties: {
      blockKind: "macd",
      fastPeriod: 12,
      slowPeriod: 26,
      signalPeriod: 9,
    },
    accent: "#9333ea",
  },
  {
    kind: "ifMacdBullish",
    label: "MACD 多头",
    description: "当 MACD diff 高于 signal 时执行后续动作。",
    shape: "diamond",
    text: "MACD 多头",
    properties: {
      blockKind: "ifMacdBullish",
    },
    accent: "#16a34a",
  },
  {
    kind: "ifMacdBearish",
    label: "MACD 空头",
    description: "当 MACD diff 低于 signal 时执行后续动作。",
    shape: "diamond",
    text: "MACD 空头",
    properties: {
      blockKind: "ifMacdBearish",
    },
    accent: "#dc2626",
  },
  {
    kind: "bollinger",
    label: "布林带",
    description: "计算中轨、上轨、下轨，适合做通道突破或回归观察。",
    shape: "rect",
    text: "布林带 20x2",
    properties: {
      blockKind: "bollinger",
      period: 20,
      multiplier: 2,
    },
    accent: "#0d9488",
  },
  {
    kind: "ifCloseAboveUpperBand",
    label: "收盘价高于上轨",
    description: "当收盘价突破布林带上轨时执行后续动作。",
    shape: "diamond",
    text: "收盘价 > 上轨",
    properties: {
      blockKind: "ifCloseAboveUpperBand",
    },
    accent: "#f97316",
  },
  {
    kind: "ifCloseBelowLowerBand",
    label: "收盘价低于下轨",
    description: "当收盘价跌破布林带下轨时执行后续动作。",
    shape: "diamond",
    text: "收盘价 < 下轨",
    properties: {
      blockKind: "ifCloseBelowLowerBand",
    },
    accent: "#2563eb",
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
  return STRATEGY_BLOCK_CATALOG.map((block) => ({
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