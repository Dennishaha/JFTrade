export type VisualExpressionNodeKind =
  | "literal"
  | "source"
  | "reference"
  | "field"
  | "history"
  | "unary"
  | "binary"
  | "call";

export type VisualExpressionValueType = "number" | "bool" | "string" | "series" | "unknown";

export type VisualExpressionCallFunction =
  | "math.min"
  | "math.max"
  | "math.abs"
  | "math.round"
  | "math.floor"
  | "math.ceil"
  | "nz"
  | "ta.crossover"
  | "ta.crossunder"
  | "ta.cross"
  | "ta.sma"
  | "ta.ema"
  | "ta.rma"
  | "ta.wma"
  | "ta.hma"
  | "ta.rsi"
  | "ta.macd"
  | "ta.supertrend"
  | "ta.atr"
  | "ta.barssince"
  | "ta.valuewhen";

export type VisualExpression =
  | { kind: "literal"; value: number | boolean | string; valueType?: VisualExpressionValueType }
  | { kind: "source"; source: string; valueType?: VisualExpressionValueType }
  | { kind: "reference"; name: string; valueType?: VisualExpressionValueType }
  | { kind: "field"; target: VisualExpression; field: string; valueType?: VisualExpressionValueType }
  | { kind: "history"; target: VisualExpression; offset: number; valueType?: VisualExpressionValueType }
  | { kind: "unary"; operator: "-" | "not"; argument: VisualExpression; valueType?: VisualExpressionValueType }
  | { kind: "binary"; operator: "+" | "-" | "*" | "/" | ">" | "<" | ">=" | "<=" | "==" | "!=" | "and" | "or"; left: VisualExpression; right: VisualExpression; valueType?: VisualExpressionValueType }
  | { kind: "call"; functionName: VisualExpressionCallFunction; args: VisualExpression[]; valueType?: VisualExpressionValueType };

export interface VisualExpressionReference {
  value: string;
  label: string;
  sourceBlockKind: string;
  valueType?: VisualExpressionValueType;
  fields?: Array<{ value: string; label: string; valueType?: VisualExpressionValueType }>;
}

export interface VisualExpressionScope {
  references: VisualExpressionReference[];
}

export interface VisualExpressionSchema {
  expressionIds: string[];
  allowedFunctions: VisualExpressionCallFunction[];
  allowedOperators: Array<Extract<VisualExpression, { kind: "binary" }>["operator"]>;
}

const SERIES_SOURCES = new Set(["open", "high", "low", "close", "volume", "hl2", "hlc3", "ohlc4"]);
const ALLOWED_CALLS = new Set<VisualExpressionCallFunction>([
  "math.min",
  "math.max",
  "math.abs",
  "math.round",
  "math.floor",
  "math.ceil",
  "nz",
  "ta.crossover",
  "ta.crossunder",
  "ta.cross",
  "ta.sma",
  "ta.ema",
  "ta.rma",
  "ta.wma",
  "ta.hma",
  "ta.rsi",
  "ta.macd",
  "ta.supertrend",
  "ta.atr",
  "ta.barssince",
  "ta.valuewhen",
]);

export const VISUAL_EXPRESSION_CALL_OPTIONS: Array<{ value: VisualExpressionCallFunction; label: string }> = [
  { value: "math.min", label: "math.min" },
  { value: "math.max", label: "math.max" },
  { value: "math.abs", label: "math.abs" },
  { value: "math.round", label: "math.round" },
  { value: "math.floor", label: "math.floor" },
  { value: "math.ceil", label: "math.ceil" },
  { value: "nz", label: "nz" },
  { value: "ta.crossover", label: "ta.crossover" },
  { value: "ta.crossunder", label: "ta.crossunder" },
  { value: "ta.cross", label: "ta.cross" },
  { value: "ta.barssince", label: "ta.barssince" },
  { value: "ta.valuewhen", label: "ta.valuewhen" },
];

export function sourceExpression(source: string): VisualExpression {
  return { kind: "source", source: normalizeSource(source) };
}

export function literalExpression(value: number | boolean | string): VisualExpression {
  return { kind: "literal", value };
}

export function referenceExpression(name: string): VisualExpression {
  return { kind: "reference", name: normalizeIdentifier(name, "signal") };
}

