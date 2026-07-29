import assert from "node:assert/strict";
import test from "node:test";

import { errorlintArguments, normalizedDiffBase } from "./run-errorlint.mjs";

test("normalizes CI diff bases and rejects empty event sentinels", () => {
  assert.equal(normalizedDiffBase(" origin/main "), "origin/main");
  assert.equal(normalizedDiffBase("00000000000000000000"), "");
  assert.equal(normalizedDiffBase(""), "");
});

test("runs errorlint only against code added since the selected base", () => {
  const args = errorlintArguments("base-sha");

  assert.deepEqual(args.slice(0, 3), [
    "run",
    "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.0",
    "run",
  ]);
  assert.equal(args.includes("--enable-only=errorlint"), true);
  assert.equal(args.includes("--new-from-rev=base-sha"), true);
  assert.throws(() => errorlintArguments(""), /requires a diff base/);
});
