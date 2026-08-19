#!/usr/bin/env node
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { performance } from "node:perf_hooks";
import { isDeepStrictEqual } from "node:util";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const fixtureRoot = path.join(repositoryRoot, "tests/fixtures/rust-migration/stage6");
const fixturePath = path.join(fixtureRoot, "assistant-rig-corpus.json");
const expectedPath = path.join(fixtureRoot, "assistant-rig-corpus.expected.json");

function run(command, args, options = {}) {
  const timeout = options.timeoutMs ?? 300_000;
  const result = spawnSync(command, args, {
    cwd: repositoryRoot,
    encoding: "utf8",
    env: { ...process.env, ...options.env },
    stdio: ["ignore", "pipe", "pipe"],
    maxBuffer: 8 * 1024 * 1024,
    timeout,
    killSignal: "SIGTERM",
  });
  if (result.error?.code === "ETIMEDOUT") throw new Error(`${command} timed out after ${timeout}ms`);
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(" ")} failed:\n${result.stdout}${result.stderr}`);
  }
  return result;
}

function percentile(values, ratio) {
  const sorted = [...values].sort((left, right) => left - right);
  return Number(sorted[Math.max(0, Math.ceil(ratio * sorted.length) - 1)].toFixed(3));
}

function parseResources(stderr) {
  if (process.platform === "darwin") {
    const rss = stderr.match(/^\s*(\d+)\s+maximum resident set size$/m);
    const times = stderr.match(/([\d.]+)\s+real\s+([\d.]+)\s+user\s+([\d.]+)\s+sys/);
    return {
      peakRssBytes: rss ? Number(rss[1]) : null,
      cpuMillis: times ? Number(((Number(times[2]) + Number(times[3])) * 1000).toFixed(3)) : null,
    };
  }
  if (process.platform === "linux") {
    const rss = stderr.match(/Maximum resident set size \(kbytes\):\s*(\d+)/);
    const user = stderr.match(/User time \(seconds\):\s*([\d.]+)/);
    const system = stderr.match(/System time \(seconds\):\s*([\d.]+)/);
    return {
      peakRssBytes: rss ? Number(rss[1]) * 1024 : null,
      cpuMillis: user && system
        ? Number(((Number(user[1]) + Number(system[1])) * 1000).toFixed(3))
        : null,
    };
  }
  return { peakRssBytes: null, cpuMillis: null };
}

function timedRun(command, args, options = {}) {
  const timeArgs = process.platform === "darwin"
    ? ["-l", command, ...args]
    : process.platform === "linux"
      ? ["-v", command, ...args]
      : null;
  const startedAt = performance.now();
  const result = timeArgs ? run("/usr/bin/time", timeArgs, options) : run(command, args, options);
  return {
    elapsedMillis: Number((performance.now() - startedAt).toFixed(3)),
    stdout: result.stdout,
    ...parseResources(result.stderr),
  };
}

export function summarizeStage6Samples(samples, warmups) {
  const elapsed = samples.map((sample) => sample.elapsedMillis);
  const rss = samples.map((sample) => sample.peakRssBytes).filter(Number.isFinite);
  const cpu = samples.map((sample) => sample.cpuMillis).filter(Number.isFinite);
  return {
    iterations: samples.length,
    warmups,
    elapsedMillis: {
      p50: percentile(elapsed, 0.5),
      p95: percentile(elapsed, 0.95),
      p99: percentile(elapsed, 0.99),
    },
    cpuMillisP50: cpu.length > 0 ? percentile(cpu, 0.5) : null,
    peakRssBytes: rss.length > 0 ? Math.max(...rss) : null,
  };
}

export function evaluateStage6Performance(goResult, rustResult) {
  const p95Ratio = rustResult.elapsedMillis.p95 / goResult.elapsedMillis.p95;
  const rssRatio = goResult.peakRssBytes && rustResult.peakRssBytes
    ? rustResult.peakRssBytes / goResult.peakRssBytes
    : null;
  return {
    rustToGoP95: Number(p95Ratio.toFixed(3)),
    rustToGoPeakRss: rssRatio === null ? null : Number(rssRatio.toFixed(3)),
    p95RegressionGatePassed: p95Ratio <= 1.05,
    rssRegressionGatePassed: rssRatio === null || rssRatio <= 1.10,
  };
}

function collect(sample, warmups, iterations) {
  for (let index = 0; index < warmups; index += 1) sample();
  return summarizeStage6Samples(Array.from({ length: iterations }, () => sample()), warmups);
}

export function runStage6Benchmark({ warmups = 3, iterations = 20 } = {}) {
  const temporaryRoot = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-rust-stage6-benchmark-"));
  try {
    const goBinary = path.join(temporaryRoot, "assistant.test");
    run("go", ["test", "-c", "-trimpath", "-o", goBinary, "./internal/assistant/engine"]);
    run("cargo", ["build", "--release", "-p", "jftrade-engine", "--bin", "jftrade-stage6-shadow"]);
    const rustBinary = path.join(repositoryRoot, "target/release/jftrade-stage6-shadow");
    const expected = JSON.parse(fs.readFileSync(expectedPath, "utf8"));
    const sampleGo = () => timedRun(goBinary, [
      "-test.run",
      "^TestRustMigrationStage6AssistantContractMatchesCorpus$",
      "-test.count=1",
    ], { env: { JFTRADE_STAGE6_FIXTURE_ROOT: fixtureRoot } });
    const sampleRust = () => {
      const sample = timedRun(rustBinary, ["--input", fixturePath]);
      if (!isDeepStrictEqual(JSON.parse(sample.stdout), expected)) {
        throw new Error("Rust Stage 6 benchmark output drifted from pinned expected JSON");
      }
      return sample;
    };
    const goResult = collect(sampleGo, warmups, iterations);
    const rustResult = collect(sampleRust, warmups, iterations);
    const expectedSha256 = createHash("sha256").update(fs.readFileSync(expectedPath)).digest("hex");
    return {
      schemaVersion: 1,
      measuredAt: new Date().toISOString(),
      machine: {
        platform: process.platform,
        arch: process.arch,
        cpu: os.cpus()[0]?.model ?? "unknown",
        logicalCpus: os.cpus().length,
        totalMemoryBytes: os.totalmem(),
      },
      buildProfile: {
        go: "go test -c -trimpath (production Assistant store and domain contract harness)",
        rust: "cargo build --release",
      },
      workload: {
        fixture: path.basename(fixturePath),
        fixtureSha256: createHash("sha256").update(fs.readFileSync(fixturePath)).digest("hex"),
        expectedSha256,
        statuses: expected.statuses.length,
        transitions: expected.transitions.length,
        rejectedInputPrompts: expected.invalidInputs.length,
        durableClaims: expected.claims.invocations.length,
        workflowTasks: expected.workflow.tasks.length,
        artifactVersions: expected.artifacts.versions,
        streamDeltas: expected.provider.deltas.length,
      },
      go: {
        ...goResult,
        corporaPerSecondP50: Number((1000 / goResult.elapsedMillis.p50).toFixed(3)),
        binaryBytes: fs.statSync(goBinary).size,
        resultHash: `sha256:${expectedSha256}`,
      },
      rust: {
        ...rustResult,
        corporaPerSecondP50: Number((1000 / rustResult.elapsedMillis.p50).toFixed(3)),
        binaryBytes: fs.statSync(rustBinary).size,
        resultHash: `sha256:${expectedSha256}`,
      },
      gates: evaluateStage6Performance(goResult, rustResult),
      limitations: [
        "The Go harness opens temporary production SQLite stores while the Rust shadow replays an in-memory checkpoint; absolute startup and binary-size values are not product-process evidence.",
        "The fake provider transcript makes no network request and does not qualify any live OpenAI-compatible provider, credential, rate-limit, or streaming transport behavior.",
        "Rig is exercised only through provider-neutral request/response adapter tests; the fixed transcript does not call a concrete Rig provider, and JFTrade remains on the Go Assistant production owner and Google ADK runtime.",
        "This local result does not replace native Linux, Windows, and macOS CI qualification.",
      ],
    };
  } finally {
    fs.rmSync(temporaryRoot, { force: true, recursive: true });
  }
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  const result = runStage6Benchmark();
  if (!result.gates.p95RegressionGatePassed || !result.gates.rssRegressionGatePassed) {
    console.error(JSON.stringify(result, null, 2));
    process.exitCode = 1;
  } else {
    console.log(JSON.stringify(result, null, 2));
  }
}
