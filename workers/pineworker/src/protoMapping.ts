import type {
  Diagnostic,
  HealthStatus,
  OrderIntent,
  PlotOutput,
  PreparedRunScriptRequest,
  RunScriptRequest,
  RunScriptResponse,
  StrategyMetrics,
  AlertEvent,
  VisualOutput,
  WorkerMetadata,
} from "./types";
import { PreparedCandleBatchBuilder, prepareRunScriptRequest } from "./preparedRequest";

type ProtoCandleBatch = {
  encoding_version?: unknown;
  encodingVersion?: unknown;
  payload?: unknown;
};

export const candleBatchEncodingVersion = 1;
export const candleBatchRecordBytes = 56;
const maxCandleBatchRecords = 200_000;

export function runScriptRequestFromProto(value: Record<string, unknown>): PreparedRunScriptRequest {
  const batch = candleBatchFromProto(asRecord(field(value, "candles")));
  const request: Omit<RunScriptRequest, "candles"> = {
    jobId: stringField(value, "job_id", "jobId"),
    source: stringField(value, "source"),
    symbol: stringField(value, "symbol"),
    timeframe: stringField(value, "timeframe"),
    params: mapField(value, "params"),
  };
  const scriptId = optionalStringField(value, "script_id", "scriptId");
  const mode = optionalStringField(value, "mode");
  if (scriptId !== undefined) {
    request.scriptId = scriptId;
  }
  if (mode !== undefined) {
    request.mode = mode;
  }
  const includePlots = optionalBooleanField(value, "include_plots", "includePlots");
  if (includePlots !== undefined) {
    request.includePlots = includePlots;
  }
  return prepareRunScriptRequest(request, batch);
}

export function runScriptResponseToProto(response: RunScriptResponse): Record<string, unknown> {
  const proto: Record<string, unknown> = {
    job_id: response.jobId,
    plots: response.plots.map(plotToProto),
    order_intents: response.orderIntents.map(orderIntentToProto),
    alerts: response.alerts.map(alertToProto),
    visual_outputs: response.visualOutputs.map(visualOutputToProto),
    logs: response.logs,
    warnings: response.warnings,
    diagnostics: response.diagnostics.map(diagnosticToProto),
    metadata: metadataToProto(response.metadata),
    error: response.error ?? "",
  };
  if (response.strategyMetrics !== undefined) {
    proto.strategy_metrics = strategyMetricsToProto(response.strategyMetrics);
  }
  return proto;
}

export function healthStatusToProto(status: HealthStatus): Record<string, unknown> {
  return {
    ok: status.ok,
    worker_id: status.workerId,
    version: status.version,
    pinets_version: status.pineTSVersion,
    capabilities: status.capabilities,
  };
}

function candleBatchFromProto(value: Record<string, unknown>) {
  const batch = value as ProtoCandleBatch;
  const version = Number(field(batch as Record<string, unknown>, "encoding_version", "encodingVersion") ?? 0);
  if (version !== candleBatchEncodingVersion) {
    throw new Error(`unsupported candle batch encoding version: ${version}`);
  }
  const rawPayload = field(batch as Record<string, unknown>, "payload");
  if (!(rawPayload instanceof Uint8Array)) {
    throw new Error("candle batch payload is required");
  }
  const payload = Buffer.from(rawPayload.buffer, rawPayload.byteOffset, rawPayload.byteLength);
  if (payload.byteLength % candleBatchRecordBytes !== 0) {
    throw new Error(`candle batch payload length ${payload.byteLength} is not a multiple of ${candleBatchRecordBytes}`);
  }
  const count = payload.byteLength / candleBatchRecordBytes;
  if (count > maxCandleBatchRecords) {
    throw new Error(`too many candles: ${count} > ${maxCandleBatchRecords}`);
  }
  const builder = new PreparedCandleBatchBuilder(count);
  for (let index = 0; index < count; index++) {
    const offset = index * candleBatchRecordBytes;
    builder.set(index, {
      openTime: Number(payload.readBigInt64LE(offset)),
      closeTime: Number(payload.readBigInt64LE(offset + 8)),
      open: payload.readDoubleLE(offset + 16),
      high: payload.readDoubleLE(offset + 24),
      low: payload.readDoubleLE(offset + 32),
      close: payload.readDoubleLE(offset + 40),
      volume: payload.readDoubleLE(offset + 48),
    });
  }
  return builder.finish();
}

