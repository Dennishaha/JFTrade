import { describe, expect, test } from "bun:test";
import { buildResponse, runScriptWithPineTS } from "./adapter";
import { DeterministicPineTSExecutor } from "./mockExecutor";
import type { PineTSExecutor, RunScriptRequest } from "./types";

describe("runScriptWithPineTS", () => {
  test("returns normalized plots, logs, metadata, and order intents", async () => {
    const response = await runScriptWithPineTS(validRequest(), {
      workerId: "worker-1",
      executor: new DeterministicPineTSExecutor(),
      peakRSSBytes: () => 123,
    });

    expect(response.error).toBeUndefined();
    expect(response.jobId).toBe("job-1");
    expect(response.plots.map((plot) => plot.name)).toEqual(["close", "signal"]);
    expect(response.outputs[0]).toEqual({ name: "close", kind: "plot", values: [10, 12] });
    expect(response.orderIntents).toHaveLength(1);
    expect(response.orderIntents[0]).toMatchObject({
      kind: "entry",
      id: "long",
      direction: "long",
      quantity: 1,
      barIndex: 1,
      time: 1_700_000_060_000,
      hasQuantity: true,
    });
    expect(response.logs[0]).toContain("job-1");
    expect(response.metadata.workerId).toBe("worker-1");
    expect(response.metadata.pineTSVersion).toBe("mock-pinets-0.0.0");
    expect(response.metadata.requestBytes).toBeGreaterThan(0);
    expect(response.metadata.responseBytes).toBeGreaterThan(0);
    expect(response.metadata.peakRSSBytes).toBe(123);
  });

  test("maps validation failure to an error response", async () => {
    const response = await runScriptWithPineTS({ ...validRequest(), jobId: "" }, {
      workerId: "worker-1",
      executor: new DeterministicPineTSExecutor(),
    });

    expect(response.error).toContain("job id is required");
    expect(response.diagnostics[0]).toMatchObject({
      severity: "error",
      code: "worker.error",
    });
  });

  test("maps executor failure to an error response", async () => {
    const executor: PineTSExecutor = {
      version: () => "failing-pinets",
      run: async () => {
        throw new Error("pinets runtime exploded");
      },
    };
    const response = await runScriptWithPineTS(validRequest(), { workerId: "worker-1", executor });

    expect(response.error).toBe("pinets runtime exploded");
    expect(response.metadata.pineTSVersion).toBe("failing-pinets");
  });
});

describe("buildResponse", () => {
  test("normalizes PineTS plot point data and malformed order intent entries", () => {
    const response = buildResponse(validRequest(), {
      plots: {
        EMA: { data: [{ value: 1 }, { value: null }, { value: 3 }] },
      },
      logs: [1, "ok"],
      warnings: ["careful"],
      orderIntents: [
        "bad",
        { kind: "exit", id: "x", quantityPct: 50, stopPrice: 9, barIndex: 0 },
      ],
    }, {
      workerId: "worker-1",
      version: "0.1.0",
      pineTSVersion: "test",
      scriptHash: "script",
      dataHash: "data",
      durationMs: 1,
      requestBytes: 2,
      responseBytes: 3,
      peakRSSBytes: 4,
    });

    expect(response.plots[0]!.values[0]).toBe(1);
    expect(Number.isNaN(response.plots[0]!.values[1])).toBe(true);
    expect(response.orderIntents).toHaveLength(1);
    expect(response.orderIntents[0]).toMatchObject({
      kind: "exit",
      id: "x",
      quantityPct: 50,
      stopPrice: 9,
      hasQuantityPct: true,
      hasStopPrice: true,
      time: 1_700_000_000_000,
    });
    expect(response.logs).toEqual(["1", "ok"]);
  });
});

function validRequest(): RunScriptRequest {
  return {
    jobId: "job-1",
    scriptId: "script-1",
    source: `//@version=6\nstrategy("x")\nplot(close, "close")`,
    symbol: "US.AAPL",
    timeframe: "1",
    mode: "backtest",
    candles: [
      { openTime: 1_700_000_000_000, open: 10, high: 11, low: 9, close: 10, volume: 100 },
      { openTime: 1_700_000_060_000, open: 10, high: 13, low: 9, close: 12, volume: 110 },
    ],
    params: { threshold: "10" },
  };
}
