export type TechnicalIndicatorType =
  | "movingAverage"
  | "rsi"
  | "macd"
  | "kdj"
  | "atr"
  | "cci"
  | "williamsR"
  | "bollinger";

export type TechnicalIndicatorConditionMode = "none" | "numeric" | "pattern";

export type TechnicalIndicatorOperator = ">" | "<";

export type TechnicalIndicatorPatternType =
  | "goldenCross"
  | "deathCross"
  | "topDivergence"
  | "bottomDivergence"
  | "closeAboveUpperBand"
  | "closeBelowLowerBand";

export interface TechnicalIndicatorBlockProperties {
  blockKind: "technicalIndicator";
  indicatorType: TechnicalIndicatorType;
  conditionMode: TechnicalIndicatorConditionMode;
  operator?: TechnicalIndicatorOperator;
  threshold?: number;
  patternType?: TechnicalIndicatorPatternType;
  lookback?: number;
  period?: number;
  windowSize?: number;
  fastPeriod?: number;
  slowPeriod?: number;
  signalPeriod?: number;
  m1?: number;
  m2?: number;
  multiplier?: number;
}

export interface TechnicalIndicatorOption {
  value: TechnicalIndicatorType;
  label: string;
}

export interface TechnicalIndicatorPatternOption {
  value: TechnicalIndicatorPatternType;
  label: string;
}

export const TECHNICAL_INDICATOR_OPTIONS: TechnicalIndicatorOption[] = [
  { value: "movingAverage", label: "双均线" },
  { value: "rsi", label: "RSI" },
  { value: "macd", label: "MACD" },
  { value: "kdj", label: "KDJ" },
  { value: "atr", label: "ATR" },
  { value: "cci", label: "CCI" },
  { value: "williamsR", label: "Williams %R" },
  { value: "bollinger", label: "布林带" },
];

const PATTERN_OPTION_MAP: Record<TechnicalIndicatorType, TechnicalIndicatorPatternOption[]> = {
  movingAverage: [
    { value: "goldenCross", label: "金叉" },
    { value: "deathCross", label: "死叉" },
  ],
  rsi: [
    { value: "topDivergence", label: "顶背离" },
    { value: "bottomDivergence", label: "底背离" },
  ],
  macd: [
    { value: "goldenCross", label: "金叉" },
    { value: "deathCross", label: "死叉" },
    { value: "topDivergence", label: "顶背离" },
    { value: "bottomDivergence", label: "底背离" },
  ],
  kdj: [
    { value: "goldenCross", label: "金叉" },
    { value: "deathCross", label: "死叉" },
    { value: "topDivergence", label: "顶背离" },
    { value: "bottomDivergence", label: "底背离" },
  ],
  atr: [],
  cci: [],
  williamsR: [],
  bollinger: [
    { value: "closeAboveUpperBand", label: "收盘价突破上轨" },
    { value: "closeBelowLowerBand", label: "收盘价跌破下轨" },
  ],
};

export function getPatternOptions(
  indicatorType: TechnicalIndicatorType,
): TechnicalIndicatorPatternOption[] {
  return PATTERN_OPTION_MAP[indicatorType];
}

export function supportsNumericCondition(indicatorType: TechnicalIndicatorType): boolean {
  return indicatorType !== "movingAverage" && indicatorType !== "bollinger";
}

export function supportsPatternCondition(indicatorType: TechnicalIndicatorType): boolean {
  return getPatternOptions(indicatorType).length > 0;
}

export function isDivergencePattern(
  patternType: string | null | undefined,
): patternType is "topDivergence" | "bottomDivergence" {
  return patternType === "topDivergence" || patternType === "bottomDivergence";
}

export function normalizeTechnicalIndicatorType(value: unknown): TechnicalIndicatorType {
  return TECHNICAL_INDICATOR_OPTIONS.some((option) => option.value === value)
    ? (value as TechnicalIndicatorType)
    : "rsi";
}

export function normalizeTechnicalIndicatorConditionMode(
  value: unknown,
  indicatorType: TechnicalIndicatorType,
): TechnicalIndicatorConditionMode {
  if (value === "none") {
    return "none";
  }
  if (value === "pattern" && supportsPatternCondition(indicatorType)) {
    return "pattern";
  }
  if (value === "numeric" && supportsNumericCondition(indicatorType)) {
    return "numeric";
  }
  if (supportsNumericCondition(indicatorType)) {
    return "numeric";
  }
  if (supportsPatternCondition(indicatorType)) {
    return "pattern";
  }
  return "none";
}

export function normalizeTechnicalIndicatorOperator(value: unknown): TechnicalIndicatorOperator {
  return value === ">" ? ">" : "<";
}

export function normalizeTechnicalIndicatorPatternType(
  indicatorType: TechnicalIndicatorType,
  value: unknown,
): TechnicalIndicatorPatternType {
  const options = getPatternOptions(indicatorType);
  if (options.some((option) => option.value === value)) {
    return value as TechnicalIndicatorPatternType;
  }

  switch (indicatorType) {
    case "movingAverage":
    case "macd":
    case "kdj":
      return "goldenCross";
    case "rsi":
      return "bottomDivergence";
    case "bollinger":
      return "closeBelowLowerBand";
    default:
      return "goldenCross";
  }
}

