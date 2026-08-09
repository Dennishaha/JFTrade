export function sanitizeMetadataValue(value: string, fallback: string): string {
  const normalized = value.replace(/[\r\n]+/g, " ").trim();
  return normalized === "" ? fallback : normalized;
}

export function sanitizePineIdentifier(value: string, fallback: string): string {
  const normalized = value
    .trim()
    .replace(/[^A-Za-z0-9_]+/g, "_")
    .replace(/^([0-9])/, "_$1")
    .replace(/^_+|_+$/g, "");
  return normalized === "" ? fallback : normalized;
}

export function isPineIdentifier(value: string): boolean {
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(value);
}

export function toPineStringLiteral(value: string): string {
  return JSON.stringify(value.replace(/\r?\n/g, " ").trim());
}

export function formatNumber(value: number): string {
  return Number.isFinite(value) ? String(value) : "0";
}

export function formatPineValue(value: unknown): string {
  if (typeof value === "number" && Number.isFinite(value)) {
    return String(value);
  }
  if (typeof value === "boolean") {
    return value ? "true" : "false";
  }
  if (typeof value === "string") {
    const trimmed = value.trim();
    if (/^(?:timestamp|color\.[A-Za-z_][A-Za-z0-9_]*|#[0-9A-Fa-f]{6}|open|high|low|close|volume|hl2|hlc3|ohlc4)\b/.test(trimmed)) {
      return trimmed;
    }
    return toPineStringLiteral(trimmed);
  }
  return "0";
}

export function indent(depth: number): string {
  return "  ".repeat(depth);
}
