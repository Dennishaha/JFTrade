import { splitTickerModifier } from "pinets";

import { chartTicker, ExtendedTickerProvider } from "./extendedTickerProvider";

type StaticPineNamespaceCall = {
  argumentsText: string;
  start: number;
};

type StaticPineFunctionCall = {
  name: string;
  args: string[];
};

type PineLexState = "code" | "line_comment" | "block_comment" | "single_quote" | "double_quote";

// PineTS constructs secondary runtimes asynchronously. A rejected provider
// request at that point is not surfaced through run(), so reject unsupported
// static routes before constructing the primary runtime.
export function preflightStaticRequestSecurityRoutes(
  source: string,
  sourceTimeframe: string,
  mainTicker: string,
  provider: ExtendedTickerProvider,
): void {
  const timeframeAliases = staticInputTimeframeAliases(source);
  for (const call of staticRequestSecurityCalls(source)) {
    const args = splitStaticPineArguments(call.argumentsText);
    if (args.length < 3) {
      throw requestSecurityPreflightError("request.security() requires symbol, timeframe, and expression arguments");
    }
    const tickerId = resolveStaticSecurityTicker(args[0]!, mainTicker, (candidate) => {
      try {
        provider.assertCanServeTicker(candidate);
      } catch (error) {
        throw requestSecurityPreflightError(errorMessage(error));
      }
    });
    const timeframe = resolveStaticSecurityTimeframe(args[1]!, sourceTimeframe, timeframeAliases);
    try {
      provider.assertCanServe(tickerId, timeframe);
    } catch (error) {
      throw requestSecurityPreflightError(errorMessage(error));
    }
  }
}

function staticRequestSecurityCalls(source: string): StaticPineNamespaceCall[] {
  return staticPineNamespaceCalls(source, "request.security");
}

function staticInputTimeframeAliases(source: string): ReadonlyMap<string, string> {
  const aliases = new Map<string, string>();
  for (const call of staticPineNamespaceCalls(source, "input.timeframe")) {
    const alias = staticInputTimeframeAlias(source, call.start);
    if (alias === undefined) {
      continue;
    }
    const timeframe = staticInputTimeframeDefault(splitStaticPineArguments(call.argumentsText));
    if (timeframe === undefined) {
      aliases.delete(alias);
      continue;
    }
    aliases.set(alias, timeframe);
  }
  return aliases;
}

function staticPineNamespaceCalls(source: string, name: string): StaticPineNamespaceCall[] {
  const calls: StaticPineNamespaceCall[] = [];
  let state: PineLexState = "code";
  for (let index = 0; index < source.length;) {
    const char = source[index]!;
    const next = source[index + 1];
    if (state === "code") {
      if (char === "/" && next === "/") {
        state = "line_comment";
        index += 2;
        continue;
      }
      if (char === "/" && next === "*") {
        state = "block_comment";
        index += 2;
        continue;
      }
      if (char === "'") {
        state = "single_quote";
        index++;
        continue;
      }
      if (char === "\"") {
        state = "double_quote";
        index++;
        continue;
      }
      if (source.startsWith(name, index) && isPineIdentifierBoundary(source, index, name.length)) {
        let open = index + name.length;
        while (isPineWhitespace(source[open])) {
          open++;
        }
        if (source[open] === "(") {
          const close = findStaticPineCallClose(source, open);
          if (close < 0) {
            throw requestSecurityPreflightError("could not parse request.security() call");
          }
          calls.push({ argumentsText: source.slice(open + 1, close), start: index });
        }
        // Continue after the identifier rather than after the complete call so
        // nested request.security() calls are preflighted too.
        index += name.length;
        continue;
      }
      index++;
      continue;
    }
    if (state === "line_comment") {
      if (char === "\n") {
        state = "code";
      }
      index++;
      continue;
    }
    if (state === "block_comment") {
      if (char === "*" && next === "/") {
        state = "code";
        index += 2;
        continue;
      }
      index++;
      continue;
    }
    if (char === "\\") {
      index += 2;
      continue;
    }
    if ((state === "single_quote" && char === "'") || (state === "double_quote" && char === "\"")) {
      state = "code";
    }
    index++;
  }
  return calls;
}

