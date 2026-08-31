import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  main as bundleMain,
  verifyReleaseCandidateBundle,
  verifySealedReleaseBundle,
} from "./check-release-candidate-bundle.mjs";

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

function sealedFixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-sealed-"));
  const candidate = path.join(root, "candidate");
  const release = path.join(root, "release");
  fs.mkdirSync(candidate, { recursive: true });
  fs.mkdirSync(release, { recursive: true });
  const sourceWorkflowRun = {
    id: 100,
    attempt: 2,
    workflow: "desktop-release.yml",
    ref: "refs/tags/v1.2.3",
    commitSha: "a".repeat(40),
  };
  const qualificationRun = {
    id: 200,
    attempt: 1,
    workflow: "desktop-release-qualification.yml",
    ref: sourceWorkflowRun.ref,
    commitSha: sourceWorkflowRun.commitSha,
  };
  const sourceArtifacts = [
    "desktop-release-linux",
    "desktop-release-macos",
    "desktop-release-windows",
    "desktop-release-windows-arm64",
  ].map((name, index) => ({
    name,
    id: 1000 + index,
    digest: `sha256:${String(index + 1).repeat(64)}`,
    expired: false,
    runId: sourceWorkflowRun.id,
    runAttempt: sourceWorkflowRun.attempt,
    workflow: sourceWorkflowRun.workflow,
    ref: sourceWorkflowRun.ref,
    commitSha: sourceWorkflowRun.commitSha,
  }));
  const sourceUpdaterArtifacts = [
    "desktop-release-updater-linux",
    "desktop-release-updater-macos",
    "desktop-release-updater-windows",
    "desktop-release-updater-windows-arm64",
  ].map((name, index) => ({
    name,
    id: 2000 + index,
    digest: `sha256:${String(index + 5).repeat(64)}`,
    expired: false,
    runId: sourceWorkflowRun.id,
    runAttempt: sourceWorkflowRun.attempt,
    workflow: sourceWorkflowRun.workflow,
    ref: sourceWorkflowRun.ref,
    commitSha: sourceWorkflowRun.commitSha,
  }));
  const files = [];
  const write = (name, content) => {
    const bytes = Buffer.from(content);
    fs.writeFileSync(path.join(candidate, name), bytes);
    fs.writeFileSync(path.join(release, name), bytes);
    files.push({ path: name, size: bytes.length, sha256: hash(bytes) });
  };
  for (const platform of ["macos-arm64", "linux-x64", "windows-x64", "windows-arm64"]) {
    write(`tauri-release-${platform}.json`, `${platform}-manifest`);
    write(`tauri-runtime-smoke-${platform}.json`, `${platform}-smoke`);
    write(`JFTrade-${platform}.spdx.json`, `${platform}-sbom`);
  }
  write("JFTrade-v1.2.3-macos-arm64.dmg", "mac-package");
  write("JFTrade-v1.2.3-linux-x64.AppImage", "linux-appimage");
  write("JFTrade-v1.2.3-linux-x64.deb", "linux-deb");
  write("JFTrade-v1.2.3-linux-x64.rpm", "linux-rpm");
  write("JFTrade-v1.2.3-windows-x64-setup.exe", "windows-package");
  write("JFTrade-v1.2.3-windows-arm64-setup.exe", "windows-arm-package");
  write("JFTrade-v1.2.3-macos-arm64.tar.gz", "mac-updater");
  write("JFTrade-v1.2.3-macos-arm64.tar.gz.sig", "mac-signature");
  write("JFTrade-v1.2.3-windows-x64.zip", "windows-updater");
  write("JFTrade-v1.2.3-windows-x64.zip.sig", "windows-signature");
  write("latest.json", "{\"version\":\"1.2.3\"}");
  write("LICENSE", "license");
  write("THIRD-PARTY-NOTICES.md", "notices");
  const sums = files.map((entry) => `${entry.sha256}  ${entry.path}`).join("\n") + "\n";
  write("SHA256SUMS", sums);
  const canonicalEvidence = {
    sourceWorkflowRun,
    sourceArtifacts,
    platforms: {},
  };
  fs.writeFileSync(path.join(candidate, "source-artifact-metadata.json"), JSON.stringify({ releaseArtifacts: sourceArtifacts, updaterArtifacts: sourceUpdaterArtifacts }));
  const canonicalPath = path.join(candidate, "release-candidate-evidence.json");
  fs.writeFileSync(canonicalPath, JSON.stringify(canonicalEvidence));
  const evidenceFile = {
    path: "release-candidate-evidence.json",
    size: fs.statSync(canonicalPath).size,
    sha256: hash(fs.readFileSync(canonicalPath)),
  };
  const manifest = {
    $schema: "./sealed-release-bundle.schema.json",
    schemaVersion: "jftrade.sealed-release-bundle.v1",
    repository: "acme/jftrade",
    releaseRef: sourceWorkflowRun.ref,
    releaseTag: "v1.2.3",
    commitSha: sourceWorkflowRun.commitSha,
    qualificationRun,
    sourceWorkflowRun,
    sourceArtifacts,
    sourceUpdaterArtifacts,
    canonicalEvidence: evidenceFile,
    files,
  };
  const manifestPath = path.join(root, "sealed-release-bundle.json");
  fs.writeFileSync(manifestPath, JSON.stringify(manifest));
  return { root, candidate, release, manifestPath, evidencePath: canonicalPath, manifest, files };
}

