import { readFileSync } from "node:fs";
import { join } from "node:path";
import { spawnChecked } from "./spawn.mjs";

export function checkPinetsPackageAndLicense(options = {}) {
  const rootDir = options.rootDir ?? process.cwd();
  const dryRun = options.dryRun === true;
  const verifyNpmVisible = options.verifyNpmVisible === true;

  console.log("==> Checking pinets package");
  const envStatus = process.env.JFTRADE_PINETS_RELEASE_PINETS_STATUS?.trim();
  let license = process.env.JFTRADE_PINETS_RELEASE_PINETS_LICENSE?.trim() ?? "";

  if (envStatus && envStatus !== "0") {
    console.error("BLOCKED: pinets package is not installed or not visible to npm workspaces");
    return false;
  }
  if (!envStatus && verifyNpmVisible && !dryRun) {
    if (spawnChecked("npm", ["ls", "pinets", "--workspaces", "--depth=1"], { cwd: rootDir }) !== 0) {
      console.error("BLOCKED: pinets package is not installed or not visible to npm workspaces");
      return false;
    }
  }
  if (!license) {
    try {
      const pkg = JSON.parse(readFileSync(join(rootDir, "node_modules/pinets/package.json"), "utf8"));
      if (pkg.name !== "pinets") {
        console.error(`BLOCKED: expected pinets package, got ${pkg.name}`);
        return false;
      }
      license = pkg.license || "";
    } catch {
      if (!envStatus) {
        console.error("BLOCKED: pinets package is not installed or not visible to npm workspaces");
        return false;
      }
    }
  }
  console.log(`==> pinets package license: ${license || "unknown"}`);
  return true;
}
