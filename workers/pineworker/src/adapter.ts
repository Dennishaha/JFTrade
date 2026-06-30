import { createHash } from "node:crypto";
import type {
  AlertEvent,
  OrderIntent,
  PineTSExecutor,
  PineTSPlot,
  PineTSRunResult,
  PlotOutput,
  PreparedRunScriptRequest,
  RunScriptResponse,
  StrategyMetrics,
  VisualOutput,
  WorkerMetadata,
} from "./types";
import { preparationOf } from "./preparedRequest";
import { validateRunScriptRequest, type WorkerLimits } from "./validation";
import { workerVersion } from "./types";

export type RunAdapterOptions = {
  workerId: string;
  executor: PineTSExecutor;
  limits?: WorkerLimits;
  peakRSSBytes: () => number;
};

export async function runScriptWithPineTS(
  request: PreparedRunScriptRequest,
  options: RunAdapterOptions,
): Promise<RunScriptResponse> {
  const started = performance.now();
  const preparation = preparationOf(request);

  try {
    validateRunScriptRequest(request, options.limits);
    const result = await options.executor.run(request);
    const response = buildResponse(request, result, {
      workerId: options.workerId,
      version: workerVersion,
      pineTSVersion: options.executor.version(),
      scriptHash: hashText(request.source),
      dataHash: preparation.dataHash,
      durationMs: elapsedMs(started),
      requestBytes: preparation.requestBytes,
      responseBytes: 0,
      peakRSSBytes: options.peakRSSBytes(),
    });
    response.metadata.responseBytes = jsonBytes(response);
    return response;
  } catch (error) {
    const response = buildErrorResponse(request, String(error instanceof Error ? error.message : error), {
      workerId: options.workerId,
      version: workerVersion,
      pineTSVersion: options.executor.version(),
      scriptHash: hashText(request.source ?? ""),
      dataHash: preparation.dataHash,
      durationMs: elapsedMs(started),
      requestBytes: preparation.requestBytes,
      responseBytes: 0,
      peakRSSBytes: options.peakRSSBytes(),
    });
    response.metadata.responseBytes = jsonBytes(response);
    return response;
  }
}

export function buildResponse(
  request: PreparedRunScriptRequest,
  result: PineTSRunResult,
  metadata: WorkerMetadata,
): RunScriptResponse {
  const includePlots = request.includePlots !== false;
  const plots = includePlots ? normalizePlots(result.plots) : [];
  const response: RunScriptResponse = {
    jobId: request.jobId,
    outputs: includePlots ? plots.map((plot) => ({
      name: plot.name,
      kind: "plot",
      values: plot.values,
    })) : [],
    plots,
    orderIntents: normalizeResultOrderIntents(result, request),
    alerts: normalizeAlerts(result.alerts),
    visualOutputs: normalizeVisualOutputs(result),
    logs: normalizeStringList(result.logs),
    warnings: normalizeStringList(result.warnings),
    diagnostics: result.diagnostics ?? [],
    metadata,
  };
  const strategyMetrics = normalizeStrategyMetrics(result.strategy);
  if (strategyMetrics !== undefined) {
    response.strategyMetrics = strategyMetrics;
  }
  return response;
}

function buildErrorResponse(
  request: PreparedRunScriptRequest,
  message: string,
  metadata: WorkerMetadata,
): RunScriptResponse {
  return {
    jobId: request.jobId ?? "",
    outputs: [],
    plots: [],
    orderIntents: [],
    alerts: [],
    visualOutputs: [],
    logs: [],
    warnings: [],
    diagnostics: [{ severity: "error", code: "worker.error", message }],
    metadata,
    error: message,
  };
}

function normalizePlots(plots: PineTSRunResult["plots"]): PlotOutput[] {
  return Object.entries(plots ?? {}).map(([name, plot]) => ({
    name,
    values: normalizePlotValues(plot),
  }));
}

function normalizePlotValues(plot: PineTSPlot | number[]): number[] {
  const source = Array.isArray(plot) ? plot : plot.data ?? [];
  return source.map((point) => {
    const value = typeof point === "number" ? point : point.value;
    return Number.isFinite(value) ? Number(value) : 0;
  });
}

function normalizeAlerts(items: unknown[] | undefined): AlertEvent[] {
  return (items ?? []).flatMap((item) => {
    if (typeof item !== "object" || item === null) {
      return [];
    }
    const raw = item as Record<string, unknown>;
    const alert: AlertEvent = {
      type: toStringValue(raw.type, "alert"),
      id: toStringValue(raw.id, ""),
      message: toStringValue(raw.message, ""),
      barIndex: toInteger(raw.bar_index ?? raw.barIndex, 0),
      time: toInteger(raw.time, 0),
    };
    setAlertString(alert, "title", raw.title);
    setAlertString(alert, "frequency", raw.freq ?? raw.frequency);
    return [alert];
  });
}

