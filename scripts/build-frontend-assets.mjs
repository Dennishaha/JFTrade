#!/usr/bin/env node
import { cpSync, rmSync } from "node:fs";
import { join, resolve } from "node:path";
import { spawnChecked } from "./lib/spawn.mjs";

const rootDir = resolve(import.meta.dirname, "..");
const srcDir = join(rootDir, "apps/web/dist");
const dstDir = join(rootDir, "internal/frontendassets/dist");
const zipPath = join(rootDir, "internal/frontendassets/dist.zip");

run("npm", ["run", "build:web"]);
rmSync(dstDir, { recursive: true, force: true });
cpSync(srcDir, dstDir, { recursive: true });
run("go", ["run", "./scripts/archive_frontend_assets.go", "-src", "internal/frontendassets/dist", "-dst", "internal/frontendassets/dist.zip"]);

function run(command, args) {
  const status = spawnChecked(command, args, {
    cwd: rootDir,
  });
  if (status !== 0) {
    process.exit(status);
  }
}
