import assert from "node:assert/strict";
import test from "node:test";

import {
  assertBytesUnchanged,
  assertEquivalent,
} from "./check-differential.mjs";

test("accepts identical Go Rust and golden snapshots", () => {
  const snapshot = { componentId: "backtest", version: 3, klines: [] };
  assert.doesNotThrow(() => assertEquivalent(snapshot, structuredClone(snapshot), structuredClone(snapshot)));
  assert.doesNotThrow(() => assertBytesUnchanged(Buffer.from("db"), Buffer.from("db")));
});

test("rejects semantic drift and database byte mutations", () => {
  assert.throws(
    () => assertEquivalent(
      { componentId: "backtest", version: 3 },
      { componentId: "backtest", version: 2 },
      { componentId: "backtest", version: 3 },
    ),
    /differs from the Go oracle/,
  );
  assert.throws(
    () => assertBytesUnchanged(Buffer.from("before"), Buffer.from("after")),
    /changed SQLite bytes/,
  );
});
