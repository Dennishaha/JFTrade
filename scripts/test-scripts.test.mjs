import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

import {
  pythonScriptTestSuites,
  resolvePythonScriptTestFiles,
  resolveScriptTestFiles,
  runPythonScriptTests,
  scriptTestSuites,
  scriptTestUsage,
} from "./test-scripts.mjs";

test("all suite registers every script test exactly once", () => {
  const discovered = discoverTests("scripts");
  const registered = resolveScriptTestFiles();
  assert.deepEqual([...registered].sort(), discovered);
  assert.equal(registered.length, new Set(registered).size);
});

test("suite selection is ordered and deduplicated", () => {
  assert.deepEqual(resolveScriptTestFiles(["desktop"]), [
    ...scriptTestSuites.desktop,
  ]);
  assert.deepEqual(resolveScriptTestFiles(["desktop", "desktop"]), [
    ...scriptTestSuites.desktop,
  ]);
  assert.throws(
    () => resolveScriptTestFiles(["missing"]),
    /unknown script test suite: missing/,
  );
  assert.throws(
    () => resolveScriptTestFiles(["all", "missing"]),
    /unknown script test suite: missing/,
  );
});

test("CLI lists suites and rejects unknown suites", () => {
  const listed = run("--list");
  assert.equal(listed.status, 0, listed.stderr);
  assert.match(listed.stdout, /^all\npolicy\ncompatibility\nrelease\ndesktop\n/m);

  const unknown = run("missing");
  assert.equal(unknown.status, 1);
  assert.match(unknown.stderr, /unknown script test suite: missing/);
  assert.match(unknown.stderr, /Usage:/);
  assert.match(scriptTestUsage(), /No suite is equivalent to 'all'/);
});

test("CLI accepts pnpm's explicit argument separator", () => {
  const listed = run("--", "--list");
  assert.equal(listed.status, 0, listed.stderr);
  assert.match(listed.stdout, /^all\npolicy\ncompatibility\nrelease\ndesktop\n/m);
});

test("compatibility suite also selects the parity audit Python tests", () => {
  const pythonFiles = resolvePythonScriptTestFiles(["compatibility"]);
  assert.deepEqual(pythonFiles, [...pythonScriptTestSuites.compatibility]);
  assert.ok(pythonFiles.includes("scripts/compatibility/test_audit_test_parity.py"));
  assert.ok(pythonFiles.includes("scripts/compatibility/test_parity_anchor_reconcile.py"));
  assert.ok(pythonFiles.includes("scripts/compatibility/test_parity_gap_triage.py"));
  assert.deepEqual(resolvePythonScriptTestFiles(["desktop"]), []);
  assert.deepEqual(resolvePythonScriptTestFiles(), pythonFiles);
});

test("parity audit Python tests pass through unittest discovery", () => {
  const status = runPythonScriptTests(resolvePythonScriptTestFiles(["compatibility"]), { stdio: "pipe" });
  assert.equal(status, 0);
});

function discoverTests(directory) {
  return fs
    .readdirSync(directory, { recursive: true, withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(".test.mjs"))
    .map((entry) => path.join(entry.parentPath, entry.name))
    .sort();
}

function run(...args) {
  return spawnSync(process.execPath, ["scripts/test-scripts.mjs", ...args], {
    encoding: "utf8",
  });
}
