#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const mapPath = path.join(repoRoot, "scripts", "module-map.json");
const contextMap = JSON.parse(fs.readFileSync(mapPath, "utf8"));

function matchesPath(file, prefix) {
  return file === prefix || file.startsWith(`${prefix}/`);
}

function trackedFiles(root) {
  return execFileSync("git", ["ls-files", "-z"], {
    cwd: root,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  }).split("\0").filter(Boolean);
}

export function validateAiContext(root = repoRoot, map = contextMap, options = {}) {
  const errors = [];
  for (const module of map.modules ?? []) {
    if (!module.id || !Array.isArray(module.paths) || module.paths.length === 0) {
      errors.push("每个模块必须有 id 和至少一个 paths");
      continue;
    }
    for (const relativePath of module.paths) {
      if (!fs.existsSync(path.join(root, relativePath))) {
        errors.push(`${module.id}: 路径不存在 ${relativePath}`);
      }
    }
  }
  for (const relativePath of map.requiredInstructionFiles ?? []) {
    if (!fs.existsSync(path.join(root, relativePath))) {
      errors.push(`缺少 AI 指令文件 ${relativePath}`);
    }
  }
  const sourceRoots = map.sourceRoots ?? [];
  const sourceExtensions = new Set(map.sourceExtensions ?? []);
  if (sourceRoots.length > 0 && sourceExtensions.size > 0) {
    const controlledSources = (options.trackedFiles ?? trackedFiles(root)).filter((file) => (
      sourceRoots.some((sourceRoot) => matchesPath(file, sourceRoot))
      && sourceExtensions.has(path.extname(file))
    ));
    const modulePaths = (map.modules ?? []).flatMap((module) => module.paths ?? []);
    const ignoredPaths = map.ignoredSourcePaths ?? [];
    for (const file of controlledSources) {
      if (!modulePaths.some((modulePath) => matchesPath(file, modulePath))
        && !ignoredPaths.some((ignoredPath) => matchesPath(file, ignoredPath))) {
        errors.push(`源码未归属任何模块且未显式忽略 ${file}`);
      }
    }
  }
  const contextFiles = [
    ...(map.requiredInstructionFiles ?? []),
    ".github/agents",
    ".github/instructions",
    ".github/skills",
  ];
  const files = contextFiles.flatMap((relativePath) => collectFiles(path.join(root, relativePath)));
  for (const file of files) {
    const source = fs.readFileSync(file, "utf8");
    for (const legacyPath of map.legacyPaths ?? []) {
      if (source.includes(legacyPath)) {
        errors.push(`${path.relative(root, file)} 仍引用已删除路径 ${legacyPath}`);
      }
    }
  }
  return errors;
}

function collectFiles(target) {
  if (!fs.existsSync(target)) return [];
  const stat = fs.statSync(target);
  if (stat.isFile()) return [target];
  return fs.readdirSync(target, { withFileTypes: true }).flatMap((entry) => (
    entry.isDirectory() ? collectFiles(path.join(target, entry.name)) : [path.join(target, entry.name)]
  ));
}

if (path.resolve(process.argv[1] ?? "") === path.resolve(fileURLToPath(import.meta.url))) {
  const errors = validateAiContext();
  if (errors.length > 0) {
    console.error(errors.map((error) => `- ${error}`).join("\n"));
    process.exitCode = 1;
  } else {
    console.log(`AI context check passed: ${contextMap.modules.length} modules, ${contextMap.requiredInstructionFiles.length} instruction files.`);
  }
}
