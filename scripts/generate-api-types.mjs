#!/usr/bin/env node

import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

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

function schemaToType(schema, context = "") {
  if (schema == null) {
    return "unknown";
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

function requestBodyType(parameters) {
  const body = (parameters ?? []).find((parameter) => parameter.in === "body");
  if (body == null) {
    return null;
  }
  return schemaToType(body.schema, `body.${body.name ?? "request"}`);
}

function responseType(response) {
  return schemaToType(response?.schema);
}

function operationToType(operation) {
  const lines = ["{"];
  const pathParameters = parametersType(operation.parameters, "path");
  const queryParameters = parametersType(operation.parameters, "query");
  const body = requestBodyType(operation.parameters);

  if (pathParameters != null || queryParameters != null) {
    lines.push("      parameters: {");
    if (pathParameters != null) {
      lines.push("        path: " + pathParameters + ";");
    }
    if (queryParameters != null) {
      lines.push("        query: " + queryParameters + ";");
    }
    lines.push("      };");
  }
  if (body != null) {
    lines.push("      requestBody: {");
    lines.push("        content: {");
    lines.push(`          "application/json": ${body};`);
    lines.push("        };");
    lines.push("      };");
  }
  lines.push("      responses: {");
  for (const [status, response] of Object.entries(operation.responses ?? {})) {
    lines.push(`        ${JSON.stringify(status)}: {`);
    lines.push(`          description: ${JSON.stringify(response.description ?? "")};`);
    lines.push("          content: {");
    lines.push(`            "application/json": ${responseType(response)};`);
    lines.push("          };");
    lines.push("        };");
  }
  lines.push("      };");
  lines.push("    }");
  return lines.join("\n");
}

async function main() {
  const raw = await readFile(inputPath, "utf8");
  const spec = JSON.parse(raw);
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

  for (const [name, schema] of Object.entries(definitions).sort(([a], [b]) => a.localeCompare(b))) {
    lines.push(`    ${JSON.stringify(name)}: ${schemaToType(schema, `definitions.${name}`)};`);
  }
  lines.push("  };");
  lines.push("}");
  lines.push("");
  lines.push("export interface paths {");

  for (const [route, pathItem] of Object.entries(paths).sort(([a], [b]) => a.localeCompare(b))) {
    lines.push(`  ${JSON.stringify(route)}: {`);
    for (const method of httpMethods) {
      if (pathItem[method] == null) {
        continue;
      }
      lines.push(`    ${method}: ${operationToType(pathItem[method])};`);
    }
    lines.push("  };");
  }

  lines.push("}");
  lines.push("");

  await mkdir(path.dirname(outputPath), { recursive: true });
  await writeFile(outputPath, `${lines.join("\n")}\n`);
  console.log(`Generated ${path.relative(repoRoot, outputPath)} from ${path.relative(repoRoot, inputPath)}`);
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
});
