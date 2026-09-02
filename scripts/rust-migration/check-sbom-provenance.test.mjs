import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  checkSbomProvenance,
  inspectSbomProvenance,
  REQUIRED_TARGETS,
} from "./check-sbom-provenance.mjs";

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

function fixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-sbom-provenance-"));
  const artifact = Buffer.from("JFTrade release artifact\n");
  const artifactSha256 = sha256(artifact);
  const sbom = JSON.stringify({
    spdxVersion: "SPDX-2.3",
    SPDXID: "SPDXRef-DOCUMENT",
    packages: [{ SPDXID: "SPDXRef-Package", name: "jftrade" }],
  });
  const provenance = JSON.stringify({
    _type: "https://in-toto.io/Statement/v1",
    subject: [{ name: "JFTrade", digest: { sha256: artifactSha256 } }],
    predicateType: "https://slsa.dev/provenance/v1",
  });
  fs.writeFileSync(path.join(root, "JFTrade.bin"), artifact);
  fs.writeFileSync(path.join(root, "JFTrade.spdx.json"), sbom);
  fs.writeFileSync(path.join(root, "JFTrade.provenance.json"), provenance);
  const target = {
    platform: "macos-arm64",
    artifact: "JFTrade.bin",
    artifactSha256,
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

test("rejects Go and Wails components recorded in an SBOM", () => {
  const { root, target } = fixture();
  const sbom = JSON.stringify({
    spdxVersion: "SPDX-2.3",
    packages: [{ name: "legacy", externalRefs: [{ referenceLocator: "pkg:golang/example/legacy" }] }],
  });
  fs.writeFileSync(path.join(root, target.sbom), sbom);
  target.sbomSha256 = sha256(sbom);
  const result = inspectSbomProvenance({ targets: [target] }, { baseDirectory: root });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /SBOM zero-Go check/);
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

test("rejects absolute, traversal, and symlinked evidence references", (context) => {
  const value = fixture();
  const outside = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-sbom-provenance-outside-"));
  context.after(() => {
    fs.rmSync(value.root, { recursive: true, force: true });
    fs.rmSync(outside, { recursive: true, force: true });
  });

  for (const name of [value.target.artifact, value.target.sbom, value.target.provenance]) {
    fs.copyFileSync(path.join(value.root, name), path.join(outside, name));
  }
  const escaped = structuredClone(value.target);
  escaped.artifact = `../${path.basename(outside)}/${value.target.artifact}`;
  escaped.sbom = `../${path.basename(outside)}/${value.target.sbom}`;
  escaped.provenance = `../${path.basename(outside)}/${value.target.provenance}`;
  let result = inspectSbomProvenance({ targets: [escaped] }, { baseDirectory: value.root });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /parent path segments/);

  const absolute = structuredClone(value.target);
  absolute.artifact = path.join(outside, value.target.artifact);
  result = inspectSbomProvenance({ targets: [absolute] }, { baseDirectory: value.root });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /relative POSIX path/);

  const driveRelative = structuredClone(value.target);
  driveRelative.artifact = "C:JFTrade.bin";
  result = inspectSbomProvenance({ targets: [driveRelative] }, { baseDirectory: value.root });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /relative POSIX path/);

  fs.symlinkSync(path.join(outside, value.target.artifact), path.join(value.root, "artifact-link"));
  const symlink = structuredClone(value.target);
  symlink.artifact = "artifact-link";
  result = inspectSbomProvenance({ targets: [symlink] }, { baseDirectory: value.root });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /symlink/);
});

test("rejects duplicate platforms and reused evidence files", () => {
  const value = fixture();
  const duplicatePlatform = inspectSbomProvenance(
    { targets: [value.target, structuredClone(value.target)] },
    { baseDirectory: value.root },
  );
  assert.equal(duplicatePlatform.valid, false);
  assert.match(duplicatePlatform.errors.join("\n"), /duplicate SBOM\/provenance target/);

  const reusedFiles = inspectSbomProvenance(
    { targets: [value.target, { ...value.target, platform: "linux-x64" }] },
    { baseDirectory: value.root },
  );
  assert.equal(reusedFiles.valid, false);
  assert.match(reusedFiles.errors.join("\n"), /duplicate SBOM\/provenance target file/);

  const mixedCollections = inspectSbomProvenance(
    { targets: [value.target], platforms: [{ ...value.target, platform: "linux-x64" }] },
    { baseDirectory: value.root },
  );
  assert.equal(mixedCollections.valid, false);
  assert.match(mixedCollections.errors.join("\n"), /only one target collection/);
});

test("validates SPDX and CycloneDX component records and checksum digests", () => {
  const value = fixture();
  const writeSbom = (document) => {
    const text = JSON.stringify(document);
    fs.writeFileSync(path.join(value.root, value.target.sbom), text);
    value.target.sbomSha256 = sha256(text);
  };

  writeSbom({
    bomFormat: "CycloneDX",
    specVersion: "1.5",
    components: [{ type: "library", name: "jftrade" }],
  });
  let result = inspectSbomProvenance({ targets: [value.target] }, { baseDirectory: value.root });
  assert.equal(result.valid, true, result.errors.join("; "));
  assert.equal(result.targets[0].sbom.format, "cyclonedx");

  writeSbom({ spdxVersion: "SPDX-2.3", packages: [null] });
  result = inspectSbomProvenance({ targets: [value.target] }, { baseDirectory: value.root });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /SPDX package 1 must include a name/);

  writeSbom({
    spdxVersion: "SPDX-2.3",
    packages: [{
      name: "jftrade",
      checksums: [{ algorithm: "SHA256", checksumValue: "not-a-digest" }],
    }],
  });
  result = inspectSbomProvenance({ targets: [value.target] }, { baseDirectory: value.root });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /invalid SHA-256 digest/);
});

test("requires provenance subjects to name and digest the exact artifact", () => {
  const value = fixture();
  const writeProvenance = (document) => {
    const text = JSON.stringify(document);
    fs.writeFileSync(path.join(value.root, value.target.provenance), text);
    value.target.provenanceSha256 = sha256(text);
  };

  writeProvenance({ subject: [{ name: "JFTrade", digest: { sha256: "a".repeat(64) } }] });
  let result = inspectSbomProvenance({ targets: [value.target] }, { baseDirectory: value.root });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /do not include the artifact SHA-256 digest/);

  writeProvenance({ subject: [{ name: "JFTrade", digest: { sha256: ["a".repeat(64)] } }] });
  result = inspectSbomProvenance({ targets: [value.target] }, { baseDirectory: value.root });
  assert.equal(result.valid, false);
  assert.match(result.errors.join("\n"), /has no SHA-256 digest/);
});

test("reports non-object manifests and target entries instead of throwing", () => {
  for (const manifest of [null, [], "not-json", 42, { targets: [null, 42, "entry"] }]) {
    assert.doesNotThrow(() => {
      const result = inspectSbomProvenance(manifest, { baseDirectory: os.tmpdir() });
      assert.equal(result.valid, false);
      assert.ok(result.errors.length > 0);
    });
  }
  assert.doesNotThrow(() => {
    const result = checkSbomProvenance(42);
    assert.equal(result.valid, false);
    assert.match(result.errors.join("\n"), /cannot read SBOM\/provenance manifest/);
  });
});
