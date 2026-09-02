import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  checkTauriReleaseRuntime,
  prepareTauriReleaseRuntime,
} from "./prepare-tauri-release-runtime.mjs";

function fixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-tauri-runtime-"));
  const write = (relativePath, contents) => {
    const filePath = path.join(root, relativePath);
    fs.mkdirSync(path.dirname(filePath), { recursive: true });
    fs.writeFileSync(filePath, contents);
    return filePath;
  };
  const nodeExecutable = write("node-source/node", "managed-node");
  const nodeLicense = write("node-source/LICENSE", "node-license");
  write("runtime-assets/pine/worker.mjs", "worker");
  for (const name of ["pineworker.proto", "pineworker_common.proto", "pineworker_types.proto"]) {
    write(`proto/pineworker/${name}`, name);
  }
  write(
    "runtime-assets/marketdata/marketdata-sidecar-darwin-arm64/marketdata-sidecar-darwin-arm64",
    "helper",
  );
  write(
    "runtime-assets/marketdata/marketdata-sidecar-darwin-arm64/_internal/runtime",
    "runtime",
  );
  return {
    cleanup: () => fs.rmSync(root, { recursive: true, force: true }),
    options: {
      architecture: "arm64",
      environment: {},
      nodeExecutable,
      nodeLicense,
      nodeVersion: "v24.1.0",
      platform: "darwin",
      repositoryRoot: root,
    },
    root,
  };
}

test("prepares an exact managed runtime and verifies all bundled resources", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const manifest = prepareTauriReleaseRuntime(value.options);

  assert.equal(manifest.schemaVersion, "jftrade.tauri-runtime.v1");
  assert.equal(manifest.nodeVersion, "v24.1.0");
  assert.deepEqual(checkTauriReleaseRuntime(value.options), manifest);
  assert.equal(fs.readFileSync(path.join(value.root, "var/tauri-runtime/node"), "utf8"), "managed-node");
  assert(manifest.files.some((entry) => entry.resource.endsWith("_internal/runtime")));
});

test("rejects a changed managed runtime instead of accepting a stale manifest", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  prepareTauriReleaseRuntime(value.options);
  fs.appendFileSync(path.join(value.root, "var/tauri-runtime/node"), "tampered");

  assert.throws(
    () => checkTauriReleaseRuntime(value.options),
    /manifest is stale/,
  );
});
