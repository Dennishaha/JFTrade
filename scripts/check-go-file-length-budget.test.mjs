import assert from "node:assert/strict";
import test from "node:test";

import { compareGoFileLength, lineCount } from "./check-go-file-length-budget.mjs";

test("counts Go source lines without a trailing blank line", () => {
  assert.equal(lineCount("a\nb\n"), 2);
  assert.equal(lineCount("a\nb"), 2);
});

test("allows only frozen production exceptions and rejects growth", () => {
  const failures = compareGoFileLength([
    { name: "internal/new.go", lines: 801, test: false },
    { name: "internal/integration/futu/marketdata_runtime.go", lines: 1000, test: false },
    { name: "internal/legacy_test.go", lines: 1201, test: true },
  ], {
    productionMaxLines: 800,
    testMaxLines: 1200,
    productionExceptions: {
      "internal/integration/futu/marketdata_runtime.go": 902,
    },
  });
  assert.deepEqual(failures, [
    "internal/new.go has 801 lines, limit 800",
    "internal/integration/futu/marketdata_runtime.go grew to 1000 lines, budget 902",
    "internal/legacy_test.go has 1201 lines, limit 1200",
  ]);
});

test("rejects stale exceptions and relaxed ceilings", () => {
  assert.deepEqual(compareGoFileLength(
    [
      { name: "internal/integration/futu/marketdata_runtime.go", lines: 700, test: false },
    ],
    {
      productionMaxLines: 801,
      testMaxLines: 1201,
      productionExceptions: {
        "internal/integration/futu/marketdata_runtime.go": 902,
      },
    },
  ), [
    "productionMaxLines must remain 800",
    "testMaxLines must remain 1200",
    "internal/integration/futu/marketdata_runtime.go has a stale exception at 700 lines",
  ]);
});

test("does not allow adding a new production exception", () => {
  const failures = compareGoFileLength(
    [
      { name: "internal/new.go", lines: 900, test: false },
      { name: "internal/assistant/engine/runner_chat.go", lines: 700, test: false },
    ],
    {
      productionMaxLines: 800,
      testMaxLines: 1200,
      productionExceptions: {
        "internal/integration/futu/marketdata_runtime.go": 902,
        "internal/new.go": 900,
        "internal/assistant/engine/runner_chat.go": 816,
      },
    },
  );
  assert.match(failures.join("\n"), /not an approved production exception/);
});
