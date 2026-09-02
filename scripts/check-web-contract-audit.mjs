#!/usr/bin/env node

import {
  existsSync,
  readFileSync,
  readdirSync,
  statSync,
} from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import {
  classifiedDeclarationCounts,
  wireContractViolations,
  generatedSchemaViolations,
  normalizeRelativePath,
  viewModelClassificationViolations,
} from "./lib/web-contract-audit.mjs";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(scriptDir, "..");
const webSourceRoot = join(repoRoot, "apps/web/src");
const contractRoot = join(webSourceRoot, "contracts");
const spec = JSON.parse(
  readFileSync(join(repoRoot, "contracts/openapi/openapi.json"), "utf8"),
);
const generatedSource = readFileSync(
  join(webSourceRoot, "generated/openapi.ts"),
  "utf8",
);
const classification = JSON.parse(
  readFileSync(join(scriptDir, "web-contract-classification.json"), "utf8"),
);

const violations = generatedSchemaViolations(
  spec,
  generatedSource,
  "apps/web/src/generated/openapi.ts",
);

const rootEntries = readdirSync(contractRoot).sort();
for (const entry of rootEntries) {
  if (entry !== "wire" && entry !== "index.ts") {
    violations.push(
      `apps/web/src/contracts/${entry}: handwritten modules are forbidden outside the wire contract boundary`,
    );
  }
}

const wireSources = new Map(
  readdirSync(join(contractRoot, "wire"))
    .filter((file) => file.endsWith(".ts"))
    .sort()
    .map((file) => [
      file,
      readFileSync(join(contractRoot, "wire", file), "utf8"),
    ]),
);
violations.push(
  ...wireContractViolations({
    indexSource: readFileSync(join(contractRoot, "index.ts"), "utf8"),
    wireSources,
    schemaNames: new Set(Object.keys(spec.definitions ?? {})),
    pathNames: new Set(Object.keys(spec.paths ?? {})),
  }),
);

const classifiedSources = new Map();
for (const file of [
  join(webSourceRoot, "types/client-api.ts"),
  ...walkTypeScriptFiles(join(webSourceRoot, "types/view-models")),
]) {
  const key = normalizeRelativePath(relative(webSourceRoot, file));
  classifiedSources.set(key, readFileSync(file, "utf8"));
}
const adapterNames = new Set(
  Object.values(classification).flatMap((entry) => entry.adapters ?? []),
);
const adapterSources = new Map();
for (const name of adapterNames) {
  const path = join(webSourceRoot, name);
  if (existsSync(path)) adapterSources.set(name, readFileSync(path, "utf8"));
}
const testFiles = new Set(
  walkTypeScriptFiles(join(repoRoot, "apps/web/tests")).map((file) =>
    normalizeRelativePath(relative(join(repoRoot, "apps/web"), file)),
  ),
);
violations.push(
  ...viewModelClassificationViolations({
    classification,
    sources: classifiedSources,
    adapterSources,
    testFiles,
  }),
);

if (violations.length > 0) {
  console.error(`Web contract audit failed with ${violations.length} violation(s):`);
  for (const violation of violations.slice(0, 80)) {
    console.error(`- ${violation}`);
  }
  if (violations.length > 80) {
    console.error(`- ... ${violations.length - 80} more`);
  }
  process.exitCode = 1;
} else {
  const counts = classifiedDeclarationCounts(classification, classifiedSources);
  console.log(
    [
      `Web contract audit passed: ${Object.keys(spec.definitions ?? {}).length} Swagger schemas match generated TypeScript field-for-field`,
      `${wireSources.size} wire alias modules`,
      `${counts["normalized-api"]} normalized API declarations`,
      `${counts["ui-view-model"]} UI declarations`,
      `${counts["client-infrastructure"]} client infrastructure declarations`,
    ].join("; "),
  );
}

function walkTypeScriptFiles(root) {
  if (!existsSync(root)) return [];
  const files = [];
  for (const entry of readdirSync(root)) {
    const path = join(root, entry);
    if (statSync(path).isDirectory()) {
      files.push(...walkTypeScriptFiles(path));
    } else if (entry.endsWith(".ts")) {
      files.push(path);
    }
  }
  return files.sort();
}
