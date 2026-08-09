import assert from "node:assert/strict";
import test from "node:test";

import {
  planAffected,
  resolveAffectedModules,
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
    [...resolveFallbackChecks(["go.mod", "apps/web/src/generated/openapi.ts", "scripts/test-affected.mjs"])].sort(),
    ["generated", "go", "scripts", "web"],
  );
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
  ]), [
    "pnpm --filter @jftrade/web exec vitest run 'tests/pages/StrategyPage.test.ts'",
    "pnpm --filter @jftrade/web exec vitest related --run 'src/features/strategy.ts'",
  ]);
});
