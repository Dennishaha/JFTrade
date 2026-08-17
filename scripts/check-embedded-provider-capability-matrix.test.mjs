import assert from "node:assert/strict";
import test from "node:test";

import {
  checkCapabilityMatrix,
  requiredCapabilityRows,
  requiredCapabilityStatements,
  validateCapabilityMatrix,
} from "./check-embedded-provider-capability-matrix.mjs";

const completeMatrix = () =>
  [...requiredCapabilityRows, ...requiredCapabilityStatements].join("\n");

test("checked-in provider capability matrix satisfies the contract", () => {
  assert.doesNotThrow(() => checkCapabilityMatrix());
});

test("provider capability matrix contains all research contract statements", () => {
  assert.doesNotThrow(() => validateCapabilityMatrix(completeMatrix()));
});

test("provider capability matrix reports the missing contract statement", () => {
  const source = completeMatrix().replace("经济日历窗口上限 31 天", "");

  assert.throws(
    () => validateCapabilityMatrix(source),
    /missing or changed/,
  );
});

test("provider capability matrix rejects market coverage drift", () => {
  const source = completeMatrix().replace(
    "| 个股资料/财务/分析师/股权 | 支持 | US/HK |",
    "| 个股资料/财务/分析师/股权 | 支持 | US/HK/SH/SZ |",
  );

  assert.throws(
    () => validateCapabilityMatrix(source),
    /个股资料\/财务\/分析师\/股权/,
  );
});
