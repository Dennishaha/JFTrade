import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

import { evaluateDesktopReleasePolicy, RELEASE_SIGNING_REQUIREMENTS } from "./check-desktop-release-policy.mjs";

const repositoryRoot = path.resolve(".");
const manifestPath = path.join(
  repositoryRoot,
  "tests/fixtures/rust-migration/stage9/closeout-evidence.json",
);

function environment(overrides = {}) {
  const values = { JFTRADE_DESKTOP_PUBLISH: "true" };
  for (const name of Object.values(RELEASE_SIGNING_REQUIREMENTS).flat()) values[name] = "super-secret-value";
  return { ...values, ...overrides };
}

function completeManifest() {
  const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  manifest.status = "closed";
  for (const gate of Object.values(manifest.gates)) {
    gate.status = "passed";
    if (gate.platforms) {
      for (const platform of Object.values(gate.platforms)) {
        for (const check of ["package", "signed", "install", "upgrade", "uninstall", "rollback", "runtimeSmoke"]) {
          platform[check] = "passed";
        }
      }
    }
  }
  manifest.ownerDeletion.go.status = "passed";
  manifest.ownerDeletion.wails.status = "passed";
  return manifest;
}

const completeOwnership = {
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

test("dry-run release policy does not require signing credentials or closeout", () => {
  const result = evaluateDesktopReleasePolicy({ environment: { JFTRADE_DESKTOP_PUBLISH: "false" } });
  assert.deepEqual(result, { publish: false, valid: true, blockers: [] });
});

test("publish policy reports every missing signing and updater value without leaking secrets", () => {
  const result = evaluateDesktopReleasePolicy({
    environment: { JFTRADE_DESKTOP_PUBLISH: "true" },
    closeoutManifest: completeManifest(),
    expectedRouteOwnership: completeOwnership,
  });
  assert.equal(result.valid, false);
  assert.equal(result.blockers.length, Object.values(RELEASE_SIGNING_REQUIREMENTS).flat().length);
  assert.ok(result.blockers.every((blocker) => !blocker.includes("super-secret-value")));
});

test("publish policy requires a complete closeout even when credentials are configured", () => {
  const manifest = completeManifest();
  manifest.gates.platformRelease.status = "open";
  const result = evaluateDesktopReleasePolicy({
    environment: environment(),
    closeoutManifest: manifest,
    expectedRouteOwnership: completeOwnership,
  });
  assert.equal(result.valid, false);
  assert.ok(result.blockers.some((blocker) => blocker.includes("Stage 9 closeout gate")));
});

test("publish policy rejects updater endpoints that are not credential-free HTTPS", () => {
  const result = evaluateDesktopReleasePolicy({
    environment: environment({ JFTRADE_TAURI_UPDATER_ENDPOINT: "http://updates.example.test/feed" }),
    closeoutManifest: completeManifest(),
    expectedRouteOwnership: completeOwnership,
  });
  assert.equal(result.valid, false);
  assert.ok(result.blockers.some((blocker) => blocker.includes("HTTPS URL without credentials")));
});
