import { describe, expect, test, vi } from "vitest";
import { buildResponse, runScriptWithPineTS } from "./adapter";
import { createServiceHandlers, includeDirsForProto, startWorkerGrpcServer, type GrpcModule, type GrpcServer } from "./grpcServer";
import { DeterministicPineTSExecutor } from "./mockExecutor";
import { normalizePineSourceForPineTS } from "./pinetsExecutor";
import { prepareCandleBatch, prepareRunScriptRequest } from "./preparedRequest";
import { createPeakRSSReader, installFatalErrorHandlers } from "./processRuntime";
import { candleBatchEncodingVersion, runScriptRequestFromProto, runScriptResponseToProto } from "./protoMapping";
import { normalizeSessionOperation, validateRunScriptRequest } from "./validation";
import type { PineTSExecutor, PineTSRunResult, PreparedRunScriptRequest, RunScriptRequest, WorkerMetadata } from "./types";

describe("Pine worker request and session boundaries", () => {
  test("validates each stateful session transition before execution", () => {
    expect(validateRunScriptRequest(rawRequest({
      mode: "live",
      sessionId: "session-1",
      sessionOperation: "open",
      expectedRevision: 0,
    }))).toBe("live");

    expect(() => validateRunScriptRequest(rawRequest({ sessionOperation: "open" }))).toThrow("session id is required");
    expect(() => validateRunScriptRequest(rawRequest({
      mode: "backtest", sessionId: "session-1", sessionOperation: "open",
    }))).toThrow("sessions require live mode");
    expect(() => validateRunScriptRequest(rawRequest({
      mode: "live", sessionId: "session-1", sessionOperation: "open", expectedRevision: 1,
    }))).toThrow("open requires expected revision 0");
    expect(() => validateRunScriptRequest(rawRequest({
      mode: "live", sessionId: "session-1", sessionOperation: "append", expectedRevision: 0,
    }))).toThrow("append requires a positive expected revision");
    expect(validateRunScriptRequest(rawRequest({
      mode: "live", sessionId: "session-1", sessionOperation: "close", expectedRevision: 4,
      source: "", symbol: "", timeframe: "", candles: [],
    }))).toBe("live");
  });

  test("rejects bounded source, candle, and all malformed candle values", () => {
    expect(() => validateRunScriptRequest(rawRequest({ source: "four" }), {
      maxCandles: 1,
      maxSourceBytes: 3,
      maxParamCount: 1,
    })).toThrow("source bytes exceed limit");
    expect(() => validateRunScriptRequest(rawRequest({ candles: [candle(), candle()] }), {
      maxCandles: 1,
      maxSourceBytes: 100,
      maxParamCount: 1,
    })).toThrow("too many candles");

    const cases: Array<[string, Partial<RunScriptRequest["candles"][number]>]> = [
      ["open time is required", { openTime: 0 }],
      ["close time is before open time", { closeTime: 0 }],
      ["open must be finite", { open: Number.NaN }],
      ["high must be finite", { high: Number.POSITIVE_INFINITY }],
      ["high is below low", { high: 8 }],
      ["open is outside high/low range", { open: 13 }],
      ["close is outside high/low range", { close: 13 }],
      ["volume is negative", { volume: -1 }],
    ];
    for (const [message, overrides] of cases) {
      expect(() => validateRunScriptRequest(rawRequest({ candles: [{ ...candle(), ...overrides }] })), message)
        .toThrow(message);
    }
    expect(normalizeSessionOperation(undefined)).toBeUndefined();
    expect(normalizeSessionOperation(" APPEND ")).toBe("append");
    expect(() => normalizeSessionOperation("replace")).toThrow("unsupported pine worker session operation");
  });

  test("routes open, append, and close through a stateful executor", async () => {
    const executor = statefulExecutor();
    const options = { workerId: "worker-1", executor, peakRSSBytes: () => 123 };

    const opened = await runScriptWithPineTS(preparedRequest({
      mode: "live", sessionId: "session-1", sessionOperation: "open", expectedRevision: 0,
    }), options);
    const appended = await runScriptWithPineTS(preparedRequest({
      mode: "live", sessionId: "session-1", sessionOperation: "append", expectedRevision: 1,
    }), options);
    const closed = await runScriptWithPineTS(preparedRequest({
      mode: "live", sessionId: "session-1", sessionOperation: "close", expectedRevision: 2,
    }), options);

    expect(opened).toMatchObject({ sessionId: "session-1", sessionRevision: 1, plots: [{ name: "close" }] });
    expect(appended).toMatchObject({ sessionId: "session-1", sessionRevision: 2, logs: ["append"] });
    expect(closed).toMatchObject({ sessionId: "session-1", sessionRevision: 3, plots: [] });
  });

  test("fails closed when the executor does not expose a requested live transition", async () => {
    const executor: PineTSExecutor = {
      version: () => "run-only",
      run: async () => ({ plots: {} }),
    };
    const options = { workerId: "worker-1", executor, peakRSSBytes: () => 123 };
    for (const [operation, revision] of [["open", 0], ["append", 1], ["close", 1]] as const) {
      const response = await runScriptWithPineTS(preparedRequest({
        mode: "live", sessionId: "session-1", sessionOperation: operation, expectedRevision: revision,
      }), options);
      expect(response.error).toContain("does not support stateful live sessions");
      expect(response).toMatchObject({ sessionId: "session-1", sessionRevision: revision });
    }
  });
});

