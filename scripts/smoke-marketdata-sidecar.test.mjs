import assert from "node:assert/strict";
import {
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, join } from "node:path";
import process from "node:process";
import test from "node:test";

import {
  resolveMarketDataSidecarExecutable,
  runMarketDataSidecarSmoke,
} from "./smoke-marketdata-sidecar.mjs";

test("resolves macOS, Linux, and Windows onedir executables", () => {
  const rootDir = join(tmpdir(), "jftrade smoke root");
  const cases = [
    ["darwin", "arm64", "marketdata-sidecar-darwin-arm64"],
    ["linux", "x64", "marketdata-sidecar-linux-amd64"],
    ["win32", "x64", "marketdata-sidecar-windows-amd64.exe"],
    ["win32", "arm64", "marketdata-sidecar-windows-arm64.exe"],
  ];

  for (const [platform, architecture, expected] of cases) {
    const executable = resolveMarketDataSidecarExecutable({
      rootDir,
      environment: {},
      platform,
      architecture,
    });
    assert.equal(basename(executable), expected);
    assert.match(executable, /internal[/\\]marketdataassets[/\\]assets[/\\]bin/);
  }
});

test("generic smoke executable override wins over the legacy alias", () => {
  const rootDir = join(tmpdir(), "jftrade smoke override");
  const executable = resolveMarketDataSidecarExecutable({
    rootDir,
    environment: {
      JFTRADE_MARKETDATA_SMOKE_EXECUTABLE: "generic/helper",
      JFTRADE_YFINANCE_SMOKE_EXECUTABLE: "legacy/helper",
    },
  });
  assert.match(executable, /generic[/\\]helper$/);
});

test("frozen smoke checks version and only provider health routes", async () => {
  const tempDir = mkdtempSync(join(tmpdir(), "jftrade-marketdata-smoke-"));
  try {
    const helper = createFakeHelper(tempDir);
    const requestLog = join(tempDir, "requests.log");
    const pidFile = join(tempDir, "helper.pid");
    const summary = join(tempDir, "summary.md");
    const logs = [];
    const result = await runMarketDataSidecarSmoke({
      executable: process.execPath,
      executableArgs: [helper],
      bundleDirectory: tempDir,
      timeoutMs: 5_000,
      environment: {
        ...process.env,
        FAKE_REQUEST_LOG: requestLog,
        FAKE_PID_FILE: pidFile,
        GITHUB_STEP_SUMMARY: summary,
      },
      log: (message) => logs.push(message),
    });

    assert.equal(result.version, "marketdata-sidecar 0.2.0");
    assert.equal(result.yfinanceVersion, "1.6.0");
    assert.equal(result.akshareVersion, "1.18.91");
    assert.ok(result.bundleBytes > 0);
    assert.match(logs.at(-1), /smoke passed/);
    const requests = readFileSync(requestLog, "utf8").trim().split("\n");
    assert.deepEqual(new Set(requests), new Set([
      "/healthz",
      "/providers/yfinance/health",
      "/providers/akshare/health",
    ]));
    assert.match(readFileSync(summary, "utf8"), /AKShare 1\.18\.91/);
    assertProcessStopped(pidFile);
  } finally {
    rmSync(tempDir, { recursive: true, force: true });
  }
});

test("frozen smoke fails closed when a provider import fails", async () => {
  const tempDir = mkdtempSync(join(tmpdir(), "jftrade-marketdata-failed-"));
  try {
    const helper = createFakeHelper(tempDir);
    const pidFile = join(tempDir, "helper.pid");
    await assert.rejects(
      runMarketDataSidecarSmoke({
        executable: process.execPath,
        executableArgs: [helper],
        bundleDirectory: tempDir,
        timeoutMs: 5_000,
        environment: {
          ...process.env,
          FAKE_FAIL_PROVIDER: "akshare",
          FAKE_PID_FILE: pidFile,
        },
        log: () => {},
      }),
      /\/providers\/akshare\/health returned AKSHARE_RUNTIME_FAILED: fake import failure/,
    );
    assertProcessStopped(pidFile);
  } finally {
    rmSync(tempDir, { recursive: true, force: true });
  }
});

test("frozen smoke rejects an unexpected version contract", async () => {
  const tempDir = mkdtempSync(join(tmpdir(), "jftrade-marketdata-version-"));
  try {
    const helper = createFakeHelper(tempDir);
    await assert.rejects(
      runMarketDataSidecarSmoke({
        executable: process.execPath,
        executableArgs: [helper],
        bundleDirectory: tempDir,
        timeoutMs: 5_000,
        environment: { ...process.env, FAKE_BAD_VERSION: "1" },
        log: () => {},
      }),
      /Unexpected market-data sidecar --version output/,
    );
  } finally {
    rmSync(tempDir, { recursive: true, force: true });
  }
});

function createFakeHelper(directory) {
  const helper = join(directory, "fake-marketdata-sidecar.mjs");
  writeFileSync(
    helper,
    `import { appendFileSync, writeFileSync } from "node:fs";
import { createServer } from "node:http";

if (process.argv.includes("--version")) {
  console.log(process.env.FAKE_BAD_VERSION ? "not-a-sidecar" : "marketdata-sidecar 0.2.0");
  process.exit(0);
}

const hostIndex = process.argv.indexOf("--host");
const portIndex = process.argv.indexOf("--port");
const host = process.argv[hostIndex + 1];
const port = Number(process.argv[portIndex + 1]);
const hits = new Map();
const versions = { yfinance: ["yfinance_version", "1.6.0"], akshare: ["provider_version", "1.18.91"] };
if (process.env.FAKE_PID_FILE) writeFileSync(process.env.FAKE_PID_FILE, String(process.pid));
const server = createServer((request, response) => {
  if (process.env.FAKE_REQUEST_LOG) appendFileSync(process.env.FAKE_REQUEST_LOG, request.url + "\\n");
  response.setHeader("content-type", "application/json");
  if (request.url === "/healthz") {
    response.end(JSON.stringify({ ok: true, version: "0.2.0" }));
    return;
  }
  const match = request.url.match(/^\\/providers\\/(yfinance|akshare)\\/health$/);
  if (!match) {
    response.statusCode = 500;
    response.end(JSON.stringify({ error: "unexpected data route" }));
    return;
  }
  const provider = match[1];
  if (process.env.FAKE_FAIL_PROVIDER === provider) {
    response.statusCode = 503;
    response.end(JSON.stringify({ error: { code: provider.toUpperCase() + "_RUNTIME_FAILED", message: "fake import failure" } }));
    return;
  }
  const count = (hits.get(provider) || 0) + 1;
  hits.set(provider, count);
  const [field, version] = versions[provider];
  if (count === 1) {
    response.statusCode = 503;
    response.end(JSON.stringify({ error: { code: provider.toUpperCase() + "_RUNTIME_WARMING", message: "fake import warming" } }));
    return;
  }
  response.end(JSON.stringify({ ok: true, runtime_state: "ready", [field]: version }));
});
server.listen(port, host);
const close = () => server.close(() => process.exit(0));
process.on("SIGTERM", close);
process.on("SIGINT", close);
`,
  );
  return helper;
}

function assertProcessStopped(pidFile) {
  const pid = Number(readFileSync(pidFile, "utf8"));
  assert.ok(Number.isSafeInteger(pid) && pid > 0);
  assert.throws(
    () => process.kill(pid, 0),
    (error) => error?.code === "ESRCH" || error?.code === "EINVAL",
    `helper process ${pid} remained alive after smoke cleanup`,
  );
}
