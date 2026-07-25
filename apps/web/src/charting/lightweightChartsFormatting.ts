import type { KlineCandle } from "./kline";

const DEFAULT_PRICE_PRECISION = 3;
const MAX_PRICE_PRECISION = 8;
const SCIENTIFIC_PRICE_THRESHOLD = 0.001;

type CompactUnit = {
  divisor: number;
  suffix: "K" | "M" | "B";
};

const VOLUME_UNITS: readonly CompactUnit[] = [
  { divisor: 1_000, suffix: "K" },
  { divisor: 1_000_000, suffix: "M" },
  { divisor: 1_000_000_000, suffix: "B" },
];

export function formatKlinePrice(price: number): string {
  if (!Number.isFinite(price)) return String(price);
  const absolute = Math.abs(price);
  if (absolute > 0 && absolute < SCIENTIFIC_PRICE_THRESHOLD) {
    const [coefficient = "0", exponent = "0"] = price
      .toExponential(2)
      .split("e");
    return `${parseFloat(coefficient)}e${exponent}`;
  }
  return String(parseFloat(price.toFixed(3)));
}

function pricePrecision(value: number): number {
  const fixed = Math.abs(value).toFixed(MAX_PRICE_PRECISION);
  const fractional = fixed.split(".")[1]?.replace(/0+$/u, "") ?? "";
  return fractional.length;
}

export function resolveKlinePricePrecision(
  candles: readonly KlineCandle[],
): number {
  let precision = 0;
  for (const candle of candles) {
    precision = Math.max(
      precision,
      pricePrecision(candle.open),
      pricePrecision(candle.high),
      pricePrecision(candle.low),
      pricePrecision(candle.close),
    );
  }
  return Math.min(
    MAX_PRICE_PRECISION,
    Math.max(DEFAULT_PRICE_PRECISION, precision),
  );
}

export function createKlinePriceFormat(
  precision = DEFAULT_PRICE_PRECISION,
) {
  return {
    type: "custom" as const,
    minMove: 10 ** -precision,
    formatter: formatKlinePrice,
  };
}

function volumeFractionDigits(value: number): number {
  const absolute = Math.abs(value);
  if (absolute >= 100) return 0;
  if (absolute >= 10) return 1;
  return 2;
}

function scaledVolume(volume: number, unit: CompactUnit): number {
  const scaled = volume / unit.divisor;
  return parseFloat(scaled.toFixed(volumeFractionDigits(scaled)));
}

export function formatKlineVolume(volume: number): string {
  if (!Number.isFinite(volume)) return formatKlinePrice(volume);
  const absolute = Math.abs(volume);
  let unitIndex = -1;
  for (let index = VOLUME_UNITS.length - 1; index >= 0; index -= 1) {
    if (absolute >= VOLUME_UNITS[index]!.divisor) {
      unitIndex = index;
      break;
    }
  }
  if (unitIndex < 0) return formatKlinePrice(volume);

  let unit = VOLUME_UNITS[unitIndex]!;
  let scaled = scaledVolume(volume, unit);
  if (Math.abs(scaled) >= 1_000 && unitIndex < VOLUME_UNITS.length - 1) {
    unit = VOLUME_UNITS[++unitIndex]!;
    scaled = scaledVolume(volume, unit);
  }
  return `${scaled}${unit.suffix}`;
}

export const KLINE_VOLUME_PRICE_FORMAT = {
  type: "custom" as const,
  minMove: 1,
  formatter: formatKlineVolume,
};
