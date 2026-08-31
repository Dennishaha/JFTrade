import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { verifyReleaseCandidateBundle } from "./check-release-candidate-bundle.mjs";

function hash(value) {
  return createHash("sha256").update(value).digest("hex");
}

function fixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-bundle-"));
  const candidate = path.join(root, "candidate");
  const release = path.join(root, "release");
  fs.mkdirSync(candidate, { recursive: true });
  fs.mkdirSync(release, { recursive: true });
  const write = (relative, value) => {
    fs.writeFileSync(path.join(candidate, relative), value);
    fs.writeFileSync(path.join(release, path.basename(relative)), value);
    const file = path.join(candidate, relative);
    return { path: relative, sha256: hash(value), size: Buffer.byteLength(value) };
  };
  const platformFiles = {
    "macos-arm64": ["tauri-release-macos-arm64.json", "JFTrade-v1.2.3-macos-arm64.dmg"],
    "linux-x64": ["tauri-release-linux-x64.json", "JFTrade-v1.2.3-linux-x64.AppImage"],
    "windows-x64": ["tauri-release-windows-x64.json", "JFTrade-v1.2.3-windows-x64.msi"],
    "windows-arm64": ["tauri-release-windows-arm64.json", "JFTrade-v1.2.3-windows-arm64.msi"],
  };
  const platforms = Object.fromEntries(Object.entries(platformFiles).map(([platform, [manifestPath, artifactPath]]) => {
    const manifest = write(manifestPath, `${platform}-manifest`);
    const artifact = write(artifactPath, `${platform}-package`);
    return [platform, { manifest, artifacts: [artifact] }];
  }));
  const evidence = {
    platforms,
    sourceArtifacts: [{ name: "desktop-release-macos", id: 10, digest: `sha256:${"a".repeat(64)}`, expired: false, runId: 10, runAttempt: 1 }],
  };
  const evidencePath = path.join(root, "evidence.json");
  fs.writeFileSync(evidencePath, JSON.stringify(evidence));
  return { root, candidate, release, evidencePath, evidence };
}

test("verifies canonical candidate package bytes against the publish directory", (context) => {
  const value = fixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  const result = verifyReleaseCandidateBundle({
    evidencePath: value.evidencePath,
    candidateRoot: value.candidate,
    releaseRoot: value.release,
  });
  assert.equal(result.status, "verified");
  assert.equal(result.files.length, 8);
});

test("fails closed when a published file drifts or source metadata is missing", (context) => {
  const value = fixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  fs.appendFileSync(path.join(value.release, "JFTrade-v1.2.3-macos-arm64.dmg"), "tamper");
  assert.throws(() => verifyReleaseCandidateBundle({
    evidencePath: value.evidencePath,
    candidateRoot: value.candidate,
    releaseRoot: value.release,
  }), /differs from candidate bundle/);
  const missing = structuredClone(value.evidence);
  delete missing.sourceArtifacts;
  fs.writeFileSync(value.evidencePath, JSON.stringify(missing));
  assert.throws(() => verifyReleaseCandidateBundle({
    evidencePath: value.evidencePath,
    candidateRoot: value.candidate,
    releaseRoot: value.release,
  }), /source artifact metadata/);
});

test("rejects symlinked files, duplicate basenames, and incomplete platform sets", (context) => {
  const value = fixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  const macArtifact = value.evidence.platforms["macos-arm64"].artifacts[0].path;
  const macPath = path.join(value.candidate, macArtifact);
  const contents = fs.readFileSync(macPath);
  fs.unlinkSync(macPath);
  fs.symlinkSync(path.join(value.root, "outside"), macPath);
  assert.throws(() => verifyReleaseCandidateBundle({
    evidencePath: value.evidencePath,
    candidateRoot: value.candidate,
    releaseRoot: value.release,
  }), /symbolic link/);
  fs.unlinkSync(macPath);
  fs.writeFileSync(macPath, contents);

  const duplicate = structuredClone(value.evidence);
  duplicate.platforms["linux-x64"].artifacts[0].path = macArtifact;
  fs.writeFileSync(value.evidencePath, JSON.stringify(duplicate));
  assert.throws(() => verifyReleaseCandidateBundle({
    evidencePath: value.evidencePath,
    candidateRoot: value.candidate,
    releaseRoot: value.release,
  }), /duplicates top-level release basename/);

  const incomplete = structuredClone(value.evidence);
  delete incomplete.platforms["windows-arm64"];
  fs.writeFileSync(value.evidencePath, JSON.stringify(incomplete));
  assert.throws(() => verifyReleaseCandidateBundle({
    evidencePath: value.evidencePath,
    candidateRoot: value.candidate,
    releaseRoot: value.release,
  }), /exactly the required platforms/);
});
