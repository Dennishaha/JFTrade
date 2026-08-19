#!/usr/bin/env node
import path from "node:path";
import { pathToFileURL } from "node:url";

import {
  assertBacktestEquivalent,
  loadStage3Expected,
  runGoReference,
  runRustReference,
} from "./check-backtest-differential.mjs";

const ALLOWED_OWNERS = new Set(["go", "shadow", "rust"]);

export function resolveBacktestOwner(args = [], env = process.env) {
  let owner = env.JFTRADE_BACKTEST_CORE_OWNER || "go";
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];
    if (argument === "--") {
      continue;
    }
    if (argument === "--owner") {
      owner = args[index + 1] ?? "";
      index += 1;
    } else if (argument.startsWith("--owner=")) {
      owner = argument.slice("--owner=".length);
    } else {
      throw new Error(`unsupported argument ${argument}`);
    }
  }
  if (!ALLOWED_OWNERS.has(owner)) {
    throw new Error(`unsupported backtest owner ${owner || "<empty>"}; expected go, shadow, or rust`);
  }
  return owner;
}

export function selectBacktestOwner(owner, providers) {
  if (owner === "go") {
    return { owner, output: providers.go(), shadowChecked: false };
  }
  if (owner === "rust") {
    return { owner, output: providers.rust(), shadowChecked: false };
  }
  if (owner !== "shadow") {
    throw new Error(`unsupported backtest owner ${owner}`);
  }
  const goOutput = providers.go();
  const rustOutput = providers.rust();
  providers.assertEquivalent(goOutput, rustOutput, providers.expected);
  return { owner: "go", output: goOutput, shadowChecked: true };
}

export function runOwnerRehearsal(owner, root) {
  return selectBacktestOwner(owner, {
    go: () => runGoReference(root),
    rust: () => runRustReference(root),
    expected: loadStage3Expected(root),
    assertEquivalent: assertBacktestEquivalent,
  });
}

const invokedPath = process.argv[1] ? pathToFileURL(path.resolve(process.argv[1])).href : "";
if (invokedPath === import.meta.url) {
  try {
    const owner = resolveBacktestOwner(process.argv.slice(2));
    const result = runOwnerRehearsal(owner);
    console.error(`stage 3 owner rehearsal selected ${result.owner}; shadowChecked=${result.shadowChecked}`);
    console.log(JSON.stringify(result.output));
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