export function normalizeTechnicalIndicatorProperties(
  properties: Record<string, unknown>,
): TechnicalIndicatorBlockProperties {
  const indicatorType = normalizeTechnicalIndicatorType(properties.indicatorType);
  const conditionMode = normalizeTechnicalIndicatorConditionMode(
    properties.conditionMode,
    indicatorType,
  );
  const normalized: TechnicalIndicatorBlockProperties = {
    blockKind: "technicalIndicator",
    indicatorType,
    conditionMode,
  };

  switch (indicatorType) {
    case "movingAverage":
      normalized.fastPeriod = normalizeInteger(properties.fastPeriod, 5);
      normalized.slowPeriod = normalizeInteger(properties.slowPeriod, 20);
      break;
    case "macd":
      normalized.fastPeriod = normalizeInteger(properties.fastPeriod, 12);
      normalized.slowPeriod = normalizeInteger(properties.slowPeriod, 26);
      normalized.signalPeriod = normalizeInteger(properties.signalPeriod, 9);
      break;
    case "kdj":
      normalized.period = normalizeInteger(properties.period, 9);
      normalized.m1 = normalizeInteger(properties.m1, 3);
      normalized.m2 = normalizeInteger(properties.m2, 3);
      break;
    case "bollinger":
      normalized.period = normalizeInteger(properties.period, 20);
      normalized.multiplier = normalizeDecimal(properties.multiplier, 2);
      break;
    default:
      normalized.period = normalizeInteger(properties.period ?? properties.windowSize, defaultPeriodForIndicator(indicatorType));
      break;
  }

  if (conditionMode === "numeric") {
    normalized.operator = normalizeTechnicalIndicatorOperator(properties.operator);
    normalized.threshold = normalizeDecimal(
      properties.threshold,
      defaultThresholdForIndicator(indicatorType, normalized.operator),
    );
  }

  if (conditionMode === "pattern") {
    normalized.patternType = normalizeTechnicalIndicatorPatternType(
      indicatorType,
      properties.patternType,
    );
    if (isDivergencePattern(normalized.patternType)) {
      normalized.lookback = normalizeInteger(properties.lookback, 5);
    }
  }

  return normalized;
}

export function nextTechnicalIndicatorNodeText(
  rawProperties: Record<string, unknown>,
): string {
  const properties = normalizeTechnicalIndicatorProperties(rawProperties);
  const indicatorLabel = indicatorTypeLabel(properties.indicatorType);

  if (properties.conditionMode === "none") {
    return `${indicatorLabel} ${indicatorParameterText(properties)}`.trim();
  }

  if (properties.conditionMode === "numeric") {
    return `${indicatorLabel} ${indicatorParameterText(properties)} ${properties.operator} ${formatThreshold(properties.threshold ?? 0)}`.trim();
  }

  const patternLabel = patternTypeLabel(properties.patternType ?? "goldenCross");
  if (isDivergencePattern(properties.patternType)) {
    return `${indicatorLabel} ${indicatorParameterText(properties)} ${patternLabel} (${properties.lookback ?? 5})`.trim();
  }
  return `${indicatorLabel} ${indicatorParameterText(properties)} ${patternLabel}`.trim();
}

export function indicatorTypeLabel(indicatorType: TechnicalIndicatorType): string {
  return TECHNICAL_INDICATOR_OPTIONS.find((option) => option.value === indicatorType)?.label ?? "技术指标";
}

export function patternTypeLabel(patternType: TechnicalIndicatorPatternType): string {
  switch (patternType) {
    case "goldenCross":
      return "金叉";
    case "deathCross":
      return "死叉";
    case "topDivergence":
      return "顶背离";
    case "bottomDivergence":
      return "底背离";
    case "closeAboveUpperBand":
      return "突破上轨";
    case "closeBelowLowerBand":
      return "跌破下轨";
    default:
      return "形态";
  }
}

function indicatorParameterText(properties: TechnicalIndicatorBlockProperties): string {
  switch (properties.indicatorType) {
    case "movingAverage":
      return `${properties.fastPeriod ?? 5}/${properties.slowPeriod ?? 20}`;
    case "macd":
      return `${properties.fastPeriod ?? 12}/${properties.slowPeriod ?? 26}/${properties.signalPeriod ?? 9}`;
    case "kdj":
      return `${properties.period ?? 9}/${properties.m1 ?? 3}/${properties.m2 ?? 3}`;
    case "bollinger":
      return `${properties.period ?? 20}x${formatThreshold(properties.multiplier ?? 2)}`;
    default:
      return String(properties.period ?? defaultPeriodForIndicator(properties.indicatorType));
  }
}

function defaultPeriodForIndicator(indicatorType: TechnicalIndicatorType): number {
  switch (indicatorType) {
    case "rsi":
    case "atr":
    case "williamsR":
      return 14;
    case "cci":
      return 20;
    default:
      return 14;
  }
}

function defaultThresholdForIndicator(
  indicatorType: TechnicalIndicatorType,
  operator: TechnicalIndicatorOperator,
): number {
  switch (indicatorType) {
    case "rsi":
      return operator === ">" ? 70 : 30;
    case "atr":
      return operator === ">" ? 2 : 1;
    case "cci":
      return operator === ">" ? 100 : -100;
    case "williamsR":
      return operator === ">" ? -20 : -80;
    case "kdj":
      return operator === ">" ? 80 : 20;
    case "macd":
      return 0;
    default:
      return 0;
  }
}

function normalizeInteger(value: unknown, fallback: number): number {
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

function normalizeDecimal(value: unknown, fallback: number): number {
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

function formatThreshold(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(2).replace(/\.00$/, "");
}
