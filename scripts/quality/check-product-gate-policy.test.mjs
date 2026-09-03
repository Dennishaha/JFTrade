import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  activeProductGateFiles,
  validateProductGateVocabulary,
} from "./check-product-gate-policy.mjs";

test("current package scripts workflows module map and architecture docs use permanent product gates", () => {
  assert.deepEqual(validateProductGateVocabulary(), []);
});

test("rejects migration-stage gates in active files while excluding history and frozen fixtures", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-product-gate-policy-"));
  try {
    write(root, "package.json", '{"scripts":{"check:old":"node scripts/rust-migration/check-stage9.mjs"}}');
    write(root, "scripts/module-map.json", "{}");
    write(root, ".github/workflows/ci.yml", "name: migration differential\n");
    write(root, "docs/architecture/current.md", "Read closeout-evidence.json.\n");
    write(root, "docs/history/go-to-rust/record.md", "Stage 9 route-ownership.json\n");
    write(root, "tests/fixtures/compatibility/api-transport/source.json", '{"schema":"stage7.v1"}\n');

    assert.ok(activeProductGateFiles(root).includes(".github/workflows/ci.yml"));
    assert.equal(activeProductGateFiles(root).some((file) => file.startsWith("docs/history/")), false);
    assert.equal(activeProductGateFiles(root).some((file) => file.startsWith("tests/fixtures/")), false);
    const errors = validateProductGateVocabulary(root);
    assert.ok(errors.some((error) => error.startsWith("package.json:1")));
    assert.ok(errors.some((error) => error.startsWith(".github/workflows/ci.yml:1")));
    assert.ok(errors.some((error) => error.startsWith("docs/architecture/current.md:1")));
    assert.equal(errors.some((error) => error.includes("docs/history")), false);
    assert.equal(errors.some((error) => error.includes("tests/fixtures")), false);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

function write(root, relativePath, contents) {
  const absolutePath = path.join(root, relativePath);
  fs.mkdirSync(path.dirname(absolutePath), { recursive: true });
  fs.writeFileSync(absolutePath, contents);
}
