#!/usr/bin/env node
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { checkPinetsPackageAndLicense } from "./lib/pinets-package.mjs";
import { findBun, runBun } from "./lib/bun.mjs";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");

export function buildDevWorker(options = {}) {
  const outDir = resolve(process.env.JFTRADE_PINEWORKER_DEV_OUT_DIR ?? join(rootDir, "var/pineworker"));
  const envFile = process.env.JFTRADE_PINEWORKER_DEV_ENV_FILE?.trim() ?? "";
  const workerEntry = join(rootDir, "workers/pineworker/src/main.ts");
  const dryRun = process.env.JFTRADE_PINEWORKER_DEV_BUILD_DRY_RUN === "1";
  const outPath = join(outDir, "worker.js");

  if (!checkPinetsPackageAndLicense({ rootDir, dryRun })) {
    throw new Error("PineTS dev worker build is blocked until the pinets package is installed.");
  }

  mkdirSync(outDir, { recursive: true });
  console.log("Building PineTS dev worker Bun bundle -> worker.js");
  if (dryRun) {
    console.log(`DRY RUN bun build --target=bun ${workerEntry} --outfile ${outPath}`);
  } else {
    const result = runBun(["build", "--target=bun", workerEntry, "--outfile", outPath]);
    if (result.status !== 0) {
      process.exit(result.status);
    }
  }

  if (envFile) {
    mkdirSync(dirname(envFile), { recursive: true });
    writeFileSync(envFile, `JFTRADE_PINEWORKER_BUNDLE=${outPath}\nJFTRADE_PINEWORKER_RUNTIME=${bunRuntimePath()}\n`);
  }
  if (options.printPath !== false) {
    console.log(outPath);
  }
  return outPath;
}

export function bunRuntimePath() {
  return findBun() || process.env.JFTRADE_BUN_BINARY?.trim() || "bun";
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    buildDevWorker();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  }
}
