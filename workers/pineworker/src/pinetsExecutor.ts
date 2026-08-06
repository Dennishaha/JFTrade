import { PineTS } from "pinets";
import { splitTickerModifier } from "pinets";
import { chartTicker, ExtendedTickerProvider } from "./extendedTickerProvider";
import {
  normalizeChartType,
  type PineTSExecutor,
  type PineTSPlot,
  type PineTSRunResult,
  type PreparedRunScriptRequest,
} from "./types";

type PineTSModule = {
  PineTS: new (source: any, symbol?: string, timeframe?: string, periods?: number) => PineTSRuntime;
};

type PineTSExecutionContext = {
  idx?: number;
  length?: number;
  dataVersion?: number;
  cache?: Record<string, unknown>;
  data?: {
    openTime?: { data?: unknown[] };
  };
  params?: Record<string, unknown>;
  strategy?: {
    pending_orders?: unknown[];
    opentrades?: unknown[];
    position_size?: unknown;
  };
};

type PineTSIteration = (context: PineTSExecutionContext) => unknown | Promise<unknown>;

type PineTSRuntime = {
  setAlertMode?: (mode: "all" | "realtime") => void;
  run(source: string, periods?: number): Promise<PineTSRunResult>;
  _executeIterations?: (
    context: PineTSExecutionContext,
    transpiledFn: PineTSIteration,
    startIdx: number,
    endIdx: number,
  ) => Promise<void>;
  updateTail?: (context: PineTSExecutionContext) => Promise<boolean>;
};

type LivePineTSRuntime = PineTSRuntime & {
  updateTail(context: PineTSExecutionContext): Promise<boolean>;
};

type PendingOrderRecord = Record<string, unknown>;

type TrackedPendingOrder = {
  ref: PendingOrderRecord;
  identity: string;
  semantics: unknown[];
};

type OrderIntentCapture = {
  supported: boolean;
  intents: Record<string, unknown>[];
  previous: TrackedPendingOrder[];
};

type NativeLiveSession = {
  runtime: LivePineTSRuntime;
  context: PineTSExecutionContext & PineTSRunResult;
  capture: OrderIntentCapture;
  request: PreparedRunScriptRequest;
  provider: ExtendedTickerProvider;
  revision: number;
  queue: Promise<void>;
  failed: boolean;
};

type ResolvedPendingOrder = {
  direction: "long" | "short";
  quantity?: number;
  quantityPct?: number;
};

type ExitTarget = {
  direction: "long" | "short";
  quantity: number;
};

const pendingOrderSemanticFields = [
  "id",
  "category",
  "from_entry",
  "direction",
  "qty",
  "qty_percent",
  "type",
  "limit",
  "stop",
  "profit",
  "loss",
  "trail_price",
  "trail_points",
  "trail_offset",
  "oca_name",
  "oca_type",
  "comment",
  "alert_message",
  "disable_alert",
  "immediately",
] as const;

const secondaryRefreshPatched = Symbol("secondaryRefreshPatched");

export class NativePineTSExecutor implements PineTSExecutor {
  private readonly liveSessions = new Map<string, NativeLiveSession>();

  constructor(private readonly module: PineTSModule, private readonly pineTSVersion = "unknown") {}

  version(): string {
    return this.pineTSVersion;
  }

  async run(request: PreparedRunScriptRequest): Promise<PineTSRunResult> {
    const execution = await this.createExecution(request);
    return compactPineTSResult(execution.context, request.includePlots !== false);
  }

  async openLiveSession(sessionId: string, request: PreparedRunScriptRequest): Promise<PineTSRunResult> {
    if (this.liveSessions.has(sessionId)) {
      throw new Error(`PineTS live session ${JSON.stringify(sessionId)} already exists`);
    }
    if ((request.expectedRevision ?? 0) !== 0) {
      throw new Error("PineTS live session open requires expected revision 0");
    }
    const execution = await this.createExecution(request);
    if (!isLivePineTSRuntime(execution.runtime)) {
      throw new Error("PineTS runtime does not expose the update-tail hook required for stateful live sessions");
    }
    this.liveSessions.set(sessionId, {
      ...execution,
      runtime: execution.runtime,
      revision: 1,
      queue: Promise.resolve(),
      failed: false,
    });
    // Warmup establishes state only. Historical intents must never escape as
    // live orders when a session is opened.
    const result = compactPineTSResult(execution.context, request.includePlots !== false);
    result.orderIntents = [];
    return result;
  }

