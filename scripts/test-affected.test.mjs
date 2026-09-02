import assert from "node:assert/strict";
import test from "node:test";

import {
  changedFiles,
  goAffectedTestCommands,
  planAffected,
  resolveAffectedModules,
  resolveBase,
  resolveFallbackChecks,
  rustAffectedClippyCommands,
  rustAffectedTestCommands,
  webAffectedTestCommands,
} from "./test-affected.mjs";

const goPackageFixture = [
  {
    Dir: "/repo/internal/productfeatures",
    ImportPath: "github.com/jftrade/jftrade-main/internal/productfeatures",
    Module: { Main: true, Path: "github.com/jftrade/jftrade-main" },
  },
  {
    Dir: "/repo/internal/research",
    ImportPath: "github.com/jftrade/jftrade-main/internal/research",
    Module: { Main: true, Path: "github.com/jftrade/jftrade-main" },
  },
  {
    Dir: "/repo/internal/integration/akshare",
    ImportPath: "github.com/jftrade/jftrade-main/internal/integration/akshare",
    Module: { Main: true, Path: "github.com/jftrade/jftrade-main" },
  },
  {
    Dir: "/repo/pkg/bbgo/types",
    ImportPath: "github.com/jftrade/jftrade-main/pkg/bbgo/types",
    Module: { Main: true, Path: "github.com/jftrade/jftrade-main" },
  },
  {
    Dir: "/repo/internal/api/productfeatures",
    ImportPath: "github.com/jftrade/jftrade-main/internal/api/productfeatures",
    Deps: ["github.com/jftrade/jftrade-main/internal/productfeatures"],
    Module: { Main: true, Path: "github.com/jftrade/jftrade-main" },
  },
  {
    Dir: "/repo/internal/research",
    ImportPath: "github.com/jftrade/jftrade-main/internal/research [github.com/jftrade/jftrade-main/internal/research.test]",
    ForTest: "github.com/jftrade/jftrade-main/internal/research",
    Deps: ["github.com/jftrade/jftrade-main/internal/productfeatures"],
    Module: { Main: true, Path: "github.com/jftrade/jftrade-main" },
  },
];

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
      "crates/jftrade-engine/src/lib.rs",
      "apps/web/src/generated/openapi.ts",
      "scripts/test-affected.mjs",
      ".github/workflows/ci.yml",
    ])].sort(),
    ["generated", "go", "rust", "scripts", "web", "workflows"],
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

  assert.deepEqual(changedFiles("/tmp/repo", undefined, { env: {}, gitCommand: fakeGit }), []);
  assert.deepEqual(calls, [
    ["rev-parse", "--verify", "origin/main"],
    ["merge-base", "HEAD", "origin/main"],
    ["diff", "--name-only", "--diff-filter=ACMRD", "merge-base"],
    ["ls-files", "--others", "--exclude-standard", "-z"],
  ]);
});

test("uses HEAD as the explicit worktree comparison base", () => {
  const calls = [];
  const fakeGit = (_root, args) => {
    calls.push(args);
    return "";
  };

  assert.deepEqual(changedFiles("/tmp/repo", "HEAD", { gitCommand: fakeGit }), []);
  assert.deepEqual(calls, [
    ["diff", "--name-only", "--diff-filter=ACMRD", "HEAD"],
    ["ls-files", "--others", "--exclude-standard", "-z"],
  ]);
});

test("builds a deterministic affected test plan", () => {
  const plan = planAffected(["workers/pineworker/src/pinetsExecutor.ts"]);
  assert.equal(plan.modules[0].id, "pineworker");
  assert.deepEqual(plan.commands, [
    "pnpm --filter @jftrade/pineworker run test",
    "pnpm --filter @jftrade/pineworker run typecheck",
  ]);
});

test("runs Rust tests and quality gates for migration engine changes", () => {
  const plan = planAffected(["crates/jftrade-engine/src/lib.rs"], { withChecks: true });
  assert.deepEqual(plan.modules.map((module) => module.id), ["rust-foundation"]);
  assert.deepEqual(plan.commands, [
    "pnpm run check:diff",
    "pnpm run check:ai-context",
    "pnpm run check:go-retirement",
    "pnpm run check:rust:layout",
    "node --test scripts/rust-migration/check-stage9-closeout.test.mjs scripts/rust-migration/stage9-route-ownership.test.mjs",
    "pnpm run test:rust:stage9:route-coverage",
    "pnpm run check:rust:target-health",
    "cargo test -p jftrade-desktop -p jftrade-engine --all-targets",
    "pnpm run format:rust:check",
    "cargo clippy -p jftrade-desktop -p jftrade-engine --all-targets --all-features -- -D warnings",
  ]);
});

test("quick Rust plan selects the changed package and defers the full integration gate", () => {
  const plan = planAffected(["crates/jftrade-engine/src/lib.rs"], {
    withChecks: true,
    profile: "quick",
  });
  assert.deepEqual(plan.commands, [
    "pnpm run check:diff",
    "pnpm run check:ai-context",
    "pnpm run check:go-retirement",
    "pnpm run check:rust:layout",
    "pnpm run check:rust:target-health",
    "cargo test -p jftrade-desktop -p jftrade-engine --all-targets",
    "pnpm run format:rust:check",
    "cargo clippy -p jftrade-desktop -p jftrade-engine --all-targets --all-features -- -D warnings",
  ]);
  assert.deepEqual(plan.deferredCommands, ["pnpm run check:rust"]);
});

