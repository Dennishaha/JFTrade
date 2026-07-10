import fs from "node:fs";
import path from "node:path";

export const desktopReleaseInputPaths = [
  "docs/swagger/docs.go",
  "docs/swagger/swagger.json",
  "docs/swagger/swagger.yaml",
  "internal/frontendassets/dist.zip",
  "internal/pineworkerassets/assets/bin/worker.mjs",
];

export function usesPreparedDesktopReleaseInputs(environment = process.env) {
  const value = String(environment.JFTRADE_DESKTOP_PREPARED ?? "").trim();
  if (value === "") return false;
  if (value === "1") return true;
  throw new Error("JFTRADE_DESKTOP_PREPARED must be 1 or unset.");
}

export function assertPreparedDesktopReleaseInputs(rootDir) {
  for (const relativePath of desktopReleaseInputPaths) {
    const inputPath = path.join(rootDir, relativePath);
    let stat;
    try {
      stat = fs.statSync(inputPath);
    } catch (error) {
      if (error?.code === "ENOENT") {
        throw new Error(`Prepared desktop release input is missing: ${relativePath}`);
      }
      throw error;
    }
    if (!stat.isFile() || stat.size === 0) {
      throw new Error(`Prepared desktop release input is empty or invalid: ${relativePath}`);
    }
  }
}
