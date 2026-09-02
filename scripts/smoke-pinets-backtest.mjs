#!/usr/bin/env node
import path from "node:path";
import { fileURLToPath } from "node:url";

import { buildDevWorker, nodeRuntimePath } from "./build-pineworker-dev.mjs";
import { spawnChecked } from "./lib/spawn.mjs";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
let workerPath = String(process.env.JFTRADE_PINEWORKER_BUNDLE ?? "").trim();
if (workerPath === "") {
  try {
    workerPath = await buildDevWorker({ printPath: false });
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  }
}

const status = spawnChecked("cargo", [
  "test",
  "-p",
  "jftrade-integration-pine",
  "--test",
  "real_worker_smoke",
  "--",
  "--ignored",
  "--nocapture",
], {
  cwd: repositoryRoot,
  env: {
    ...process.env,
    JFTRADE_PINEWORKER_BUNDLE: path.resolve(workerPath),
    JFTRADE_PINEWORKER_RUNTIME: String(process.env.JFTRADE_PINEWORKER_RUNTIME ?? "").trim() || nodeRuntimePath(),
    JFTRADE_PINEWORKER_PROTO: path.join(repositoryRoot, "proto/pineworker/pineworker.proto"),
  },
});
process.exit(status);
