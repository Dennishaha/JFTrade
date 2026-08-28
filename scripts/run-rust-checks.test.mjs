import assert from "node:assert/strict";
import test from "node:test";

import { executionStagesForRustGate } from "./run-rust-checks.mjs";

const commandNames = (gate) => executionStagesForRustGate(gate)
  .flatMap(({ commands }) => commands)
  .map(([, args]) => args.at(-1));

test("workspace Rust gate keeps compile-heavy checks sequential", () => {
  const stages = executionStagesForRustGate("workspace");
  assert.deepEqual(stages.map(({ mode }) => mode), ["sequential", "parallel", "sequential"]);
  assert.deepEqual(commandNames("workspace"), [
    "check:rust:target-health",
    "check:rust:layout",
    "check:rust:production-policy",
    "test:rust:stage9:route-coverage",
    "format:rust:check",
    "lint:rust",
    "test:rust",
  ]);
});

test("differential Rust gate batches independent stages before Stage 9", () => {
  const stages = executionStagesForRustGate("differential");
  assert.deepEqual(stages.map(({ mode }) => mode), [
    "sequential",
    "parallel",
    "parallel",
    "parallel",
    "sequential",
    "sequential",
  ]);
  assert.equal(commandNames("differential").at(-1), "test:rust:stage9:product-differential");
  assert.equal(new Set(commandNames("differential")).size, 9);
});

test("full Rust gate composes workspace and differential checks once", () => {
  const expected = [...commandNames("workspace"), ...commandNames("differential")];
  expected.splice(commandNames("workspace").length, 1);
  assert.deepEqual(commandNames("full"), expected);
  assert.equal(commandNames("full").filter((name) => name === "check:rust:target-health").length, 1);
  assert.throws(() => executionStagesForRustGate("unknown"), /unknown Rust gate/);
});
