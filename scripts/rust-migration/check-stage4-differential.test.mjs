import assert from "node:assert/strict";
import test from "node:test";

import {
  assertStage4Equivalent,
  runStage4Process,
} from "./check-stage4-differential.mjs";

test("requires the Rust Stage 4 output to match the pinned Go contract", () => {
  const expected = { version: "stage4.v1", marketdata: [], pine: [] };
  assert.doesNotThrow(() => assertStage4Equivalent(structuredClone(expected), expected));
  assert.throws(
    () => assertStage4Equivalent({ ...expected, pine: [{ ok: true }] }, expected),
    /pinned Go compatibility contract/,
  );
});

test("terminates a stuck Stage 4 process and permits recovery", () => {
  assert.throws(
    () => runStage4Process(process.execPath, ["-e", "setTimeout(() => {}, 5000)"], { timeoutMs: 100 }),
    /timed out after 100ms/,
  );
  assert.equal(
    runStage4Process(process.execPath, ["-e", "process.stdout.write('recovered')"], { timeoutMs: 1000 }),
    "recovered",
  );
});
