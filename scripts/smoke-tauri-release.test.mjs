import assert from "node:assert/strict";
import path from "node:path";
import test from "node:test";

import { releaseBundlePaths } from "./smoke-tauri-release.mjs";

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
});
