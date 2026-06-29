import { PineTS } from "pinets";
import type { PineTSExecutor, PineTSRunResult, RunScriptRequest } from "./types";

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

  async run(request: RunScriptRequest): Promise<PineTSRunResult> {
    const periods = Math.max(1, request.candles.length);
    const pineTS = new this.module.PineTS(
      request.candles.map(toPineTSCandle),
      request.symbol,
      request.timeframe,
      periods,
    );
    pineTS.setAlertMode?.("all");
    return pineTS.run(normalizePineSourceForPineTS(request.source), periods);
  }
}

export async function createNativePineTSExecutor(version = "unknown"): Promise<NativePineTSExecutor> {
  return new NativePineTSExecutor({ PineTS }, version);
}

function toPineTSCandle(candle: RunScriptRequest["candles"][number]): Record<string, number> {
  return {
    openTime: candle.openTime,
    closeTime: candle.closeTime ?? candle.openTime,
    open: candle.open,
    high: candle.high,
    low: candle.low,
    close: candle.close,
    volume: candle.volume,
  };
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
