#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { lstatSync, realpathSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
export const backupRestoreDrillTimeoutMs = 300_000;
export const backupRestoreDrillTest =
  "product_data_management::tests::backup_restore_drill_tests::backup_restore_upgrade_corruption_and_rollback_are_fail_closed";

export function backupRestoreDrillCommand() {
  return {
    executable: "cargo",
    args: ["test", "--locked", "-p", "jftrade-engine", "--lib", backupRestoreDrillTest, "--", "--exact", "--nocapture"],
  };
}

function resolveRoot(root) {
  const resolved = realpathSync(path.resolve(root));
  if (!lstatSync(resolved).isDirectory()) throw new Error(`backup/restore drill root is not a directory: ${root}`);
  return resolved;
}

function resolveTimeout(timeoutMs) {
  if (!Number.isSafeInteger(timeoutMs) || timeoutMs <= 0 || timeoutMs > backupRestoreDrillTimeoutMs) {
    throw new Error(`backup/restore drill timeout must be a positive integer no greater than ${backupRestoreDrillTimeoutMs}: ${timeoutMs}`);
  }
  return timeoutMs;
}

export function runBackupRestoreDrill({
  root = repositoryRoot,
  runner = runCommand,
  timeoutMs = backupRestoreDrillTimeoutMs,
} = {}) {
  const resolvedRoot = resolveRoot(root);
  const resolvedTimeout = resolveTimeout(timeoutMs);
  runner(backupRestoreDrillCommand(), resolvedRoot, resolvedTimeout);
  return {
    schemaVersion: "jftrade.backup-restore-drill.v1",
    status: "passed",
    scope: "repository-rust-behavior",
    test: backupRestoreDrillTest,
    limitations: [
      "Native prior-version install, upgrade, rollback and retained-crash evidence is collected by release evidence workflows.",
    ],
  };
}

export function runCommand({ executable, args }, root, timeoutMs = backupRestoreDrillTimeoutMs) {
  const resolvedTimeout = resolveTimeout(timeoutMs);
  const result = spawnSync(executable, args, {
    cwd: root, encoding: "utf8", stdio: "inherit", timeout: resolvedTimeout, killSignal: "SIGTERM",
  });
  if (result.error?.code === "ETIMEDOUT") throw new Error(`${executable} timed out after ${resolvedTimeout}ms`);
  if (result.error) throw result.error;
  if (result.signal) throw new Error(`${executable} ${args.join(" ")} failed with signal ${result.signal}`);
  if (result.status !== 0) throw new Error(`${executable} ${args.join(" ")} failed with exit ${result.status}`);
}

export function main() {
  try {
    console.log(JSON.stringify(runBackupRestoreDrill(), null, 2));
    return 0;
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    return 1;
  }
}

if (path.resolve(process.argv[1] ?? "") === fileURLToPath(import.meta.url)) process.exitCode = main();
