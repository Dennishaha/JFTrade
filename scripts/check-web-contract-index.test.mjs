import assert from "node:assert/strict";
import test from "node:test";

import { contractIndexViolations } from "./lib/web-contract-index.mjs";

test("accepts star, named, and type-only re-exports from explicit modules", () => {
  const source = [
    'export * from "./system";',
    'export { emptySystemStatus } from "./system";',
    'export type { SystemStatusResponse } from "./system";',
    "",
  ].join("\n");

  assert.deepEqual(contractIndexViolations(source), []);
});

test("rejects declarations and imports even when they use contract-like names", () => {
  const source = [
    'import type { components } from "@/generated/openapi";',
    "export interface HealthResponse { ok: boolean }",
    'export type Status = "ok";',
    "export const emptyStatus = { ok: false };",
  ].join("\n");

  assert.deepEqual(
    contractIndexViolations(source).map(({ line }) => line),
    [1, 2, 3, 4],
  );
});

test("rejects local export lists without a module source", () => {
  const source = [
    "const localValue = 1;",
    "export { localValue };",
  ].join("\n");

  assert.deepEqual(
    contractIndexViolations(source).map(({ line }) => line),
    [1, 2],
  );
});
