#!/usr/bin/env node
import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const allowedKeys = new Set(["schemaVersion", "version", "capability", "sourceRelease", "files"]);

export function validateCompatibilityManifests(root = repositoryRoot) {
  const fixtureRoot = path.join(root, "tests/fixtures/compatibility");
  const errors = [];
  for (const capability of fs.readdirSync(fixtureRoot).sort()) {
    const manifestPath = path.join(fixtureRoot, capability, "manifest.json");
    if (!fs.existsSync(manifestPath)) {
      errors.push(`${capability}: manifest.json is missing`);
      continue;
    }
    const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
    for (const key of Object.keys(manifest)) if (!allowedKeys.has(key)) errors.push(`${capability}: unsupported manifest field ${key}`);
    if (manifest.schemaVersion !== "jftrade.compatibility-manifest.v1") errors.push(`${capability}: invalid schemaVersion`);
    if (manifest.capability !== capability) errors.push(`${capability}: capability field mismatch`);
    if (!/^v\d+\.\d+\.\d+$/.test(manifest.sourceRelease ?? "")) errors.push(`${capability}: sourceRelease must be a release tag`);
    if (!Array.isArray(manifest.files) || manifest.files.length === 0) {
      errors.push(`${capability}: files must be non-empty`);
      continue;
    }
    for (const entry of manifest.files) {
      if (!/^[A-Za-z0-9._-]+$/.test(entry?.path ?? "")) {
        errors.push(`${capability}: unsafe fixture path ${String(entry?.path)}`);
        continue;
      }
      const fixturePath = path.join(fixtureRoot, capability, entry.path);
      if (!fs.existsSync(fixturePath)) {
        errors.push(`${capability}/${entry.path}: fixture is missing`);
        continue;
      }
      const digest = createHash("sha256").update(fs.readFileSync(fixturePath)).digest("hex");
      if (digest !== entry.sha256) errors.push(`${capability}/${entry.path}: SHA-256 mismatch`);
    }
  }
  return errors;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const errors = validateCompatibilityManifests();
  if (errors.length > 0) {
    console.error(errors.map((error) => `- ${error}`).join("\n"));
    process.exitCode = 1;
  } else {
    console.log("Compatibility fixture manifests and frozen SHA-256 digests passed.");
  }
}
