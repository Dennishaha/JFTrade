#!/usr/bin/env node
import { execFileSync } from "node:child_process";
import { readdirSync, readFileSync } from "node:fs";
import { dirname, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const fixedDefaultMaxLines = 800;

export function inspectVueSource(contents, loadStyle = () => "") {
  let scopedStyleLines = 0;
  let externalStyleLines = 0;
  const loaded = new Set();
  for (const match of contents.matchAll(/<style\b([^>]*)>([\s\S]*?)<\/style>/g)) {
    const attributes = match[1] ?? "";
    if (!/\bscoped\b/.test(attributes)) continue;
    const src = /\bsrc\s*=\s*["']([^"']+)["']/.exec(attributes)?.[1];
    if (src && !loaded.has(src)) {
      loaded.add(src);
      const lines = countLines(loadStyle(src));
      scopedStyleLines += lines;
      externalStyleLines += lines;
    } else if (!src) {
      scopedStyleLines += countLines(match[2] ?? "");
    }
  }
  const sourceLines = countLines(contents);
  return { sourceLines, scopedStyleLines, externalStyleLines, effectiveLines: sourceLines + externalStyleLines };
}

export function compareWebComponentBudget(components, budget, baseBudget = null) {
  const failures = [];
  const limit = budget.defaultMaxLines;
  if (limit !== fixedDefaultMaxLines) {
    failures.push(`defaultMaxLines must remain ${fixedDefaultMaxLines}`);
  }
  const styleTotal = components.reduce((sum, item) => sum + item.scopedStyleLines, 0);
  if (!Number.isInteger(budget.scopedStyleLinesMax) || budget.scopedStyleLinesMax < 0) {
    failures.push("scopedStyleLinesMax must be a non-negative integer");
  } else if (styleTotal > budget.scopedStyleLinesMax) {
    failures.push(`scoped style lines ${styleTotal} exceed budget ${budget.scopedStyleLinesMax}`);
  } else if (styleTotal < budget.scopedStyleLinesMax) {
    failures.push(`scopedStyleLinesMax is stale at ${budget.scopedStyleLinesMax}; reduce it to ${styleTotal}`);
  }
  const byName = new Map(components.map((item) => [item.name, item]));
  const exceptions = budget.exceptions ?? {};
  for (const item of components) {
    const exception = exceptions[item.name];
    if (item.effectiveLines <= limit) {
      if (exception) failures.push(`${item.name} has a stale exception at ${item.effectiveLines} lines`);
      continue;
    }
    if (!exception) {
      failures.push(`${item.name} has ${item.effectiveLines} effective lines, limit ${limit}`);
      continue;
    }
    if (!Number.isInteger(exception.maxLines) || exception.maxLines <= limit) {
      failures.push(`${item.name} exception maxLines must exceed ${limit}`);
    } else if (item.effectiveLines > exception.maxLines) {
      failures.push(`${item.name} grew to ${item.effectiveLines} effective lines, budget ${exception.maxLines}`);
    } else if (item.effectiveLines < exception.maxLines) {
      failures.push(`${item.name} exception is stale at ${exception.maxLines}; reduce it to ${item.effectiveLines}`);
    }
    if (typeof exception.reason !== "string" || exception.reason.trim().length < 12) {
      failures.push(`${item.name} exception needs a concrete reason`);
    }
  }
  for (const name of Object.keys(exceptions)) {
    if (!byName.has(name)) failures.push(`${name} exception does not match a Vue component`);
  }
  failures.push(...compareBudgetToMergeBase(budget, baseBudget));
  return { failures, scopedStyleLines: styleTotal };
}

export function compareBudgetToMergeBase(budget, baseBudget) {
  if (baseBudget === null) return [];

  const failures = [];
  if (budget.defaultMaxLines > baseBudget.defaultMaxLines) {
    failures.push(
      `defaultMaxLines grew from ${baseBudget.defaultMaxLines} to ${budget.defaultMaxLines}`,
    );
  }
  if (budget.scopedStyleLinesMax > baseBudget.scopedStyleLinesMax) {
    failures.push(
      `scopedStyleLinesMax grew from ${baseBudget.scopedStyleLinesMax} to ${budget.scopedStyleLinesMax}`,
    );
  }
  const baseExceptions = baseBudget.exceptions ?? {};
  for (const [name, exception] of Object.entries(budget.exceptions ?? {})) {
    const baseException = baseExceptions[name];
    if (baseException === undefined) {
      failures.push(`${name} is a new component budget exception relative to merge-base`);
    } else if (exception.maxLines > baseException.maxLines) {
      failures.push(
        `${name} exception grew from ${baseException.maxLines} to ${exception.maxLines}`,
      );
    }
  }
  return failures;
}

export function inspectWebModuleLayout(files) {
  const failures = [];
  const normalized = files.map(({ name, contents = "" }) => ({
    name: name.replaceAll("\\", "/"),
    contents,
  }));
  for (const file of normalized) {
    if (/^apps\/web\/src\/components\/[^/]+\.vue$/.test(file.name)) {
      failures.push(`${file.name}: root Vue components must be assigned to a feature directory`);
    }
    if (/^apps\/web\/src\/composables\/[^/]+\.ts$/.test(file.name)) {
      failures.push(`${file.name}: root composables must be assigned to an owner directory`);
    }
    if (
      /^apps\/web\/src\/features\/(?:strategyVisualBuilder|pineSourceStructure)/.test(
        file.name,
      )
    ) {
      failures.push(`${file.name}: legacy flat feature path is forbidden`);
    }
    for (const match of file.contents.matchAll(
      /(?:from\s*|import\s*)["'](@\/features\/(?:strategy-builder|pine-structure)\/[^"']+)["']/g,
    )) {
      failures.push(`${file.name}: import ${match[1]} through the feature index`);
    }
  }
  for (const entry of [
    "apps/web/src/features/strategy-builder/index.ts",
    "apps/web/src/features/pine-structure/index.ts",
  ]) {
    if (!normalized.some((file) => file.name === entry)) {
      failures.push(`${entry}: required feature index is missing`);
    }
  }
  return failures;
}

function countLines(contents) {
  return contents ? contents.split("\n").length - (contents.endsWith("\n") ? 1 : 0) : 0;
}

function walk(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = resolve(directory, entry.name);
    return entry.isDirectory() ? walk(path) : [path];
  });
}

