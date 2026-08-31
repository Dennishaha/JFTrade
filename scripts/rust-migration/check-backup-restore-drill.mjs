#!/usr/bin/env node

import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const closeoutManifest = path.join(
  repositoryRoot,
  "tests/fixtures/rust-migration/stage9/closeout-evidence.json",
);
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

function digest(filePath) {
  return createHash("sha256").update(readFileSync(filePath)).digest("hex");
}

export function runBackupRestoreDrill({
  root = repositoryRoot,
  runner = runCommand,
} = {}) {
  const before = digest(path.join(root, path.relative(repositoryRoot, closeoutManifest)));
  const command = backupRestoreDrillCommand();
  try {
    runner(command, root);
  } finally {
    const after = digest(path.join(root, path.relative(repositoryRoot, closeoutManifest)));
    if (before !== after) {
      throw new Error(
        "backup/restore drill changed the Stage 9 closeout manifest; evidence must be recorded separately",
      );
    }
  }
}

function runCommand({ executable, args }, root) {
  const result = spawnSync(executable, args, {
    cwd: root,
    encoding: "utf8",
    stdio: "inherit",
    timeout: 300_000,
    killSignal: "SIGTERM",
  });
  if (result.error?.code === "ETIMEDOUT") {
    throw new Error(`${executable} timed out after 300000ms`);
  }
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(`${executable} ${args.join(" ")} failed with exit ${result.status}`);
  }
}

export function main() {
  try {
    runBackupRestoreDrill();
    console.log(
      "Repository-local backup/restore recovery drill passed; this is not prior-version or four-platform release evidence.",
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
