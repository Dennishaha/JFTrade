#!/usr/bin/env node
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { isAbsolute, relative, resolve } from "node:path";

const webSourcePrefix = "apps/web/src/";
const coveragePolicy = JSON.parse(
  readFileSync(fileURLToPath(new URL("../apps/web/coverage-policy.json", import.meta.url)), "utf8"),
);

export const webDiffCoverageThresholds = Object.freeze({
  ordinary: Object.freeze({
    statements: coveragePolicy.globalThresholds.statements,
    branches: coveragePolicy.globalThresholds.branches,
  }),
  critical: Object.freeze({
    statements: coveragePolicy.criticalThresholds.statements,
    branches: coveragePolicy.criticalThresholds.branches,
  }),
});

// Keep this list focused on code that can place/confirm orders, manage risk,
// carry a live market feed, or execute/render a backtest. It intentionally
// lives beside the diff gate so future changes cannot lower its protection by
// being moved out of a broad directory glob.
// BacktestPage and useBacktestRuns remain diff-critical until their existing
// branch baseline is raised through a dedicated business-scenario expansion.
// This preserves strict coverage for newly changed backtest behavior without
// turning the global test command into a knowingly failing static gate.
const criticalWebFiles = new Set([
  ...coveragePolicy.criticalExactPaths,
  ...coveragePolicy.diffOnlyCriticalExactPaths,
]);
const criticalWebPrefixes = coveragePolicy.criticalPrefixes;

