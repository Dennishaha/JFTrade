import assert from "node:assert/strict";
import test from "node:test";

import {
  stage9ProductCommands,
  stage9ReferenceEnvironment,
} from "./check-stage9-product-differential.mjs";

test("Stage 9 product differential batches compiler work into six processes", () => {
  const environment = stage9ReferenceEnvironment("/tmp/stage9-product");
  const commands = stage9ProductCommands(environment, ["stage9_alerts", "stage9_plugins"]);
  assert.equal(commands.length, 6);
  assert.deepEqual(commands.map(({ executable }) => executable), [
    "go", "go", "go", "cargo", "cargo", "cargo",
  ]);
  assert.deepEqual(commands.slice(0, 3).map(({ args }) => args[1]), [
    "./scripts/rust-migration",
    "./internal/app/apiserver/servercoretest",
    "./internal/app/apiserver/webaccess",
  ]);
  assert.ok(commands.some(({ args }) => args.includes("--lib")));
  const integration = commands.find(({ label }) => label.includes("integration"));
  assert.deepEqual(integration.args.slice(-4), [
    "--test", "stage9_alerts", "--test", "stage9_plugins",
  ]);
  assert.equal(commands.some(({ executable }) => executable === "node"), false);
});

test("Stage 9 reference outputs stay isolated inside the temporary root", () => {
  const environment = stage9ReferenceEnvironment("/tmp/stage9-product");
  assert.equal(Object.keys(environment).length, 12);
  for (const value of Object.values(environment)) {
    assert.match(value, /^\/tmp\/stage9-product(?:\/|$)/);
  }
  assert.notEqual(
    environment.JFTRADE_STAGE9_DATA_MANAGEMENT_ROOT,
    environment.JFTRADE_STAGE9_DATA_MANAGEMENT_CLEANUP_ROOT,
  );
});
