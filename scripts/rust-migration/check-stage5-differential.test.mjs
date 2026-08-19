import assert from "node:assert/strict";
import test from "node:test";

import {
  assertStage5Equivalent,
  runStage5Process,
} from "./check-stage5-differential.mjs";

test("requires Stage 5 equality and rejects any shadow dispatch", () => {
  const expected = { version: "stage5.v1", commands: [{ dispatch: false }] };
  assert.doesNotThrow(() => assertStage5Equivalent(structuredClone(expected), expected));
  assert.throws(
    () => assertStage5Equivalent({ ...expected, commands: [{ dispatch: true }] }, expected),
    /pinned Go compatibility contract|attempted a dispatch/,
  );
});

test("terminates a stuck Stage 5 process and permits recovery", () => {
  assert.throws(
    () => runStage5Process(process.execPath, ["-e", "setTimeout(() => {}, 5000)"], { timeoutMs: 100 }),
    /timed out after 100ms/,
  );
  assert.equal(
    runStage5Process(process.execPath, ["-e", "process.stdout.write('recovered')"], { timeoutMs: 1000 }),
    "recovered",
  );
});
