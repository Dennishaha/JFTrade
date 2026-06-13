import fs from "node:fs";
import { pathToFileURL } from "node:url";

export function parseBenchmarkFile(path) {
  const rows = new Map();
  for (const line of fs.readFileSync(path, "utf8").split("\n")) {
    const match = line.match(/^(Benchmark\S+)-\d+\s+\d+\s+([\d.]+) ns\/op\s+([\d.]+) B\/op\s+([\d.]+) allocs\/op/);
    if (!match) continue;
    const values = rows.get(match[1]) ?? [];
    values.push({ ns: Number(match[2]), bytes: Number(match[3]), allocs: Number(match[4]) });
    rows.set(match[1], values);
  }
  return rows;
}

function median(values) {
  const sorted = [...values].sort((left, right) => left - right);
  return sorted[Math.floor(sorted.length / 2)];
}

export function compareBenchmarks(baseRows, headRows) {
  const comparisons = [];
  for (const [name, baseSamples] of baseRows) {
    const headSamples = headRows.get(name);
    if (!headSamples?.length) throw new Error(`missing head benchmark ${name}`);
    const metrics = {};
    for (const metric of ["ns", "bytes", "allocs"]) {
      const base = median(baseSamples.map((sample) => sample[metric]));
      const head = median(headSamples.map((sample) => sample[metric]));
      metrics[metric] = { base, head, delta: base === 0 ? 0 : ((head / base) - 1) * 100 };
    }
    comparisons.push({ name, metrics });
  }
  return comparisons;
}

export function assertBenchmarkGates(mode, comparisons) {
  const failures = [];
  for (const comparison of comparisons) {
    const { ns, bytes, allocs } = comparison.metrics;
    const nsLimit = mode === "golden" ? 15 : 10;
    if (ns.delta > nsLimit) failures.push(`${comparison.name} ns/op regressed ${ns.delta.toFixed(1)}%`);
    if (bytes.delta > 10) failures.push(`${comparison.name} B/op regressed ${bytes.delta.toFixed(1)}%`);
    if (allocs.delta > 10) failures.push(`${comparison.name} allocs/op regressed ${allocs.delta.toFixed(1)}%`);
  }
  if (mode === "runtime" || mode === "golden") {
    const hasTargetImprovement = comparisons.some(({ metrics }) => metrics.ns.delta <= -20 || metrics.bytes.delta <= -20);
    if (!hasTargetImprovement) failures.push(`${mode} benchmarks did not improve ns/op or B/op by at least 20%`);
  }
  if (failures.length) throw new Error(failures.join("\n"));
}

function main() {
  const [mode, basePath, headPath] = process.argv.slice(2);
  if (!["compile", "runtime", "golden"].includes(mode) || !basePath || !headPath) {
    throw new Error("usage: node scripts/check-pine-benchmark-gates.mjs <compile|runtime|golden> <base.txt> <head.txt>");
  }
  const comparisons = compareBenchmarks(parseBenchmarkFile(basePath), parseBenchmarkFile(headPath));
  assertBenchmarkGates(mode, comparisons);
  for (const { name, metrics } of comparisons) {
    console.log(`${name}: ns ${metrics.ns.delta.toFixed(1)}%, bytes ${metrics.bytes.delta.toFixed(1)}%, allocs ${metrics.allocs.delta.toFixed(1)}%`);
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main();
}
