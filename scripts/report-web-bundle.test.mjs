#!/usr/bin/env node

import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import {
  collectBundleReport,
  htmlAssetReferences,
} from "./report-web-bundle.mjs";

const generousBudget = {
  maxInitialJavaScriptGzipBytes: 1000,
  maxInitialCssGzipBytes: 1000,
  maxLargestAsyncJavaScriptGzipBytes: 1000,
  maxTotalJavaScriptGzipBytes: 2000,
  forbiddenInitialAssetPatterns: ["\\.worker-"],
};
const monacoLanguageNames = ["javascript", "python", "typescript"];

assert.deepEqual(
  [...htmlAssetReferences('<script src="/assets/index-a.js"></script><link href="/assets/app.css">')],
  ["assets/index-a.js", "assets/app.css"],
);

const root = mkdtempSync(join(tmpdir(), "jftrade-web-bundle-"));
try {
  mkdirSync(join(root, "assets"));
  writeFileSync(join(root, "assets", "index-a.js"), "export const app = true;\n");
  writeFileSync(join(root, "assets", "app.css"), ".app { display: block; }\n");
  writeFileSync(join(root, "assets", "ts.worker-a.js"), "export const worker = true;\n");
  writeFileSync(join(root, "assets", "editor.worker-a.js"), "export const editor = true;\n");
  writeFileSync(join(root, "assets", "javascript-a.js"), "export const js = true;\n");
  writeFileSync(join(root, "assets", "typescript-a.js"), "export const ts = true;\n");
  const html = '<script src="/assets/index-a.js"></script><link href="/assets/app.css">';
  const passing = collectBundleReport({
    distRoot: root,
    html,
    budget: generousBudget,
    monacoLanguageNames,
  });
  assert.equal(passing.failures.length, 0);
  assert.equal(passing.totals.initialJavaScript.files, 1);
  assert.equal(passing.totals.initialCss.files, 1);
  assert(passing.assets.some((asset) => asset.path === "assets/ts.worker-a.js"));

  const eagerWorker = collectBundleReport({
    distRoot: root,
    html: `${html}<script src="/assets/ts.worker-a.js"></script>`,
    budget: generousBudget,
    monacoLanguageNames,
  });
  assert.match(eagerWorker.failures.join("\n"), /heavy lazy asset entered the initial graph/);

  const strict = collectBundleReport({
    distRoot: root,
    html,
    budget: { ...generousBudget, maxInitialJavaScriptGzipBytes: 1 },
    monacoLanguageNames,
  });
  assert.match(strict.failures.join("\n"), /initial JavaScript gzip/);

  writeFileSync(join(root, "assets", "python-a.js"), "export const python = true;\n");
  writeFileSync(join(root, "assets", "css.worker-a.js"), "export const css = true;\n");
  const forbiddenMonaco = collectBundleReport({
    distRoot: root,
    html,
    budget: generousBudget,
    monacoLanguageNames,
  });
  assert.match(forbiddenMonaco.failures.join("\n"), /unexpected Monaco language chunk/);
  assert.match(forbiddenMonaco.failures.join("\n"), /forbidden Monaco worker/);

  rmSync(join(root, "assets", "editor.worker-a.js"));
  const missingWorker = collectBundleReport({
    distRoot: root,
    html,
    budget: generousBudget,
    monacoLanguageNames: ["javascript", "typescript"],
  });
  assert.match(missingWorker.failures.join("\n"), /required Monaco worker is missing/);
} finally {
  rmSync(root, { recursive: true, force: true });
}

console.log("web bundle report tests passed");
