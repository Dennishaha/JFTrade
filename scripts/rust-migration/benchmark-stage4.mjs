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
const fixtureRoot = path.join(repositoryRoot, "tests/fixtures/rust-migration/stage4");
const fixturePath = path.join(fixtureRoot, "provider-lifecycle-corpus.json");
const expectedPath = path.join(fixtureRoot, "provider-lifecycle-corpus.expected.json");

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
  if (result.error?.code === "ETIMEDOUT") {
    throw new Error(`${command} timed out after ${timeout}ms`);
  }
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
  const result = timeArgs
    ? run("/usr/bin/time", timeArgs, options)
    : run(command, args, options);
  return {
    elapsedMillis: Number((performance.now() - startedAt).toFixed(3)),
    stdout: result.stdout,
    ...parseResources(result.stderr),
  };
}

export function summarizeStage4Samples(samples, warmups) {
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

export function evaluateStage4Performance(goResult, rustResult) {
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

function sampleGo(references) {
  const samples = references.map(({ binary, test }) =>
    timedRun(binary, ["-test.run", `^${test}$`, "-test.count=1"], {
      env: { JFTRADE_STAGE4_FIXTURE_ROOT: fixtureRoot },
    }));
  return {
    elapsedMillis: Number(samples.reduce((total, sample) => total + sample.elapsedMillis, 0).toFixed(3)),
    cpuMillis: samples.every((sample) => Number.isFinite(sample.cpuMillis))
      ? Number(samples.reduce((total, sample) => total + sample.cpuMillis, 0).toFixed(3))
      : null,
    peakRssBytes: samples.every((sample) => Number.isFinite(sample.peakRssBytes))
      ? Math.max(...samples.map((sample) => sample.peakRssBytes))
      : null,
  };
}

function sampleRust(binary, expected) {
  const sample = timedRun(binary, ["--input", fixturePath]);
  const actual = JSON.parse(sample.stdout);
  if (!isDeepStrictEqual(actual, expected)) {
    throw new Error("Rust stage 4 benchmark output drifted from the pinned expected JSON");
  }
  return sample;
}

function collect(sample, warmups, iterations) {
  for (let index = 0; index < warmups; index += 1) sample();
  return summarizeStage4Samples(
    Array.from({ length: iterations }, () => sample()),
    warmups,
  );
}

export function runStage4Benchmark({ warmups = 3, iterations = 20 } = {}) {
  const temporaryRoot = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-rust-stage4-benchmark-"));
  try {
    const references = [
      ["marketdata", "./internal/marketdata", "TestRustMigrationStage4DemandAndProviderLifecycleMatchesCorpus"],
      ["futu", "./internal/integration/futu", "TestRustMigrationStage4OpenDFrameAndSubscriptionPlanMatchesCorpus"],
      ["pineworker", "./pkg/strategy/pineworker", "TestRustMigrationStage4PineLifecycleMatchesCorpus"],
    ].map(([name, packageName, test]) => {
      const binary = path.join(temporaryRoot, `${name}.test`);
      run("go", ["test", "-c", "-trimpath", "-o", binary, packageName]);
      return { binary, test };
    });
    run("cargo", ["build", "--release", "-p", "jftrade-engine", "--bin", "jftrade-stage4-shadow"]);
    const rustBinary = path.join(repositoryRoot, "target/release/jftrade-stage4-shadow");
    const expected = JSON.parse(fs.readFileSync(expectedPath, "utf8"));
    const goResult = collect(() => sampleGo(references), warmups, iterations);
    const rustResult = collect(() => sampleRust(rustBinary, expected), warmups, iterations);
    const expectedSha256 = createHash("sha256")
      .update(fs.readFileSync(expectedPath))
      .digest("hex");
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
        go: "go test -c -trimpath (three production-owner behavior harnesses)",
        rust: "cargo build --release",
      },
      workload: {
        fixture: path.basename(fixturePath),
        fixtureSha256: createHash("sha256").update(fs.readFileSync(fixturePath)).digest("hex"),
        expectedSha256,
        marketDataOperations: expected.marketdata.length,
        pineLifecycleOperations: expected.pine.length,
        openDSubscriptions: expected.futu.plan.physical.length,
        healthProbes: expected.futu.probes.length,
      },
      go: {
        ...goResult,
        corporaPerSecondP50: Number((1000 / goResult.elapsedMillis.p50).toFixed(3)),
        binaryBytes: references.reduce((total, item) => total + fs.statSync(item.binary).size, 0),
        resultHash: `sha256:${expectedSha256}`,
      },
      rust: {
        ...rustResult,
        corporaPerSecondP50: Number((1000 / rustResult.elapsedMillis.p50).toFixed(3)),
        binaryBytes: fs.statSync(rustBinary).size,
        resultHash: `sha256:${expectedSha256}`,
      },
      gates: evaluateStage4Performance(goResult, rustResult),
      limitations: [
        "The Go reference executes three compiled production-owner package test harnesses while Rust uses one composition replay binary; absolute startup and binary-size comparisons are conservative and not product-process measurements.",
        "The fixture covers lifecycle state, cache invalidation, subscription planning, framing, and health mapping; it does not open live Yahoo, AKShare, OpenD, or Pine strategy traffic.",
        "Real helper/worker startup-to-ready and shutdown qualification remains a release-platform and explicit-live workflow gate.",
        "This local result does not replace native Linux, Windows, and macOS CI qualification.",
      ],
    };
  } finally {
    fs.rmSync(temporaryRoot, { force: true, recursive: true });
  }
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  const result = runStage4Benchmark();
  if (!result.gates.p95RegressionGatePassed || !result.gates.rssRegressionGatePassed) {
    console.error(JSON.stringify(result, null, 2));
    process.exitCode = 1;
  } else {
    console.log(JSON.stringify(result, null, 2));
  }
}
