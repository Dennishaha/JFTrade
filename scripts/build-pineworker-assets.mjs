#!/usr/bin/env node
import { mkdirSync, readFileSync, readdirSync, rmSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { runBun } from "./lib/bun.mjs";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const outDir = resolve(process.env.JFTRADE_PINEWORKER_ASSET_OUT_DIR ?? join(rootDir, "internal/pineworkerassets/assets/bin"));
const workerEntry = join(rootDir, "workers/pineworker/src/main.ts");
const dryRun = process.env.JFTRADE_PINEWORKER_ASSET_BUILD_DRY_RUN === "1";

const targets = [
  ["bun-darwin-arm64", "worker-darwin-arm64"],
  ["bun-darwin-x64", "worker-darwin-x64"],
  ["bun-linux-x64", "worker-linux-x64"],
  ["bun-linux-arm64", "worker-linux-arm64"],
  ["bun-windows-x64", "worker-windows-x64.exe"],
  ["bun-windows-arm64", "worker-windows-arm64.exe"],
];

if (!checkPinetsPackageAndLicense()) {
  console.error("PineTS worker asset build is blocked until the pinets package is installed.");
  process.exit(1);
}

mkdirSync(outDir, { recursive: true });
for (const entry of readdirSync(outDir, { withFileTypes: true })) {
  if (entry.isFile() && entry.name !== ".gitkeep") {
    rmSync(join(outDir, entry.name), { force: true });
  }
}

for (const [bunTarget, outputName] of targets) {
  const outFile = join(outDir, outputName);
  console.log(`Building PineTS worker ${bunTarget} -> ${outputName}`);
  if (dryRun) {
    console.log(`DRY RUN bun build --compile --target=${bunTarget} ${workerEntry} --outfile ${outFile}`);
    continue;
  }
  const result = runBun(["build", "--compile", `--target=${bunTarget}`, workerEntry, "--outfile", outFile]);
  if (result.status !== 0) {
    process.exit(result.status);
  }
}

function checkPinetsPackageAndLicense() {
  console.log("==> Checking pinets package");
  const envStatus = process.env.JFTRADE_PINETS_RELEASE_PINETS_STATUS?.trim();
  const envLicense = process.env.JFTRADE_PINETS_RELEASE_PINETS_LICENSE?.trim();
  if (envStatus && envStatus !== "0") {
    console.error("BLOCKED: pinets package is not installed or not visible to npm workspaces");
    return false;
  }
  if (envStatus === "0") {
    console.log(`==> pinets package license: ${envLicense || "unknown"}`);
    return true;
  }

  try {
    const pkgPath = join(rootDir, "node_modules/pinets/package.json");
    const pkg = JSON.parse(readFileSync(pkgPath, "utf8"));
    if (pkg.name !== "pinets") {
      console.error(`BLOCKED: expected pinets package, got ${pkg.name}`);
      return false;
    }
    console.log(`==> pinets package license: ${pkg.license || "unknown"}`);
    return true;
  } catch {
    console.error("BLOCKED: pinets package is not installed or not visible to npm workspaces");
    return false;
  }
}
