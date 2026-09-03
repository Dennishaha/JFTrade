#!/usr/bin/env node
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import process from "node:process";

import { spawnChecked } from "./lib/spawn.mjs";
import { buildApiTransportCorpus } from "./compatibility/generate-api-transport-corpus.mjs";

const repoRoot = path.resolve(import.meta.dirname, "..");
export const trackedOutputs = Object.freeze([
  // The canonical OpenAPI document is an input, never a generated output.
  "apps/web/src/generated/openapi.ts",
]);

export async function generateContracts({ check = false } = {}) {
  const outputRoot = check
    ? await fs.mkdtemp(path.join(os.tmpdir(), "jftrade-contracts-"))
    : repoRoot;
  const canonicalOpenAPI = path.join(repoRoot, "contracts/openapi/openapi.json");
  const environment = {
    ...process.env,
    JFTRADE_GENERATED_ROOT: outputRoot,
    JFTRADE_OPENAPI_SOURCE: canonicalOpenAPI,
  };

  try {
    await assertGeneratedRouteSourcesMatch(repoRoot);
    run("node", ["scripts/generate-api-types.mjs"], environment);
    run("node", ["scripts/generate-docs.mjs"], environment);
    if (check) {
      await assertTrackedOutputsMatch(outputRoot);
      console.log("Generated contract check passed without modifying the worktree.");
    }
  } finally {
    if (check) await fs.rm(outputRoot, { recursive: true, force: true });
  }
}

export async function assertGeneratedRouteSourcesMatch(
  outputRoot,
  { expectedRoot = repoRoot } = {},
) {
  const generatedOpenAPI = await readJson(
    path.join(outputRoot, "contracts/openapi/openapi.json"),
    "generated OpenAPI",
  );
  const generatedCorpus = buildApiTransportCorpus(generatedOpenAPI);
  const trackedCorpus = await readJson(
    path.join(expectedRoot, "tests/fixtures/compatibility/api-transport/api-control-plane-corpus.json"),
    "tracked API transport corpus",
  );
  if (JSON.stringify(generatedCorpus) !== JSON.stringify(trackedCorpus)) {
    throw new Error(
      "Generated API transport corpus differs from tests/fixtures/compatibility/api-transport/api-control-plane-corpus.json",
    );
  }

  const manifest = await readJson(
    path.join(expectedRoot, "crates/jftrade-engine/src/product_production_route_manifest.json"),
    "Rust production route manifest",
  );
  assertRouteSetsMatch(generatedCorpus.routes, manifest.operations, "Rust production route manifest");
}

async function readJson(file, label) {
  try {
    return JSON.parse(await fs.readFile(file, "utf8"));
  } catch (error) {
    throw new Error(`Cannot read ${label} ${file}: ${error.message}`);
  }
}

function assertRouteSetsMatch(expectedRoutes, actualRoutes, label) {
  if (!Array.isArray(actualRoutes)) {
    throw new Error(`${label} must contain operations`);
  }
  const keys = (routes) => routes.map((route) => `${route.method} ${route.path}`).sort();
  const expected = keys(expectedRoutes);
  const actual = keys(actualRoutes);
  if (new Set(actual).size !== actual.length || JSON.stringify(expected) !== JSON.stringify(actual)) {
    throw new Error(`Generated OpenAPI and ${label} differ`);
  }
}

export async function assertTrackedOutputsMatch(
  outputRoot,
  { expectedRoot = repoRoot, outputs = trackedOutputs } = {},
) {
  const mismatches = [];
  for (const relativePath of outputs) {
    const expectedPath = path.join(expectedRoot, relativePath);
    const generatedPath = path.join(outputRoot, relativePath);
    const [expected, generated] = await Promise.all([
      fs.readFile(expectedPath),
      fs.readFile(generatedPath),
    ]);
    if (!expected.equals(generated)) {
      mismatches.push(relativePath);
    }
  }
  if (mismatches.length > 0) {
    throw new Error(`Generated outputs differ:\n${mismatches.map((file) => `- ${file}`).join("\n")}`);
  }
}

function run(command, args, env) {
  const status = spawnChecked(command, args, { cwd: repoRoot, env });
  if (status !== 0) {
    throw new Error(`${command} ${args.join(" ")} exited with status ${status}`);
  }
}

if (path.resolve(process.argv[1] ?? "") === path.resolve(import.meta.filename)) {
  generateContracts({ check: process.argv.includes("--check") }).catch((error) => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  });
}