function normalizeVisualOutputs(result: PineTSRunResult): VisualOutput[] {
  const explicit = normalizeVisualOutputItems(result.visualOutputs, "visual");
  const drawings = normalizeVisualOutputItems(result.drawings, "drawing");
  return [...explicit, ...drawings];
}

function normalizeVisualOutputItems(value: unknown, fallbackKind: string): VisualOutput[] {
  if (value === undefined || value === null) {
    return [];
  }
  const items = Array.isArray(value) ? value : [value];
  return items.flatMap((item, index) => {
    if (typeof item !== "object" || item === null) {
      return [];
    }
    const raw = item as Record<string, unknown>;
    return [{
      kind: toStringValue(raw.kind, fallbackKind),
      name: toStringValue(raw.name ?? raw.id, `${fallbackKind}-${index + 1}`),
      payloadJson: stableStringify(raw),
    }];
  });
}

function normalizeOrderIntents(items: unknown[] | undefined, request: PreparedRunScriptRequest): OrderIntent[] {
  return (items ?? []).flatMap((item) => {
    if (typeof item !== "object" || item === null) {
      return [];
    }
    const raw = item as Record<string, unknown>;
    const barIndex = toInteger(raw.barIndex, request.candles.length - 1);
    const candle = request.candles[barIndex] ?? request.candles[request.candles.length - 1];
    const intent: OrderIntent = {
      kind: toStringValue(raw.kind, "entry"),
      disableAlert: Boolean(raw.disableAlert),
      barIndex,
      time: toInteger(raw.time, candle?.openTime ?? 0),
      hasQuantity: raw.quantity !== undefined,
      hasQuantityPct: raw.quantityPct !== undefined,
      hasLimitPrice: raw.limitPrice !== undefined,
      hasStopPrice: raw.stopPrice !== undefined,
    };
    setString(intent, "id", raw.id);
    setString(intent, "fromEntry", raw.fromEntry);
    setString(intent, "direction", raw.direction);
    setNumber(intent, "quantity", raw.quantity);
    setNumber(intent, "quantityPct", raw.quantityPct);
    setNumber(intent, "limitPrice", raw.limitPrice);
    setNumber(intent, "stopPrice", raw.stopPrice);
    setString(intent, "comment", raw.comment);
    setString(intent, "alertMessage", raw.alertMessage);
    return [intent];
  });
}

function normalizeResultOrderIntents(result: PineTSRunResult, request: PreparedRunScriptRequest): OrderIntent[] {
  if ((result.orderIntents ?? []).length > 0) {
    return normalizeOrderIntents(result.orderIntents, request);
  }
  return orderIntentsFromStrategyTrades(result.strategy, request);
}

function normalizeStrategyMetrics(strategy: unknown): StrategyMetrics | undefined {
  if (typeof strategy !== "object" || strategy === null) {
    return undefined;
  }
  const raw = strategy as Record<string, unknown>;
  const buyAndHoldPnl = optionalNumber(raw.buy_and_hold_pnl ?? raw.buyAndHoldPnl);
  const buyAndHoldPerGain = optionalNumber(raw.buy_and_hold_per_gain ?? raw.buyAndHoldPerGain);
  const strategyOutperformance = optionalNumber(raw.strategy_outperformance ?? raw.strategyOutperformance);
  const hasBuyAndHoldPnl = buyAndHoldPnl !== undefined;
  const hasBuyAndHoldPerGain = buyAndHoldPerGain !== undefined;
  const hasStrategyOutperformance = strategyOutperformance !== undefined;
  if (!hasBuyAndHoldPnl && !hasBuyAndHoldPerGain && !hasStrategyOutperformance) {
    return undefined;
  }
  return {
    buyAndHoldPnl: buyAndHoldPnl ?? 0,
    buyAndHoldPerGain: buyAndHoldPerGain ?? 0,
    strategyOutperformance: strategyOutperformance ?? 0,
    hasBuyAndHoldPnl,
    hasBuyAndHoldPerGain,
    hasStrategyOutperformance,
  };
}

