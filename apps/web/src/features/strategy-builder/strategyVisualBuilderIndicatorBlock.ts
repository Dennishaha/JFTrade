export type TechnicalIndicatorType =
  | "movingAverage"
  | "rsi"
  | "macd"
  | "kdj"
  | "atr"
  | "cci"
  | "williamsR"
  | "bollinger"
  | "stdev"
  | "variance"
  | "highest"
  | "lowest"
  | "sum"
  | "vwap"
  | "mfi"
  | "dmi"
  | "supertrend"
  | "sar"
  | "linreg"
  | "obv"
  | "pivotHigh"
  | "pivotLow"
  | "keltner"
  | "alma";

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

export type IndicatorTimeframe = "" | "1" | "5" | "15" | "30" | "45" | "60" | "120" | "240" | "D" | "W" | "M";

export type TechnicalIndicatorInputSlot = "primary" | "fast" | "slow";

export type TechnicalIndicatorPatternType =
  | "goldenCross"
  | "deathCross"
  | "topDivergence"
  | "bottomDivergence"
  | "closeAboveUpperBand"
  | "closeBelowLowerBand";

export interface GetTechnicalIndicatorBlockProperties {
  blockKind: "getTechnicalIndicator";
  indicatorType: TechnicalIndicatorType;
  variableName?: string;
  source?: "open" | "high" | "low" | "close" | "volume" | "hl2" | "hlc3" | "ohlc4";
  movingAverageType?: MovingAverageIndicatorType;
  timeframe?: IndicatorTimeframe;
  period?: number;
  windowSize?: number;
  fastPeriod?: number;
  slowPeriod?: number;
  signalPeriod?: number;
  m1?: number;
  m2?: number;
  multiplier?: number;
  factor?: number;
  adxSmoothing?: number;
  start?: number;
  increment?: number;
  maximum?: number;
  offset?: number;
  sigma?: number;
  leftBars?: number;
  rightBars?: number;
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

export interface IndicatorTimeframeOption {
  value: IndicatorTimeframe;
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
  capabilityId: string;
  parameterShape:
    | "windowSize"
    | "period"
    | "sourceOnly"
    | "macd"
    | "kdj"
    | "bollinger"
    | "dmi"
    | "supertrend"
    | "sar"
    | "linreg"
    | "pivot"
    | "sourcePeriodMultiplier"
    | "alma";
  defaultSource?: GetTechnicalIndicatorBlockProperties["source"];
  defaultPeriod?: number;
  defaultWindowSize?: number;
  defaultFastPeriod?: number;
  defaultSlowPeriod?: number;
  defaultSignalPeriod?: number;
  defaultM1?: number;
  defaultM2?: number;
  defaultMultiplier?: number;
  defaultFactor?: number;
  defaultAdxSmoothing?: number;
  defaultStart?: number;
  defaultIncrement?: number;
  defaultMaximum?: number;
  defaultOffset?: number;
  defaultSigma?: number;
  defaultLeftBars?: number;
  defaultRightBars?: number;
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
  { value: "stdev", label: "标准差" },
  { value: "variance", label: "方差" },
  { value: "highest", label: "最高值" },
  { value: "lowest", label: "最低值" },
  { value: "sum", label: "区间求和" },
  { value: "vwap", label: "VWAP" },
  { value: "mfi", label: "MFI" },
  { value: "dmi", label: "DMI/ADX" },
  { value: "supertrend", label: "Supertrend" },
  { value: "sar", label: "Parabolic SAR" },
  { value: "linreg", label: "线性回归" },
  { value: "obv", label: "OBV" },
  { value: "pivotHigh", label: "Pivot High" },
  { value: "pivotLow", label: "Pivot Low" },
  { value: "keltner", label: "Keltner 通道" },
  { value: "alma", label: "ALMA" },
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

export const INDICATOR_TIMEFRAME_OPTIONS: IndicatorTimeframeOption[] = [
  { value: "", label: "当前周期" },
  { value: "1", label: "1分钟" },
  { value: "5", label: "5分钟" },
  { value: "15", label: "15分钟" },
  { value: "30", label: "30分钟" },
  { value: "45", label: "45分钟" },
  { value: "60", label: "1小时" },
  { value: "120", label: "2小时" },
  { value: "240", label: "4小时" },
  { value: "D", label: "日线" },
  { value: "W", label: "周线" },
  { value: "M", label: "月线" },
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
    capabilityId: "indicator.ma_source_aware",
    parameterShape: "windowSize",
    defaultSource: "close",
    defaultWindowSize: 20,
  },
  rsi: {
    label: "RSI",
    conditionModes: ["numeric", "pattern"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.rsi",
    parameterShape: "period",
    defaultSource: "close",
    defaultPeriod: 14,
    numericTargetLabel: "RSI 值",
  },
  macd: {
    label: "MACD",
    conditionModes: ["numeric", "pattern"],
    defaultConditionMode: "pattern",
    capabilityId: "indicator.macd",
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
    capabilityId: "indicator.v15_common_ta_set",
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
    capabilityId: "indicator.atr",
    parameterShape: "period",
    defaultPeriod: 14,
    numericTargetLabel: "ATR 值",
  },
  cci: {
    label: "CCI",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.cci",
    parameterShape: "period",
    defaultSource: "hlc3",
    defaultPeriod: 20,
    numericTargetLabel: "CCI 值",
  },
  williamsR: {
    label: "Williams %R",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.williams_r",
    parameterShape: "period",
    defaultPeriod: 14,
    numericTargetLabel: "Williams %R 值",
  },
  bollinger: {
    label: "布林带",
    conditionModes: ["pattern"],
    defaultConditionMode: "pattern",
    capabilityId: "indicator.bollinger",
    parameterShape: "bollinger",
    defaultSource: "close",
    defaultPeriod: 20,
    defaultMultiplier: 2,
  },
  stdev: {
    label: "标准差",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.source_aware_core",
    parameterShape: "period",
    defaultSource: "close",
    defaultPeriod: 20,
    numericTargetLabel: "标准差",
  },
  variance: {
    label: "方差",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.source_aware_core",
    parameterShape: "period",
    defaultSource: "close",
    defaultPeriod: 20,
    numericTargetLabel: "方差",
  },
  highest: {
    label: "最高值",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.rolling_window",
    parameterShape: "period",
    defaultSource: "high",
    defaultPeriod: 20,
    numericTargetLabel: "最高值",
  },
  lowest: {
    label: "最低值",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.rolling_window",
    parameterShape: "period",
    defaultSource: "low",
    defaultPeriod: 20,
    numericTargetLabel: "最低值",
  },
  sum: {
    label: "区间求和",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.sum",
    parameterShape: "period",
    defaultSource: "volume",
    defaultPeriod: 20,
    numericTargetLabel: "求和值",
  },
  vwap: {
    label: "VWAP",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.vwap_mfi_dmi_supertrend",
    parameterShape: "sourceOnly",
    defaultSource: "hlc3",
    numericTargetLabel: "VWAP",
  },
  mfi: {
    label: "MFI",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.vwap_mfi_dmi_supertrend",
    parameterShape: "period",
    defaultSource: "hlc3",
    defaultPeriod: 14,
    numericTargetLabel: "MFI",
  },
  dmi: {
    label: "DMI/ADX",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.vwap_mfi_dmi_supertrend",
    parameterShape: "dmi",
    defaultPeriod: 14,
    defaultAdxSmoothing: 14,
    numericTargetLabel: "ADX",
  },
  supertrend: {
    label: "Supertrend",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.vwap_mfi_dmi_supertrend",
    parameterShape: "supertrend",
    defaultPeriod: 10,
    defaultFactor: 3,
    numericTargetLabel: "方向",
  },
  sar: {
    label: "Parabolic SAR",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.sar",
    parameterShape: "sar",
    defaultStart: 0.02,
    defaultIncrement: 0.02,
    defaultMaximum: 0.2,
    numericTargetLabel: "SAR 值",
  },
  linreg: {
    label: "线性回归",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.linreg_obv_pivots",
    parameterShape: "linreg",
    defaultSource: "close",
    defaultPeriod: 5,
    defaultOffset: 0,
    numericTargetLabel: "线性回归值",
  },
  obv: {
    label: "OBV",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.linreg_obv_pivots",
    parameterShape: "sourceOnly",
    defaultSource: "close",
    numericTargetLabel: "OBV",
  },
  pivotHigh: {
    label: "Pivot High",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.linreg_obv_pivots",
    parameterShape: "pivot",
    defaultSource: "high",
    defaultLeftBars: 2,
    defaultRightBars: 2,
    numericTargetLabel: "Pivot High",
  },
  pivotLow: {
    label: "Pivot Low",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.linreg_obv_pivots",
    parameterShape: "pivot",
    defaultSource: "low",
    defaultLeftBars: 2,
    defaultRightBars: 2,
    numericTargetLabel: "Pivot Low",
  },
  keltner: {
    label: "Keltner 通道",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.keltner_alma",
    parameterShape: "sourcePeriodMultiplier",
    defaultSource: "close",
    defaultPeriod: 20,
    defaultMultiplier: 1.5,
    numericTargetLabel: "上轨",
  },
  alma: {
    label: "ALMA",
    conditionModes: ["numeric"],
    defaultConditionMode: "numeric",
    capabilityId: "indicator.keltner_alma",
    parameterShape: "alma",
    defaultSource: "close",
    defaultPeriod: 20,
    defaultOffset: 0.85,
    defaultSigma: 6,
    numericTargetLabel: "ALMA",
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
  stdev: [],
  variance: [],
  highest: [],
  lowest: [],
  sum: [],
  vwap: [],
  mfi: [],
  dmi: [],
  supertrend: [],
  sar: [],
  linreg: [],
  obv: [],
  pivotHigh: [],
  pivotLow: [],
  keltner: [],
  alma: [],
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
  stdev: ["primary"],
  variance: ["primary"],
  highest: ["primary"],
  lowest: ["primary"],
  sum: ["primary"],
  vwap: ["primary"],
  mfi: ["primary"],
  dmi: ["primary"],
  supertrend: ["primary"],
  sar: ["primary"],
  linreg: ["primary"],
  obv: ["primary"],
  pivotHigh: ["primary"],
  pivotLow: ["primary"],
  keltner: ["primary"],
  alma: ["primary"],
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

export function normalizeIndicatorTimeframe(value: unknown): IndicatorTimeframe {
  if (typeof value !== "string") {
    return "";
  }
  const normalized = value.trim().toUpperCase();
  switch (normalized) {
    case "":
      return "";
    case "1":
    case "5":
    case "15":
    case "30":
    case "45":
    case "60":
    case "120":
    case "240":
    case "D":
    case "W":
    case "M":
      return normalized as IndicatorTimeframe;
    default:
      return "";
  }
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
  const timeframe = normalizeIndicatorTimeframe(properties.timeframe);
  if (timeframe !== "") {
    normalized.timeframe = timeframe;
  }

  switch (definition.parameterShape) {
    case "windowSize":
      normalized.movingAverageType = normalizeMovingAverageIndicatorType(
        properties.movingAverageType,
      );
      normalized.source = normalizeIndicatorSource(properties.source, definition.defaultSource);
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
    case "sourcePeriodMultiplier":
      normalized.source = normalizeIndicatorSource(properties.source, definition.defaultSource);
      normalized.period = normalizeInteger(
        properties.period,
        definition.defaultPeriod ?? 20,
      );
      normalized.multiplier = normalizeDecimal(
        properties.multiplier,
        definition.defaultMultiplier ?? 2,
      );
      break;
    case "sourceOnly":
      normalized.source = normalizeIndicatorSource(properties.source, definition.defaultSource);
      break;
    case "dmi":
      normalized.period = normalizeInteger(
        properties.period,
        definition.defaultPeriod ?? 14,
      );
      normalized.adxSmoothing = normalizeInteger(
        properties.adxSmoothing,
        definition.defaultAdxSmoothing ?? 14,
      );
      break;
    case "supertrend":
      normalized.period = normalizeInteger(
        properties.period,
        definition.defaultPeriod ?? 10,
      );
      normalized.factor = normalizeDecimal(
        properties.factor,
        definition.defaultFactor ?? 3,
      );
      break;
    case "sar":
      normalized.start = normalizePositiveDecimal(
        properties.start,
        definition.defaultStart ?? 0.02,
      );
      normalized.increment = normalizePositiveDecimal(
        properties.increment,
        definition.defaultIncrement ?? 0.02,
      );
      normalized.maximum = normalizePositiveDecimal(
        properties.maximum,
        definition.defaultMaximum ?? 0.2,
      );
      break;
    case "linreg":
      normalized.source = normalizeIndicatorSource(properties.source, definition.defaultSource);
      normalized.period = normalizeInteger(
        properties.period,
        definition.defaultPeriod ?? 5,
      );
      normalized.offset = normalizeNonNegativeInteger(
        properties.offset,
        definition.defaultOffset ?? 0,
      );
      break;
    case "pivot":
      normalized.source = normalizeIndicatorSource(properties.source, definition.defaultSource);
      normalized.leftBars = normalizeInteger(
        properties.leftBars,
        definition.defaultLeftBars ?? 2,
      );
      normalized.rightBars = normalizeInteger(
        properties.rightBars,
        definition.defaultRightBars ?? 2,
      );
      break;
    case "alma":
      normalized.source = normalizeIndicatorSource(properties.source, definition.defaultSource);
      normalized.period = normalizeInteger(
        properties.period,
        definition.defaultPeriod ?? 20,
      );
      normalized.offset = normalizeDecimal(
        properties.offset,
        definition.defaultOffset ?? 0.85,
      );
      normalized.sigma = normalizePositiveDecimal(
        properties.sigma,
        definition.defaultSigma ?? 6,
      );
      break;
    default:
      normalized.source = normalizeIndicatorSource(properties.source, definition.defaultSource);
      normalized.period = normalizeInteger(
        properties.period,
        definition.defaultPeriod ?? 14,
      );
      break;
  }

  return normalized;
}

function normalizeIndicatorSource(
  value: unknown,
  fallback: GetTechnicalIndicatorBlockProperties["source"] = "close",
): NonNullable<GetTechnicalIndicatorBlockProperties["source"]> {
	const normalizedValue = typeof value === "string" ? value.trim().toLowerCase() : "";
	switch (normalizedValue) {
		case "open":
		case "high":
		case "low":
		case "volume":
		case "hl2":
		case "hlc3":
		case "ohlc4":
			return normalizedValue;
		case "close":
			return "close";
		default:
			return fallback ?? "close";
	}
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

function indicatorInputParameterText(
  properties: GetTechnicalIndicatorBlockProperties,
): string {
  const timeframeSuffix = indicatorTimeframeSuffix(properties.timeframe ?? "");
  switch (properties.indicatorType) {
    case "movingAverage":
      return `${properties.movingAverageType ?? "MA"} ${properties.windowSize ?? 20}${timeframeSuffix}`;
    case "macd":
      return `${properties.fastPeriod ?? 12}/${properties.slowPeriod ?? 26}/${properties.signalPeriod ?? 9}`;
    case "kdj":
      return `${properties.period ?? 9}/${properties.m1 ?? 3}/${properties.m2 ?? 3}`;
    case "bollinger":
      return `${properties.period ?? 20}x${formatThreshold(properties.multiplier ?? 2)}`;
    case "vwap":
    case "obv":
      return properties.source ?? getTechnicalIndicatorDefinition(properties.indicatorType).defaultSource ?? "close";
    case "dmi":
      return `${properties.period ?? 14}/${properties.adxSmoothing ?? 14}`;
    case "supertrend":
      return `${formatThreshold(properties.factor ?? 3)}/${properties.period ?? 10}`;
    case "sar":
      return `${formatThreshold(properties.start ?? 0.02)}/${formatThreshold(properties.increment ?? 0.02)}/${formatThreshold(properties.maximum ?? 0.2)}`;
    case "linreg":
      return `${properties.period ?? 5}/${properties.offset ?? 0}`;
    case "pivotHigh":
    case "pivotLow":
      return `${properties.leftBars ?? 2}/${properties.rightBars ?? 2}`;
    case "keltner":
      return `${properties.period ?? 20}x${formatThreshold(properties.multiplier ?? 1.5)}`;
    case "alma":
      return `${properties.period ?? 20}/${formatThreshold(properties.offset ?? 0.85)}/${formatThreshold(properties.sigma ?? 6)}`;
    default:
      return String(properties.period ?? defaultPeriodForIndicator(properties.indicatorType));
  }
}

function defaultPeriodForIndicator(indicatorType: TechnicalIndicatorType): number {
  return getTechnicalIndicatorDefinition(indicatorType).defaultPeriod ?? 14;
}

export function indicatorTimeframeLabel(timeframe: IndicatorTimeframe): string {
  return INDICATOR_TIMEFRAME_OPTIONS.find((option) => option.value === timeframe)?.label ?? timeframe;
}

function indicatorTimeframeSuffix(timeframe: IndicatorTimeframe): string {
  return timeframe === "" ? "" : indicatorTimeframeLabel(timeframe);
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
    case "mfi":
      return operator === ">" ? 80 : 20;
    case "dmi":
      return operator === ">" ? 25 : 20;
    case "supertrend":
      return 0;
    default:
      return 0;
  }
}

function normalizeNonNegativeInteger(value: unknown, fallback: number): number {
  if (typeof value === "number" && Number.isFinite(value)) {
    return Math.max(0, Math.round(value));
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) {
      return Math.max(0, Math.round(parsed));
    }
  }
  return fallback;
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

function normalizePositiveDecimal(value: unknown, fallback: number): number {
  const normalized = normalizeDecimal(value, fallback);
  return normalized > 0 ? normalized : fallback;
}

function formatThreshold(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(2).replace(/\.00$/, "");
}
