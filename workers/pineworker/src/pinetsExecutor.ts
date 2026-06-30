import { PineTS } from "pinets";
import type { PineTSExecutor, PineTSRunResult, PreparedRunScriptRequest } from "./types";

type PineTSModule = {
  PineTS: new (candles: unknown[], symbol?: string, timeframe?: string, periods?: number) => {
    setAlertMode?: (mode: "all" | "realtime") => void;
    run(source: string, periods?: number): Promise<PineTSRunResult>;
  };
};

export class NativePineTSExecutor implements PineTSExecutor {
  constructor(private readonly module: PineTSModule, private readonly pineTSVersion = "unknown") {}

  version(): string {
    return this.pineTSVersion;
  }

  async run(request: PreparedRunScriptRequest): Promise<PineTSRunResult> {
    const periods = Math.max(1, request.candles.length);
    const pineTS = new this.module.PineTS(
      request.candles as Record<string, number>[],
      request.symbol,
      request.timeframe,
      periods,
    );
    pineTS.setAlertMode?.("all");
    const result = await pineTS.run(normalizePineSourceForPineTS(request.source), periods);
    return compactPineTSResult(result, request.includePlots !== false);
  }
}

export async function createNativePineTSExecutor(version = "unknown"): Promise<NativePineTSExecutor> {
  return new NativePineTSExecutor({ PineTS }, version);
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
