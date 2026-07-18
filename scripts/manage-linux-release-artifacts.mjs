import fs from "node:fs";
import path from "node:path";
import process from "node:process";

import {
  linuxPackageExtension,
  linuxPackageFormats,
  linuxReleaseArtifactName,
} from "./lib/desktop-release-artifacts.mjs";

const [command, ...args] = process.argv.slice(2);

if (command === "finalize") {
  const [format, version, arch, sourceDir, releaseDir] = args;
  if (![format, version, arch, sourceDir, releaseDir].every(Boolean))
    fail(
      "Usage: manage-linux-release-artifacts.mjs finalize <format> <version> <arch> <source-dir> <release-dir>",
    );

  const extension = linuxPackageExtension(format);
  const matches = fs
    .readdirSync(sourceDir, { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(`.${extension}`));
  if (matches.length !== 1)
    fail(
      `Expected exactly one .${extension} artifact in ${sourceDir}, found ${matches.length}`,
    );

  const source = path.join(sourceDir, matches[0].name);
  const targetName = linuxReleaseArtifactName(version, arch, format);
  const target = path.join(releaseDir, targetName);
  fs.mkdirSync(releaseDir, { recursive: true });
  for (const entry of fs.readdirSync(releaseDir, { withFileTypes: true })) {
    if (entry.isFile() && entry.name.endsWith(`.${extension}`))
      fs.rmSync(path.join(releaseDir, entry.name), { force: true });
  }
  fs.renameSync(source, target);
  console.log(`Linux ${format} artifact finalized at ${target}`);
} else if (command === "verify") {
  const [version, arch, releaseDir] = args;
  if (![version, arch, releaseDir].every(Boolean))
    fail(
      "Usage: manage-linux-release-artifacts.mjs verify <version> <arch> <release-dir>",
    );

  const expected = linuxPackageFormats
    .map((format) => linuxReleaseArtifactName(version, arch, format))
    .sort();
  const actual = fs.readdirSync(releaseDir).sort();
  if (JSON.stringify(actual) !== JSON.stringify(expected))
    fail(
      `Unexpected Linux release entries in ${releaseDir}: expected ${expected.join(", ")}; found ${actual.join(", ")}`,
    );
  for (const name of expected) {
    const artifact = path.join(releaseDir, name);
    const stat = fs.statSync(artifact);
    if (!stat.isFile() || stat.size === 0)
      fail(`Linux release artifact is missing or empty: ${artifact}`);
  }
  console.log(`Verified Linux release artifacts in ${releaseDir}`);
} else {
  fail("Expected command: finalize or verify");
}

function fail(message) {
  console.error(message);
  process.exit(1);
}
