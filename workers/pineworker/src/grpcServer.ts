import { timingSafeEqual } from "node:crypto";
import { runScriptWithPineTS, type RunAdapterOptions } from "./adapter";
import { healthStatusToProto, runScriptRequestFromProto, runScriptResponseToProto } from "./protoMapping";
import { workerVersion, type HealthStatus } from "./types";

export type ProtoLoaderModule = {
  loadSync(path: string, options: Record<string, unknown>): unknown;
};

export type GrpcModule = {
  Server: new (options?: Record<string, unknown>) => GrpcServer;
  ServerCredentials: {
    createInsecure(): unknown;
  };
  loadPackageDefinition(definition: unknown): Record<string, unknown>;
};

export type GrpcServer = {
  addService(service: unknown, handlers: Record<string, unknown>): void;
  bindAsync(address: string, credentials: unknown, callback: (error: Error | null, port: number) => void): void;
  forceShutdown?: () => void;
};

export type WorkerServerOptions = RunAdapterOptions & {
  protoPath: string;
  address: string;
  grpc: GrpcModule;
  protoLoader: ProtoLoaderModule;
  maxMessageBytes: number;
  authToken?: string;
};

export type StartedWorkerServer = {
  address: string;
  port: number;
  shutdown(): void;
};

export async function startWorkerGrpcServer(options: WorkerServerOptions): Promise<StartedWorkerServer> {
  validatePrivateBoundary(options.address, options.authToken);
  const protoPath = normalizeProtoPath(options.protoPath);
  const packageDefinition = options.protoLoader.loadSync(protoPath, {
    keepCase: true,
    longs: Number,
    enums: String,
    defaults: true,
    oneofs: true,
    includeDirs: includeDirsForProto(protoPath),
  });
  const loaded = options.grpc.loadPackageDefinition(packageDefinition);
  const service = pineWorkerServiceDefinition(loaded);
  const server = new options.grpc.Server({
    "grpc.max_receive_message_length": options.maxMessageBytes,
    "grpc.max_send_message_length": options.maxMessageBytes,
  });
  server.addService(service, createServiceHandlers(options));
  const port = await bindServer(server, options.address, options.grpc.ServerCredentials.createInsecure());
  return {
    address: options.address,
    port,
    shutdown: () => server.forceShutdown?.(),
  };
}

function validatePrivateBoundary(address: string, authToken: string | undefined): void {
  const host = address.slice(0, address.lastIndexOf(":"));
  if (!(["127.0.0.1", "localhost", "[::1]"].includes(host))) {
    throw new Error("pine worker must bind to a loopback address");
  }
  if (authToken !== undefined && authToken.trim() !== "" && authToken.trim().length < 32) {
    throw new Error("pine worker bearer token must contain at least 32 characters");
  }
}

export function createServiceHandlers(options: RunAdapterOptions & { authToken?: string }): Record<string, unknown> {
  return {
    HealthCheck: (call: UnaryCall, callback: UnaryCallback) => {
      if (!authorize(call, options.authToken, callback)) return;
      callback(null, healthStatusToProto(healthStatus(options)));
    },
    RunScript: async (call: UnaryCall, callback: UnaryCallback) => {
      if (!authorize(call, options.authToken, callback)) return;
      try {
        const request = runScriptRequestFromProto(asRecord(call.request));
        call.request = undefined;
        const response = await runScriptWithPineTS(request, options);
        callback(null, runScriptResponseToProto(response));
      } catch (error) {
        call.request = undefined;
        callback(error instanceof Error ? error : new Error(String(error)));
      }
    },
    AnalyzeScript: async (call: UnaryCall, callback: UnaryCallback) => {
      if (!authorize(call, options.authToken, callback)) return;
      try {
        const request = runScriptRequestFromProto({
          ...asRecord(call.request),
          mode: "analyze",
          candles: { encoding_version: 1, payload: Buffer.alloc(0) },
        });
        call.request = undefined;
        const response = await runScriptWithPineTS(request, options);
        callback(null, {
          job_id: response.jobId,
          ok: response.error === undefined,
          diagnostics: response.diagnostics,
          inputs: [],
          plots: response.plots.map((plot) => plot.name),
          strategy_config: {},
          metadata: runScriptResponseToProto(response).metadata,
          error: response.error ?? "",
        });
      } catch (error) {
        call.request = undefined;
        callback(error instanceof Error ? error : new Error(String(error)));
      }
    },
  };
}

function healthStatus(options: RunAdapterOptions): HealthStatus {
  const capabilities = ["health", "analyze", "run", "atomic-order-intent-v1", "extended-ticker-heikinashi-v1"];
  if (
    options.executor.openLiveSession !== undefined &&
    options.executor.appendLiveSession !== undefined &&
    options.executor.closeLiveSession !== undefined
  ) {
    capabilities.push("live-session-v1");
  }
  return {
    ok: true,
    workerId: options.workerId,
    version: workerVersion,
    pineTSVersion: options.executor.version(),
    capabilities,
  };
}

function pineWorkerServiceDefinition(loaded: Record<string, unknown>): unknown {
  const root = loaded.jftrade as Record<string, unknown> | undefined;
  const strategy = root?.strategy as Record<string, unknown> | undefined;
  const pineworker = strategy?.pineworker as Record<string, unknown> | undefined;
  const v1 = pineworker?.v1 as Record<string, unknown> | undefined;
  const service = v1?.PineWorker as { service?: unknown } | undefined;
  if (!service?.service) {
    throw new Error("PineWorker service definition not found");
  }
  return service.service;
}

function bindServer(server: GrpcServer, address: string, credentials: unknown): Promise<number> {
  return new Promise((resolve, reject) => {
    server.bindAsync(address, credentials, (error, port) => {
      if (error) {
        reject(error);
        return;
      }
      resolve(port);
    });
  });
}

function asRecord(value: unknown): Record<string, unknown> {
  return typeof value === "object" && value !== null ? value as Record<string, unknown> : {};
}

export function normalizeProtoPath(path: string): string {
  return path.replaceAll("\\", "/");
}

export function dirname(path: string): string {
  const normalized = normalizeProtoPath(path);
  const index = normalized.lastIndexOf("/");
  return index < 0 ? "." : normalized.slice(0, index);
}

export function includeDirsForProto(path: string): string[] {
  const protoDir = dirname(path);
  return [dirname(dirname(protoDir))];
}

type UnaryCall = {
  request?: unknown;
  metadata?: {
    get(key: string): unknown[];
  };
};

type UnaryCallback = (error: Error | null, response?: unknown) => void;

function authorize(call: UnaryCall, token: string | undefined, callback: UnaryCallback): boolean {
  const expected = token?.trim();
  if (!expected) return true;
  const provided = call.metadata?.get("authorization")[0];
  if (typeof provided === "string") {
    const providedBytes = Buffer.from(provided);
    const expectedBytes = Buffer.from(`Bearer ${expected}`);
    if (providedBytes.length === expectedBytes.length && timingSafeEqual(providedBytes, expectedBytes)) {
      return true;
    }
  }
  const error = new Error("missing or invalid pine worker bearer token") as Error & { code: number };
  error.code = 16;
  callback(error);
  return false;
}
