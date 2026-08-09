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
  logCount: number;
  warningCount: number;
  diagnosticCount: number;
};

export function resultMarker(result: PineTSRunResult, capture: OrderIntentCapture): ResultMarker {
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

