import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import {
  assertPreparedDesktopReleaseInputs,
  desktopReleaseInputPaths,
  usesPreparedDesktopReleaseInputs,
} from "./desktop-release-inputs.mjs";

assert.equal(usesPreparedDesktopReleaseInputs({}), false);
assert.equal(usesPreparedDesktopReleaseInputs({ JFTRADE_DESKTOP_PREPARED: "1" }), true);
assert.throws(
  () => usesPreparedDesktopReleaseInputs({ JFTRADE_DESKTOP_PREPARED: "true" }),
  /must be 1 or unset/,
);

const rootDir = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-desktop-inputs-"));
try {
  assert.throws(
    () => assertPreparedDesktopReleaseInputs(rootDir),
    /input is missing/,
  );

  for (const relativePath of desktopReleaseInputPaths) {
    const inputPath = path.join(rootDir, relativePath);
    fs.mkdirSync(path.dirname(inputPath), { recursive: true });
    fs.writeFileSync(inputPath, "prepared\n", "utf8");
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
