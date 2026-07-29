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
  const html = '<script src="/assets/index-a.js"></script><link href="/assets/app.css">';
  const passing = collectBundleReport({ distRoot: root, html, budget: generousBudget });
  assert.equal(passing.failures.length, 0);
  assert.equal(passing.totals.initialJavaScript.files, 1);
  assert.equal(passing.totals.initialCss.files, 1);
  assert.equal(passing.largestAsyncJavaScript.path, "assets/ts.worker-a.js");

  const eagerWorker = collectBundleReport({
    distRoot: root,
    html: `${html}<script src="/assets/ts.worker-a.js"></script>`,
    budget: generousBudget,
  });
  assert.match(eagerWorker.failures.join("\n"), /heavy lazy asset entered the initial graph/);

  const strict = collectBundleReport({
    distRoot: root,
    html,
    budget: { ...generousBudget, maxInitialJavaScriptGzipBytes: 1 },
  });
  assert.match(strict.failures.join("\n"), /initial JavaScript gzip/);
} finally {
  rmSync(root, { recursive: true, force: true });
}

console.log("web bundle report tests passed");
