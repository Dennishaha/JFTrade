import type { OrderIntentCapture, PineTSExecutionContext } from "./pinetsExecutor";
import {
  normalizeChartType,
  type PineTSPlot,
  type PineTSRunResult,
  type PreparedRunScriptRequest,
} from "./types";

export type ResultMarker = {
  intentCount: number;
  plotLengths: Record<string, number>;
  alertCount: number;
  visualCount: number;
  drawingCount: number;
  /**
   * A structural snapshot of the drawings that were already emitted.
   *
   * PineTS keeps drawing objects in place while a live session advances.  A
   * length marker therefore cannot tell an in-place update from an unchanged
   * drawing (or a replacement at the same array index).  The field is
   * optional for backwards compatibility with callers that construct markers
   * themselves; those callers retain the historical count-slice behaviour.
   */
  drawingSnapshots?: Record<string, DrawingSnapshot>;
  logCount: number;
  warningCount: number;
  diagnosticCount: number;
};

type DrawingSnapshot = {
  serialized: string;
  index: number;
  id?: unknown;
  name?: unknown;
  kind?: unknown;
};

export function resultMarker(result: PineTSRunResult, capture: OrderIntentCapture): ResultMarker {
  const drawings = drawingItems(result.drawings);
  return {
    intentCount: capture.intents.length,
    plotLengths: Object.fromEntries(Object.entries(result.plots ?? {}).map(([name, plot]) => [name, plotLength(plot)])),
    alertCount: result.alerts?.length ?? 0,
    visualCount: result.visualOutputs?.length ?? 0,
    drawingCount: drawings.length,
    drawingSnapshots: snapshotDrawings(drawings),
    logCount: result.logs?.length ?? 0,
    warningCount: result.warnings?.length ?? 0,
    diagnosticCount: result.diagnostics?.length ?? 0,
  };
}

export function incrementalResult(
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
  const drawings = drawingItems(result.drawings);
  if (marker.drawingSnapshots !== undefined) {
    delta.drawings = drawingDelta(drawings, marker.drawingSnapshots);
  } else if (Array.isArray(result.drawings)) {
    // Markers created before structural snapshots were introduced keep their
    // old append-only semantics.  This fallback is deliberately retained so
    // external integrations do not silently change behaviour.
    delta.drawings = result.drawings.slice(marker.drawingCount ?? 0);
  } else if (result.drawings !== undefined) {
    delta.drawings = marker.drawingCount > 0 ? undefined : result.drawings;
  }
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

function drawingItems(value: unknown): unknown[] {
  if (value === undefined || value === null) return [];
  return Array.isArray(value) ? value : [value];
}

function snapshotDrawings(items: unknown[]): Record<string, DrawingSnapshot> {
  const snapshots: Record<string, DrawingSnapshot> = {};
  const occurrences = new Map<string, number>();
  for (const [index, item] of items.entries()) {
    const identity = drawingIdentity(item, index);
    const occurrence = occurrences.get(identity.base) ?? 0;
    occurrences.set(identity.base, occurrence + 1);
    const key = `${identity.base}#${occurrence}`;
    snapshots[key] = {
      serialized: stableSerialize(item),
      index,
      id: identity.id,
      name: identity.name,
      kind: identity.kind,
    };
  }
  return snapshots;
}

function drawingDelta(
  current: unknown[],
  previous: Record<string, DrawingSnapshot>,
): unknown[] {
  const next = snapshotDrawings(current);
  const changed: unknown[] = [];

  for (const [key, snapshot] of Object.entries(next)) {
    const before = previous[key];
    if (before === undefined || before.serialized !== snapshot.serialized) {
      changed.push(current[snapshot.index]);
    }
  }

  for (const [key, snapshot] of Object.entries(previous)) {
    if (next[key] !== undefined) continue;
    changed.push(drawingTombstone(snapshot));
  }
  return changed;
}

function drawingTombstone(snapshot: DrawingSnapshot): Record<string, unknown> {
  const tombstone: Record<string, unknown> = {
    deleted: true,
    tombstone: true,
    operation: "delete",
  };
  if (snapshot.id !== undefined) tombstone.id = snapshot.id;
  if (snapshot.name !== undefined) tombstone.name = snapshot.name;
  if (snapshot.kind !== undefined) tombstone.kind = snapshot.kind;
  // Positional drawings have no stable identity.  Keeping the old position in
  // the tombstone lets a consumer remove the correct entry when it chooses to
  // render anonymous drawings.
  if (snapshot.id === undefined && snapshot.name === undefined) {
    tombstone.index = snapshot.index;
  }
  return tombstone;
}

function drawingIdentity(value: unknown, index: number): {
  base: string;
  id?: unknown;
  name?: unknown;
  kind?: unknown;
} {
  if (typeof value !== "object" || value === null) {
    return { base: `index:${index}` };
  }
  const raw = value as Record<string, unknown>;
  if (raw.id !== undefined && raw.id !== null && String(raw.id) !== "") {
    return { base: `id:${stableSerialize(raw.id)}`, id: raw.id, name: raw.name, kind: raw.kind };
  }
  if (raw.name !== undefined && raw.name !== null && String(raw.name) !== "") {
    return { base: `name:${stableSerialize(raw.name)}`, name: raw.name, id: raw.id, kind: raw.kind };
  }
  return { base: `index:${index}`, id: raw.id, name: raw.name, kind: raw.kind };
}

/** Stable, JSON-like serialization used only for equality comparisons. */
function stableSerialize(value: unknown): string {
  if (value === null) return "null";
  switch (typeof value) {
    case "string":
      return JSON.stringify(value);
    case "number":
      return Number.isFinite(value) ? JSON.stringify(value) : "null";
    case "boolean":
      return value ? "true" : "false";
    case "bigint":
      return JSON.stringify(`${value.toString()}n`);
    case "undefined":
      return "undefined";
    case "function":
    case "symbol":
      return JSON.stringify(String(value));
    default:
      break;
  }
  if (Array.isArray(value)) {
    return `[${value.map((item) => stableSerialize(item)).join(",")}]`;
  }
  const object = value as Record<string, unknown>;
  const keys = Object.keys(object).sort();
  return `{${keys.map((key) => `${JSON.stringify(key)}:${stableSerialize(object[key])}`).join(",")}}`;
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

export function assertSameLiveSessionDefinition(
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

export function compactPineTSResult(result: PineTSRunResult, includePlots: boolean): PineTSRunResult {
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
