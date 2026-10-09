#!/usr/bin/env node
import { createHash } from "node:crypto";
import { chmod, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { spawn } from "node:child_process";
import { join, resolve } from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { rustCompileEnvironment } from "../lib/tauri-runtime.mjs";

export const nextestVersion = "0.9.145";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const releases = Object.freeze({
  "darwin-arm64": Object.freeze({
    target: "universal-apple-darwin",
    sha256: "52ecaedb4f5af9267ef7ed02bc937d2a15a94ff96cb663080e81311f798c9905",
  }),
  "darwin-x64": Object.freeze({
    target: "universal-apple-darwin",
    sha256: "52ecaedb4f5af9267ef7ed02bc937d2a15a94ff96cb663080e81311f798c9905",
  }),
  "linux-arm64": Object.freeze({
    target: "aarch64-unknown-linux-gnu",
    sha256: "0ad2815fd91a7ecec3a7e25c749b584f66729ac688fd26b2fd494f7ff94b7fd0",
  }),
  "linux-x64": Object.freeze({
    target: "x86_64-unknown-linux-gnu",
    sha256: "32aa82416099eb12fffae9cf1a279ad201fecbd3f74826c613e32e9006b29867",
  }),
  "win32-arm64": Object.freeze({
    target: "aarch64-pc-windows-msvc",
    sha256: "29614accc4b232b0fabdda1805d449bae3130a8de6cb343a5b2584e0e886cf0a",
  }),
  "win32-x64": Object.freeze({
    target: "x86_64-pc-windows-msvc",
    sha256: "bfecd545af057adb83be7ddfe135fd067fb9c990544da1ea111e675f45d628a9",
  }),
});

export function releaseFor(platform = process.platform, arch = process.arch) {
  const release = releases[`${platform}-${arch}`];
  if (!release) throw new Error(`cargo-nextest ${nextestVersion} does not support ${platform}-${arch}`);
  const archiveName = `cargo-nextest-${nextestVersion}-${release.target}.tar.gz`;
  return Object.freeze({
    ...release,
    archiveName,
    url: `https://github.com/nextest-rs/nextest/releases/download/cargo-nextest-${nextestVersion}/${archiveName}`,
  });
}

export function sha256(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

export function normalizePnpmArguments(args) {
  const separator = args.indexOf("--");
  return separator < 0 ? [...args] : [...args.slice(0, separator), ...args.slice(separator + 1)];
}

function run(command, args, options = {}) {
  return new Promise((complete, reject) => {
    const child = spawn(command, args, {
      cwd: repositoryRoot,
      env: rustCompileEnvironment(process.env),
      stdio: options.capture ? ["ignore", "pipe", "inherit"] : "inherit",
    });
    let stdout = "";
    if (options.capture) child.stdout.on("data", (chunk) => { stdout += chunk; });
    child.once("error", reject);
    child.once("close", (status, signal) => {
      if (signal) reject(new Error(`${command} terminated by ${signal}`));
      else complete({ status: status ?? 1, stdout });
    });
  });
}

async function downloadArchive(release, archivePath) {
  let bytes;
  try {
    bytes = await readFile(archivePath);
  } catch (error) {
    if (error?.code !== "ENOENT") throw error;
    const response = await fetch(release.url, { redirect: "follow" });
    if (!response.ok) throw new Error(`failed to download ${release.url}: HTTP ${response.status}`);
    bytes = Buffer.from(await response.arrayBuffer());
    await writeFile(archivePath, bytes, { flag: "wx" }).catch(async (writeError) => {
      if (writeError?.code !== "EEXIST") throw writeError;
      bytes = await readFile(archivePath);
    });
  }
  const actual = sha256(bytes);
  if (actual !== release.sha256) {
    await rm(archivePath, { force: true });
    throw new Error(`cargo-nextest archive checksum mismatch: expected ${release.sha256}, got ${actual}`);
  }
}

export async function ensureCargoNextest() {
  const release = releaseFor();
  const cacheRoot = join(repositoryRoot, "target", "jftrade-tools", `cargo-nextest-${nextestVersion}`);
  const archivePath = join(cacheRoot, release.archiveName);
  const binaryName = process.platform === "win32" ? "cargo-nextest.exe" : "cargo-nextest";
  const binaryPath = join(cacheRoot, release.target, binaryName);
  await mkdir(cacheRoot, { recursive: true });
  let versionResult;
  try {
    versionResult = await run(binaryPath, ["nextest", "--version"], { capture: true });
  } catch (error) {
    if (error?.code !== "ENOENT") throw error;
  }
  if (versionResult?.status === 0 && versionResult.stdout.includes(`cargo-nextest ${nextestVersion}`)) {
    return binaryPath;
  }

  await downloadArchive(release, archivePath);
  const installRoot = join(cacheRoot, release.target);
  await rm(installRoot, { recursive: true, force: true });
  await mkdir(installRoot, { recursive: true });
  const extracted = await run("tar", ["-xzf", archivePath, "-C", installRoot]);
  if (extracted.status !== 0) throw new Error(`failed to extract ${release.archiveName}`);
  if (process.platform !== "win32") await chmod(binaryPath, 0o755);
  versionResult = await run(binaryPath, ["nextest", "--version"], { capture: true });
  if (versionResult.status !== 0 || !versionResult.stdout.includes(`cargo-nextest ${nextestVersion}`)) {
    throw new Error(`extracted cargo-nextest is not version ${nextestVersion}`);
  }
  return binaryPath;
}

export async function runCargoNextest(args) {
  args = normalizePnpmArguments(args);
  if (args.length === 0) throw new Error("Usage: cargo-nextest.mjs <run|archive|list> [arguments]");
  const binaryPath = await ensureCargoNextest();
  const result = await run(binaryPath, ["nextest", ...args]);
  return result.status;
}

async function main() {
  try {
    process.exitCode = await runCargoNextest(process.argv.slice(2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}

if (resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) await main();
