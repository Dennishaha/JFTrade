#!/usr/bin/env node
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import process from "node:process";

import { spawnChecked } from "./lib/spawn.mjs";

const repoRoot = path.resolve(import.meta.dirname, "..");
const trackedOutputs = [
	"docs/swagger/docs.go",
	"docs/swagger/swagger.json",
	"docs/swagger/swagger.yaml",
	"apps/web/src/generated/openapi.ts",
	"tests/fixtures/openapi-baseline.json",
	"docs/reference/generated/api.md",
	"docs/reference/generated/types.md",
	"docs/reference/generated/pine-v6-support.md",
];

export async function generateContracts({ check = false } = {}) {
  const outputRoot = check
    ? await fs.mkdtemp(path.join(os.tmpdir(), "jftrade-contracts-"))
    : repoRoot;
  const environment = {
    ...process.env,
    JFTRADE_GENERATED_ROOT: outputRoot,
    UPDATE_OPENAPI_SNAPSHOT: "1",
    JFTRADE_OPENAPI_BASELINE: path.join(outputRoot, "tests/fixtures/openapi-baseline.json"),
    JFTRADE_OPENAPI_SOURCE: path.join(outputRoot, "docs/swagger/swagger.json"),
  };

  try {
    run("go", ["generate", "./cmd/jftrade-api"], environment);
    run("node", ["scripts/generate-api-types.mjs"], environment);
    const runtimeSwaggerPath = await writeRuntimeSwagger(outputRoot);
    environment.JFTRADE_OPENAPI_SOURCE = runtimeSwaggerPath;
    run(
      "go",
      ["test", "./internal/app/apiserver/servercoretest", "-run", "^TestOpenAPISpecStable$", "-count=1"],
      environment,
    );
    run("node", ["scripts/generate-docs.mjs"], environment);
    if (check) {
      await assertTrackedOutputsMatch(outputRoot);
      console.log("Generated contract check passed without modifying the worktree.");
    }
  } finally {
    if (check) {
      await fs.rm(outputRoot, { recursive: true, force: true });
    }
  }
}

async function assertTrackedOutputsMatch(outputRoot) {
  const mismatches = [];
  for (const relativePath of trackedOutputs) {
    const expectedPath = path.join(repoRoot, relativePath);
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

async function writeRuntimeSwagger(outputRoot) {
  const sourcePath = path.join(outputRoot, "docs/swagger/swagger.json");
  const runtimePath = path.join(outputRoot, "docs/swagger/swagger.runtime.json");
  const document = JSON.parse(await fs.readFile(sourcePath, "utf8"));
  document.host = "";
  document.basePath = "/";
  await fs.writeFile(runtimePath, `${JSON.stringify(document, null, 2)}\n`, "utf8");
  return runtimePath;
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
