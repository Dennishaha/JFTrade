#!/usr/bin/env node

import process from "node:process";
import { resolve } from "node:path";

import { spawnChecked } from "./lib/spawn.mjs";

const rootDir = resolve(import.meta.dirname, "..");

run("go", ["generate", "./cmd/jftrade-api"]);
run("node", ["scripts/generate-api-types.mjs"]);
run(
  "go",
  [
    "test",
    "./internal/app/apiserver/servercore",
    "-run",
    "^TestOpenAPISpecStable$",
    "-count=1",
  ],
  {
    env: {
      ...process.env,
      UPDATE_OPENAPI_SNAPSHOT: "1",
    },
  },
);

function run(command, args, options = {}) {
  const status = spawnChecked(command, args, {
    cwd: rootDir,
    ...options,
  });
  if (status !== 0) {
    process.exit(status);
  }
}
