import assert from "node:assert/strict";
import test from "node:test";

import {
  compareWebFileLengthBudget,
  fileLengthLimit,
  lineCount,
} from "./check-web-file-length-budget.mjs";

const budget = {
  srcMaxLines: 800,
  testMaxLines: 1200,
  exceptions: { "apps/web/src/legacy.ts": 900 },
};

test("counts lines the same way as the component budget", () => {
  assert.equal(lineCount(""), 0);
  assert.equal(lineCount("a\nb\n"), 2);
  assert.equal(lineCount("a\nb"), 2);
});

test("applies the test limit only under apps/web/tests", () => {
  assert.equal(fileLengthLimit("apps/web/src/pages/RiskPage.vue", budget), 800);
  assert.equal(fileLengthLimit("apps/web/tests/RiskPage.test.ts", budget), 1200);
});

test("rejects new growth, missing records, and stale exceptions", () => {
  const failures = compareWebFileLengthBudget([
    { name: "apps/web/src/new.ts", lines: 801 },
    { name: "apps/web/src/legacy.ts", lines: 901 },
    { name: "apps/web/src/shrunk.ts", lines: 700 },
    { name: "apps/web/tests/new.test.ts", lines: 1201 },
  ], {
    srcMaxLines: 800,
    testMaxLines: 1200,
    exceptions: {
      "apps/web/src/legacy.ts": 900,
      "apps/web/src/shrunk.ts": 850,
      "apps/web/src/deleted.ts": 900,
    },
  });
  assert.deepEqual(failures, [
    "apps/web/src/new.ts has 801 lines, limit 800",
    "apps/web/src/legacy.ts grew to 901 lines, budget 900",
    "apps/web/src/shrunk.ts has a stale exception at 700 lines",
    "apps/web/tests/new.test.ts has 1201 lines, limit 1200",
    "apps/web/src/deleted.ts exception does not match a scanned file",
  ]);
});

test("accepts files at or below their frozen exception", () => {
  const failures = compareWebFileLengthBudget(
    [
      { name: "apps/web/src/legacy.ts", lines: 900 },
      { name: "apps/web/src/slimmer.ts", lines: 810 },
    ],
    {
      srcMaxLines: 800,
      testMaxLines: 1200,
      exceptions: { "apps/web/src/legacy.ts": 900, "apps/web/src/slimmer.ts": 850 },
    },
  );
  assert.deepEqual(failures, []);
});

test("applies the same ratchet rules to css files", () => {
  const failures = compareWebFileLengthBudget(
    [
      { name: "apps/web/src/new.css", lines: 801 },
      { name: "apps/web/src/legacy.css", lines: 1251 },
      { name: "apps/web/src/shrunk.css", lines: 800 },
      { name: "apps/web/src/frozen.css", lines: 936 },
    ],
    {
      srcMaxLines: 800,
      testMaxLines: 1200,
      exceptions: {
        "apps/web/src/legacy.css": 1250,
        "apps/web/src/shrunk.css": 900,
        "apps/web/src/frozen.css": 936,
      },
    },
  );
  assert.deepEqual(failures, [
    "apps/web/src/new.css has 801 lines, limit 800",
    "apps/web/src/legacy.css grew to 1251 lines, budget 1250",
    "apps/web/src/shrunk.css has a stale exception at 800 lines",
  ]);
});

test("rejects slack ceilings that could hide growth", () => {
  const failures = compareWebFileLengthBudget(
    [{ name: "apps/web/src/legacy.ts", lines: 850 }],
    { srcMaxLines: 801, testMaxLines: 1201, exceptions: { "apps/web/src/legacy.ts": 800 } },
  );
  assert.deepEqual(failures, [
    "srcMaxLines must remain 800",
    "testMaxLines must remain 1200",
    "apps/web/src/legacy.ts exception must exceed 801",
  ]);
});
