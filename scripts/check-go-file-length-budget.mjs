#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

const repoRoot = path.resolve(import.meta.dirname, "..");
const budgetPath = path.join(repoRoot, "scripts", "go-file-length-budget.json");
const scanRoots = ["cmd", "internal", "pkg"];
const frozenProductionExceptions = new Set([
  "internal/integration/futu/marketdata_runtime.go",
]);

export function lineCount(contents) {
  return contents ? contents.split("\n").length - (contents.endsWith("\n") ? 1 : 0) : 0;
}

export function compareGoFileLength(files, budget) {
  const failures = [];
  if (!Number.isInteger(budget.productionMaxLines) || budget.productionMaxLines !== 800) {
    failures.push("productionMaxLines must remain 800");
  }
  if (!Number.isInteger(budget.testMaxLines) || budget.testMaxLines !== 1200) {
    failures.push("testMaxLines must remain 1200");
  }
  const exceptions = budget.productionExceptions ?? {};
  for (const name of Object.keys(exceptions)) {
    if (!frozenProductionExceptions.has(name)) {
      failures.push(`${name} is not an approved production exception`);
    }
  }
  for (const name of frozenProductionExceptions) {
    if (exceptions[name] === undefined) failures.push(`${name} production exception was removed without splitting the file`);
  }
  const names = new Set(files.map((file) => file.name));
  for (const file of files) {
    const limit = file.test ? budget.testMaxLines : budget.productionMaxLines;
    const exception = file.test ? undefined : exceptions[file.name];
    if (file.lines <= limit) {
      if (exception !== undefined) failures.push(`${file.name} has a stale exception at ${file.lines} lines`);
      continue;
    }
    if (exception === undefined) {
      failures.push(`${file.name} has ${file.lines} lines, limit ${limit}`);
    } else if (!Number.isInteger(exception) || exception <= limit) {
      failures.push(`${file.name} exception must exceed ${limit}`);
    } else if (file.lines > exception) {
      failures.push(`${file.name} grew to ${file.lines} lines, budget ${exception}`);
    }
  }
  for (const name of Object.keys(exceptions)) {
    if (!names.has(name)) failures.push(`${name} exception does not match a scanned file`);
  }
  return failures;
}

function walk(directory) {
  if (!fs.existsSync(directory)) return [];
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const target = path.join(directory, entry.name);
    return entry.isDirectory() ? walk(target) : [target];
  });
}

function collectFiles(root) {
  return scanRoots.flatMap((scanRoot) => walk(path.join(root, scanRoot)))
    .filter((file) => file.endsWith(".go"))
    .filter((file) => !file.endsWith(".pb.go") && !file.split(path.sep).includes("pb"))
    .filter((file) => !fs.readFileSync(file, "utf8").startsWith("// Code generated"))
    .map((file) => ({
      name: path.relative(root, file).split(path.sep).join("/"),
      lines: lineCount(fs.readFileSync(file, "utf8")),
      test: file.endsWith("_test.go"),
    }));
}

function main() {
  const budget = JSON.parse(fs.readFileSync(budgetPath, "utf8"));
  const failures = compareGoFileLength(collectFiles(repoRoot), budget);
  if (failures.length > 0) {
    console.error("Go file length budget regressed:");
    failures.forEach((failure) => console.error(`- ${failure}`));
    process.exitCode = 1;
    return;
  }
  console.log(`Go file length budget passed: production <= ${budget.productionMaxLines}, tests <= ${budget.testMaxLines}.`);
}

if (path.resolve(process.argv[1] ?? "") === path.resolve(import.meta.filename)) main();
