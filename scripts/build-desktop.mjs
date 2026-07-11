import process from "node:process";

import { spawnChecked } from "./lib/spawn.mjs";
import { resolveDesktopBuildMetadata } from "./lib/desktop-release-metadata.mjs";

const aliases = {
  darwin: "darwin",
  macos: "darwin",
  win32: "windows",
  windows: "windows",
  linux: "linux",
};
const host = aliases[process.platform] || process.platform;
const requested = parseSpec(
  process.env.JFTRADE_DESKTOP_TARGET || process.argv[2] || host,
);
if (!requested.target)
  fail(`Unknown desktop target: ${process.argv[2] || host}`);
if (requested.target !== host) {
  fail(
    `Desktop GUI builds run on their native OS (${requested.target} requested on ${host}).`,
  );
}

const metadata = resolveDesktopBuildMetadata();
const arch = process.env.GOARCH || requested.arch || hostArch();
runWailsTask(`${requested.target}:build`, arch, metadata);

export function runWailsTask(task, arch, metadata, extra = []) {
  const args = [
    "scripts/wails3.mjs",
    "task",
    task,
    `ARCH=${arch}`,
    `VERSION=${metadata.numericVersion}`,
    `COMMIT=${metadata.commit}`,
    `BUILD_TIME=${metadata.buildTime}`,
    ...extra,
  ];
  const status = spawnChecked(process.execPath, args, { cwd: process.cwd() });
  if (status !== 0) process.exit(status);
}

function parseSpec(value) {
  const normalized = String(value || "").toLowerCase();
  const match = normalized.match(/^(.*?)[-_:](amd64|arm64)$/);
  if (match) return { target: aliases[match[1]] || "", arch: match[2] };
  return { target: aliases[normalized] || "", arch: "" };
}

function hostArch() {
  if (process.arch === "arm64") return "arm64";
  if (process.arch === "x64") return "amd64";
  fail(`Unsupported desktop architecture: ${process.arch}`);
}

function fail(message) {
  console.error(message);
  process.exit(1);
}
