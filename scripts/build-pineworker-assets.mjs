#!/usr/bin/env node
import { mkdirSync, readdirSync, rmSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { checkPinetsPackageAndLicense } from "./lib/pinets-package.mjs";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const outDir = resolve(process.env.JFTRADE_PINEWORKER_ASSET_OUT_DIR ?? join(rootDir, "internal/pineworkerassets/assets/bin"));
const workerEntry = join(rootDir, "workers/pineworker/src/main.ts");
const dryRun = process.env.JFTRADE_PINEWORKER_ASSET_BUILD_DRY_RUN === "1";

const outputName = "worker.mjs";

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
console.log(`Building platform-independent PineTS Node worker -> ${outputName}`);
if (dryRun) {
  console.log(`DRY RUN vite build --ssr ${workerEntry} --target node24 --format esm --outFile ${outFile} --noExternal`);
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
          entryFileNames: outputName,
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

async function loadViteBuild() {
  const requireFromWorker = createRequire(join(rootDir, "workers/pineworker/package.json"));
  const vitePath = requireFromWorker.resolve("vite");
  const vite = await import(pathToFileURL(vitePath).href);
  return vite.build;
}
