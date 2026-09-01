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

function nativePackageName(version, platform, kind) {
  if (platform === "macos-arm64") return `JFTrade_${version}_aarch64.dmg`;
  if (platform === "linux-x64" && kind === "appimage") return `JFTrade_${version}_amd64.AppImage`;
  if (platform === "linux-x64" && kind === "deb") return `JFTrade_${version}_x86_64.deb`;
  if (platform === "linux-x64" && kind === "rpm") return `JFTrade-${version}-1.x86_64.rpm`;
  if (platform === "windows-x64") return `JFTrade_${version}_x64-setup.exe`;
  if (platform === "windows-arm64") return `JFTrade_${version}_arm64-setup.exe`;
  throw new Error(`unsupported fixture package: ${platform}/${kind}`);
}

function renamePackagesToNativeNames(root, version) {
  for (const [platform] of PLATFORMS) {
    const manifestPath = path.join(root, `tauri-release-${platform}.json`);
    const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
    manifest.packages = manifest.packages.map((packageEntry) => {
      const sourcePath = path.join(root, packageEntry.path);
      const contents = fs.readFileSync(sourcePath);
      const name = nativePackageName(version, platform, packageEntry.kind);
      const targetPath = path.join(path.dirname(sourcePath), name);
      fs.renameSync(sourcePath, targetPath);
      return {
        ...packageEntry,
        path: path.relative(root, targetPath).split(path.sep).join("/"),
        sha256: sha256(contents),
        size: contents.length,
      };
    });
    fs.writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
  }
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

test("accepts native Tauri package names for every platform and package kind", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  renamePackagesToNativeNames(value.currentRoot, "1.2.4");
  renamePackagesToNativeNames(value.previousRoot, "1.2.3");

  const report = inspectRollbackArtifactPair({
    currentRoot: value.currentRoot,
    previousRoot: value.previousRoot,
    currentVersion: "1.2.4",
    previousVersion: "1.2.3",
    allowDowngrade: true,
    instructionsPath: value.instructionsPath,
  });

  assert.deepEqual(
    Object.fromEntries(Object.entries(report.current.platforms).map(([platform, value]) => [
      platform,
      value.packageCount,
    ])),
    {
      "macos-arm64": 1,
      "linux-x64": 3,
      "windows-x64": 1,
      "windows-arm64": 1,
    },
  );
});

test("rejects native package names whose architecture belongs to another platform", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const manifestPath = path.join(value.currentRoot, "tauri-release-macos-arm64.json");
  const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  manifest.packages[0] = {
    kind: "dmg",
    ...entry(value.currentRoot, "packages/JFTrade_1.2.4_amd64.dmg", "wrong native architecture"),
  };
  fs.writeFileSync(manifestPath, `${JSON.stringify(manifest)}\n`);

  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /must contain platform macos-arm64/,
  );
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

test("rejects symlinked and non-regular release tree entries", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const outside = write(value.root, "outside/package.bin", "outside");
  fs.symlinkSync(outside, path.join(value.currentRoot, "package-link"));
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /symbolic link/,
  );

  fs.unlinkSync(path.join(value.currentRoot, "package-link"));
  fs.mkdirSync(path.join(value.currentRoot, "directory-entry"));
  write(value.currentRoot, "other/directory-entry", "same basename outside declaration");
  const manifest = JSON.parse(fs.readFileSync(path.join(value.currentRoot, "tauri-release-macos-arm64.json"), "utf8"));
  manifest.packages[0].path = "directory-entry";
  write(value.currentRoot, "tauri-release-macos-arm64.json", `${JSON.stringify(manifest)}\n`);
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /regular file|missing/,
  );
});

