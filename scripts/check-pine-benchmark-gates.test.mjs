import assert from "node:assert/strict";
import test from "node:test";

import { assertBenchmarkGates, compareBenchmarks } from "./check-pine-benchmark-gates.mjs";

const samples = (ns, bytes, allocs) => [{ ns, bytes, allocs }, { ns, bytes, allocs }];

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

test("requires a twenty percent target improvement for golden benchmarks", () => {
  const comparisons = compareBenchmarks(
    new Map([["BenchmarkGolden", samples(100, 100, 10)]]),
    new Map([["BenchmarkGolden", samples(90, 90, 10)]]),
  );
  assert.throws(() => assertBenchmarkGates("golden", comparisons), /at least 20%/);
});
