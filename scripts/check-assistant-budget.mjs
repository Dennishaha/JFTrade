#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

const repoRoot = path.resolve(import.meta.dirname, "..");
const budgetPath = path.join(repoRoot, "scripts", "assistant-budget.json");
const engineImport = "github.com/jftrade/jftrade-main/internal/assistant/engine";

export function compareAssistantBudget(actual, budget) {
  const failures = [];
  for (const [actualKey, budgetKey, label] of [
    ["engineProductionLines", "engineProductionLinesMax", "engine production lines"],
    ["engineTestLines", "engineTestLinesMax", "engine test lines"],
    ["externalDependencyFiles", "externalDependencyFilesMax", "external engine dependency files"],
  ]) {
    if (!Number.isInteger(budget[budgetKey]) || budget[budgetKey] < 0) {
      failures.push(`${budgetKey} must be a non-negative integer`);
    } else if (actual[actualKey] > budget[budgetKey]) {
      failures.push(`${label} ${actual[actualKey]} exceed budget ${budget[budgetKey]}`);
    }
  }
  return failures;
}

export function inspectAssistantBudget(root = repoRoot) {
  const engineRootPath = path.join(root, "internal", "assistant", "engine");
  const files = fs.readdirSync(engineRootPath, { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(".go"))
    .map((entry) => path.join(engineRootPath, entry.name));
  const production = files.filter((file) => !file.endsWith("_test.go"));
  const tests = files.filter((file) => file.endsWith("_test.go"));
  const external = new Set();
  for (const candidate of walk(root).filter((candidate) => candidate.endsWith(".go"))) {
    if (candidate.startsWith(`${engineRootPath}${path.sep}`)) continue;
    if (fs.readFileSync(candidate, "utf8").includes(`"${engineImport}"`)) external.add(candidate);
  }
  return {
    engineProductionLines: production.reduce((sum, file) => sum + countLines(file), 0),
    engineTestLines: tests.reduce((sum, file) => sum + countLines(file), 0),
    externalDependencyFiles: external.size,
  };
}

function walk(directory) {
  if (!fs.existsSync(directory)) return [];
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const target = path.join(directory, entry.name);
    return entry.isDirectory() ? walk(target) : [target];
  });
}

function countLines(file) {
  const source = fs.readFileSync(file, "utf8");
  return source ? source.split("\n").length - (source.endsWith("\n") ? 1 : 0) : 0;
}

function main() {
  const budget = JSON.parse(fs.readFileSync(budgetPath, "utf8"));
  const actual = inspectAssistantBudget();
  const failures = compareAssistantBudget(actual, budget);
  if (failures.length > 0) {
    console.error("Assistant engine budget regressed:");
    failures.forEach((failure) => console.error(`- ${failure}`));
    process.exitCode = 1;
    return;
  }
  console.log(`Assistant engine budget passed: ${actual.engineProductionLines}/${budget.engineProductionLinesMax} production lines, ${actual.engineTestLines}/${budget.engineTestLinesMax} test lines, ${actual.externalDependencyFiles}/${budget.externalDependencyFilesMax} external files.`);
  if (Number.isInteger(budget.externalDependencyFilesTarget)) {
    console.log(`Assistant engine budget target: external dependency files ${budget.externalDependencyFilesTarget}.`);
  }
}

if (path.resolve(process.argv[1] ?? "") === path.resolve(import.meta.filename)) main();
