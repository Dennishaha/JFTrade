import { createHash, type Hash } from "node:crypto";
import {
  preparedRunScriptRequest,
  type Candle,
  type PreparedRunScriptRequest,
  type RunScriptRequest,
} from "./types";

const jsonKeyBytes = {
  jobId: jsonStringBytes("jobId") + 1,
  scriptId: jsonStringBytes("scriptId") + 1,
  source: jsonStringBytes("source") + 1,
  symbol: jsonStringBytes("symbol") + 1,
  timeframe: jsonStringBytes("timeframe") + 1,
  mode: jsonStringBytes("mode") + 1,
  candles: jsonStringBytes("candles") + 1,
  params: jsonStringBytes("params") + 1,
  includePlots: jsonStringBytes("includePlots") + 1,
  openTime: jsonStringBytes("openTime") + 1,
  closeTime: jsonStringBytes("closeTime") + 1,
  open: jsonStringBytes("open") + 1,
  high: jsonStringBytes("high") + 1,
  low: jsonStringBytes("low") + 1,
  close: jsonStringBytes("close") + 1,
  volume: jsonStringBytes("volume") + 1,
};

export type PreparedCandleBatch = {
  candles: Candle[];
  jsonBytes: number;
  dataHash: string;
};

export class PreparedCandleBatchBuilder {
  readonly candles: Candle[];
  private readonly hash: Hash;
  private jsonBytes: number;

  constructor(count: number) {
    this.candles = new Array<Candle>(count);
    this.hash = createHash("sha256");
    this.hash.update("[");
    this.jsonBytes = count === 0 ? 2 : 2 + count - 1;
  }

  set(index: number, candle: Candle): void {
    this.candles[index] = candle;
    if (index > 0) {
      this.hash.update(",");
    }
    this.hash.update(JSON.stringify(candle));
    this.jsonBytes += candleJSONBytes(candle);
  }

  finish(): PreparedCandleBatch {
    this.hash.update("]");
    return {
      candles: this.candles,
      jsonBytes: this.jsonBytes,
      dataHash: this.hash.digest("hex"),
    };
  }
}

export function prepareCandleBatch(candles: Candle[]): PreparedCandleBatch {
  const builder = new PreparedCandleBatchBuilder(candles.length);
  for (let index = 0; index < candles.length; index++) {
    const candle = candles[index]!;
    builder.set(index, {
      openTime: candle.openTime,
      closeTime: candle.closeTime ?? candle.openTime,
      open: candle.open,
      high: candle.high,
      low: candle.low,
      close: candle.close,
      volume: candle.volume,
    });
  }
  return builder.finish();
}

export function prepareRunScriptRequest(
  request: Omit<RunScriptRequest, "candles">,
  batch: PreparedCandleBatch,
): PreparedRunScriptRequest {
  const prepared = {
    ...request,
    candles: batch.candles,
  } as PreparedRunScriptRequest;
  Object.defineProperty(prepared, preparedRunScriptRequest, {
    configurable: false,
    enumerable: false,
    writable: false,
    value: {
      requestBytes: estimateRunScriptRequestBytes(prepared, batch.jsonBytes),
      dataHash: batch.dataHash,
    },
  });
  return prepared;
}

export function preparationOf(request: PreparedRunScriptRequest): PreparedRunScriptRequest[typeof preparedRunScriptRequest] {
  const preparation = request[preparedRunScriptRequest];
  if (preparation === undefined) {
    throw new Error("prepared Pine worker request is required");
  }
  return preparation;
}

function estimateRunScriptRequestBytes(request: RunScriptRequest, candlesBytes: number): number {
  return knownKeyObjectBytes([
    [jsonKeyBytes.jobId, jsonStringBytes(request.jobId)],
    [jsonKeyBytes.scriptId, request.scriptId === undefined ? undefined : jsonStringBytes(request.scriptId)],
    [jsonKeyBytes.source, jsonStringBytes(request.source)],
    [jsonKeyBytes.symbol, jsonStringBytes(request.symbol)],
    [jsonKeyBytes.timeframe, jsonStringBytes(request.timeframe)],
    [jsonKeyBytes.mode, request.mode === undefined ? undefined : jsonStringBytes(request.mode)],
    [jsonKeyBytes.candles, candlesBytes],
    [jsonKeyBytes.params, paramsJSONBytes(request.params)],
    [jsonKeyBytes.includePlots, request.includePlots === undefined ? undefined : (request.includePlots ? 4 : 5)],
  ]);
}

function candleJSONBytes(candle: Candle): number {
  let bytes = 2 + jsonKeyBytes.openTime + numberJSONBytes(candle.openTime);
  if (candle.closeTime !== undefined) {
    bytes += 1 + jsonKeyBytes.closeTime + numberJSONBytes(candle.closeTime);
  }
  bytes += 1 + jsonKeyBytes.open + numberJSONBytes(candle.open);
  bytes += 1 + jsonKeyBytes.high + numberJSONBytes(candle.high);
  bytes += 1 + jsonKeyBytes.low + numberJSONBytes(candle.low);
  bytes += 1 + jsonKeyBytes.close + numberJSONBytes(candle.close);
  bytes += 1 + jsonKeyBytes.volume + numberJSONBytes(candle.volume);
  return bytes;
}

function paramsJSONBytes(params: RunScriptRequest["params"]): number | undefined {
  if (params === undefined) {
    return undefined;
  }
  return objectBytes(Object.entries(params).map(([key, value]) => [key, value]));
}

function objectBytes(entries: [string, string | number | undefined][]): number {
  return knownKeyObjectBytes(entries.map(([key, value]) => [
    jsonStringBytes(key) + 1,
    typeof value === "string" ? jsonStringBytes(value) : value,
  ]));
}

function knownKeyObjectBytes(entries: [number, number | undefined][]): number {
  let bytes = 2;
  let count = 0;
  for (const [keyBytes, valueBytes] of entries) {
    if (valueBytes === undefined) {
      continue;
    }
    if (count > 0) {
      bytes += 1;
    }
    bytes += keyBytes + valueBytes;
    count++;
  }
  return bytes;
}

function jsonStringBytes(value: string): number {
  return Buffer.byteLength(JSON.stringify(value), "utf8");
}

function numberJSONBytes(value: number): number {
  return Number.isFinite(value) ? String(value).length : 4;
}
