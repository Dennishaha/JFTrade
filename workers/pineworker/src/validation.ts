import type { Candle, RunMode, RunScriptRequest } from "./types";

export type WorkerLimits = {
  maxCandles?: number;
  maxSourceBytes: number;
  maxParamCount: number;
};

export const defaultWorkerLimits: WorkerLimits = {
  maxCandles: 0,
  maxSourceBytes: 1_000_000,
  maxParamCount: 256,
};

export function normalizeMode(mode: RunScriptRequest["mode"]): RunMode {
  const normalized = String(mode ?? "backtest").trim().toLowerCase();
  if (normalized === "backtest" || normalized === "live" || normalized === "analyze") {
    return normalized;
  }
  throw new Error(`unsupported pine worker mode: ${mode}`);
}

export function validateRunScriptRequest(
  request: RunScriptRequest,
  limits: WorkerLimits = defaultWorkerLimits,
): RunMode {
  requireText(request.jobId, "job id");
  const operation = normalizeSessionOperation(request.sessionOperation);
  if (operation !== undefined) {
    requireText(request.sessionId, "session id");
  }
  const mode = normalizeMode(request.mode);
  if (operation !== undefined && mode !== "live") {
    throw new Error("pine worker sessions require live mode");
  }
  if (operation === "open" && (request.expectedRevision ?? 0) !== 0) {
    throw new Error("pine worker session open requires expected revision 0");
  }
  if (operation === "append" && (request.expectedRevision ?? 0) === 0) {
    throw new Error("pine worker session append requires a positive expected revision");
  }
  if (operation === "close") {
    return "live";
  }
  requireText(request.source, "source");
  if (mode !== "analyze") {
    requireText(request.symbol, "symbol");
    requireText(request.timeframe, "timeframe");
  }
  if (byteLength(request.source) > limits.maxSourceBytes) {
    throw new Error(`source bytes exceed limit: ${byteLength(request.source)} > ${limits.maxSourceBytes}`);
  }
  if (Object.keys(request.params ?? {}).length > limits.maxParamCount) {
    throw new Error(`param count exceeds limit: ${Object.keys(request.params ?? {}).length} > ${limits.maxParamCount}`);
  }
  if (request.candles.length === 0 && mode !== "analyze") {
    throw new Error("candles are required");
  }
  const maxCandles = limits.maxCandles ?? 0;
  if (maxCandles > 0 && request.candles.length > maxCandles) {
    throw new Error(`too many candles: ${request.candles.length} > ${maxCandles}`);
  }
  request.candles.forEach(validateCandle);
  return mode;
}

export function normalizeSessionOperation(
  operation: RunScriptRequest["sessionOperation"],
): "open" | "append" | "close" | undefined {
  const normalized = String(operation ?? "").trim().toLowerCase();
  if (normalized === "") return undefined;
  if (normalized === "open" || normalized === "append" || normalized === "close") return normalized;
  throw new Error(`unsupported pine worker session operation: ${operation}`);
}

function validateCandle(candle: Candle, index: number): void {
  if (!Number.isFinite(candle.openTime) || candle.openTime <= 0) {
    throw new Error(`candle ${index}: open time is required`);
  }
  if (candle.closeTime !== undefined && candle.closeTime < candle.openTime) {
    throw new Error(`candle ${index}: close time is before open time`);
  }
  for (const [name, value] of Object.entries({
    open: candle.open,
    high: candle.high,
    low: candle.low,
    close: candle.close,
    volume: candle.volume,
  })) {
    if (!Number.isFinite(value)) {
      throw new Error(`candle ${index}: ${name} must be finite`);
    }
  }
  if (candle.high < candle.low) {
    throw new Error(`candle ${index}: high is below low`);
  }
  if (candle.open < candle.low || candle.open > candle.high) {
    throw new Error(`candle ${index}: open is outside high/low range`);
  }
  if (candle.close < candle.low || candle.close > candle.high) {
    throw new Error(`candle ${index}: close is outside high/low range`);
  }
  if (candle.volume < 0) {
    throw new Error(`candle ${index}: volume is negative`);
  }
}

function requireText(value: string | undefined, name: string): void {
  if ((value ?? "").trim() === "") {
    throw new Error(`${name} is required`);
  }
}

function byteLength(value: string): number {
  return new TextEncoder().encode(value).byteLength;
}
