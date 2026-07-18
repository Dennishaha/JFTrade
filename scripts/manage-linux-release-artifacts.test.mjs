import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";

const directory = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-linux-release-"));
const releaseDir = path.join(directory, "release");

try {
  for (const [format, generatedName] of [
    ["appimage", "generated-x86_64.AppImage"],
    ["deb", "jftrade.deb"],
    ["rpm", "jftrade.rpm"],
  ]) {
    const sourceDir = path.join(directory, format);
    fs.mkdirSync(sourceDir, { recursive: true });
    fs.writeFileSync(path.join(sourceDir, generatedName), `${format}\n`, "utf8");
    const result = run("finalize", format, "1.2.3", "amd64", sourceDir, releaseDir);
    assert.equal(result.status, 0, result.stderr);
  }

  assert.deepEqual(fs.readdirSync(releaseDir).sort(), [
    "JFTrade-1.2.3-linux-x64.AppImage",
    "JFTrade-1.2.3-linux-x64.deb",
    "JFTrade-1.2.3-linux-x64.rpm",
  ]);
  assert.equal(run("verify", "1.2.3", "amd64", releaseDir).status, 0);

  fs.writeFileSync(path.join(releaseDir, "jftrade-desktop-linux-amd64"), "raw\n");
  const unexpected = run("verify", "1.2.3", "amd64", releaseDir);
  assert.notEqual(unexpected.status, 0);
  assert.match(unexpected.stderr, /Unexpected Linux release entries/);
} finally {
  fs.rmSync(directory, { recursive: true, force: true });
}

function run(...args) {
  return spawnSync(
    process.execPath,
    ["scripts/manage-linux-release-artifacts.mjs", ...args],
    { encoding: "utf8" },
  );
}

console.log("Linux release artifact management tests passed");
