import type {
  Candle,
  Diagnostic,
  HealthStatus,
  OrderIntent,
  PlotOutput,
  RunScriptRequest,
  RunScriptResponse,
  SeriesOutput,
  AlertEvent,
  VisualOutput,
  WorkerMetadata,
} from "./types";

type ProtoCandle = {
  open_time?: number;
  openTime?: number;
  close_time?: number;
  closeTime?: number;
  open?: number;
  high?: number;
  low?: number;
  close?: number;
  volume?: number;
};

export function runScriptRequestFromProto(value: Record<string, unknown>): RunScriptRequest {
  const candles = arrayValue<ProtoCandle>(field(value, "candles")).map(candleFromProto);
  const request: RunScriptRequest = {
    jobId: stringField(value, "job_id", "jobId"),
    source: stringField(value, "source"),
    symbol: stringField(value, "symbol"),
    timeframe: stringField(value, "timeframe"),
    candles,
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
  return request;
}

export function runScriptResponseToProto(response: RunScriptResponse): Record<string, unknown> {
  return {
    job_id: response.jobId,
    outputs: response.outputs.map(seriesOutputToProto),
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

function candleFromProto(value: ProtoCandle): Candle {
  const candle: Candle = {
    openTime: numberField(value, "open_time", "openTime"),
    open: numberField(value, "open"),
    high: numberField(value, "high"),
    low: numberField(value, "low"),
    close: numberField(value, "close"),
    volume: numberField(value, "volume"),
  };
  const closeTime = optionalNumberField(value, "close_time", "closeTime");
  if (closeTime !== undefined) {
    candle.closeTime = closeTime;
  }
  return candle;
}

function seriesOutputToProto(output: SeriesOutput): Record<string, unknown> {
  return {
    name: output.name,
    kind: output.kind,
    values: output.values,
  };
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

function numberField(value: Record<string, unknown>, snake: string, camel = snake): number {
  const raw = field(value, snake, camel);
  return typeof raw === "number" ? raw : Number(raw ?? 0);
}

function optionalNumberField(value: Record<string, unknown>, snake: string, camel = snake): number | undefined {
  const raw = field(value, snake, camel);
  if (raw === undefined || raw === null) {
    return undefined;
  }
  return typeof raw === "number" ? raw : Number(raw);
}

function arrayValue<T>(value: unknown): T[] {
  return Array.isArray(value) ? value as T[] : [];
}

function mapField(value: Record<string, unknown>, key: string): Record<string, string> {
  const raw = value[key];
  if (typeof raw !== "object" || raw === null || Array.isArray(raw)) {
    return {};
  }
  return Object.fromEntries(Object.entries(raw).map(([entryKey, entryValue]) => [entryKey, String(entryValue)]));
}
