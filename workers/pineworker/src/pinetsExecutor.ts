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
    return pineTS.run(request.source, periods);
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
