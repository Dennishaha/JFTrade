export const SOURCE_EXTRA_ARGS_KEY = "__sourceExtraArgs";

export function splitCallArgs(value: string): string[] {
  const args: string[] = [];
  let current = "";
  let depth = 0;
  let quote: string | null = null;
  for (const char of value) {
    if (quote !== null) {
      current += char;
      if (char === quote) quote = null;
      continue;
    }
    if (char === "\"" || char === "'") {
      quote = char;
      current += char;
      continue;
    }
    if (char === "(" || char === "[" || char === "{") depth += 1;
    if (char === ")" || char === "]" || char === "}") depth = Math.max(0, depth - 1);
    if (char === "," && depth === 0) {
      args.push(current.trim());
      current = "";
      continue;
    }
    current += char;
  }
  if (current.trim() !== "") args.push(current.trim());
  return args;
}

export function trimDetail(value: string, fallback: string): string {
  const normalized = value.trim();
  const detail = normalized === "" ? fallback.trim() : normalized;
  return detail.length > 96 ? `${detail.slice(0, 93)}...` : detail;
}

export function unquote(value: string): string {
  const text = value.trim();
  if ((text.startsWith("\"") && text.endsWith("\"")) || (text.startsWith("'") && text.endsWith("'"))) {
    return text.slice(1, -1);
  }
  return text;
}
