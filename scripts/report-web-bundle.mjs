#!/usr/bin/env node

import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { extname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { gzipSync } from "node:zlib";

import { inspectMonacoLayout } from "./lib/monaco-layout.mjs";

const defaultDist = "apps/web/dist";
const defaultBudget = "scripts/web-bundle-budget.json";

export function collectBundleReport({ distRoot, html, budget, monacoLanguageNames = [] }) {
  const referenced = htmlAssetReferences(html);
  const assets = walkFiles(distRoot)
    .filter((file) => {
      const path = normalizePath(relative(distRoot, file));
      return path.startsWith("assets/") && [".css", ".js"].includes(extname(file));
    })
    .map((file) => assetMetrics(distRoot, file, referenced))
    .sort((left, right) => right.gzipBytes - left.gzipBytes);
  const initialJavaScript = assets.filter(
    (asset) => asset.initial && asset.extension === ".js",
  );
  const initialCss = assets.filter(
    (asset) => asset.initial && asset.extension === ".css",
  );
  const asyncJavaScript = assets.filter(
    (asset) => !asset.initial && asset.extension === ".js",
  );
  const asyncNonWorkerJavaScript = asyncJavaScript.filter(
    (asset) => !isWorkerAsset(asset.path),
  );
  const allJavaScript = assets.filter((asset) => asset.extension === ".js");
  const totals = {
    initialJavaScript: sumMetrics(initialJavaScript),
    initialCss: sumMetrics(initialCss),
    totalJavaScript: sumMetrics(allJavaScript),
  };
  const largestAsyncJavaScript = asyncJavaScript[0] ?? null;
  const largestAsyncNonWorkerJavaScript = asyncNonWorkerJavaScript[0] ?? null;
  const failures = compareBudget({
    assets,
    budget,
    largestAsyncJavaScript,
    largestAsyncNonWorkerJavaScript,
    monacoLanguageNames,
    totals,
  });
  return {
    assets,
    failures,
    largestAsyncJavaScript,
    largestAsyncNonWorkerJavaScript,
    totals,
  };
}

export function htmlAssetReferences(html) {
  const references = new Set();
  for (const match of html.matchAll(/(?:src|href)=["']([^"'?]+)(?:\?[^"']*)?["']/g)) {
    const value = match[1].replace(/^\.\//, "").replace(/^\//, "");
    if (value.startsWith("assets/") || value.endsWith(".js") || value.endsWith(".css")) {
      references.add(value);
    }
  }
  return references;
}

export function compareBudget({
  assets,
  budget,
  largestAsyncJavaScript,
  largestAsyncNonWorkerJavaScript,
  monacoLanguageNames = [],
  totals,
}) {
  const failures = [];
  checkLimit(
    failures,
    "initial JavaScript gzip",
    totals.initialJavaScript.gzipBytes,
    budget.maxInitialJavaScriptGzipBytes,
  );
  checkLimit(
    failures,
    "initial CSS gzip",
    totals.initialCss.gzipBytes,
    budget.maxInitialCssGzipBytes,
  );
  checkLimit(
    failures,
    "largest async JavaScript gzip",
    largestAsyncJavaScript?.gzipBytes ?? 0,
    budget.maxLargestAsyncJavaScriptGzipBytes,
  );
  checkLimit(
    failures,
    "largest async non-worker JavaScript gzip",
    largestAsyncNonWorkerJavaScript?.gzipBytes ?? 0,
    budget.maxLargestAsyncNonWorkerJavaScriptGzipBytes,
  );
  checkLimit(
    failures,
    "total JavaScript gzip",
    totals.totalJavaScript.gzipBytes,
    budget.maxTotalJavaScriptGzipBytes,
  );
  const patterns = (budget.forbiddenInitialAssetPatterns ?? []).map(
    (pattern) => new RegExp(pattern),
  );
  for (const asset of assets.filter((candidate) => candidate.initial)) {
    for (const pattern of patterns) {
      if (pattern.test(asset.path)) {
        failures.push(`${asset.path}: heavy lazy asset entered the initial graph (${pattern})`);
      }
    }
  }
  failures.push(...monacoBundleFailures(assets, monacoLanguageNames));
  return failures;
}

export function monacoBundleFailures(assets, languageNames) {
  const failures = [];
  const allowedLanguages = new Set(["javascript", "typescript"]);
  for (const language of languageNames) {
    if (
      !allowedLanguages.has(language) &&
      assets.some((asset) => matchesHashedAsset(asset.path, language))
    ) {
      failures.push(`assets/${language}-*.js: unexpected Monaco language chunk`);
    }
  }

  for (const worker of ["css.worker", "html.worker", "json.worker"]) {
    if (assets.some((asset) => matchesHashedAsset(asset.path, worker))) {
      failures.push(`assets/${worker}-*.js: forbidden Monaco worker`);
    }
  }
  for (const worker of ["editor.worker", "ts.worker"]) {
    if (!assets.some((asset) => matchesHashedAsset(asset.path, worker))) {
      failures.push(`assets/${worker}-*.js: required Monaco worker is missing`);
    }
  }
  return failures;
}

function matchesHashedAsset(path, stem) {
  const filename = normalizePath(path).split("/").at(-1) ?? "";
  return filename.startsWith(`${stem}-`) && filename.endsWith(".js");
}

function isWorkerAsset(path) {
  return /\.worker-[^/]+\.js$/.test(normalizePath(path));
}

function assetMetrics(distRoot, absolutePath, referenced) {
  const path = normalizePath(relative(distRoot, absolutePath));
  const contents = readFileSync(absolutePath);
  return {
    path,
    extension: extname(path),
    initial: referenced.has(path),
    rawBytes: contents.length,
    gzipBytes: gzipSync(contents, { level: 9 }).length,
  };
}

function sumMetrics(assets) {
  return assets.reduce(
    (total, asset) => ({
      files: total.files + 1,
      rawBytes: total.rawBytes + asset.rawBytes,
      gzipBytes: total.gzipBytes + asset.gzipBytes,
    }),
    { files: 0, rawBytes: 0, gzipBytes: 0 },
  );
}

function checkLimit(failures, label, actual, limit) {
  if (!Number.isFinite(limit) || limit <= 0) {
    failures.push(`${label}: budget must be a positive number`);
  } else if (actual > limit) {
    failures.push(`${label}: ${formatBytes(actual)} exceeds ${formatBytes(limit)}`);
  }
}

function walkFiles(root) {
  const files = [];
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    const path = join(root, entry.name);
    if (entry.isDirectory()) files.push(...walkFiles(path));
    else if (entry.isFile()) files.push(path);
  }
  return files;
}

function readBudget(path) {
  const budget = JSON.parse(readFileSync(path, "utf8"));
  if (budget.version !== 1) throw new Error("web bundle budget version must be 1");
  return budget;
}

function formatBytes(bytes) {
  return `${(bytes / 1024).toFixed(1)} KiB`;
}

function normalizePath(path) {
  return path.replaceAll("\\", "/");
}

function parseArgs(args) {
  const options = { budget: defaultBudget, dist: defaultDist };
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];
    if (argument === "--dist") options.dist = requireValue(args[++index], argument);
    else if (argument.startsWith("--dist=")) options.dist = requireValue(argument.slice(7), "--dist");
    else if (argument === "--budget") options.budget = requireValue(args[++index], argument);
    else if (argument.startsWith("--budget=")) options.budget = requireValue(argument.slice(9), "--budget");
    else throw new Error(`unknown argument: ${argument}`);
  }
  return options;
}

