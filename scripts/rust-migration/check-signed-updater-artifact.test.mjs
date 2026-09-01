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

const KEY_ID = Buffer.from("jftrade!", "utf8");
const PUBLIC_KEY_BODY = Buffer.concat([
  Buffer.from([0x45, 0x64]),
  KEY_ID,
  Buffer.alloc(32, 0x11),
]).toString("base64");
const PUBLIC_KEY = `untrusted comment: minisign public key\n${PUBLIC_KEY_BODY}`;
const SIGNATURE_BODY = Buffer.concat([
  Buffer.from([0x45, 0x64]),
  KEY_ID,
  Buffer.alloc(64, 0x22),
]).toString("base64");
const GLOBAL_SIGNATURE = Buffer.alloc(64, 0x33).toString("base64");
const SIGNATURE = [
  "untrusted comment: signature from minisign secret key",
  SIGNATURE_BODY,
  "trusted comment: timestamp: 2026-08-31T00:00:00Z",
  GLOBAL_SIGNATURE,
].join("\n");

function validFeedOptions(value, overrides = {}) {
  return {
    artifactRoot: value.artifacts,
    feed: value.feed,
    endpoint: "https://updates.example.test/latest.json",
    publicKey: PUBLIC_KEY,
    configPath: value.configPath,
    ...overrides,
  };
}

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
    ...validFeedOptions(value, { feedPath: value.feedPath }),
    expectedVersion: "1.2.3",
    expectedTargets: ["darwin-aarch64"],
  });
  assert.equal(report.schemaVersion, "jftrade.tauri-signed-updater.v1");
  assert.deepEqual(report.feed.targets, ["darwin-aarch64"]);
  assert.equal(report.artifacts[0].archive, "JFTrade_1.2.3_aarch64.app.tar.gz");
  assert.equal(report.artifacts[0].archiveBytes, Buffer.byteLength("signed archive bytes"));
  assert.equal(report.artifacts[0].signatureBytes, Buffer.byteLength(`${SIGNATURE}\n`));
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
      publicKey: PUBLIC_KEY,
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
  assert.throws(
    () => validateUpdaterConfiguration({
      endpoint: "https://updates.example.test/latest.json",
      publicKey: "not-a-minisign-key",
    }),
    /canonical base64|decode to 42 bytes/,
  );
});

test("rejects feed signature drift and archives not represented by the feed", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const drifted = structuredClone(value.feed);
  drifted.platforms["darwin-aarch64"].signature = SIGNATURE.replace(
    SIGNATURE_BODY,
    `${SIGNATURE_BODY.slice(0, 20)}A${SIGNATURE_BODY.slice(21)}`,
  );
  assert.throws(
    () => inspectSignedTauriUpdaterArtifacts(validFeedOptions(value, { feed: drifted })),
    /signature does not match/,
  );
  fs.writeFileSync(path.join(value.artifacts, "JFTrade_1.2.3_x86_64.zip"), "second archive");
  fs.writeFileSync(path.join(value.artifacts, "JFTrade_1.2.3_x86_64.zip.sig"), SIGNATURE);
  assert.throws(
    () => inspectSignedTauriUpdaterArtifacts(validFeedOptions(value)),
    /not represented in feed/,
  );
});

test("rejects a structurally valid signature from a different configured key", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const otherKey = Buffer.concat([
    Buffer.from([0x45, 0x64]),
    Buffer.from("otherkey", "utf8"),
    Buffer.alloc(32, 0x44),
  ]).toString("base64");
  assert.throws(
    () => inspectSignedTauriUpdaterArtifacts(validFeedOptions(value, { publicKey: otherKey })),
    /signature key does not match configured updater public key/,
  );
});

