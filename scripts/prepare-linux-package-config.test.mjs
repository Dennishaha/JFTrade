import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";

const directory = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-nfpm-"));
const output = path.join(directory, "nfpm.yaml");
const result = spawnSync(
  process.execPath,
  [
    "scripts/prepare-linux-package-config.mjs",
    "build/linux/nfpm.yaml",
    output,
    "1.2.3",
    "amd64",
    path.join(directory, "JFTrade"),
    path.join(directory, "JFTrade.desktop"),
    path.join(directory, "JFTrade.png"),
    path.join(directory, "LICENSE"),
    path.join(directory, "THIRD-PARTY-NOTICES.md"),
  ],
  { encoding: "utf8" },
);
assert.equal(result.status, 0, result.stderr);
const config = fs.readFileSync(output, "utf8");
assert(config.includes("version: 1.2.3"));
assert(config.includes("arch: amd64"));
assert(config.includes("name: jftrade"));
assert(
  config.includes(
    "maintainer: JFTrade Maintainers <38273998+Dennishaha@users.noreply.github.com>",
  ),
);
assert(config.includes("homepage: https://github.com/Dennishaha/jftrade"));
assert(config.includes("license: AGPL-3.0-only"));
assert(!config.includes("LicenseRef-Proprietary"));
assert(!config.includes("archlinux"));
assert(config.includes(path.join(directory, "LICENSE")));
assert(config.includes(path.join(directory, "THIRD-PARTY-NOTICES.md")));
assert(!config.includes("__"));
fs.rmSync(directory, { recursive: true, force: true });
