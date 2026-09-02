#!/usr/bin/env node

import { readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";

import {
  buildQualityAllowlist,
  compareQualityGaps,
  findOpenAPIQualityGaps,
} from "./lib/openapi-quality.mjs";

const repoRoot = path.resolve(import.meta.dirname, "..");
const specPath = path.join(repoRoot, "contracts/openapi/openapi.json");
const allowlistPath = path.join(repoRoot, "scripts/openapi-quality-allowlist.json");

const spec = JSON.parse(await readFile(specPath, "utf8"));
const gaps = findOpenAPIQualityGaps(spec);

if (process.argv.includes("--print-allowlist")) {
  process.stdout.write(`${JSON.stringify(buildQualityAllowlist(gaps), null, 2)}\n`);
  process.exit(0);
}

const allowlist = JSON.parse(await readFile(allowlistPath, "utf8"));
const result = compareQualityGaps(gaps, allowlist);
if (result.unexpected.length === 0 && result.stale.length === 0 && result.duplicates.length === 0) {
  console.log(`OpenAPI quality check passed with ${gaps.length} explicitly tracked P0 gap(s).`);
  process.exit(0);
}

for (const entry of result.unexpected) {
  console.error(`unexpected OpenAPI quality gap: ${entry.id}`);
}
for (const entry of result.stale) {
  console.error(`resolved OpenAPI quality gap must be removed from allowlist: ${entry.id}`);
}
for (const entry of result.duplicates) {
  console.error(`duplicate OpenAPI quality allowlist entry: ${entry.id}`);
}
process.exit(1);