function plotToProto(plot: PlotOutput): Record<string, unknown> {
  return {
    name: plot.name,
    values: plot.values,
  };
}

function alertToProto(alert: AlertEvent): Record<string, unknown> {
  return {
    type: alert.type,
    id: alert.id,
    message: alert.message,
    title: alert.title ?? "",
    frequency: alert.frequency ?? "",
    bar_index: alert.barIndex,
    time: alert.time,
  };
}

function visualOutputToProto(output: VisualOutput): Record<string, unknown> {
  return {
    kind: output.kind,
    name: output.name,
    payload_json: output.payloadJson,
  };
}

function strategyMetricsToProto(metrics: StrategyMetrics): Record<string, unknown> {
  return {
    buy_and_hold_pnl: metrics.buyAndHoldPnl,
    buy_and_hold_per_gain: metrics.buyAndHoldPerGain,
    strategy_outperformance: metrics.strategyOutperformance,
    has_buy_and_hold_pnl: metrics.hasBuyAndHoldPnl,
    has_buy_and_hold_per_gain: metrics.hasBuyAndHoldPerGain,
    has_strategy_outperformance: metrics.hasStrategyOutperformance,
  };
}

function diagnosticToProto(diagnostic: Diagnostic): Record<string, unknown> {
  return {
    severity: diagnostic.severity,
    code: diagnostic.code,
    message: diagnostic.message,
    line: diagnostic.line ?? 0,
    column: diagnostic.column ?? 0,
  };
}

function orderIntentToProto(intent: OrderIntent): Record<string, unknown> {
  return {
    kind: intent.kind,
    id: intent.id ?? "",
    from_entry: intent.fromEntry ?? "",
    direction: intent.direction ?? "",
    quantity: intent.quantity ?? 0,
    quantity_pct: intent.quantityPct ?? 0,
    limit_price: intent.limitPrice ?? 0,
    stop_price: intent.stopPrice ?? 0,
    comment: intent.comment ?? "",
    alert_message: intent.alertMessage ?? "",
    disable_alert: intent.disableAlert ?? false,
    bar_index: intent.barIndex,
    time: intent.time,
    has_quantity: intent.hasQuantity ?? intent.quantity !== undefined,
    has_quantity_pct: intent.hasQuantityPct ?? intent.quantityPct !== undefined,
    has_limit_price: intent.hasLimitPrice ?? intent.limitPrice !== undefined,
    has_stop_price: intent.hasStopPrice ?? intent.stopPrice !== undefined,
  };
}

function metadataToProto(metadata: WorkerMetadata): Record<string, unknown> {
  return {
    worker_id: metadata.workerId,
    version: metadata.version,
    pinets_version: metadata.pineTSVersion,
    script_hash: metadata.scriptHash,
    data_hash: metadata.dataHash,
    duration_ms: metadata.durationMs,
    request_bytes: metadata.requestBytes,
    response_bytes: metadata.responseBytes,
    peak_rss_bytes: metadata.peakRSSBytes,
  };
}

function field(value: Record<string, unknown>, snake: string, camel = snake): unknown {
  return value[snake] ?? value[camel];
}

function stringField(value: Record<string, unknown>, snake: string, camel = snake): string {
  return String(field(value, snake, camel) ?? "");
}

function optionalStringField(value: Record<string, unknown>, snake: string, camel = snake): string | undefined {
  const raw = field(value, snake, camel);
  return typeof raw === "string" && raw !== "" ? raw : undefined;
}

function optionalBooleanField(value: Record<string, unknown>, snake: string, camel = snake): boolean | undefined {
  const raw = field(value, snake, camel);
  if (raw === undefined || raw === null) {
    return undefined;
  }
  return typeof raw === "boolean" ? raw : String(raw).toLowerCase() === "true";
}

function mapField(value: Record<string, unknown>, key: string): Record<string, string> {
  const raw = value[key];
  if (typeof raw !== "object" || raw === null || Array.isArray(raw)) {
    return {};
  }
  return Object.fromEntries(Object.entries(raw).map(([entryKey, entryValue]) => [entryKey, String(entryValue)]));
}

function asRecord(value: unknown): Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value) ? value as Record<string, unknown> : {};
}
