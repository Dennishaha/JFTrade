import assert from "node:assert/strict";
import fs from "node:fs";

const scripts = [
  { path: "build-release.sh" },
  { path: "build-release.ps1" },
];

for (const script of scripts) {
  const source = fs.readFileSync(script.path, "utf8");
  assert(source.includes("pnpm install --frozen-lockfile"), `${script.path} does not install locked dependencies`);
  assert(source.includes("cargo") && source.includes("rustup"), `${script.path} does not check the Rust toolchain`);
  assert(source.includes("pnpm run build:desktop"), `${script.path} does not invoke the Tauri release build`);
  assert(source.includes("JFTRADE_DESKTOP_RELEASE_TAG"), `${script.path} does not require a release tag`);
  assert(source.includes("pnpm run check:zero-go"), `${script.path} does not enforce the zero-Go release gate`);
}

console.log("API release script tests passed");
