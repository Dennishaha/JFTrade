#!/usr/bin/env node
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { performance } from "node:perf_hooks";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const fixtureRoot = path.join(repositoryRoot, "tests/fixtures/rust-migration/stage2");

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? repositoryRoot,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  });
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

function timedRun(command, args) {
  const timeArgs = process.platform === "darwin"
    ? ["-l", command, ...args]
    : process.platform === "linux"
      ? ["-v", command, ...args]
      : null;
  const startedAt = performance.now();
  const result = timeArgs
    ? run("/usr/bin/time", timeArgs)
    : run(command, args);
  return {
    elapsedMillis: Number((performance.now() - startedAt).toFixed(3)),
    ...parseResources(result.stderr),
  };
}

function benchmark(command, args, warmups, iterations) {
  for (let index = 0; index < warmups; index += 1) run(command, args);
  const samples = [];
  for (let index = 0; index < iterations; index += 1) {
    samples.push(timedRun(command, args));
  }
  return summarizeSamples(samples, warmups);
}

export function summarizeSamples(samples, warmups) {
  const elapsed = samples.map((sample) => sample.elapsedMillis);
  const rss = samples.map((sample) => sample.peakRssBytes).filter(Number.isFinite);
  const cpu = samples.map((sample) => sample.cpuMillis).filter(Number.isFinite);
  return {
    iterations: samples.length,
    warmups,
    elapsedMillis: {
      p50: percentile(elapsed, 0.5),
      p95: percentile(elapsed, 0.95),
    },
    cpuMillisP50: cpu.length > 0 ? percentile(cpu, 0.5) : null,
    peakRssBytes: rss.length > 0 ? Math.max(...rss) : null,
  };
}

export function runBenchmark({ warmups = 3, iterations = 20 } = {}) {
  const temporaryRoot = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-rust-stage2-benchmark-"));
  try {
    const goBinary = path.join(temporaryRoot, "sqlite-oracle");
    const databasePath = path.join(temporaryRoot, "backtest.db");
    run("go", ["build", "-trimpath", "-o", goBinary, "./scripts/rust-migration/cmd/sqlite-oracle"]);
    run("cargo", ["build", "--release", "-p", "jftrade-store-sqlite", "--bin", "jftrade-sqlite-inspect"]);
    const rustBinary = path.join(repositoryRoot, "target/release/jftrade-sqlite-inspect");
    const fixturePath = path.join(fixtureRoot, "backtest-readonly.sql");
    run(goBinary, ["--sql", fixturePath, "--db", databasePath]);

    const goResult = benchmark(
      goBinary,
      ["--inspect-only", "--db", databasePath],
      warmups,
      iterations,
    );
    const rustResult = benchmark(rustBinary, [databasePath], warmups, iterations);
    const fixtureHash = createHash("sha256").update(fs.readFileSync(fixturePath)).digest("hex");
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
        go: "go build -trimpath",
        rust: "cargo build --release",
      },
      workload: {
        fixture: "backtest-readonly.sql",
        fixtureSha256: fixtureHash,
        tables: 2,
        rows: 3,
      },
      go: {
        ...goResult,
        binaryBytes: fs.statSync(goBinary).size,
      },
      rust: {
        ...rustResult,
        binaryBytes: fs.statSync(rustBinary).size,
      },
      ratios: {
        rustToGoP95: Number((rustResult.elapsedMillis.p95 / goResult.elapsedMillis.p95).toFixed(3)),
        rustToGoPeakRss: goResult.peakRssBytes && rustResult.peakRssBytes
          ? Number((rustResult.peakRssBytes / goResult.peakRssBytes).toFixed(3))
          : null,
      },
    };
  } finally {
    fs.rmSync(temporaryRoot, { force: true, recursive: true });
  }
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  console.log(JSON.stringify(runBenchmark(), null, 2));
}
