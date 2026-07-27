import assert from "node:assert/strict";
import test from "node:test";

import { generateAPITypes, schemaToType } from "./generate-api-types.mjs";

test("generates Swagger media types, inherited parameters, and every response status", () => {
  const output = generateAPITypes({
    swagger: "2.0",
    consumes: ["application/json"],
    produces: ["application/json"],
    definitions: {
      Example: {
        type: "object",
        required: ["state", "tags"],
        properties: {
          state: { type: "string", enum: ["ready", "busy"] },
          tags: { type: "array", items: { type: "string", enum: ["a", "b"] } },
          note: { type: "string", "x-nullable": true },
        },
      },
    },
    paths: {
      "/examples/{exampleId}": {
        parameters: [
          { name: "exampleId", in: "path", required: true, type: "integer" },
          { name: "verbose", in: "query", type: "string" },
        ],
        post: {
          consumes: ["application/json", "application/merge-patch+json"],
          produces: ["application/json", "text/plain"],
          parameters: [
            { name: "verbose", in: "query", type: "boolean" },
            { name: "request", in: "body", required: true, schema: { $ref: "#/definitions/Example" } },
          ],
          responses: {
            200: { description: "ready", schema: { $ref: "#/definitions/Example" } },
            202: { description: "accepted", schema: { type: "array", items: { type: "integer" } } },
            400: { description: "bad request", schema: { type: "string" } },
          },
        },
      },
    },
  });

  assert.match(output, /state: "ready" \| "busy";/);
  assert.match(output, /tags: Array<"a" \| "b">;/);
  assert.match(output, /note\?: string \| null;/);
  assert.match(output, /exampleId: number;/);
  assert.match(output, /verbose\?: boolean;/);
  assert.doesNotMatch(output, /verbose\?: string;/);
  assert.match(output, /requestBody: \{/);
  assert.match(output, /"application\/json": components\["schemas"\]\["Example"\];/);
  assert.match(output, /"application\/merge-patch\+json": components\["schemas"\]\["Example"\];/);
  assert.match(output, /"text\/plain": components\["schemas"\]\["Example"\];/);
  assert.match(output, /"202": \{/);
  assert.match(output, /"400": \{/);
});

test("keeps optional request bodies and typed form arrays", () => {
  const output = generateAPITypes({
    swagger: "2.0",
    paths: {
      "/uploads": {
        post: {
          consumes: ["multipart/form-data"],
          produces: ["application/json"],
          parameters: [
            { name: "labels", in: "formData", type: "array", items: { type: "string" } },
          ],
          responses: { 201: { description: "created", schema: { type: "boolean" } } },
        },
      },
    },
  });

  assert.match(output, /requestBody\?: \{/);
  assert.match(output, /"multipart\/form-data": \{/);
  assert.match(output, /labels\?: Array<string>;/);
  assert.match(output, /"201": \{/);
});

test("supports OpenAPI nullable and null type unions", () => {
  assert.equal(schemaToType({ type: "number", nullable: true }), "number | null");
  assert.equal(schemaToType({ type: ["string", "null"] }), "string | null");
});

test("uses JSON media for protocol startup errors", () => {
  const output = generateAPITypes({
    swagger: "2.0",
    definitions: {
      ErrorEnvelope: {
        type: "object",
        properties: { ok: { type: "boolean" } },
      },
    },
    paths: {
      "/stream": {
        get: {
          produces: ["text/event-stream"],
          "x-error-produces": ["application/json"],
          responses: {
            200: { description: "stream", schema: { type: "string" } },
            503: {
              description: "unavailable",
              schema: { $ref: "#/definitions/ErrorEnvelope" },
            },
          },
        },
      },
    },
  });

  assert.match(
    output,
    /"200": \{[\s\S]*?"text\/event-stream": string;[\s\S]*?"503": \{[\s\S]*?"application\/json": components\["schemas"\]\["ErrorEnvelope"\];/,
  );
});
