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

export type TechnicalIndicatorComparisonMode = Exclude<
  TechnicalIndicatorConditionMode,
  "none"
>;

export type TechnicalIndicatorOperator = ">" | "<";

export type MovingAverageIndicatorType =
  | "MA"
  | "EMA"
  | "SMA"
  | "SMMA"
  | "LWMA"
  | "TMA"
  | "EXPMA"
  | "HMA"
  | "VWMA"
  | "BOLL";

export type IndicatorPeriodUnit = "bar" | "minute" | "hour" | "day" | "week" | "month";

export type TechnicalIndicatorInputSlot = "primary" | "fast" | "slow";

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
  movingAverageType?: MovingAverageIndicatorType;
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

export interface GetTechnicalIndicatorBlockProperties {
  blockKind: "getTechnicalIndicator";
  indicatorType: TechnicalIndicatorType;
  variableName?: string;
  movingAverageType?: MovingAverageIndicatorType;
  periodUnit?: IndicatorPeriodUnit;
  period?: number;
  windowSize?: number;
  fastPeriod?: number;
  slowPeriod?: number;
  signalPeriod?: number;
  m1?: number;
  m2?: number;
  multiplier?: number;
}

export interface TechnicalIndicatorConditionBlockProperties {
  blockKind: "technicalIndicatorCondition";
  indicatorType: TechnicalIndicatorType;
  conditionMode: TechnicalIndicatorComparisonMode;
  inputPrimaryNodeId?: string;
  inputFastNodeId?: string;
  inputSlowNodeId?: string;
  operator?: TechnicalIndicatorOperator;
  threshold?: number;
  patternType?: TechnicalIndicatorPatternType;
  lookback?: number;
}

export interface TechnicalIndicatorOption {
  value: TechnicalIndicatorType;
  label: string;
}

export interface TechnicalIndicatorConditionModeOption {
  value: TechnicalIndicatorConditionMode;
  label: string;
}

export interface MovingAverageIndicatorOption {
  value: MovingAverageIndicatorType;
  label: string;
}

export interface IndicatorPeriodUnitOption {
  value: IndicatorPeriodUnit;
  label: string;
}

export interface TechnicalIndicatorPatternOption {
  value: TechnicalIndicatorPatternType;
  label: string;
}

