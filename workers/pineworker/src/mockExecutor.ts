import type { PineTSExecutor, PineTSRunResult, RunScriptRequest } from "./types";

export class DeterministicPineTSExecutor implements PineTSExecutor {
  version(): string {
    return "mock-pinets-0.0.0";
  }

  async run(request: RunScriptRequest): Promise<PineTSRunResult> {
    const closes = request.candles.map((candle) => candle.close);
    const threshold = Number(request.params?.threshold ?? closes[0] ?? 0);
    const signals = closes.map((close) => (close > threshold ? 1 : close < threshold ? -1 : 0));
    const lastIndex = Math.max(0, request.candles.length - 1);
    const lastSignal = signals[lastIndex] ?? 0;

    return {
      plots: {
        close: closes,
        signal: signals,
      },
      logs: [`mock execution completed for ${request.jobId}`],
      warnings: request.source.includes("strategy.")
        ? []
        : ["no strategy namespace calls were observed by mock executor"],
      orderIntents: lastSignal === 0
        ? []
        : [{
          kind: lastSignal > 0 ? "entry" : "close",
          id: lastSignal > 0 ? "long" : "flat",
          direction: lastSignal > 0 ? "long" : "flat",
          quantity: 1,
          barIndex: lastIndex,
        }],
    };
  }
}
