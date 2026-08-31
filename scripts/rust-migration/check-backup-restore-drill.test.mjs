import assert from "node:assert/strict";
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync, rmSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  backupRestoreDrillCommand,
  runBackupRestoreDrill,
} from "./check-backup-restore-drill.mjs";

function fixtureRoot() {
  const root = mkdtempSync(path.join(os.tmpdir(), "jftrade-backup-restore-script-"));
  const manifest = path.join(root, "tests/fixtures/rust-migration/stage9");
  const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
  mkdirSync(manifest, { recursive: true });
  copyFileSync(
    path.join(repositoryRoot, "tests/fixtures/rust-migration/stage9/closeout-evidence.json"),
    path.join(manifest, "closeout-evidence.json"),
  );
  return root;
}

test("backup restore command targets only the repository-local Rust behavior test", () => {
  const command = backupRestoreDrillCommand();
  assert.equal(command.executable, "cargo");
  assert.deepEqual(command.args, [
    "test",
    "--locked",
    "-p",
    "jftrade-engine",
    "--lib",
    "product_data_management::tests::backup_restore_drill_tests::backup_restore_upgrade_corruption_and_rollback_are_fail_closed",
    "--",
    "--exact",
    "--nocapture",
  ]);
  assert.ok(!command.args.includes("--features"));
});

test("backup restore runner preserves the closeout manifest", () => {
  const root = fixtureRoot();
  try {
    const manifest = path.join(root, "tests/fixtures/rust-migration/stage9/closeout-evidence.json");
    const before = readFileSync(manifest);
    let calls = 0;
    runBackupRestoreDrill({
      root,
      runner: () => {
        calls += 1;
      },
    });
    assert.equal(calls, 1);
    assert.deepEqual(readFileSync(manifest), before);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("backup restore runner fails closed when a runner changes closeout evidence", () => {
  const root = fixtureRoot();
  try {
    const manifest = path.join(root, "tests/fixtures/rust-migration/stage9/closeout-evidence.json");
    assert.throws(
      () => runBackupRestoreDrill({
        root,
        runner: () => writeFileSync(manifest, "{}"),
      }),
      /changed the Stage 9 closeout manifest/,
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