export function binaryExpression(
  left: VisualExpression,
  operator: Extract<VisualExpression, { kind: "binary" }>["operator"],
  right: VisualExpression,
): VisualExpression {
  return { kind: "binary", left, operator, right };
}

export function normalizeVisualExpression(
  value: unknown,
  fallback: VisualExpression = sourceExpression("close"),
): VisualExpression {
  if (!isRecord(value)) {
    return fallback;
  }
  switch (value.kind) {
    case "literal": {
      const rawValue = value.value;
      if (typeof rawValue === "number" || typeof rawValue === "boolean" || typeof rawValue === "string") {
        return { kind: "literal", value: rawValue };
      }
      return fallback;
    }
    case "source":
      return { kind: "source", source: normalizeSource(value.source) };
    case "reference":
      return { kind: "reference", name: normalizeIdentifier(value.name, "signal") };
    case "field":
      return {
        kind: "field",
        target: normalizeVisualExpression(value.target, fallback),
        field: normalizeIdentifier(value.field, "value"),
      };
    case "history":
      return {
        kind: "history",
        target: normalizeVisualExpression(value.target, fallback),
        offset: normalizeNonNegativeInteger(value.offset, 1),
      };
    case "unary":
      return {
        kind: "unary",
        operator: value.operator === "not" ? "not" : "-",
        argument: normalizeVisualExpression(value.argument, fallback),
      };
    case "binary":
      return {
        kind: "binary",
        operator: normalizeBinaryOperator(value.operator),
        left: normalizeVisualExpression(value.left, fallback),
        right: normalizeVisualExpression(value.right, literalExpression(0)),
      };
    case "call": {
      const functionName = normalizeCallFunction(value.functionName);
      return {
        kind: "call",
        functionName,
        args: Array.isArray(value.args)
          ? value.args.slice(0, 4).map((arg) => normalizeVisualExpression(arg, sourceExpression("close")))
          : [],
      };
    }
    default:
      return fallback;
  }
}

export function renderVisualExpressionToPine(
  expression: VisualExpression | unknown,
  fallback = "close",
): string {
  const normalized = normalizeVisualExpression(expression, parsePineExpressionToVisualExpression(fallback) ?? sourceExpression("close"));
  switch (normalized.kind) {
    case "literal":
      return formatLiteral(normalized.value);
    case "source":
      return normalizeSource(normalized.source);
    case "reference":
      return normalizeIdentifier(normalized.name, "signal");
    case "field":
      return `${renderVisualExpressionToPine(normalized.target, fallback)}.${normalizeIdentifier(normalized.field, "value")}`;
    case "history":
      return `${renderVisualExpressionToPine(normalized.target, fallback)}[${normalized.offset}]`;
    case "unary":
      return normalized.operator === "not"
        ? `not ${renderVisualExpressionToPine(normalized.argument, fallback)}`
        : `-${renderVisualExpressionToPine(normalized.argument, fallback)}`;
    case "binary":
      return `${wrapBinaryOperand(normalized.left)} ${normalized.operator} ${wrapBinaryOperand(normalized.right)}`;
    case "call":
      return `${normalized.functionName}(${normalized.args.map((arg) => renderVisualExpressionToPine(arg, fallback)).join(", ")})`;
  }
}

