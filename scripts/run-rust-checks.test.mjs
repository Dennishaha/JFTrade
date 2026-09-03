import assert from "node:assert/strict";
import test from "node:test";

import { executionStagesForRustGate } from "./run-rust-checks.mjs";

const commandNames = (gate) => executionStagesForRustGate(gate)
  .flatMap(({ commands }) => commands)
  .map(([, args]) => args.at(-1));

test("workspace Rust gate runs the test suite once before compatibility replay", () => {
  const stages = executionStagesForRustGate("workspace");
  assert.deepEqual(stages.map(({ mode }) => mode), ["sequential", "sequential"]);
  assert.deepEqual(commandNames("workspace"), [
    "check:rust:target-health",
    "test:rust",
    "check:compatibility",
  ]);
});

test("static Rust gate separates independent policy checks from compile-heavy lint", () => {
  const stages = executionStagesForRustGate("static");
  assert.deepEqual(stages.map(({ mode }) => mode), [
    "sequential",
    "parallel",
    "sequential",
  ]);
  assert.deepEqual(commandNames("static"), [
    "check:rust:target-health",
    "check:rust:architecture",
    "check:rust:production-policy",
    "format:rust:check",
    "lint:rust",
    "check:rust:policy",
  ]);
});

test("full Rust gate composes static, one workspace test, and compatibility once", () => {
  assert.deepEqual(commandNames("full"), [
    ...commandNames("static"),
    "test:rust",
    "check:compatibility",
  ]);
  assert.equal(commandNames("full").filter((name) => name === "check:rust:target-health").length, 1);
  assert.throws(() => executionStagesForRustGate("unknown"), /unknown Rust gate/);
});
