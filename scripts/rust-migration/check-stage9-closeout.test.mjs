import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  evaluateCandidate,
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

function releaseCandidateManifest() {
  const { manifest } = completeManifest();
  manifest.status = "in_progress";
  for (const gate of ["postReleaseSmoke", "hardCutReadiness"]) {
    manifest.gates[gate].status = "open";
  }
  manifest.ownerDeletion.go.status = "open";
  manifest.ownerDeletion.wails.status = "open";
  return manifest;
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

test("Stage 9 closeout keeps hard-cut and owner-deletion gates open", () => {
  const manifest = readManifest();
  const result = evaluateCloseout(manifest, {
    expectedRouteOwnership: routeOwnershipSnapshot(repositoryRoot),
  });
  assert.equal(result.valid, true);
  assert.equal(manifest.gates.hardCutReadiness.status, "open");
  assert.equal(manifest.ownerDeletion.go.status, "open");
  assert.equal(manifest.ownerDeletion.wails.status, "open");
  assert.ok(
    !result.blockers.some((blocker) => blocker.includes("is passed while prerequisite gate")),
  );
});

test("Stage 9 closeout checker reports open evidence without failing by default", () => {
  assert.equal(main(["--manifest", manifestPath]), 0);
});

test("Stage 9 closeout checker fails closed in check mode", () => {
  assert.equal(main(["--check", "--manifest", manifestPath]), 1);
});

test("Stage 9 candidate admission allows pre-release gates while post-release gates remain open", () => {
  const manifest = releaseCandidateManifest();
  const expectedRouteOwnership = routeOwnershipSnapshot(repositoryRoot);
  const candidate = evaluateCandidate(manifest, { expectedRouteOwnership });
  assert.equal(candidate.valid, true);
  assert.equal(candidate.complete, true);
  assert.deepEqual(candidate.blockers, []);

  const closeout = evaluateCloseout(manifest, { expectedRouteOwnership });
  assert.equal(closeout.valid, true);
  assert.equal(closeout.complete, false);
  assert.ok(closeout.blockers.some((blocker) => blocker === "gate postReleaseSmoke is open"));
  assert.ok(closeout.blockers.some((blocker) => blocker === "gate hardCutReadiness is open"));
});

test("Stage 9 candidate admission rejects a pre-publication post-release claim", () => {
  const manifest = releaseCandidateManifest();
  manifest.gates.postReleaseSmoke.status = "passed";
  const result = evaluateCandidate(manifest, {
    expectedRouteOwnership: routeOwnershipSnapshot(repositoryRoot),
  });
  assert.equal(result.valid, true);
  assert.equal(result.complete, false);
  assert.ok(result.blockers.some((blocker) => blocker.includes("postReleaseSmoke")));
});

test("Stage 9 closeout checker accepts a complete evidence manifest only with all gates passed", () => {
  const { manifest, expectedRouteOwnership } = completeManifest();
  const result = evaluateCloseout(manifest, { expectedRouteOwnership });
  assert.equal(result.valid, true);
  assert.equal(result.complete, true);
  assert.deepEqual(result.blockers, []);
});

test("Stage 9 full checker accepts a closed manifest from the CLI", () => {
  const { manifest } = completeManifest();
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-closeout-closed-"));
  const candidatePath = path.join(directory, "closeout-evidence.json");
  fs.writeFileSync(candidatePath, `${JSON.stringify(manifest)}\n`, "utf8");
  try {
    const result = spawnSync(
      process.execPath,
      [path.join(repositoryRoot, "scripts/rust-migration/check-stage9-closeout.mjs"), "--check", "--manifest", candidatePath],
      { cwd: repositoryRoot, encoding: "utf8" },
    );
    assert.equal(result.status, 0);
    assert.match(result.stdout, /ready for formal close/);
  } finally {
    fs.rmSync(directory, { recursive: true, force: true });
  }
});

test("Stage 9 candidate CLI is executable while full closeout remains fail-closed", () => {
  const manifest = releaseCandidateManifest();
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-closeout-candidate-"));
  const candidatePath = path.join(directory, "closeout-evidence.json");
  fs.writeFileSync(candidatePath, `${JSON.stringify(manifest)}\n`, "utf8");
  try {
    const candidate = spawnSync(
      process.execPath,
      [path.join(repositoryRoot, "scripts/rust-migration/check-stage9-closeout.mjs"), "--candidate", "--manifest", candidatePath],
      { cwd: repositoryRoot, encoding: "utf8" },
    );
    assert.equal(candidate.status, 0);
    assert.match(candidate.stdout, /release-candidate evidence: candidate admission passed/);
  } finally {
    fs.rmSync(directory, { recursive: true, force: true });
  }

  const candidate = spawnSync(
    process.execPath,
    [path.join(repositoryRoot, "scripts/rust-migration/check-stage9-closeout.mjs"), "--candidate"],
    { cwd: repositoryRoot, encoding: "utf8" },
  );
  assert.equal(candidate.status, 1);
  assert.match(candidate.stdout, /candidate admission blocked/);
  assert.equal(main(["--check", "--manifest", manifestPath]), 1);
});

test("Stage 9 closeout checker rejects missing and unknown evidence fields", () => {
  const manifest = readManifest();
  delete manifest.gates.platformRelease;
  manifest.ownerDeletion.extra = { status: "passed" };
  const errors = validateManifest(manifest);
  assert.ok(errors.some((error) => error.includes("platformRelease is required")));
  assert.ok(errors.some((error) => error.includes("ownerDeletion.extra is not allowed")));
});

test("Stage 9 closeout checker rejects an unknown owner entry status", () => {
  const manifest = readManifest();
  manifest.ownerDeletion.wails.entryStatus = "retired";
  const errors = validateManifest(manifest);
  assert.ok(errors.some((error) => error.includes("ownerDeletion.wails.entryStatus")));
});

test("Stage 9 closeout keeps a passed owner gate blocked when its entrypoint is retained", () => {
  const { manifest, expectedRouteOwnership } = completeManifest();
  manifest.ownerDeletion.wails.entryStatus = "retained";
  const result = evaluateCloseout(manifest, { expectedRouteOwnership });
  assert.equal(result.valid, true);
  assert.equal(result.complete, false);
  assert.ok(result.blockers.some((blocker) => blocker.includes("entrypoint status is retained")));
});

test("Stage 9 candidate checker fails closed for a missing manifest", () => {
  assert.equal(main(["--candidate", "--manifest", path.join(repositoryRoot, "missing-closeout.json")]), 1);
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
