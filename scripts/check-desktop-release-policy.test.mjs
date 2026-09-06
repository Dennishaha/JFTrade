import assert from "node:assert/strict";
import test from "node:test";

import {
  DESKTOP_RELEASE_OPERATIONS,
  evaluateDesktopReleasePolicy,
  RELEASE_SIGNING_REQUIREMENTS,
} from "./check-desktop-release-policy.mjs";

function signedEnvironment(overrides = {}) {
  const values = { JFTRADE_DESKTOP_OPERATION: "candidate" };
  for (const name of Object.values(RELEASE_SIGNING_REQUIREMENTS).flat()) values[name] = "super-secret-value";
  values.JFTRADE_TAURI_UPDATER_ENDPOINT = "https://updates.example.test/feed.json";
  return { ...values, ...overrides };
}

test("release operations are explicit and limited to the three governed lanes", () => {
  assert.deepEqual(DESKTOP_RELEASE_OPERATIONS, ["rehearsal", "candidate", "publish"]);
  const result = evaluateDesktopReleasePolicy({
    environment: { JFTRADE_DESKTOP_OPERATION: "dry-run" },
  });
  assert.equal(result.valid, false);
  assert.match(result.blockers[0], /unsupported desktop release operation/);
});

test("rehearsal policy does not require or inspect signing credentials", () => {
  const result = evaluateDesktopReleasePolicy({
    environment: { JFTRADE_DESKTOP_OPERATION: "rehearsal" },
  });
  assert.deepEqual(result, {
    operation: "rehearsal",
    publish: false,
    valid: true,
    blockers: [],
  });
});

test("candidate policy reports every missing signing value without leaking secrets", () => {
  const result = evaluateDesktopReleasePolicy({
    environment: { JFTRADE_DESKTOP_OPERATION: "candidate" },
  });
  assert.equal(result.valid, false);
  assert.equal(result.blockers.length, Object.values(RELEASE_SIGNING_REQUIREMENTS).flat().length);
  assert.ok(result.blockers.every((blocker) => !blocker.includes("super-secret-value")));
});

test("candidate policy accepts signing configuration before post-release gates close", () => {
  const result = evaluateDesktopReleasePolicy({ environment: signedEnvironment() });
  assert.deepEqual(result, {
    operation: "candidate",
    publish: false,
    valid: true,
    blockers: [],
  });
});

test("candidate policy rejects updater endpoints that are not credential-free HTTPS", () => {
  const result = evaluateDesktopReleasePolicy({
    environment: signedEnvironment({ JFTRADE_TAURI_UPDATER_ENDPOINT: "http://updates.example.test/feed" }),
  });
  assert.equal(result.valid, false);
  assert.ok(result.blockers.some((blocker) => blocker.includes("HTTPS URL without credentials")));
});

test("publish policy needs no signing secrets because it only consumes a sealed candidate", () => {
  const result = evaluateDesktopReleasePolicy({
    environment: { JFTRADE_DESKTOP_OPERATION: "publish" },
  });
  assert.deepEqual(result, {
    operation: "publish",
    publish: true,
    valid: true,
    blockers: [],
  });
});

test("p2_03: release qualification verifies platform signing boundaries and blocks unverified candidates", () => {
  // Rehearsal builds are allowed without signing secrets
  const rehearsal = evaluateDesktopReleasePolicy({
    environment: { JFTRADE_DESKTOP_OPERATION: "rehearsal" },
  });
  assert.equal(rehearsal.valid, true);

  // Candidate builds fail-closed without credentials
  const candidate = evaluateDesktopReleasePolicy({
    environment: { JFTRADE_DESKTOP_OPERATION: "candidate" },
  });
  assert.equal(candidate.valid, false);
  assert.ok(candidate.blockers.length >= 12);
});
