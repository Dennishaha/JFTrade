import { createHash } from "node:crypto";
import { describe, expect, test } from "vitest";
import { buildResponse, runScriptWithPineTS } from "./adapter";
import { DeterministicPineTSExecutor } from "./mockExecutor";
import { createNativePineTSExecutor } from "./pinetsExecutor";
import { prepareCandleBatch, prepareRunScriptRequest } from "./preparedRequest";
import type { PineTSExecutor, PreparedRunScriptRequest, RunScriptRequest } from "./types";

describe("runScriptWithPineTS", () => {
  test("rejects unprepared requests instead of recomputing metadata", async () => {
    const raw = { ...validRequest() } as PreparedRunScriptRequest;
    await expect(runScriptWithPineTS(raw, {
      workerId: "worker-1",
      executor: new DeterministicPineTSExecutor(),
      peakRSSBytes: () => 123,
    })).rejects.toThrow("prepared Pine worker request is required");
  });

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
    expect(response.metadata.requestBytes).toBe(Buffer.byteLength(JSON.stringify(validRequest()), "utf8"));
    expect(response.metadata.dataHash).toBe(createHash("sha256").update(JSON.stringify(validRequest().candles)).digest("hex"));
    expect(response.metadata.responseBytes).toBe(Buffer.byteLength(JSON.stringify({
      ...response,
      metadata: { ...response.metadata, responseBytes: 0 },
    }), "utf8"));
    expect(response.metadata.peakRSSBytes).toBe(123);
  });

  test("maps validation failure to an error response", async () => {
    const response = await runScriptWithPineTS(validRequest({ jobId: "" }), {
      workerId: "worker-1",
      executor: new DeterministicPineTSExecutor(),
      peakRSSBytes: () => 123,
    });

    expect(response.error).toContain("job id is required");
    expect(response.diagnostics[0]).toMatchObject({
      severity: "error",
      code: "worker.error",
    });
  });

  test("preserves non-finite candle validation on prepared requests", async () => {
    const response = await runScriptWithPineTS(validRequest({
      candles: [{ openTime: 1, closeTime: 2, open: 1, high: Number.POSITIVE_INFINITY, low: 0, close: 1, volume: 1 }],
    }), {
      workerId: "worker-1",
      executor: new DeterministicPineTSExecutor(),
      peakRSSBytes: () => 123,
    });
    expect(response.error).toContain("high must be finite");
  });

  test("maps executor failure to an error response", async () => {
    const executor: PineTSExecutor = {
      version: () => "failing-pinets",
      run: async () => {
        throw new Error("pinets runtime exploded");
      },
    };
    const response = await runScriptWithPineTS(validRequest(), { workerId: "worker-1", executor, peakRSSBytes: () => 123 });

    expect(response.error).toBe("pinets runtime exploded");
    expect(response.metadata.pineTSVersion).toBe("failing-pinets");
  });

  test("omits plots and outputs when the protocol disables plot return", async () => {
    const response = await runScriptWithPineTS(validRequest({ includePlots: false }), {
      workerId: "worker-1",
      executor: new DeterministicPineTSExecutor(),
      peakRSSBytes: () => 123,
    });

    expect(response.error).toBeUndefined();
    expect(response.plots).toEqual([]);
    expect(response.outputs).toEqual([]);
    expect(response.orderIntents).toHaveLength(1);
    expect(response.metadata.requestBytes).toBe(Buffer.byteLength(JSON.stringify(validRequest({ includePlots: false })), "utf8"));
  });

  test("preserves a filled real PineTS stop-loss as a stop exit", async () => {
    const candles: RunScriptRequest["candles"] = [
      { openTime: 1_700_000_000_000, closeTime: 1_700_000_059_999, open: 100, high: 101, low: 99, close: 100, volume: 100 },
      { openTime: 1_700_000_060_000, closeTime: 1_700_000_119_999, open: 100, high: 104, low: 98, close: 101, volume: 100 },
      { openTime: 1_700_000_120_000, closeTime: 1_700_000_179_999, open: 101, high: 102, low: 94, close: 96, volume: 100 },
    ];
    const response = await runScriptWithPineTS(validRequest({
      source: [
        `//@version=6`,
        `strategy("filled stop loss", initial_capital=100000)`,
        `if bar_index == 0`,
        `    strategy.entry("Long", strategy.long, qty=1)`,
        `if bar_index == 1 and strategy.position_size > 0`,
        `    strategy.exit("StopLoss", from_entry="Long", stop=95)`,
      ].join("\n"),
      candles,
    }), {
      workerId: "worker-1",
      executor: await createNativePineTSExecutor("0.9.28"),
      peakRSSBytes: () => 123,
    });

    expect(response.error).toBeUndefined();
    expect(response.orderIntents).toEqual([
      expect.objectContaining({
        kind: "entry",
        id: "Long",
        direction: "long",
        quantity: 1,
        hasQuantity: true,
        hasStopPrice: false,
        barIndex: 0,
      }),
      expect.objectContaining({
        kind: "exit",
        id: "StopLoss",
        fromEntry: "Long",
        direction: "long",
        quantity: 1,
        hasQuantity: true,
        stopPrice: 95,
        hasStopPrice: true,
        hasLimitPrice: false,
        barIndex: 1,
      }),
    ]);
  });

  test("preserves a filled real PineTS short stop as a short-position exit", async () => {
    const candles: RunScriptRequest["candles"] = [
      { openTime: 1_700_000_000_000, closeTime: 1_700_000_059_999, open: 100, high: 101, low: 99, close: 100, volume: 100 },
      { openTime: 1_700_000_060_000, closeTime: 1_700_000_119_999, open: 100, high: 104, low: 96, close: 99, volume: 100 },
      { openTime: 1_700_000_120_000, closeTime: 1_700_000_179_999, open: 99, high: 111, low: 98, close: 108, volume: 100 },
    ];
    const response = await runScriptWithPineTS(validRequest({
      source: [
        `//@version=6`,
        `strategy("filled short stop", initial_capital=100000)`,
        `if bar_index == 0`,
        `    strategy.entry("Short", strategy.short, qty=2)`,
        `if bar_index == 1 and strategy.position_size < 0`,
        `    strategy.exit("ShortStop", from_entry="Short", stop=110)`,
      ].join("\n"),
      candles,
    }), {
      workerId: "worker-1",
      executor: await createNativePineTSExecutor("0.9.28"),
      peakRSSBytes: () => 123,
    });

    expect(response.error).toBeUndefined();
    expect(response.orderIntents[1]).toEqual(expect.objectContaining({
      kind: "exit",
      id: "ShortStop",
      fromEntry: "Short",
      direction: "short",
      quantity: 2,
      hasQuantity: true,
      stopPrice: 110,
      hasStopPrice: true,
      hasLimitPrice: false,
      barIndex: 1,
    }));
  });

  test("returns no intents when a same-bar protective exit cannot be submitted atomically", async () => {
    const response = await runScriptWithPineTS(validRequest({
      source: [
        `//@version=6`,
        `strategy("non atomic bracket", initial_capital=100000)`,
        `if bar_index == 0`,
        `    strategy.entry("Long", strategy.long, qty=1)`,
        `    strategy.exit("StopLoss", from_entry="Long", stop=95)`,
      ].join("\n"),
    }), {
      workerId: "worker-1",
      executor: await createNativePineTSExecutor("0.9.28"),
      peakRSSBytes: () => 123,
    });

    expect(response.error).toContain("cannot atomically express a parent-linked or reduce-only protective exit");
    expect(response.orderIntents).toEqual([]);
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

  test("does not infer market intents from ambiguous PineTS closed trades", () => {
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

    expect(response.orderIntents).toEqual([]);
  });

  test("does not infer market intents from ambiguous PineTS open trades", () => {
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

    expect(response.orderIntents).toEqual([]);
  });

  test("normalizes PineTS strategy metrics from v0.9.28 state", () => {
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

function validRequest(overrides: Partial<RunScriptRequest> = {}): PreparedRunScriptRequest {
  const request: RunScriptRequest = {
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
	...overrides,
  };
	const { candles, ...fields } = request;
	return prepareRunScriptRequest(fields, prepareCandleBatch(candles));
}
