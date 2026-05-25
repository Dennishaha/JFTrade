export function readTechnicalIndicatorProperties(source: string): Record<string, unknown> {
  if (source.includes('ctx.indicators["ma:')) {
    const matches = [...source.matchAll(/ctx\.indicators\["ma:(\d+)"\]/g)];
    return {
      blockKind: "technicalIndicator",
      indicatorType: "movingAverage",
      conditionMode: source.includes("if (prevFastAverage") ? "pattern" : "none",
      patternType: source.includes("fastAverage < slowAverage") ? "deathCross" : "goldenCross",
      fastPeriod: Number(matches[0]?.[1] ?? 5),
      slowPeriod: Number(matches[1]?.[1] ?? 20),
    };
  }
  if (source.includes('ctx.indicators["rsi:')) {
    const period = Number(source.match(/ctx\.indicators\["rsi:(\d+)"\]/)?.[1] ?? 14);
    if (source.includes('const divergenceSignal = ctx.indicators["divergence:rsi:')) {
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
  if (source.includes('ctx.indicators["macd:')) {
    const match = source.match(/ctx\.indicators\["macd:(\d+):(\d+):(\d+)"\]/);
    const fastPeriod = Number(match?.[1] ?? 12);
    const slowPeriod = Number(match?.[2] ?? 26);
    const signalPeriod = Number(match?.[3] ?? 9);
    if (source.includes('const divergenceSignal = ctx.indicators["divergence:macd:')) {
      return {
        blockKind: "technicalIndicator",
        indicatorType: "macd",
        conditionMode: "pattern",
        patternType: source.includes(":top:") ? "topDivergence" : "bottomDivergence",
        lookback: Number(source.match(/divergence:macd:[^\"]+:(\d+)/)?.[1] ?? 5),
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
  if (source.includes('ctx.indicators["kdj:')) {
    const match = source.match(/ctx\.indicators\["kdj:(\d+):(\d+):(\d+)"\]/);
    const period = Number(match?.[1] ?? 9);
    const m1 = Number(match?.[2] ?? 3);
    const m2 = Number(match?.[3] ?? 3);
    if (source.includes('const divergenceSignal = ctx.indicators["divergence:kdj:')) {
      return {
        blockKind: "technicalIndicator",
        indicatorType: "kdj",
        conditionMode: "pattern",
        patternType: source.includes(":top:") ? "topDivergence" : "bottomDivergence",
        lookback: Number(source.match(/divergence:kdj:[^\"]+:(\d+)/)?.[1] ?? 5),
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
  if (source.includes('ctx.indicators["atr:')) {
    const period = Number(source.match(/ctx\.indicators\["atr:(\d+)"\]/)?.[1] ?? 14);
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
  if (source.includes('ctx.indicators["cci:')) {
    const period = Number(source.match(/ctx\.indicators\["cci:(\d+)"\]/)?.[1] ?? 20);
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
  if (source.includes('ctx.indicators["williamsr:')) {
    const period = Number(source.match(/ctx\.indicators\["williamsr:(\d+)"\]/)?.[1] ?? 14);
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
  if (source.includes('ctx.indicators["bollinger:')) {
    const match = source.match(/ctx\.indicators\["bollinger:(\d+):(-?\d+(?:\.\d+)?)"\]/);
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

function readOptionalNumber(value: string | undefined): number | undefined {
  if (value === undefined) {
    return undefined;
  }
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}