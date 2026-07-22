#!/usr/bin/env node
import { execFileSync } from "node:child_process";
import { spawnChecked } from "./lib/spawn.mjs";

const diffBase = normalizedDiffBase(process.env.JFTRADE_DIFF_BASE) || defaultDiffBase();

let status = spawnChecked("pnpm", ["--filter", "@jftrade/web", "run", "test:coverage", ...process.argv.slice(2)]);
if (status !== 0) {
  process.exit(status);
}

if (diffBase) {
  status = spawnChecked(process.execPath, ["scripts/check-web-diff-coverage.mjs", "--base", diffBase]);
}

process.exit(status);

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
    console.warn("Web coverage diff gate is disabled because origin/main is unavailable; set JFTRADE_DIFF_BASE=<git-ref> to enable it.");
    return "";
  }
}
