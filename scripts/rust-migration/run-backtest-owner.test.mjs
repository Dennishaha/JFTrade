import assert from "node:assert/strict";
import test from "node:test";

import {
  resolveBacktestOwner,
  selectBacktestOwner,
} from "./run-backtest-owner.mjs";

function providers(overrides = {}) {
  return {
    rust: () => ({ engine: "rust" }),
    expected: { engine: "rust" },
    assertEquivalent: (rustOutput, expected) => {
      assert.deepEqual(rustOutput, expected);
    },
    ...overrides,
  };
}

test("keeps Rust as the only supported compatibility replay owner", () => {
  assert.equal(resolveBacktestOwner([], {}), "rust");
  assert.equal(resolveBacktestOwner([], { JFTRADE_BACKTEST_CORE_OWNER: "rust" }), "rust");
  assert.equal(resolveBacktestOwner(["--owner=rust"], { JFTRADE_BACKTEST_CORE_OWNER: "other" }), "rust");
  assert.throws(() => resolveBacktestOwner(["--owner=other"], {}), /unsupported backtest owner/);
});

test("replays the pinned fixture and fails closed on drift", () => {
  assert.deepEqual(selectBacktestOwner("rust", providers()), {
    owner: "rust",
    output: { engine: "rust" },
    fixtureChecked: true,
  });
  assert.throws(
    () => selectBacktestOwner("rust", providers({ expected: { engine: "different" } })),
    /Expected values to be strictly deep-equal/,
  );
});