export function parseArguments(argv) {
  const options = {
    baseRef: undefined,
    coveragePath: undefined,
    repoRoot: undefined,
  };

  for (let index = 0; index < argv.length; index++) {
    const arg = argv[index];
    if (arg === "--base" || arg === "--coverage" || arg === "--repo-root") {
      const value = argv[index + 1];
      if (value === undefined || value.startsWith("--")) {
        throw new Error(`${arg} requires a value`);
      }
      index++;
      if (arg === "--base") options.baseRef = value;
      if (arg === "--coverage") options.coveragePath = value;
      if (arg === "--repo-root") options.repoRoot = value;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }

  if (!options.baseRef) {
    throw new Error("--base <git-ref> is required");
  }
  return options;
}

export function checkWebDiffCoverage({ baseRef, coveragePath, repoRoot = process.cwd() }) {
  if (!baseRef) {
    throw new Error("baseRef is required");
  }
  const gitRoot = gitRepositoryRoot(repoRoot);
  const resolvedCoveragePath = resolve(gitRoot, coveragePath ?? "apps/web/coverage/coverage-final.json");
  const changedFiles = changedWebSourceLines(gitRoot, baseRef);
  if (changedFiles.size === 0) {
    return {
      baseRef,
      coveragePath: resolvedCoveragePath,
      reports: [],
      passed: true,
    };
  }
  const coverage = JSON.parse(readFileSync(resolvedCoveragePath, "utf8"));
  const reports = [];

  for (const [path, changedLines] of changedFiles) {
    const coverageEntry = coverageEntryForPath(coverage, gitRoot, path);
    const kind = isCriticalWebPath(path) ? "critical" : "ordinary";
    const thresholds = webDiffCoverageThresholds[kind];

    if (coverageEntry === undefined) {
      if (hasLikelyExecutableChange(gitRoot, path, changedLines)) {
        reports.push({
          path,
          kind,
          missingCoverage: true,
          statements: { hit: 0, total: 0, percentage: 0, threshold: thresholds.statements },
          branches: { hit: 0, total: 0, percentage: 0, threshold: thresholds.branches },
          passed: false,
        });
      }
      continue;
    }

    const statements = selectedStatements(coverageEntry, changedLines);
    const sourceLines = readFileSync(resolve(gitRoot, path), "utf8").split("\n");
    const branches = selectedBranches(coverageEntry, changedLines, sourceLines);
    const statementSummary = coverageSummary(statements, thresholds.statements);
    const branchSummary = coverageSummary(branches, thresholds.branches);

    reports.push({
      path,
      kind,
      missingCoverage: false,
      statements: statementSummary,
      branches: branchSummary,
      passed: statementSummary.percentage >= statementSummary.threshold && branchSummary.percentage >= branchSummary.threshold,
    });
  }

  return {
    baseRef,
    coveragePath: resolvedCoveragePath,
    reports,
    passed: reports.every((report) => report.passed),
  };
}

export function isCriticalWebPath(path) {
  const normalized = normalizePath(path);
  return criticalWebFiles.has(normalized) || criticalWebPrefixes.some((prefix) => normalized.startsWith(prefix));
}

function gitRepositoryRoot(cwd) {
  return runGit(cwd, ["rev-parse", "--show-toplevel"]).trim();
}

function changedWebSourceLines(repoRoot, baseRef) {
  runGit(repoRoot, ["rev-parse", "--verify", `${baseRef}^{commit}`]);
  // Compare the PR merge-base to the working tree rather than to HEAD. This
  // makes the local command gate staged and unstaged changes too, while a CI
  // checkout still naturally evaluates the checked-out commit.
  const comparisonBase = runGit(repoRoot, ["merge-base", baseRef, "HEAD"]).trim();
  const names = runGit(repoRoot, [
    "diff",
    "--name-only",
    "--diff-filter=ACMR",
    "--find-renames",
    comparisonBase,
    "--",
    webSourcePrefix,
  ]).split("\n").filter((path) => isWebSourceFile(path));
  const untrackedNames = new Set(runGit(repoRoot, [
    "ls-files",
    "--others",
    "--exclude-standard",
    "--",
    webSourcePrefix,
  ]).split("\n").filter((path) => isWebSourceFile(path)));
  const sourceNames = new Set([...names, ...untrackedNames]);
  const changed = new Map();

  for (const path of sourceNames) {
    const diff = runGit(repoRoot, ["diff", "--no-ext-diff", "--unified=0", "--find-renames", comparisonBase, "--", path]);
    const lines = untrackedNames.has(path)
      ? allLineNumbers(repoRoot, path)
      : addedLineNumbers(diff);
    if (lines.size > 0) {
      changed.set(normalizePath(path), lines);
    }
  }
  return changed;
}

function allLineNumbers(repoRoot, path) {
  const lineCount = readFileSync(resolve(repoRoot, path), "utf8").split("\n").length;
  return new Set(Array.from({ length: lineCount }, (_value, index) => index + 1));
}

function runGit(cwd, args) {
  const result = spawnSync("git", args, { cwd, encoding: "utf8" });
  if (result.error) {
    throw new Error(`could not run git ${args.join(" ")}: ${result.error.message}`);
  }
  if (result.status !== 0) {
    throw new Error((result.stderr || result.stdout || `git ${args.join(" ")} failed`).trim());
  }
  return result.stdout;
}

function isWebSourceFile(path) {
  const normalized = normalizePath(path);
  return normalized.startsWith(webSourcePrefix) && /\.(?:ts|tsx|vue)$/.test(normalized) && !normalized.endsWith(".d.ts");
}

function addedLineNumbers(diff) {
  const lines = new Set();
  for (const line of diff.split("\n")) {
    const hunk = /^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@/.exec(line);
    if (!hunk) continue;
    const start = Number(hunk[1]);
    const count = hunk[2] === undefined ? 1 : Number(hunk[2]);
    for (let offset = 0; offset < count; offset++) {
      lines.add(start + offset);
    }
  }
  return lines;
}

function coverageEntryForPath(coverage, repoRoot, path) {
  const normalizedPath = normalizePath(path);
  for (const [coveragePath, entry] of Object.entries(coverage)) {
    const normalizedCoveragePath = relativeCoveragePath(repoRoot, coveragePath);
    if (normalizedCoveragePath === normalizedPath) {
      return entry;
    }
  }
  // macOS can spell temporary paths as both /var/... and /private/var/...;
  // retain a suffix fallback so a canonicalized Git root still finds V8's
  // absolute source path without weakening the source-root restriction.
  const suffix = `/${normalizedPath}`;
  for (const [coveragePath, entry] of Object.entries(coverage)) {
    if (normalizePath(coveragePath.replace(/^file:\/\//, "")).endsWith(suffix)) {
      return entry;
    }
  }
  return undefined;
}

function relativeCoveragePath(repoRoot, coveragePath) {
  const normalized = normalizePath(coveragePath.replace(/^file:\/\//, ""));
  if (!isAbsolute(normalized)) {
    return normalized.replace(/^\.\//, "");
  }
  return normalizePath(relative(repoRoot, normalized));
}

function selectedStatements(entry, changedLines) {
  const selected = [];
  for (const [id, location] of Object.entries(entry.statementMap ?? {})) {
    if (locationTouchesChangedLine(location, changedLines)) {
      selected.push(Number(entry.s?.[id] ?? 0));
    }
  }
  return selected;
}

function selectedBranches(entry, changedLines, sourceLines) {
  const selected = [];
  const concreteHitLines = new Set(
    Object.entries(entry.branchMap ?? {})
      .filter(([id, branch]) => branchHasConcreteSpan(branch) && (entry.b?.[id] ?? []).some((hit) => Number(hit) > 0))
      .map(([, branch]) => branchStartLine(branch))
      .filter((line) => line !== undefined),
  );
  for (const [id, branch] of Object.entries(entry.branchMap ?? {})) {
    const locations = Array.isArray(branch.locations) ? branch.locations : [];
    // Vue's generated render functions can be compiled more than once in a
    // full V8 coverage run. When those reports are merged, some generated
    // conditional expressions collapse both alternatives onto the exact same
    // source span. Such entries cannot identify an uncovered source branch and
    // otherwise create zero-hit phantom branches in the diff gate.
    if (branch.type === "cond-expr" && locationsCollapseToOneSpan(locations)) {
      continue;
    }
    // Merged V8 reports can also retain a second, zero-hit copy of a Vue
    // branch after losing all source-map end columns. A concrete, hit-bearing
    // branch on the same source line is the usable record; the degraded copy
    // is not a separately identifiable branch.
    const hits = entry.b?.[id] ?? [];
    const sourceLine = sourceLines[Number(branchStartLine(branch) ?? 0) - 1] ?? "";
    const isStaticVueBinding = /^\s*<[\w-]+\b[^>]*:[\w-]+(?:\.\w+)*\s*=\s*["'](?:-?\d+(?:\.\d+)?|true|false|null)["']/.test(
      sourceLine,
    );
    // Vue can map static bound literals such as :value="0" to generated
    // conditional expressions. With no concrete alternative spans, these are
    // not source branches and cannot be made executable through a test.
    if (
      branch.type === "cond-expr"
      && hits.length > 0
      && hits.every((hit) => Number(hit) === 0)
      && !branchHasConcreteSpan(branch)
      && isStaticVueBinding
    ) {
      continue;
    }
    if (
      hits.length > 0
      && hits.every((hit) => Number(hit) === 0)
      && !branchHasConcreteSpan(branch)
      && concreteHitLines.has(branchStartLine(branch))
    ) {
      continue;
    }
    const touchesChange = locationTouchesChangedLine(branch.loc, changedLines) || locations.some((location) =>
      locationTouchesChangedLine(location, changedLines),
    );
    if (!touchesChange) continue;
    for (const hit of hits) {
      selected.push(Number(hit));
    }
  }
  return selected;
}

function branchStartLine(branch) {
  return branch.loc?.start?.line ?? branch.locations?.[0]?.start?.line;
}

function branchHasConcreteSpan(branch) {
  const locations = [branch.loc, ...(Array.isArray(branch.locations) ? branch.locations : [])].filter(Boolean);
  return locations.some((location) => Number.isInteger(location?.end?.column));
}

function locationsCollapseToOneSpan(locations) {
  if (locations.length < 2) return false;
  const spans = locations.map((location) =>
    [
      location?.start?.line ?? null,
      location?.start?.column ?? null,
      location?.end?.line ?? null,
      location?.end?.column ?? null,
    ].join(":"),
  );
  return new Set(spans).size === 1;
}

function locationTouchesChangedLine(location, changedLines) {
  if (!location?.start?.line) return false;
  const start = location.start.line;
  const end = location.end?.line ?? start;
  for (const line of changedLines) {
    if (line >= start && line <= end) return true;
  }
  return false;
}

function coverageSummary(values, threshold) {
  const total = values.length;
  const hit = values.filter((value) => value > 0).length;
  return {
    hit,
    total,
    percentage: total === 0 ? 100 : (hit / total) * 100,
    threshold,
  };
}

function hasLikelyExecutableChange(repoRoot, path, changedLines) {
  const source = readFileSync(resolve(repoRoot, path), "utf8").split("\n");
  return [...changedLines].some((lineNumber) => isLikelyExecutableSourceLine(source[lineNumber - 1] ?? ""));
}

function isLikelyExecutableSourceLine(line) {
  const trimmed = line.trim();
  if (trimmed === "" || trimmed.startsWith("//") || trimmed.startsWith("/*") || trimmed.startsWith("*") || trimmed.startsWith("<")) {
    return false;
  }
  if (/^(?:export\s+)?(?:type|interface|declare|namespace)\b/.test(trimmed) || /^import\s+type\b/.test(trimmed)) {
    return false;
  }
  if (/^[{};,]+$/.test(trimmed) || /^export\s*\{\s*type\b/.test(trimmed)) {
    return false;
  }
  return true;
}

function normalizePath(path) {
  return path.replaceAll("\\", "/").replace(/^\.\//, "");
}

export function formatWebDiffCoverageReport(result) {
  if (result.reports.length === 0) {
    return `Web diff coverage: no executable source changes relative to ${result.baseRef}.`;
  }
  const lines = [`Web diff coverage relative to ${result.baseRef}:`];
  for (const report of result.reports) {
    if (report.missingCoverage) {
      lines.push(`- ${report.kind} ${report.path}: missing coverage entry for executable changed code`);
      continue;
    }
    lines.push(
      `- ${report.kind} ${report.path}: statements ${formatMetric(report.statements)}, branches ${formatMetric(report.branches)}`,
    );
  }
  return lines.join("\n");
}

function formatMetric(metric) {
  return `${metric.hit}/${metric.total} (${metric.percentage.toFixed(2)}%, required ${metric.threshold}%)`;
}

function main() {
  try {
    const options = parseArguments(process.argv.slice(2));
    const result = checkWebDiffCoverage(options);
    console.log(formatWebDiffCoverageReport(result));
    if (!result.passed) {
      process.exitCode = 1;
    }
  } catch (error) {
    console.error(`Web diff coverage check failed: ${error instanceof Error ? error.message : String(error)}`);
    process.exitCode = 1;
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main();
}
