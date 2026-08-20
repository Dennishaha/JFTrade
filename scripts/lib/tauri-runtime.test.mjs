import assert from "node:assert/strict";
import path from "node:path";
import test from "node:test";

import { tauriDevelopmentEnvironment, tauriPreparation } from "./tauri-runtime.mjs";

test("tauri dev prepares the retained PineTS asset and exact runtime paths", () => {
  const root = path.resolve("fixture-repository");
  const environment = tauriDevelopmentEnvironment(root, { KEEP: "yes" }, "/runtime/node");

  assert.deepEqual(tauriPreparation("dev"), ["pnpm", ["run", "build:pineworker:dev"]]);
  assert.deepEqual(tauriPreparation("build"), ["pnpm", ["run", "prepare:tauri-release"]]);
  assert.equal(environment.KEEP, "yes");
  assert.equal(environment.JFTRADE_PINEWORKER_RUNTIME, "/runtime/node");
  assert.equal(environment.JFTRADE_PINEWORKER_BUNDLE, path.join(root, "var/pineworker/worker.mjs"));
  assert.equal(
    environment.JFTRADE_PINEWORKER_PROTO,
    path.join(root, "pkg/strategy/pineworker/proto/pineworker.proto"),
  );
});

test("tauri dev preserves explicit PineTS overrides", () => {
  const environment = tauriDevelopmentEnvironment(
    "/repo",
    {
      JFTRADE_PINEWORKER_BUNDLE: "/custom/worker.mjs",
      JFTRADE_PINEWORKER_RUNTIME: "/custom/node",
      JFTRADE_PINEWORKER_PROTO: "/custom/pineworker.proto",
    },
    "/default/node",
  );
  assert.equal(environment.JFTRADE_PINEWORKER_BUNDLE, "/custom/worker.mjs");
  assert.equal(environment.JFTRADE_PINEWORKER_RUNTIME, "/custom/node");
  assert.equal(environment.JFTRADE_PINEWORKER_PROTO, "/custom/pineworker.proto");
});