  async appendLiveSession(
    sessionId: string,
    expectedRevision: number,
    request: PreparedRunScriptRequest,
  ): Promise<{ result: PineTSRunResult; revision: number }> {
    const session = this.liveSessions.get(sessionId);
    if (session === undefined || session.failed) {
      throw new Error(`PineTS live session ${JSON.stringify(sessionId)} is not available`);
    }
    return this.withLiveSessionLock(session, async () => {
      if (this.liveSessions.get(sessionId) !== session || session.failed) {
        throw new Error(`PineTS live session ${JSON.stringify(sessionId)} is not available`);
      }
      if (session.revision !== expectedRevision) {
        throw new Error(
          `PineTS live session ${JSON.stringify(sessionId)} revision mismatch: expected ${expectedRevision}, current ${session.revision}`,
        );
      }
      assertSameLiveSessionDefinition(session.request, request);
      const lastOpenTime = session.request.candles[session.request.candles.length - 1]?.openTime ?? 0;
      let previousOpenTime = lastOpenTime;
      for (const candle of request.candles) {
        if (candle.openTime <= previousOpenTime) {
          throw new Error(
            `PineTS live session ${JSON.stringify(sessionId)} requires strictly increasing closed candle open times`,
          );
        }
        previousOpenTime = candle.openTime;
      }

      const marker = resultMarker(session.context, session.capture);
      try {
        for (const candle of request.candles) {
          session.provider.append(candle);
          session.request.candles.push({ ...candle });
          const updated = await session.runtime.updateTail(session.context);
          if (!updated) {
            throw new Error("Pineworker runtime did not update its tail after a closed candle append");
          }
          stabilizeSecondaryContexts(session.context);
        }
      } catch (error) {
        session.failed = true;
        this.liveSessions.delete(sessionId);
        throw new Error(
          `PineTS live session ${JSON.stringify(sessionId)} was invalidated after an append failure: ${String(error instanceof Error ? error.message : error)}`,
        );
      }
      session.revision++;
      return {
        result: incrementalResult(
          session.context,
          session.capture,
          marker,
          request.includePlots !== false,
          request.candles.length,
        ),
        revision: session.revision,
      };
    });
  }

  async closeLiveSession(sessionId: string, expectedRevision: number): Promise<number> {
    const session = this.liveSessions.get(sessionId);
    if (session === undefined) {
      return expectedRevision;
    }
    return this.withLiveSessionLock(session, async () => {
      if (this.liveSessions.get(sessionId) !== session) {
        return expectedRevision;
      }
      if (expectedRevision !== 0 && session.revision !== expectedRevision) {
        throw new Error(
          `PineTS live session ${JSON.stringify(sessionId)} revision mismatch: expected ${expectedRevision}, current ${session.revision}`,
        );
      }
      this.liveSessions.delete(sessionId);
      return session.revision;
    });
  }

  private async createExecution(request: PreparedRunScriptRequest): Promise<{
    runtime: PineTSRuntime;
    context: PineTSExecutionContext & PineTSRunResult;
    capture: OrderIntentCapture;
    request: PreparedRunScriptRequest;
    provider: ExtendedTickerProvider;
  }> {
    const periods = Math.max(1, request.candles.length);
    const provider = new ExtendedTickerProvider(request.symbol, request.timeframe, request.candles);
    const mainTicker = chartTicker(request.symbol, normalizeChartType(request.chartType));
    provider.assertCanServe(mainTicker, request.timeframe);
    preflightStaticRequestSecurityRoutes(request.source, request.timeframe, mainTicker, provider);
    const pineTS = new this.module.PineTS(
      provider,
      mainTicker,
      request.timeframe,
      periods,
    );
    pineTS.setAlertMode?.("all");
    const orderCapture = installOrderIntentCapture(pineTS, request);
    const result = await pineTS.run(normalizePineSourceForPineTS(request.source), periods);
    initializeContextDataVersion(result as PineTSExecutionContext);
    stabilizeSecondaryContexts(result as PineTSExecutionContext);
    if (result.strategy !== undefined) {
      if (!orderCapture.supported) {
        throw new Error("PineTS runtime does not expose the per-bar execution hook required for safe strategy order capture");
      }
      if (result.orderIntents === undefined) {
        result.orderIntents = orderCapture.intents;
      }
    }
    return {
      runtime: pineTS,
      context: result as PineTSExecutionContext & PineTSRunResult,
      capture: orderCapture,
      request,
      provider,
    };
  }

