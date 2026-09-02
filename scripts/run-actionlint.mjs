#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";

import { createLinter } from "actionlint";

const repositoryRoot = path.resolve(import.meta.dirname, "..");
const workflowDirectory = path.join(repositoryRoot, ".github/workflows");
const files = fs.readdirSync(workflowDirectory)
  .filter((name) => /\.ya?ml$/u.test(name))
  .sort();
let failures = 0;

for (const name of files) {
  const relativePath = `.github/workflows/${name}`;
  const source = fs.readFileSync(path.join(workflowDirectory, name), "utf8");
  // The WASM wrapper retains parser state across invocations and can trap on
  // later files.  A fresh instance keeps each workflow check isolated.
  const lint = await createLinter();
  for (const result of lint(source, relativePath)) {
    // The bundled actionlint release predates current GitHub-hosted labels.
    // Matrix labels are validated by the workflow platform itself.
    if (result.kind === "runner-label") continue;
    if (result.kind === "permissions" && result.message.includes('"attestations"')) continue;
    failures += 1;
    console.error(`${result.file}:${result.line}:${result.column}: ${result.message} [${result.kind}]`);
  }
}

if (failures > 0) {
  console.error(`actionlint failed with ${failures} issue(s).`);
  process.exit(1);
}

console.log(`actionlint passed: ${files.length} workflow files.`);
