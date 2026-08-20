#!/usr/bin/env node

import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { currentMarketDataSidecarAssetPath } from "./lib/desktop-release-inputs.mjs";

const defaultRepositoryRoot = fileURLToPath(new URL("..", import.meta.url));
const protoNames = ["pineworker.proto", "pineworker_common.proto", "pineworker_types.proto"];

function sha256(filePath) {
  const hash = createHash("sha256");
  hash.update(fs.readFileSync(filePath));
  return hash.digest("hex");
}

function requiredFile(filePath, label) {
  let stat;
  try {
    stat = fs.statSync(filePath);
  } catch (error) {
    if (error?.code === "ENOENT") throw new Error(`${label} is missing: ${filePath}`);
    throw error;
  }
  if (!stat.isFile() || stat.size === 0) {
    throw new Error(`${label} is empty or invalid: ${filePath}`);
  }
  return filePath;
}

function filesBelow(directory) {
  return fs
    .readdirSync(directory, { withFileTypes: true })
    .flatMap((entry) => {
      const child = path.join(directory, entry.name);
      return entry.isDirectory() ? filesBelow(child) : [child];
    })
    .sort();
}

export function findNodeLicense(nodeExecutable = process.execPath) {
  let directory = path.dirname(nodeExecutable);
  for (let depth = 0; depth < 4; depth += 1) {
    const candidate = path.join(directory, "LICENSE");
    if (fs.existsSync(candidate)) return candidate;
    const parent = path.dirname(directory);
    if (parent === directory) break;
    directory = parent;
  }
  throw new Error(`Node LICENSE was not found above ${nodeExecutable}`);
}

export function releaseRuntimeInputs({
  repositoryRoot = defaultRepositoryRoot,
  environment = process.env,
  platform = process.platform,
  architecture = process.arch,
  nodeExecutable = process.execPath,
  nodeLicense = findNodeLicense(nodeExecutable),
  nodeVersion = process.version,
  runtimeDirectory = path.join(repositoryRoot, "var/tauri-runtime"),
} = {}) {
  const targetPlatform = String(environment.GOOS || platform).trim() === "win32"
    ? "windows"
    : String(environment.GOOS || platform).trim();
  const rawArchitecture = String(environment.GOARCH || architecture).trim();
  const targetArchitecture = rawArchitecture === "x64" ? "amd64" : rawArchitecture;
  const helperRelative = currentMarketDataSidecarAssetPath({
    environment,
    platform,
    architecture,
  });
  const helperDirectory = path.join(repositoryRoot, helperRelative);
  const helperBase = path.basename(helperDirectory);
  const helperExecutable = path.join(
    helperDirectory,
    `${helperBase}${helperBase.includes("-windows-") ? ".exe" : ""}`,
  );
  const nodeName = targetPlatform === "windows" ? "node.exe" : "node";
  return {
    architecture: targetArchitecture,
    helperDirectory,
    helperExecutable,
    nodeExecutable,
    nodeLicense,
    nodeName,
    nodeVersion,
    pineBundle: path.join(
      repositoryRoot,
      "internal/pineworkerassets/assets/bin/worker.mjs",
    ),
    platform: targetPlatform,
    protoDirectory: path.join(repositoryRoot, "pkg/strategy/pineworker/proto"),
    repositoryRoot,
    runtimeDirectory,
  };
}

function validateInputs(inputs) {
  requiredFile(inputs.nodeExecutable, "Node runtime");
  requiredFile(inputs.nodeLicense, "Node license");
  requiredFile(inputs.pineBundle, "PineTS worker bundle");
  requiredFile(inputs.helperExecutable, "market-data helper executable");
  for (const name of protoNames) {
    requiredFile(path.join(inputs.protoDirectory, name), `PineTS protobuf ${name}`);
  }
  const helperFiles = filesBelow(inputs.helperDirectory);
  if (helperFiles.length === 0) {
    throw new Error(`market-data helper directory is empty: ${inputs.helperDirectory}`);
  }
  return helperFiles;
}

function expectedManifest(inputs, nodePath, licensePath, helperFiles) {
  const file = (source, resource) => ({ resource, sha256: sha256(source) });
  return {
    schemaVersion: "jftrade.tauri-runtime.v1",
    target: {
      architecture: inputs.architecture,
      platform: inputs.platform,
    },
    nodeVersion: inputs.nodeVersion,
    files: [
      file(nodePath, `runtime/node/${inputs.nodeName}`),
      file(licensePath, "runtime/node/LICENSE-node.txt"),
      file(inputs.pineBundle, "runtime/pineworker/worker.mjs"),
      ...protoNames.map((name) =>
        file(path.join(inputs.protoDirectory, name), `runtime/pineworker/proto/${name}`),
      ),
      ...helperFiles.map((source) =>
        file(
          source,
          path
            .join(
              "runtime/marketdata",
              path.relative(path.dirname(inputs.helperDirectory), source),
            )
            .split(path.sep)
            .join("/"),
        ),
      ),
    ].sort((left, right) => left.resource.localeCompare(right.resource)),
  };
}

export function prepareTauriReleaseRuntime(options = {}) {
  const inputs = releaseRuntimeInputs(options);
  const helperFiles = validateInputs(inputs);
  fs.mkdirSync(inputs.runtimeDirectory, { recursive: true });
  const nodePath = path.join(inputs.runtimeDirectory, inputs.nodeName);
  const otherNodePath = path.join(
    inputs.runtimeDirectory,
    inputs.nodeName === "node" ? "node.exe" : "node",
  );
  fs.rmSync(otherNodePath, { force: true });
  fs.copyFileSync(inputs.nodeExecutable, nodePath);
  if (inputs.nodeName === "node") fs.chmodSync(nodePath, 0o755);
  const licensePath = path.join(inputs.runtimeDirectory, "LICENSE-node.txt");
  fs.copyFileSync(inputs.nodeLicense, licensePath);
  const manifest = expectedManifest(inputs, nodePath, licensePath, helperFiles);
  fs.writeFileSync(
    path.join(inputs.runtimeDirectory, "manifest.json"),
    `${JSON.stringify(manifest, null, 2)}\n`,
  );
  return manifest;
}

export function checkTauriReleaseRuntime(options = {}) {
  const inputs = releaseRuntimeInputs(options);
  const helperFiles = validateInputs(inputs);
  const nodePath = requiredFile(
    path.join(inputs.runtimeDirectory, inputs.nodeName),
    "prepared Node runtime",
  );
  const licensePath = requiredFile(
    path.join(inputs.runtimeDirectory, "LICENSE-node.txt"),
    "prepared Node license",
  );
  const manifestPath = requiredFile(
    path.join(inputs.runtimeDirectory, "manifest.json"),
    "Tauri runtime manifest",
  );
  const actual = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  const expected = expectedManifest(inputs, nodePath, licensePath, helperFiles);
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error("Tauri runtime manifest is stale; run prepare:tauri-release again");
  }
  return actual;
}

function isMainModule() {
  return process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url);
}

if (isMainModule()) {
  const checkOnly = process.argv.slice(2).includes("--check");
  const manifest = checkOnly
    ? checkTauriReleaseRuntime()
    : prepareTauriReleaseRuntime();
  console.log(
    `${checkOnly ? "Verified" : "Prepared"} Tauri runtime ${manifest.nodeVersion} with ${manifest.files.length} signed resource inputs.`,
  );
}
