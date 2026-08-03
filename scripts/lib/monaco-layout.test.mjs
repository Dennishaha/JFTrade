#!/usr/bin/env node

import assert from "node:assert/strict";
import { existsSync } from "node:fs";

import {
  inspectMonacoLayout,
  requiredMonacoSubpaths,
  resolveMonacoSubpath,
} from "./monaco-layout.mjs";

const layout = inspectMonacoLayout();

assert.equal(layout.declaredVersion, "0.56.0");
assert.equal(layout.installedVersion, "0.56.0");
assert(layout.languageDefinitions.length > 60, "Monaco language definitions were not discovered");
assert(layout.languageDefinitions.includes("javascript"));
assert(layout.languageDefinitions.includes("typescript"));
for (const subpath of requiredMonacoSubpaths) {
  assert(existsSync(resolveMonacoSubpath(subpath)), `Monaco subpath is missing: ${subpath}`);
  assert.equal(layout.resolvedSubpaths[subpath], resolveMonacoSubpath(subpath));
}

console.log("monaco layout tests passed");
