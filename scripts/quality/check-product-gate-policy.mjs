#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

const fixedActiveFiles = Object.freeze([
  "package.json",
  "scripts/module-map.json",
  "AGENTS.md",
  "README.md",
  "docs/README.md",
  "docs/architecture.md",
]);

const activeDirectories = Object.freeze([
  { directory: ".github/workflows", extensions: new Set([".yml", ".yaml"]) },
  { directory: "docs/architecture", extensions: new Set([".md"]) },
]);

const forbiddenTerms = Object.freeze([
  { label: "numbered migration stage", pattern: /\bstage[\s_-]*[2-9]\b/i },
  { label: "migration differential", pattern: /\bmigration[\s_-]+differential\b/i },
  { label: "numbered differential gate", pattern: /\bstage[\s_-]*[2-9][^\n]*\bdifferential\b/i },
  { label: "migration script path", pattern: /\bscripts\/rust-migration\b/i },
  { label: "route ownership ledger", pattern: /\broute-ownership(?:\.json)?\b/i },
  { label: "committed closeout evidence", pattern: /\bcloseout-evidence(?:\.json)?\b/i },
  { label: "migration closeout field", pattern: /\b(?:hardCutReadiness|ownerDeletion|routeCutover|goRemovalStatus)\b/ },
  { label: "Go retirement gate", pattern: /\b(?:check:)?go-retirement\b/i },
]);

function walkFiles(root, relativeDirectory, extensions) {
  const absoluteDirectory = path.join(root, relativeDirectory);
  if (!fs.existsSync(absoluteDirectory)) return [];
  return fs.readdirSync(absoluteDirectory, { recursive: true, withFileTypes: true })
    .filter((entry) => entry.isFile() && extensions.has(path.extname(entry.name)))
    .map((entry) => path.relative(root, path.join(entry.parentPath, entry.name)).split(path.sep).join("/"));
}

export function activeProductGateFiles(root = repositoryRoot) {
  const discovered = activeDirectories.flatMap(({ directory, extensions }) => (
    walkFiles(root, directory, extensions)
  ));
  return [...new Set([...fixedActiveFiles, ...discovered])]
    .filter((file) => fs.existsSync(path.join(root, file)))
    .sort();
}

export function validateProductGateVocabulary(root = repositoryRoot) {
  const errors = [];
  for (const file of activeProductGateFiles(root)) {
    const lines = fs.readFileSync(path.join(root, file), "utf8").split(/\r?\n/);
    for (const [index, line] of lines.entries()) {
      for (const { label, pattern } of forbiddenTerms) {
        if (pattern.test(line)) errors.push(`${file}:${index + 1} contains ${label}`);
      }
    }
  }
  return errors;
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  const errors = validateProductGateVocabulary();
  if (errors.length > 0) {
    console.error(["Permanent product gate policy failed:", ...errors.map((error) => `- ${error}`)].join("\n"));
    process.exitCode = 1;
  } else {
    console.log("Permanent product gate policy passed: active configuration contains no migration-stage gate vocabulary.");
  }
}
