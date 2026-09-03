import assert from "node:assert/strict";
import test from "node:test";

import {
  assertBacktestEquivalent,
  runProcess,
} from "./check-backtest.mjs";

test("requires the Rust replay to match the pinned compatibility golden", () => {
  const snapshot = { version: 1, cases: [{ id: "partial" }] };
  assert.doesNotThrow(() => assertBacktestEquivalent(
    snapshot,
    structuredClone(snapshot),
  ));
  assert.throws(
    () => assertBacktestEquivalent({ version: 1, cases: [] }, snapshot),
    /pinned compatibility golden/,
  );
});

test("terminates a timed out replay process and permits a clean recovery", () => {
  assert.throws(
    () => runProcess(process.execPath, ["-e", "setTimeout(() => {}, 5000)"], { timeoutMs: 100 }),
    /timed out after 100ms/,
  );
  assert.equal(
    runProcess(process.execPath, ["-e", "process.stdout.write('recovered')"], { timeoutMs: 1000 }),
    "recovered",
  );
});
