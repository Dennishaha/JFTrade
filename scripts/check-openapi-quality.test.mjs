import assert from "node:assert/strict";
import test from "node:test";

import {
  buildQualityAllowlist,
  compareQualityGaps,
  findOpenAPIQualityGaps,
} from "./lib/openapi-quality.mjs";

function typedEnvelope(dataSchema) {
  return {
    allOf: [
      { $ref: "#/definitions/httpserver.Envelope" },
      { type: "object", properties: { data: dataSchema } },
    ],
  };
}

function baseSpec() {
  return {
    swagger: "2.0",
    definitions: {
      "httpserver.Envelope": {
        type: "object",
        properties: { data: {}, ok: { type: "boolean" } },
      },
      "httpserver.ErrorEnvelope": {
        type: "object",
        properties: { error: { type: "object" }, ok: { type: "boolean" } },
      },
      CreateItemRequest: {
        type: "object",
        properties: { name: { type: "string" } },
      },
      Item: { type: "object", properties: { id: { type: "string" } } },
    },
    paths: {
      "/items/{itemId}": {
        get: {
          produces: ["application/json"],
          parameters: [{ name: "itemId", in: "path", required: true, type: "string" }],
          responses: {
            200: { schema: typedEnvelope({ $ref: "#/definitions/Item" }) },
            404: { schema: { $ref: "#/definitions/httpserver.ErrorEnvelope" } },
          },
        },
        post: {
          consumes: ["application/json"],
          produces: ["application/json"],
          parameters: [
            { name: "itemId", in: "path", required: true, type: "string" },
            {
              name: "request",
              in: "body",
              required: true,
              schema: { $ref: "#/definitions/CreateItemRequest" },
            },
          ],
          responses: {
            200: { schema: typedEnvelope({ $ref: "#/definitions/Item" }) },
            400: { schema: { $ref: "#/definitions/httpserver.ErrorEnvelope" } },
          },
        },
      },
      "/api/v1/adk/chat/stream": {
        post: {
          produces: ["text/event-stream"],
          "x-error-produces": ["application/json"],
          responses: {
            200: { schema: { type: "string" } },
            500: { schema: { $ref: "#/definitions/httpserver.ErrorEnvelope" } },
          },
        },
      },
      "/api/v1/adk/runs/{runId}/stream": {
        get: {
          produces: ["text/event-stream"],
          "x-error-produces": ["application/json"],
          parameters: [{ name: "runId", in: "path", required: true, type: "string" }],
          responses: {
            200: { schema: { type: "string" } },
            404: { schema: { $ref: "#/definitions/httpserver.ErrorEnvelope" } },
          },
        },
      },
      "/api/v1/adk/streams/{streamId}": {
        get: {
          produces: ["text/event-stream"],
          "x-error-produces": ["application/json"],
          parameters: [{ name: "streamId", in: "path", required: true, type: "string" }],
          responses: {
            200: { schema: { type: "string" } },
            404: { schema: { $ref: "#/definitions/httpserver.ErrorEnvelope" } },
          },
        },
      },
      "/api/v1/ws/live": {
        get: {
          "x-error-produces": ["application/json"],
          responses: {
            101: { description: "Switching Protocols" },
            503: { schema: { $ref: "#/definitions/httpserver.ErrorEnvelope" } },
          },
        },
      },
    },
  };
}

test("accepts typed JSON envelopes and accurately documented protocols", () => {
  assert.deepEqual(findOpenAPIQualityGaps(baseSpec()), []);
});

test("reports bare envelopes, path parameter errors, and JSON protocol media", () => {
  const spec = baseSpec();
  spec.paths["/items/{itemId}"].get.parameters[0].required = false;
  spec.paths["/items/{itemId}"].get.responses[200].schema = {
    $ref: "#/definitions/httpserver.Envelope",
  };
  spec.paths["/api/v1/adk/chat/stream"].post.produces = ["application/json"];
  delete spec.paths["/api/v1/adk/chat/stream"].post["x-error-produces"];
  delete spec.paths["/api/v1/adk/runs/{runId}/stream"].get.parameters;
  delete spec.paths["/api/v1/ws/live"].get.responses[101];

  const ids = findOpenAPIQualityGaps(spec).map((entry) => entry.id);
  assert(ids.includes("json-success-data|GET /items/{itemId}|200"));
  assert(ids.includes("path-parameter-required|GET /items/{itemId}|itemId"));
  assert(ids.includes("protocol-json-media|POST /api/v1/adk/chat/stream|application/json"));
  assert(ids.includes("protocol-media|POST /api/v1/adk/chat/stream|text/event-stream"));
  assert(
    ids.includes(
      "protocol-error-media|POST /api/v1/adk/chat/stream|application/json",
    ),
  );
  assert(ids.includes("path-parameter-missing|GET /api/v1/adk/runs/{runId}/stream|runId"));
  assert(ids.includes("protocol-status|GET /api/v1/ws/live|101"));
});

test("reports generic error envelopes and missing independent JSON request DTOs", () => {
  const spec = baseSpec();
  spec.paths["/items/{itemId}"].get.responses[404].schema = {
    $ref: "#/definitions/httpserver.Envelope",
  };
  spec.paths["/items/{itemId}"].post.parameters = spec.paths[
    "/items/{itemId}"
  ].post.parameters.filter((parameter) => parameter.in !== "body");
  delete spec.paths["/api/v1/adk/chat/stream"].post.responses[500];

  const ids = findOpenAPIQualityGaps(spec).map((entry) => entry.id);
  assert(ids.includes("json-error-envelope|GET /items/{itemId}|404"));
  assert(ids.includes("json-request-body|POST /items/{itemId}|0"));
  assert(
    ids.includes(
      "error-response-missing|POST /api/v1/adk/chat/stream|4xx-or-5xx",
    ),
  );
});

test("reports request DTOs reused as successful response data", () => {
  const spec = baseSpec();
  spec.paths["/items/{itemId}"].post.responses[200].schema = typedEnvelope({
    $ref: "#/definitions/CreateItemRequest",
  });

  const ids = findOpenAPIQualityGaps(spec).map((entry) => entry.id);
  assert(
    ids.includes(
      "json-request-response-schema|POST /items/{itemId}|#/definitions/CreateItemRequest",
    ),
  );
});

test("allowlist rejects new gaps and stale or duplicate exceptions", () => {
  const gaps = [{ id: "rule|GET /one|200" }, { id: "rule|GET /two|200" }];
  const allowlist = buildQualityAllowlist(gaps);
  assert.deepEqual(compareQualityGaps(gaps, allowlist), {
    unexpected: [],
    stale: [],
    duplicates: [],
  });

  const drifted = compareQualityGaps([gaps[0]], {
    version: 1,
    gaps: [...allowlist.gaps, allowlist.gaps[0], { id: "stale|GET /old|200" }],
  });
  assert.equal(drifted.unexpected.length, 0);
  assert.equal(drifted.stale.length, 2);
  assert.equal(drifted.duplicates.length, 1);
});
