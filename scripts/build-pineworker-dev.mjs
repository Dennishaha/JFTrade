#!/usr/bin/env node
import { chmodSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { checkPinetsPackageAndLicense } from "./lib/pinets-package.mjs";
import { runBun } from "./lib/bun.mjs";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");

export function buildDevWorker(options = {}) {
  const outDir = resolve(process.env.JFTRADE_PINEWORKER_DEV_OUT_DIR ?? join(rootDir, "var/pineworker"));
  const envFile = process.env.JFTRADE_PINEWORKER_DEV_ENV_FILE?.trim() ?? "";
  const workerEntry = join(rootDir, "workers/pineworker/src/main.ts");
  const dryRun = process.env.JFTRADE_PINEWORKER_DEV_BUILD_DRY_RUN === "1";
  const target = currentTarget();
  const outPath = join(outDir, target.outputName);

  if (!checkPinetsPackageAndLicense({ rootDir, dryRun })) {
    throw new Error("PineTS dev worker build is blocked until the pinets package is installed.");
  }

  mkdirSync(outDir, { recursive: true });
  console.log(`Building PineTS dev worker ${target.bunTarget} -> ${target.outputName}`);
  if (dryRun) {
    console.log(`DRY RUN bun build --compile --target=${target.bunTarget} ${workerEntry} --outfile ${outPath}`);
  } else {
    const result = runBun(["build", "--compile", `--target=${target.bunTarget}`, workerEntry, "--outfile", outPath]);
    if (result.status !== 0) {
      process.exit(result.status);
    }
    chmodSync(outPath, 0o755);
  }

  if (envFile) {
    mkdirSync(dirname(envFile), { recursive: true });
    writeFileSync(envFile, `JFTRADE_PINEWORKER_BINARY=${outPath}\n`);
  }
  if (options.printPath !== false) {
    console.log(outPath);
  }
  return outPath;
}

function currentTarget() {
  const key = `${process.platform}:${process.arch}`;
  switch (key) {
    case "darwin:arm64":
      return { bunTarget: "bun-darwin-arm64", outputName: "worker-darwin-arm64" };
    case "darwin:x64":
      return { bunTarget: "bun-darwin-x64", outputName: "worker-darwin-x64" };
    case "linux:x64":
      return { bunTarget: "bun-linux-x64", outputName: "worker-linux-x64" };
    case "linux:arm64":
      return { bunTarget: "bun-linux-arm64", outputName: "worker-linux-arm64" };
    case "win32:x64":
      return { bunTarget: "bun-windows-x64", outputName: "worker-windows-x64.exe" };
    case "win32:arm64":
      return { bunTarget: "bun-windows-arm64", outputName: "worker-windows-arm64.exe" };
    default:
      throw new Error(`unsupported PineTS worker dev platform: ${process.platform}/${process.arch}`);
  }
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    buildDevWorker();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  }
}
