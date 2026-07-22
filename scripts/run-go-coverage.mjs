#!/usr/bin/env node
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";
import { spawnChecked } from "./lib/spawn.mjs";

export function stripPnpmRunSeparator(args) {
  // `pnpm run <script> -- <args>` retains the separator in Node's argv. It is
  // meaningful to pnpm, but must not be forwarded to the Go flag parser because
  // it would turn every following coverage option into a positional argument.
  return args[0] === "--" ? args.slice(1) : args;
}

function main() {
  const forwardedArgs = stripPnpmRunSeparator(process.argv.slice(2));
  const hasDiffBase = forwardedArgs.some((arg, index) => arg === "-diff-base" || (index > 0 && forwardedArgs[index - 1] === "-diff-base") || arg.startsWith("-diff-base="));
  const diffBase = normalizedDiffBase(process.env.JFTRADE_DIFF_BASE) || (hasDiffBase ? "" : defaultDiffBase());
  const args = ["run", "./cmd/check-go-coverage", ...forwardedArgs];

  if (diffBase && !hasDiffBase) {
    args.push(`-diff-base=${diffBase}`);
  }

  process.exitCode = spawnChecked("go", args);
}

function normalizedDiffBase(value) {
  const base = value?.trim();
  if (!base || /^0+$/.test(base)) {
    return "";
  }
  return base;
}

function defaultDiffBase() {
  try {
    execFileSync("git", ["rev-parse", "--verify", "origin/main"], { cwd: process.cwd(), stdio: "ignore" });
    return "origin/main";
  } catch {
    console.warn("Coverage diff gate is disabled because origin/main is unavailable; set JFTRADE_DIFF_BASE=<git-ref> to enable it.");
    return "";
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main();
}
