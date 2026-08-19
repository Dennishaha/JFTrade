#!/usr/bin/env node
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { performance } from "node:perf_hooks";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const fixturePath = path.join(
  repositoryRoot,
  "tests/fixtures/rust-migration/stage3/backtest-corpus.json",
);

function run(command, args, options = {}) {
  const timeout = options.timeoutMs ?? 300_000;
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? repositoryRoot,
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
    throw new Error(command + " " + args.join(" ") + " failed:\n" + (result.stderr || result.stdout));
  }
  return result;
}

function percentile(values, percentileValue) {
  const sorted = [...values].sort((left, right) => left - right);
  const index = Math.max(0, Math.ceil(percentileValue * sorted.length) - 1);
  return Number(sorted[index].toFixed(3));
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

function timedRun(command, args, options) {
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

function benchmark(command, args, options, warmups, iterations, parseHash) {
  for (let index = 0; index < warmups; index += 1) run(command, args, options);
  const samples = [];
  let resultHash = "";
  for (let index = 0; index < iterations; index += 1) {
    const sample = timedRun(command, args, options);
    resultHash = parseHash(sample.stdout);
    samples.push(sample);
  }
  return { ...summarizeBacktestSamples(samples, warmups), resultHash };
}

export function summarizeBacktestSamples(samples, warmups) {
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

export function evaluateBacktestPerformance(goResult, rustResult) {
  const p95Ratio = rustResult.elapsedMillis.p95 / goResult.elapsedMillis.p95;
  const rssRatio = goResult.peakRssBytes && rustResult.peakRssBytes
    ? rustResult.peakRssBytes / goResult.peakRssBytes
    : null;
  return {
    rustToGoP95: Number(p95Ratio.toFixed(3)),
    rustToGoPeakRss: rssRatio === null ? null : Number(rssRatio.toFixed(3)),
    p95RegressionGatePassed: p95Ratio <= 1.05,
    rssRegressionGatePassed: rssRatio === null || rssRatio <= 1.10,
    computeTargetPassed: p95Ratio <= (1 / 1.5) || (rssRatio !== null && rssRatio <= 0.70),
  };
}

function parseGoHash(stdout) {
  const marker = stdout.match(/JFTRADE_STAGE3_PROBE=(\{[^\n]+\})/);
  if (!marker) throw new Error("Go stage 3 benchmark marker is missing");
  return JSON.parse(marker[1]).resultHash;
}

function parseRustHash(stdout) {
  return JSON.parse(stdout).cases[0].resultHash;
}

export function runBacktestBenchmark({ warmups = 3, iterations = 20, repeats = 1000 } = {}) {
  const temporaryRoot = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-rust-stage3-benchmark-"));
  try {
    const goBinary = path.join(temporaryRoot, "backtest-reference.test");
    run("go", ["test", "-c", "-trimpath", "-o", goBinary, "./pkg/backtest"]);
    run("cargo", ["build", "--release", "-p", "jftrade-backtest", "--bin", "jftrade-backtest-shadow"]);
    const rustBinary = path.join(repositoryRoot, "target/release/jftrade-backtest-shadow");
    const goResult = benchmark(
      goBinary,
      ["-test.run", "^TestRustMigrationStage3ProcessProbe$", "-test.v"],
      {
        env: {
          JFTRADE_STAGE3_PROCESS_REPEAT: String(repeats),
          JFTRADE_STAGE3_FIXTURE_ROOT: path.dirname(fixturePath),
        },
      },
      warmups,
      iterations,
      parseGoHash,
    );
    const rustResult = benchmark(
      rustBinary,
      ["--input", fixturePath, "--repeat", String(repeats)],
      {},
      warmups,
      iterations,
      parseRustHash,
    );
    if (goResult.resultHash !== rustResult.resultHash) {
      throw new Error(`benchmark result hash mismatch: Go ${goResult.resultHash}, Rust ${rustResult.resultHash}`);
    }
    const casesPerProcess = JSON.parse(fs.readFileSync(fixturePath, "utf8")).cases.length * repeats;
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
        go: "go test -c -trimpath (production matcher reference harness)",
        rust: "cargo build --release",
      },
      workload: {
        fixture: "backtest-corpus.json",
        fixtureSha256: createHash("sha256").update(fs.readFileSync(fixturePath)).digest("hex"),
        casesPerCorpus: casesPerProcess / repeats,
        repeats,
        casesPerProcess,
      },
      go: {
        ...goResult,
        casesPerSecondP50: Number((casesPerProcess / (goResult.elapsedMillis.p50 / 1000)).toFixed(3)),
        binaryBytes: fs.statSync(goBinary).size,
      },
      rust: {
        ...rustResult,
        casesPerSecondP50: Number((casesPerProcess / (rustResult.elapsedMillis.p50 / 1000)).toFixed(3)),
        binaryBytes: fs.statSync(rustBinary).size,
      },
      gates: evaluateBacktestPerformance(goResult, rustResult),
      limitations: [
        "Go uses a compiled test harness because conservative-bar-v1 is intentionally package-private.",
        "Peak RSS and binary size include the Go test harness; use throughput and semantic hashes for the primary compute comparison.",
        "This local result does not replace native Linux, Windows, and macOS CI qualification.",
      ],
    };
  } finally {
    fs.rmSync(temporaryRoot, { force: true, recursive: true });
  }
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  const result = runBacktestBenchmark();
  if (!result.gates.p95RegressionGatePassed ||
      !result.gates.rssRegressionGatePassed ||
      !result.gates.computeTargetPassed) {
    console.error(JSON.stringify(result, null, 2));
    process.exitCode = 1;
  } else {
    console.log(JSON.stringify(result, null, 2));
  }
}
