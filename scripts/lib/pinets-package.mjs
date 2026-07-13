import { readFileSync } from "node:fs";
import { join } from "node:path";
import { spawnChecked } from "./spawn.mjs";

export function checkPinetsPackageAndLicense(options = {}) {
  const rootDir = options.rootDir ?? process.cwd();
  const dryRun = options.dryRun === true;
  const verifyWorkspaceVisible = options.verifyWorkspaceVisible === true;

  console.log("==> Checking pinets package");
  const envStatus = process.env.JFTRADE_PINETS_RELEASE_PINETS_STATUS?.trim();
  let license = process.env.JFTRADE_PINETS_RELEASE_PINETS_LICENSE?.trim() ?? "";

  if (envStatus && envStatus !== "0") {
    console.error("BLOCKED: pinets package is not installed or not visible to the pnpm workspace");
    return false;
  }
  if (!envStatus && verifyWorkspaceVisible && !dryRun) {
    if (spawnChecked("pnpm", ["--filter", "@jftrade/pineworker", "list", "pinets", "--depth=0"], { cwd: rootDir }) !== 0) {
      console.error("BLOCKED: pinets package is not installed or not visible to the pnpm workspace");
      return false;
    }
  }
  if (!license) {
    try {
      const pkg = JSON.parse(readFileSync(join(rootDir, "workers/pineworker/node_modules/pinets/package.json"), "utf8"));
      if (pkg.name !== "pinets") {
        console.error(`BLOCKED: expected pinets package, got ${pkg.name}`);
        return false;
      }
      license = pkg.license || "";
    } catch {
      if (!envStatus) {
        console.error("BLOCKED: pinets package is not installed or not visible to the pnpm workspace");
        return false;
      }
    }
  }
  console.log(`==> pinets package license: ${license || "unknown"}`);
  return true;
}
