import assert from "node:assert/strict";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  resolveCommandTimeoutMs,
  runStage9ProductDifferential,
  stage9ProductCommands,
} from "./check-stage9-product-differential.mjs";

test("Stage 9 product compatibility replay batches compiler work into three Rust processes", () => {
  const commands = stage9ProductCommands(["stage9_alerts", "stage9_plugins"]);
  assert.equal(commands.length, 3);
  assert.deepEqual(commands.map(({ executable }) => executable), [
    "cargo", "cargo", "cargo",
  ]);
  assert.ok(commands.some(({ args }) => args.includes("--lib")));
  const integration = commands.find(({ label }) => label.includes("integration"));
  assert.deepEqual(integration.args.slice(-4), [
    "--test", "stage9_alerts", "--test", "stage9_plugins",
  ]);
  assert.equal(commands.some(({ executable }) => executable === "node"), false);
  assert.equal(commands.some(({ executable }) => executable === "go"), false);
});

test("Stage 9 product replay accepts a bounded cold-runner timeout without disabling watchdogs", async () => {
  assert.equal(resolveCommandTimeoutMs({}), 300_000);
  assert.equal(
    resolveCommandTimeoutMs({ JFTRADE_STAGE9_PRODUCT_TIMEOUT_MS: "1800000" }),
    1_800_000,
  );
  for (const value of ["0", "-1", "1.5", "1800001", "unbounded"]) {
    assert.throws(
      () => resolveCommandTimeoutMs({ JFTRADE_STAGE9_PRODUCT_TIMEOUT_MS: value }),
      /JFTRADE_STAGE9_PRODUCT_TIMEOUT_MS/,
    );
  }

  const observedTimeouts = [];
  await runStage9ProductDifferential({
    root: fileURLToPath(new URL("../..", import.meta.url)),
    timeoutMs: 1_800_000,
    runner: async (_specification, { timeoutMs }) => observedTimeouts.push(timeoutMs),
  });
  assert.deepEqual(observedTimeouts, [1_800_000, 1_800_000, 1_800_000]);
});
