import assert from "node:assert/strict";
import test from "node:test";

import {
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
