#!/usr/bin/env node

import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { REQUIRED_PLATFORMS } from "./check-release-candidate.mjs";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function sha256(filePath) {
  return createHash("sha256").update(fs.readFileSync(filePath)).digest("hex");
}

function requiredFile(root, relative, label) {
  if (typeof relative !== "string" || relative.trim() === "" || path.isAbsolute(relative)
    || relative.includes("\\") || relative.split("/").some((part) => !part || part === "." || part === "..")) {
    throw new Error(`${label} must be a safe relative POSIX path`);
  }
  const resolved = path.resolve(root, relative);
  const base = path.resolve(root);
  const outside = path.relative(base, resolved);
  if (outside === ".." || outside.startsWith(`..${path.sep}`)) throw new Error(`${label} escapes its bundle root`);
  let current = base;
  for (const part of path.relative(base, resolved).split(path.sep).filter(Boolean)) {
    current = path.join(current, part);
    let entry;
    try {
      entry = fs.lstatSync(current);
    } catch (error) {
      throw new Error(`${label} is missing: ${relative} (${error.message})`);
    }
    if (entry.isSymbolicLink()) throw new Error(`${label} must not traverse a symbolic link: ${relative}`);
  }
  const stat = fs.lstatSync(resolved);
  if (!stat.isFile() || stat.size === 0) throw new Error(`${label} is missing or empty: ${relative}`);
  return { path: resolved, relative, size: stat.size, sha256: sha256(resolved) };
}

function compareFile(candidateRoot, releaseRoot, reference, label, seenReleaseNames) {
  const candidate = requiredFile(candidateRoot, reference.path, `${label} candidate file`);
  const releaseName = path.posix.basename(reference.path);
  if (releaseName !== reference.path) {
    throw new Error(`${label} must be a top-level release file: ${reference.path}`);
  }
  if (seenReleaseNames.has(releaseName)) {
    throw new Error(`${label} duplicates top-level release basename: ${releaseName}`);
  }
  seenReleaseNames.add(releaseName);
  const release = requiredFile(releaseRoot, releaseName, `${label} published file`);
  if (reference.sha256 !== candidate.sha256) throw new Error(`${label} candidate evidence digest is stale`);
  if (reference.size !== undefined && reference.size !== candidate.size) throw new Error(`${label} candidate evidence size is stale`);
  if (release.sha256 !== candidate.sha256 || release.size !== candidate.size) {
    throw new Error(`${label} published file differs from candidate bundle: ${releaseName}`);
  }
  return { path: releaseName, sha256: release.sha256, size: release.size };
}

function readJson(filePath, label) {
  let text;
  try {
    text = fs.readFileSync(filePath, "utf8");
  } catch (error) {
    throw new Error(`cannot read ${label}: ${error.message}`);
  }
  try {
    return JSON.parse(text);
  } catch (error) {
    throw new Error(`cannot parse ${label}: ${error.message}`);
  }
}

/** Verify that the published files are byte-identical to one downloaded candidate bundle. */
export function verifyReleaseCandidateBundle({ evidencePath, candidateRoot, releaseRoot } = {}) {
  const evidence = readJson(path.resolve(evidencePath ?? ""), "canonical candidate evidence");
  if (!isRecord(evidence) || !isRecord(evidence.platforms)) throw new Error("canonical candidate evidence has no platforms");
  if (!Array.isArray(evidence.sourceArtifacts) || evidence.sourceArtifacts.length === 0) {
    throw new Error("canonical candidate evidence must include source artifact metadata");
  }
  const platforms = Object.keys(evidence.platforms);
  const missingPlatforms = REQUIRED_PLATFORMS.filter((platform) => !platforms.includes(platform));
  const unknownPlatforms = platforms.filter((platform) => !REQUIRED_PLATFORMS.includes(platform));
  if (missingPlatforms.length > 0 || unknownPlatforms.length > 0 || platforms.length !== REQUIRED_PLATFORMS.length) {
    throw new Error(`canonical candidate evidence must contain exactly the required platforms (missing: ${missingPlatforms.join(",") || "none"}; unknown: ${unknownPlatforms.join(",") || "none"})`);
  }
  const candidateBase = path.resolve(candidateRoot ?? "");
  const releaseBase = path.resolve(releaseRoot ?? "");
  const files = [];
  const seenReleaseNames = new Set();
  for (const platform of REQUIRED_PLATFORMS) {
    const value = evidence.platforms[platform];
    if (!isRecord(value) || !isRecord(value.manifest) || !Array.isArray(value.artifacts)) {
      throw new Error(`canonical candidate evidence platform is incomplete: ${platform}`);
    }
    files.push(compareFile(candidateBase, releaseBase, value.manifest, `${platform}.manifest`, seenReleaseNames));
    for (const [index, artifact] of value.artifacts.entries()) {
      files.push(compareFile(candidateBase, releaseBase, artifact, `${platform}.artifacts[${index}]`, seenReleaseNames));
    }
  }
  return {
    status: "verified",
    evidencePath: path.resolve(evidencePath),
    candidateRoot: candidateBase,
    releaseRoot: releaseBase,
    sourceArtifacts: evidence.sourceArtifacts,
    files,
  };
}

function parseArgs(args) {
  const values = {};
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];
    if (!argument.startsWith("--")) throw new Error(`unknown argument: ${argument}`);
    const [key, inline] = argument.slice(2).split("=", 2);
    const value = inline ?? args[++index];
    if (!value) throw new Error(`--${key} requires a value`);
    values[key] = value;
  }
  for (const key of ["evidence", "candidate-root", "release-root"]) {
    if (!values[key]) throw new Error(`--${key} is required`);
  }
  return values;
}

export function main(args = process.argv.slice(2)) {
  try {
    const values = parseArgs(args);
    const report = verifyReleaseCandidateBundle({
      evidencePath: values.evidence,
      candidateRoot: values["candidate-root"],
      releaseRoot: values["release-root"],
    });
    console.log(JSON.stringify(report, null, 2));
    return 0;
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    return 1;
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = main();
}
