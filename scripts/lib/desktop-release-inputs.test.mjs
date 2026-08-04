import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import {
  assertPreparedDesktopReleaseInputs,
  currentMarketDataSidecarAssetPath,
  desktopReleaseInputPaths,
  desktopReleaseInputPathsForCurrentPlatform,
  usesPreparedDesktopReleaseInputs,
} from "./desktop-release-inputs.mjs";

assert.equal(usesPreparedDesktopReleaseInputs({}), false);
assert.equal(usesPreparedDesktopReleaseInputs({ JFTRADE_DESKTOP_PREPARED: "1" }), true);
assert.throws(
  () => usesPreparedDesktopReleaseInputs({ JFTRADE_DESKTOP_PREPARED: "true" }),
  /must be 1 or unset/,
);
assert.equal(
  currentMarketDataSidecarAssetPath({
    environment: {},
    platform: "darwin",
    architecture: "arm64",
  }),
  "internal/marketdataassets/assets/bin/marketdata-sidecar-darwin-arm64",
);
assert.equal(
  currentMarketDataSidecarAssetPath({
    environment: { GOOS: "windows", GOARCH: "amd64" },
  }),
  "internal/marketdataassets/assets/bin/marketdata-sidecar-windows-amd64",
);
assert.throws(
  () =>
    currentMarketDataSidecarAssetPath({
      environment: {},
      platform: "freebsd",
      architecture: "x64",
    }),
  /Unsupported desktop market-data asset target/,
);

const rootDir = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-desktop-inputs-"));
try {
  assert.throws(
    () => assertPreparedDesktopReleaseInputs(rootDir),
    /input is missing/,
  );

  const currentPlatformInputs = desktopReleaseInputPathsForCurrentPlatform();
  assert.deepEqual(currentPlatformInputs.slice(0, -1), desktopReleaseInputPaths);
  for (const relativePath of currentPlatformInputs) {
    const inputPath = path.join(rootDir, relativePath);
    fs.mkdirSync(path.dirname(inputPath), { recursive: true });
    if (relativePath.startsWith("internal/marketdataassets/assets/bin/")) {
      fs.mkdirSync(inputPath, { recursive: true });
      const binaryBase = path.basename(relativePath);
      const extension = binaryBase.includes("-windows-") ? ".exe" : "";
      fs.writeFileSync(path.join(inputPath, `${binaryBase}${extension}`), "prepared\n", "utf8");
    } else {
      fs.writeFileSync(inputPath, "prepared\n", "utf8");
    }
  }
  assert.doesNotThrow(() => assertPreparedDesktopReleaseInputs(rootDir));

  const emptyInput = path.join(rootDir, desktopReleaseInputPaths[0]);
  fs.writeFileSync(emptyInput, "", "utf8");
  assert.throws(
    () => assertPreparedDesktopReleaseInputs(rootDir),
    /input is empty or invalid/,
  );
} finally {
  fs.rmSync(rootDir, { recursive: true, force: true });
}

console.log("desktop release input tests passed");
