import assert from "node:assert/strict";
import test from "node:test";

import {
  changedFiles,
  fullGatePlan,
  githubOutputs,
  planAffected,
  planGateLanes,
  resolveAffectedModules,
  resolveBase,
  resolveFallbackChecks,
  rustAffectedClippyCommands,
  rustAffectedPackages,
  rustAffectedTestCommands,
  webAffectedTestCommands,
} from "./test-affected.mjs";

const rustWorkspace = [
  { name: "jftrade-kernel", manifest: "crates/jftrade-kernel/Cargo.toml", dependencies: [] },
  { name: "jftrade-marketdata", manifest: "crates/jftrade-marketdata/Cargo.toml", dependencies: ["jftrade-kernel"] },
  { name: "jftrade-engine", manifest: "crates/jftrade-engine/Cargo.toml", dependencies: ["jftrade-marketdata"] },
  { name: "jftrade-desktop", manifest: "apps/desktop/src-tauri/Cargo.toml", dependencies: ["jftrade-engine"] },
];
const rustOptions = { workspace: rustWorkspace, fileExists: () => true, maxAffectedPackages: 8 };

test("maps changed files to declared product modules", () => {
  const modules = resolveAffectedModules([
    "crates/jftrade-marketdata/src/lib.rs",
    "apps/web/src/composables/backtest/useBacktestPage.ts",
  ]);
  assert.deepEqual(modules.map((module) => module.id), ["rust", "web"]);
});

test("classifies language and generated fallback checks", () => {
  assert.deepEqual([...resolveFallbackChecks([
    "crates/jftrade-engine/src/lib.rs",
    "apps/web/src/generated/openapi.ts",
    "scripts/test-affected.mjs",
    ".github/workflows/ci.yml",
  ])].sort(), ["generated", "rust", "scripts", "web", "workflows"]);
});

test("uses the merge-base for a diverged branch", () => {
  const calls = [];
  const fakeGit = (_root, args) => {
    calls.push(args);
    if (args[0] === "rev-parse") return "feature-base";
    if (args[0] === "merge-base") return "merge-base-commit";
    return "";
  };
  assert.equal(resolveBase("/tmp/repo", {
    env: { JFTRADE_DIFF_BASE: "feature-base" }, gitCommand: fakeGit,
  }), "merge-base-commit");
  assert.deepEqual(calls, [
    ["rev-parse", "--verify", "feature-base"],
    ["merge-base", "HEAD", "feature-base"],
  ]);
});

test("fails when no merge base can be resolved", () => {
  assert.throws(() => resolveBase("/tmp/repo", {
    env: {}, gitCommand: () => { throw new Error("missing"); },
  }), /unable to resolve/);
});

test("returns changed and untracked files deterministically", () => {
  const fakeGit = (_root, args) => args[0] === "diff" ? "b\na" : "c\0a\0";
  assert.deepEqual(changedFiles("/tmp/repo", "HEAD", { gitCommand: fakeGit }), ["a", "b", "c"]);
});

test("main always selects every product lane", () => {
  const plan = planGateLanes(["docs/README.md"], { main: true });
  assert.equal(plan.full, true);
  assert.ok(Object.values(plan.lanes).every(Boolean));
  assert.equal(plan.compatibilityCapabilities.length, 7);
});

test("workflow Cargo toolchain and gate changes force a full plan", () => {
  for (const file of [
    ".github/workflows/ci.yml",
    "Cargo.lock",
    "rust-toolchain.toml",
    "scripts/quality/check-contracts.mjs",
    "scripts/module-map.json",
  ]) {
    const plan = planGateLanes([file]);
    assert.equal(plan.full, true, file);
    assert.ok(Object.values(plan.lanes).every(Boolean), file);
  }
});

test("unknown product paths fail closed to the full plan", () => {
  const plan = planGateLanes(["crates-new/jftrade-surprise/src/lib.rs"]);
  assert.equal(plan.full, true);
  assert.match(plan.reason, /unknown path/);
});

test("test-only Rust changes stay in their crate while production changes include reverse dependencies", () => {
  assert.deepEqual(rustAffectedPackages([
    "crates/jftrade-kernel/tests/value_contracts.rs",
  ], rustOptions), ["jftrade-kernel"]);
  assert.deepEqual(rustAffectedPackages([
    "crates/jftrade-kernel/src/lib.rs",
  ], rustOptions), ["jftrade-desktop", "jftrade-engine", "jftrade-kernel", "jftrade-marketdata"]);
});

