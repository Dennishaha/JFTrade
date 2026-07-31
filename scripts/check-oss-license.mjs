#!/usr/bin/env node
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";

const AGPL_V3_OFFICIAL_SHA256 =
  "0d96a4ff68ad6d4b6f1f30f713b18d5184912ba8dd389f86aa7710db079abcb0";
const PROJECT_LICENSE = "AGPL-3.0-only";
const COPYRIGHT_NOTICE = "Copyright (C) 2026 JFTrade Contributors";
const YFINANCE_SIDECAR_EXTRAS = {
  runtime: [
    "yfinance==0.2.61",
    "curl_cffi==0.15.0",
    "fastapi==0.115.14",
    "uvicorn[standard]==0.35.0",
    "pydantic==2.12.2",
  ],
  build: ["pyinstaller==6.21.0"],
  test: [
    "httpx==0.28.1",
    "pytest==8.4.1",
    "pytest-asyncio==1.0.0",
  ],
};

function read(path) {
  return readFileSync(path, "utf8");
}

function requireText(text, needle, source) {
  if (!text.includes(needle)) {
    throw new Error(`${source} is missing ${JSON.stringify(needle)}`);
  }
}

function requireExactTomlStringArray(text, key, expected, source) {
  const match = text.match(
    new RegExp(`^${key}\\s*=\\s*\\[([\\s\\S]*?)^\\]`, "m"),
  );
  if (match == null) {
    throw new Error(`${source} is missing TOML array ${JSON.stringify(key)}`);
  }
  const actual = match[1]
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const item = line.match(/^"([^"]+)",$/);
      if (item == null) {
        throw new Error(
          `${source} has an unsupported ${key} entry ${JSON.stringify(line)}`,
        );
      }
      return item[1];
    });
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(
      `${source} ${key} = ${JSON.stringify(actual)}, want ${JSON.stringify(expected)}`,
    );
  }
}

const license = readFileSync("LICENSE");
const licenseHash = createHash("sha256").update(license).digest("hex");
if (licenseHash !== AGPL_V3_OFFICIAL_SHA256) {
  throw new Error(
    `LICENSE sha256 = ${licenseHash}, want official AGPLv3 ${AGPL_V3_OFFICIAL_SHA256}`,
  );
}

for (const manifestPath of [
  "package.json",
  "apps/web/package.json",
  "workers/pineworker/package.json",
]) {
  const manifest = JSON.parse(read(manifestPath));
  if (manifest.license !== PROJECT_LICENSE) {
    throw new Error(
      `${manifestPath} license = ${JSON.stringify(manifest.license)}, want ${PROJECT_LICENSE}`,
    );
  }
}

const yfinanceSidecarManifestPath =
  "workers/yfinance-sidecar/pyproject.toml";
const yfinanceSidecarManifest = read(yfinanceSidecarManifestPath);
for (const needle of [
  'name = "yfinance-sidecar"',
  'requires-python = ">=3.11"',
  `license = "${PROJECT_LICENSE}"`,
  'requires = ["setuptools==80.9.0"]',
]) {
  requireText(yfinanceSidecarManifest, needle, yfinanceSidecarManifestPath);
}
for (const [extra, dependencies] of Object.entries(YFINANCE_SIDECAR_EXTRAS)) {
  requireExactTomlStringArray(
    yfinanceSidecarManifest,
    extra,
    dependencies,
    yfinanceSidecarManifestPath,
  );
}

const readme = read("README.md");
for (const needle of [
  COPYRIGHT_NOTICE,
  PROJECT_LICENSE,
  "[LICENSE](LICENSE)",
  "docs/legal/third-party-notices.md",
  "不自动授权未来版本",
  "不提供任何明示或默示担保",
]) {
  requireText(readme, needle, "README.md");
}

const notice = read("docs/legal/third-party-notices.md");
for (const needle of [
  "pinets",
  "Version: `0.9.29`",
  "github.com/c9s/bbgo@v1.64.2",
  "Copyright (c) 2016 Mark Chenoweth",
  "Copyright Suneido Software Corp.",
  "Copyright 2000-2016 JetBrains s.r.o.",
  "Copyright 2000-2024 JetBrains s.r.o. and contributors",
  "Permission is hereby granted, free of charge",
  "Apache License",
  "Version 2.0, January 2004",
  "END OF TERMS AND CONDITIONS",
  "Corresponding Source",
]) {
  requireText(notice, needle, "docs/legal/third-party-notices.md");
}
for (const dependency of [
  "setuptools==80.9.0",
  ...Object.values(YFINANCE_SIDECAR_EXTRAS).flat(),
]) {
  const separator = dependency.indexOf("==");
  const packageName = dependency.slice(0, separator);
  const version = dependency.slice(separator + 2);
  requireText(
    notice,
    `| \`${packageName}\` | \`${version}\` |`,
    "docs/legal/third-party-notices.md",
  );
}
for (const needle of [
  "yfinance helper direct Python dependencies",
  "Apache-2.0",
  "BSD-3-Clause",
  "workers/yfinance-sidecar/pyproject.toml",
]) {
  requireText(notice, needle, "docs/legal/third-party-notices.md");
}

const licenseDoc = read("docs/legal/license.md");
for (const needle of [
  COPYRIGHT_NOTICE,
  PROJECT_LICENSE,
  "<<< ../../LICENSE{text}",
  "./third-party-notices.md",
]) {
  requireText(licenseDoc, needle, "docs/legal/license.md");
}

const legalUI = read("apps/web/src/components/settings/SettingsOpenSourceSection.vue");
for (const needle of [
  COPYRIGHT_NOTICE,
  PROJECT_LICENSE,
  "不提供任何明示或默示担保",
  "corresponding-source-link",
]) {
  requireText(legalUI, needle, "SettingsOpenSourceSection.vue");
}

const linuxPackage = read("build/linux/nfpm.yaml");
const desktopMetadata = [
  linuxPackage,
  read("build/config.yml"),
  read("build/Taskfile.yml"),
].join("\n");
if (/LicenseRef-Proprietary|license:\s*Proprietary/i.test(desktopMetadata)) {
  throw new Error("desktop package metadata still declares a proprietary license");
}
for (const needle of [
  "license: AGPL-3.0-only",
  "/usr/share/licenses/jftrade/LICENSE",
  "/usr/share/licenses/jftrade/THIRD-PARTY-NOTICES.md",
]) {
  requireText(linuxPackage, needle, "build/linux/nfpm.yaml");
}
requireText(desktopMetadata, COPYRIGHT_NOTICE, "desktop metadata");

const releaseWorkflow = read(".github/workflows/desktop-release.yml");
requireText(releaseWorkflow, "release/LICENSE", "desktop-release.yml");
requireText(
  releaseWorkflow,
  "release/THIRD-PARTY-NOTICES.md",
  "desktop-release.yml",
);
requireText(releaseWorkflow, "sha256sum > SHA256SUMS", "desktop-release.yml");
requireText(releaseWorkflow, "release/*", "desktop-release.yml");

console.log(
  `OSS license check passed: ${PROJECT_LICENSE}, LICENSE sha256 ${licenseHash}`,
);
