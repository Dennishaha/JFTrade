import fs from "node:fs";
import path from "node:path";

export const desktopReleaseInputPaths = [
  "docs/swagger/docs.go",
  "docs/swagger/swagger.json",
  "docs/swagger/swagger.yaml",
  "internal/frontendassets/dist.zip",
  "internal/pineworkerassets/assets/bin/worker.mjs",
];

const platformNames = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows",
  windows: "windows",
};
const architectureNames = {
  arm64: "arm64",
  amd64: "amd64",
  x64: "amd64",
};

export function currentYFinanceSidecarAssetPath({
  environment = process.env,
  platform = process.platform,
  architecture = process.arch,
} = {}) {
  const goos = platformNames[String(environment.GOOS || platform).trim()];
  const goarch =
    architectureNames[String(environment.GOARCH || architecture).trim()];
  if (!goos || !goarch) {
    throw new Error(
      `Unsupported desktop yfinance asset target: ${environment.GOOS || platform}/${environment.GOARCH || architecture}`,
    );
  }
  return `internal/yfinanceassets/assets/bin/yfinance-sidecar-${goos}-${goarch}`;
}

export function desktopReleaseInputPathsForCurrentPlatform(options = {}) {
  return [...desktopReleaseInputPaths, currentYFinanceSidecarAssetPath(options)];
}

export function usesPreparedDesktopReleaseInputs(environment = process.env) {
  const value = String(environment.JFTRADE_DESKTOP_PREPARED ?? "").trim();
  if (value === "") return false;
  if (value === "1") return true;
  throw new Error("JFTRADE_DESKTOP_PREPARED must be 1 or unset.");
}

export function assertPreparedDesktopReleaseInputs(rootDir, options = {}) {
  for (const relativePath of desktopReleaseInputPathsForCurrentPlatform(options)) {
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
    if (relativePath.startsWith("internal/yfinanceassets/assets/bin/")) {
      if (!stat.isDirectory()) {
        throw new Error(`Prepared desktop release input is empty or invalid: ${relativePath}`);
      }
      const binaryBase = path.basename(relativePath);
      const extension = binaryBase.includes("-windows-") ? ".exe" : "";
      const executablePath = path.join(inputPath, `${binaryBase}${extension}`);
      let executable;
      try {
        executable = fs.statSync(executablePath);
      } catch (error) {
        if (error?.code === "ENOENT") {
          throw new Error(`Prepared desktop release input is missing: ${relativePath}/${path.basename(executablePath)}`);
        }
        throw error;
      }
      if (!executable.isFile() || executable.size === 0) {
        throw new Error(`Prepared desktop release input is empty or invalid: ${relativePath}/${path.basename(executablePath)}`);
      }
      continue;
    }
    if (!stat.isFile() || stat.size === 0) {
      throw new Error(`Prepared desktop release input is empty or invalid: ${relativePath}`);
    }
  }
}
