#!/usr/bin/env node
import path from "node:path";
import { pathToFileURL } from "node:url";

import {
  assertBacktestEquivalent,
  loadStage3Expected,
  runRustReference,
} from "./check-backtest-differential.mjs";

const ALLOWED_OWNERS = new Set(["rust"]);

export function resolveBacktestOwner(args = [], env = process.env) {
  let owner = env.JFTRADE_BACKTEST_CORE_OWNER || "rust";
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
    throw new Error(`unsupported backtest owner ${owner || "<empty>"}; expected rust`);
  }
  return owner;
}

export function selectBacktestOwner(owner, providers) {
  if (owner === "rust") {
    const output = providers.rust();
    providers.assertEquivalent(output, providers.expected);
    return { owner, output, fixtureChecked: true };
  }
  throw new Error(`unsupported backtest owner ${owner}`);
}

export function runOwnerRehearsal(owner, root) {
  return selectBacktestOwner(owner, {
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
    console.error(`stage 3 compatibility replay selected ${result.owner}; fixtureChecked=${result.fixtureChecked}`);
    console.log(JSON.stringify(result.output));
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