  private async withLiveSessionLock<T>(session: NativeLiveSession, operation: () => Promise<T>): Promise<T> {
    const previous = session.queue;
    let release!: () => void;
    session.queue = new Promise<void>((resolve) => {
      release = resolve;
    });
    await previous;
    try {
      return await operation();
    } finally {
      release();
    }
  }
}

function isLivePineTSRuntime(runtime: PineTSRuntime): runtime is LivePineTSRuntime {
  return typeof runtime.updateTail === "function";
}

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
function preflightStaticRequestSecurityRoutes(
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

function initializeContextDataVersion(context: PineTSExecutionContext): void {
  context.dataVersion ??= 0;
  for (const cached of Object.values(context.cache ?? {})) {
    if (typeof cached !== "object" || cached === null) {
      continue;
    }
    const entry = cached as { pineTS?: unknown; context?: unknown; dataVersion?: unknown };
    if (entry.pineTS !== undefined && entry.context !== undefined && entry.dataVersion === undefined) {
      entry.dataVersion = context.dataVersion;
    }
  }
}

function stabilizeSecondaryContexts(context: PineTSExecutionContext): void {
  for (const cached of Object.values(context.cache ?? {})) {
    if (typeof cached !== "object" || cached === null) {
      continue;
    }
    const entry = cached as { pineTS?: PineTSRuntime; context?: PineTSExecutionContext };
    const runtime = entry.pineTS;
    const secondaryContext = entry.context;
    if (runtime === undefined || secondaryContext === undefined) {
      continue;
    }
    alignSecondaryParams(secondaryContext);
    const markedRuntime = runtime as PineTSRuntime & { [secondaryRefreshPatched]?: boolean };
    if (markedRuntime[secondaryRefreshPatched] || runtime.updateTail === undefined) {
      continue;
    }
    const updateTail = runtime.updateTail;
    runtime.updateTail = async (updatedContext) => {
      const changed = await updateTail.call(runtime, updatedContext);
      if (changed) {
        alignSecondaryParams(updatedContext);
      }
      return changed;
    };
    markedRuntime[secondaryRefreshPatched] = true;
  }
}

// PineTS restores a tail snapshot when it refreshes a secondary context. Its
// request parameter arrays remain cumulative, while its timestamp arrays are
// tail-aligned. Keep those indices aligned for request.security lookups.
function alignSecondaryParams(context: PineTSExecutionContext): void {
  const dataLength = context.data?.openTime?.data?.length ?? 0;
  if (dataLength === 0) {
    return;
  }
  for (const value of Object.values(context.params ?? {})) {
    if (Array.isArray(value) && value.length > dataLength) {
      value.splice(0, value.length - dataLength);
    }
  }
}

type ResultMarker = {
  intentCount: number;
  plotLengths: Record<string, number>;
  alertCount: number;
  visualCount: number;
  logCount: number;
  warningCount: number;
  diagnosticCount: number;
};

function resultMarker(result: PineTSRunResult, capture: OrderIntentCapture): ResultMarker {
  return {
    intentCount: capture.intents.length,
    plotLengths: Object.fromEntries(Object.entries(result.plots ?? {}).map(([name, plot]) => [name, plotLength(plot)])),
    alertCount: result.alerts?.length ?? 0,
    visualCount: result.visualOutputs?.length ?? 0,
    logCount: result.logs?.length ?? 0,
    warningCount: result.warnings?.length ?? 0,
    diagnosticCount: result.diagnostics?.length ?? 0,
  };
}

function incrementalResult(
  result: PineTSRunResult,
  capture: OrderIntentCapture,
  marker: ResultMarker,
  includePlots: boolean,
  appendedBarCount = 0,
): PineTSRunResult {
  const delta: PineTSRunResult = {
    orderIntents: capture.intents.slice(marker.intentCount),
  };
  if (result.alerts !== undefined) delta.alerts = result.alerts.slice(marker.alertCount);
  if (result.visualOutputs !== undefined) delta.visualOutputs = result.visualOutputs.slice(marker.visualCount);
  if (result.logs !== undefined) delta.logs = result.logs.slice(marker.logCount);
  if (result.warnings !== undefined) delta.warnings = result.warnings.slice(marker.warningCount);
  if (result.diagnostics !== undefined) delta.diagnostics = result.diagnostics.slice(marker.diagnosticCount);
  if (includePlots) {
    delta.plots = Object.fromEntries(Object.entries(result.plots ?? {}).map(([name, plot]) => [
      name,
      slicePlot(plot, Math.max(
        marker.plotLengths[name] ?? 0,
        plotLength(plot) - appendedBarCount,
      )),
    ]));
  }
  return compactPineTSResult(delta, includePlots);
}

function plotLength(plot: PineTSPlot | number[]): number {
  if (Array.isArray(plot)) return plot.length;
  return plot?.data?.length ?? 0;
}

function slicePlot(
  plot: PineTSPlot | number[],
  start: number,
): PineTSPlot | number[] {
  if (Array.isArray(plot)) return plot.slice(start);
  return { ...plot, data: plot.data?.slice(start) ?? [] };
}

function assertSameLiveSessionDefinition(
  opened: PreparedRunScriptRequest,
  appended: PreparedRunScriptRequest,
): void {
  const equalParams = JSON.stringify(sortedEntries(opened.params)) === JSON.stringify(sortedEntries(appended.params));
  if (
    opened.source !== appended.source ||
    opened.scriptId !== appended.scriptId ||
    opened.symbol !== appended.symbol ||
    opened.timeframe !== appended.timeframe ||
    normalizeChartType(opened.chartType) !== normalizeChartType(appended.chartType) ||
    !equalParams
  ) {
    throw new Error("PineTS live session append cannot change script, symbol, timeframe, chart type, or params");
  }
}

function sortedEntries(values: Record<string, string> | undefined): [string, string][] {
  return Object.entries(values ?? {}).sort(([left], [right]) => left.localeCompare(right));
}

export async function createNativePineTSExecutor(version = "unknown"): Promise<NativePineTSExecutor> {
  return new NativePineTSExecutor({ PineTS: PineTS as unknown as PineTSModule["PineTS"] }, version);
}

function compactPineTSResult(result: PineTSRunResult, includePlots: boolean): PineTSRunResult {
  const compact: PineTSRunResult = {};
  if (includePlots && result.plots !== undefined) compact.plots = result.plots;
  if (result.alerts !== undefined) compact.alerts = result.alerts;
  if (result.visualOutputs !== undefined) compact.visualOutputs = result.visualOutputs;
  if (result.drawings !== undefined) compact.drawings = result.drawings;
  if (result.logs !== undefined) compact.logs = result.logs;
  if (result.warnings !== undefined) compact.warnings = result.warnings;
  if (result.diagnostics !== undefined) compact.diagnostics = result.diagnostics;
  if (result.orderIntents !== undefined) compact.orderIntents = result.orderIntents;
  if (result.strategy !== undefined) compact.strategy = compactStrategyResult(result.strategy);
  return compact;
}

function compactStrategyResult(value: unknown): unknown {
  if (typeof value !== "object" || value === null) {
    return value;
  }
  const source = value as Record<string, unknown>;
  return {
    closedtrades: compactTrades(source.closedtrades, true),
    opentrades: compactTrades(source.opentrades, false),
    buy_and_hold_pnl: source.buy_and_hold_pnl ?? source.buyAndHoldPnl,
    buy_and_hold_per_gain: source.buy_and_hold_per_gain ?? source.buyAndHoldPerGain,
    strategy_outperformance: source.strategy_outperformance ?? source.strategyOutperformance,
  };
}

function compactTrades(value: unknown, includeExit: boolean): unknown[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.flatMap((item) => {
    if (typeof item !== "object" || item === null) {
      return [];
    }
    const source = item as Record<string, unknown>;
    const trade: Record<string, unknown> = {
      entry_id: source.entry_id,
      entry_bar_index: source.entry_bar_index,
      size: source.size,
    };
    if (includeExit) {
      trade.exit_id = source.exit_id;
      trade.exit_bar_index = source.exit_bar_index;
    }
    return [trade];
  });
}

function installOrderIntentCapture(pineTS: PineTSRuntime, request: PreparedRunScriptRequest): OrderIntentCapture {
  const capture: OrderIntentCapture = { supported: false, intents: [], previous: [] };
  const executeIterations = pineTS._executeIterations;
  if (typeof executeIterations !== "function") {
    return capture;
  }

  capture.supported = true;
  // PineTS deletes filled orders from its final strategy state. Capture after
  // each script bar, while preserving the runtime's normal execution path.
  pineTS._executeIterations = async (context, transpiledFn, startIdx, endIdx) => {
    const capturingFn: PineTSIteration = async (iterationContext) => {
      const result = await transpiledFn(iterationContext);
      capturePendingOrderEvents(iterationContext, request, capture);
      return result;
    };
    await executeIterations.call(pineTS, context, capturingFn, startIdx, endIdx);
  };
  return capture;
}

function capturePendingOrderEvents(
  context: PineTSExecutionContext,
  request: PreparedRunScriptRequest,
  capture: OrderIntentCapture,
): void {
  if (context.strategy === undefined) {
    return;
  }
  const barIndex = integerOr(context.idx, request.candles.length - 1);
  const current = pendingOrders(context).map((order) => trackPendingOrder(order, context));
  const matchedPrevious = new Set<number>();
  const cancellations = new Map<string, Record<string, unknown>>();
  const placements: Record<string, unknown>[] = [];

  for (const order of current) {
    const exactIndex = capture.previous.findIndex((previous, index) =>
      !matchedPrevious.has(index) && previous.ref === order.ref,
    );
    if (exactIndex >= 0) {
      matchedPrevious.add(exactIndex);
      const previous = capture.previous[exactIndex]!;
      if (!sameOrderSemantics(previous.semantics, order.semantics)) {
        addCancellation(cancellations, previous.ref, request, barIndex);
        placements.push(orderIntentFromPendingOrder(order.ref, context, request, barIndex));
      }
      continue;
    }

    const unchangedIndex = capture.previous.findIndex((previous, index) =>
      !matchedPrevious.has(index) &&
      pendingOrderStatus(previous.ref) === "pending" &&
      previous.identity === order.identity &&
      sameOrderSemantics(previous.semantics, order.semantics),
    );
    if (unchangedIndex >= 0) {
      matchedPrevious.add(unchangedIndex);
      continue;
    }

    const modifiedIndex = capture.previous.findIndex((previous, index) =>
      !matchedPrevious.has(index) &&
      pendingOrderStatus(previous.ref) === "pending" &&
      previous.identity === order.identity,
    );
    if (modifiedIndex >= 0) {
      matchedPrevious.add(modifiedIndex);
      addCancellation(cancellations, capture.previous[modifiedIndex]!.ref, request, barIndex);
    }
    placements.push(orderIntentFromPendingOrder(order.ref, context, request, barIndex));
  }

  for (let index = 0; index < capture.previous.length; index++) {
    if (matchedPrevious.has(index)) {
      continue;
    }
    const previous = capture.previous[index]!;
    if (pendingOrderStatus(previous.ref) !== "filled") {
      addCancellation(cancellations, previous.ref, request, barIndex);
    }
  }

  capture.intents.push(...cancellations.values(), ...placements);
  capture.previous = current;
}

function pendingOrders(context: PineTSExecutionContext): PendingOrderRecord[] {
  const orders = context.strategy?.pending_orders;
  if (!Array.isArray(orders)) {
    return [];
  }
  return orders.filter((order): order is PendingOrderRecord =>
    typeof order === "object" && order !== null && pendingOrderStatus(order as PendingOrderRecord) === "pending",
  );
}

function trackPendingOrder(order: PendingOrderRecord, context: PineTSExecutionContext): TrackedPendingOrder {
  const category = optionalString(order.category) ?? "entry";
  const resolved = resolvePendingOrder(order, category, context);
  return {
    ref: order,
    identity: [
      optionalString(order.category) ?? "entry",
      optionalString(order.id) ?? "",
      optionalString(order.from_entry) ?? "",
    ].join("\u0000"),
    // Deliberately excludes placement bar/time and PineTS bookkeeping so an
    // unchanged strategy.exit refresh does not become a duplicate broker order.
    semantics: [
      ...pendingOrderSemanticFields.map((field) => order[field]),
      resolved.direction,
      resolved.quantity,
      resolved.quantityPct,
    ],
  };
}

function sameOrderSemantics(left: unknown[], right: unknown[]): boolean {
  return left.length === right.length && left.every((value, index) => Object.is(value, right[index]));
}

function pendingOrderStatus(order: PendingOrderRecord): string {
  return optionalString(order.status) ?? "pending";
}

function addCancellation(
  cancellations: Map<string, Record<string, unknown>>,
  order: PendingOrderRecord,
  request: PreparedRunScriptRequest,
  barIndex: number,
): void {
  const id = optionalString(order.id);
  if (id === undefined || cancellations.has(id)) {
    return;
  }
  cancellations.set(id, {
    kind: "cancel",
    id,
    barIndex,
    time: candleOpenTime(request, barIndex),
  });
}

function orderIntentFromPendingOrder(
  order: PendingOrderRecord,
  context: PineTSExecutionContext,
  request: PreparedRunScriptRequest,
  barIndex: number,
): Record<string, unknown> {
  const id = optionalString(order.id) ?? "";
  rejectUnsupportedConditionalExit(order, id);

  const category = optionalString(order.category) ?? "entry";
  validatePendingOrderPriceFields(order, id);
  if (category !== "exit" && !(typeof order.qty === "number" && Number.isFinite(order.qty) && order.qty > 0)) {
    throw new Error(`Pine strategy entry order ${JSON.stringify(id)} has no positive quantity`);
  }
  const resolved = resolvePendingOrder(order, category, context);
  const intent: Record<string, unknown> = {
    kind: category === "exit" ? "exit" : "entry",
    id,
    direction: resolved.direction,
    barIndex,
    time: candleOpenTime(request, barIndex),
  };
  setIntentString(intent, "fromEntry", order.from_entry);
  setPositiveIntentNumber(intent, "quantity", resolved.quantity);
  setPositiveIntentNumber(intent, "quantityPct", resolved.quantityPct);
  setIntentNumber(intent, "limitPrice", order.limit);
  setIntentNumber(intent, "stopPrice", order.stop);
  setIntentString(intent, "comment", order.comment);
  setIntentString(intent, "alertMessage", order.alert_message);
  if (typeof order.disable_alert === "boolean") {
    intent.disableAlert = order.disable_alert;
  }
  annotateAtomicOrderSemantics(intent, order, category, context, request, barIndex);
  return intent;
}

function annotateAtomicOrderSemantics(
  intent: Record<string, unknown>,
  order: PendingOrderRecord,
  category: string,
  context: PineTSExecutionContext,
  request: PreparedRunScriptRequest,
  barIndex: number,
): void {
  const id = optionalString(order.id) ?? "";
  if (category === "exit") {
    intent.reduceOnly = true;
    const fromEntry = optionalString(order.from_entry);
    if (fromEntry !== undefined) {
      intent.parentId = fromEntry;
      if (pendingEntryOrders(context).some((entry) => optionalString(entry.id) === fromEntry)) {
        intent.atomicGroupId = atomicGroupID(request, barIndex, fromEntry);
      }
    }
    if (optionalPositiveNumber(order.limit) !== undefined && optionalPositiveNumber(order.stop) !== undefined) {
      intent.ocoGroupId = ocoGroupID(request, barIndex, id);
      if (intent.atomicGroupId === undefined) {
        intent.atomicGroupId = intent.ocoGroupId;
      }
    }
    return;
  }

  if (id !== "" && pendingOrders(context).some((candidate) =>
    (optionalString(candidate.category) ?? "entry") === "exit" && optionalString(candidate.from_entry) === id
  )) {
    intent.atomicGroupId = atomicGroupID(request, barIndex, id);
  }
}

function atomicGroupID(request: PreparedRunScriptRequest, barIndex: number, entryId: string): string {
  return ["pine", request.symbol, String(barIndex), "parent", entryId].join(":");
}

function ocoGroupID(request: PreparedRunScriptRequest, barIndex: number, exitId: string): string {
  return ["pine", request.symbol, String(barIndex), "oco", exitId].join(":");
}

function rejectUnsupportedConditionalExit(order: PendingOrderRecord, id: string): void {
  if ((optionalString(order.category) ?? "entry") !== "exit") {
    return;
  }
  const unsupported = ["profit", "loss", "trail_price", "trail_points", "trail_offset"]
    .filter((field) => order[field] !== undefined);
  if (unsupported.length > 0) {
    throw new Error(
      `Pine strategy exit ${JSON.stringify(id)} uses unsupported conditional fields: ${unsupported.join(", ")}; ` +
      "the worker cannot safely convert tick-based or trailing exits to broker prices",
    );
  }
}

function validatePendingOrderPriceFields(order: PendingOrderRecord, id: string): void {
  const type = optionalString(order.type) ?? "market";
  const hasPositivePrice = (value: unknown): boolean =>
    typeof value === "number" && Number.isFinite(value) && value > 0;
  if ((type === "limit" || type === "stop-limit") && !hasPositivePrice(order.limit)) {
    throw new Error(`Pine strategy order ${JSON.stringify(id)} has a ${type} type without a valid limit price`);
  }
  if ((type === "stop" || type === "stop-limit") && !hasPositivePrice(order.stop)) {
    throw new Error(`Pine strategy order ${JSON.stringify(id)} has a ${type} type without a valid stop price`);
  }
}

function resolvePendingOrder(
  order: PendingOrderRecord,
  category: string,
  context: PineTSExecutionContext,
): ResolvedPendingOrder {
  if (category === "exit") {
    const target = resolveExitTarget(order, context);
    const rawQuantity = optionalPositiveNumber(order.qty);
    if (order.qty !== undefined && order.qty !== 0 && rawQuantity === undefined) {
      throw new Error(`Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} has invalid quantity`);
    }
    const rawQuantityPct = optionalPositiveNumber(order.qty_percent);
    if (order.qty_percent !== undefined && rawQuantityPct === undefined) {
      throw new Error(
        `Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} has invalid quantity percent`,
      );
    }
    const requestedQuantity = rawQuantity ?? (
      rawQuantityPct === undefined ? target.quantity : target.quantity * Math.min(rawQuantityPct, 100) / 100
    );
    const quantity = Math.min(requestedQuantity, target.quantity);
    if (!(Number.isFinite(quantity) && quantity > 0)) {
      throw new Error(`Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} has no positive target quantity`);
    }
    return { direction: target.direction, quantity };
  }
  const value = order.direction;
  if (value === 1 || value === "long" || value === "buy") {
    return resolvedEntryOrder("long", order);
  }
  if (value === -1 || value === "short" || value === "sell") {
    return resolvedEntryOrder("short", order);
  }
  throw new Error(`Pine strategy entry order has unsupported direction: ${String(value)}`);
}

function resolvedEntryOrder(direction: "long" | "short", order: PendingOrderRecord): ResolvedPendingOrder {
  const resolved: ResolvedPendingOrder = { direction };
  const quantity = optionalPositiveNumber(order.qty);
  const quantityPct = optionalPositiveNumber(order.qty_percent);
  if (quantity !== undefined) resolved.quantity = quantity;
  if (quantityPct !== undefined) resolved.quantityPct = quantityPct;
  return resolved;
}

function resolveExitTarget(order: PendingOrderRecord, context: PineTSExecutionContext): ExitTarget {
  const strategy = context.strategy;
  const openTrades = recordArray(strategy?.opentrades);
  const intendedTradeIDs = stringArray(order._intended_trade_ids);
  if (intendedTradeIDs.length > 0) {
    const intendedIDs = new Set(intendedTradeIDs);
    const intendedTrades = openTrades.filter((trade) => intendedIDs.has(optionalString(trade.id) ?? ""));
    const matchedIDs = new Set(intendedTrades.flatMap((trade) => {
      const id = optionalString(trade.id);
      return id === undefined ? [] : [id];
    }));
    if (matchedIDs.size !== intendedIDs.size) {
      throw new Error(
        `Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} has unresolved intended trades`,
      );
    }
    return exitTargetFromSignedQuantities(intendedTrades.map((trade) => trade.size), order);
  }

  const fromEntry = optionalString(order.from_entry);
  if (fromEntry !== undefined) {
    const matchingPendingEntries = pendingEntryOrders(context)
      .filter((pending) => optionalString(pending.id) === fromEntry);
    if (matchingPendingEntries.length > 0) {
      return exitTargetFromPendingEntries(matchingPendingEntries, order);
    }
    const signedQuantities = openTrades
      .filter((trade) => optionalString(trade.entry_id) === fromEntry)
      .map((trade) => trade.size);
    if (signedQuantities.length > 0) {
      return exitTargetFromSignedQuantities(signedQuantities, order);
    }
    throw new Error(
      `Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} references entry ` +
      `${JSON.stringify(fromEntry)} without a matching open or pending trade`,
    );
  }

  if (pendingEntryOrders(context).length > 0) {
    throwUnsupportedPendingEntryExit(order);
  }
  const positionSize = strategy?.position_size;
  if (directionFromSignedNumber(positionSize) !== undefined) {
    return exitTargetFromSignedQuantities([positionSize], order);
  }
  throw new Error(
    `Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} has no provable long/short position direction`,
  );
}

function exitTargetFromPendingEntries(entries: PendingOrderRecord[], order: PendingOrderRecord): ExitTarget {
  const signedQuantities = entries.map((entry) => {
    const quantity = optionalPositiveNumber(entry.qty);
    if (quantity === undefined) {
      throw new Error(
        `Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} references a pending entry without a positive quantity`,
      );
    }
    const direction = entry.direction;
    if (direction === 1 || direction === "long" || direction === "buy") return quantity;
    if (direction === -1 || direction === "short" || direction === "sell") return -quantity;
    throw new Error(
      `Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} references a pending entry with an unsupported direction`,
    );
  });
  return exitTargetFromSignedQuantities(signedQuantities, order);
}

function exitTargetFromSignedQuantities(values: unknown[], order: PendingOrderRecord): ExitTarget {
  const signedQuantities = values.filter((value): value is number =>
    typeof value === "number" && Number.isFinite(value) && value !== 0,
  );
  if (signedQuantities.length !== values.length) {
    throw new Error(`Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} has invalid target size`);
  }
  const directions = new Set(signedQuantities.map((value) => value > 0 ? "long" : "short"));
  if (directions.size !== 1) {
    throw new Error(
      `Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} has ambiguous target direction`,
    );
  }
  const quantity = signedQuantities.reduce((sum, value) => sum + Math.abs(value), 0);
  if (!(Number.isFinite(quantity) && quantity > 0)) {
    throw new Error(`Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} has no positive target quantity`);
  }
  return { direction: directions.values().next().value!, quantity };
}

function pendingEntryOrders(context: PineTSExecutionContext): PendingOrderRecord[] {
  return pendingOrders(context)
    .filter((pending) => (optionalString(pending.category) ?? "entry") !== "exit");
}

function throwUnsupportedPendingEntryExit(order: PendingOrderRecord): never {
  throw new Error(
    `Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} depends on an unfilled entry; ` +
    "the worker cannot bind this exit to one unique parent entry for atomic placement",
  );
}

function directionFromSignedNumber(value: unknown): "long" | "short" | undefined {
  if (typeof value !== "number" || !Number.isFinite(value) || value === 0) {
    return undefined;
  }
  return value > 0 ? "long" : "short";
}

function optionalPositiveNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : undefined;
}

