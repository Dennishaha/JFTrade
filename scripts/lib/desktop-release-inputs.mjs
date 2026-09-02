import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";

export const desktopReleaseInputPaths = [
  "contracts/openapi/openapi.json",
  "internal/frontendassets/dist.zip",
  "internal/pineworkerassets/assets/bin/worker.mjs",
];

export const desktopReleaseInputManifestPath = "artifacts/desktop-release-inputs.json";
const desktopReleaseInputManifestSchema = "jftrade.desktop-release-inputs.v1";

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

export function currentMarketDataSidecarAssetPath({
  environment = process.env,
  platform = process.platform,
  architecture = process.arch,
} = {}) {
  const goos = platformNames[String(environment.GOOS || platform).trim()];
  const goarch =
    architectureNames[String(environment.GOARCH || architecture).trim()];
  if (!goos || !goarch) {
    throw new Error(
      `Unsupported desktop market-data asset target: ${environment.GOOS || platform}/${environment.GOARCH || architecture}`,
    );
  }
  return `internal/marketdataassets/assets/bin/marketdata-sidecar-${goos}-${goarch}`;
}

export function desktopReleaseInputPathsForCurrentPlatform(options = {}) {
  return [...desktopReleaseInputPaths, currentMarketDataSidecarAssetPath(options)];
}

export function usesPreparedDesktopReleaseInputs(environment = process.env) {
  const value = String(environment.JFTRADE_DESKTOP_PREPARED ?? "").trim();
  if (value === "") return false;
  if (value === "1") return true;
  throw new Error("JFTRADE_DESKTOP_PREPARED must be 1 or unset.");
}

export function assertPreparedDesktopReleaseInputs(rootDir, options = {}) {
  assertDesktopReleaseInputFiles(rootDir);
  const platformInputs = desktopReleaseInputPathsForCurrentPlatform(options).slice(
    desktopReleaseInputPaths.length,
  );
  for (const relativePath of platformInputs) {
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
    if (relativePath.startsWith("internal/marketdataassets/assets/bin/")) {
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

function assertDesktopReleaseInputFiles(rootDir) {
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

function sha256(filePath) {
  return createHash("sha256").update(fs.readFileSync(filePath)).digest("hex");
}

function expectedInputManifest(rootDir) {
  assertDesktopReleaseInputFiles(rootDir);
  return {
    schemaVersion: desktopReleaseInputManifestSchema,
    files: desktopReleaseInputPaths
      .map((relativePath) => {
        const stat = fs.statSync(path.join(rootDir, relativePath));
        return {
          path: relativePath,
          sha256: sha256(path.join(rootDir, relativePath)),
          size: stat.size,
        };
      })
      .sort((left, right) => left.path.localeCompare(right.path)),
  };
}

export function writeDesktopReleaseInputManifest(
  rootDir,
  outputPath = path.join(rootDir, desktopReleaseInputManifestPath),
) {
  const manifest = expectedInputManifest(rootDir);
  fs.mkdirSync(path.dirname(outputPath), { recursive: true });
  fs.writeFileSync(outputPath, `${JSON.stringify(manifest, null, 2)}\n`, "utf8");
  return manifest;
}

export function verifyDesktopReleaseInputManifest(
  rootDir,
  manifestPath = path.join(rootDir, desktopReleaseInputManifestPath),
) {
  let actual;
  try {
    actual = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  } catch (error) {
    throw new Error(`Prepared desktop release input manifest is unreadable: ${error.message}`);
  }
  const expected = expectedInputManifest(rootDir);
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error("Prepared desktop release input manifest is stale or mismatched");
  }
  return actual;
}