test("rejects unsafe encoded and platform-specific manifest paths", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const manifestPath = path.join(value.currentRoot, "tauri-release-macos-arm64.json");
  const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  for (const unsafePath of [
    "../outside/JFTrade-1.2.4-macos-arm64-dmg.dmg",
    "packages/%2e%2e/outside.dmg",
    "packages\\JFTrade-1.2.4-macos-arm64-dmg.dmg",
  ]) {
    const candidate = structuredClone(manifest);
    candidate.packages[0].path = unsafePath;
    fs.writeFileSync(manifestPath, `${JSON.stringify(candidate)}\n`);
    assert.throws(
      () => inspectRollbackArtifactPair({
        currentRoot: value.currentRoot,
        previousRoot: value.previousRoot,
        currentVersion: "1.2.4",
        previousVersion: "1.2.3",
        allowDowngrade: true,
        instructionsPath: value.instructionsPath,
      }),
      /safe relative POSIX path|parent path|encoded path traversal/,
    );
  }

  const feedPath = path.join(value.currentRoot, "latest.json");
  const feed = JSON.parse(fs.readFileSync(feedPath, "utf8"));
  feed.platforms["darwin-aarch64"].url = "https://updates.example.test/releases/%2e%2e/JFTrade_1.2.4_macos-arm64.tar.gz";
  fs.writeFileSync(feedPath, `${JSON.stringify(feed)}\n`);
  const restored = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  restored.packages[0] = manifest.packages[0];
  fs.writeFileSync(manifestPath, `${JSON.stringify(restored)}\n`);
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /path traversal segments/,
  );
});

test("rejects duplicate feed target identities and artifact basenames", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const feedPath = path.join(value.currentRoot, "latest.json");
  const feed = JSON.parse(fs.readFileSync(feedPath, "utf8"));
  feed.platforms["darwin-arm64"] = structuredClone(feed.platforms["darwin-aarch64"]);
  fs.writeFileSync(feedPath, `${JSON.stringify(feed)}\n`);
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /duplicate target identity/,
  );

  fs.unlinkSync(feedPath);
  createRelease(value.currentRoot, "1.2.4");
  const duplicateArchiveFeed = JSON.parse(fs.readFileSync(feedPath, "utf8"));
  duplicateArchiveFeed.platforms["linux-x86_64"].url = duplicateArchiveFeed.platforms["darwin-aarch64"].url;
  fs.writeFileSync(feedPath, `${JSON.stringify(duplicateArchiveFeed)}\n`);
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /duplicate updater archive reference/,
  );

  createRelease(value.currentRoot, "1.2.4");
  const feedJson = fs.readFileSync(feedPath, "utf8");
  fs.writeFileSync(feedPath, feedJson.replace('"version": "1.2.4"', '"version": "1.2.4",\n  "version": "1.2.4"'));
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /duplicate JSON object key/,
  );

  createRelease(value.currentRoot, "1.2.4");
  const archivePath = path.join(value.currentRoot, "updater/JFTrade_1.2.4_macos-arm64.tar.gz");
  const archiveManifestPath = path.join(value.currentRoot, "tauri-release-macos-arm64.json");
  const archiveManifest = JSON.parse(fs.readFileSync(archiveManifestPath, "utf8"));
  fs.rmSync(archivePath);
  fs.rmSync(`${archivePath}.sig`);
  write(value.currentRoot, "duplicate/JFTrade_1.2.4_macos-arm64.tar.gz", "duplicate archive");
  write(value.currentRoot, "duplicate/JFTrade_1.2.4_macos-arm64.tar.gz.sig", `${SIGNATURE}\n`);
  write(value.currentRoot, "second/JFTrade_1.2.4_macos-arm64.tar.gz", "second duplicate archive");
  write(value.currentRoot, "second/JFTrade_1.2.4_macos-arm64.tar.gz.sig", `${SIGNATURE}\n`);
  fs.writeFileSync(archiveManifestPath, `${JSON.stringify(archiveManifest)}\n`);
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /duplicate basename/,
  );
});