function requireValue(value, flag) {
  if (!value || value.startsWith("--")) throw new Error(`${flag} requires a value`);
  return value;
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  const distRoot = resolve(options.dist);
  const indexPath = join(distRoot, "index.html");
  const budgetPath = resolve(options.budget);
  if (!existsSync(indexPath) || !statSync(indexPath).isFile()) {
    throw new Error(`web bundle index not found: ${indexPath}; build the web app first`);
  }
  const monacoLayout = inspectMonacoLayout();
  const report = collectBundleReport({
    distRoot,
    html: readFileSync(indexPath, "utf8"),
    budget: readBudget(budgetPath),
    monacoLanguageNames: monacoLayout.languageDefinitions,
  });
  console.log(
    `Monaco ${monacoLayout.installedVersion}: ${monacoLayout.languageDefinitions.length} language definitions audited`,
  );
  const rows = [
    ["initial JavaScript", report.totals.initialJavaScript],
    ["initial CSS", report.totals.initialCss],
    ["all JavaScript", report.totals.totalJavaScript],
  ];
  for (const [label, metrics] of rows) {
    console.log(
      `${label}: ${metrics.files} files, ${formatBytes(metrics.rawBytes)} raw, ` +
        `${formatBytes(metrics.gzipBytes)} gzip`,
    );
  }
  if (report.largestAsyncJavaScript) {
    console.log(
      `largest async JavaScript: ${report.largestAsyncJavaScript.path}, ` +
        `${formatBytes(report.largestAsyncJavaScript.gzipBytes)} gzip`,
    );
  }
  if (report.largestAsyncNonWorkerJavaScript) {
    console.log(
      `largest async non-worker JavaScript: ${report.largestAsyncNonWorkerJavaScript.path}, ` +
        `${formatBytes(report.largestAsyncNonWorkerJavaScript.gzipBytes)} gzip`,
    );
  }
  console.log("largest JavaScript assets:");
  for (const asset of report.assets.filter((item) => item.extension === ".js").slice(0, 10)) {
    console.log(
      `- ${asset.path}: ${formatBytes(asset.rawBytes)} raw, ` +
        `${formatBytes(asset.gzipBytes)} gzip${asset.initial ? " [initial]" : ""}`,
    );
  }
  if (report.failures.length > 0) {
    console.error(`Web bundle budget failed with ${report.failures.length} violation(s):`);
    for (const failure of report.failures) console.error(`- ${failure}`);
    process.exitCode = 1;
  } else {
    console.log("Web bundle budget passed.");
  }
}

if (resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  try {
    main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
