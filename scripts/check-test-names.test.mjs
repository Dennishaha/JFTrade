import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  comparePolicyState,
  hasManagedCoverageName,
  isTestFile,
  parseAllowlist,
} from "./check-test-names.mjs";

const scriptPath = fileURLToPath(new URL("./check-test-names.mjs", import.meta.url));

test("recognizes Go and TypeScript test files", () => {
  assert.equal(isTestFile("pkg/trading/order_test.go"), true);
  assert.equal(isTestFile("apps/web/tests/order-flow.spec.ts"), true);
  assert.equal(isTestFile("apps/web/src/order.ts"), false);
});

test("recognizes coverage names with or without numbers and numeric shorthand", () => {
  assert.equal(hasManagedCoverageName("pkg/trading/coverage_98_test.go"), true);
  assert.equal(hasManagedCoverageName("pkg/trading/order_coverage_test.go"), true);
  assert.equal(hasManagedCoverageName("apps/web/tests/AccountPageCoverage.test.ts"), true);
  assert.equal(hasManagedCoverageName("workers/pineworker/coveragePolicy.test.ts"), true);
  assert.equal(hasManagedCoverageName("apps/web/tests/order-c95.spec.ts"), true);
  assert.equal(hasManagedCoverageName("apps/web/tests/order_c_98.spec.ts"), true);
  assert.equal(hasManagedCoverageName("apps/web/tests/order-risk.spec.ts"), false);
  assert.equal(hasManagedCoverageName("apps/web/tests/rfc9110.spec.ts"), false);
});

test("parses comments and blank lines out of the legacy allowlist", () => {
  assert.deepEqual(
    [...parseAllowlist("# legacy\npkg/a/c98_test.go\n\n pkg/b/coverage_95_test.go \n")],
    ["pkg/a/c98_test.go", "pkg/b/coverage_95_test.go"],
  );
});

test("accepts an exact allowlist that only shrinks from the base", () => {
  const violations = ["pkg/a/c98_test.go"];
  const allowlist = new Set(violations);
  const base = new Set([...violations, "pkg/b/coverage_95_test.go"]);
  assert.deepEqual(comparePolicyState(violations, allowlist, base), {
    unallowlisted: [], stale: [], growth: [],
  });
});

test("reports new filenames, stale entries, and allowlist growth independently", () => {
  const result = comparePolicyState(
    ["pkg/new/c98_test.go"],
    new Set(["pkg/stale/coverage_95_test.go"]),
    new Set(["pkg/old/c98_test.go"]),
  );
  assert.deepEqual(result, {
    unallowlisted: ["pkg/new/c98_test.go"],
    stale: ["pkg/stale/coverage_95_test.go"],
    growth: ["pkg/stale/coverage_95_test.go"],
  });
});

test("derives the allowlist baseline from the merge-base tree", (t) => {
  const repo = temporaryRepository(t);
  write(repo, "pkg/legacy/behavior_coverage_test.go", "package legacy\n");
  commitAll(repo, "legacy tests");
  const base = git(repo, ["rev-parse", "HEAD"]).trim();

  write(repo, "scripts/test-name-allowlist.txt", "pkg/legacy/behavior_coverage_test.go\n");
  const result = runPolicy(repo, base);

  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /baseline derived from the .* tree/);
});

test("ignores an incomplete legacy allowlist and derives the current rule from the base tree", (t) => {
  const repo = temporaryRepository(t);
  write(repo, "pkg/legacy/behavior_coverage_test.go", "package legacy\n");
  write(repo, "scripts/test-name-allowlist.txt", "# Older policy did not track this file.\n");
  commitAll(repo, "legacy policy");
  const base = git(repo, ["rev-parse", "HEAD"]).trim();

  write(repo, "scripts/test-name-allowlist.txt", "pkg/legacy/behavior_coverage_test.go\n");
  const result = runPolicy(repo, base);

  assert.equal(result.status, 0, result.stderr);
});

test("rejects a new coverage test even when the allowlist adds it", (t) => {
  const repo = temporaryRepository(t);
  write(repo, "pkg/legacy/coverage_95_test.go", "package legacy\n");
  commitAll(repo, "legacy tests");
  const base = git(repo, ["rev-parse", "HEAD"]).trim();

  write(
    repo,
    "scripts/test-name-allowlist.txt",
    "pkg/legacy/coverage_95_test.go\npkg/new/OrderFlowCoverage.test.ts\n",
  );
  write(repo, "pkg/new/OrderFlowCoverage.test.ts", "export {};\n");
  const result = runPolicy(repo, base);

  assert.equal(result.status, 1);
  assert.match(result.stderr, /allowlist may only shrink/);
  assert.match(result.stderr, /pkg\/new\/OrderFlowCoverage\.test\.ts/);
});

test("fails closed when Git cannot resolve the requested base", (t) => {
  const repo = temporaryRepository(t);
  write(repo, "scripts/test-name-allowlist.txt", "");
  commitAll(repo, "policy");

  const result = runPolicy(repo, "missing-ref");

  assert.equal(result.status, 1);
  assert.match(result.stderr, /unable to resolve merge base/);
});

function temporaryRepository(t) {
  const repo = mkdtempSync(join(tmpdir(), "jftrade-test-names-"));
  t.after(() => rmSync(repo, { recursive: true, force: true }));
  git(repo, ["init", "-q"]);
  git(repo, ["config", "user.email", "test@example.com"]);
  git(repo, ["config", "user.name", "Test"]);
  return repo;
}

function write(repo, path, contents) {
  const target = join(repo, path);
  mkdirSync(dirname(target), { recursive: true });
  writeFileSync(target, contents);
}

function commitAll(repo, message) {
  git(repo, ["add", "."]);
  git(repo, ["commit", "-q", "-m", message]);
}

function git(repo, args) {
  return execFileSync("git", args, {
    cwd: repo,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  });
}

function runPolicy(repo, base) {
  return spawnSync(
    process.execPath,
    [scriptPath, "--repo-root", repo, "--base", base],
    {
      cwd: repo,
      encoding: "utf8",
      env: { ...process.env, JFTRADE_DIFF_BASE: "" },
    },
  );
}