describe("Pine worker protocol failure boundaries", () => {
  test("normalizes incomplete runtime output without inventing broker intent fields", () => {
    const response = buildResponse(preparedRequest(), {
      plots: {
        mixed: { data: [{ value: Number.POSITIVE_INFINITY }, { value: 2 }] },
        direct: [1, 2],
      },
      alerts: [null, { type: "", id: 1, message: null, barIndex: "bad", time: 3 }],
      visualOutputs: [null, { id: "shape-1" }],
      drawings: { kind: "", name: "", value: 1 },
      orderIntents: [null, {
        kind: 1,
        id: "",
        fromEntry: "entry-1",
        direction: "long",
        quantity: "not-a-number",
        quantityPct: 0,
        limitPrice: Number.POSITIVE_INFINITY,
        stopPrice: 10,
        comment: "covered",
        alertMessage: "alert",
        parentId: "parent-1",
        atomicGroupId: "atomic-1",
        ocoGroupId: "oco-1",
        reduceOnly: "yes",
        barIndex: 99,
        time: "invalid",
      }],
      logs: [undefined, 1],
      warnings: [false],
      strategy: { unknown: true },
    } as unknown as PineTSRunResult, metadata());

    expect(response.plots).toEqual([
      { name: "mixed", values: [0, 2] },
      { name: "direct", values: [1, 2] },
    ]);
    expect(response.alerts).toEqual([{ type: "alert", id: "", message: "", barIndex: 0, time: 3 }]);
    expect(response.visualOutputs).toHaveLength(2);
    expect(response.orderIntents).toEqual([expect.objectContaining({
      kind: "entry", fromEntry: "entry-1", stopPrice: 10, hasStopPrice: true,
      barIndex: 99, time: 1_700_000_000_000, reduceOnly: true,
    })]);
    expect(response.strategyMetrics).toBeUndefined();
  });

  test("keeps protobuf handler failures transport-visible and advertises live sessions only when supported", async () => {
    const handlers = createServiceHandlers({
      workerId: "worker-1",
      executor: statefulExecutor(),
      peakRSSBytes: () => 123,
    });
    const health = await unary(handlers.HealthCheck, {});
    expect(health.capabilities).toContain("live-session-v1");
    await expect(unary(handlers.RunScript, {
      job_id: "bad-request",
      candles: { encoding_version: 0 },
    })).rejects.toThrow("unsupported candle batch encoding version");
  });

  test("handles null and hostile transport requests without retaining request payloads", async () => {
    const handlers = createServiceHandlers({
      workerId: "worker-1",
      executor: statefulExecutor(),
      peakRSSBytes: () => 123,
    });
    const analysis = await unary(handlers.AnalyzeScript, null);
    expect(analysis).toMatchObject({ ok: false, error: "job id is required" });

    const hostileRequest = new Proxy({}, {
      get: () => {
        throw "invalid transport getter";
      },
    });
    await expect(unary(handlers.RunScript, hostileRequest)).rejects.toThrow("invalid transport getter");
    expect(includeDirsForProto("pineworker.proto")).toEqual(["."]);
  });

  test("rejects missing service definitions and bind failures instead of starting a partial server", async () => {
    await expect(startWorkerGrpcServer(serverOptions({
      loadPackageDefinition: () => ({}),
    }))).rejects.toThrow("PineWorker service definition not found");
    await expect(startWorkerGrpcServer(serverOptions({
      server: new FailingGrpcServer(),
    }))).rejects.toThrow("bind failed");
  });

  test("rejects missing protobuf bytes and preserves protocol false defaults", () => {
    expect(() => runScriptRequestFromProto({
      job_id: "job-1",
      source: "strategy(\"x\")",
      symbol: "US.AAPL",
      timeframe: "1",
      candles: { encoding_version: candleBatchEncodingVersion, payload: "not-bytes" },
    })).toThrow("candle batch payload is required");

    const response = runScriptResponseToProto({
      jobId: "job-1", outputs: [], plots: [], orderIntents: [{ kind: "entry", barIndex: 0, time: 1 }],
      alerts: [], visualOutputs: [], logs: [], warnings: [], diagnostics: [], metadata: metadata(),
    });
    expect(response).toMatchObject({
      order_intents: [{ has_quantity: false, has_quantity_pct: false, has_limit_price: false, has_stop_price: false }],
    });
    expect(response).not.toHaveProperty("strategy_metrics");
  });
});

