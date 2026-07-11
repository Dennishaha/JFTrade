import process from "node:process";

import { spawnChecked } from "./lib/spawn.mjs";

const platform = process.argv[2];
const input = process.argv[3];
if (!platform || !input)
  fail("Usage: sign-desktop-release.mjs <macos|windows> <artifact>");

const args = ["scripts/wails3.mjs", "tool", "sign", "--input", input];
if (platform === "macos") {
  const identity = credential("JFTRADE_MACOS_SIGN_IDENTITY");
  const profile = credential("JFTRADE_MACOS_NOTARY_PROFILE");
  requireComplete([identity, profile], "macOS");
  if (!identity) process.exit(0);
  args.push(
    "--identity",
    identity,
    "--keychain-profile",
    profile,
    "--notarize",
    "--hardened-runtime",
  );
} else if (platform === "windows") {
  const certificate = credential("JFTRADE_WINDOWS_CERTIFICATE");
  const password = credential("JFTRADE_WINDOWS_CERTIFICATE_PASSWORD");
  requireComplete([certificate, password], "Windows");
  if (!certificate) process.exit(0);
  args.push(
    "--certificate",
    certificate,
    "--password",
    password,
    "--timestamp",
    "http://timestamp.digicert.com",
  );
} else {
  fail(`Unsupported signing platform: ${platform}`);
}

const status = spawnChecked(process.execPath, args, { cwd: process.cwd() });
if (status !== 0) process.exit(status);

function credential(name) {
  return String(process.env[name] || "").trim();
}
function requireComplete(values, name) {
  const count = values.filter(Boolean).length;
  if (count !== 0 && count !== values.length)
    fail(`${name} signing credentials must be all set or all unset.`);
}
function fail(message) {
  console.error(message);
  process.exit(1);
}
