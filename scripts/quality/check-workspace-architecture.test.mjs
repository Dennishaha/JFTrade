import assert from "node:assert/strict";
import test from "node:test";

import {
  validateLayoutPolicy,
  validateRustFileLengths,
} from "./check-workspace-architecture.mjs";

function metadata(packages) {
  return {
    packages,
    workspace_members: packages.map((pkg) => pkg.id),
  };
}

function declaration(overrides = {}) {
  return {
    name: "jftrade-kernel",
    path: "crates/jftrade-kernel",
    status: "active",
    layer: "foundation",
    allowedWorkspaceDependencies: [],
    ...overrides,
  };
}

function cargoPackage(name, packagePath, dependencies = []) {
  return {
    id: `${name} 0.1.0 (path+file:///repo/${packagePath})`,
    name,
    manifest_path: `/repo/${packagePath}/Cargo.toml`,
    dependencies: dependencies.map((dependency) => ({ name: dependency })),
  };
}

test("accepts registered active packages at their declared paths", () => {
  const kernel = declaration();
  assert.deepEqual(validateLayoutPolicy(
    { bannedCrateSegments: ["common"], packages: [kernel] },
    metadata([cargoPackage(kernel.name, kernel.path)]),
    { repositoryRoot: "/repo", checkContents: false, pathExists: () => true },
  ), []);
});

test("rejects undeclared workspace crates and dependencies across an unregistered boundary", () => {
  const kernel = declaration();
  const engine = declaration({
    name: "jftrade-engine",
    path: "crates/jftrade-engine",
    layer: "composition",
  });
  const errors = validateLayoutPolicy(
    { bannedCrateSegments: ["common"], packages: [kernel, engine] },
    metadata([
      cargoPackage(kernel.name, kernel.path, ["jftrade-store-sqlite"]),
      cargoPackage(engine.name, engine.path),
      cargoPackage("jftrade-store-sqlite", "crates/jftrade-store-sqlite"),
    ]),
    { repositoryRoot: "/repo", checkContents: false, pathExists: () => true },
  );
  assert.ok(errors.includes("workspace package jftrade-store-sqlite is not registered in workspace-architecture-policy.json"));
  assert.ok(errors.includes("jftrade-kernel may not depend on workspace package jftrade-store-sqlite"));
});

test("rejects ownerless crate names and planned directories created early", () => {
  const planned = declaration({
    name: "jftrade-common",
    path: "crates/jftrade-common",
    status: "planned",
  });
  const errors = validateLayoutPolicy(
    { bannedCrateSegments: ["common"], packages: [planned] },
    metadata([]),
    { repositoryRoot: "/repo", checkContents: false, pathExists: () => true },
  );
  assert.ok(errors.includes("jftrade-common uses banned ownerless segment common"));
  assert.ok(errors.includes("planned package path exists before activation: crates/jftrade-common"));
});

test("rejects product and native production files that regrow beyond 800 lines", () => {
  assert.deepEqual(validateRustFileLengths([
    ["crates/jftrade-engine/src/product.rs", "line\n".repeat(800)],
  ]), []);
  assert.deepEqual(validateRustFileLengths([
    ["crates/jftrade-engine/src/product_settings.rs", "line\n".repeat(801)],
    ["apps/desktop/src-tauri/src/native_lifecycle.rs", "line\n".repeat(802)],
  ]), [
    "crates/jftrade-engine/src/product_settings.rs has 801 lines; production Rust limit is 800",
    "apps/desktop/src-tauri/src/native_lifecycle.rs has 802 lines; production Rust limit is 800",
  ]);
});
