#!/usr/bin/env node
import { resolve } from "node:path";
import process from "node:process";

import { spawnChecked } from "./lib/spawn.mjs";

const repoRoot = resolve(import.meta.dirname, "..");
const outputRoot = resolve(process.env.JFTRADE_GENERATED_ROOT || repoRoot);
const outputDir = resolve(outputRoot, "docs", "swagger");
const apiPackageDir = resolve(repoRoot, "cmd", "jftrade-api");

const args = [
  "run",
  "github.com/swaggo/swag/cmd/swag@v1.16.6",
  "init",
  "-g",
  "docs.go",
  "-d",
  ".,../../internal/app/apiserver/servercore,../../internal/app/apiserver/webaccess,../../internal/api/system,../../internal/api/marketdata,../../internal/api/live,../../internal/api/productfeatures,../../internal/api/assistant,../../internal/api/backtest,../../internal/api/settings,../../internal/api/strategy,../../internal/api/trading,../../internal/api/watchlist,../../internal/api/research",
  "-o",
  outputDir,
  "--parseDependency",
  "--parseInternal",
  "--requiredByDefault",
];

const status = spawnChecked("go", args, { cwd: apiPackageDir });
process.exitCode = status;
