import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { releaseBundlePaths, writeTauriSmokeReport } from "./smoke-tauri-release.mjs";

test("resolves the native executable and resource root for supported bundles", () => {
  const root = path.resolve("fixture-root");
  const mac = releaseBundlePaths({ root, platform: "darwin", executableOverride: "" });
  assert.equal(
    mac.executable,
    path.join(root, "target/release/bundle/macos/JFTrade.app/Contents/MacOS/jftrade-desktop"),
  );
  assert.equal(
    mac.resourceRoot,
    path.join(root, "target/release/bundle/macos/JFTrade.app/Contents/Resources"),
  );
  assert.equal(
    releaseBundlePaths({ root, platform: "win32", executableOverride: "" }).executable,
    path.join(root, "target/release/jftrade-desktop.exe"),
  );
  assert.equal(
    releaseBundlePaths({ root, platform: "linux", executableOverride: "" }).executable,
    path.join(root, "target/release/jftrade-desktop"),
  );
  const linuxOverride = releaseBundlePaths({
    root,
    platform: "linux",
    executableOverride: path.join(root, "target/release/jftrade-desktop"),
  });
  assert.equal(linuxOverride.resourceRoot, path.join(root, "target/release"));
  const macOverride = releaseBundlePaths({
    root,
    platform: "darwin",
    executableOverride: path.join(
      root,
      "target/release/bundle/macos/JFTrade.app/Contents/MacOS/jftrade-desktop",
    ),
  });
  assert.equal(
    macOverride.resourceRoot,
    path.join(root, "target/release/bundle/macos/JFTrade.app/Contents/Resources"),
  );
});

test("writes a machine-readable smoke report without asserting external platform qualification", () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-tauri-smoke-report-"));
  try {
    const reportPath = path.join(directory, "artifacts/report.json");
    const report = {
      schemaVersion: "jftrade.tauri-runtime-smoke.v1",
      externalRequired: ["native package installation and rollback"],
    };
    writeTauriSmokeReport(reportPath, report);
    assert.deepEqual(JSON.parse(fs.readFileSync(reportPath, "utf8")), report);
  } finally {
    fs.rmSync(directory, { recursive: true, force: true });
  }
});
