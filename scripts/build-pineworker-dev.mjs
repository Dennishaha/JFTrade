#!/usr/bin/env node
import { mkdirSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
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
    console.log(`DRY RUN vite build --ssr ${workerEntry} --target node24 --format esm --outFile ${outPath} --noExternal`);
  } else {
    const viteBuild = await loadViteBuild();
    await viteBuild({
      configFile: false,
      root: rootDir,
      logLevel: "info",
      build: {
        ssr: workerEntry,
        target: "node24",
        outDir,
        emptyOutDir: false,
        minify: false,
        rollupOptions: {
          output: {
            format: "es",
            entryFileNames: "worker.mjs",
            codeSplitting: false,
            banner: 'import { createRequire as __jftradeCreateRequire } from "node:module"; const require = __jftradeCreateRequire(import.meta.url);',
          },
        },
      },
      ssr: {
        target: "node",
        noExternal: true,
      },
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

async function loadViteBuild() {
  const requireFromWorker = createRequire(join(rootDir, "workers/pineworker/package.json"));
  const vitePath = requireFromWorker.resolve("vite");
  const vite = await import(pathToFileURL(vitePath).href);
  return vite.build;
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    await buildDevWorker();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  }
}
