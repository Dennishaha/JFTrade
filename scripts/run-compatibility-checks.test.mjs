import assert from "node:assert/strict";
import test from "node:test";

import { compatibilityStages } from "./run-compatibility-checks.mjs";

test("runs all independent compatibility capabilities in parallel", () => {
  const stages = compatibilityStages("all");
  assert.equal(stages.length, 2);
  assert.equal(stages[0].mode, "sequential");
  assert.equal(stages[0].commands[0][1].at(-1), "check:compatibility:manifests");
  assert.equal(stages[1].mode, "parallel");
  assert.deepEqual(
    stages[1].commands.map(([, args]) => args.at(-1)),
    [
      "check:compatibility:storage",
      "check:compatibility:backtest",
      "check:compatibility:provider-runtime",
      "check:compatibility:trading-strategy",
      "check:compatibility:assistant-runtime",
      "check:compatibility:api-transport",
      "check:compatibility:desktop-runtime",
    ],
  );
});

test("supports one capability and rejects unknown names", () => {
  assert.equal(compatibilityStages("storage")[0].mode, "sequential");
  assert.throws(() => compatibilityStages("migration"), /unknown compatibility capability/);
});
