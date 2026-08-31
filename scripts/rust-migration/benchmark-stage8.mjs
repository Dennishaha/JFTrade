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
const fixtureRoot = path.join(repositoryRoot, "tests/fixtures/rust-migration/stage8");
const fixturePath = path.join(fixtureRoot, "desktop-shell-corpus.json");
const expectedPath = path.join(fixtureRoot, "desktop-shell-corpus.expected.json");
const baselinePath = path.join(fixtureRoot, `resource-baseline.${process.platform}-${process.arch}.json`);

function run(command, args, options = {}) {
  const timeout = options.timeoutMs ?? 300_000;
  const result = spawnSync(command, args, {
    cwd: repositoryRoot,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
    maxBuffer: 16 * 1024 * 1024,
    timeout,
    killSignal: "SIGTERM",
  });
  if (result.error?.code === "ETIMEDOUT") throw new Error(`${command} timed out after ${timeout}ms`);
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`${command} ${args.join(" ")} failed:\n${result.stdout}${result.stderr}`);
  return result;
}

function percentile(values, ratio) {
  const sorted = [...values].sort((left, right) => left - right);
  return Number(sorted[Math.max(0, Math.ceil(ratio * sorted.length) - 1)].toFixed(3));
}

function parseResources(stderr) {
  if (process.platform !== "darwin") return { cpuMillis: null, peakRssBytes: null };
  const rss = stderr.match(/^\s*(\d+)\s+maximum resident set size$/m);
  const times = stderr.match(/([\d.]+)\s+real\s+([\d.]+)\s+user\s+([\d.]+)\s+sys/);
  return {
    cpuMillis: times ? Number(((Number(times[2]) + Number(times[3])) * 1000).toFixed(3)) : null,
    peakRssBytes: rss ? Number(rss[1]) : null,
  };
}

function timedRun(command, args) {
  const useTime = process.platform === "darwin" && fs.existsSync("/usr/bin/time");
  const startedAt = performance.now();
  const result = useTime ? run("/usr/bin/time", ["-l", command, ...args]) : run(command, args);
  return {
    elapsedMillis: Number((performance.now() - startedAt).toFixed(3)),
    stdout: result.stdout,
    ...parseResources(result.stderr),
  };
}

export function summarizeStage8Samples(samples, warmups) {
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

export function evaluateStage8Performance(goResult, rustResult) {
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
  return summarizeStage8Samples(Array.from({ length: iterations }, () => sample()), warmups);
}

export function runStage8Benchmark({ warmups = 3, iterations = 20 } = {}) {
  const temporaryRoot = fs.mkdtempSync(path.join(os.tmpdir(), "jftrade-rust-stage8-benchmark-"));
  try {
    const goBinary = path.join(temporaryRoot, "desktop-reference.test");
    run("go", ["test", "-c", "-trimpath", "-o", goBinary, "./internal/desktop"]);
    run("cargo", ["build", "--release", "-p", "jftrade-desktop", "--bin", "jftrade-stage8-shadow"]);
    const rustBinary = path.join(repositoryRoot, "target/release/jftrade-stage8-shadow");
    const expected = JSON.parse(fs.readFileSync(expectedPath, "utf8"));
    const goTests = "^(TestProductDataDirByPlatform|TestProductDataDirUsesCurrentPlatform)$";
    const sampleGo = () => timedRun(goBinary, ["-test.run", goTests, "-test.count=1"]);
    const sampleRust = () => {
      const sample = timedRun(rustBinary, ["--input", fixturePath]);
      if (!isDeepStrictEqual(JSON.parse(sample.stdout), expected)) {
        throw new Error("Rust Stage 8 benchmark output drifted from pinned expected JSON");
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
        go: "go test -c -trimpath (retained Go desktop utility reference)",
        rust: "cargo build --release (Tauri facade projection; no native WebView)",
      },
      workload: {
        fixture: path.basename(fixturePath),
        fixtureSha256: createHash("sha256").update(fs.readFileSync(fixturePath)).digest("hex"),
        expectedSha256,
        platforms: expected.profiles.length,
        links: expected.links.length,
        commands: expected.commands.length,
        events: expected.events.length,
      },
      go: { ...goResult, binaryBytes: fs.statSync(goBinary).size },
      rust: { ...rustResult, binaryBytes: fs.statSync(rustBinary).size },
      gates: evaluateStage8Performance(goResult, rustResult),
      limitations: [
        "The Go sample executes retained desktop utility reference tests; the Rust sample projects the Tauri facade and supervised lifecycle plan without opening a native WebView.",
        "This benchmark does not qualify UI rendering, signing, installers, actual subprocesses, upgrade compatibility, or orphan detection.",
        "The local Darwin result does not replace native macOS ARM64, Linux x64, Windows x64, and Windows ARM64 package smoke.",
      ],
    };
  } finally {
    fs.rmSync(temporaryRoot, { force: true, recursive: true });
  }
}

if (pathToFileURL(path.resolve(process.argv[1] ?? "")).href === import.meta.url) {
  const result = runStage8Benchmark();
  if (process.argv.includes("--write-baseline")) {
    fs.writeFileSync(baselinePath, `${JSON.stringify(result, null, 2)}\n`);
  }
  if (!result.gates.p95RegressionGatePassed || !result.gates.rssRegressionGatePassed) {
    console.error(JSON.stringify(result, null, 2));
    process.exitCode = 1;
  } else {
    console.log(JSON.stringify(result, null, 2));
  }
}
