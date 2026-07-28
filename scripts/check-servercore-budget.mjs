#!/usr/bin/env node
import { readdirSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const defaultPackagePath = "internal/app/apiserver/servercore";
const defaultBudgetPath = "scripts/servercore-budget.json";

export function inspectServercore(sources, dependencies, testSources = []) {
  let productionLines = 0;
  let testLines = 0;
  let serverMethods = 0;
  let applicationMethods = 0;
  let serverFields = 0;
  let applicationFields = 0;
  const directImportFiles = Object.fromEntries(dependencies.map((dependency) => [dependency, []]));

  for (const source of sources) {
    productionLines += countLines(source.contents);
    serverMethods += countReceiverMethods(source.contents, "Server");
    applicationMethods += countReceiverMethods(source.contents, "serverApplication");
    serverFields += countStructFields(source.contents, "Server");
    applicationFields += countStructFields(source.contents, "serverApplication");
    for (const dependency of dependencies) {
      if (new RegExp(`^[ \\t]*(?:[A-Za-z_.]\\w*[ \\t]+)?[\"\\x60]${escapeRegExp(dependency)}[\"\\x60]`, "m").test(source.contents)) {
        directImportFiles[dependency].push(source.name);
      }
    }
  }
  for (const source of testSources) {
    testLines += countLines(source.contents);
  }
  for (const files of Object.values(directImportFiles)) files.sort();
  return {
    productionLines,
    testLines,
    serverMethods,
    applicationMethods,
    effectiveServerMethods: serverMethods + applicationMethods,
    serverFields,
    applicationFields,
    aggregateFields: serverFields + applicationFields,
    directImportFiles,
  };
}

export function compareBudget(actual, budget) {
  const failures = [];
  const dimensions = [
    ["productionLines", "productionLinesMax", "production lines"],
    ["testLines", "testLinesMax", "test lines"],
    ["serverMethods", "serverMethodsMax", "*Server methods"],
    ["applicationMethods", "applicationMethodsMax", "serverApplication methods"],
    ["effectiveServerMethods", "effectiveServerMethodsMax", "effective *Server method surface"],
    ["aggregateFields", "aggregateFieldsMax", "aggregate Server fields"],
  ];
  for (const [actualKey, budgetKey, label] of dimensions) {
    if (!Number.isInteger(budget[budgetKey]) || budget[budgetKey] < 0) {
      failures.push(`${budgetKey} must be a non-negative integer`);
    } else if (actual[actualKey] > budget[budgetKey]) {
      failures.push(`${label} ${actual[actualKey]} exceed budget ${budget[budgetKey]}`);
    }
  }
  for (const [dependency, files] of Object.entries(actual.directImportFiles)) {
    const allowed = new Set(budget.directImportFiles[dependency] || []);
    const growth = files.filter((file) => !allowed.has(file));
    if (growth.length > 0) failures.push(`${dependency} direct-import file set grew: ${growth.join(", ")}`);
  }
  return failures;
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  const repoRoot = resolve(options.repoRoot);
  const packagePath = resolve(repoRoot, options.packagePath);
  const budget = JSON.parse(readFileSync(resolve(repoRoot, options.budgetPath), "utf8"));
  const entries = readdirSync(packagePath, { withFileTypes: true });
  const readSources = (predicate) => entries
    .filter((entry) => entry.isFile() && predicate(entry.name))
    .map((entry) => ({
      name: entry.name,
      contents: readFileSync(resolve(packagePath, entry.name), "utf8"),
    }));
  const sources = readSources(
    (name) => name.endsWith(".go") && !name.endsWith("_test.go"),
  );
  const testSources = readSources((name) => name.endsWith("_test.go"));
  const actual = inspectServercore(
    sources,
    Object.keys(budget.directImportFiles),
    testSources,
  );
  const failures = compareBudget(actual, budget);
  if (failures.length > 0) {
    console.error("servercore temporary budget regressed:");
    for (const failure of failures) console.error(`- ${failure}`);
    process.exitCode = 1;
    return;
  }
  console.log(
    `servercore budget passed: ${actual.productionLines}/${budget.productionLinesMax} production lines, ` +
    `${actual.testLines}/${budget.testLinesMax} test lines, ` +
    `${actual.serverMethods}/${budget.serverMethodsMax} *Server methods, ` +
    `${actual.applicationMethods}/${budget.applicationMethodsMax} serverApplication methods, ` +
    `${actual.effectiveServerMethods}/${budget.effectiveServerMethodsMax} effective methods, ` +
    `${actual.aggregateFields}/${budget.aggregateFieldsMax} aggregate fields.`,
  );
}

function countLines(contents) {
  if (!contents) return 0;
  return contents.split("\n").length - (contents.endsWith("\n") ? 1 : 0);
}

function countReceiverMethods(contents, receiverType) {
  const escapedType = escapeRegExp(receiverType);
  return [
    ...contents.matchAll(
      new RegExp(
        `^func\\s*\\(\\s*(?:[A-Za-z_]\\w*\\s+)?\\*?${escapedType}\\s*\\)\\s*[A-Za-z_]\\w*\\s*\\(`,
        "gm",
      ),
    ),
  ].length;
}

function countStructFields(contents, structType) {
  const escapedType = escapeRegExp(structType);
  const declaration = new RegExp(`\\btype\\s+${escapedType}\\s+struct\\s*\\{`, "m").exec(contents);
  if (!declaration) return 0;
  const openBrace = contents.indexOf("{", declaration.index);
  const body = structBody(contents, openBrace);
  if (body == null) return 0;

  let fields = 0;
  let depth = 0;
  for (const sourceLine of body.split("\n")) {
    const line = sourceLine.replace(/\/\/.*$/, "").trim();
    if (depth === 0 && line !== "") {
      const named = /^([A-Za-z_]\w*(?:\s*,\s*[A-Za-z_]\w*)*)\s+/.exec(line);
      if (named) {
        fields += named[1].split(",").length;
      } else if (/^\*?[A-Za-z_]\w*(?:\.[A-Za-z_]\w*)?(?:\[[^\]]*\])?(?:\s+`[^`]*`)?$/.test(line)) {
        fields += 1;
      }
    }
    depth += countCharacter(line, "{") - countCharacter(line, "}");
  }
  return fields;
}

function structBody(contents, openBrace) {
  let depth = 1;
  for (let index = openBrace + 1; index < contents.length; index += 1) {
    if (contents[index] === "{") depth += 1;
    if (contents[index] === "}") depth -= 1;
    if (depth === 0) return contents.slice(openBrace + 1, index);
  }
  return null;
}

function countCharacter(value, character) {
  return [...value].filter((current) => current === character).length;
}

function parseArgs(args) {
  const options = { budgetPath: defaultBudgetPath, packagePath: defaultPackagePath, repoRoot: process.cwd() };
  for (let index = 0; index < args.length; index += 1) {
    const flag = args[index];
    const value = args[index + 1];
    if (!["--repo-root", "--package", "--budget"].includes(flag) || !value) {
      throw new Error("Usage: node scripts/check-servercore-budget.mjs [--repo-root <path>] [--package <path>] [--budget <path>]");
    }
    if (flag === "--repo-root") options.repoRoot = value;
    if (flag === "--package") options.packagePath = value;
    if (flag === "--budget") options.budgetPath = value;
    index += 1;
  }
  return options;
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

if (resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  try {
    main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
