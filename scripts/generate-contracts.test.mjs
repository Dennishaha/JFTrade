import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";

import { assertTrackedOutputsMatch, trackedOutputs } from "./generate-contracts.mjs";

test("compares only committed outputs and keeps runtime Swagger temporary", () => {
  assert.deepEqual(trackedOutputs, [
    "apps/web/src/generated/openapi.ts",
    "tests/fixtures/openapi-baseline.json",
    "docs/reference/generated/pine-v6-support.md",
  ]);
  assert.equal(trackedOutputs.includes("docs/swagger/docs.go"), false);
  assert.equal(trackedOutputs.includes("docs/swagger/swagger.runtime.json"), false);
  assert.equal(trackedOutputs.includes("docs/reference/generated/api.md"), false);
  assert.equal(trackedOutputs.includes("docs/reference/generated/types.md"), false);
});

test("all compared outputs exist in the Git tree of a clean clone", () => {
  const result = spawnSync("git", ["ls-files", "--error-unmatch", "--", ...trackedOutputs], {
    encoding: "utf8",
  });
  assert.equal(result.status, 0, result.stderr);
});

test("reports drift using injected roots without requiring ignored local artifacts", async (t) => {
  const expectedRoot = await makeRoot();
  const generatedRoot = await makeRoot();
  t.after(() => Promise.all([
    rm(expectedRoot, { recursive: true, force: true }),
    rm(generatedRoot, { recursive: true, force: true }),
  ]));
  const outputs = ["contract.txt", "tests/fixtures/openapi-baseline.json"];
  await writeOutputs(expectedRoot, outputs, "same");
  await writeOutputs(generatedRoot, outputs, "same");

  await assert.doesNotReject(assertTrackedOutputsMatch(generatedRoot, { expectedRoot, outputs }));
  await writeOutputs(generatedRoot, [outputs[1]], "changed");
  await assert.rejects(
    assertTrackedOutputsMatch(generatedRoot, { expectedRoot, outputs }),
    /tests\/fixtures\/openapi-baseline\.json/,
  );
});

async function makeRoot() {
  return mkdtemp(path.join(tmpdir(), "jftrade-contract-test-"));
}

async function writeOutputs(root, outputs, value) {
  for (const output of outputs) {
    const file = path.join(root, output);
    await mkdir(path.dirname(file), { recursive: true });
    await writeFile(file, value);
  }
}
