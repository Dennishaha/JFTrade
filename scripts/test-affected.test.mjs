import assert from "node:assert/strict";
import test from "node:test";

import {
  changedFiles,
  planAffected,
  resolveAffectedModules,
  resolveBase,
  resolveFallbackChecks,
  webAffectedTestCommands,
} from "./test-affected.mjs";

test("maps changed files to the narrowest declared modules", () => {
  const modules = resolveAffectedModules([
    "internal/marketdata/service.go",
    "apps/web/src/composables/backtest/useBacktestPage.ts",
  ]);
  assert.deepEqual(modules.map((module) => module.id), ["marketdata", "strategy-backtest", "web"]);
});

test("classifies language and generated fallback checks", () => {
  assert.deepEqual(
    [...resolveFallbackChecks([
      "go.mod",
      "apps/web/src/generated/openapi.ts",
      "scripts/test-affected.mjs",
      ".github/workflows/ci.yml",
    ])].sort(),
    ["generated", "go", "scripts", "web", "workflows"],
  );
});

test("uses the merge-base for a diverged branch", () => {
  const calls = [];
  const fakeGit = (_root, args) => {
    calls.push(args);
    if (args[0] === "rev-parse") {
      return "feature-base";
    }
    if (args[0] === "merge-base") {
      return "merge-base-commit";
    }
    return "";
  };

  assert.equal(resolveBase("/tmp/repo", {
    env: { JFTRADE_DIFF_BASE: "feature-base" },
    gitCommand: fakeGit,
  }), "merge-base-commit");
  assert.deepEqual(calls, [
    ["rev-parse", "--verify", "feature-base"],
    ["merge-base", "HEAD", "feature-base"],
  ]);
});

test("returns no files for a clean checkout without probing a fixture repository", () => {
  const calls = [];
  const fakeGit = (_root, args) => {
    calls.push(args);
    if (args[0] === "rev-parse") return "origin-main";
    if (args[0] === "merge-base") return "merge-base";
    return "";
  };

  assert.deepEqual(changedFiles("/tmp/repo", undefined, { gitCommand: fakeGit }), []);
  assert.deepEqual(calls, [
    ["rev-parse", "--verify", "origin/main"],
    ["merge-base", "HEAD", "origin/main"],
    ["diff", "--name-only", "--diff-filter=ACMRD", "merge-base"],
    ["ls-files", "--others", "--exclude-standard", "-z"],
  ]);
});

test("builds a deterministic affected test plan", () => {
  const plan = planAffected(["workers/pineworker/src/pinetsExecutor.ts"]);
  assert.equal(plan.modules[0].id, "pineworker");
  assert.deepEqual(plan.commands, [
    "pnpm --filter @jftrade/pineworker run test",
    "go test ./pkg/strategy/pineworker -count=1",
  ]);
});

test("uses the Vitest dependency graph for changed Web sources", () => {
  assert.deepEqual(webAffectedTestCommands([
    "apps/web/src/features/strategy.ts",
    "apps/web/tests/pages/StrategyPage.test.ts",
    "docs/strategy.md",
  ], { fileExists: () => true }), [
    "pnpm --filter @jftrade/web exec vitest run 'tests/pages/StrategyPage.test.ts'",
    "pnpm --filter @jftrade/web exec vitest related --run 'src/features/strategy.ts'",
  ]);
});

test("classifies deleted Web tests but does not execute their missing path", () => {
  assert.deepEqual(
    webAffectedTestCommands(["apps/web/tests/pages/DeletedPage.test.ts"], {
      fileExists: () => false,
    }),
    [],
  );
  assert.equal(resolveAffectedModules(["apps/web/tests/pages/DeletedPage.test.ts"])[0].id, "web");
});

test("runs actionlint only for changed GitHub workflow files", () => {
  const scriptPlan = planAffected(["scripts/check-diff.mjs"], { withChecks: true });
  assert.equal(scriptPlan.commands.includes("pnpm run check:actionlint"), false);

  const workflowPlan = planAffected([".github/workflows/ci.yml"], { withChecks: true });
  assert.equal(workflowPlan.commands.includes("pnpm run check:actionlint"), true);
});
