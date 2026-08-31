import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  atomicallyRetainRollbackArtifact,
  inspectRollbackArtifactPair,
  main,
  runRollbackArtifactHarness,
  validateVersionTransition,
} from "./check-rollback-artifact.mjs";

const PLATFORMS = [
  ["macos-arm64", "arm64", [["dmg", "dmg"]], "darwin-aarch64"],
  ["linux-x64", "amd64", [["appimage", "AppImage"], ["deb", "deb"], ["rpm", "rpm"]], "linux-x86_64"],
  ["windows-x64", "amd64", [["nsis", "exe"]], "windows-x86_64"],
  ["windows-arm64", "arm64", [["nsis", "exe"]], "windows-aarch64"],
];

const SIGNATURE = [
  "untrusted comment: fixture signature (not cryptographic evidence)",
  "RURqRkFLRV9TSUdOQVRVUkU=",
].join("\n");

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

function write(root, relativePath, value) {
  const filePath = path.join(root, relativePath);
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, value);
  return filePath;
}

function entry(root, relativePath, value) {
  write(root, relativePath, value);
  return {
    path: relativePath,
    sha256: sha256(value),
    size: Buffer.byteLength(value),
  };
}

function createRelease(root, version) {
  const feed = { version, notes: `fixture ${version}`, platforms: {} };
  for (const [platform, architecture, packageKinds, target] of PLATFORMS) {
    const packages = packageKinds.map(([kind, extension]) => {
      const suffix = extension === "exe" ? "-setup.exe" : `.${extension}`;
      const relativePath = `packages/JFTrade-${version}-${platform}-${kind}${suffix}`;
      return { kind, ...entry(root, relativePath, `${version}:${platform}:${kind}`) };
    });
    const archivePath = `updater/JFTrade_${version}_${platform}.tar.gz`;
    const archive = entry(root, archivePath, `${version}:${platform}:updater`);
    const signaturePath = `${archivePath}.sig`;
    const signature = entry(root, signaturePath, `${SIGNATURE}\n`);
    const manifest = {
      schemaVersion: "jftrade.tauri-release-artifacts.v1",
      target: { architecture, platform },
      version,
      scope: "package-and-integrity",
      packages,
      appBundle: null,
      updaterSignatures: [signature],
      updaterArchives: [archive],
    };
    write(root, `tauri-release-${platform}.json`, `${JSON.stringify(manifest, null, 2)}\n`);
    feed.platforms[target] = {
      url: `https://updates.example.test/${path.basename(archivePath)}`,
      signature: SIGNATURE,
    };
  }
  write(root, "latest.json", `${JSON.stringify(feed, null, 2)}\n`);
  return feed;
}

function removeUpdaterArchiveLists(root) {
  for (const [platform] of PLATFORMS) {
    const manifestPath = path.join(root, `tauri-release-${platform}.json`);
    const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
    delete manifest.updaterArchives;
    fs.writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
  }
}

function flattenReleaseFiles(root) {
  for (const filePath of fs.readdirSync(path.join(root, "packages"), { withFileTypes: true })) {
    fs.renameSync(path.join(root, "packages", filePath.name), path.join(root, filePath.name));
  }
  for (const filePath of fs.readdirSync(path.join(root, "updater"), { withFileTypes: true })) {
    fs.renameSync(path.join(root, "updater", filePath.name), path.join(root, filePath.name));
  }
  fs.rmSync(path.join(root, "packages"), { recursive: true, force: true });
  fs.rmSync(path.join(root, "updater"), { recursive: true, force: true });
}

function fixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-rollback-artifact-"));
  const currentRoot = path.join(root, "current");
  const previousRoot = path.join(root, "previous");
  fs.mkdirSync(currentRoot);
  fs.mkdirSync(previousRoot);
  createRelease(currentRoot, "1.2.4");
  createRelease(previousRoot, "1.2.3");
  const instructionsPath = write(
    currentRoot,
    "rollback.md",
    "Rollback 1.2.4 to 1.2.3: retain the signed package and updater metadata, stop the product, then explicitly install the previous version on a native runner.",
  );
  return {
    root,
    currentRoot,
    previousRoot,
    instructionsPath,
    cleanup: () => fs.rmSync(root, { recursive: true, force: true }),
  };
}

test("validates all platform package manifests, updater metadata and versions as a rollback pair", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const report = inspectRollbackArtifactPair({
    currentRoot: value.currentRoot,
    previousRoot: value.previousRoot,
    currentVersion: "1.2.4",
    previousVersion: "1.2.3",
    allowDowngrade: true,
    instructionsPath: value.instructionsPath,
  });
  assert.equal(report.schemaVersion, "jftrade.rollback-artifact.v1");
  assert.equal(report.scope, "previous-version-integrity-and-pairing");
  assert.equal(report.versionPolicy.downgradeAllowed, true);
  assert.deepEqual(Object.keys(report.current.platforms), PLATFORMS.map(([platform]) => platform));
  assert.equal(report.current.updaterMetadata.version, "1.2.4");
  assert.equal(report.previous.updaterMetadata.version, "1.2.3");
  assert.equal(report.current.packageSigning, "not-verified-by-node");
  assert.match(report.limitations.join(" "), /native install, downgrade, rollback/);
});

