import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  inspectTauriReleaseArtifacts,
  writeTauriReleaseArtifactManifest,
} from "./verify-tauri-release-artifacts.mjs";

function fixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-tauri-artifacts-"));
  const bundle = path.join(root, "apps/desktop/src-tauri/target/release/bundle");
  const write = (relativePath, contents) => {
    const filePath = path.join(bundle, relativePath);
    fs.mkdirSync(path.dirname(filePath), { recursive: true });
    fs.writeFileSync(filePath, contents);
  };
  write("appimage/JFTrade_1.2.3_amd64.AppImage", "appimage");
  write("deb/JFTrade_1.2.3_amd64.deb", "deb");
  write("rpm/JFTrade-1.2.3-1.x86_64.rpm", "rpm");
  write("updater/JFTrade_1.2.3_amd64.AppImage.sig", "signature");
  write("updater/JFTrade_1.2.3_amd64.AppImage.tar.gz", "archive");
  return { bundle, cleanup: () => fs.rmSync(root, { recursive: true, force: true }), root };
}

test("records exact Tauri package hashes and explicit qualification limits", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const manifest = writeTauriReleaseArtifactManifest({
    root: value.root,
    bundleRoot: value.bundle,
    platform: "linux-x64",
    architecture: "amd64",
    version: "1.2.3",
    outputPath: path.join(value.root, "artifacts/release.json"),
  });

  assert.equal(manifest.schemaVersion, "jftrade.tauri-release-artifacts.v1");
  assert.deepEqual(
    manifest.packages.map((entry) => entry.kind),
    ["appimage", "deb", "rpm"],
  );
  assert.equal(manifest.updaterSignatures.length, 1);
  assert.equal(manifest.updaterArchives.length, 1);
  assert.match(manifest.limitations.join(" "), /native install\/upgrade\/uninstall\/rollback/);
  assert.deepEqual(
    JSON.parse(fs.readFileSync(path.join(value.root, "artifacts/release.json"), "utf8")),
    manifest,
  );
});

test("requires every package and updater signature for a publish inspection", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  fs.rmSync(path.join(value.bundle, "rpm"), { recursive: true });
  assert.throws(
    () => inspectTauriReleaseArtifacts({
      bundleRoot: value.bundle,
      platform: "linux-x64",
      architecture: "amd64",
      version: "1.2.3",
      requireUpdater: true,
    }),
    /rpm package count is 0/,
  );
  fs.mkdirSync(path.join(value.bundle, "rpm"), { recursive: true });
  fs.writeFileSync(path.join(value.bundle, "rpm/JFTrade-1.2.3-1.x86_64.rpm"), "rpm");
  fs.rmSync(path.join(value.bundle, "updater"), { recursive: true });
  fs.mkdirSync(path.join(value.bundle, "updater"), { recursive: true });
  fs.writeFileSync(path.join(value.bundle, "updater/JFTrade_1.2.3_amd64.AppImage.sig"), "signature");
  assert.throws(
    () => inspectTauriReleaseArtifacts({
      bundleRoot: value.bundle,
      platform: "linux-x64",
      architecture: "amd64",
      version: "1.2.3",
      requireUpdater: true,
    }),
    /requires Tauri updater archive\(s\) and signature\(s\)/,
  );
});
