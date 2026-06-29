#!/usr/bin/env node
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { build } from "esbuild";
import { checkPinetsPackageAndLicense } from "./lib/pinets-package.mjs";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");

export async function buildDevWorker(options = {}) {
  const outDir = resolve(process.env.JFTRADE_PINEWORKER_DEV_OUT_DIR ?? join(rootDir, "var/pineworker"));
  const envFile = process.env.JFTRADE_PINEWORKER_DEV_ENV_FILE?.trim() ?? "";
  const workerEntry = join(rootDir, "workers/pineworker/src/main.ts");
  const dryRun = process.env.JFTRADE_PINEWORKER_DEV_BUILD_DRY_RUN === "1";
  const outPath = join(outDir, "worker.mjs");

  if (!checkPinetsPackageAndLicense({ rootDir, dryRun })) {
    throw new Error("PineTS dev worker build is blocked until the pinets package is installed.");
  }

  mkdirSync(outDir, { recursive: true });
  console.log("Building PineTS dev worker Node bundle -> worker.mjs");
  if (dryRun) {
    console.log(`DRY RUN esbuild ${workerEntry} --bundle --platform=node --format=esm --target=node24 --outfile=${outPath}`);
  } else {
    await build({
      entryPoints: [workerEntry],
      bundle: true,
      platform: "node",
      format: "esm",
      target: "node24",
      outfile: outPath,
      banner: { js: 'import { createRequire } from "node:module"; const require = createRequire(import.meta.url);' },
      logLevel: "info",
    });
  }

  if (envFile) {
    mkdirSync(dirname(envFile), { recursive: true });
    writeFileSync(envFile, `JFTRADE_PINEWORKER_BUNDLE=${outPath}\nJFTRADE_PINEWORKER_RUNTIME=${nodeRuntimePath()}\n`);
  }
  if (options.printPath !== false) {
    console.log(outPath);
  }
  return outPath;
}

export function nodeRuntimePath() {
  return process.env.JFTRADE_NODE_BINARY?.trim() || process.execPath || "node";
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    await buildDevWorker();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  }
}
