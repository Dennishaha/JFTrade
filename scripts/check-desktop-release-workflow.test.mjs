import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";

const workflow = fs.readFileSync(".github/workflows/desktop-release.yml", "utf8");
const postReleaseWorkflow = fs.readFileSync(
  ".github/workflows/desktop-post-release-closeout.yml",
  "utf8",
);
const qualificationWorkflow = fs.readFileSync(
  ".github/workflows/desktop-release-qualification.yml",
  "utf8",
);

test("desktop publish lane is gated by closeout and signing prerequisites", () => {
  assert.match(workflow, /Verify Stage 9 static release-candidate admission/);
  assert.match(workflow, /check-stage9-closeout\.mjs --candidate-static/);
  assert.match(workflow, /check-release-candidate\.mjs/);
  assert.match(workflow, /candidate_evidence_config/);
  assert.match(workflow, /gh api[\s\S]*actions\/workflows\/\$CANDIDATE_WORKFLOW\/runs/);
  assert.match(workflow, /run_path/);
  assert.match(workflow, /run_status/);
  assert.match(workflow, /run_conclusion/);
  assert.match(workflow, /run_sha/);
  assert.match(workflow, /run_branch/);
  assert.match(workflow, /run_attempt/);
  assert.match(workflow, /actions\/download-artifact@v8[\s\S]*run-id: \$\{\{ needs\.release-inputs\.outputs\.candidate_run_id \}\}/);
  assert.match(workflow, /github-token: \$\{\{ github\.token \}\}/);
  assert.match(workflow, /repository: \$\{\{ github\.repository \}\}/);
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
  for (const input of [
    "release_ref",
    "commit_sha",
    "release_run_id",
    "release_run_attempt",
    "release_workflow",
    "artifact_digests",
  ]) {
    assert.match(postReleaseWorkflow, new RegExp(`${input}:`));
  }
  assert.match(postReleaseWorkflow, /--expected-artifact-digests/);
  assert.match(postReleaseWorkflow, /--expected-run-id/);
  assert.match(postReleaseWorkflow, /--expected-attempt/);
  assert.match(postReleaseWorkflow, /--expected-workflow/);
  assert.match(postReleaseWorkflow, /EVIDENCE_REF.*RELEASE_TAG/);
  assert.match(postReleaseWorkflow, /check-stage9-closeout\.mjs --check/);
  assert.doesNotMatch(postReleaseWorkflow, /needs:\s*publish/);
  assert.doesNotMatch(postReleaseWorkflow, /needs\.publish/);
});

test("qualification workflow produces an artifact-bound candidate config from one verified run", () => {
  assert.match(qualificationWorkflow, /workflow_dispatch:/);
  assert.match(qualificationWorkflow, /release_ref:/);
  assert.match(qualificationWorkflow, /source_run_id:/);
  assert.match(qualificationWorkflow, /actions\/download-artifact@v8/);
  assert.match(qualificationWorkflow, /run-id:/);
  assert.match(qualificationWorkflow, /github-token:/);
  assert.match(qualificationWorkflow, /repository:/);
  assert.match(qualificationWorkflow, /check-stage9-closeout\.mjs --candidate-static/);
  assert.match(qualificationWorkflow, /check-release-candidate\.mjs/);
  assert.match(qualificationWorkflow, /candidate-evidence-config\.json/);
  assert.match(qualificationWorkflow, /upload-artifact@v7/);
  assert.match(qualificationWorkflow, /head_sha/);
  assert.match(qualificationWorkflow, /conclusion.*success/);
  assert.match(qualificationWorkflow, /evidence_run_id:/);
  assert.match(qualificationWorkflow, /evidence_run_attempt:/);
  assert.match(qualificationWorkflow, /evidence_workflow:/);
  assert.match(qualificationWorkflow, /evidence_artifact:/);
  assert.match(qualificationWorkflow, /evidence_manifest:/);
  assert.match(qualificationWorkflow, /evidence_workflow is not a trusted producer workflow/);
  assert.match(qualificationWorkflow, /EVIDENCE_WORKFLOW.*desktop-release-evidence.yml/);
  assert.match(qualificationWorkflow, /EVIDENCE_ARTIFACT.*simple artifact name/);
  assert.match(qualificationWorkflow, /artifact_matches/);
  assert.match(qualificationWorkflow, /EVIDENCE_RUN_ATTEMPT/);
  assert.match(qualificationWorkflow, /workflow_run.id/);
  assert.match(qualificationWorkflow, /digest.*sha256/);
  assert.match(qualificationWorkflow, /Download external release evidence artifact/);
  assert.match(qualificationWorkflow, /release-evidence-inputs\.schema\.json/);
  assert.match(qualificationWorkflow, /validateExternalEvidenceManifest/);
  assert.match(qualificationWorkflow, /must not traverse a symlink/);
  assert.match(qualificationWorkflow, /target path escapes candidate evidence directory/);
  assert.doesNotMatch(qualificationWorkflow, /cat\s+>.*external_release_runner_evidence_required/);
  assert.doesNotMatch(qualificationWorkflow, /check-signed-updater-lifecycle\.mjs\s*>.*rollback/);
  assert.match(qualificationWorkflow, /sourceWorkflowRun/);
  assert.match(qualificationWorkflow, /external evidence manifest missing valid/);
  assert.match(qualificationWorkflow, /candidate-inputs\/\*\*/);
  assert.match(qualificationWorkflow, /source-artifact-metadata\.json/);
});

test("publish verifies the downloaded canonical candidate bundle without rebuilding it", () => {
  const publishSection = workflow.slice(workflow.indexOf("\n  publish:"));
  assert.match(publishSection, /candidate-inputs/);
  assert.match(publishSection, /candidate-output\/release-candidate-evidence\.json/);
  assert.match(publishSection, /check-release-candidate-bundle\.mjs/);
  assert.match(publishSection, /cp "\$canonical_path" artifacts\/release-candidate-evidence\.json/);
  assert.doesNotMatch(
    publishSection,
    /check-release-candidate\.mjs[\s\S]{0,600}--build/,
  );
  assert.match(publishSection, /canonical candidate evidence/);
  assert.match(publishSection, /duplicate release basename/);
  assert.match(publishSection, /find incoming -type f -print0/);
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