test("Rust manifests and metadata failures fall back to the workspace", () => {
  assert.deepEqual(rustAffectedTestCommands(["Cargo.lock"]), ["pnpm run test:rust"]);
  assert.deepEqual(rustAffectedClippyCommands(["rust-toolchain.toml"]), ["pnpm run lint:rust"]);
  assert.deepEqual(rustAffectedTestCommands(["crates/jftrade-kernel/src/lib.rs"], {
    loadWorkspace: () => { throw new Error("metadata failed"); },
  }), ["pnpm run test:rust"]);
});

test("maps compatibility fixture changes to exactly one capability", () => {
  const plan = planGateLanes([
    "tests/fixtures/compatibility/assistant-runtime/assistant-rig-corpus.json",
  ]);
  assert.equal(plan.full, false);
  assert.equal(plan.lanes.compatibility, true);
  assert.deepEqual(plan.compatibilityCapabilities, ["assistant-runtime"]);
});

test("engine production changes select Rust contracts Pine and every compatibility replay", () => {
  const plan = planGateLanes(["crates/jftrade-engine/src/product.rs"]);
  assert.equal(plan.full, false);
  assert.equal(plan.lanes.rust_static, true);
  assert.equal(plan.lanes.rust_tests, true);
  assert.equal(plan.lanes.contracts, true);
  assert.equal(plan.lanes.pine, true);
  assert.equal(plan.lanes.desktop, true);
  assert.equal(plan.compatibilityCapabilities.length, 7);
});

test("production runtime inputs select desktop smoke while test-only changes stay in their lane", () => {
  for (const file of [
    "crates/jftrade-kernel/src/lib.rs",
    "apps/web/src/App.vue",
    "workers/pineworker/src/main.ts",
    "workers/marketdata-sidecar/src/marketdata_sidecar/main.py",
  ]) {
    const plan = planGateLanes([file]);
    assert.equal(plan.lanes.desktop, true, file);
    assert.equal(plan.lanes.contracts, true, `${file} must validate the desktop contract input`);
  }
  for (const file of [
    "crates/jftrade-kernel/tests/value_contracts.rs",
    "apps/web/src/App.test.ts",
    "workers/pineworker/tests/runtime.test.ts",
    "workers/marketdata-sidecar/tests/test_runtime.py",
  ]) {
    assert.equal(planGateLanes([file]).lanes.desktop, false, file);
  }
});

test("quick local Rust plan is targeted and defers the complete Rust gate", () => {
  const plan = planAffected(["crates/jftrade-kernel/src/lib.rs"], {
    withChecks: true, profile: "quick", rustOptions,
  });
  assert.deepEqual(plan.commands, [
    "pnpm run check:policy",
    "pnpm run check:contracts",
    "pnpm run check:rust:target-health",
    "cargo test -p jftrade-desktop -p jftrade-engine -p jftrade-kernel -p jftrade-marketdata --all-targets --locked",
    "pnpm run format:rust:check",
    "cargo clippy -p jftrade-desktop -p jftrade-engine -p jftrade-kernel -p jftrade-marketdata --all-targets --all-features -- -D warnings",
    "pnpm run check:desktop",
  ]);
  assert.deepEqual(plan.deferredCommands, ["pnpm run check:rust"]);
});

test("quick fail-closed plans run the parallel core preflight and defer complete product validation", () => {
  const plan = planAffected([".github/workflows/ci.yml"], {
    withChecks: true,
    profile: "quick",
  });
  assert.equal(plan.full, true);
  assert.deepEqual(plan.commands, ["pnpm run test:preflight"]);
  assert.deepEqual(plan.deferredCommands, ["pnpm run check:all"]);
});

test("web planner targets direct tests and related sources", () => {
  assert.deepEqual(webAffectedTestCommands([
    "apps/web/src/foo.test.ts",
    "apps/web/src/bar.ts",
    "apps/web/src/deleted.ts",
  ], { fileExists: (file) => !file.endsWith("deleted.ts"), webRoot: "/repo/apps/web" }), [
    "pnpm --filter @jftrade/web exec vitest run 'src/foo.test.ts'",
    "pnpm --filter @jftrade/web exec vitest related --run 'src/bar.ts'",
  ]);
});

test("GitHub outputs expose every lane and capability matrix", () => {
  const output = githubOutputs(fullGatePlan("fallback"));
  assert.match(output, /^full=true$/m);
  assert.match(output, /^rust_tests=true$/m);
  assert.match(output, /^desktop=true$/m);
  assert.match(output, /compatibility_capabilities=\["storage"/);
});
