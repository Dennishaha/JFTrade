import assert from "node:assert/strict";
import test from "node:test";

import {
  assertTradingStrategyEquivalent,
  runTradingStrategyProcess,
} from "./check-trading-strategy.mjs";

test("requires trading-strategy equality and rejects any real dispatch", () => {
  const expected = { version: "stage5.v1", commands: [{ dispatch: false }] };
  assert.doesNotThrow(() => assertTradingStrategyEquivalent(structuredClone(expected), expected));
  assert.throws(
    () => assertTradingStrategyEquivalent({ ...expected, commands: [{ dispatch: true }] }, expected),
    /pinned compatibility contract|attempted a real dispatch/,
  );
});

test("terminates a stuck trading replay and permits recovery", () => {
  assert.throws(
    () => runTradingStrategyProcess(process.execPath, ["-e", "setTimeout(() => {}, 5000)"], { timeoutMs: 100 }),
    /timed out after 100ms/,
  );
  assert.equal(
    runTradingStrategyProcess(process.execPath, ["-e", "process.stdout.write('recovered')"], { timeoutMs: 1000 }),
    "recovered",
  );
});
