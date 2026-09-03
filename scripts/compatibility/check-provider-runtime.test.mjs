import assert from "node:assert/strict";
import test from "node:test";

import {
  assertProviderRuntimeEquivalent,
  runProviderRuntimeProcess,
} from "./check-provider-runtime.mjs";

test("requires provider runtime output to match the pinned contract", () => {
  const expected = { version: "stage4.v1", marketdata: [], pine: [] };
  assert.doesNotThrow(() => assertProviderRuntimeEquivalent(structuredClone(expected), expected));
  assert.throws(
    () => assertProviderRuntimeEquivalent({ ...expected, pine: [{ ok: true }] }, expected),
    /pinned compatibility contract/,
  );
});

test("terminates a stuck provider replay and permits recovery", () => {
  assert.throws(
    () => runProviderRuntimeProcess(process.execPath, ["-e", "setTimeout(() => {}, 5000)"], { timeoutMs: 100 }),
    /timed out after 100ms/,
  );
  assert.equal(
    runProviderRuntimeProcess(process.execPath, ["-e", "process.stdout.write('recovered')"], { timeoutMs: 1000 }),
    "recovered",
  );
});
