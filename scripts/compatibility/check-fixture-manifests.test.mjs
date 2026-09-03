import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { validateCompatibilityManifests } from "./check-fixture-manifests.mjs";

test("accepts product capability manifests and rejects digest drift", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-compatibility-manifest-"));
  try {
    const directory = path.join(root, "tests/fixtures/compatibility/storage");
    fs.mkdirSync(directory, { recursive: true });
    const bytes = Buffer.from("frozen fixture\n");
    fs.writeFileSync(path.join(directory, "fixture.json"), bytes);
    fs.writeFileSync(path.join(directory, "manifest.json"), JSON.stringify({
      schemaVersion: "jftrade.compatibility-manifest.v1",
      version: 1,
      capability: "storage",
      sourceRelease: "v0.27.0",
      files: [{ path: "fixture.json", sha256: createHash("sha256").update(bytes).digest("hex") }],
    }));
    assert.deepEqual(validateCompatibilityManifests(root), []);
    fs.appendFileSync(path.join(directory, "fixture.json"), "drift");
    assert.match(validateCompatibilityManifests(root).join("\n"), /SHA-256 mismatch/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});
