#!/usr/bin/env node
import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { spawnChecked } from "./lib/spawn.mjs";

const rootDir = resolve(import.meta.dirname, "..");
const srcDir = join(rootDir, "apps/web/dist");
const manifestPath = join(rootDir, "runtime-assets/web/manifest.json");

run("pnpm", ["run", "build:web:generated"]);
run("pnpm", ["run", "build:docs:generated"]);
run("pnpm", ["run", "stage:docs"]);
run("pnpm", ["--filter", "@jftrade/web", "run", "test:production-bundle"]);
writeWebManifest(srcDir, manifestPath);

function writeWebManifest(directory, outputPath) {
  const files = filesBelow(directory).map((file) => ({
    path: file.slice(directory.length + 1).split("\\").join("/"),
    sha256: createHash("sha256").update(readFileSync(file)).digest("hex"),
  }));
  mkdirSync(dirname(outputPath), { recursive: true });
  writeFileSync(outputPath, `${JSON.stringify({ schemaVersion: "jftrade.web-assets.v1", files }, null, 2)}\n`);
}

function filesBelow(directory) {
  return readdirSync(directory, { withFileTypes: true })
    .flatMap((entry) => {
      const child = join(directory, entry.name);
      return entry.isDirectory() ? filesBelow(child) : [child];
    })
    .sort();
}

function run(command, args) {
  const status = spawnChecked(command, args, {
    cwd: rootDir,
  });
  if (status !== 0) {
    process.exit(status);
  }
}
