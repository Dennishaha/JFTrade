#!/usr/bin/env node

import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath, pathToFileURL } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, "..");
const inputPath = path.join(repoRoot, "docs/swagger/swagger.json");
const outputPath = path.join(repoRoot, "apps/web/src/generated/openapi.ts");

const httpMethods = ["get", "post", "put", "patch", "delete", "options", "head"];

function propertyKey(name) {
  return /^[A-Za-z_$][\w$]*$/.test(name) ? name : JSON.stringify(name);
}

function schemaRefName(ref) {
  const prefix = "#/definitions/";
  if (!ref.startsWith(prefix)) {
    return "unknown";
  }
  return ref.slice(prefix.length);
}

export function schemaToType(schema, context = "") {
  if (schema == null) {
    return "unknown";
  }
  const declaredTypes = Array.isArray(schema.type) ? schema.type : [];
  if (
    schema["x-nullable"] === true ||
    schema.nullable === true ||
    declaredTypes.includes("null")
  ) {
    const nonNullableSchema = { ...schema };
    delete nonNullableSchema["x-nullable"];
    delete nonNullableSchema.nullable;
    if (declaredTypes.length > 0) {
      nonNullableSchema.type = declaredTypes.filter((type) => type !== "null");
      if (nonNullableSchema.type.length === 1) {
        nonNullableSchema.type = nonNullableSchema.type[0];
      }
    }
    return `${schemaToType(nonNullableSchema, context)} | null`;
  }
  if (schema.$ref) {
    return `components["schemas"][${JSON.stringify(schemaRefName(schema.$ref))}]`;
  }
  if (Array.isArray(schema.allOf) && schema.allOf.length > 0) {
    return schema.allOf.map((entry, index) => schemaToType(entry, `${context}.allOf[${index}]`)).join(" & ");
  }
  if (schema.enum && Array.isArray(schema.enum)) {
    return schema.enum.map((value) => JSON.stringify(value)).join(" | ") || "never";
  }

  const type = Array.isArray(schema.type) ? schema.type[0] : schema.type;
  switch (type) {
    case "integer":
    case "number":
      return "number";
    case "string":
      return "string";
    case "boolean":
      return "boolean";
    case "array":
      return `Array<${schemaToType(schema.items, `${context}.items`)}>`;
    case "object":
    case undefined:
      return objectSchemaToType(schema, context);
    default:
      return "unknown";
  }
}

function objectSchemaToType(schema, context) {
  const properties = schema.properties ?? {};
  const required = new Set(schema.required ?? []);
  const entries = Object.entries(properties);
  const additional = schema.additionalProperties;

  if (entries.length === 0) {
    if (additional === true || additional == null) {
      return schema.type === "object" ? "Record<string, unknown>" : "unknown";
    }
    return `Record<string, ${schemaToType(additional, `${context}.additionalProperties`)}>`;
  }

  const lines = ["{"];
  for (const [name, propertySchema] of entries) {
    const optional = required.has(name) ? "" : "?";
    lines.push(`    ${propertyKey(name)}${optional}: ${schemaToType(propertySchema, `${context}.${name}`)};`);
  }
  if (additional != null && additional !== false) {
    const additionalType = additional === true ? "unknown" : schemaToType(additional, `${context}.additionalProperties`);
    lines.push(`    [key: string]: ${additionalType};`);
  }
  lines.push("  }");
  return lines.join("\n");
}

function parametersType(parameters, location) {
  const params = (parameters ?? []).filter((parameter) => parameter.in === location);
  if (params.length === 0) {
    return null;
  }
  const required = new Set(params.filter((parameter) => parameter.required).map((parameter) => parameter.name));
  const lines = ["{"];
  for (const parameter of params) {
    const schema = parameter.schema ?? parameter;
    const optional = required.has(parameter.name) ? "" : "?";
    lines.push(`        ${propertyKey(parameter.name)}${optional}: ${schemaToType(schema, `${location}.${parameter.name}`)};`);
  }
  lines.push("      }");
  return lines.join("\n");
}

function mergeParameters(pathParameters, operationParameters) {
  const merged = [...(pathParameters ?? [])];
  for (const parameter of operationParameters ?? []) {
    const index = merged.findIndex(
      (candidate) =>
        candidate.in === parameter.in && candidate.name === parameter.name,
    );
    if (index === -1) {
      merged.push(parameter);
    } else {
      merged[index] = parameter;
    }
  }
  return merged;
}

function formDataBodyType(parameters) {
  const formParameters = (parameters ?? []).filter(
    (parameter) => parameter.in === "formData",
  );
  if (formParameters.length === 0) {
    return null;
  }
  const required = new Set(
    formParameters
      .filter((parameter) => parameter.required)
      .map((parameter) => parameter.name),
  );
  const lines = ["{"];
  for (const parameter of formParameters) {
    const optional = required.has(parameter.name) ? "" : "?";
    lines.push(
      `            ${propertyKey(parameter.name)}${optional}: ${schemaToType(parameter, `formData.${parameter.name}`)};`,
    );
  }
  lines.push("          }");
  return lines.join("\n");
}

