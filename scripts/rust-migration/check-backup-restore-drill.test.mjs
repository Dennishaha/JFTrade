import assert from "node:assert/strict";
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  backupRestoreDrillCommand,
  backupRestoreDrillTimeoutMs,
  runCommand,
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
    let receivedRoot;
    let receivedTimeout;
    runBackupRestoreDrill({
      root: path.join(root, "."),
      runner: (_command, cwd, timeoutMs) => {
        calls += 1;
        receivedRoot = cwd;
        receivedTimeout = timeoutMs;
      },
    });
    assert.equal(calls, 1);
    assert.equal(receivedRoot, realpathSync(root));
    assert.equal(receivedTimeout, backupRestoreDrillTimeoutMs);
    assert.deepEqual(readFileSync(manifest), before);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("backup restore runner preserves runner failures while rejecting changed closeout evidence", () => {
  const root = fixtureRoot();
  try {
    const manifest = path.join(root, "tests/fixtures/rust-migration/stage9/closeout-evidence.json");
    const workerCrash = new Error("simulated retained worker crash");
    assert.throws(() => runBackupRestoreDrill({
        root,
        runner: () => {
          rmSync(manifest);
          throw workerCrash;
        },
      }), (error) => {
      assert.ok(error instanceof AggregateError);
      assert.match(error.message, /simulated retained worker crash/);
      assert.match(error.message, /changed the Stage 9 closeout manifest/);
      assert.equal(error.errors[0], workerCrash);
      return true;
    });
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("backup restore runner does not swallow an unchanged runner failure", () => {
  const root = fixtureRoot();
  try {
    const runnerFailure = new Error("cargo test failed");
    assert.throws(
      () => runBackupRestoreDrill({
        root,
        runner: () => {
          throw runnerFailure;
        },
      }),
      (error) => error === runnerFailure,
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("backup restore runner rejects a missing closeout manifest after the drill", () => {
  const root = fixtureRoot();
  try {
    const manifest = path.join(root, "tests/fixtures/rust-migration/stage9/closeout-evidence.json");
    assert.throws(
      () => runBackupRestoreDrill({
        root,
        runner: () => rmSync(manifest),
      }),
      /changed the Stage 9 closeout manifest; evidence must be recorded separately: closeout manifest is unavailable/,
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("backup restore command reports worker signal and timeout failures", () => {
  const root = fixtureRoot();
  try {
    assert.throws(
      () => runCommand(
        {
          executable: process.execPath,
          args: ["-e", "process.kill(process.pid, 'SIGTERM')"],
        },
        root,
        1_000,
      ),
      /failed with signal SIGTERM/,
    );
    assert.throws(
      () => runCommand(
        {
          executable: process.execPath,
          args: ["-e", "setTimeout(() => {}, 5_000)"],
        },
        root,
        100,
      ),
      /timed out after 100ms/,
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("backup restore runner requires a bounded positive timeout", () => {
  const root = fixtureRoot();
  try {
    for (const timeoutMs of [0, -1, 1.5, Number.POSITIVE_INFINITY, backupRestoreDrillTimeoutMs + 1]) {
      assert.throws(
        () => runBackupRestoreDrill({ root, timeoutMs }),
        /timeout must be a positive integer/,
      );
    }
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
