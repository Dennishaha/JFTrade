const HTTP_METHODS = ["get", "post", "put", "patch", "delete", "options", "head"];

const PROTOCOL_OPERATIONS = new Map([
  ["POST /api/v1/adk/chat/stream", { kind: "sse", successStatus: "200" }],
  ["GET /api/v1/adk/runs/{runId}/stream", { kind: "sse", successStatus: "200" }],
  ["GET /api/v1/adk/streams/{streamId}", { kind: "sse", successStatus: "200" }],
  ["GET /api/v1/ws/live", { kind: "websocket", successStatus: "101" }],
]);

function operationMediaTypes(spec, operation, field) {
  return operation[field] ?? spec[field] ?? [];
}

function isJSONMediaType(mediaType) {
  return (
    mediaType === "application/json" ||
    /^application\/[^;]+\+json(?:;|$)/.test(mediaType)
  );
}

function operationParameters(pathItem, operation) {
  const parameters = [...(pathItem.parameters ?? [])];
  for (const parameter of operation.parameters ?? []) {
    const index = parameters.findIndex(
      (candidate) =>
        candidate.in === parameter.in && candidate.name === parameter.name,
    );
    if (index === -1) {
      parameters.push(parameter);
    } else {
      parameters[index] = parameter;
    }
  }
  return parameters;
}

function schemaForRef(spec, ref) {
  const prefix = "#/definitions/";
  if (typeof ref !== "string" || !ref.startsWith(prefix)) {
    return null;
  }
  return spec.definitions?.[ref.slice(prefix.length)] ?? null;
}

function isConcreteSchema(spec, schema, seen = new Set()) {
  if (schema == null || typeof schema !== "object") {
    return false;
  }
  if (schema.$ref != null) {
    if (seen.has(schema.$ref) || schema.$ref === "#/definitions/httpserver.Envelope") {
      return false;
    }
    seen.add(schema.$ref);
    return isConcreteSchema(spec, schemaForRef(spec, schema.$ref), seen);
  }
  if (Array.isArray(schema.enum) && schema.enum.length > 0) {
    return true;
  }
  if (schema.type === "array") {
    return isConcreteSchema(spec, schema.items, seen);
  }
  if (["boolean", "integer", "number", "string"].includes(schema.type)) {
    return true;
  }
  if (Array.isArray(schema.allOf)) {
    return schema.allOf.some((entry) => isConcreteSchema(spec, entry, new Set(seen)));
  }
  return Object.keys(schema.properties ?? {}).length > 0 || schema.additionalProperties != null;
}

function envelopeDataSchemas(spec, schema, seen = new Set()) {
  if (schema == null || typeof schema !== "object") {
    return [];
  }
  if (schema.$ref != null) {
    if (seen.has(schema.$ref)) {
      return [];
    }
    seen.add(schema.$ref);
    return envelopeDataSchemas(spec, schemaForRef(spec, schema.$ref), seen);
  }
  const candidates = [];
  if (schema.properties?.data != null) {
    candidates.push(schema.properties.data);
  }
  for (const entry of schema.allOf ?? []) {
    candidates.push(...envelopeDataSchemas(spec, entry, new Set(seen)));
  }
  return candidates;
}

function hasConcreteEnvelopeData(spec, response) {
  return envelopeDataSchemas(spec, response?.schema).some((schema) =>
    isConcreteSchema(spec, schema),
  );
}

function schemaReferences(schema, targetRef, seen = new Set()) {
  if (schema == null || typeof schema !== "object") {
    return false;
  }
  if (schema.$ref != null) {
    if (schema.$ref === targetRef) {
      return true;
    }
    if (seen.has(schema.$ref)) {
      return false;
    }
    seen.add(schema.$ref);
  }
  return (schema.allOf ?? []).some((entry) =>
    schemaReferences(entry, targetRef, new Set(seen)),
  );
}

function jsonBodyGaps(spec, operationKey, pathItem, operation) {
  const consumesJSON = operationMediaTypes(spec, operation, "consumes").some(
    isJSONMediaType,
  );
  if (!consumesJSON) {
    return [];
  }
  const bodyParameters = operationParameters(pathItem, operation).filter(
    (parameter) => parameter.in === "body",
  );
  if (bodyParameters.length !== 1) {
    return [gap("json-request-body", operationKey, String(bodyParameters.length))];
  }
  const schemaRef = bodyParameters[0].schema?.$ref;
  if (
    typeof schemaRef !== "string" ||
    !schemaRef.startsWith("#/definitions/") ||
    schemaRef === "#/definitions/httpserver.Envelope"
  ) {
    return [gap("json-request-dto", operationKey, schemaRef ?? "missing")];
  }
  const gaps = [];
  for (const [status, response] of Object.entries(operation.responses ?? {})) {
    if (!/^2\d\d$/.test(status)) {
      continue;
    }
    const reusesRequestSchema = envelopeDataSchemas(spec, response?.schema).some(
      (dataSchema) => schemaReferences(dataSchema, schemaRef),
    );
    if (reusesRequestSchema) {
      gaps.push(gap("json-request-response-schema", operationKey, schemaRef));
      break;
    }
  }
  return gaps;
}

