#!/usr/bin/env node

import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { lstatSync, readFileSync, realpathSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
export const backupRestoreDrillTimeoutMs = 300_000;
export const closeoutManifestRelativePath =
  "tests/fixtures/rust-migration/stage9/closeout-evidence.json";
export const backupRestoreDrillTest =
  "product_data_management::tests::backup_restore_drill_tests::backup_restore_upgrade_corruption_and_rollback_are_fail_closed";

export function backupRestoreDrillCommand() {
  return {
    executable: "cargo",
    args: [
      "test",
      "--locked",
      "-p",
      "jftrade-engine",
      "--lib",
      backupRestoreDrillTest,
      "--",
      "--exact",
      "--nocapture",
    ],
  };
}

function resolveRoot(root) {
  const requested = path.resolve(root);
  let resolved;
  try {
    resolved = realpathSync(requested);
  } catch (error) {
    throw new Error(`backup/restore drill root is unavailable: ${requested}: ${error.message}`, {
      cause: error,
    });
  }
  const stat = lstatSync(resolved);
  if (!stat.isDirectory()) {
    throw new Error(`backup/restore drill root is not a directory: ${requested}`);
  }
  return resolved;
}

function resolveTimeout(timeoutMs) {
  if (
    !Number.isSafeInteger(timeoutMs)
    || timeoutMs <= 0
    || timeoutMs > backupRestoreDrillTimeoutMs
  ) {
    throw new Error(
      `backup/restore drill timeout must be a positive integer no greater than ${backupRestoreDrillTimeoutMs}: ${timeoutMs}`,
    );
  }
  return timeoutMs;
}

function manifestPath(root) {
  const resolved = path.resolve(root, closeoutManifestRelativePath);
  const relative = path.relative(root, resolved);
  if (relative === ".." || relative.startsWith(`..${path.sep}`)) {
    throw new Error("backup/restore drill closeout manifest escaped its root");
  }
  let current = root;
  for (const segment of closeoutManifestRelativePath.split("/")) {
    current = path.join(current, segment);
    try {
      if (lstatSync(current).isSymbolicLink()) {
        throw new Error("backup/restore drill closeout manifest must not traverse a symlink");
      }
    } catch (error) {
      if (error.code === "ENOENT") break;
      throw error;
    }
  }
  return resolved;
}

function readManifestSnapshot(filePath) {
  let stat;
  try {
    stat = lstatSync(filePath);
  } catch (error) {
    throw new Error(`closeout manifest is unavailable: ${filePath}: ${error.message}`, {
      cause: error,
    });
  }
  if (stat.isSymbolicLink() || !stat.isFile()) {
    throw new Error(`closeout manifest must be a regular file: ${filePath}`);
  }
  const bytes = readFileSync(filePath);
  return {
    digest: createHash("sha256").update(bytes).digest("hex"),
    bytes,
  };
}

const manifestChangedMessage =
  "backup/restore drill changed the Stage 9 closeout manifest; evidence must be recorded separately";

function manifestInvariantError(filePath, before) {
  try {
    const after = readManifestSnapshot(filePath);
    if (before.digest === after.digest && before.bytes.equals(after.bytes)) return null;
    return new Error(manifestChangedMessage);
  } catch (error) {
    return new Error(`${manifestChangedMessage}: ${error.message}`, { cause: error });
  }
}

function errorMessage(error) {
  return error instanceof Error ? error.message : String(error);
}

export function runBackupRestoreDrill({
  root = repositoryRoot,
  runner = runCommand,
  timeoutMs = backupRestoreDrillTimeoutMs,
} = {}) {
  const resolvedRoot = resolveRoot(root);
  const resolvedTimeout = resolveTimeout(timeoutMs);
  const manifest = manifestPath(resolvedRoot);
  const before = readManifestSnapshot(manifest);
  const command = backupRestoreDrillCommand();
  let runnerError;
  let runnerFailed = false;
  try {
    runner(command, resolvedRoot, resolvedTimeout);
  } catch (error) {
    runnerError = error;
    runnerFailed = true;
  }
  const manifestError = manifestInvariantError(manifest, before);
  if (runnerFailed && manifestError) {
    throw new AggregateError(
      [runnerError, manifestError],
      `backup/restore drill failed: ${errorMessage(runnerError)}; ${manifestError.message}`,
    );
  }
  if (manifestError) throw manifestError;
  if (runnerFailed) throw runnerError;
}

export function runCommand({ executable, args }, root, timeoutMs = backupRestoreDrillTimeoutMs) {
  const resolvedTimeout = resolveTimeout(timeoutMs);
  const result = spawnSync(executable, args, {
    cwd: root,
    encoding: "utf8",
    stdio: "inherit",
    timeout: resolvedTimeout,
    killSignal: "SIGTERM",
  });
  if (result.error?.code === "ETIMEDOUT") {
    throw new Error(`${executable} timed out after ${resolvedTimeout}ms`);
  }
  if (result.error) throw result.error;
  if (result.signal) {
    throw new Error(`${executable} ${args.join(" ")} failed with signal ${result.signal}`);
  }
  if (result.status !== 0) {
    throw new Error(`${executable} ${args.join(" ")} failed with exit ${result.status}`);
  }
}

export function main() {
  try {
    runBackupRestoreDrill();
    console.log(
      "Repository-local Rust database backup/restore drill passed; this is not prior-version, four-platform, or retained worker crash recovery evidence.",
    );
    return 0;
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    return 1;
  }
}

if (path.resolve(process.argv[1] ?? "") === fileURLToPath(import.meta.url)) {
  process.exitCode = main();
}
