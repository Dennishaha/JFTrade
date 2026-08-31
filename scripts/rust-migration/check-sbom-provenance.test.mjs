import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  inspectSbomProvenance,
  REQUIRED_TARGETS,
} from "./check-sbom-provenance.mjs";

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

function fixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-sbom-provenance-"));
  const artifact = Buffer.from("JFTrade release artifact\n");
  const sbom = JSON.stringify({
    spdxVersion: "SPDX-2.3",
    SPDXID: "SPDXRef-DOCUMENT",
    packages: [{ SPDXID: "SPDXRef-Package", name: "jftrade" }],
  });
  const provenance = JSON.stringify({
    _type: "https://in-toto.io/Statement/v1",
    subject: [{ name: "JFTrade", digest: { sha256: "a".repeat(64) } }],
    predicateType: "https://slsa.dev/provenance/v1",
  });
  fs.writeFileSync(path.join(root, "JFTrade.bin"), artifact);
  fs.writeFileSync(path.join(root, "JFTrade.spdx.json"), sbom);
  fs.writeFileSync(path.join(root, "JFTrade.provenance.json"), provenance);
  const target = {
    platform: "macos-arm64",
    artifact: "JFTrade.bin",
    artifactSha256: sha256(artifact),
    sbom: "JFTrade.spdx.json",
    sbomSha256: sha256(sbom),
    provenance: "JFTrade.provenance.json",
    provenanceSha256: sha256(provenance),
  };
  return { root, target };
}

test("verifies artifact, SPDX components, and provenance subject/digest", () => {
  const { root, target } = fixture();
  const result = inspectSbomProvenance({ targets: [target] }, { baseDirectory: root });
  assert.equal(result.valid, true);
  assert.equal(result.status, "partial_inputs_verified");
  assert.deepEqual(result.missingTargets, REQUIRED_TARGETS.slice(1));
  assert.equal(result.targets[0].sbom.format, "spdx");
  assert.equal(result.targets[0].sbom.componentCount, 1);
  assert.equal(result.targets[0].provenance.digestCount, 1);
  assert.equal(result.releaseQualified, false);
  assert.match(result.externalRequirements.join("\n"), /Anchore/);
});

test("rejects an artifact whose SHA-256 no longer matches the manifest", () => {
  const { root, target } = fixture();
  fs.appendFileSync(path.join(root, target.artifact), "tampered");
  const result = inspectSbomProvenance({ targets: [target] }, { baseDirectory: root });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /artifact SHA-256 mismatch/);
});

test("rejects an empty SBOM even when its empty-file hash is supplied", () => {
  const { root, target } = fixture();
  fs.writeFileSync(path.join(root, target.sbom), "");
  target.sbomSha256 = sha256("");
  const result = inspectSbomProvenance({ targets: [target] }, { baseDirectory: root });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /SBOM file is empty/);
});

test("reports missing platform targets without failing unless required", () => {
  const { root, target } = fixture();
  const manifest = { targets: [target] };
  const partial = inspectSbomProvenance(manifest, { baseDirectory: root });
  assert.equal(partial.valid, true);
  assert.equal(partial.warnings.length, 1);
  const required = inspectSbomProvenance(manifest, { baseDirectory: root, requireTargets: true });
  assert.equal(required.valid, false);
  assert.match(required.errors.join("\n"), /missing SBOM\/provenance targets/);
});

test("rejects provenance without a subject SHA-256 digest", () => {
  const { root, target } = fixture();
  const bad = JSON.stringify({ subject: [{ name: "JFTrade", digest: {} }] });
  fs.writeFileSync(path.join(root, target.provenance), bad);
  target.provenanceSha256 = sha256(bad);
  const result = inspectSbomProvenance({ targets: [target] }, { baseDirectory: root });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /has no SHA-256 digest/);
});
