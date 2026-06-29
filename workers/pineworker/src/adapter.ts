import type {
  OrderIntent,
  PineTSExecutor,
  PineTSPlot,
  PineTSRunResult,
  PlotOutput,
  RunScriptRequest,
  RunScriptResponse,
  WorkerMetadata,
} from "./types";
import { validateRunScriptRequest, type WorkerLimits } from "./validation";
import { workerVersion } from "./types";

export type RunAdapterOptions = {
  workerId: string;
  executor: PineTSExecutor;
  limits?: WorkerLimits;
  peakRSSBytes?: () => number;
};

export async function runScriptWithPineTS(
  request: RunScriptRequest,
  options: RunAdapterOptions,
): Promise<RunScriptResponse> {
  const started = performance.now();
  const requestBytes = jsonBytes(request);

  try {
    validateRunScriptRequest(request, options.limits);
    const result = await options.executor.run(request);
    const response = buildResponse(request, result, {
      workerId: options.workerId,
      version: workerVersion,
      pineTSVersion: options.executor.version(),
      scriptHash: await hashText(request.source),
      dataHash: await hashText(JSON.stringify(request.candles)),
      durationMs: elapsedMs(started),
      requestBytes,
      responseBytes: 0,
      peakRSSBytes: options.peakRSSBytes?.() ?? 0,
    });
    response.metadata.responseBytes = jsonBytes(response);
    return response;
  } catch (error) {
    const response = buildErrorResponse(request, String(error instanceof Error ? error.message : error), {
      workerId: options.workerId,
      version: workerVersion,
      pineTSVersion: options.executor.version(),
      scriptHash: await hashText(request.source ?? ""),
      dataHash: await hashText(JSON.stringify(request.candles ?? [])),
      durationMs: elapsedMs(started),
      requestBytes,
      responseBytes: 0,
      peakRSSBytes: options.peakRSSBytes?.() ?? 0,
    });
    response.metadata.responseBytes = jsonBytes(response);
    return response;
  }
}

export function buildResponse(
  request: RunScriptRequest,
  result: PineTSRunResult,
  metadata: WorkerMetadata,
): RunScriptResponse {
  return {
    jobId: request.jobId,
    outputs: normalizePlots(result.plots).map((plot) => ({
      name: plot.name,
      kind: "plot",
      values: plot.values,
    })),
    plots: normalizePlots(result.plots),
    orderIntents: normalizeResultOrderIntents(result, request),
    logs: normalizeStringList(result.logs),
    warnings: normalizeStringList(result.warnings),
    diagnostics: result.diagnostics ?? [],
    metadata,
  };
}

function buildErrorResponse(
  request: RunScriptRequest,
  message: string,
  metadata: WorkerMetadata,
): RunScriptResponse {
  return {
    jobId: request.jobId ?? "",
    outputs: [],
    plots: [],
    orderIntents: [],
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

function normalizeOrderIntents(items: unknown[] | undefined, request: RunScriptRequest): OrderIntent[] {
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

function normalizeResultOrderIntents(result: PineTSRunResult, request: RunScriptRequest): OrderIntent[] {
  if ((result.orderIntents ?? []).length > 0) {
    return normalizeOrderIntents(result.orderIntents, request);
  }
  return orderIntentsFromStrategyTrades(result.strategy, request);
}

function orderIntentsFromStrategyTrades(strategy: unknown, request: RunScriptRequest): OrderIntent[] {
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

function signalBarIndex(value: unknown, request: RunScriptRequest): number {
  const fillBarIndex = toInteger(value, request.candles.length - 1);
  return clampInteger(fillBarIndex - 1, 0, Math.max(0, request.candles.length - 1));
}

function candleTime(request: RunScriptRequest, barIndex: number): number {
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
  return new TextEncoder().encode(JSON.stringify(value)).byteLength;
}

async function hashText(value: string): Promise<string> {
  const bytes = new TextEncoder().encode(value);
  const digest = await crypto.subtle.digest("SHA-256", bytes);
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, "0")).join("");
}

function elapsedMs(started: number): number {
  return Math.max(0, Math.round(performance.now() - started));
}
