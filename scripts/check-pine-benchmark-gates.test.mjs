import assert from "node:assert/strict";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { assertBenchmarkGates, compareBenchmarks, parseBenchmarkFile } from "./check-pine-benchmark-gates.mjs";

const samples = (ns, bytes, allocs) => [{ ns, bytes, allocs }, { ns, bytes, allocs }];

function writeBenchFile(contents) {
  const directory = mkdtempSync(join(tmpdir(), "jftrade-pine-bench-"));
  const path = join(directory, "bench.txt");
  writeFileSync(path, contents);
  return { directory, path };
}

test("allows runtime improvement without regressions", () => {
  const comparisons = compareBenchmarks(
    new Map([["BenchmarkRuntime", samples(100, 100, 10)]]),
    new Map([["BenchmarkRuntime", samples(75, 90, 10)]]),
  );
  assert.doesNotThrow(() => assertBenchmarkGates("runtime", comparisons));
});

test("rejects compile regressions over ten percent", () => {
  const comparisons = compareBenchmarks(
    new Map([["BenchmarkCompile", samples(100, 100, 10)]]),
    new Map([["BenchmarkCompile", samples(111, 100, 10)]]),
  );
  assert.throws(() => assertBenchmarkGates("compile", comparisons), /regressed 11.0%/);
});

test("allows golden non-regression without mandatory improvement", () => {
  const comparisons = compareBenchmarks(
    new Map([["BenchmarkGolden", samples(100, 100, 10)]]),
    new Map([["BenchmarkGolden", samples(90, 90, 10)]]),
  );
  assert.doesNotThrow(() => assertBenchmarkGates("golden", comparisons));
});

test("requires a twenty percent target improvement in performance mode", () => {
  const comparisons = compareBenchmarks(
    new Map([["BenchmarkGolden", samples(100, 100, 10)]]),
    new Map([["BenchmarkGolden", samples(90, 90, 10)]]),
  );
  assert.throws(
    () => assertBenchmarkGates("golden", comparisons, { requireImprovement: true }),
    /at least 20%/,
  );
});

test("parses go test benchmem lines", () => {
  const { directory, path } = writeBenchFile(`goos: darwin
goarch: arm64
BenchmarkPineAnalyzeScript/minimal-3\t100\t39060 ns/op\t31948 B/op\t323 allocs/op
BenchmarkPineAnalyzeScript/minimal-3\t100\t39100 ns/op\t31948 B/op\t323 allocs/op
`);
  try {
    const rows = parseBenchmarkFile(path);
    assert.equal(rows.get("BenchmarkPineAnalyzeScript/minimal")?.length, 2);
    assert.equal(rows.get("BenchmarkPineAnalyzeScript/minimal")[0].ns, 39060);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});

test("rejects a bench file with no parseable results", () => {
  const { directory, path } = writeBenchFile("ok  \tgithub.com/jftrade/jftrade-main/pkg/strategy/pine\t0.123s\n");
  try {
    assert.throws(() => parseBenchmarkFile(path), /No benchmarks parsed from/);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});
