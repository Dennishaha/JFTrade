import assert from "node:assert/strict";
import test from "node:test";

import {
  compareOpenAPIImportPolicy,
  directGeneratedOpenAPIImportFiles,
  isOpenAPIImportInfrastructure,
  parseOpenAPIImportAllowlist,
} from "./check-web-openapi-imports.mjs";

test("finds alias and relative raw OpenAPI imports in TypeScript and Vue sources", () => {
  const imports = directGeneratedOpenAPIImportFiles(new Map([
    [
      "apps/web/src/composables/alias.ts",
      'import type { components } from "@/generated/openapi";\n',
    ],
    [
      "apps/web/src/components/relative.vue",
      '<script setup lang="ts">\nimport type { paths } from "../generated/openapi";\n</script>\n',
    ],
    [
      "apps/web/src/components/comment.vue",
      '<script setup lang="ts">\n// import type { paths } from "@/generated/openapi";\n</script>\n',
    ],
  ]));

  assert.deepEqual(imports, [
    "apps/web/src/components/relative.vue",
    "apps/web/src/composables/alias.ts",
  ]);
});

test("sanctions only the wire contract boundary and exact client infrastructure", () => {
  assert.equal(
    isOpenAPIImportInfrastructure("apps/web/src/contracts/wire/system.ts"),
    true,
  );
  assert.equal(
    isOpenAPIImportInfrastructure("apps/web/src/composables/shared/apiClient.ts"),
    true,
  );
  assert.equal(
    isOpenAPIImportInfrastructure("apps/web/tests/contracts/contractsModularization.test.ts"),
    true,
  );
  assert.equal(
    isOpenAPIImportInfrastructure("apps/web/src/components/apiClient.ts"),
    false,
  );
  assert.equal(
    isOpenAPIImportInfrastructure("apps/web/src/composables/apiClient.ts"),
    false,
  );
});

test("reports new imports, stale exceptions, and allowlist growth independently", () => {
  const result = compareOpenAPIImportPolicy({
    directImports: [
      "apps/web/src/components/legacy.vue",
      "apps/web/src/components/new.vue",
      "apps/web/src/components/unallowlisted.vue",
      "apps/web/src/contracts/wire/system.ts",
    ],
    allowlistEntries: new Map([
      ["apps/web/src/components/legacy.vue", "legacy"],
      ["apps/web/src/components/resolved.vue", "resolved"],
      ["apps/web/src/components/new.vue", "newly allowlisted"],
    ]),
    baseDirectImports: [
      "apps/web/src/components/legacy.vue",
      "apps/web/src/components/resolved.vue",
    ],
  });

  assert.deepEqual(result.unallowlisted, [
    "apps/web/src/components/unallowlisted.vue",
  ]);
  assert.deepEqual(result.stale, ["apps/web/src/components/resolved.vue"]);
  assert.deepEqual(result.growth, ["apps/web/src/components/new.vue"]);
  assert.deepEqual(result.currentDebt, [
    "apps/web/src/components/legacy.vue",
    "apps/web/src/components/new.vue",
    "apps/web/src/components/unallowlisted.vue",
  ]);
});

test("requires normalized debt entries with concrete migration reasons", () => {
  const parsed = parseOpenAPIImportAllowlist({
    version: 1,
    legacyDirectImports: {
      "apps\\web\\src\\legacy.ts": "short",
      "apps/web/src/contracts/wire/system.ts":
        "wire contracts are infrastructure and need no exception",
    },
  });

  assert.ok(parsed.failures.some((failure) => failure.includes("normalized")));
  assert.ok(parsed.failures.some((failure) => failure.includes("concrete migration reason")));
  assert.ok(parsed.failures.some((failure) => failure.includes("must not be allowlisted")));
});

test("accepts an empty legacy manifest after all consumers migrate", () => {
  const parsed = parseOpenAPIImportAllowlist({
    version: 1,
    legacyDirectImports: {},
  });

  assert.deepEqual([...parsed.entries], []);
  assert.deepEqual(parsed.failures, []);
});
