import { describe, expect, test } from "vitest";
import { createServiceHandlers, includeDirsForProto, startWorkerGrpcServer, type GrpcModule, type GrpcServer } from "./grpcServer";
import { DeterministicPineTSExecutor } from "./mockExecutor";

describe("createServiceHandlers", () => {
  test("serves health and run calls through the adapter", async () => {
    const handlers = createServiceHandlers({
      workerId: "worker-1",
      executor: new DeterministicPineTSExecutor(),
    });

    const health = await unary(handlers.HealthCheck, {});
    expect(health).toMatchObject({ ok: true, worker_id: "worker-1", capabilities: ["health", "analyze", "run"] });

    const response = await unary(handlers.RunScript, {
      job_id: "job-1",
      source: `//@version=6\nstrategy("x")`,
      symbol: "US.AAPL",
      timeframe: "1",
      mode: "backtest",
      candles: [
        { open_time: 1, open: 10, high: 12, low: 9, close: 10, volume: 100 },
        { open_time: 2, open: 10, high: 13, low: 9, close: 12, volume: 100 },
      ],
      params: { threshold: "10" },
    });

    expect(response.job_id).toBe("job-1");
    expect(response.error).toBe("");
    expect(response.plots).toContainEqual({ name: "close", values: [10, 12] });
    expect(response.order_intents).toContainEqual(expect.objectContaining({
      kind: "entry",
      id: "long",
      has_quantity: true,
    }));
  });
});

describe("startWorkerGrpcServer", () => {
  test("loads proto, registers service, applies message limits, and binds", async () => {
    const fakeServer = new FakeGrpcServer();
    const grpc: GrpcModule = {
      Server: class {
        constructor(options?: Record<string, unknown>) {
          fakeServer.options = options;
          return fakeServer;
        }
      } as unknown as GrpcModule["Server"],
      ServerCredentials: { createInsecure: () => "insecure" },
      loadPackageDefinition: () => ({
        jftrade: { strategy: { pineworker: { v1: { PineWorker: { service: "service-definition" } } } } },
      }),
    };
    const protoLoader = {
      loadSync: (path: string, options: Record<string, unknown>) => {
        fakeServer.protoPath = path;
        fakeServer.protoOptions = options;
        return {};
      },
    };

    const started = await startWorkerGrpcServer({
      workerId: "worker-1",
      executor: new DeterministicPineTSExecutor(),
      protoPath: "/repo/pkg/strategy/pineworker/proto/pineworker.proto",
      address: "127.0.0.1:50051",
      grpc,
      protoLoader,
      maxMessageBytes: 1024,
    });

    expect(started.port).toBe(50051);
    expect(fakeServer.protoPath).toBe("/repo/pkg/strategy/pineworker/proto/pineworker.proto");
    expect(fakeServer.protoOptions?.keepCase).toBe(true);
    expect(fakeServer.protoOptions?.includeDirs).toEqual(["/repo/pkg/strategy/pineworker/proto", "/repo/pkg/strategy/pineworker"]);
    expect(fakeServer.options).toMatchObject({
      "grpc.max_receive_message_length": 1024,
      "grpc.max_send_message_length": 1024,
    });
    expect(fakeServer.service).toBe("service-definition");
    expect(Object.keys(fakeServer.handlers ?? {})).toEqual(["HealthCheck", "RunScript", "AnalyzeScript"]);
    started.shutdown();
    expect(fakeServer.shutdown).toBe(true);
  });

  test("normalizes Windows proto paths before computing include dirs", () => {
    expect(includeDirsForProto(String.raw`C:\repo\pkg\strategy\pineworker\proto\pineworker.proto`)).toEqual([
      "C:/repo/pkg/strategy/pineworker/proto",
      "C:/repo/pkg/strategy/pineworker",
    ]);
  });
});

function unary(handler: unknown, request: unknown): Promise<Record<string, unknown>> {
  return new Promise((resolve, reject) => {
    (handler as (call: unknown, callback: (error: Error | null, response?: unknown) => void) => void)(
      { request },
      (error, response) => error ? reject(error) : resolve(response as Record<string, unknown>),
    );
  });
}

class FakeGrpcServer implements GrpcServer {
  options?: Record<string, unknown>;
  protoPath = "";
  protoOptions?: Record<string, unknown>;
  service?: unknown;
  handlers?: Record<string, unknown>;
  shutdown = false;

  addService(service: unknown, handlers: Record<string, unknown>): void {
    this.service = service;
    this.handlers = handlers;
  }

  bindAsync(_address: string, _credentials: unknown, callback: (error: Error | null, port: number) => void): void {
    callback(null, 50051);
  }

  start(): void {}

  forceShutdown(): void {
    this.shutdown = true;
  }
}
