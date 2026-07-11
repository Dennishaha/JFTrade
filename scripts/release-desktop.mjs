import fs from "node:fs";
import path from "node:path";
import process from "node:process";

import { spawnChecked } from "./lib/spawn.mjs";
import {
  requireDesktopReleaseMetadata,
  resolveDesktopBuildMetadata,
} from "./lib/desktop-release-metadata.mjs";

const aliases = {
  darwin: "darwin",
  macos: "darwin",
  win32: "windows",
  windows: "windows",
  linux: "linux",
};
const host = aliases[process.platform] || process.platform;
const spec = parseSpec(process.argv[2] || host);
const format = String(process.argv[3] || "").toLowerCase();
if (!spec.target) fail(`Unknown desktop release target: ${process.argv[2]}`);
if (spec.target !== host)
  fail(
    `Desktop releases run on their native OS (${spec.target} requested on ${host}).`,
  );

const metadata = requireDesktopReleaseMetadata(resolveDesktopBuildMetadata());
const arch = spec.arch || hostArch();
const task = resolveTask(spec.target, format);
const qualifier = signingQualifier(spec.target);
const args = [
  "scripts/wails3.mjs",
  "task",
  task,
  `ARCH=${arch}`,
  `VERSION=${metadata.version}`,
  `COMMIT=${metadata.commit}`,
  `BUILD_TIME=${metadata.buildTime}`,
  `QUALIFIER=${qualifier}`,
];
if (format === "rpm" || format === "archlinux" || format === "deb")
  args.push(`FORMAT=${format}`);
const status = spawnChecked(process.execPath, args, { cwd: process.cwd() });
if (status !== 0) process.exit(status);

for (const artifact of expectedArtifacts(
  spec.target,
  arch,
  metadata.version,
  format,
  qualifier,
)) {
  const stat = fs.statSync(artifact, { throwIfNoEntry: false });
  if (!stat?.isFile() || stat.size === 0)
    fail(`Desktop release artifact is missing or empty: ${artifact}`);
  console.log(`Desktop release artifact verified at ${artifact}`);
}

function resolveTask(target, selectedFormat) {
  if (target === "windows" && selectedFormat === "msix")
    return "windows:package:msix";
  if (target === "linux" && selectedFormat === "appimage")
    return "linux:package:appimage";
  if (
    target === "linux" &&
    ["deb", "rpm", "archlinux"].includes(selectedFormat)
  )
    return "linux:package:linux";
  return `${target}:package`;
}

function expectedArtifacts(target, arch, version, selectedFormat, qualifier) {
  const dir = path.join("dist", "desktop-release", `${target}-${arch}`);
  if (target === "darwin")
    return [
      path.join(dir, `JFTrade-${version}-macos-${arch}-${qualifier}.dmg`),
    ];
  if (target === "windows" && selectedFormat === "msix")
    return [path.join(dir, `JFTrade-${version}-windows-${arch}.msix`)];
  if (target === "windows")
    return [
      path.join(
        dir,
        arch === "amd64"
          ? `JFTrade-${version}-windows-x64-${qualifier}-setup.exe`
          : `JFTrade-${version}-windows-arm64-preview-${qualifier}-setup.exe`,
      ),
    ];
  if (selectedFormat === "appimage") return findBySuffix(dir, ".AppImage");
  if (["deb", "rpm", "archlinux"].includes(selectedFormat))
    return findBySuffix(
      dir,
      selectedFormat === "archlinux" ? ".pkg.tar.zst" : `.${selectedFormat}`,
    );
  return [
    path.join(dir, `jftrade-desktop-linux-${arch}`),
    ...findBySuffix(dir, ".AppImage"),
    ...findBySuffix(dir, ".deb"),
  ];
}

function findBySuffix(dir, suffix) {
  if (!fs.existsSync(dir)) return [path.join(dir, `missing${suffix}`)];
  const matches = fs
    .readdirSync(dir)
    .filter((entry) => entry.endsWith(suffix))
    .map((entry) => path.join(dir, entry));
  return matches.length ? matches : [path.join(dir, `missing${suffix}`)];
}

function parseSpec(value) {
  const normalized = String(value || "").toLowerCase();
  const match = normalized.match(/^(.*?)[-_:](amd64|arm64)$/);
  if (match) return { target: aliases[match[1]] || "", arch: match[2] };
  return { target: aliases[normalized] || "", arch: "" };
}
function signingQualifier(target) {
  const names =
    target === "darwin"
      ? ["JFTRADE_MACOS_SIGN_IDENTITY", "JFTRADE_MACOS_NOTARY_PROFILE"]
      : target === "windows"
        ? [
            "JFTRADE_WINDOWS_CERTIFICATE",
            "JFTRADE_WINDOWS_CERTIFICATE_PASSWORD",
          ]
        : [];
  const count = names.filter((name) =>
    String(process.env[name] || "").trim(),
  ).length;
  if (count !== 0 && count !== names.length)
    fail(
      `Signing credentials must be all set or all unset: ${names.join(", ")}`,
    );
  return count === names.length && count > 0 ? "signed" : "unsigned";
}
function hostArch() {
  return process.arch === "arm64" ? "arm64" : "amd64";
}
function fail(message) {
  console.error(message);
  process.exit(1);
}