function recordArray(value: unknown): PendingOrderRecord[] {
  return Array.isArray(value)
    ? value.filter((item): item is PendingOrderRecord => typeof item === "object" && item !== null)
    : [];
}

function stringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string" && item !== "") : [];
}

function setIntentString(intent: Record<string, unknown>, key: string, value: unknown): void {
  const normalized = optionalString(value);
  if (normalized !== undefined) {
    intent[key] = normalized;
  }
}

function setPositiveIntentNumber(intent: Record<string, unknown>, key: string, value: unknown): void {
  if (typeof value === "number" && Number.isFinite(value) && value > 0) {
    intent[key] = value;
  }
}

function setIntentNumber(intent: Record<string, unknown>, key: string, value: unknown): void {
  if (typeof value === "number" && Number.isFinite(value)) {
    intent[key] = value;
  }
}

function optionalString(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined;
}

function integerOr(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isInteger(value) ? value : fallback;
}

function candleOpenTime(request: PreparedRunScriptRequest, barIndex: number): number {
  return request.candles[barIndex]?.openTime ?? 0;
}

export function normalizePineSourceForPineTS(source: string): string {
  let output = "";
  let index = 0;
  let state: "code" | "line_comment" | "block_comment" | "single_quote" | "double_quote" = "code";
  while (index < source.length) {
    const char = source[index];
    const next = source[index + 1];
    if (state === "code") {
      if (char === "/" && next === "/") {
        output += "//";
        index += 2;
        state = "line_comment";
        continue;
      }
      if (char === "/" && next === "*") {
        output += "/*";
        index += 2;
        state = "block_comment";
        continue;
      }
      if (char === "'") {
        output += char;
        index++;
        state = "single_quote";
        continue;
      }
      if (char === "\"") {
        output += char;
        index++;
        state = "double_quote";
        continue;
      }
      if (source.startsWith("timenow", index) && !isIdentifierChar(source[index - 1]) && !isIdentifierChar(source[index + "timenow".length])) {
        output += "time_close";
        index += "timenow".length;
        continue;
      }
      output += char;
      index++;
      continue;
    }
    output += char;
    index++;
    if (state === "line_comment" && char === "\n") {
      state = "code";
    } else if (state === "block_comment" && char === "*" && next === "/") {
      output += next;
      index++;
      state = "code";
    } else if (state === "single_quote" && char === "'" && source[index - 2] !== "\\") {
      state = "code";
    } else if (state === "double_quote" && char === "\"" && source[index - 2] !== "\\") {
      state = "code";
    }
  }
  return output;
}

function isIdentifierChar(value: string | undefined): boolean {
  return value !== undefined && /[A-Za-z0-9_]/.test(value);
}
