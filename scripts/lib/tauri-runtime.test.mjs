import assert from "node:assert/strict";
import path from "node:path";
import test from "node:test";

import {
  tauriCommandOptions,
  tauriDevelopmentEnvironment,
  tauriPreparation,
  tauriReleaseBuild,
} from "./tauri-runtime.mjs";

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
    path.join(root, "proto/pineworker/pineworker.proto"),
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

test("tauri release injects one validated version into Rust and bundle metadata", () => {
  const release = tauriReleaseBuild(
    {
      KEEP: "yes",
      JFTRADE_DESKTOP_RELEASE_TAG: "v1.2.3",
      JFTRADE_DESKTOP_COMMIT: "abc123",
      JFTRADE_DESKTOP_BUILD_TIME: "2026-08-24T00:00:00Z",
      JFTRADE_BUILD_VERSION: "stale",
      JFTRADE_DESKTOP_PUBLISH: "true",
    },
    new Date("2026-08-24T01:00:00Z"),
  );
  assert.equal(release.environment.KEEP, "yes");
  assert.equal(release.environment.JFTRADE_BUILD_VERSION, "1.2.3");
  assert.equal(release.environment.JFTRADE_BUILD_COMMIT, "abc123");
  assert.equal(
    release.environment.JFTRADE_BUILD_TIME,
    "2026-08-24T00:00:00.000Z",
  );
  assert.deepEqual(release.finalOptions, [
    "--config",
    JSON.stringify({ version: "1.2.3", bundle: { createUpdaterArtifacts: true } }),
  ]);
  assert.deepEqual(
    tauriCommandOptions(
      "build",
      ["--config", JSON.stringify({ version: "9.9.9" })],
      release,
    ),
    [
      "--config",
      JSON.stringify({ version: "9.9.9" }),
      "--config",
      JSON.stringify({ version: "1.2.3", bundle: { createUpdaterArtifacts: true } }),
    ],
  );
  assert.deepEqual(
    tauriCommandOptions("build", ["--", "--no-bundle"], release),
    [
      "--no-bundle",
      "--config",
      JSON.stringify({ version: "1.2.3", bundle: { createUpdaterArtifacts: true } }),
    ],
  );
});

test("tauri release disables updater artifact generation for non-publish builds", () => {
  const release = tauriReleaseBuild(
    { JFTRADE_DESKTOP_RELEASE_TAG: "v1.2.3", JFTRADE_DESKTOP_PUBLISH: "false" },
    new Date("2026-08-24T01:00:00Z"),
  );
  assert.deepEqual(release.finalOptions, [
    "--config",
    JSON.stringify({ version: "1.2.3", bundle: { createUpdaterArtifacts: false } }),
  ]);
});

test("tauri release forwards native signing configuration only when provisioned", () => {
  const release = tauriReleaseBuild({
    JFTRADE_DESKTOP_RELEASE_TAG: "v1.2.3",
    JFTRADE_DESKTOP_PUBLISH: "true",
    JFTRADE_MACOS_SIGN_IDENTITY: "Developer ID Application: JFTrade",
    JFTRADE_WINDOWS_CERTIFICATE_THUMBPRINT: "ABC123",
  });
  const config = JSON.parse(release.finalOptions[1]);
  assert.deepEqual(config.bundle.macOS, {
    signingIdentity: "Developer ID Application: JFTrade",
  });
  assert.deepEqual(config.bundle.windows, {
    certificateThumbprint: "ABC123",
    digestAlgorithm: "sha256",
    timestampUrl: "http://timestamp.digicert.com",
  });
});

test("tauri release rejects development and zero versions before preparation", () => {
  assert.throws(
    () => tauriReleaseBuild({ JFTRADE_DESKTOP_RELEASE_TAG: "main" }),
    /vX\.Y\.Z/,
  );
  assert.throws(
    () => tauriReleaseBuild({ JFTRADE_DESKTOP_RELEASE_TAG: "v0.0.0" }),
    /v0\.0\.0/,
  );
});