export function parsePineExpressionToVisualExpression(expression: string): VisualExpression | null {
  const trimmed = stripWrappingParens(expression.trim());
  if (trimmed === "") {
    return null;
  }

  const binary = parseBinaryExpression(trimmed);
  if (binary !== null) {
    return binary;
  }

  if (trimmed.startsWith("not ")) {
    const argument = parsePineExpressionToVisualExpression(trimmed.slice(4));
    return argument === null ? null : { kind: "unary", operator: "not", argument };
  }
  if (/^-\s*[A-Za-z_(0-9]/.test(trimmed)) {
    const argument = parsePineExpressionToVisualExpression(trimmed.slice(1));
    return argument === null ? null : { kind: "unary", operator: "-", argument };
  }

  const call = parseCallExpression(trimmed);
  if (call !== null) {
    return call;
  }

  const history = parseHistoryExpression(trimmed);
  if (history !== null) {
    return history;
  }

  const field = parseFieldExpression(trimmed);
  if (field !== null) {
    return field;
  }

  if (SERIES_SOURCES.has(trimmed.toLowerCase())) {
    return sourceExpression(trimmed.toLowerCase());
  }
  if (trimmed === "true" || trimmed === "false") {
    return literalExpression(trimmed === "true");
  }
  const numberValue = Number(trimmed);
  if (Number.isFinite(numberValue)) {
    return literalExpression(numberValue);
  }
  const stringValue = readStringLiteral(trimmed);
  if (stringValue !== null) {
    return literalExpression(stringValue);
  }
  if (/^[A-Za-z_][A-Za-z0-9_]*$/.test(trimmed)) {
    return referenceExpression(trimmed);
  }
  return null;
}

export function expressionToLegacyString(
  expression: VisualExpression | unknown,
  fallback: string,
): string {
  return renderVisualExpressionToPine(expression, fallback);
}

function parseBinaryExpression(expression: string): VisualExpression | null {
  const operatorGroups: Array<Array<Extract<VisualExpression, { kind: "binary" }>["operator"]>> = [
    ["or"],
    ["and"],
    [">=", "<=", "==", "!=", ">", "<"],
    ["+", "-"],
    ["*", "/"],
  ];
  for (const operators of operatorGroups) {
    const match = findTopLevelOperator(expression, operators);
    if (match === null) {
      continue;
    }
    const left = parsePineExpressionToVisualExpression(expression.slice(0, match.index));
    const right = parsePineExpressionToVisualExpression(expression.slice(match.index + match.operator.length));
    if (left !== null && right !== null) {
      return { kind: "binary", left, operator: match.operator, right };
    }
  }
  return null;
}

function parseCallExpression(expression: string): VisualExpression | null {
  const call = expression.match(/^([A-Za-z_][A-Za-z0-9_.]*)\((.*)\)$/);
  if (call === null) {
    return null;
  }
  const rawFunctionName = normalizeCallFunctionName((call[1] ?? "").trim().toLowerCase());
  if (!ALLOWED_CALLS.has(rawFunctionName as VisualExpressionCallFunction)) {
    return null;
  }
  const functionName = rawFunctionName as VisualExpressionCallFunction;
  const args = splitArguments(call[2] ?? "")
    .map((arg) => parsePineExpressionToVisualExpression(arg))
    .filter((arg): arg is VisualExpression => arg !== null);
  return { kind: "call", functionName, args };
}

function parseHistoryExpression(expression: string): VisualExpression | null {
  const match = expression.match(/^(.+)\[(\d+)\]$/);
  if (match === null) {
    return null;
  }
  const target = parsePineExpressionToVisualExpression(match[1] ?? "");
  return target === null
    ? null
    : { kind: "history", target, offset: normalizeNonNegativeInteger(match[2], 1) };
}

function parseFieldExpression(expression: string): VisualExpression | null {
  const match = expression.match(/^(.+)\.([A-Za-z_][A-Za-z0-9_]*)$/);
  if (match === null) {
    return null;
  }
  const target = parsePineExpressionToVisualExpression(match[1] ?? "");
  return target === null
    ? null
    : { kind: "field", target, field: match[2] ?? "value" };
}

function findTopLevelOperator(
  expression: string,
  operators: Array<Extract<VisualExpression, { kind: "binary" }>["operator"]>,
): { index: number; operator: Extract<VisualExpression, { kind: "binary" }>["operator"] } | null {
  let depth = 0;
  let inString: "\"" | "'" | null = null;
  for (let index = expression.length - 1; index >= 0; index -= 1) {
    const char = expression[index];
    if (inString !== null) {
      if (char === inString && expression[index - 1] !== "\\") {
        inString = null;
      }
      continue;
    }
    if (char === "\"" || char === "'") {
      inString = char;
      continue;
    }
    if (char === ")" || char === "]") {
      depth += 1;
      continue;
    }
    if (char === "(" || char === "[") {
      depth -= 1;
      continue;
    }
    if (depth !== 0) {
      continue;
    }
    for (const operator of operators) {
      const start = index - operator.length + 1;
      if (start < 0 || expression.slice(start, index + 1) !== operator) {
        continue;
      }
      if ((operator === "and" || operator === "or") && !hasWordBoundaries(expression, start, operator.length)) {
        continue;
      }
      return { index: start, operator };
    }
  }
  return null;
}

function wrapBinaryOperand(expression: VisualExpression): string {
  const rendered = renderVisualExpressionToPine(expression);
  return expression.kind === "binary" ? `(${rendered})` : rendered;
}

function normalizeBinaryOperator(value: unknown): Extract<VisualExpression, { kind: "binary" }>["operator"] {
  return value === "+"
    || value === "-"
    || value === "*"
    || value === "/"
    || value === ">"
    || value === "<"
    || value === ">="
    || value === "<="
    || value === "=="
    || value === "!="
    || value === "and"
    || value === "or"
    ? value
    : ">";
}

function normalizeCallFunction(value: unknown): VisualExpressionCallFunction {
  const rawValue = typeof value === "string" ? value.trim().toLowerCase() : "";
  const functionName = normalizeCallFunctionName(rawValue);
  if (ALLOWED_CALLS.has(functionName as VisualExpressionCallFunction)) {
    return functionName as VisualExpressionCallFunction;
  }
  return "math.max";
}

function normalizeCallFunctionName(value: string): string {
  switch (value) {
    case "barssince":
      return "ta.barssince";
    case "valuewhen":
      return "ta.valuewhen";
    default:
      return value;
  }
}

function normalizeSource(value: unknown): string {
  const rawValue = typeof value === "string" ? value.trim().toLowerCase() : "";
  return SERIES_SOURCES.has(rawValue) ? rawValue : "close";
}

function normalizeIdentifier(value: unknown, fallback: string): string {
  const rawValue = typeof value === "string" ? value.trim() : "";
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(rawValue) ? rawValue : fallback;
}

function normalizeNonNegativeInteger(value: unknown, fallback: number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.max(0, Math.round(parsed)) : fallback;
}

function formatLiteral(value: string | number | boolean): string {
  if (typeof value === "number") {
    return Number.isInteger(value) ? String(value) : String(Number(value.toFixed(8)));
  }
  if (typeof value === "boolean") {
    return value ? "true" : "false";
  }
  return `"${value.replace(/\\/g, "\\\\").replace(/"/g, "\\\"")}"`;
}

function splitArguments(value: string): string[] {
  const args: string[] = [];
  let current = "";
  let depth = 0;
  let inString: "\"" | "'" | null = null;
  for (let index = 0; index < value.length; index += 1) {
    const char = value[index];
    if (inString !== null) {
      current += char;
      if (char === inString && value[index - 1] !== "\\") {
        inString = null;
      }
      continue;
    }
    if (char === "\"" || char === "'") {
      current += char;
      inString = char;
      continue;
    }
    if (char === "(" || char === "[") {
      depth += 1;
    } else if (char === ")" || char === "]") {
      depth -= 1;
    }
    if (char === "," && depth === 0) {
      args.push(current.trim());
      current = "";
      continue;
    }
    current += char;
  }
  if (current.trim() !== "") {
    args.push(current.trim());
  }
  return args;
}

function stripWrappingParens(value: string): string {
  let result = value.trim();
  while (result.startsWith("(") && result.endsWith(")") && wrapsWholeExpression(result)) {
    result = result.slice(1, -1).trim();
  }
  return result;
}

function wrapsWholeExpression(value: string): boolean {
  let depth = 0;
  let inString: "\"" | "'" | null = null;
  for (let index = 0; index < value.length; index += 1) {
    const char = value[index];
    if (inString !== null) {
      if (char === inString && value[index - 1] !== "\\") {
        inString = null;
      }
      continue;
    }
    if (char === "\"" || char === "'") {
      inString = char;
      continue;
    }
    if (char === "(") {
      depth += 1;
    } else if (char === ")") {
      depth -= 1;
      if (depth === 0 && index < value.length - 1) {
        return false;
      }
    }
  }
  return depth === 0;
}

function hasWordBoundaries(value: string, start: number, length: number): boolean {
  const before = value[start - 1] ?? " ";
  const after = value[start + length] ?? " ";
  return !/[A-Za-z0-9_]/.test(before) && !/[A-Za-z0-9_]/.test(after);
}

function readStringLiteral(value: string): string | null {
  const trimmed = value.trim();
  if ((trimmed.startsWith("\"") && trimmed.endsWith("\"")) || (trimmed.startsWith("'") && trimmed.endsWith("'"))) {
    return trimmed.slice(1, -1);
  }
  return null;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
