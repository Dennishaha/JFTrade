#!/usr/bin/env node

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

import { contractIndexViolations } from "./lib/web-contract-index.mjs";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(scriptDir, "..");
const indexPath = resolve(repoRoot, "apps/web/src/contracts/index.ts");
const violations = contractIndexViolations(
  readFileSync(indexPath, "utf8"),
  indexPath,
);

if (violations.length === 0) {
  console.log(
    "Web contract index passed: the compatibility entrypoint contains re-exports only.",
  );
} else {
  console.error(
    "apps/web/src/contracts/index.ts must remain a re-export-only compatibility entrypoint:",
  );
  for (const violation of violations) {
    console.error(`- apps/web/src/contracts/index.ts:${violation.line} ${violation.message}`);
  }
  process.exitCode = 1;
}
