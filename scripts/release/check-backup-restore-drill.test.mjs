import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  backupRestoreDrillCommand,
  backupRestoreDrillTimeoutMs,
  runBackupRestoreDrill,
  runCommand,
} from "./check-backup-restore-drill.mjs";

test("backup restore command targets the repository Rust behavior", () => {
  const command = backupRestoreDrillCommand();
  assert.equal(command.executable, "cargo");
  assert.deepEqual(command.args.slice(0, 6), ["test", "--locked", "-p", "jftrade-engine", "--lib", command.args[5]]);
  assert.match(command.args[5], /backup_restore_upgrade_corruption_and_rollback_are_fail_closed/);
  assert.ok(!command.args.includes("--features"));
});

test("backup restore runner returns a runtime receipt without committed state", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-backup-restore-"));
  try {
    let invocation;
    const receipt = runBackupRestoreDrill({
      root,
      runner: (command, cwd, timeoutMs) => { invocation = { command, cwd, timeoutMs }; },
    });
    assert.equal(invocation.cwd, fs.realpathSync(root));
    assert.equal(invocation.timeoutMs, backupRestoreDrillTimeoutMs);
    assert.equal(receipt.schemaVersion, "jftrade.backup-restore-drill.v1");
    assert.equal(receipt.status, "passed");
    assert.equal(fs.readdirSync(root).length, 0);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("backup restore runner preserves execution failures", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-backup-restore-"));
  try {
    const failure = new Error("cargo test failed");
    assert.throws(() => runBackupRestoreDrill({ root, runner: () => { throw failure; } }), (error) => error === failure);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("backup restore command reports signal timeout and invalid timeout failures", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-backup-restore-"));
  try {
    assert.throws(() => runCommand({
      executable: process.execPath,
      args: ["-e", "process.kill(process.pid, 'SIGTERM')"],
    }, root, 1_000), /failed with signal SIGTERM/);
    assert.throws(() => runCommand({
      executable: process.execPath,
      args: ["-e", "setTimeout(() => {}, 5000)"],
    }, root, 100), /timed out after 100ms/);
    for (const timeoutMs of [0, -1, 1.5, backupRestoreDrillTimeoutMs + 1]) {
      assert.throws(() => runBackupRestoreDrill({ root, timeoutMs, runner: () => {} }), /timeout must be a positive integer/);
    }
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});
