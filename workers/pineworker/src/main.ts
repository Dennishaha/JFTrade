import * as grpc from "@grpc/grpc-js";
import * as protoLoader from "@grpc/proto-loader";
import { startWorkerGrpcServer } from "./grpcServer";
import { DeterministicPineTSExecutor } from "./mockExecutor";
import { createNativePineTSExecutor } from "./pinetsExecutor";

declare const Bun: { argv?: string[] } | undefined;

const args = parseArgs((typeof Bun !== "undefined" ? Bun.argv ?? [] : []).slice(2));
const executor = args.mock ? new DeterministicPineTSExecutor() : await createNativePineTSExecutor(args.pinetsVersion);

await startWorkerGrpcServer({
  workerId: args.workerId,
  executor,
  protoPath: args.protoPath,
  address: args.address,
  grpc,
  protoLoader,
  maxMessageBytes: args.maxMessageBytes,
});

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