interface TechnicalIndicatorDefinition {
  label: string;
  getterLabel?: string;
  conditionModes: TechnicalIndicatorComparisonMode[];
  defaultConditionMode: TechnicalIndicatorComparisonMode;
  parameterShape: "windowSize" | "period" | "macd" | "kdj" | "bollinger";
  defaultPeriod?: number;
  defaultWindowSize?: number;
  defaultFastPeriod?: number;
  defaultSlowPeriod?: number;
  defaultSignalPeriod?: number;
  defaultM1?: number;
  defaultM2?: number;
  defaultMultiplier?: number;
  numericTargetLabel?: string;
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

export const MOVING_AVERAGE_INDICATOR_OPTIONS: MovingAverageIndicatorOption[] = [
  { value: "MA", label: "MA" },
  { value: "EMA", label: "EMA" },
  { value: "SMA", label: "SMA" },
  { value: "SMMA", label: "SMMA" },
  { value: "LWMA", label: "LWMA" },
  { value: "TMA", label: "TMA" },
  { value: "EXPMA", label: "EXPMA" },
  { value: "HMA", label: "HMA" },
  { value: "VWMA", label: "VWMA" },
  { value: "BOLL", label: "BOLL" },
];

export const INDICATOR_PERIOD_UNIT_OPTIONS: IndicatorPeriodUnitOption[] = [
  { value: "bar", label: "柱" },
  { value: "minute", label: "分钟" },
  { value: "hour", label: "小时" },
  { value: "day", label: "日" },
  { value: "week", label: "周" },
  { value: "month", label: "月" },
];

const TECHNICAL_INDICATOR_DEFINITION_MAP: Record<
  TechnicalIndicatorType,
  TechnicalIndicatorDefinition
> = {
  movingAverage: {
    label: "双均线",
    getterLabel: "均线",
    conditionModes: ["pattern"],
    defaultConditionMode: "pattern",
    parameterShape: "windowSize",
    defaultWindowSize: 20,
  },
  rsi: {
    label: "RSI",
    conditionModes: ["numeric", "pattern"],
    defaultConditionMode: "numeric",
    parameterShape: "period",
    defaultPeriod: 14,
    numericTargetLabel: "RSI 值",
  },
  macd: {
    label: "MACD",
    conditionModes: ["numeric", "pattern"],
    defaultConditionMode: "pattern",
    parameterShape: "macd",
    defaultFastPeriod: 12,
    defaultSlowPeriod: 26,
    defaultSignalPeriod: 9,
    numericTargetLabel: "柱状图",
  },
  kdj: {
    label: "KDJ",
    conditionModes: ["numeric", "pattern"],
    defaultConditionMode: "pattern",
    parameterShape: "kdj",
    defaultPeriod: 9,
    defaultM1: 3,
    defaultM2: 3,
    numericTargetLabel: "J 值",
  },
  atr: {
    label: "ATR",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    parameterShape: "period",
    defaultPeriod: 14,
    numericTargetLabel: "ATR 值",
  },
  cci: {
    label: "CCI",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    parameterShape: "period",
    defaultPeriod: 20,
    numericTargetLabel: "CCI 值",
  },
  williamsR: {
    label: "Williams %R",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    parameterShape: "period",
    defaultPeriod: 14,
    numericTargetLabel: "Williams %R 值",
  },
  bollinger: {
    label: "布林带",
    conditionModes: ["pattern"],
    defaultConditionMode: "pattern",
    parameterShape: "bollinger",
    defaultPeriod: 20,
    defaultMultiplier: 2,
  },
};

const CONDITION_MODE_LABEL_MAP: Record<
  TechnicalIndicatorConditionMode,
  string
> = {
  none: "仅加载指标",
  numeric: "数值型",
  pattern: "形态型",
};

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

const TECHNICAL_INDICATOR_INPUT_SLOTS: Record<
  TechnicalIndicatorType,
  TechnicalIndicatorInputSlot[]
> = {
  movingAverage: ["fast", "slow"],
  rsi: ["primary"],
  macd: ["primary"],
  kdj: ["primary"],
  atr: ["primary"],
  cci: ["primary"],
  williamsR: ["primary"],
  bollinger: ["primary"],
};

export function getTechnicalIndicatorDefinition(
  indicatorType: TechnicalIndicatorType,
): TechnicalIndicatorDefinition {
  return TECHNICAL_INDICATOR_DEFINITION_MAP[indicatorType];
}

export function getTechnicalIndicatorGetterLabel(
  indicatorType: TechnicalIndicatorType,
): string {
  const definition = getTechnicalIndicatorDefinition(indicatorType);
  return definition.getterLabel ?? definition.label;
}

export const GET_TECHNICAL_INDICATOR_OPTIONS: TechnicalIndicatorOption[] =
  TECHNICAL_INDICATOR_OPTIONS.map((option) => ({
    ...option,
    label: getTechnicalIndicatorGetterLabel(option.value),
  }));

export function getTechnicalIndicatorInputSlots(
  indicatorType: TechnicalIndicatorType,
): TechnicalIndicatorInputSlot[] {
  return [...TECHNICAL_INDICATOR_INPUT_SLOTS[indicatorType]];
}

export function getTechnicalIndicatorConditionModeOptions(
  indicatorType: TechnicalIndicatorType,
  includeNone = false,
): TechnicalIndicatorConditionModeOption[] {
  const definition = getTechnicalIndicatorDefinition(indicatorType);
  const values: TechnicalIndicatorConditionMode[] = includeNone
    ? ["none", ...definition.conditionModes]
    : [...definition.conditionModes];
  return values.map((value) => ({
    value,
    label: CONDITION_MODE_LABEL_MAP[value],
  }));
}

export function getPatternOptions(
  indicatorType: TechnicalIndicatorType,
): TechnicalIndicatorPatternOption[] {
  return PATTERN_OPTION_MAP[indicatorType];
}

export function supportsNumericCondition(indicatorType: TechnicalIndicatorType): boolean {
  return getTechnicalIndicatorDefinition(indicatorType).conditionModes.includes("numeric");
}

export function supportsPatternCondition(indicatorType: TechnicalIndicatorType): boolean {
  return getTechnicalIndicatorDefinition(indicatorType).conditionModes.includes("pattern");
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

export function normalizeMovingAverageIndicatorType(
  value: unknown,
): MovingAverageIndicatorType {
  return MOVING_AVERAGE_INDICATOR_OPTIONS.some((option) => option.value === value)
    ? (value as MovingAverageIndicatorType)
    : "MA";
}

export function normalizeIndicatorPeriodUnit(
  value: unknown,
): IndicatorPeriodUnit {
  return INDICATOR_PERIOD_UNIT_OPTIONS.some((option) => option.value === value)
    ? (value as IndicatorPeriodUnit)
    : "bar";
}

export function normalizeTechnicalIndicatorConditionMode(
  value: unknown,
  indicatorType: TechnicalIndicatorType,
): TechnicalIndicatorConditionMode {
  const definition = getTechnicalIndicatorDefinition(indicatorType);
  if (value === "none") {
    return "none";
  }
  if (value === "pattern" && definition.conditionModes.includes("pattern")) {
    return "pattern";
  }
  if (value === "numeric" && definition.conditionModes.includes("numeric")) {
    return "numeric";
  }
  return definition.defaultConditionMode;
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
      normalized.movingAverageType = normalizeMovingAverageIndicatorType(
        properties.movingAverageType,
      );
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

export function normalizeGetTechnicalIndicatorProperties(
  properties: Record<string, unknown>,
): GetTechnicalIndicatorBlockProperties {
  const indicatorType = normalizeTechnicalIndicatorType(properties.indicatorType);
  const definition = getTechnicalIndicatorDefinition(indicatorType);
  const normalized: GetTechnicalIndicatorBlockProperties = {
    blockKind: "getTechnicalIndicator",
    indicatorType,
  };
  const variableName = normalizeStrategyIndicatorVariableName(properties.variableName);
  if (variableName !== undefined) {
    normalized.variableName = variableName;
  }

  switch (definition.parameterShape) {
    case "windowSize":
      normalized.movingAverageType = normalizeMovingAverageIndicatorType(
        properties.movingAverageType,
      );
      normalized.periodUnit = normalizeIndicatorPeriodUnit(properties.periodUnit);
      normalized.windowSize = normalizeInteger(
        properties.windowSize ?? properties.period,
        definition.defaultWindowSize ?? 20,
      );
      break;
    case "macd":
      normalized.fastPeriod = normalizeInteger(
        properties.fastPeriod,
        definition.defaultFastPeriod ?? 12,
      );
      normalized.slowPeriod = normalizeInteger(
        properties.slowPeriod,
        definition.defaultSlowPeriod ?? 26,
      );
      normalized.signalPeriod = normalizeInteger(
        properties.signalPeriod,
        definition.defaultSignalPeriod ?? 9,
      );
      break;
    case "kdj":
      normalized.period = normalizeInteger(
        properties.period,
        definition.defaultPeriod ?? 9,
      );
      normalized.m1 = normalizeInteger(properties.m1, definition.defaultM1 ?? 3);
      normalized.m2 = normalizeInteger(properties.m2, definition.defaultM2 ?? 3);
      break;
    case "bollinger":
      normalized.period = normalizeInteger(
        properties.period,
        definition.defaultPeriod ?? 20,
      );
      normalized.multiplier = normalizeDecimal(
        properties.multiplier,
        definition.defaultMultiplier ?? 2,
      );
      break;
    default:
      normalized.period = normalizeInteger(
        properties.period,
        definition.defaultPeriod ?? 14,
      );
      break;
  }

  return normalized;
}

export function normalizeTechnicalIndicatorConditionProperties(
  properties: Record<string, unknown>,
): TechnicalIndicatorConditionBlockProperties {
  const indicatorType = normalizeTechnicalIndicatorType(properties.indicatorType);
  const conditionMode = normalizeTechnicalIndicatorConditionMode(
    properties.conditionMode,
    indicatorType,
  );
  const normalized: TechnicalIndicatorConditionBlockProperties = {
    blockKind: "technicalIndicatorCondition",
    indicatorType,
    conditionMode: conditionMode === "none"
      ? getTechnicalIndicatorDefinition(indicatorType).defaultConditionMode
      : conditionMode,
  };
  const allowedSlots = new Set(getTechnicalIndicatorInputSlots(indicatorType));
  const inputPrimaryNodeId = normalizeStrategyIndicatorReferenceNodeId(properties.inputPrimaryNodeId);
  const inputFastNodeId = normalizeStrategyIndicatorReferenceNodeId(properties.inputFastNodeId);
  const inputSlowNodeId = normalizeStrategyIndicatorReferenceNodeId(properties.inputSlowNodeId);
  if (allowedSlots.has("primary") && inputPrimaryNodeId !== undefined) {
    normalized.inputPrimaryNodeId = inputPrimaryNodeId;
  }
  if (allowedSlots.has("fast") && inputFastNodeId !== undefined) {
    normalized.inputFastNodeId = inputFastNodeId;
  }
  if (allowedSlots.has("slow") && inputSlowNodeId !== undefined) {
    normalized.inputSlowNodeId = inputSlowNodeId;
  }

  if (normalized.conditionMode === "numeric") {
    normalized.operator = normalizeTechnicalIndicatorOperator(properties.operator);
    normalized.threshold = normalizeDecimal(
      properties.threshold,
      defaultThresholdForIndicator(indicatorType, normalized.operator),
    );
  }

  if (normalized.conditionMode === "pattern") {
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

export function nextGetTechnicalIndicatorNodeText(
  rawProperties: Record<string, unknown>,
): string {
  const properties = normalizeGetTechnicalIndicatorProperties(rawProperties);
  const indicatorLabel = getTechnicalIndicatorGetterLabel(properties.indicatorType);
  return `获取 ${indicatorLabel} ${indicatorInputParameterText(properties)}`.trim();
}

export function nextTechnicalIndicatorConditionNodeText(
  rawProperties: Record<string, unknown>,
): string {
  const properties = normalizeTechnicalIndicatorConditionProperties(rawProperties);
  const indicatorLabel = indicatorTypeLabel(properties.indicatorType);

  if (properties.conditionMode === "numeric") {
    return `${indicatorLabel} ${properties.operator ?? "<"} ${formatThreshold(properties.threshold ?? 0)}`;
  }

  const patternLabel = patternTypeLabel(properties.patternType ?? "goldenCross");
  if (isDivergencePattern(properties.patternType)) {
    return `${indicatorLabel} ${patternLabel} (${properties.lookback ?? 5})`;
  }
  return `${indicatorLabel} ${patternLabel}`;
}

export function indicatorTypeLabel(indicatorType: TechnicalIndicatorType): string {
  return getTechnicalIndicatorDefinition(indicatorType).label;
}

export function normalizeStrategyIndicatorVariableName(
  value: unknown,
): string | undefined {
  if (typeof value !== "string") {
    return undefined;
  }

  const normalized = value.replace(/\s+/g, " ").trim();
  return normalized === "" ? undefined : normalized;
}

export function normalizeStrategyIndicatorReferenceNodeId(
  value: unknown,
): string | undefined {
  if (typeof value !== "string") {
    return undefined;
  }

  const normalized = value.trim();
  return normalized === "" ? undefined : normalized;
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

function indicatorInputParameterText(
  properties: GetTechnicalIndicatorBlockProperties,
): string {
  switch (properties.indicatorType) {
    case "movingAverage":
      return `${properties.movingAverageType ?? "MA"} ${properties.windowSize ?? 20}${indicatorPeriodUnitSuffix(properties.periodUnit ?? "bar")}`;
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
  return getTechnicalIndicatorDefinition(indicatorType).defaultPeriod ?? 14;
}

export function indicatorPeriodUnitLabel(unit: IndicatorPeriodUnit): string {
  switch (unit) {
    case "minute":
      return "分钟";
    case "hour":
      return "小时";
    case "day":
      return "日";
    case "week":
      return "周";
    case "month":
      return "月";
    case "bar":
    default:
      return "柱";
  }
}

function indicatorPeriodUnitSuffix(unit: IndicatorPeriodUnit): string {
  return unit === "bar" ? "" : indicatorPeriodUnitLabel(unit);
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
