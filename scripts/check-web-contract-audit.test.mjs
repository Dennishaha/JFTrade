import assert from "node:assert/strict";
import test from "node:test";

import { generateAPITypes } from "./generate-api-types.mjs";
import {
  generatedContractViolations,
  generatedSchemaViolations,
  normalizeRelativePath,
  viewModelClassificationViolations,
} from "./lib/web-contract-audit.mjs";

test("normalizes Windows relative paths for contract classification keys", () => {
  assert.equal(
    normalizeRelativePath("types\\view-models\\assistant.ts"),
    "types/view-models/assistant.ts",
  );
});

const fixtureSpec = {
  definitions: {
    "fixture.Item": {
      type: "object",
      required: ["id", "state", "tags"],
      properties: {
        id: { type: "string" },
        note: { type: "string", "x-nullable": true },
        state: { type: "string", enum: ["ready", "failed"] },
        tags: { type: "array", items: { type: "string" } },
      },
    },
  },
  paths: {},
};

test("audits generated schemas field-for-field including required, nullable, enum, and arrays", () => {
  const generated = generateAPITypes(fixtureSpec);
  assert.deepEqual(generatedSchemaViolations(fixtureSpec, generated), []);

  const changed = generated
    .replace("id: string;", "id?: string;")
    .replace("Array<string>", "Array<number>");
  const violations = generatedSchemaViolations(fixtureSpec, changed);
  assert.equal(violations.length, 1);
  assert.match(violations[0], /fixture\.Item/);
});

test("accepts direct generated aliases and rejects handwritten contract declarations", () => {
  const valid = generatedContractViolations({
    indexSource: 'export * from "./generated/items";\n',
    generatedSources: new Map([
      [
        "items.ts",
        [
          'import type { components } from "@/generated/openapi";',
          'export type Item = components["schemas"]["fixture.Item"];',
          "",
        ].join("\n"),
      ],
    ]),
    schemaNames: new Set(["fixture.Item"]),
  });
  assert.deepEqual(valid, []);

  const invalid = generatedContractViolations({
    indexSource: 'export * from "./manual";\n',
    generatedSources: new Map([
      ["manual.ts", "export interface Item { id: string }\n"],
    ]),
    schemaNames: new Set(["fixture.Item"]),
  });
  assert.ok(invalid.some((message) => message.includes("non-generated")));
  assert.ok(invalid.some((message) => message.includes("generated type aliases")));
});

test("requires every normalized model module to declare real adapters and tests", () => {
  const violations = viewModelClassificationViolations({
    classification: {
      "types/view-models/item.ts": {
        kind: "normalized-api",
        adapters: ["composables/itemMapper.ts"],
        tests: ["tests/itemMapper.test.ts"],
      },
    },
    sources: new Map([
      [
        "types/view-models/item.ts",
        "export interface ItemView { id: string }\n",
      ],
    ]),
    adapterSources: new Map(),
    testFiles: new Set(),
  });
  assert.ok(violations.some((message) => message.includes("does not exist")));
  assert.ok(violations.some((message) => message.includes("does not exist")));
});

test("matches canonical classification paths against Windows source and test paths", () => {
  const violations = viewModelClassificationViolations({
    classification: {
      "types/view-models/item.ts": {
        kind: "normalized-api",
        adapters: ["composables/itemMapper.ts"],
        tests: ["tests/itemMapper.test.ts"],
      },
    },
    sources: new Map([
      [
        "types\\view-models\\item.ts",
        'export interface ItemView { id: string }\n',
      ],
    ]),
    adapterSources: new Map([
      [
        "composables\\itemMapper.ts",
        [
          'import type { ItemView } from "@/types/view-models/item";',
          "export function mapItem(value: ItemView) { return value; }",
          "",
        ].join("\n"),
      ],
    ]),
    testFiles: new Set(["tests\\itemMapper.test.ts"]),
  });
  assert.deepEqual(violations, []);
});

test("rejects unclassified handwritten type modules", () => {
  const violations = viewModelClassificationViolations({
    classification: {},
    sources: new Map([
      ["types/view-models/orphan.ts", "export type Orphan = string;\n"],
    ]),
    adapterSources: new Map(),
    testFiles: new Set(),
  });
  assert.deepEqual(violations, [
    "types/view-models/orphan.ts: missing type classification",
  ]);
});
