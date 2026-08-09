import {
  normalizeVisualExpression,
  parsePineExpressionToVisualExpression,
  sourceExpression,
  type VisualExpression,
} from "./strategyVisualBuilderExpressions";
import {
  normalizeSeriesSource,
  type StateValueType,
  type StrategyInputType,
} from "./strategyVisualBuilderCatalog";

export function normalizeStopLossInteger(value: unknown, fallback: number): number {
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

export function normalizeNonNegativeInteger(value: unknown, fallback: number): number {
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

export function normalizeClockHour(value: unknown, fallback: number): number {
  return Math.min(23, Math.max(0, normalizeIntegerValue(value, fallback)));
}

export function normalizeClockMinute(value: unknown, fallback: number): number {
  return Math.min(59, Math.max(0, normalizeIntegerValue(value, fallback)));
}

export function normalizeDayOfWeek(value: unknown, fallback: number): number {
  return Math.min(7, Math.max(1, normalizeIntegerValue(value, fallback)));
}

export function formatClock(hour: number, minute: number): string {
  return `${String(normalizeClockHour(hour, 0)).padStart(2, "0")}:${String(normalizeClockMinute(minute, 0)).padStart(2, "0")}`;
}

export function normalizeIntegerValue(value: unknown, fallback: number): number {
  if (typeof value === "number" && Number.isFinite(value)) {
    return Math.round(value);
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) {
      return Math.round(parsed);
    }
  }
  return fallback;
}

export function normalizeStopLossDecimal(value: unknown, fallback: number): number {
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

export function normalizePineName(value: unknown, fallback: string): string {
  const raw = typeof value === "string" ? value.trim() : "";
  const normalized = raw
    .replace(/[^A-Za-z0-9_]+/g, "_")
    .replace(/^([0-9])/, "_$1")
    .replace(/^_+|_+$/g, "");
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(normalized) ? normalized : fallback;
}

export function normalizePineField(value: unknown): string | undefined {
  if (typeof value !== "string") {
    return undefined;
  }
  const trimmed = value.trim();
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(trimmed) ? trimmed : undefined;
}

export function normalizeSafeExpression(value: unknown, fallback: string): string {
  const raw = typeof value === "string" ? value.trim() : "";
  if (raw === "" || /(?:array\.|map\.|matrix\.|request\.security|strategy\.|line\.|label\.|table\.|:=|\bfor\b|\bwhile\b)/i.test(raw)) {
    return fallback;
  }
  return raw.replace(/[\r\n]+/g, " ");
}

export function normalizeTimeframe(value: unknown): string {
  const raw = typeof value === "string" ? value.trim().toUpperCase() : "";
  return ["1", "5", "15", "30", "45", "60", "120", "240", "D", "W", "M"].includes(raw)
    ? raw
    : "D";
}

export function normalizeInputDefaultValue(
  inputType: StrategyInputType,
  value: unknown,
): number | string {
  switch (inputType) {
    case "float":
      return normalizeStopLossDecimal(value, 2);
    case "source":
      return normalizeSeriesSource(value);
    case "timeframe":
      return normalizeTimeframe(value);
    case "time":
      return typeof value === "string" && value.trim() !== "" ? value.trim() : "timestamp(2026, 1, 1)";
    case "color":
      return typeof value === "string" && value.trim() !== "" ? value.trim() : "color.green";
    case "int":
    default:
      return normalizeStopLossInteger(value, 20);
  }
}

export function normalizeStateInitialValue(
  valueType: StateValueType,
  value: unknown,
): number | boolean | string {
  switch (valueType) {
    case "number":
      return normalizeStopLossDecimal(value, 0);
    case "string":
      return typeof value === "string" ? value : "";
    case "bool":
    default:
      return value === true || value === "true";
  }
}

export function isOneOf<const T extends string>(
  value: unknown,
  options: readonly T[],
): value is T {
  return typeof value === "string" && (options as readonly string[]).includes(value);
}
