import assert from "node:assert/strict";
import test from "node:test";

import {
  checkCapabilityMatrix,
  requiredCapabilityStatements,
  validateCapabilityMatrix,
} from "./check-embedded-provider-capability-matrix.mjs";

test("checked-in provider capability matrix satisfies the contract", () => {
  assert.doesNotThrow(() => checkCapabilityMatrix());
});

test("provider capability matrix contains all research contract statements", () => {
  const source = requiredCapabilityStatements.join("\n");

  assert.doesNotThrow(() => validateCapabilityMatrix(source));
});

test("provider capability matrix reports the missing contract statement", () => {
  assert.throws(
    () => validateCapabilityMatrix("| 榜单（领涨/领跌/成交活跃）"),
    /missing/,
  );
});