test("verifies a sealed four-platform bundle and its checksum declarations", (context) => {
  const value = sealedFixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  const result = verifySealedReleaseBundle({
    manifestPath: value.manifestPath,
    evidencePath: value.evidencePath,
    candidateRoot: value.candidate,
    releaseRoot: value.release,
  });
  assert.equal(result.status, "verified");
  assert.equal(result.files.length, value.files.length);
  assert.equal(result.sourceArtifacts.length, 4);
  assert.equal(result.sourceUpdaterArtifacts.length, 4);
});

test("rejects sealed bundle hash drift, foreign source metadata, missing files, and symlinks", (context) => {
  const value = sealedFixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  fs.appendFileSync(path.join(value.release, "JFTrade-v1.2.3-linux-x64.deb"), "tamper");
  assert.throws(() => verifySealedReleaseBundle({
    manifestPath: value.manifestPath,
    evidencePath: value.evidencePath,
    candidateRoot: value.candidate,
    releaseRoot: value.release,
  }), /digest\/size mismatch/);
  fs.copyFileSync(path.join(value.candidate, "JFTrade-v1.2.3-linux-x64.deb"), path.join(value.release, "JFTrade-v1.2.3-linux-x64.deb"));
  const foreign = structuredClone(value.manifest);
  foreign.sourceArtifacts[0].id = 99999;
  fs.writeFileSync(value.manifestPath, JSON.stringify(foreign));
  assert.throws(() => verifySealedReleaseBundle({
    manifestPath: value.manifestPath,
    evidencePath: value.evidencePath,
    candidateRoot: value.candidate,
    releaseRoot: value.release,
  }), /canonical source artifact metadata differs/);
  const missing = structuredClone(value.manifest);
  missing.files = missing.files.filter((entry) => entry.path !== "latest.json");
  fs.writeFileSync(value.manifestPath, JSON.stringify(missing));
  assert.throws(() => verifySealedReleaseBundle({
    manifestPath: value.manifestPath,
    evidencePath: value.evidencePath,
    candidateRoot: value.candidate,
    releaseRoot: value.release,
  }), /updater feed|exactly match sealed bundle/);
  const symlink = sealedFixture();
  context.after(() => fs.rmSync(symlink.root, { recursive: true, force: true }));
  const target = path.join(symlink.candidate, "LICENSE");
  fs.unlinkSync(target);
  fs.symlinkSync(path.join(symlink.root, "outside"), target);
  assert.throws(() => verifySealedReleaseBundle({
    manifestPath: symlink.manifestPath,
    evidencePath: symlink.evidencePath,
    candidateRoot: symlink.candidate,
    releaseRoot: symlink.release,
  }), /symbolic link/);
});

test("rejects foreign qualification/source run bindings and unknown CLI flags", (context) => {
  const value = sealedFixture();
  context.after(() => fs.rmSync(value.root, { recursive: true, force: true }));
  const foreignQualification = structuredClone(value.manifest);
  foreignQualification.qualificationRun.id = 999;
  fs.writeFileSync(value.manifestPath, JSON.stringify(foreignQualification));
  assert.throws(() => verifySealedReleaseBundle({
    manifestPath: value.manifestPath,
    evidencePath: value.evidencePath,
    candidateRoot: value.candidate,
    releaseRoot: value.release,
    expectedQualificationRun: value.manifest.qualificationRun,
    expectedSourceWorkflowRun: value.manifest.sourceWorkflowRun,
  }), /qualificationRun\.id/);
  const foreignSource = structuredClone(value.manifest);
  foreignSource.sourceWorkflowRun.ref = "refs/tags/v9.9.9";
  fs.writeFileSync(value.manifestPath, JSON.stringify(foreignSource));
  assert.throws(() => verifySealedReleaseBundle({
    manifestPath: value.manifestPath,
    evidencePath: value.evidencePath,
    candidateRoot: value.candidate,
    releaseRoot: value.release,
    expectedQualificationRun: value.manifest.qualificationRun,
    expectedSourceWorkflowRun: value.manifest.sourceWorkflowRun,
  }), /sourceWorkflowRun\.ref/);
  const foreignArtifactDigest = structuredClone(value.manifest);
  foreignArtifactDigest.sourceUpdaterArtifacts[0].digest = `sha256:${"f".repeat(64)}`;
  fs.writeFileSync(value.manifestPath, JSON.stringify(foreignArtifactDigest));
  assert.throws(() => verifySealedReleaseBundle({
    manifestPath: value.manifestPath,
    evidencePath: value.evidencePath,
    candidateRoot: value.candidate,
    releaseRoot: value.release,
    expectedQualificationRun: value.manifest.qualificationRun,
    expectedSourceWorkflowRun: value.manifest.sourceWorkflowRun,
  }), /canonical|sourceUpdaterArtifacts|downloaded updater artifact/);
  const foreignCommit = structuredClone(value.manifest);
  foreignCommit.commitSha = "b".repeat(40);
  fs.writeFileSync(value.manifestPath, JSON.stringify(foreignCommit));
  assert.throws(() => verifySealedReleaseBundle({
    manifestPath: value.manifestPath,
    evidencePath: value.evidencePath,
    candidateRoot: value.candidate,
    releaseRoot: value.release,
  }), /release binding/);
  assert.equal(bundleMain([
    "--evidence", value.evidencePath,
    "--candidate-root", value.candidate,
    "--release-root", value.release,
    "--nope", "x",
  ]), 1);
});