test("quick Stage 9 ledger plan runs ownership gates without the product differential", () => {
  const plan = planAffected([
    "tests/fixtures/rust-migration/stage9/ledgers/research-preset-read.md",
  ], { profile: "quick" });
  assert.deepEqual(plan.commands, [
    "pnpm run check:rust:layout",
    "node --test scripts/rust-migration/check-stage9-closeout.test.mjs scripts/rust-migration/stage9-route-ownership.test.mjs",
    "pnpm run test:rust:stage9:route-coverage",
  ]);
  assert.equal(plan.commands.includes("pnpm run test:rust:stage9:product-differential"), false);
  assert.deepEqual(plan.deferredCommands, ["pnpm run check:rust"]);
});

test("full Stage 9 product changes replace overlapping group differentials", () => {
  const plan = planAffected([
    "scripts/rust-migration/check-stage9-product-differential.mjs",
    "scripts/rust-migration/check-stage9-watchlist-write.mjs",
  ], { profile: "full" });
  assert.equal(
    plan.commands.filter((command) => command === "pnpm run test:rust:stage9:product-differential").length,
    1,
  );
  assert.equal(plan.commands.includes("node scripts/rust-migration/check-stage9-watchlist-write.mjs"), false);
  assert.equal(plan.commands.includes("pnpm run check:rust:target-health"), true);
});

test("Rust affected commands fall back to the workspace for shared manifests", () => {
  assert.deepEqual(rustAffectedTestCommands(["Cargo.lock"]), ["pnpm run test:rust"]);
  assert.deepEqual(rustAffectedClippyCommands(["rust-toolchain.toml"]), ["pnpm run lint:rust"]);
  assert.deepEqual(rustAffectedTestCommands(["crates/jftrade-calendar/src/lib.rs"]), [
    "cargo test -p jftrade-calendar -p jftrade-desktop -p jftrade-engine --all-targets",
  ]);
  assert.deepEqual(rustAffectedTestCommands(["crates/jftrade-engine/tests/stage9_alerts.rs"]), [
    "cargo test -p jftrade-engine --all-targets",
  ]);
  assert.deepEqual(rustAffectedTestCommands(["crates/jftrade-kernel/src/lib.rs"]), [
    "pnpm run test:rust",
  ]);
});

test("rejects an unknown affected-test profile", () => {
  assert.throws(
    () => planAffected(["crates/jftrade-engine/src/lib.rs"], { profile: "slow" }),
    /unknown affected-test profile/,
  );
});

test("selects changed Go packages and their production and test dependents", () => {
  assert.deepEqual(goAffectedTestCommands(["internal/productfeatures/service.go"], {
    root: "/repo",
    packages: goPackageFixture,
    fileExists: () => true,
  }), [
    "go test -p=4 github.com/jftrade/jftrade-main/internal/api/productfeatures github.com/jftrade/jftrade-main/internal/productfeatures github.com/jftrade/jftrade-main/internal/research -count=1 -timeout 300s",
  ]);
});

test("always selects real tests for previously unclassified Go source families", () => {
  for (const file of [
    "internal/productfeatures/service.go",
    "internal/research/presets.go",
    "internal/integration/akshare/client.go",
    "pkg/bbgo/types/order.go",
  ]) {
    const commands = goAffectedTestCommands([file], {
      root: "/repo",
      packages: goPackageFixture,
      fileExists: () => true,
    });
    assert.equal(commands.length, 1, file);
    assert.match(commands[0], /^go test /, file);
    assert.notEqual(commands[0], "go test ./... -count=1 -timeout 300s", file);
  }
});

test("limits test-only Go changes to their owning package", () => {
  assert.deepEqual(goAffectedTestCommands(["internal/productfeatures/service_test.go"], {
    root: "/repo",
    packages: goPackageFixture,
    fileExists: () => true,
  }), [
    "go test -p=4 github.com/jftrade/jftrade-main/internal/productfeatures -count=1 -timeout 300s",
  ]);
});

test("falls back to the full Go suite for deleted, unresolved, module, and broad changes", () => {
  const full = ["go test ./... -count=1 -timeout 300s"];
  assert.deepEqual(goAffectedTestCommands(["internal/productfeatures/deleted.go"], {
    root: "/repo",
    packages: goPackageFixture,
    fileExists: () => false,
  }), full);
  assert.deepEqual(goAffectedTestCommands(["internal/missing/source.go"], {
    root: "/repo",
    packages: goPackageFixture,
    fileExists: () => true,
  }), full);
  assert.deepEqual(goAffectedTestCommands(["go.mod"]), full);
  assert.deepEqual(goAffectedTestCommands(["internal/productfeatures/service.go"], {
    root: "/repo",
    packages: goPackageFixture,
    fileExists: () => true,
    maxAffectedPackages: 3,
  }), full);
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
