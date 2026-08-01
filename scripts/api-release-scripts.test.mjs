import assert from "node:assert/strict";
import fs from "node:fs";

const scripts = [
  { path: "build-release.sh", buildMarker: 'for target in "${TARGETS[@]}"' },
  { path: "build-release.ps1", buildMarker: "foreach ($target in $targets)" },
];

for (const script of scripts) {
  const source = fs.readFileSync(script.path, "utf8");
  const installIndex = source.indexOf("pnpm install --frozen-lockfile");
  const auditIndex = source.indexOf("pnpm run audit:dependencies");
  const frontendIndex = source.indexOf("pnpm run build:web");
  const workerIndex = source.indexOf("pnpm run build:pineworker");
  const yfinanceCheckIndex = source.indexOf(
    script.path.endsWith(".sh")
      ? "require_yfinance_assets"
      : "Assert-YFinanceAssets",
    workerIndex,
  );
  const testIndex = source.indexOf("go test ./... -count=1 -timeout 300s");
  const buildIndex = source.indexOf(script.buildMarker);

  assert(installIndex >= 0, `${script.path} does not install locked dependencies`);
  assert(!source.includes("dependencies are usable; continuing"));
  assert(auditIndex >= 0, `${script.path} does not audit locked dependencies`);
  assert(frontendIndex > auditIndex, `${script.path} builds frontend assets before audit passes`);
  assert(workerIndex >= 0, `${script.path} does not build the PineTS worker`);
  assert(
    yfinanceCheckIndex > workerIndex,
    `${script.path} does not verify pre-staged yfinance helpers after building shared assets`,
  );
  assert(
    testIndex > yfinanceCheckIndex,
    `${script.path} does not test after preparing release assets`,
  );
  assert(buildIndex > testIndex, `${script.path} builds release binaries before tests pass`);
  for (const asset of [
    "yfinance-sidecar-darwin-arm64",
    "yfinance-sidecar-linux-amd64",
    "yfinance-sidecar-windows-amd64",
    "yfinance-sidecar-windows-arm64",
  ]) {
    assert(source.includes(asset), `${script.path} does not require ${asset}`);
  }
  assert(
    source.includes("matching OS/architecture") &&
      source.includes("pnpm run build:yfinance-sidecar"),
    `${script.path} does not explain how to stage native yfinance helpers`,
  );
}

console.log("API release script tests passed");