function orderIntentsFromStrategyTrades(strategy: unknown, request: PreparedRunScriptRequest): OrderIntent[] {
  if (typeof strategy !== "object" || strategy === null) {
    return [];
  }
  const raw = strategy as Record<string, unknown>;
  const intents: OrderIntent[] = [];
  for (const trade of arrayOfRecords(raw.closedtrades)) {
    const entryID = toStringValue(trade.entry_id, "entry");
    const size = Math.abs(optionalNumber(trade.size) ?? 1);
    const direction = (optionalNumber(trade.size) ?? 1) < 0 ? "short" : "long";
    const entryBarIndex = signalBarIndex(trade.entry_bar_index, request);
    const exitBarIndex = signalBarIndex(trade.exit_bar_index, request);
    intents.push({
      kind: "entry",
      id: entryID,
      direction,
      quantity: size,
      hasQuantity: true,
      barIndex: entryBarIndex,
      time: candleTime(request, entryBarIndex),
    });
    intents.push({
      kind: "close",
      id: optionalString(trade.exit_id) ?? `close_${entryID}`,
      fromEntry: entryID,
      direction,
      quantity: size,
      hasQuantity: true,
      barIndex: exitBarIndex,
      time: candleTime(request, exitBarIndex),
    });
  }
  for (const trade of arrayOfRecords(raw.opentrades)) {
    const entryID = toStringValue(trade.entry_id, "entry");
    const size = Math.abs(optionalNumber(trade.size) ?? 1);
    const direction = (optionalNumber(trade.size) ?? 1) < 0 ? "short" : "long";
    const entryBarIndex = signalBarIndex(trade.entry_bar_index, request);
    intents.push({
      kind: "entry",
      id: entryID,
      direction,
      quantity: size,
      hasQuantity: true,
      barIndex: entryBarIndex,
      time: candleTime(request, entryBarIndex),
    });
  }
  return intents;
}

function arrayOfRecords(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value)
    ? value.filter((item): item is Record<string, unknown> => typeof item === "object" && item !== null)
    : [];
}

function signalBarIndex(value: unknown, request: PreparedRunScriptRequest): number {
  const fillBarIndex = toInteger(value, request.candles.length - 1);
  return clampInteger(fillBarIndex - 1, 0, Math.max(0, request.candles.length - 1));
}

function candleTime(request: PreparedRunScriptRequest, barIndex: number): number {
  return request.candles[barIndex]?.openTime ?? 0;
}

function clampInteger(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

function normalizeStringList(items: unknown[] | undefined): string[] {
  return (items ?? []).map((item) => String(item));
}

function toStringValue(value: unknown, fallback: string): string {
  return typeof value === "string" && value.trim() !== "" ? value : fallback;
}

function optionalString(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined;
}

function optionalNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function setString<T extends keyof OrderIntent>(intent: OrderIntent, key: T, value: unknown): void {
  const normalized = optionalString(value);
  if (normalized !== undefined) {
    (intent as Record<string, unknown>)[key] = normalized;
  }
}

function setAlertString<T extends keyof AlertEvent>(alert: AlertEvent, key: T, value: unknown): void {
  const normalized = optionalString(value);
  if (normalized !== undefined) {
    (alert as Record<string, unknown>)[key] = normalized;
  }
}

function setNumber<T extends keyof OrderIntent>(intent: OrderIntent, key: T, value: unknown): void {
  const normalized = optionalNumber(value);
  if (normalized !== undefined) {
    (intent as Record<string, unknown>)[key] = normalized;
  }
}

function toInteger(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isInteger(value) ? value : fallback;
}

function jsonBytes(value: unknown): number {
  return jsonValueBytes(value);
}

function jsonStringBytes(value: string): number {
  return Buffer.byteLength(JSON.stringify(value), "utf8");
}

function numberJSONBytes(value: number): number {
  return Number.isFinite(value) ? String(value).length : 4;
}

function jsonValueBytes(value: unknown): number {
  if (value === null) {
    return 4;
  }
  switch (typeof value) {
    case "string":
      return jsonStringBytes(value);
    case "number":
      return numberJSONBytes(value);
    case "boolean":
      return value ? 4 : 5;
    case "undefined":
    case "function":
    case "symbol":
      return 0;
    case "object":
      return Array.isArray(value) ? jsonArrayBytes(value) : jsonObjectBytes(value as Record<string, unknown>);
    default:
      return 0;
  }
}

function jsonArrayBytes(values: unknown[]): number {
  if (values.length === 0) {
    return 2;
  }
  let bytes = 2 + values.length - 1;
  for (const value of values) {
    const valueBytes = jsonValueBytes(value);
    bytes += valueBytes === 0 && (value === undefined || typeof value === "function" || typeof value === "symbol") ? 4 : valueBytes;
  }
  return bytes;
}

function jsonObjectBytes(value: Record<string, unknown>): number {
  let bytes = 2;
  let count = 0;
  for (const [key, item] of Object.entries(value)) {
    if (item === undefined || typeof item === "function" || typeof item === "symbol") {
      continue;
    }
    if (count > 0) {
      bytes += 1;
    }
    bytes += jsonStringBytes(key) + 1 + jsonValueBytes(item);
    count++;
  }
  return bytes;
}

function stableStringify(value: unknown): string {
  return JSON.stringify(value, (_key, item) => {
    if (typeof item !== "object" || item === null || Array.isArray(item)) {
      return item;
    }
    return Object.keys(item).sort().reduce<Record<string, unknown>>((acc, key) => {
      acc[key] = (item as Record<string, unknown>)[key];
      return acc;
    }, {});
  }) ?? "";
}

function hashText(value: string): string {
  return createHash("sha256").update(value).digest("hex");
}

function elapsedMs(started: number): number {
  return Math.max(0, Math.round(performance.now() - started));
}