test("binds artifact names and feeds to the exact manifest version", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const manifestPath = path.join(value.currentRoot, "tauri-release-macos-arm64.json");
  const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  const originalPackage = structuredClone(manifest.packages[0]);
  const wrongName = "packages/JFTrade-11.2.40-macos-arm64-dmg.dmg";
  const wrongEntry = entry(value.currentRoot, wrongName, "wrong version package");
  manifest.packages[0] = { kind: "dmg", ...wrongEntry };
  fs.writeFileSync(manifestPath, `${JSON.stringify(manifest)}\n`);
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /must contain release version 1\.2\.4/,
  );

  const restored = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  restored.packages[0] = originalPackage;
  fs.writeFileSync(manifestPath, `${JSON.stringify(restored)}\n`);
  const feedPath = path.join(value.currentRoot, "latest.json");
  const feed = JSON.parse(fs.readFileSync(feedPath, "utf8"));
  feed.version = "1.2.3";
  fs.writeFileSync(feedPath, `${JSON.stringify(feed)}\n`);
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /updater metadata\.version .*does not match 1\.2\.4/,
  );
  assert.ok(restored);
});

test("binds package paths to their manifest platform and package kind", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const manifestPath = path.join(value.currentRoot, "tauri-release-macos-arm64.json");
  const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  const wrongPlatform = entry(
    value.currentRoot,
    "packages/JFTrade-1.2.4-linux-x64.dmg",
    "linux package with a misleading dmg extension",
  );
  manifest.packages[0] = { kind: "dmg", ...wrongPlatform };
  fs.writeFileSync(manifestPath, `${JSON.stringify(manifest)}\n`);
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /must contain platform macos-arm64/,
  );

  manifest.packages[0] = {
    kind: "dmg",
    ...entry(value.currentRoot, "packages/JFTrade-1.2.4-macos-arm64.deb", "wrong package extension"),
  };
  fs.writeFileSync(manifestPath, `${JSON.stringify(manifest)}\n`);
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /must use \.dmg package extension/,
  );
});

test("rejects sidecar drift, duplicate signature entries and sidecar path substitution", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const manifestPath = path.join(value.currentRoot, "tauri-release-macos-arm64.json");
  const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  const signaturePath = path.join(value.currentRoot, manifest.updaterSignatures[0].path);
  fs.writeFileSync(signaturePath, "different sidecar\n");
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /sha256 does not match/,
  );

  const restoredSignature = entry(value.currentRoot, manifest.updaterSignatures[0].path, `${SIGNATURE}\n`);
  manifest.updaterSignatures[0] = { path: restoredSignature.path, sha256: restoredSignature.sha256, size: restoredSignature.size };
  manifest.updaterSignatures.push(structuredClone(manifest.updaterSignatures[0]));
  fs.writeFileSync(manifestPath, `${JSON.stringify(manifest)}\n`);
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /duplicate updater signature/,
  );

  manifest.updaterSignatures.pop();
  const mismatchedSignature = "updater/substitute.sig";
  const substitute = entry(value.currentRoot, mismatchedSignature, `${SIGNATURE}\n`);
  manifest.updaterSignatures[0].path = mismatchedSignature;
  manifest.updaterSignatures[0].sha256 = substitute.sha256;
  manifest.updaterSignatures[0].size = substitute.size;
  fs.writeFileSync(manifestPath, `${JSON.stringify(manifest)}\n`);
  assert.throws(
    () => inspectRollbackArtifactPair({
      currentRoot: value.currentRoot,
      previousRoot: value.previousRoot,
      currentVersion: "1.2.4",
      previousVersion: "1.2.3",
      allowDowngrade: true,
      instructionsPath: value.instructionsPath,
    }),
    /no matching updater archive|does not match its updater archive sidecar/,
  );
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

