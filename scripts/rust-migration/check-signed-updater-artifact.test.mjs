import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  inspectSignedTauriUpdaterArtifacts,
  inspectTauriUpdaterConfig,
  validateUpdaterConfiguration,
} from "./check-signed-updater-artifact.mjs";
import {
  inspectUpdaterInstallLifecycle,
  runUpdaterPreInstallHarness,
} from "./check-signed-updater-lifecycle.mjs";

const SIGNATURE = [
  "untrusted comment: minisign signature",
  "RURqRkFLRV9TSUdOQVRVUkU=",
  "trusted comment: timestamp: 2026-08-31T00:00:00Z",
  "RURqRkFLRV9UUlVTVEVEX1NJR05BVFVSRT0=",
].join("\n");

function fixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-signed-updater-"));
  const artifacts = path.join(root, "release");
  const configPath = path.join(root, "tauri.conf.json");
  fs.mkdirSync(artifacts, { recursive: true });
  fs.writeFileSync(configPath, JSON.stringify({
    plugins: { updater: { pubkey: "" } },
    bundle: { createUpdaterArtifacts: true },
  }));
  const archive = "JFTrade_1.2.3_aarch64.app.tar.gz";
  fs.writeFileSync(path.join(artifacts, archive), "signed archive bytes");
  fs.writeFileSync(path.join(artifacts, `${archive}.sig`), `${SIGNATURE}\n`);
  const feed = {
    version: "1.2.3",
    notes: "signed test fixture",
    pub_date: "2026-08-31T00:00:00Z",
    platforms: {
      "darwin-aarch64": {
        signature: SIGNATURE,
        url: `https://updates.example.test/releases/${archive}`,
      },
    },
  };
  const feedPath = path.join(root, "latest.json");
  fs.writeFileSync(feedPath, `${JSON.stringify(feed, null, 2)}\n`);
  return {
    root,
    artifacts,
    configPath,
    feed,
    feedPath,
    cleanup: () => fs.rmSync(root, { recursive: true, force: true }),
  };
}

test("verifies Tauri feed entries, archive sidecars and exact signature text", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const report = inspectSignedTauriUpdaterArtifacts({
    artifactRoot: value.artifacts,
    feedPath: value.feedPath,
    endpoint: "https://updates.example.test/latest.json",
    publicKey: "untrusted comment: minisign public key\nRWQTESTPUBLICKEY=",
    expectedVersion: "1.2.3",
    expectedTargets: ["darwin-aarch64"],
    configPath: value.configPath,
  });
  assert.equal(report.schemaVersion, "jftrade.tauri-signed-updater.v1");
  assert.deepEqual(report.feed.targets, ["darwin-aarch64"]);
  assert.equal(report.artifacts[0].archive, "JFTrade_1.2.3_aarch64.app.tar.gz");
  assert.equal(report.publicKeyConfigured, true);
  assert.match(report.limitations.join(" "), /Minisign cryptographic verification/);
});

test("fails closed for partial endpoint/public-key configuration and non-HTTPS values", () => {
  assert.throws(
    () => validateUpdaterConfiguration({ endpoint: "https://updates.example.test/latest.json" }),
    /must be configured together/,
  );
  assert.throws(
    () => validateUpdaterConfiguration({
      endpoint: "http://updates.example.test/latest.json",
      publicKey: "RWQTESTPUBLICKEY=",
    }),
    /HTTPS URL without credentials/,
  );
  assert.throws(
    () => validateUpdaterConfiguration({
      endpoint: "https://updates.example.test/latest.json",
      publicKey: "untrusted comment: minisign secret key",
    }),
    /private\/secret key/,
  );
});

test("rejects feed signature drift and archives not represented by the feed", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const drifted = structuredClone(value.feed);
  drifted.platforms["darwin-aarch64"].signature = `${SIGNATURE} drift`;
  assert.throws(
    () => inspectSignedTauriUpdaterArtifacts({
      artifactRoot: value.artifacts,
      feed: drifted,
      endpoint: "https://updates.example.test/latest.json",
      publicKey: "RWQTESTPUBLICKEY=",
      configPath: value.configPath,
    }),
    /signature does not match/,
  );
  fs.writeFileSync(path.join(value.artifacts, "JFTrade_1.2.3_x86_64.zip"), "second archive");
  fs.writeFileSync(path.join(value.artifacts, "JFTrade_1.2.3_x86_64.zip.sig"), SIGNATURE);
  assert.throws(
    () => inspectSignedTauriUpdaterArtifacts({
      artifactRoot: value.artifacts,
      feed: value.feed,
      endpoint: "https://updates.example.test/latest.json",
      publicKey: "RWQTESTPUBLICKEY=",
      configPath: value.configPath,
    }),
    /not represented in feed/,
  );
});

test("requires updater artifact generation in the Tauri config", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const config = JSON.parse(fs.readFileSync(value.configPath, "utf8"));
  config.bundle.createUpdaterArtifacts = false;
  fs.writeFileSync(value.configPath, JSON.stringify(config));
  assert.throws(() => inspectTauriUpdaterConfig(value.configPath), /createUpdaterArtifacts must be true/);
});

test("source lifecycle keeps process shutdown before updater install", () => {
  const report = inspectUpdaterInstallLifecycle();
  assert.equal(report.preInstallStopsProduct, true);
  assert.deepEqual(report.shutdownEvidence, ["http_join", "marketdata_helper", "pine_worker"]);
});

test("reproducible pre-install harness stops Rust API, Pine and Python stand-ins first", async () => {
  const report = await runUpdaterPreInstallHarness();
  assert.equal(report.allStoppedBeforeInstall, true);
  assert.deepEqual(
    report.events.map((event) => event.action),
    ["started", "started", "started", "stopped", "stopped", "stopped", "install"],
  );
  assert.deepEqual(report.roles, ["rust-api", "pine-worker", "python-helper"]);
});
