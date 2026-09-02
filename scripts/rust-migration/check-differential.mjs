#!/usr/bin/env node
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const fixtureRoot = path.join(repositoryRoot, "tests/fixtures/rust-migration/stage2");

function run(command, args, root = repositoryRoot) {
  const result = spawnSync(command, args, {
    cwd: root,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(command + " " + args.join(" ") + " failed:\n" + (result.stderr || result.stdout));
  }
  return result.stdout.trim();
}

export function assertEquivalent(rustSnapshot, expectedSnapshot) {
  assert.deepEqual(rustSnapshot, expectedSnapshot, "snapshot differs from the pinned golden");
}

export function assertBytesUnchanged(before, after) {
  assert.ok(before.equals(after), "Rust read-only inspection changed SQLite bytes");
}

export function runDifferential(root = repositoryRoot) {
  const temporaryRoot = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-rust-stage2-"));
  const databasePath = path.join(temporaryRoot, "backtest.db");
  try {
    const seededOutput = run("cargo", [
      "run", "--quiet", "-p", "jftrade-store-sqlite", "--bin", "jftrade-sqlite-inspect", "--",
      "--seed-sql",
      path.join(fixtureRoot, "backtest-readonly.sql"),
      databasePath,
    ], root);
    const before = fs.readFileSync(databasePath);
    const rustOutput = run("cargo", [
      "run",
      "--quiet",
      "-p",
      "jftrade-store-sqlite",
      "--bin",
      "jftrade-sqlite-inspect",
      "--",
      databasePath,
    ], root);
    const after = fs.readFileSync(databasePath);
    const expected = JSON.parse(fs.readFileSync(
      path.join(fixtureRoot, "backtest-readonly.expected.json"),
      "utf8",
    ));
    assertBytesUnchanged(before, after);
    assertEquivalent(JSON.parse(seededOutput), expected);
    assertEquivalent(JSON.parse(rustOutput), expected);
    return { rows: expected.klines.length, tables: expected.tables.length };
  } finally {
    fs.rmSync(temporaryRoot, { force: true, recursive: true });
  }
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  const result = runDifferential();
  console.log(
    "Rust SQLite compatibility replay passed: " + result.tables + " tables, " + result.rows + " K-lines.",
  );
}
