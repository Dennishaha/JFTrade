import assert from "node:assert/strict";
import fs from "node:fs";

const scripts = [
  { path: "build-release.sh", buildMarker: 'for target in "${TARGETS[@]}"' },
  { path: "build-release.ps1", buildMarker: "foreach ($target in $targets)" },
];

for (const script of scripts) {
  const source = fs.readFileSync(script.path, "utf8");
  const workerIndex = source.indexOf("npm run build:pineworker");
  const testIndex = source.indexOf("go test ./... -count=1 -timeout 300s");
  const buildIndex = source.indexOf(script.buildMarker);

  assert(workerIndex >= 0, `${script.path} does not build the PineTS worker`);
  assert(testIndex > workerIndex, `${script.path} does not test after preparing release assets`);
  assert(buildIndex > testIndex, `${script.path} builds release binaries before tests pass`);
}

console.log("API release script tests passed");
