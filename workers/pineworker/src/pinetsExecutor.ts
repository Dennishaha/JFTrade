import type { PineTSExecutor, PineTSRunResult, RunScriptRequest } from "./types";

type PineTSModule = {
  PineTS: new (candles: unknown[]) => {
    run(source: string): Promise<PineTSRunResult>;
  };
};

export class NativePineTSExecutor implements PineTSExecutor {
  constructor(private readonly module: PineTSModule, private readonly pineTSVersion = "unknown") {}

  version(): string {
    return this.pineTSVersion;
  }

  async run(request: RunScriptRequest): Promise<PineTSRunResult> {
    const pineTS = new this.module.PineTS(request.candles.map(toPineTSCandle));
    return pineTS.run(request.source);
  }
}

export async function createNativePineTSExecutor(version = "unknown"): Promise<NativePineTSExecutor> {
  const module = await dynamicImport("pinets") as PineTSModule;
  return new NativePineTSExecutor(module, version);
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

function dynamicImport(specifier: string): Promise<unknown> {
  return new Function("specifier", "return import(specifier)")(specifier) as Promise<unknown>;
}
