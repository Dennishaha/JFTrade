#!/usr/bin/env node
import { buildDevWorker, bunRuntimePath } from "./build-pineworker-dev.mjs";
import { spawnChecked } from "./lib/spawn.mjs";

let workerPath = "";
try {
  workerPath = buildDevWorker({ printPath: false });
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}

const status = spawnChecked("go", ["test", "./pkg/backtest", "-run", "TestRealPineTSBacktestSmoke", "-count=1", "-v"], {
  env: {
    ...process.env,
    JFTRADE_PINETS_BACKTEST_SMOKE: "1",
    JFTRADE_PINEWORKER_BUNDLE: workerPath,
    JFTRADE_PINEWORKER_RUNTIME: bunRuntimePath(),
    JFTRADE_PINEWORKER_WORKERS: "1",
  },
});
process.exit(status);
