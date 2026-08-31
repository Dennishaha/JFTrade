import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";

const workflow = fs.readFileSync(".github/workflows/desktop-release.yml", "utf8");
const postReleaseWorkflow = fs.readFileSync(
  ".github/workflows/desktop-post-release-closeout.yml",
  "utf8",
);

test("desktop publish lane is gated by closeout and signing prerequisites", () => {
  assert.match(workflow, /Verify Stage 9 static release-candidate admission/);
  assert.match(workflow, /check-stage9-closeout\.mjs --candidate-static/);
  assert.match(workflow, /check-release-candidate\.mjs/);
  assert.match(workflow, /candidate_evidence_config/);
  assert.match(workflow, /TAURI_SIGNING_PRIVATE_KEY:/);
  assert.match(workflow, /JFTRADE_TAURI_UPDATER_PUBKEY:/);
  assert.match(workflow, /JFTRADE_TAURI_UPDATER_ENDPOINT:/);
  assert.match(workflow, /check-signed-updater-artifact\.mjs --config-only/);
  assert.match(workflow, /check-signed-updater-lifecycle\.mjs/);
  assert.match(workflow, /JFTRADE_DESKTOP_PUBLISH == 'true'/);
  assert.match(workflow, /name: desktop-release-updater-macos/);
  assert.match(workflow, /name: desktop-release-updater-linux/);
  assert.match(workflow, /name: desktop-release-updater-windows-arm64/);
  assert.match(workflow, /name: desktop-release-updater-windows/);
  assert.match(workflow, /-name '\*\.sig'/);
});

test("desktop workflow separates candidate admission from post-release full closeout", () => {
  const publishIndex = workflow.indexOf("\n  publish:");
  assert.ok(publishIndex > 0);
  const prePublish = workflow.slice(0, publishIndex);
  assert.doesNotMatch(prePublish, /check-stage9-closeout\.mjs --check/);
  assert.match(prePublish, /check-stage9-closeout\.mjs --candidate-static/);
  const sumsIndex = workflow.indexOf("Generate SHA256SUMS");
  const candidateIndex = workflow.indexOf("Verify artifact-bound Stage 9 release candidate evidence");
  assert.ok(sumsIndex > publishIndex);
  assert.ok(candidateIndex > sumsIndex);
  assert.ok(candidateIndex < workflow.indexOf("Upload assets and publish GitHub release"));
  assert.doesNotMatch(workflow, /post-release-closeout:/);
});

test("post-release closeout is an independent evidence-ref workflow", () => {
  assert.match(postReleaseWorkflow, /workflow_dispatch:/);
  assert.match(postReleaseWorkflow, /evidence_ref:/);
  assert.match(postReleaseWorkflow, /release_tag:/);
  assert.match(postReleaseWorkflow, /post_release_reports:/);
  assert.match(postReleaseWorkflow, /actions\/checkout@v7/);
  assert.match(postReleaseWorkflow, /ref: \$\{\{ inputs\.evidence_ref \}\}/);
  assert.match(postReleaseWorkflow, /check-post-release-smoke\.mjs/);
  assert.match(postReleaseWorkflow, /check-stage9-closeout\.mjs --check/);
  assert.doesNotMatch(postReleaseWorkflow, /needs:\s*publish/);
  assert.doesNotMatch(postReleaseWorkflow, /needs\.publish/);
});

test("desktop publish lane cannot silently continue with unsigned platform credentials", () => {
  assert.match(workflow, /Publishing requires complete macOS signing and notarization credentials/);
  assert.match(workflow, /Publishing requires complete Windows signing credentials/);
  assert.doesNotMatch(workflow, /producing an unsigned (macOS|Windows) release/);
});

test("desktop release builds and inspects Tauri bundles instead of legacy Wails outputs", () => {
  assert.doesNotMatch(workflow, /go tool wails3|bin\/JFTrade/);
  for (const platform of ["macos-arm64", "windows-x64", "windows-arm64", "linux-x64"]) {
    assert.match(workflow, new RegExp(`tauri-release-${platform}\\.json`));
    assert.match(workflow, new RegExp(`tauri-runtime-smoke-${platform}\\.json`));
  }
  assert.match(workflow, /target\/release\/bundle\/dmg/);
  assert.match(workflow, /target\/release\/bundle\/nsis/);
  assert.match(workflow, /target\/release\/bundle/);
  assert.match(workflow, /desktop-release-inputs\.json/);
  assert.match(workflow, /Import-PfxCertificate/);
  assert.match(workflow, /signtool verify \/pa/);
  assert.match(workflow, /xvfb-run -a pnpm run smoke:tauri-release/);
  assert.match(workflow, /steps\.linux_package_artifacts\.outputs\.appimage/);
  assert.doesNotMatch(workflow, /steps\.linux_artifacts\.outputs\.(appimage|deb|rpm)/);
});