function main() {
  const root = resolve(process.cwd());
  const budget = JSON.parse(readFileSync(resolve(root, "scripts/web-component-budget.json"), "utf8"));
  const sourceFiles = walk(resolve(root, "apps/web/src"));
  const components = sourceFiles
    .filter((path) => path.endsWith(".vue"))
    .map((path) => ({
      name: relative(root, path).split(sep).join("/"),
      ...inspectVueSource(readFileSync(path, "utf8"), (src) => {
        if (!src.startsWith(".")) throw new Error(`${path}: style src must be local: ${src}`);
        return readFileSync(resolve(dirname(path), src), "utf8");
      }),
    }));
  const result = compareWebComponentBudget(components, budget, readMergeBaseBudget(root));
  result.failures.push(
    ...inspectWebModuleLayout(
      sourceFiles
        .filter((path) => /\.(?:ts|vue)$/.test(path))
        .map((path) => ({
          name: relative(root, path).split(sep).join("/"),
          contents: readFileSync(path, "utf8"),
        })),
    ),
  );
  if (result.failures.length) {
    console.error("web component budget regressed:");
    result.failures.forEach((failure) => console.error(`- ${failure}`));
    process.exitCode = 1;
    return;
  }
  const oversized = components.filter((item) => item.effectiveLines > budget.defaultMaxLines).length;
  console.log(`web component and module layout budget passed: ${components.length} components, ${oversized} frozen exceptions, ${result.scopedStyleLines}/${budget.scopedStyleLinesMax} scoped style lines.`);
}

function readMergeBaseBudget(root) {
  const base = process.env.JFTRADE_DIFF_BASE || defaultBase(root);
  if (base === "") return null;
  const mergeBase = git(root, ["merge-base", base, "HEAD"]).trim();
  const path = `${mergeBase}:scripts/web-component-budget.json`;
  try {
    git(root, ["cat-file", "-e", path]);
  } catch {
    return null;
  }
  return JSON.parse(git(root, ["show", path]));
}

function defaultBase(root) {
  for (const candidate of ["origin/main", "HEAD^"]) {
    try {
      git(root, ["rev-parse", "--verify", candidate]);
      return candidate;
    } catch {
      // Try the next local baseline.
    }
  }
  return "";
}

function git(cwd, args) {
  return execFileSync("git", args, {
    cwd,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  });
}

if (resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) main();
