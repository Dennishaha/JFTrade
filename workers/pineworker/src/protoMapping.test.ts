import { describe, expect, test } from "vitest";
import { preparationOf } from "./preparedRequest";
import { candleBatchEncodingVersion, healthStatusToProto, runScriptRequestFromProto, runScriptResponseToProto } from "./protoMapping";

describe("proto mapping", () => {
  test("maps snake_case RunScriptRequest into worker request", () => {
    const request = runScriptRequestFromProto({
      job_id: "job-1",
      script_id: "script-1",
      source: `//@version=6\nstrategy("x")`,
      symbol: "US.AAPL",
      timeframe: "1",
      mode: "backtest",
      candles: {
        encoding_version: candleBatchEncodingVersion,
        payload: encodeCandles([{ openTime: 1, closeTime: 2, open: 10, high: 12, low: 9, close: 11, volume: 100 }]),
      },
      params: { threshold: 10 },
      include_plots: false,
    });

    expect(request.jobId).toBe("job-1");
    expect(request.scriptId).toBe("script-1");
    expect(request.candles[0]).toEqual({ openTime: 1, closeTime: 2, open: 10, high: 12, low: 9, close: 11, volume: 100 });
    expect(request.params).toEqual({ threshold: "10" });
    expect(request.includePlots).toBe(false);
    expect(preparationOf(request).dataHash).toHaveLength(64);
  });

  test("rejects legacy and malformed binary candle batches", () => {
    expect(() => runScriptRequestFromProto({
      job_id: "job-1",
      source: `//@version=6\nstrategy("x")`,
      symbol: "US.AAPL",
      timeframe: "1",
      mode: "backtest",
      candles: {
        open_time: [1],
      },
      params: {},
    })).toThrow("unsupported candle batch encoding version: 0");
    expect(() => runScriptRequestFromProto({
      job_id: "job-1",
      source: `//@version=6\nstrategy("x")`,
      symbol: "US.AAPL",
      timeframe: "1",
      mode: "backtest",
      candles: { encoding_version: candleBatchEncodingVersion, payload: Buffer.alloc(55) },
      params: {},
    })).toThrow("payload length 55 is not a multiple of 56");
    expect(() => runScriptRequestFromProto({
      job_id: "job-1",
      source: `//@version=6\nstrategy("x")`,
      symbol: "US.AAPL",
      timeframe: "1",
      mode: "backtest",
      candles: { encoding_version: candleBatchEncodingVersion, payload: Buffer.alloc(200_001 * 56) },
      params: {},
    })).toThrow("too many candles: 200001 > 200000");
  });

  test("decodes the shared binary golden vector", () => {
    const payload = Buffer.from("feffffffffffffff0300000000000000000000000000f83f0000000000000440000000000000e0bf00000000000000400000000000000000", "hex");
    const request = runScriptRequestFromProto({
      job_id: "golden",
      source: "strategy(\"x\")",
      symbol: "US.TEST",
      timeframe: "1",
      mode: "backtest",
      candles: { encoding_version: candleBatchEncodingVersion, payload },
    });
    expect(request.candles).toEqual([{ openTime: -2, closeTime: 3, open: 1.5, high: 2.5, low: -0.5, close: 2, volume: 0 }]);
  });

  test("maps worker response into protobuf-shaped response", () => {
    const response = runScriptResponseToProto({
      jobId: "job-1",
      outputs: [{ name: "signal", kind: "plot", values: [1] }],
      plots: [{ name: "signal", values: [1] }],
      orderIntents: [{
        kind: "entry",
        id: "long",
        direction: "long",
        quantity: 1,
        barIndex: 0,
        time: 1,
      }],
      alerts: [{
        type: "alertcondition",
        id: "alert-1",
        message: "crossed",
        title: "Cross",
        frequency: "all",
        barIndex: 0,
        time: 1,
      }],
      visualOutputs: [{
        kind: "label",
        name: "entry-label",
        payloadJson: "{\"text\":\"Long\"}",
      }],
      strategyMetrics: {
        buyAndHoldPnl: 0,
        buyAndHoldPerGain: 12.5,
        strategyOutperformance: -3.25,
        hasBuyAndHoldPnl: true,
        hasBuyAndHoldPerGain: true,
        hasStrategyOutperformance: true,
      },
      logs: ["log"],
      warnings: ["warn"],
      diagnostics: [{ severity: "info", code: "x", message: "ok" }],
      metadata: {
        workerId: "worker-1",
        version: "0.1.0",
        pineTSVersion: "mock",
        scriptHash: "script",
        dataHash: "data",
        durationMs: 1,
        requestBytes: 2,
        responseBytes: 3,
        peakRSSBytes: 4,
      },
    });

    expect(response).toMatchObject({
      job_id: "job-1",
      order_intents: [{ id: "long", has_quantity: true }],
      alerts: [{ id: "alert-1", frequency: "all", bar_index: 0 }],
      visual_outputs: [{ kind: "label", name: "entry-label", payload_json: "{\"text\":\"Long\"}" }],
      strategy_metrics: {
        buy_and_hold_pnl: 0,
        buy_and_hold_per_gain: 12.5,
        strategy_outperformance: -3.25,
        has_buy_and_hold_pnl: true,
        has_buy_and_hold_per_gain: true,
        has_strategy_outperformance: true,
      },
      metadata: { worker_id: "worker-1", pinets_version: "mock" },
      error: "",
    });
    expect(response).not.toHaveProperty("outputs");
  });

  test("maps health status", () => {
    expect(healthStatusToProto({
      ok: true,
      workerId: "worker-1",
      version: "0.1.0",
      pineTSVersion: "mock",
      capabilities: ["run"],
    })).toEqual({
      ok: true,
      worker_id: "worker-1",
      version: "0.1.0",
      pinets_version: "mock",
      capabilities: ["run"],
    });
  });
});

function encodeCandles(candles: Array<{ openTime: number; closeTime: number; open: number; high: number; low: number; close: number; volume: number }>): Buffer {
  const payload = Buffer.alloc(candles.length * 56);
  candles.forEach((candle, index) => {
    const offset = index * 56;
    payload.writeBigInt64LE(BigInt(candle.openTime), offset);
    payload.writeBigInt64LE(BigInt(candle.closeTime), offset + 8);
    payload.writeDoubleLE(candle.open, offset + 16);
    payload.writeDoubleLE(candle.high, offset + 24);
    payload.writeDoubleLE(candle.low, offset + 32);
    payload.writeDoubleLE(candle.close, offset + 40);
    payload.writeDoubleLE(candle.volume, offset + 48);
  });
  return payload;
}