test("rejects unsafe URL paths, archive version drift and incomplete digest metadata", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const archive = "JFTrade_1.2.3_aarch64.app.tar.gz";
  for (const url of [
    `https://updates.example.test/releases/../${archive}`,
    `https://updates.example.test/releases/%2e%2e/${archive}`,
  ]) {
    const traversal = structuredClone(value.feed);
    traversal.platforms["darwin-aarch64"].url = url;
    assert.throws(
      () => inspectSignedTauriUpdaterArtifacts(validFeedOptions(value, { feed: traversal })),
      /path traversal segments/,
    );
  }

  const wrongVersion = structuredClone(value.feed);
  wrongVersion.platforms["darwin-aarch64"].url = wrongVersion.platforms["darwin-aarch64"].url
    .replace("1.2.3", "1.2.4");
  fs.writeFileSync(path.join(value.artifacts, "JFTrade_1.2.4_aarch64.app.tar.gz"), "wrong version archive");
  fs.writeFileSync(path.join(value.artifacts, "JFTrade_1.2.4_aarch64.app.tar.gz.sig"), `${SIGNATURE}\n`);
  assert.throws(
    () => inspectSignedTauriUpdaterArtifacts(validFeedOptions(value, { feed: wrongVersion })),
    /must contain feed version 1\.2\.3/,
  );

  const metadata = structuredClone(value.feed);
  metadata.platforms["darwin-aarch64"].sha256 = "0".repeat(64);
  metadata.platforms["darwin-aarch64"].size = Buffer.byteLength("signed archive bytes");
  assert.throws(
    () => inspectSignedTauriUpdaterArtifacts(validFeedOptions(value, { feed: metadata })),
    /sha256 does not match/,
  );
  delete metadata.platforms["darwin-aarch64"].sha256;
  assert.throws(
    () => inspectSignedTauriUpdaterArtifacts(validFeedOptions(value, { feed: metadata })),
    /must provide both sha256 and size/,
  );
});

test("rejects duplicate target identities and archive references", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const duplicateTarget = structuredClone(value.feed);
  duplicateTarget.platforms["darwin-arm64"] = structuredClone(
    duplicateTarget.platforms["darwin-aarch64"],
  );
  assert.throws(
    () => inspectSignedTauriUpdaterArtifacts(validFeedOptions(value, { feed: duplicateTarget })),
    /duplicate target identity/,
  );

  const duplicateArchive = structuredClone(value.feed);
  duplicateArchive.platforms["linux-x64"] = structuredClone(
    duplicateArchive.platforms["darwin-aarch64"],
  );
  assert.throws(
    () => inspectSignedTauriUpdaterArtifacts(validFeedOptions(value, { feed: duplicateArchive })),
    /duplicate updater archive reference/,
  );

  const entry = JSON.stringify(value.feed.platforms["darwin-aarch64"]);
  fs.writeFileSync(
    value.feedPath,
    `{"version":"1.2.3","platforms":{"darwin-aarch64":${entry},"darwin-aarch64":${entry}}}\n`,
  );
  assert.throws(
    () => inspectSignedTauriUpdaterArtifacts(validFeedOptions(value, { feed: undefined, feedPath: value.feedPath })),
    /duplicate JSON object key/,
  );
});

test("rejects symlinked artifact roots, archives and sidecars", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const linkedArchive = path.join(value.artifacts, "linked.zip");
  fs.symlinkSync(path.join(value.artifacts, "JFTrade_1.2.3_aarch64.app.tar.gz"), linkedArchive);
  assert.throws(
    () => inspectSignedTauriUpdaterArtifacts(validFeedOptions(value)),
    /symbolic link/,
  );
  fs.unlinkSync(linkedArchive);

  const linkedRoot = path.join(value.root, "release-link");
  fs.symlinkSync(value.artifacts, linkedRoot, "dir");
  assert.throws(
    () => inspectSignedTauriUpdaterArtifacts(validFeedOptions(value, { artifactRoot: linkedRoot })),
    /directory must not be a symbolic link/,
  );
  fs.unlinkSync(linkedRoot);

  const sidecarPath = path.join(value.artifacts, "JFTrade_1.2.3_aarch64.app.tar.gz.sig");
  const sidecarBackup = path.join(value.root, "sidecar-backup");
  fs.renameSync(sidecarPath, sidecarBackup);
  fs.symlinkSync(sidecarBackup, sidecarPath);
  assert.throws(
    () => inspectSignedTauriUpdaterArtifacts(validFeedOptions(value)),
    /symbolic link/,
  );
});

test("rejects empty and non-Minisign sidecar signatures", (context) => {
  const value = fixture();
  context.after(value.cleanup);
  const archivePath = path.join(value.artifacts, "JFTrade_1.2.3_aarch64.app.tar.gz");
  fs.writeFileSync(archivePath, "");
  assert.throws(
    () => inspectSignedTauriUpdaterArtifacts(validFeedOptions(value)),
    /archive .*must not be empty/,
  );
  fs.writeFileSync(archivePath, "signed archive bytes");
  const sidecarPath = path.join(value.artifacts, "JFTrade_1.2.3_aarch64.app.tar.gz.sig");
  fs.writeFileSync(sidecarPath, "\n");
  assert.throws(
    () => inspectSignedTauriUpdaterArtifacts(validFeedOptions(value)),
    /non-empty text|four-line Minisign/,
  );
  fs.writeFileSync(sidecarPath, "not-a-signature");
  assert.throws(
    () => inspectSignedTauriUpdaterArtifacts(validFeedOptions(value)),
    /four-line Minisign signature/,
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
