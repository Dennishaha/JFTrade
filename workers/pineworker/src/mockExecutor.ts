import type { PineTSExecutor, PineTSRunResult, PreparedRunScriptRequest } from "./types";

export class DeterministicPineTSExecutor implements PineTSExecutor {
  version(): string {
    return "mock-pinets-0.0.0";
  }

  async run(request: PreparedRunScriptRequest): Promise<PineTSRunResult> {
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
      alerts: [{
        type: "alert",
        id: "mock-alert",
        message: `mock alert for ${request.jobId}`,
        title: "Mock Alert",
        freq: "all",
        bar_index: lastIndex,
        time: request.candles[lastIndex]?.openTime ?? 0,
      }],
      visualOutputs: [{
        kind: "plotshape",
        name: "mock-signal-shape",
        barIndex: lastIndex,
        value: lastSignal,
      }],
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
