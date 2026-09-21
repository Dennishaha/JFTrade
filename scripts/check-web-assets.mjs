#!/usr/bin/env node
import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
const defaultDistRoot = path.join(repositoryRoot, "apps/web/dist");
const defaultManifestPath = path.join(repositoryRoot, "runtime-assets/web/manifest.json");

export const stagedWebAssetSchema = "jftrade.web-assets.v1";

// Content the packaged console must keep serving from its staged documentation
// pages. The license and notice text is asserted on the rendered pages, so a
// documentation build that silently drops or empties a legal page fails here.
export const stagedDocumentationRequirements = {
  "docs/legal/license.html": [
    "AGPL-3.0-only",
    "GNU AFFERO GENERAL PUBLIC LICENSE",
    "Copyright (C) 2026 JFTrade Contributors",
  ],
  "docs/legal/third-party-notices.html": [
    "pinets",
    "0.9.31",
    "github.com/c9s/bbgo@v1.64.2",
    "Permission is hereby granted, free of charge",
    "Version 2.0, January 2004",
    "END OF TERMS AND CONDITIONS",
    "Corresponding Source",
  ],
};

const textAssetExtensions = new Set([".html", ".js", ".css", ".json", ".txt", ".map"]);
const removedRuntimeReferences = [
  "pkg/strategy/pineruntime",
  "BenchmarkPineRuntime",
  "BenchmarkRunExecutesPineGoldenMatrix",
];

export function verifyStagedWebAssets({ distRoot, manifestPath }) {
  const failures = [];
  const staged = stagedFiles(distRoot, failures);
  if (staged === null) {
    return failures;
  }
  const manifest = readManifest(manifestPath, failures);
  if (manifest === null) {
    return failures;
  }
  if (manifest.schemaVersion !== stagedWebAssetSchema) {
    failures.push(
      `staged web manifest schema = ${JSON.stringify(manifest.schemaVersion)}, want ${stagedWebAssetSchema}`,
    );
  }
  const declared = new Map(
    (Array.isArray(manifest.files) ? manifest.files : []).map((entry) => [entry.path, entry.sha256]),
  );
  const stagedPaths = new Set(staged.map((file) => file.relative));
  for (const file of staged) {
    const digest = declared.get(file.relative);
    if (digest === undefined) {
      failures.push(`staged web asset is missing from the release manifest: ${file.relative}`);
    } else if (digest !== file.sha256) {
      failures.push(`release manifest digest differs for the staged asset: ${file.relative}`);
    }
  }
  for (const relative of declared.keys()) {
    if (!stagedPaths.has(relative)) {
      failures.push(`release manifest declares an asset that is not staged: ${relative}`);
    }
  }
  failures.push(...underscoreAssetFailures(staged, declared));
  failures.push(...documentationFailures(distRoot, stagedPaths));
  failures.push(...removedRuntimeFailures(distRoot, staged));
  return failures;
}

function underscoreAssetFailures(staged, declared) {
  const failures = [];
  for (const file of staged) {
    const name = path.posix.basename(file.relative);
    if (path.posix.dirname(file.relative) !== "assets" || !name.startsWith("_")) {
      continue;
    }
    if (!declared.has(file.relative)) {
      failures.push(`staged bundle asset with a leading underscore is not packaged: ${file.relative}`);
    }
  }
  return failures;
}

function documentationFailures(distRoot, stagedPaths) {
  const failures = [];
  for (const [relative, needles] of Object.entries(stagedDocumentationRequirements)) {
    if (!stagedPaths.has(relative)) {
      failures.push(`staged documentation page is missing: ${relative}`);
      continue;
    }
    const content = fs.readFileSync(path.join(distRoot, relative), "utf8");
    if (content.trim() === "") {
      failures.push(`staged documentation page is empty: ${relative}`);
      continue;
    }
    for (const needle of needles) {
      if (!content.includes(needle)) {
        failures.push(`staged documentation page ${relative} is missing ${JSON.stringify(needle)}`);
      }
    }
  }
  return failures;
}

function removedRuntimeFailures(distRoot, staged) {
  const failures = [];
  for (const file of staged) {
    if (file.relative.startsWith("docs/") || !textAssetExtensions.has(path.extname(file.relative))) {
      continue;
    }
    const content = fs.readFileSync(path.join(distRoot, file.relative), "utf8");
    for (const reference of removedRuntimeReferences) {
      if (content.includes(reference)) {
        failures.push(`staged text asset ${file.relative} still references ${reference}`);
      }
    }
  }
  return failures;
}

function stagedFiles(distRoot, failures = null) {
  try {
    return filesBelow(distRoot)
      .map((file) => ({
        absolute: file,
        relative: path.relative(distRoot, file).split(path.sep).join("/"),
        sha256: createHash("sha256").update(fs.readFileSync(file)).digest("hex"),
      }))
      .sort((left, right) => left.relative.localeCompare(right.relative));
  } catch (error) {
    if (failures === null) throw error;
    failures.push(`staged web assets directory is unreadable: ${error.message}`);
    return null;
  }
}

function filesBelow(directory) {
  return fs
    .readdirSync(directory, { withFileTypes: true })
    .flatMap((entry) => {
      const child = path.join(directory, entry.name);
      return entry.isDirectory() ? filesBelow(child) : entry.isFile() ? [child] : [];
    });
}

function readManifest(manifestPath, failures) {
  try {
    return JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  } catch (error) {
    failures.push(`staged web manifest is unreadable: ${error.message}`);
    return null;
  }
}

function parseArgs(arguments_) {
  const options = { distRoot: defaultDistRoot, manifestPath: defaultManifestPath };
  for (let index = 0; index < arguments_.length; index += 1) {
    const argument = arguments_[index];
    if (argument === "--dist") options.distRoot = path.resolve(arguments_[(index += 1)]);
    else if (argument === "--manifest") options.manifestPath = path.resolve(arguments_[(index += 1)]);
    else throw new Error(`unknown argument: ${argument}`);
  }
  return options;
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  try {
    const { distRoot, manifestPath } = parseArgs(process.argv.slice(2));
    const failures = verifyStagedWebAssets({ distRoot, manifestPath });
    if (failures.length > 0) {
      throw new Error(failures.map((failure) => `- ${failure}`).join("\n"));
    }
    console.log(`Staged web assets verified: ${stagedFiles(distRoot).length} files.`);
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
