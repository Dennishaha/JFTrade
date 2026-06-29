import { describe, expect, test } from "vitest";
import { healthStatusToProto, runScriptRequestFromProto, runScriptResponseToProto } from "./protoMapping";

describe("proto mapping", () => {
  test("maps snake_case RunScriptRequest into worker request", () => {
    const request = runScriptRequestFromProto({
      job_id: "job-1",
      script_id: "script-1",
      source: `//@version=6\nstrategy("x")`,
      symbol: "US.AAPL",
      timeframe: "1",
      mode: "backtest",
      candles: [{ open_time: 1, close_time: 2, open: 10, high: 12, low: 9, close: 11, volume: 100 }],
      params: { threshold: 10 },
    });

    expect(request.jobId).toBe("job-1");
    expect(request.scriptId).toBe("script-1");
    expect(request.candles[0]).toEqual({ openTime: 1, closeTime: 2, open: 10, high: 12, low: 9, close: 11, volume: 100 });
    expect(request.params).toEqual({ threshold: "10" });
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
      metadata: { worker_id: "worker-1", pinets_version: "mock" },
      error: "",
    });
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