describe("Pine worker runtime fallback paths", () => {
  test("does not rewrite timenow inside quoted Pine source literals", () => {
    expect(normalizePineSourceForPineTS("label.new(bar_index, close, 'timenow')")).toBe(
      "label.new(bar_index, close, 'timenow')",
    );
  });

  test("covers mock flat and short signals while preserving strategy warnings", async () => {
    const executor = new DeterministicPineTSExecutor();
    const flat = await executor.run(preparedRequest({
      source: "strategy.flat()", params: { threshold: "10" }, candles: [{ ...candle(), close: 10 }],
    }));
    const short = await executor.run(preparedRequest({
      source: "indicator(\"x\")", params: { threshold: "10" }, candles: [{ ...candle(), close: 9 }],
    }));
    expect(flat.orderIntents).toEqual([]);
    expect(flat.warnings).toEqual([]);
    expect(short.orderIntents).toEqual([expect.objectContaining({ kind: "close", direction: "flat" })]);
    expect(short.warnings).toHaveLength(1);
  });

  test("requires RSS accounting and registers both fatal process handlers", () => {
    expect(() => createPeakRSSReader(undefined)).toThrow("process.resourceUsage is required");
    const handlers = new Map<string, (error: unknown) => void>();
    const logError = vi.fn();
    installFatalErrorHandlers({ on: (event, handler) => handlers.set(event, handler) }, logError);
    handlers.get("unhandledRejection")?.("failed promise");
    expect(logError).toHaveBeenCalledWith("pineworker unhandled rejection", "failed promise");
  });
});

function rawRequest(overrides: Partial<RunScriptRequest> = {}): RunScriptRequest {
  return {
    jobId: "job-1",
    source: "strategy(\"test\")",
    symbol: "US.AAPL",
    timeframe: "1",
    mode: "backtest",
    candles: [candle()],
    params: { threshold: "10" },
    ...overrides,
  };
}

function preparedRequest(overrides: Partial<RunScriptRequest> = {}): PreparedRunScriptRequest {
  const request = rawRequest(overrides);
  const { candles, ...fields } = request;
  return prepareRunScriptRequest(fields, prepareCandleBatch(candles));
}

function candle() {
  return {
    openTime: 1_700_000_000_000,
    closeTime: 1_700_000_060_000,
    open: 10,
    high: 12,
    low: 9,
    close: 11,
    volume: 100,
  };
}

function metadata(): WorkerMetadata {
  return {
    workerId: "worker-1",
    version: "0.1.0",
    pineTSVersion: "test",
    scriptHash: "script",
    dataHash: "data",
    durationMs: 1,
    requestBytes: 2,
    responseBytes: 3,
    peakRSSBytes: 4,
  };
}

function statefulExecutor(): PineTSExecutor {
  return {
    version: () => "stateful-test",
    run: async () => ({ plots: { close: [11] } }),
    openLiveSession: async () => ({ plots: { close: [11] } }),
    appendLiveSession: async () => ({ result: { logs: ["append"] }, revision: 2 }),
    closeLiveSession: async () => 3,
  };
}

function unary(handler: unknown, request: unknown): Promise<Record<string, unknown>> {
  return new Promise((resolve, reject) => {
    (handler as (call: unknown, callback: (error: Error | null, response?: unknown) => void) => void)(
      { request },
      (error, response) => error ? reject(error) : resolve(response as Record<string, unknown>),
    );
  });
}

function serverOptions(overrides: {
  loadPackageDefinition?: GrpcModule["loadPackageDefinition"];
  server?: GrpcServer;
} = {}) {
  const server = overrides.server ?? new SuccessfulGrpcServer();
  const grpc: GrpcModule = {
    Server: class {
      constructor() {
        return server;
      }
    } as unknown as GrpcModule["Server"],
    ServerCredentials: { createInsecure: () => "insecure" },
    loadPackageDefinition: overrides.loadPackageDefinition ?? (() => ({
      jftrade: { strategy: { pineworker: { v1: { PineWorker: { service: "service" } } } } },
    })),
  };
  return {
    workerId: "worker-1",
    executor: statefulExecutor(),
    protoPath: "/repo/pkg/strategy/pineworker/proto/pineworker.proto",
    address: "127.0.0.1:50051",
    grpc,
    protoLoader: { loadSync: () => ({}) },
    maxMessageBytes: 1024,
    peakRSSBytes: () => 123,
  };
}

class SuccessfulGrpcServer implements GrpcServer {
  addService(): void {}

  bindAsync(_address: string, _credentials: unknown, callback: (error: Error | null, port: number) => void): void {
    callback(null, 50051);
  }
}

class FailingGrpcServer implements GrpcServer {
  addService(): void {}

  bindAsync(_address: string, _credentials: unknown, callback: (error: Error | null, port: number) => void): void {
    callback(new Error("bind failed"), 0);
  }
}