test("validates rollback source and containment before creating a retained root", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const missingSource = path.join(value.root, "missing-previous");
  const retainedRoot = path.join(value.root, "retained-before-source-check");
  assert.throws(
    () => atomicallyRetainRollbackArtifact({
      sourceRoot: missingSource,
      retainedRoot,
      version: "1.2.3",
    }),
    /rollback source is missing/,
  );
  assert.equal(fs.existsSync(retainedRoot), false);

  const insideSourceRoot = path.join(value.previousRoot, "retained-inside-source");
  assert.throws(
    () => atomicallyRetainRollbackArtifact({
      sourceRoot: value.previousRoot,
      retainedRoot: insideSourceRoot,
      version: "1.2.3",
    }),
    /inside rollback source/,
  );
  assert.equal(fs.existsSync(insideSourceRoot), false);
});

test("rejects a symlink ancestor before creating a rollback destination", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const outside = path.join(value.root, "outside-retention");
  fs.mkdirSync(outside);
  const linkedAncestor = path.join(value.root, "retained-link");
  try {
    fs.symlinkSync(outside, linkedAncestor, "dir");
  } catch (error) {
    if (["EACCES", "EPERM", "ENOTSUP"].includes(error.code)) {
      context.skip(`directory symlinks are unavailable: ${error.code}`);
      return;
    }
    throw error;
  }
  const retainedRoot = path.join(linkedAncestor, "nested", "retained");
  assert.throws(
    () => atomicallyRetainRollbackArtifact({
      sourceRoot: value.previousRoot,
      retainedRoot,
      version: "1.2.3",
    }),
    /symbolic link/,
  );
  assert.equal(fs.existsSync(path.join(outside, "nested")), false);
});

test("rejects a source symlink ancestor before retaining its files", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const outside = path.join(value.root, "outside-source");
  const sourceDirectory = path.join(outside, "previous");
  fs.mkdirSync(sourceDirectory, { recursive: true });
  fs.writeFileSync(path.join(sourceDirectory, "secret.txt"), "secret");
  const linkedAncestor = path.join(value.root, "source-link");
  try {
    fs.symlinkSync(outside, linkedAncestor, "dir");
  } catch (error) {
    if (["EACCES", "EPERM", "ENOTSUP"].includes(error.code)) {
      context.skip(`directory symlinks are unavailable: ${error.code}`);
      return;
    }
    throw error;
  }
  const retainedRoot = path.join(value.root, "retained-source-link");
  assert.throws(
    () => atomicallyRetainRollbackArtifact({
      sourceRoot: path.join(linkedAncestor, "previous"),
      retainedRoot,
      version: "1.2.3",
    }),
    /symbolic link/,
  );
  assert.equal(fs.existsSync(retainedRoot), false);
});

test("refuses unsafe rollback retention sources and destinations", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const outside = write(value.root, "outside/secret.txt", "do not copy through a link");
  const linkedSource = path.join(value.root, "linked-previous");
  fs.symlinkSync(value.previousRoot, linkedSource, "dir");
  assert.throws(
    () => atomicallyRetainRollbackArtifact({
      sourceRoot: linkedSource,
      retainedRoot: path.join(value.root, "retained"),
      version: "1.2.3",
    }),
    /symbolic link/,
  );

  const retainedRoot = path.join(value.root, "retained");
  fs.rmSync(retainedRoot, { recursive: true, force: true });
  fs.mkdirSync(retainedRoot);
  const linkedDestination = path.join(retainedRoot, "1.2.3");
  fs.symlinkSync(outside, linkedDestination);
  assert.throws(
    () => atomicallyRetainRollbackArtifact({
      sourceRoot: value.previousRoot,
      retainedRoot,
      version: "1.2.3",
    }),
    /refusing overwrite/,
  );

  assert.throws(
    () => atomicallyRetainRollbackArtifact({
      sourceRoot: value.previousRoot,
      retainedRoot: path.join(value.previousRoot, "retained"),
      version: "1.2.3",
    }),
    /inside rollback source/,
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
