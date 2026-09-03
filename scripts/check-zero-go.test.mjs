import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { inspectArtifact, validateSourceInventory } from "./check-zero-go.mjs";

test("source inventory rejects Go files, module files, commands, and Wails entrypoints", () => {
  const sources = new Map([
    ["package.json", '{"scripts":{"legacy":"go test ./..."}}'],
    [".github/workflows/legacy.yml", "uses: actions/setup-go@v6"],
    ["apps/wails/main.ts", "export {}"],
    ["scripts/legacy-generator.mjs", 'spawnChecked("go", ["run", "./cmd/generator"]);'],
    ["crates/api/src/auth.rs", 'const LEGACY_ORIGIN: &str = "wails://localhost";'],
  ]);
  const errors = validateSourceInventory(
    ["internal/legacy.go", "go.mod", ...sources.keys()],
    (file) => sources.get(file),
  );
  assert.match(errors.join("\n"), /tracked Go artifact: internal\/legacy\.go/);
  assert.match(errors.join("\n"), /tracked Go artifact: go\.mod/);
  assert.match(errors.join("\n"), /active Go\/Wails reference: package\.json/);
  assert.match(errors.join("\n"), /retired production entrypoint: apps\/wails\/main\.ts/);
  assert.match(errors.join("\n"), /active Go\/Wails reference: scripts\/legacy-generator\.mjs/);
  assert.match(errors.join("\n"), /active Go\/Wails reference: crates\/api\/src\/auth\.rs/);
});

test("historical docs and fixtures may preserve Go provenance", () => {
  const files = ["docs/history.md", "tests/fixtures/compatibility/storage/manifest.json"];
  const errors = validateSourceInventory(files, () => "go test ./historical");
  assert.deepEqual(errors, []);
});

test("artifact scan rejects Go build info, Wails metadata, and Go source", (context) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-zero-go-"));
  context.after(() => fs.rmSync(root, { force: true, recursive: true }));
  fs.writeFileSync(path.join(root, "legacy.go"), "package legacy\n");
  fs.writeFileSync(path.join(root, "component.spdx.json"), '{"packages":[{"purl":"pkg:golang/example/legacy"}]}');
  fs.writeFileSync(path.join(root, "legacy.bin"), Buffer.concat([
    Buffer.from("prefix"),
    Buffer.from([0xff, 0x20]),
    Buffer.from("Go buildinf:"),
  ]));
  const errors = inspectArtifact(root);
  assert.match(errors.join("\n"), /retired file: legacy\.go/);
  assert.match(errors.join("\n"), /metadata contains Go\/Wails component/);
  assert.match(errors.join("\n"), /contains a Go executable/);
});

test("artifact scan accepts Rust, Node, and Python runtime files", (context) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-zero-go-clean-"));
  context.after(() => fs.rmSync(root, { force: true, recursive: true }));
  fs.writeFileSync(path.join(root, "jftrade-desktop"), "rust executable fixture");
  fs.writeFileSync(path.join(root, "worker.mjs"), "export {};\n");
  fs.writeFileSync(path.join(root, "sbom.spdx.json"), '{"packages":[{"name":"jftrade"}]}');
  assert.deepEqual(inspectArtifact(root), []);
});
