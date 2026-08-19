#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const fixtureRoot = path.join(repositoryRoot, "tests/fixtures/rust-migration/stage8");
const corpusPath = path.join(fixtureRoot, "desktop-shell-corpus.json");
const expectedPath = path.join(fixtureRoot, "desktop-shell-corpus.expected.json");

const result = spawnSync("cargo", [
  "run", "--quiet", "-p", "jftrade-desktop", "--bin", "jftrade-stage8-shadow", "--",
  "--input", corpusPath,
], {
  cwd: repositoryRoot,
  encoding: "utf8",
  stdio: ["ignore", "pipe", "pipe"],
  timeout: 300_000,
});
if (result.error) throw result.error;
if (result.status !== 0) throw new Error(result.stderr || result.stdout);
const output = JSON.parse(result.stdout);
if (process.argv.includes("--write-expected")) {
  fs.writeFileSync(expectedPath, `${JSON.stringify(output, null, 2)}\n`);
} else {
  process.stdout.write(`${JSON.stringify(output, null, 2)}\n`);
}