function staticInputTimeframeAlias(source: string, inputStart: number): string | undefined {
  const prefix = source.slice(0, inputStart);
  const match = /(?:^|\n)\s*(?:(?:var|varip)\s+)?(?:(?:bool|int|float|string|color)\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*(?::=|=)\s*$/i.exec(prefix);
  return match?.[1];
}

function staticInputTimeframeDefault(args: string[]): string | undefined {
  let defaultExpression = args[0];
  for (const arg of args) {
    const named = /^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*([\s\S]+)$/.exec(arg);
    if (named !== null && named[1]!.toLowerCase() === "defval") {
      defaultExpression = named[2]!;
      break;
    }
  }
  return defaultExpression === undefined ? undefined : staticPineString(stripStaticPineOuterParens(defaultExpression));
}

function splitStaticPineArguments(argumentsText: string): string[] {
  const args: string[] = [];
  let current = "";
  let state: PineLexState = "code";
  let parenDepth = 0;
  let bracketDepth = 0;
  let braceDepth = 0;
  for (let index = 0; index < argumentsText.length;) {
    const char = argumentsText[index]!;
    const next = argumentsText[index + 1];
    if (state === "code") {
      if (char === "/" && next === "/") {
        current += " ";
        state = "line_comment";
        index += 2;
        continue;
      }
      if (char === "/" && next === "*") {
        current += " ";
        state = "block_comment";
        index += 2;
        continue;
      }
      if (char === "'") {
        current += char;
        state = "single_quote";
        index++;
        continue;
      }
      if (char === "\"") {
        current += char;
        state = "double_quote";
        index++;
        continue;
      }
      if (char === "(") parenDepth++;
      if (char === ")") parenDepth--;
      if (char === "[") bracketDepth++;
      if (char === "]") bracketDepth--;
      if (char === "{") braceDepth++;
      if (char === "}") braceDepth--;
      if (char === "," && parenDepth === 0 && bracketDepth === 0 && braceDepth === 0) {
        args.push(current.trim());
        current = "";
        index++;
        continue;
      }
      current += char;
      index++;
      continue;
    }
    if (state === "line_comment") {
      if (char === "\n") {
        current += char;
        state = "code";
      }
      index++;
      continue;
    }
    if (state === "block_comment") {
      if (char === "*" && next === "/") {
        state = "code";
        index += 2;
        continue;
      }
      index++;
      continue;
    }
    current += char;
    if (char === "\\" && next !== undefined) {
      current += next;
      index += 2;
      continue;
    }
    if ((state === "single_quote" && char === "'") || (state === "double_quote" && char === "\"")) {
      state = "code";
    }
    index++;
  }
  args.push(current.trim());
  return args;
}

function resolveStaticSecurityTicker(
  expression: string,
  mainTicker: string,
  validateTicker?: (tickerId: string) => void,
): string {
  const value = stripStaticPineOuterParens(expression);
  const stringValue = staticPineString(value);
  if (stringValue !== undefined) {
    return validatedStaticTicker(stringValue === "" ? mainTicker : stringValue, validateTicker);
  }
  if (value.toLowerCase() === "syminfo.tickerid") {
    return validatedStaticTicker(mainTicker, validateTicker);
  }
  const call = staticPineFunctionCall(value);
  if (call === undefined) {
    throw requestSecurityPreflightError(
      `requires a static current-symbol ticker expression; received ${JSON.stringify(value)}`,
    );
  }
  switch (call.name) {
    case "ticker.heikinashi":
      if (call.args.length !== 1) {
        throw requestSecurityPreflightError("ticker.heikinashi() requires one static symbol argument");
      }
      return validatedStaticTicker(
        chartTicker(resolveStaticSecurityTicker(call.args[0]!, mainTicker, validateTicker), "heikinashi"),
        validateTicker,
      );
    case "ticker.standard":
      if (call.args.length > 1) {
        throw requestSecurityPreflightError("ticker.standard() accepts at most one static symbol argument");
      }
      return validatedStaticTicker(chartTicker(
        call.args.length === 0 ? mainTicker : resolveStaticSecurityTicker(call.args[0]!, mainTicker, validateTicker),
        "standard",
      ), validateTicker);
    case "ticker.inherit": {
      if (call.args.length !== 2) {
        throw requestSecurityPreflightError("ticker.inherit() requires static source and symbol arguments");
      }
      const fromTicker = resolveStaticSecurityTicker(call.args[0]!, mainTicker, validateTicker);
      const symbol = resolveStaticSecurityTicker(call.args[1]!, mainTicker, validateTicker);
      return validatedStaticTicker(
        chartTicker(symbol, splitTickerModifier(fromTicker).modifier === "heikinashi" ? "heikinashi" : "standard"),
        validateTicker,
      );
    }
    default:
      throw requestSecurityPreflightError(
        `requires a supported static ticker expression; received ${JSON.stringify(value)}`,
      );
  }
}

function validatedStaticTicker(tickerId: string, validateTicker?: (tickerId: string) => void): string {
  validateTicker?.(tickerId);
  return tickerId;
}

function resolveStaticSecurityTimeframe(
  expression: string,
  sourceTimeframe: string,
  aliases: ReadonlyMap<string, string>,
): string {
  const value = stripStaticPineOuterParens(expression);
  const literal = staticPineString(value);
  if (literal !== undefined) {
    return literal === "" ? sourceTimeframe : literal;
  }
  if (aliases.has(value)) {
    const alias = aliases.get(value)!;
    return alias === "" ? sourceTimeframe : alias;
  }
  const inputCall = staticPineFunctionCall(value);
  if (inputCall?.name === "input.timeframe") {
    const defaultValue = staticInputTimeframeDefault(inputCall.args);
    if (defaultValue !== undefined) {
      return defaultValue === "" ? sourceTimeframe : defaultValue;
    }
  }
  {
    throw requestSecurityPreflightError(
      `requires a static timeframe string; received ${JSON.stringify(expression.trim())}`,
    );
  }
}

function staticPineFunctionCall(expression: string): StaticPineFunctionCall | undefined {
  const match = /^([A-Za-z_][A-Za-z0-9_.]*)\s*\(/.exec(expression);
  if (match === null) {
    return undefined;
  }
  const open = expression.indexOf("(", match[1]!.length);
  const close = findStaticPineCallClose(expression, open);
  if (close < 0 || expression.slice(close + 1).trim() !== "") {
    return undefined;
  }
  const argumentsText = expression.slice(open + 1, close);
  return {
    name: match[1]!.toLowerCase(),
    args: argumentsText.trim() === "" ? [] : splitStaticPineArguments(argumentsText),
  };
}

function stripStaticPineOuterParens(expression: string): string {
  let value = expression.trim();
  while (value.startsWith("(")) {
    const close = findStaticPineCallClose(value, 0);
    if (close !== value.length - 1) {
      break;
    }
    value = value.slice(1, -1).trim();
  }
  return value;
}

function staticPineString(expression: string): string | undefined {
  if (expression.length < 2) {
    return undefined;
  }
  const quote = expression[0]!;
  if ((quote !== "\"" && quote !== "'") || expression.at(-1) !== quote) {
    return undefined;
  }
  let value = "";
  for (let index = 1; index < expression.length - 1; index++) {
    const char = expression[index]!;
    if (char !== "\\" || index === expression.length - 2) {
      value += char;
      continue;
    }
    index++;
    const escaped = expression[index]!;
    switch (escaped) {
      case "n": value += "\n"; break;
      case "r": value += "\r"; break;
      case "t": value += "\t"; break;
      default: value += escaped;
    }
  }
  return value;
}

function findStaticPineCallClose(source: string, open: number): number {
  let depth = 0;
  let state: PineLexState = "code";
  for (let index = open; index < source.length;) {
    const char = source[index]!;
    const next = source[index + 1];
    if (state === "code") {
      if (char === "/" && next === "/") {
        state = "line_comment";
        index += 2;
        continue;
      }
      if (char === "/" && next === "*") {
        state = "block_comment";
        index += 2;
        continue;
      }
      if (char === "'") {
        state = "single_quote";
        index++;
        continue;
      }
      if (char === "\"") {
        state = "double_quote";
        index++;
        continue;
      }
      if (char === "(") depth++;
      if (char === ")") {
        depth--;
        if (depth === 0) {
          return index;
        }
      }
      index++;
      continue;
    }
    if (state === "line_comment") {
      if (char === "\n") state = "code";
      index++;
      continue;
    }
    if (state === "block_comment") {
      if (char === "*" && next === "/") {
        state = "code";
        index += 2;
        continue;
      }
      index++;
      continue;
    }
    if (char === "\\") {
      index += 2;
      continue;
    }
    if ((state === "single_quote" && char === "'") || (state === "double_quote" && char === "\"")) {
      state = "code";
    }
    index++;
  }
  return -1;
}

function isPineIdentifierBoundary(source: string, start: number, length: number): boolean {
  return !isPineIdentifierChar(source[start - 1]) && !isPineIdentifierChar(source[start + length]);
}

function isPineIdentifierChar(value: string | undefined): boolean {
  return value !== undefined && /[A-Za-z0-9_]/.test(value);
}

function isPineWhitespace(value: string | undefined): boolean {
  return value !== undefined && /\s/.test(value);
}

function requestSecurityPreflightError(message: string): Error {
  return new Error(`Pineworker request.security preflight: ${message}`);
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

