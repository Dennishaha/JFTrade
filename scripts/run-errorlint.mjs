#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

import { spawnChecked } from "./lib/spawn.mjs";

const errorlintPackage = "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.0";

export function normalizedDiffBase(value) {
  const base = value?.trim();
  return !base || /^0+$/.test(base) ? "" : base;
}

export function errorlintArguments(base) {
  if (!base) {
    throw new Error("errorlint requires a diff base");
  }
  return [
    "run",
    errorlintPackage,
    "run",
    "--enable-only=errorlint",
    `--new-from-rev=${base}`,
    "--timeout=5m",
    "--build-tags=gtk3",
    "--max-issues-per-linter=0",
    "--max-same-issues=0",
  ];
}

function main() {
  const base = normalizedDiffBase(process.env.JFTRADE_DIFF_BASE) || defaultDiffBase();
  if (!base) {
    throw new Error("unable to determine an errorlint diff base; set JFTRADE_DIFF_BASE=<git-ref>");
  }
  console.log(`Running incremental errorlint against ${base}.`);
  process.exitCode = spawnChecked("go", errorlintArguments(base));
}

function defaultDiffBase() {
  for (const candidate of ["origin/main", "HEAD^"]) {
    try {
      execFileSync("git", ["rev-parse", "--verify", candidate], { stdio: "ignore" });
      return candidate;
    } catch {
      // Try the next local fallback.
    }
  }
  return "";
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
