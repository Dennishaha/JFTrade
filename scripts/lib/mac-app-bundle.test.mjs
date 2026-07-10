import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { writeMacAppBundle } from "./mac-app-bundle.mjs";

const temporary = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-mac-bundle-"));
try {
  const binary = path.join(temporary, "desktop");
  fs.writeFileSync(binary, "binary", "utf8");

  const development = path.join(temporary, "JFTrade Dev.app");
  writeMacAppBundle(development, binary, { development: true });
  const developmentPlist = fs.readFileSync(
    path.join(development, "Contents", "Info.plist"),
    "utf8",
  );
  assert.match(
    developmentPlist,
    /<string>com\.jftrade\.desktop\.dev<\/string>/,
  );
  assert.match(developmentPlist, /<string>JFTrade Dev<\/string>/);
  assert.equal(
    fs.existsSync(path.join(development, "Contents", "MacOS", "JFTrade Dev")),
    true,
  );

  const release = path.join(temporary, "JFTrade.app");
  writeMacAppBundle(release, binary, {
    version: "1.2.3",
    commit: "abc123",
    buildTime: "2026-07-10T00:00:00Z",
  });
  const releasePlist = fs.readFileSync(
    path.join(release, "Contents", "Info.plist"),
    "utf8",
  );
  for (const expected of [
    "com.jftrade.desktop",
    "1.2.3",
    "abc123",
    "2026-07-10T00:00:00Z",
  ]) {
    assert.equal(releasePlist.includes(expected), true, `missing ${expected}`);
  }
} finally {
  fs.rmSync(temporary, { recursive: true, force: true });
}

console.log("mac app bundle tests passed");