test("refuses a downgrade unless the caller explicitly allows rollback mode", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      instructionsPath: value.instructionsPath,
    }),
    /version downgrade .*explicit allowDowngrade/,
  );
  assert.deepEqual(
    validateVersionTransition({ currentVersion: "1.2.4", previousVersion: "1.2.3", allowDowngrade: true }),
    {
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      downgrade: true,
      downgradeAllowed: true,
    },
  );
  assert.throws(
    () => validateVersionTransition({ currentVersion: "1.2.3", previousVersion: "1.2.4", allowDowngrade: true }),
    /newer than current/,
  );
});

test("accepts the release manifest shape that derives updater archives from signature paths", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  removeUpdaterArchiveLists(value.currentRoot);
  removeUpdaterArchiveLists(value.previousRoot);
  const report = inspectRollbackArtifactPair({
    currentRoot: value.currentRoot,
    previousRoot: value.previousRoot,
    currentVersion: "1.2.4",
    previousVersion: "1.2.3",
    allowDowngrade: true,
    instructionsPath: value.instructionsPath,
  });
  assert.equal(report.current.updaterMetadata.targets["linux-x64"].archive, "JFTrade_1.2.4_linux-x64.tar.gz");
  assert.equal(report.previous.updaterMetadata.targets["windows-arm64"].archive, "JFTrade_1.2.3_windows-arm64.tar.gz");
});

test("resolves manifest paths after the desktop publish job flattens release assets", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  flattenReleaseFiles(value.currentRoot);
  flattenReleaseFiles(value.previousRoot);
  const report = inspectRollbackArtifactPair({
    currentRoot: value.currentRoot,
    previousRoot: value.previousRoot,
    currentVersion: "1.2.4",
    previousVersion: "1.2.3",
    allowDowngrade: true,
    instructionsPath: value.instructionsPath,
  });
  assert.equal(report.current.platforms["macos-arm64"].packageCount, 1);
  assert.equal(report.previous.platforms["linux-x64"].archiveNames.length, 1);
});

test("rejects mismatched updater metadata and missing package integrity", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const feedPath = path.join(value.currentRoot, "latest.json");
  const feed = JSON.parse(fs.readFileSync(feedPath, "utf8"));
  feed.version = "1.2.3";
  fs.writeFileSync(feedPath, JSON.stringify(feed));
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /updater metadata.version .*does not match/,
  );
  createRelease(value.currentRoot, "1.2.4");
  const manifestPath = path.join(value.previousRoot, "tauri-release-linux-x64.json");
  const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  fs.rmSync(path.join(value.previousRoot, manifest.packages[0].path));
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /packages\[0\] is missing/,
  );
});

test("requires rollback instructions to name both versions and an explicit operation", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  fs.writeFileSync(value.instructionsPath, "Keep this note for later.");
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /explicit rollback\/downgrade/,
  );
});

test("retains a rollback directory with copy-then-rename and refuses overwrite", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const retainedRoot = path.join(value.root, "retained");
  const result = atomicallyRetainRollbackArtifact({
    sourceRoot: value.previousRoot,
    retainedRoot,
    version: "1.2.3",
  });
  assert.equal(result.atomicRename, true);
  assert.equal(result.sourcePreserved, true);
  assert.equal(fs.existsSync(path.join(retainedRoot, "1.2.3", "latest.json")), true);
  assert.throws(
    () => atomicallyRetainRollbackArtifact({
      sourceRoot: value.previousRoot,
      retainedRoot,
      version: "1.2.3",
    }),
    /refusing overwrite/,
  );
});

test("harness validates then atomically retains the previous release without installing it", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const report = runRollbackArtifactHarness({
    currentRoot: value.currentRoot,
    previousRoot: value.previousRoot,
    retainedRoot: path.join(value.root, "retained"),
    currentVersion: "1.2.4",
    previousVersion: "1.2.3",
    instructionsPath: value.instructionsPath,
  });
  assert.equal(report.retention.retainedVersion, "1.2.3");
  assert.equal(report.retention.rollbackInstall, "not-executed");
  assert.equal(report.retention.sourcePreserved, true);
});

test("CLI validates a fixture pair without changing the closeout manifest", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const manifestPath = path.join(
    path.dirname(fileURLToPath(import.meta.url)),
    "../../tests/fixtures/rust-migration/stage9/closeout-evidence.json",
  );
  const before = fs.readFileSync(manifestPath);
  const scriptPath = fileURLToPath(new URL("./check-rollback-artifact.mjs", import.meta.url));
  const result = spawnSync(process.execPath, [
    scriptPath,
    "--current-dir", value.currentRoot,
    "--previous-dir", value.previousRoot,
    "--current", "1.2.4",
    "--previous", "1.2.3",
    "--allow-downgrade",
    "--instructions", value.instructionsPath,
  ], { cwd: path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../.."), encoding: "utf8" });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /Verified rollback artifact pairing 1\.2\.4 -> 1\.2\.3/);
  assert.deepEqual(fs.readFileSync(manifestPath), before);
});
