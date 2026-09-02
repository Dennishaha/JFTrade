import assert from "node:assert/strict";
import test from "node:test";

import { isUnderActiveRoot, validateGoRetirement } from "./check-go-retirement.mjs";

const signature = (file, line) => new Map([[`${file}\0${line}`, 1]]);

test("retirement permits deletion without permitting a new or moved Go file", () => {
  const baselineFiles = ["internal/a.go", "pkg/b.go", "go.mod"];
  assert.deepEqual(validateGoRetirement({
    baselineFiles,
    currentFiles: ["internal/a.go"],
    baselineSignatures: new Map(),
    currentSignatures: new Map(),
  }), []);
  assert.deepEqual(validateGoRetirement({
    baselineFiles,
    currentFiles: ["internal/a.go", "internal/new.go"],
    baselineSignatures: new Map(),
    currentSignatures: new Map(),
  }), ["new or moved Go artifact: internal/new.go"]);
});

test("retirement permits removing active commands and rejects new command signatures", () => {
  const baseline = signature("package.json", "go test ./...");
  assert.deepEqual(validateGoRetirement({
    baselineFiles: [], currentFiles: [], baselineSignatures: baseline, currentSignatures: new Map(),
  }), []);
  assert.deepEqual(validateGoRetirement({
    baselineFiles: [],
    currentFiles: [],
    baselineSignatures: baseline,
    currentSignatures: signature("package.json", "go build ./..."),
  }), ["new active Go/Wails configuration in package.json: go build ./..."]);
});

test("retirement excludes only the gate's own negative fixtures from active configuration", () => {
  assert.equal(isUnderActiveRoot("scripts/check-go-retirement.test.mjs"), false);
  assert.equal(isUnderActiveRoot("scripts/check-zero-go.test.mjs"), false);
  assert.equal(isUnderActiveRoot("scripts/unrelated.test.mjs"), true);
  assert.equal(isUnderActiveRoot("package.json"), true);
});
