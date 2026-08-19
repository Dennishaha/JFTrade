import assert from "node:assert/strict";
import test from "node:test";

import {
  assertBacktestEquivalent,
  extractGoReference,
  runProcess,
} from "./check-backtest-differential.mjs";

test("extracts the machine-readable Go reference from test output", () => {
  const output = [
    "=== RUN   TestRustMigrationStage3CorpusMatchesGolden",
    "    rust_migration_stage3_test.go:177: {\"version\":1,\"cases\":[]}",
    "--- PASS: TestRustMigrationStage3CorpusMatchesGolden (0.00s)",
  ].join("\n");
  assert.deepEqual(extractGoReference(output), { version: 1, cases: [] });
});

test("requires Go Rust and golden outputs to agree", () => {
  const snapshot = { version: 1, cases: [{ id: "partial" }] };
  assert.doesNotThrow(() => assertBacktestEquivalent(
    snapshot,
    structuredClone(snapshot),
    structuredClone(snapshot),
  ));
  assert.throws(
    () => assertBacktestEquivalent(snapshot, { version: 1, cases: [] }, snapshot),
    /differs from the Go execution model/,
  );
});

test("terminates a timed out shadow process and permits a clean recovery", () => {
  assert.throws(
    () => runProcess(process.execPath, ["-e", "setTimeout(() => {}, 5000)"], { timeoutMs: 100 }),
    /timed out after 100ms/,
  );
  assert.equal(
    runProcess(process.execPath, ["-e", "process.stdout.write('recovered')"], { timeoutMs: 1000 }),
    "recovered",
  );
});
