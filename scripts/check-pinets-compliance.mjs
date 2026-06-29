#!/usr/bin/env node
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";

const require = createRequire(import.meta.url);
const pinetsEntry = require.resolve("pinets");
const pinetsPackage = JSON.parse(readFileSync(join(dirname(dirname(pinetsEntry)), "package.json"), "utf8"));
const notice = readFileSync("docs/legal/third-party-notices.md", "utf8");

const requiredNoticeText = [
  "PineTS / pinets",
  "AGPL-3.0-only",
  "https://github.com/LuxAlgo/PineTS",
  "runtime=pine-pinets",
  "workers/pineworker",
  "scripts/pinets-worker.mjs",
  "scripts/build-pineworker-assets.mjs",
  "scripts/build-pineworker-dev.mjs",
  "npm run build:pineworker",
  "npm run check:pinets-release",
  "corresponding source",
  "network users",
];

if (pinetsPackage.name !== "pinets") {
  throw new Error(`expected pinets package, got ${pinetsPackage.name}`);
}
if (pinetsPackage.version !== "0.9.26") {
  throw new Error(`pinets version = ${pinetsPackage.version}, want 0.9.26`);
}
if (pinetsPackage.license !== "AGPL-3.0-only") {
  throw new Error(`pinets license = ${pinetsPackage.license}, want AGPL-3.0-only`);
}
for (const needle of requiredNoticeText) {
  if (!notice.includes(needle)) {
    throw new Error(`docs/legal/third-party-notices.md is missing ${JSON.stringify(needle)}`);
  }
}
if (/optional Node worker used by the experimental `pinets-shadow` engine/.test(notice)) {
  throw new Error("third-party notice still describes PineTS as shadow-only");
}

console.log(`pinets compliance notice covers ${pinetsPackage.name}@${pinetsPackage.version} ${pinetsPackage.license}`);
