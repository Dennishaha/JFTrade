#!/usr/bin/env node
import { buildDevWorker } from "./build-pineworker-dev.mjs";
import { spawnChecked } from "./lib/spawn.mjs";

let workerPath = "";
try {
  workerPath = buildDevWorker({ printPath: false });
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}

if (process.env.JFTRADE_DEV_API_PINEWORKER_DRY_RUN === "1") {
  const workers = pineWorkerCount();
  console.log(`DRY RUN JFTRADE_PINEWORKER_BINARY=${workerPath} JFTRADE_PINEWORKER_WORKERS=${workers} go run ./cmd/jftrade-api`);
  process.exit(0);
}

const workers = pineWorkerCount();
const status = spawnChecked("go", ["run", "./cmd/jftrade-api"], {
  env: {
    ...process.env,
    JFTRADE_PINEWORKER_BINARY: workerPath,
    JFTRADE_PINEWORKER_WORKERS: workers,
  },
});
process.exit(status);

function pineWorkerCount() {
  return process.env.JFTRADE_PINEWORKER_WORKERS?.trim() || "1";
}
