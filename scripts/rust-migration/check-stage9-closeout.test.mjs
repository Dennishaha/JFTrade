import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  evaluateCloseout,
  main,
  REQUIRED_PLATFORMS,
  routeOwnershipSnapshot,
  validateManifest,
} from "./check-stage9-closeout.mjs";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const manifestPath = path.join(
  repositoryRoot,
  "tests/fixtures/rust-migration/stage9/closeout-evidence.json",
);

function readManifest() {
  return JSON.parse(fs.readFileSync(manifestPath, "utf8"));
}

function completeManifest() {
  const manifest = readManifest();
  const expectedRouteOwnership = {
    baselineOperations: 278,
    shadowRoutes: 0,
    cutoverTestOnlyRoutes: 0,
    cutoverQualifiedRoutes: 278,
    remainingRoutes: 0,
    goProductionOwnerRoutes: 0,
    rustProductionOwnerRoutes: 278,
    removedGoRoutes: 278,
    remainingByCapability: {},
  };
  manifest.status = "closed";
  for (const gate of Object.values(manifest.gates)) {
    gate.status = "passed";
  }
  for (const platform of REQUIRED_PLATFORMS) {
    for (const check of [
      "package",
      "signed",
      "install",
      "upgrade",
      "uninstall",
      "rollback",
      "runtimeSmoke",
    ]) {
      manifest.gates.platformRelease.platforms[platform][check] = "passed";
    }
  }
  manifest.ownerDeletion.go.status = "passed";
  manifest.ownerDeletion.wails.status = "passed";
  return { manifest, expectedRouteOwnership };
}

test("Stage 9 closeout fixture is structurally valid but remains open", () => {
  const manifest = readManifest();
  assert.deepEqual(validateManifest(manifest), []);
  const result = evaluateCloseout(manifest, {
    expectedRouteOwnership: routeOwnershipSnapshot(repositoryRoot),
  });
  assert.equal(result.valid, true);
  assert.equal(result.complete, false);
  assert.match(result.blockers.join("\n"), /gate platformRelease is open/);
  assert.match(result.blockers.join("\n"), /platform macos-arm64 package is open/);
});

test("Stage 9 closeout checker reports open evidence without failing by default", () => {
  assert.equal(main(["--manifest", manifestPath]), 0);
});

test("Stage 9 closeout checker fails closed in check mode", () => {
  assert.equal(main(["--check", "--manifest", manifestPath]), 1);
});

test("Stage 9 closeout checker accepts a complete evidence manifest only with all gates passed", () => {
  const { manifest, expectedRouteOwnership } = completeManifest();
  const result = evaluateCloseout(manifest, { expectedRouteOwnership });
  assert.equal(result.valid, true);
  assert.equal(result.complete, true);
  assert.deepEqual(result.blockers, []);
});

test("Stage 9 closeout checker rejects missing and unknown evidence fields", () => {
  const manifest = readManifest();
  delete manifest.gates.platformRelease;
  manifest.ownerDeletion.extra = { status: "passed" };
  const errors = validateManifest(manifest);
  assert.ok(errors.some((error) => error.includes("platformRelease is required")));
  assert.ok(errors.some((error) => error.includes("ownerDeletion.extra is not allowed")));
});

test("Stage 9 closeout manifest rejects hand-maintained route counts", () => {
  const manifest = readManifest();
  manifest.routeOwnership = {
    baselineOperations: 278,
    shadowRoutes: 26,
    cutoverTestOnlyRoutes: 30,
    remainingRoutes: 222,
  };
  const errors = validateManifest(manifest);
  assert.ok(errors.some((error) => error.includes("routeOwnership is not allowed")));
});

test("Stage 9 closeout checker CLI is executable in a child process", () => {
  const result = spawnSync(
    process.execPath,
    [path.join(repositoryRoot, "scripts/rust-migration/check-stage9-closeout.mjs"), "--check"],
    { cwd: repositoryRoot, encoding: "utf8" },
  );
  assert.equal(result.status, 1);
  assert.match(result.stdout, /Stage 9 closeout evidence/);
});
