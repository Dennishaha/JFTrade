#!/usr/bin/env node
import { mkdirSync, readdirSync, rmSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { runBun } from "./lib/bun.mjs";
import { checkPinetsPackageAndLicense } from "./lib/pinets-package.mjs";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const outDir = resolve(process.env.JFTRADE_PINEWORKER_ASSET_OUT_DIR ?? join(rootDir, "internal/pineworkerassets/assets/bin"));
const workerEntry = join(rootDir, "workers/pineworker/src/main.ts");
const dryRun = process.env.JFTRADE_PINEWORKER_ASSET_BUILD_DRY_RUN === "1";

const outputName = "worker.js";

if (!checkPinetsPackageAndLicense({ rootDir, dryRun })) {
  console.error("PineTS worker asset build is blocked until the pinets package is installed.");
  process.exit(1);
}

mkdirSync(outDir, { recursive: true });
for (const entry of readdirSync(outDir, { withFileTypes: true })) {
  if (entry.isFile() && entry.name !== ".gitkeep") {
    rmSync(join(outDir, entry.name), { force: true });
  }
}

const outFile = join(outDir, outputName);
console.log(`Building platform-independent PineTS Bun worker -> ${outputName}`);
if (dryRun) {
  console.log(`DRY RUN bun build --target=bun ${workerEntry} --outfile ${outFile}`);
} else {
  const result = runBun(["build", "--target=bun", workerEntry, "--outfile", outFile]);
  if (result.status !== 0) process.exit(result.status);
}
