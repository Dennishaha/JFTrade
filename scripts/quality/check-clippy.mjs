#!/usr/bin/env node
import { spawn } from "node:child_process";
import { resolve } from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { rustCompileEnvironment } from "../lib/tauri-runtime.mjs";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

export function normalizePnpmArguments(args) {
  if (args.length > 0 && args[0] === "--") {
    return args.slice(1);
  }
  return [...args];
}

export function buildClippyArguments(rawArgs = []) {
  const normalized = normalizePnpmArguments(rawArgs);
  const separatorIndex = normalized.indexOf("--");
  const cargoArgs = separatorIndex >= 0 ? normalized.slice(0, separatorIndex) : normalized;
  const clippyFlags = separatorIndex >= 0 ? normalized.slice(separatorIndex + 1) : [];

  const hasScope = cargoArgs.some(
    (arg) => arg === "--workspace" || arg === "-p" || arg.startsWith("--package"),
  );
  const scope = hasScope ? cargoArgs.filter(Boolean) : ["--workspace", ...cargoArgs];

  const fixedFlags = ["--all-targets", "--all-features", "--locked"];
  const finalCargoArgs = [...scope];
  for (const flag of fixedFlags) {
    if (!finalCargoArgs.includes(flag)) {
      finalCargoArgs.push(flag);
    }
  }

  const finalClippyFlags = [...clippyFlags];
  const hasDenyWarnings =
    finalClippyFlags.includes("warnings") &&
    finalClippyFlags.some((flag) => flag === "-D" || flag === "--deny");
  if (!hasDenyWarnings) {
    finalClippyFlags.unshift("-D", "warnings");
  }

  return [...finalCargoArgs, "--", ...finalClippyFlags];
}

export function runClippy(args = process.argv.slice(2), options = {}) {
  const clippyArgs = buildClippyArguments(args);
  const env = rustCompileEnvironment(options.env ?? process.env);
  const root = options.root ?? repositoryRoot;

  return new Promise((complete, reject) => {
    const child = spawn("cargo", ["clippy", ...clippyArgs], {
      cwd: root,
      env,
      stdio: options.stdio ?? "inherit",
    });
    child.once("error", reject);
    child.once("close", (status, signal) => {
      if (signal) reject(new Error(`cargo clippy terminated by ${signal}`));
      else complete(status ?? 1);
    });
  });
}

async function main() {
  try {
    process.exitCode = await runClippy();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}

if (resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  await main();
}
