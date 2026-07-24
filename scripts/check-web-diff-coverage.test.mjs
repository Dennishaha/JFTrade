#!/usr/bin/env node
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { checkWebDiffCoverage, formatWebDiffCoverageReport, isCriticalWebPath, parseArguments } from "./check-web-diff-coverage.mjs";

const tempRoot = mkdtempSync(join(tmpdir(), "jftrade-web-diff-coverage-"));
try {
  setupRepository(tempRoot);
  const baseRef = git(tempRoot, ["rev-parse", "HEAD"]);
  writeChangedWebSources(tempRoot);

  writeCoverage(tempRoot, { ordinaryStatementHits: 9, criticalStatementHits: 19 });
  const passing = checkWebDiffCoverage({ baseRef, repoRoot: tempRoot });
  assert.equal(passing.passed, true, formatWebDiffCoverageReport(passing));
  assert.equal(passing.reports.length, 2);
  assert.equal(passing.reports.find((report) => report.kind === "critical")?.branches.percentage, (20 / 22) * 100);
  assert.equal(isCriticalWebPath("apps/web/src/components/risk/HardStopControlPanel.vue"), true);
  assert.equal(isCriticalWebPath("apps/web/src/components/SettingsAppearanceSection.vue"), false);

  writeCoverage(tempRoot, { ordinaryStatementHits: 8, criticalStatementHits: 19 });
  const ordinaryFailure = checkWebDiffCoverage({ baseRef, repoRoot: tempRoot });
  assert.equal(ordinaryFailure.passed, false);
  assert.match(formatWebDiffCoverageReport(ordinaryFailure), /ordinary .*ordinary\.vue: statements 8\/10 \(80\.00%, required 90%\)/);

  writeCoverage(tempRoot, { ordinaryStatementHits: 9, criticalStatementHits: 19, omitCritical: true });
  const missingCoverage = checkWebDiffCoverage({ baseRef, repoRoot: tempRoot });
  assert.equal(missingCoverage.passed, false);
  assert.match(formatWebDiffCoverageReport(missingCoverage), /missing coverage entry/);

  writeCoverage(tempRoot, { ordinaryStatementHits: 9, criticalStatementHits: 19 });
  writeFileSync(join(tempRoot, "apps/web/src/untracked.ts"), "export const untracked = 1;\n");
  const untrackedFailure = checkWebDiffCoverage({ baseRef, repoRoot: tempRoot });
  assert.equal(untrackedFailure.passed, false);
  assert.match(formatWebDiffCoverageReport(untrackedFailure), /ordinary apps\/web\/src\/untracked\.ts: missing coverage entry/);

  assert.deepEqual(parseArguments(["--base", "origin/main", "--coverage", "report.json"]), {
    baseRef: "origin/main",
    coveragePath: "report.json",
    repoRoot: undefined,
  });
  assert.throws(() => parseArguments([]), /--base <git-ref> is required/);
  assert.throws(() => parseArguments(["--base"]), /requires a value/);
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

function setupRepository(root) {
  mkdirSync(join(root, "apps/web/src/components/risk"), { recursive: true });
  writeFileSync(
    join(root, "apps/web/src/ordinary.vue"),
    "<script setup lang=\"ts\">\nfunction ordinary(value: number) {\n  return value;\n}\n</script>\n<template>\n  <option :value=\"0\">Zero</option>\n</template>\n",
  );
  writeFileSync(join(root, "apps/web/src/components/risk/guard.ts"), "export function guard(value: number) {\n  return value;\n}\n");
  git(root, ["init"]);
  git(root, ["config", "user.email", "coverage@example.test"]);
  git(root, ["config", "user.name", "Coverage Test"]);
  git(root, ["add", "."]);
  git(root, ["commit", "-m", "base"]);
}

function writeChangedWebSources(root) {
  writeFileSync(
    join(root, "apps/web/src/ordinary.vue"),
    "<script setup lang=\"ts\">\nfunction ordinary(value: number) {\n  const next = value > 0 ? value : 0;\n  return next;\n}\n</script>\n<template>\n  <option :value=\"0\">Default</option>\n</template>\n",
  );
  writeFileSync(join(root, "apps/web/src/components/risk/guard.ts"), "export function guard(value: number) {\n  return value > 0 ? value : 0;\n}\n");
}

function writeCoverage(root, { ordinaryStatementHits, criticalStatementHits, omitCritical = false }) {
  const coverageDirectory = join(root, "apps/web/coverage");
  mkdirSync(coverageDirectory, { recursive: true });
  const coverage = {
    [join(root, "apps/web/src/ordinary.vue")]: coverageEntry(ordinaryStatementHits, 17, 3, 8),
  };
  if (!omitCritical) {
    coverage[join(root, "apps/web/src/components/risk/guard.ts")] = coverageEntry(criticalStatementHits, 18);
  }
  writeFileSync(join(coverageDirectory, "coverage-final.json"), JSON.stringify(coverage));
}

function coverageEntry(statementHits, branchHits, sourceLine = 2, generatedBindingLine) {
  const statementMap = {};
  const statements = {};
  for (let index = 0; index < 10; index++) {
    statementMap[index] = location(sourceLine);
    statements[index] = index < statementHits ? 1 : 0;
  }
  const branchMap = {};
  const branches = {};
  for (let index = 0; index < 20; index++) {
    branchMap[index] = { type: "if", line: sourceLine, loc: location(sourceLine), locations: [location(sourceLine)] };
    branches[index] = [index < branchHits ? 1 : 0];
  }
  const collapsedLocation = location(sourceLine);
  branchMap[20] = {
    type: "cond-expr",
    line: sourceLine,
    loc: collapsedLocation,
    locations: [collapsedLocation, collapsedLocation],
  };
  branches[20] = [0, 0];
  branchMap[21] = {
    type: "cond-expr",
    line: 2,
    loc: locationSpan(sourceLine, 0, 2),
    locations: [locationSpan(sourceLine, 0, 1), locationSpan(sourceLine, 1, 2)],
  };
  branches[21] = [1, 1];
  const degradedLocation = { start: { line: sourceLine, column: 0 }, end: { line: sourceLine, column: null } };
  branchMap[22] = {
    type: "if",
    line: sourceLine,
    loc: degradedLocation,
    locations: [degradedLocation],
  };
  branches[22] = [0];
  if (generatedBindingLine !== undefined) {
    const generatedLocation = {
      start: { line: generatedBindingLine, column: 2 },
      end: { line: generatedBindingLine, column: null },
    };
    branchMap[23] = {
      type: "cond-expr",
      line: generatedBindingLine,
      loc: generatedLocation,
      locations: [
        { start: { line: generatedBindingLine, column: 9 }, end: { line: generatedBindingLine, column: null } },
        generatedLocation,
      ],
    };
    branches[23] = [0, 0];
  }
  return { statementMap, s: statements, branchMap, b: branches, fnMap: {}, f: {} };
}

function location(line) {
  return { start: { line, column: 0 }, end: { line, column: 1 } };
}

function locationSpan(line, startColumn, endColumn) {
  return { start: { line, column: startColumn }, end: { line, column: endColumn } };
}

function git(cwd, args) {
  return execFileSync("git", args, { cwd, encoding: "utf8" }).trim();
}
