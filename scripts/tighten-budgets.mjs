#!/usr/bin/env node
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import process from "node:process";

import { inspectAssistantBudget } from "./check-assistant-budget.mjs";
import { readServercoreBudgetState } from "./check-servercore-budget.mjs";

const repoRoot = path.resolve(import.meta.dirname, "..");

function tightenBudget(file, actual, budget, dimensions) {
  let changed = false;
  for (const dimension of dimensions) {
    const actualValue = actual[dimension];
    const maxKey = `${dimension}Max`;
    const max = budget[maxKey];
    if (!Number.isInteger(actualValue) || !Number.isInteger(max)) continue;
    if (actualValue > max) {
      throw new Error(`${file}: ${dimension} ${actualValue} exceeds budget ${max}; refusing to tighten`);
    }
    if (actualValue < max) {
      budget[maxKey] = actualValue;
      changed = true;
      console.log(`${path.basename(file)}: ${maxKey} ${max} -> ${actualValue}`);
    }
  }
  return changed;
}

const servercorePath = path.join(repoRoot, "scripts", "servercore-budget.json");
const servercore = readServercoreBudgetState({ repoRoot });
if (tightenBudget(
  servercorePath,
  servercore.actual,
  servercore.budget,
  [
    "productionLines",
    "testLines",
    "serverMethods",
    "applicationMethods",
    "effectiveServerMethods",
    "aggregateFields",
  ],
)) {
  writeFileSync(servercorePath, `${JSON.stringify(servercore.budget, null, 2)}\n`);
}

const assistantPath = path.join(repoRoot, "scripts", "assistant-budget.json");
const assistantBudget = JSON.parse(readFileSync(assistantPath, "utf8"));
if (tightenBudget(
  assistantPath,
  inspectAssistantBudget(repoRoot),
  assistantBudget,
  ["engineProductionLines", "engineTestLines", "externalDependencyFiles"],
)) {
  writeFileSync(assistantPath, `${JSON.stringify(assistantBudget, null, 2)}\n`);
}

console.log("budget ratchet up to date (maxes only ever decrease).");
