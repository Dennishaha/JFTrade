import type {
  IndicatorPeriodUnit,
  MovingAverageIndicatorType,
} from "./strategyVisualBuilderIndicatorBlock";
import { normalizeIndicatorPeriodUnit } from "./strategyVisualBuilderIndicatorBlock";

interface MovingAverageMatch {
  movingAverageType: MovingAverageIndicatorType;
  period: number;
  periodUnit?: IndicatorPeriodUnit;
}

interface ParsedIndicatorKey {
  indicatorType:
    | "movingAverage"
    | "rsi"
    | "macd"
    | "kdj"
    | "atr"
    | "cci"
    | "williamsR"
    | "bollinger";
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

const MOVING_AVERAGE_INDICATOR_PATTERN = /ctx\.indicators\[(?:"|')ma:(?:(MA|EMA|SMA|SMMA|LWMA|TMA|EXPMA|HMA|VWMA|BOLL):)?(\d+)(?::(bar|minute|hour|day|week|month))?(?:"|')\]/g;

export function readTechnicalIndicatorProperties(source: string): Record<string, unknown> {
  const movingAverageMatches = readMovingAverageMatches(source);
  if (movingAverageMatches.length > 0) {
    return {
      blockKind: "technicalIndicator",
      indicatorType: "movingAverage",
      conditionMode: source.includes("if (prevFastAverage") ? "pattern" : "none",
      patternType: source.includes("fastAverage < slowAverage") ? "deathCross" : "goldenCross",
      movingAverageType: movingAverageMatches[0]?.movingAverageType ?? "MA",
      fastPeriod: movingAverageMatches[0]?.period ?? 5,
      slowPeriod: movingAverageMatches[1]?.period ?? 20,
    };
  }
  if (source.includes('ctx.indicators["rsi:') || source.includes("ctx.indicators['rsi:")) {
    const period = Number(source.match(/ctx\.indicators\[(?:"|')rsi:(\d+)(?:"|')\]/)?.[1] ?? 14);
    if (source.includes('const divergenceSignal = ctx.indicators["divergence:rsi:') || source.includes("const divergenceSignal = ctx.indicators['divergence:rsi:")) {
      return {
        blockKind: "technicalIndicator",
        indicatorType: "rsi",
        conditionMode: "pattern",
        patternType: source.includes(":top:") ? "topDivergence" : "bottomDivergence",
        lookback: Number(source.match(/divergence:rsi:\d+:(?:top|bottom):(\d+)/)?.[1] ?? 5),
        period,
      };
    }
    const numeric = source.match(/if \(latestRsi\s*([<>])\s*(-?\d+(?:\.\d+)?)\)/);
    return {
      blockKind: "technicalIndicator",
      indicatorType: "rsi",
      conditionMode: numeric === null ? "none" : "numeric",
      operator: numeric?.[1] ?? "<",
      threshold: numeric === null ? undefined : Number(numeric[2]),
      period,
    };
  }
  if (source.includes('ctx.indicators["macd:') || source.includes("ctx.indicators['macd:")) {
    const match = source.match(/ctx\.indicators\[(?:"|')macd:(\d+):(\d+):(\d+)(?:"|')\]/);
    const fastPeriod = Number(match?.[1] ?? 12);
    const slowPeriod = Number(match?.[2] ?? 26);
    const signalPeriod = Number(match?.[3] ?? 9);
    if (source.includes('const divergenceSignal = ctx.indicators["divergence:macd:') || source.includes("const divergenceSignal = ctx.indicators['divergence:macd:")) {
      return {
        blockKind: "technicalIndicator",
        indicatorType: "macd",
        conditionMode: "pattern",
        patternType: source.includes(":top:") ? "topDivergence" : "bottomDivergence",
        lookback: Number(source.match(/divergence:macd:[^"]+:(\d+)/)?.[1] ?? 5),
        fastPeriod,
        slowPeriod,
        signalPeriod,
      };
    }
    if (source.includes("latestMacd.previousDiff")) {
      return {
        blockKind: "technicalIndicator",
        indicatorType: "macd",
        conditionMode: "pattern",
        patternType: source.includes("latestMacdDiff < latestMacdSignal") ? "deathCross" : "goldenCross",
        fastPeriod,
        slowPeriod,
        signalPeriod,
      };
    }
    const numeric = source.match(/if \(latestMacdHistogram\s*([<>])\s*(-?\d+(?:\.\d+)?)\)/);
    return {
      blockKind: "technicalIndicator",
      indicatorType: "macd",
      conditionMode: numeric === null ? "none" : "numeric",
      operator: numeric?.[1] ?? ">",
      threshold: numeric === null ? undefined : Number(numeric[2]),
      fastPeriod,
      slowPeriod,
      signalPeriod,
    };
  }
  if (source.includes('ctx.indicators["kdj:') || source.includes("ctx.indicators['kdj:")) {
    const match = source.match(/ctx\.indicators\[(?:"|')kdj:(\d+):(\d+):(\d+)(?:"|')\]/);
    const period = Number(match?.[1] ?? 9);
    const m1 = Number(match?.[2] ?? 3);
    const m2 = Number(match?.[3] ?? 3);
    if (source.includes('const divergenceSignal = ctx.indicators["divergence:kdj:') || source.includes("const divergenceSignal = ctx.indicators['divergence:kdj:")) {
      return {
        blockKind: "technicalIndicator",
        indicatorType: "kdj",
        conditionMode: "pattern",
        patternType: source.includes(":top:") ? "topDivergence" : "bottomDivergence",
        lookback: Number(source.match(/divergence:kdj:[^"]+:(\d+)/)?.[1] ?? 5),
        period,
        m1,
        m2,
      };
    }
    if (source.includes("previousKValue")) {
      return {
        blockKind: "technicalIndicator",
        indicatorType: "kdj",
        conditionMode: source.includes("if (latestJValue") ? "numeric" : "pattern",
        patternType: source.includes("latestKValue < latestDValue") ? "deathCross" : "goldenCross",
        operator: source.match(/if \(latestJValue\s*([<>])/)?.[1],
        threshold: readOptionalNumber(source.match(/if \(latestJValue\s*[<>]\s*(-?\d+(?:\.\d+)?)\)/)?.[1]),
        period,
        m1,
        m2,
      };
    }
  }
  if (source.includes('ctx.indicators["atr:') || source.includes("ctx.indicators['atr:")) {
    const period = Number(source.match(/ctx\.indicators\[(?:"|')atr:(\d+)(?:"|')\]/)?.[1] ?? 14);
    const numeric = source.match(/if \(latestAtr\s*([<>])\s*(-?\d+(?:\.\d+)?)\)/);
    return {
      blockKind: "technicalIndicator",
      indicatorType: "atr",
      conditionMode: numeric === null ? "none" : "numeric",
      operator: numeric?.[1] ?? ">",
      threshold: numeric === null ? undefined : Number(numeric[2]),
      period,
    };
  }
  if (source.includes('ctx.indicators["cci:') || source.includes("ctx.indicators['cci:")) {
    const period = Number(source.match(/ctx\.indicators\[(?:"|')cci:(\d+)(?:"|')\]/)?.[1] ?? 20);
    const numeric = source.match(/if \(latestCci\s*([<>])\s*(-?\d+(?:\.\d+)?)\)/);
    return {
      blockKind: "technicalIndicator",
      indicatorType: "cci",
      conditionMode: numeric === null ? "none" : "numeric",
      operator: numeric?.[1] ?? ">",
      threshold: numeric === null ? undefined : Number(numeric[2]),
      period,
    };
  }
  if (source.includes('ctx.indicators["williamsr:') || source.includes("ctx.indicators['williamsr:")) {
    const period = Number(source.match(/ctx\.indicators\[(?:"|')williamsr:(\d+)(?:"|')\]/)?.[1] ?? 14);
    const numeric = source.match(/if \(latestWilliamsR\s*([<>])\s*(-?\d+(?:\.\d+)?)\)/);
    return {
      blockKind: "technicalIndicator",
      indicatorType: "williamsR",
      conditionMode: numeric === null ? "none" : "numeric",
      operator: numeric?.[1] ?? ">",
      threshold: numeric === null ? undefined : Number(numeric[2]),
      period,
    };
  }
  if (source.includes('ctx.indicators["bollinger:') || source.includes("ctx.indicators['bollinger:")) {
    const match = source.match(/ctx\.indicators\[(?:"|')bollinger:(\d+):(-?\d+(?:\.\d+)?)(?:"|')\]/);
    return {
      blockKind: "technicalIndicator",
      indicatorType: "bollinger",
      conditionMode: source.includes("if (close") ? "pattern" : "none",
      patternType: source.includes("if (close > latestBollingerUpper)") ? "closeAboveUpperBand" : "closeBelowLowerBand",
      period: Number(match?.[1] ?? 20),
      multiplier: Number(match?.[2] ?? 2),
    };
  }
  return {
    blockKind: "technicalIndicator",
    indicatorType: "rsi",
    conditionMode: "none",
    period: 14,
  };
}

export function readGetTechnicalIndicatorProperties(source: string): Record<string, unknown> {
  const key = readFirstIndicatorKey(source);
  if (key === null) {
    return {
      blockKind: "getTechnicalIndicator",
      indicatorType: "rsi",
      period: 14,
    };
  }

  switch (key.indicatorType) {
    case "movingAverage":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "movingAverage",
        movingAverageType: key.movingAverageType ?? "MA",
        windowSize: key.windowSize ?? 20,
        periodUnit: key.periodUnit ?? "bar",
      };
    case "rsi":
    case "atr":
    case "cci":
    case "williamsR":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: key.indicatorType,
        period: key.period ?? 14,
      };
    case "macd":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "macd",
        fastPeriod: key.fastPeriod ?? 12,
        slowPeriod: key.slowPeriod ?? 26,
        signalPeriod: key.signalPeriod ?? 9,
      };
    case "kdj":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "kdj",
        period: key.period ?? 9,
        m1: key.m1 ?? 3,
        m2: key.m2 ?? 3,
      };
    case "bollinger":
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "bollinger",
        period: key.period ?? 20,
        multiplier: key.multiplier ?? 2,
      };
    default:
      return {
        blockKind: "getTechnicalIndicator",
        indicatorType: "rsi",
        period: 14,
      };
  }
}

function readFirstIndicatorKey(source: string): ParsedIndicatorKey | null {
  const movingAverageMatch = readMovingAverageMatches(source)[0];
  if (movingAverageMatch !== undefined) {
    return {
      indicatorType: "movingAverage",
      movingAverageType: movingAverageMatch.movingAverageType,
      windowSize: movingAverageMatch.period,
      periodUnit: movingAverageMatch.periodUnit ?? "bar",
    };
  }

  const rsiMatch = source.match(/ctx\.indicators\[(?:"|')rsi:(\d+)(?:"|')\]/);
  if (rsiMatch !== null) {
    return { indicatorType: "rsi", period: Number(rsiMatch[1] ?? 14) };
  }

  const macdMatch = source.match(/ctx\.indicators\[(?:"|')macd:(\d+):(\d+):(\d+)(?:"|')\]/);
  if (macdMatch !== null) {
    return {
      indicatorType: "macd",
      fastPeriod: Number(macdMatch[1] ?? 12),
      slowPeriod: Number(macdMatch[2] ?? 26),
      signalPeriod: Number(macdMatch[3] ?? 9),
    };
  }

  const kdjMatch = source.match(/ctx\.indicators\[(?:"|')kdj:(\d+):(\d+):(\d+)(?:"|')\]/);
  if (kdjMatch !== null) {
    return {
      indicatorType: "kdj",
      period: Number(kdjMatch[1] ?? 9),
      m1: Number(kdjMatch[2] ?? 3),
      m2: Number(kdjMatch[3] ?? 3),
    };
  }

  const atrMatch = source.match(/ctx\.indicators\[(?:"|')atr:(\d+)(?:"|')\]/);
  if (atrMatch !== null) {
    return { indicatorType: "atr", period: Number(atrMatch[1] ?? 14) };
  }

  const cciMatch = source.match(/ctx\.indicators\[(?:"|')cci:(\d+)(?:"|')\]/);
  if (cciMatch !== null) {
    return { indicatorType: "cci", period: Number(cciMatch[1] ?? 20) };
  }

  const williamsRMatch = source.match(/ctx\.indicators\[(?:"|')williamsr:(\d+)(?:"|')\]/);
  if (williamsRMatch !== null) {
    return {
      indicatorType: "williamsR",
      period: Number(williamsRMatch[1] ?? 14),
    };
  }

  const bollingerMatch = source.match(/ctx\.indicators\[(?:"|')bollinger:(\d+):(-?\d+(?:\.\d+)?)(?:"|')\]/);
  if (bollingerMatch !== null) {
    return {
      indicatorType: "bollinger",
      period: Number(bollingerMatch[1] ?? 20),
      multiplier: Number(bollingerMatch[2] ?? 2),
    };
  }

  return null;
}

function readMovingAverageMatches(source: string): MovingAverageMatch[] {
  return [...source.matchAll(MOVING_AVERAGE_INDICATOR_PATTERN)].map((match) => ({
    movingAverageType: readMovingAverageIndicatorType(match[1]),
    period: Number(match[2] ?? 20),
    periodUnit: normalizeIndicatorPeriodUnit(match[3]),
  }));
}

function readMovingAverageIndicatorType(
  value: string | undefined,
): MovingAverageIndicatorType {
  switch (value) {
    case "EMA":
    case "SMA":
    case "SMMA":
    case "LWMA":
    case "TMA":
    case "EXPMA":
    case "HMA":
    case "VWMA":
    case "BOLL":
      return value;
    default:
      return "MA";
  }
}

function readOptionalNumber(value: string | undefined): number | undefined {
  if (value === undefined) {
    return undefined;
  }
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}