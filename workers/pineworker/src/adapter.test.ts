import { describe, expect, test } from "vitest";
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
    expect(response.alerts).toContainEqual(expect.objectContaining({
      type: "alert",
      id: "mock-alert",
      message: "mock alert for job-1",
      title: "Mock Alert",
      frequency: "all",
      barIndex: 1,
      time: 1_700_000_060_000,
    }));
    expect(response.visualOutputs).toContainEqual(expect.objectContaining({
      kind: "plotshape",
      name: "mock-signal-shape",
      payloadJson: expect.stringContaining("\"barIndex\":1"),
    }));
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
      alerts: [
        { type: "alertcondition", id: "alert-1", message: "crossed", title: "Cross", freq: "all", bar_index: 0, time: 1_700_000_000_000 },
        "bad",
      ],
      drawings: { kind: "label", name: "entry-label", text: "Long" },
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
    expect(response.plots[0]!.values[1]).toBe(0);
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
    expect(response.alerts).toEqual([
      expect.objectContaining({
        type: "alertcondition",
        id: "alert-1",
        message: "crossed",
        title: "Cross",
        frequency: "all",
        barIndex: 0,
        time: 1_700_000_000_000,
      }),
    ]);
    expect(response.visualOutputs).toEqual([
      expect.objectContaining({
        kind: "label",
        name: "entry-label",
        payloadJson: "{\"kind\":\"label\",\"name\":\"entry-label\",\"text\":\"Long\"}",
      }),
    ]);
  });

  test("derives order intents from PineTS strategy closed trades", () => {
    const response = buildResponse(validRequest(), {
      strategy: {
        closedtrades: [{
          entry_id: "SmokeLong",
          entry_bar_index: 1,
          exit_id: "close_SmokeLong",
          exit_bar_index: 2,
          size: 2,
        }],
      },
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

    expect(response.orderIntents).toEqual([
      expect.objectContaining({
        kind: "entry",
        id: "SmokeLong",
        direction: "long",
        quantity: 2,
        hasQuantity: true,
        barIndex: 0,
        time: 1_700_000_000_000,
      }),
      expect.objectContaining({
        kind: "close",
        id: "close_SmokeLong",
        fromEntry: "SmokeLong",
        direction: "long",
        quantity: 2,
        hasQuantity: true,
        barIndex: 1,
        time: 1_700_000_060_000,
      }),
    ]);
  });

  test("derives order intents from PineTS short strategy trades", () => {
    const response = buildResponse(validRequest(), {
      strategy: {
        closedtrades: [{
          entry_id: "ES",
          entry_bar_index: 1,
          exit_id: "XS",
          exit_bar_index: 2,
          size: -2,
        }],
        opentrades: [{
          entry_id: "OpenShort",
          entry_bar_index: 1,
          size: -1,
        }],
      },
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

    expect(response.orderIntents).toEqual([
      expect.objectContaining({
        kind: "entry",
        id: "ES",
        direction: "short",
        quantity: 2,
        hasQuantity: true,
        barIndex: 0,
      }),
      expect.objectContaining({
        kind: "close",
        id: "XS",
        fromEntry: "ES",
        direction: "short",
        quantity: 2,
        hasQuantity: true,
        barIndex: 1,
      }),
      expect.objectContaining({
        kind: "entry",
        id: "OpenShort",
        direction: "short",
        quantity: 1,
        hasQuantity: true,
        barIndex: 0,
      }),
    ]);
  });

  test("normalizes PineTS strategy metrics from v0.9.27 state", () => {
    const response = buildResponse(validRequest(), {
      strategy: {
        buy_and_hold_pnl: 0,
        buy_and_hold_per_gain: 12.5,
        strategy_outperformance: -3.25,
        _first_entry_price: 10,
      },
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

    expect(response.strategyMetrics).toEqual({
      buyAndHoldPnl: 0,
      buyAndHoldPerGain: 12.5,
      strategyOutperformance: -3.25,
      hasBuyAndHoldPnl: true,
      hasBuyAndHoldPerGain: true,
      hasStrategyOutperformance: true,
    });
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
