import assert from "node:assert/strict";
import test from "node:test";

import {
  resolveBacktestOwner,
  selectBacktestOwner,
} from "./run-backtest-owner.mjs";

function providers(overrides = {}) {
  return {
    go: () => ({ engine: "go" }),
    rust: () => ({ engine: "rust" }),
    expected: { engine: "go" },
    assertEquivalent: (goOutput, rustOutput, expected) => {
      assert.deepEqual(rustOutput, goOutput);
      assert.deepEqual(rustOutput, expected);
    },
    ...overrides,
  };
}

test("keeps Go as the default owner and lets the flag override the environment", () => {
  assert.equal(resolveBacktestOwner([], {}), "go");
  assert.equal(resolveBacktestOwner([], { JFTRADE_BACKTEST_CORE_OWNER: "shadow" }), "shadow");
  assert.equal(resolveBacktestOwner(["--owner=go"], { JFTRADE_BACKTEST_CORE_OWNER: "rust" }), "go");
  assert.throws(() => resolveBacktestOwner(["--owner=other"], {}), /unsupported backtest owner/);
});

test("runs only the explicitly selected owner", () => {
  let rustCalls = 0;
  const result = selectBacktestOwner("go", providers({ rust: () => { rustCalls += 1; } }));
  assert.deepEqual(result, { owner: "go", output: { engine: "go" }, shadowChecked: false });
  assert.equal(rustCalls, 0);
});

test("shadow mode fails closed on a mismatch and returns the Go owner on agreement", () => {
  assert.throws(() => selectBacktestOwner("shadow", providers()), /Expected values to be strictly deep-equal/);
  const snapshot = { engine: "same" };
  assert.deepEqual(
    selectBacktestOwner("shadow", providers({
      go: () => snapshot,
      rust: () => structuredClone(snapshot),
      expected: structuredClone(snapshot),
    })),
    { owner: "go", output: snapshot, shadowChecked: true },
  );
});

test("rollback after a Rust rehearsal is a stateless switch back to Go", () => {
  const selectedRust = selectBacktestOwner("rust", providers());
  const rolledBack = selectBacktestOwner("go", providers());
  assert.deepEqual(selectedRust.output, { engine: "rust" });
  assert.deepEqual(rolledBack, { owner: "go", output: { engine: "go" }, shadowChecked: false });
});
