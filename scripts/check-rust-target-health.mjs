#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
export const codegenObjectLimit = 50_000;

export function countCodegenObjects(directory, limit = codegenObjectLimit, fileSystem = fs) {
  if (!fileSystem.existsSync(directory)) return 0;
  const handle = fileSystem.opendirSync(directory);
  let count = 0;
  try {
    for (;;) {
      const entry = handle.readSync();
      if (!entry) break;
      if (entry.isFile() && entry.name.endsWith(".rcgu.o")) {
        count += 1;
        if (count >= limit) break;
      }
    }
  } finally {
    handle.closeSync();
  }
  return count;
}

export function inspectRustTarget(root = repositoryRoot, limit = codegenObjectLimit) {
  const profiles = ["debug", "release"];
  const unhealthy = [];
  for (const profile of profiles) {
    const directory = path.join(root, "target", profile, "deps");
    const codegenObjects = countCodegenObjects(directory, limit);
    if (codegenObjects >= limit) unhealthy.push({ directory, codegenObjects });
  }
  return { healthy: unhealthy.length === 0, limit, unhealthy };
}

function main() {
  const result = inspectRustTarget();
  if (result.healthy) {
    console.log(`Rust target health passed: fewer than ${result.limit} intermediate .rcgu.o files per profile.`);
    return;
  }
  for (const entry of result.unhealthy) {
    console.error(
      `Rust target health failed: ${entry.directory} contains at least ${entry.codegenObjects} intermediate .rcgu.o files.`,
    );
  }
  console.error("Run `pnpm run clean:rust:artifacts` after confirming no Cargo process is active, then retry.");
  process.exitCode = 1;
}

if (path.resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  main();
}