function gap(rule, operationKey, detail) {
  return {
    id: `${rule}|${operationKey}|${detail}`,
    rule,
    operation: operationKey,
    detail,
  };
}

function protocolGaps(spec, operationKey, operation, protocol) {
  const gaps = [];
  const produces = operationMediaTypes(spec, operation, "produces");
  const errorProduces = operation["x-error-produces"] ?? [];
  if (produces.some(isJSONMediaType)) {
    gaps.push(
      gap(
        "protocol-json-media",
        operationKey,
        produces.filter(isJSONMediaType).join(","),
      ),
    );
  }
  if (protocol.kind === "sse" && !produces.includes("text/event-stream")) {
    gaps.push(gap("protocol-media", operationKey, "text/event-stream"));
  }
  if (operation.responses?.[protocol.successStatus] == null) {
    gaps.push(gap("protocol-status", operationKey, protocol.successStatus));
  }
  if (!errorProduces.some(isJSONMediaType)) {
    gaps.push(gap("protocol-error-media", operationKey, "application/json"));
  }
  return gaps;
}

function errorResponseGaps(operationKey, operation) {
  const gaps = [];
  const errors = Object.entries(operation.responses ?? {}).filter(([status]) =>
    /^[45]\d\d$/.test(status),
  );
  if (errors.length === 0) {
    return [gap("error-response-missing", operationKey, "4xx-or-5xx")];
  }
  for (const [status, response] of errors) {
    if (
      !schemaReferences(
        response?.schema,
        "#/definitions/httpserver.ErrorEnvelope",
      )
    ) {
      gaps.push(gap("json-error-envelope", operationKey, status));
    }
  }
  return gaps;
}

export function findOpenAPIQualityGaps(spec) {
  const gaps = [];
  for (const [route, pathItem] of Object.entries(spec.paths ?? {})) {
    const placeholders = [...route.matchAll(/\{([^{}]+)\}/g)].map((match) => match[1]);
    for (const method of HTTP_METHODS) {
      const operation = pathItem[method];
      if (operation == null) {
        continue;
      }
      const operationKey = `${method.toUpperCase()} ${route}`;
      const parameters = operationParameters(pathItem, operation);
      for (const name of placeholders) {
        const parameter = parameters.find(
          (entry) => entry.in === "path" && entry.name === name,
        );
        if (parameter == null) {
          gaps.push(gap("path-parameter-missing", operationKey, name));
        } else if (parameter.required !== true) {
          gaps.push(gap("path-parameter-required", operationKey, name));
        }
      }

      gaps.push(...jsonBodyGaps(spec, operationKey, pathItem, operation));
      gaps.push(...errorResponseGaps(operationKey, operation));

      const protocol = PROTOCOL_OPERATIONS.get(operationKey);
      if (protocol != null) {
        gaps.push(...protocolGaps(spec, operationKey, operation, protocol));
        continue;
      }

      const producesJSON = operationMediaTypes(
        spec,
        operation,
        "produces",
      ).some(isJSONMediaType);
      if (!producesJSON) {
        continue;
      }
      for (const [status, response] of Object.entries(operation.responses ?? {})) {
        if (!/^2\d\d$/.test(status) || status === "204") {
          continue;
        }
        if (!hasConcreteEnvelopeData(spec, response)) {
          gaps.push(gap("json-success-data", operationKey, status));
        }
      }
    }
  }
  return gaps.sort((left, right) => left.id.localeCompare(right.id));
}

export function compareQualityGaps(gaps, allowlist) {
  const actual = new Set(gaps.map((entry) => entry.id));
  const allowedEntries = allowlist?.gaps ?? [];
  const allowed = new Set(allowedEntries.map((entry) => entry.id));
  return {
    unexpected: gaps.filter((entry) => !allowed.has(entry.id)),
    stale: allowedEntries.filter((entry) => !actual.has(entry.id)),
    duplicates: allowedEntries.filter(
      (entry, index) =>
        allowedEntries.findIndex((candidate) => candidate.id === entry.id) !==
        index,
    ),
  };
}

export function buildQualityAllowlist(gaps) {
  return {
    version: 1,
    gaps: gaps.map((entry) => ({
      id: entry.id,
      reason: "P0 domain contract migration pending",
    })),
  };
}
