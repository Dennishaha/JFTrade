#!/usr/bin/env node
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const defaultPolicyPath = fileURLToPath(new URL("./layout-policy.json", import.meta.url));

function walkFiles(root) {
  if (!fs.existsSync(root)) return [];
  const files = [];
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    const entryPath = path.join(root, entry.name);
    if (entry.isDirectory()) files.push(...walkFiles(entryPath));
    else if (entry.isFile()) files.push(entryPath);
  }
  return files;
}

function relativePath(root, target) {
  return path.relative(root, target).split(path.sep).join("/");
}

function workspaceDependencyNames(pkg, workspaceNames) {
  return new Set(
    pkg.dependencies
      .map((dependency) => dependency.rename ?? dependency.name)
      .filter((name) => workspaceNames.has(name)),
  );
}

function validatePackageContents(root, declaration, errors) {
  const packageRoot = path.join(root, declaration.path);
  const sourceFiles = walkFiles(path.join(packageRoot, "src")).filter((file) => file.endsWith(".rs"));
  if (sourceFiles.length === 0) {
    errors.push(`${declaration.name} has no production Rust source`);
    return;
  }
  const integrationTests = walkFiles(path.join(packageRoot, "tests")).filter((file) => file.endsWith(".rs"));
  const hasNearbyTests = sourceFiles.some((file) => fs.readFileSync(file, "utf8").includes("#[cfg(test)]"));
  if (integrationTests.length === 0 && !hasNearbyTests) {
    errors.push(`${declaration.name} has no colocated or integration behavior tests`);
  }
}

export function validateRustFileLengths(files, maximumLines = 800) {
  const errors = [];
  for (const [file, contents] of files) {
    const lineCount = contents === "" ? 0 : contents.split(/\r?\n/).length - Number(contents.endsWith("\n"));
    if (lineCount > maximumLines) {
      errors.push(`${file} has ${lineCount} lines; production Rust limit is ${maximumLines}`);
    }
  }
  return errors;
}

function validateBoundedProductionFileLengths(root) {
  const boundedFamilies = [
    ["crates/jftrade-engine/src", "product"],
    ["apps/desktop/src-tauri/src", "native"],
  ];
  const files = boundedFamilies.flatMap(([sourcePath, prefix]) => walkFiles(path.join(root, sourcePath))
    .filter((file) => path.basename(file).startsWith(prefix))
    .filter((file) => file.endsWith(".rs") && !file.endsWith("_tests.rs"))
    .map((file) => [relativePath(root, file), fs.readFileSync(file, "utf8")]));
  return validateRustFileLengths(files);
}

export function validateLayoutPolicy(policy, metadata, options = {}) {
  const root = options.repositoryRoot ?? repositoryRoot;
  const pathExists = options.pathExists ?? fs.existsSync;
  const checkContents = options.checkContents ?? true;
  const errors = [];
  const declarations = new Map();

  for (const declaration of policy.packages ?? []) {
    if (declarations.has(declaration.name)) errors.push(`duplicate package declaration: ${declaration.name}`);
    declarations.set(declaration.name, declaration);
    for (const field of ["path", "status", "layer", "goOwner", "rustOwner", "cutover", "goDeleteWhen"]) {
      if (!declaration[field]) errors.push(`${declaration.name} is missing ${field}`);
    }
    if (!Number.isInteger(declaration.stage) || declaration.stage < 1) {
      errors.push(`${declaration.name} has an invalid migration stage`);
    }
    const segments = declaration.name.split("-");
    const banned = segments.find((segment) => policy.bannedCrateSegments?.includes(segment));
    if (banned) errors.push(`${declaration.name} uses banned ownerless segment ${banned}`);
  }

  const workspacePackages = metadata.packages.filter((pkg) => metadata.workspace_members.includes(pkg.id));
  const workspaceNames = new Set(workspacePackages.map((pkg) => pkg.name));
  for (const pkg of workspacePackages) {
    const declaration = declarations.get(pkg.name);
    if (!declaration) {
      errors.push(`workspace package ${pkg.name} is not registered in layout-policy.json`);
      continue;
    }
    if (declaration.status !== "active") errors.push(`workspace package ${pkg.name} is not marked active`);
    const actualPath = relativePath(root, path.dirname(pkg.manifest_path));
    if (actualPath !== declaration.path) {
      errors.push(`${pkg.name} path is ${actualPath}; expected ${declaration.path}`);
    }
    const allowed = new Set(declaration.allowedWorkspaceDependencies ?? []);
    for (const dependency of workspaceDependencyNames(pkg, workspaceNames)) {
      if (!allowed.has(dependency)) errors.push(`${pkg.name} may not depend on workspace package ${dependency}`);
    }
    for (const dependency of allowed) {
      if (!declarations.has(dependency)) errors.push(`${pkg.name} allows undeclared workspace dependency ${dependency}`);
    }
    if (checkContents) validatePackageContents(root, declaration, errors);
  }

  for (const declaration of declarations.values()) {
    const exists = pathExists(path.join(root, declaration.path));
    if (declaration.status === "active" && !workspaceNames.has(declaration.name)) {
      errors.push(`active package ${declaration.name} is missing from the Cargo workspace`);
    }
    if (declaration.status === "planned" && exists) {
      errors.push(`planned package path exists before activation: ${declaration.path}`);
    }
  }
  return errors;
}

export function loadCargoMetadata(root = repositoryRoot) {
  return JSON.parse(execFileSync(
    "cargo",
    ["metadata", "--no-deps", "--format-version=1"],
    { cwd: root, encoding: "utf8", stdio: ["ignore", "pipe", "inherit"] },
  ));
}

export function checkLayout(root = repositoryRoot, policyPath = defaultPolicyPath) {
  const policy = JSON.parse(fs.readFileSync(policyPath, "utf8"));
  return [
    ...validateLayoutPolicy(policy, loadCargoMetadata(root), { repositoryRoot: root }),
    ...validateBoundedProductionFileLengths(root),
  ];
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  const errors = checkLayout();
  if (errors.length > 0) {
    console.error(errors.map((error) => `- ${error}`).join("\n"));
    process.exitCode = 1;
  } else {
    console.log("Rust migration layout policy passed.");
  }
}
