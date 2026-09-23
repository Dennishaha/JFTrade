import assert from "node:assert/strict";
import test from "node:test";

import { MIGRATION_MANIFEST_SCHEMA, validateMigrationManifest } from "./check-migration-manifest.mjs";

test("migration manifest binds supported components to the published baseline", () => {
  assert.equal(MIGRATION_MANIFEST_SCHEMA, "jftrade.migration-manifest.v1");
  assert.deepEqual(validateMigrationManifest(), []);
});

test("migration checker rejects release qualification without release receipts", async () => {
  const fs = await import("node:fs");
  const os = await import("node:os");
  const path = await import("node:path");
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-migration-manifest-"));
  try {
    fs.mkdirSync(path.join(root, "tests/fixtures/release"), { recursive: true });
    fs.copyFileSync("tests/fixtures/release/migration-manifest.json", path.join(root, "tests/fixtures/release/migration-manifest.json"));
    fs.copyFileSync("tests/fixtures/release/upgrade-baselines.json", path.join(root, "tests/fixtures/release/upgrade-baselines.json"));
    const file = path.join(root, "tests/fixtures/release/migration-manifest.json");
    const manifest = JSON.parse(fs.readFileSync(file, "utf8"));
    manifest.status = "release_qualified";
    fs.writeFileSync(file, JSON.stringify(manifest));
    const { validateMigrationManifest } = await import("./check-migration-manifest.mjs");
    const errors = validateMigrationManifest(root);
    assert.match(errors.join("\n"), /releaseVerification.status=passed|every component/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("migration checker rejects a baseline package digest drift", async () => {
  const fs = await import("node:fs");
  const os = await import("node:os");
  const path = await import("node:path");
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-migration-digest-"));
  try {
    fs.mkdirSync(path.join(root, "tests/fixtures/release"), { recursive: true });
    fs.copyFileSync("tests/fixtures/release/migration-manifest.json", path.join(root, "tests/fixtures/release/migration-manifest.json"));
    fs.copyFileSync("tests/fixtures/release/upgrade-baselines.json", path.join(root, "tests/fixtures/release/upgrade-baselines.json"));
    const file = path.join(root, "tests/fixtures/release/migration-manifest.json");
    const manifest = JSON.parse(fs.readFileSync(file, "utf8"));
    manifest.source.packageDigests[0].sha256 = "0".repeat(64);
    fs.writeFileSync(file, JSON.stringify(manifest));
    const errors = validateMigrationManifest(root);
    assert.match(errors.join("\n"), /does not match upgrade-baselines package/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});
