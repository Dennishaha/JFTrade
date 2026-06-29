import * as grpc from "@grpc/grpc-js";
import * as protoLoader from "@grpc/proto-loader";
import { startWorkerGrpcServer } from "./grpcServer";
import { DeterministicPineTSExecutor } from "./mockExecutor";
import { createNativePineTSExecutor } from "./pinetsExecutor";

declare const Bun: { argv?: string[] } | undefined;
declare const process: {
  on?: (event: string, handler: (error: unknown) => void) => unknown;
  once?: (event: string, handler: () => void) => unknown;
} | undefined;

const args = parseArgs((typeof Bun !== "undefined" ? Bun.argv ?? [] : []).slice(2));
installFatalErrorLogging();
const executor = args.mock ? new DeterministicPineTSExecutor() : await createNativePineTSExecutor(args.pinetsVersion);

const server = await startWorkerGrpcServer({
  workerId: args.workerId,
  executor,
  protoPath: args.protoPath,
  address: args.address,
  grpc,
  protoLoader,
  maxMessageBytes: args.maxMessageBytes,
});
await waitForShutdown(server.shutdown);

function parseArgs(values: string[]) {
  const options = new Map<string, string>();
  for (let index = 0; index < values.length; index += 2) {
    const key = values[index]?.replace(/^--/, "");
    if (!key) {
      continue;
    }
    options.set(key, values[index + 1] ?? "true");
  }
  return {
    address: options.get("address") ?? "127.0.0.1:50051",
    workerId: options.get("worker-id") ?? "pineworker-1",
    protoPath: options.get("proto") ?? "pkg/strategy/pineworker/proto/pineworker.proto",
    pinetsVersion: options.get("pinets-version") ?? "unknown",
    maxMessageBytes: Number(options.get("max-message-bytes") ?? 64 * 1024 * 1024),
    mock: options.get("mock") === "true",
  };
}

function waitForShutdown(shutdown: () => void): Promise<void> {
  return new Promise((resolve) => {
    let stopped = false;
    const keepAlive = setInterval(() => undefined, 60_000);
    const stop = () => {
      if (stopped) {
        return;
      }
      stopped = true;
      clearInterval(keepAlive);
      shutdown();
      resolve();
    };
    if (typeof process === "undefined" || !process.once) {
      return;
    }
    process.once("SIGINT", stop);
    process.once("SIGTERM", stop);
  });
}

function installFatalErrorLogging(): void {
  process?.on?.("uncaughtException", (error) => {
    console.error("pineworker uncaught exception", error);
  });
  process?.on?.("unhandledRejection", (error) => {
    console.error("pineworker unhandled rejection", error);
  });
}
