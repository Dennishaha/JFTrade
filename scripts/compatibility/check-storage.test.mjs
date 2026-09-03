import assert from "node:assert/strict";
import test from "node:test";

import {
  assertBytesUnchanged,
  assertEquivalent,
} from "./check-storage.mjs";

test("accepts a Rust snapshot matching the pinned compatibility golden", () => {
  const snapshot = { componentId: "backtest", version: 3, klines: [] };
  assert.doesNotThrow(() => assertEquivalent(snapshot, structuredClone(snapshot)));
  assert.doesNotThrow(() => assertBytesUnchanged(Buffer.from("db"), Buffer.from("db")));
});

test("rejects semantic drift and database byte mutations", () => {
  assert.throws(
    () => assertEquivalent(
      { componentId: "backtest", version: 2 },
      { componentId: "backtest", version: 3 },
    ),
    /differs from the pinned golden/,
  );
  assert.throws(
    () => assertBytesUnchanged(Buffer.from("before"), Buffer.from("after")),
    /changed SQLite bytes/,
  );
});