function requestBody(parameters) {
  const body = (parameters ?? []).find((parameter) => parameter.in === "body");
  if (body != null) {
    return {
      required: body.required === true,
      type: schemaToType(body.schema, `body.${body.name ?? "request"}`),
    };
  }
  const formData = formDataBodyType(parameters);
  if (formData == null) {
    return null;
  }
  return {
    required: (parameters ?? []).some(
      (parameter) =>
        parameter.in === "formData" && parameter.required === true,
    ),
    type: formData,
  };
}

function responseType(response) {
  return schemaToType(response?.schema);
}

function mediaTypes(operationValues, globalValues) {
  const values = operationValues ?? globalValues ?? [];
  return [
    ...new Set(
      values.filter(
        (value) => typeof value === "string" && value.length > 0,
      ),
    ),
  ];
}

function contentType(media, value, indent) {
  const lines = [`${indent}content: {`];
  for (const mediaType of media) {
    lines.push(`${indent}  ${JSON.stringify(mediaType)}: ${value};`);
  }
  lines.push(`${indent}};`);
  return lines;
}

function operationToType(spec, pathItem, operation) {
  const lines = ["{"];
  const parameters = mergeParameters(pathItem.parameters, operation.parameters);
  const pathParameters = parametersType(parameters, "path");
  const queryParameters = parametersType(parameters, "query");
  const headerParameters = parametersType(parameters, "header");
  const body = requestBody(parameters);
  const consumes = mediaTypes(operation.consumes, spec.consumes);
  const produces = mediaTypes(operation.produces, spec.produces);
  const errorProduces = mediaTypes(operation["x-error-produces"], []);

  if (pathParameters != null || queryParameters != null || headerParameters != null) {
    lines.push("      parameters: {");
    if (pathParameters != null) {
      lines.push("        path: " + pathParameters + ";");
    }
    if (queryParameters != null) {
      lines.push("        query: " + queryParameters + ";");
    }
    if (headerParameters != null) {
      lines.push("        header: " + headerParameters + ";");
    }
    lines.push("      };");
  }
  if (body != null) {
    lines.push(`      requestBody${body.required ? "" : "?"}: {`);
    lines.push(...contentType(consumes, body.type, "        "));
    lines.push("      };");
  }
  lines.push("      responses: {");
  for (const [status, response] of Object.entries(operation.responses ?? {})) {
    const responseProduces =
      /^[45]\d\d$/.test(status) && errorProduces.length > 0
        ? errorProduces
        : produces;
    lines.push(`        ${JSON.stringify(status)}: {`);
    lines.push(`          description: ${JSON.stringify(response.description ?? "")};`);
    if (responseProduces.length > 0 && response.schema != null) {
      lines.push(...contentType(responseProduces, responseType(response), "          "));
    }
    lines.push("        };");
  }
  lines.push("      };");
  lines.push("    }");
  return lines.join("\n");
}

export function generateAPITypes(spec) {
  const definitions = spec.definitions ?? {};
  const paths = spec.paths ?? {};
  const lines = [
    "/* eslint-disable */",
    "/* tslint:disable */",
    "// This file is generated by scripts/generate-api-types.mjs.",
    "// Do not edit it directly.",
    "",
    "export interface components {",
    "  schemas: {",
  ];

  for (const [name, schema] of Object.entries(definitions).sort(([a], [b]) =>
    a.localeCompare(b),
  )) {
    lines.push(`    ${JSON.stringify(name)}: ${schemaToType(schema, `definitions.${name}`)};`);
  }
  lines.push("  };");
  lines.push("}");
  lines.push("");
  lines.push("export interface paths {");

  for (const [route, pathItem] of Object.entries(paths).sort(([a], [b]) =>
    a.localeCompare(b),
  )) {
    lines.push(`  ${JSON.stringify(route)}: {`);
    for (const method of httpMethods) {
      if (pathItem[method] == null) {
        continue;
      }
      lines.push(`    ${method}: ${operationToType(spec, pathItem, pathItem[method])};`);
    }
    lines.push("  };");
  }

  lines.push("}");
  lines.push("");

  return `${lines.join("\n")}\n`;
}

async function main() {
  const raw = await readFile(inputPath, "utf8");
  const spec = JSON.parse(raw);
  const output = generateAPITypes(spec);

  await mkdir(path.dirname(outputPath), { recursive: true });
  await writeFile(outputPath, output);
  console.log(
    `Generated ${path.relative(repoRoot, outputPath)} from ${path.relative(repoRoot, inputPath)}`,
  );
}

const invokedPath =
  process.argv[1] == null
    ? null
    : pathToFileURL(path.resolve(process.argv[1])).href;
if (invokedPath === import.meta.url) {
  main().catch((error) => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  });
}
