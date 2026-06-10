import fs from "node:fs/promises";
import path from "node:path";
import ts from "typescript";

const rootDir = process.cwd();
const generatedDir = path.join(rootDir, "docs", "reference", "generated");
const swaggerPath = path.join(rootDir, "docs", "swagger", "swagger.json");
const contractsPath = path.join(rootDir, "apps", "web", "src", "contracts", "index.ts");

await fs.mkdir(generatedDir, { recursive: true });
await Promise.all([generateApiDocs(), generateTypeDocs()]);

async function generateApiDocs() {
  const swagger = JSON.parse(await fs.readFile(swaggerPath, "utf8"));
  const operationsByTag = new Map();

  for (const [routePath, pathItem] of Object.entries(swagger.paths ?? {})) {
    for (const method of ["get", "post", "put", "patch", "delete", "head", "options"]) {
      const operation = pathItem?.[method];
      if (!operation) {
        continue;
      }
      const tags = Array.isArray(operation.tags) && operation.tags.length > 0 ? operation.tags : ["default"];
      for (const tag of tags) {
        if (!operationsByTag.has(tag)) {
          operationsByTag.set(tag, []);
        }
        operationsByTag.get(tag).push({ method: method.toUpperCase(), path: routePath, operation });
      }
    }
  }

  const lines = [
    "# HTTP API",
    "",
    "> 自动生成，请勿手改。来源：`docs/swagger/swagger.json`。",
    "",
  ];

  for (const tag of [...operationsByTag.keys()].sort()) {
    lines.push(`## ${escapeMarkdown(tag)}`, "");
    const operations = operationsByTag
      .get(tag)
      .sort((a, b) => `${a.path} ${a.method}`.localeCompare(`${b.path} ${b.method}`));

    for (const { method, path: routePath, operation } of operations) {
      lines.push(`### \`${method} ${routePath}\``, "");
      if (operation.summary) {
        lines.push(`**Summary:** ${escapeMarkdown(operation.summary)}`, "");
      }
      if (operation.description) {
        lines.push(operation.description.trim(), "");
      }

      const parameters = collectParameters(operation);
      if (parameters.length > 0) {
        lines.push("| Name | In | Required | Type | Description |", "| --- | --- | --- | --- | --- |");
        for (const parameter of parameters) {
          lines.push(
            `| \`${escapeTable(parameter.name ?? "")}\` | ${escapeTable(parameter.in ?? "")} | ${parameter.required ? "yes" : "no"} | ${escapeTable(formatSchema(parameter.schema ?? parameter))} | ${escapeTable(parameter.description ?? "")} |`,
          );
        }
        lines.push("");
      }

      const responses = operation.responses ?? {};
      if (Object.keys(responses).length > 0) {
        lines.push("| Response | Schema | Description |", "| --- | --- | --- |");
        for (const [status, response] of Object.entries(responses)) {
          lines.push(
            `| \`${escapeTable(status)}\` | ${escapeTable(formatSchema(response.schema))} | ${escapeTable(response.description ?? "")} |`,
          );
        }
        lines.push("");
      }
    }
  }

  await fs.writeFile(path.join(generatedDir, "api.md"), `${lines.join("\n").trimEnd()}\n`, "utf8");
}

async function generateTypeDocs() {
  const sourceText = await fs.readFile(contractsPath, "utf8");
  const sourceFile = ts.createSourceFile(contractsPath, sourceText, ts.ScriptTarget.Latest, true);
  const exportedDeclarations = [];

  for (const statement of sourceFile.statements) {
    if (!hasExportModifier(statement)) {
      continue;
    }
    if (ts.isInterfaceDeclaration(statement) || ts.isTypeAliasDeclaration(statement) || ts.isEnumDeclaration(statement)) {
      exportedDeclarations.push(statement);
    }
  }

  const lines = [
    "# 数据类型",
    "",
    "> 自动生成，请勿手改。来源：`apps/web/src/contracts/index.ts`。",
    "",
  ];

  for (const declaration of exportedDeclarations) {
    const name = declaration.name?.text ?? "anonymous";
    lines.push(`## \`${name}\``, "", "```ts", declaration.getText(sourceFile), "```", "");
  }

  await fs.writeFile(path.join(generatedDir, "types.md"), `${lines.join("\n").trimEnd()}\n`, "utf8");
}

function collectParameters(operation) {
  const parameters = [];
  if (Array.isArray(operation.parameters)) {
    parameters.push(...operation.parameters);
  }
  if (operation.requestBody) {
    parameters.push({
      name: "requestBody",
      in: "body",
      required: Boolean(operation.requestBody.required),
      schema: operation.requestBody.content?.["application/json"]?.schema ?? operation.requestBody.schema,
      description: operation.requestBody.description ?? "",
    });
  }
  return parameters;
}

function formatSchema(schema) {
  if (!schema) {
    return "";
  }
  if (schema.$ref) {
    return schema.$ref.replace("#/definitions/", "").replace("#/components/schemas/", "");
  }
  if (schema.type === "array") {
    return `${formatSchema(schema.items) || "unknown"}[]`;
  }
  if (schema.type) {
    return schema.format ? `${schema.type}(${schema.format})` : schema.type;
  }
  return "";
}

function hasExportModifier(node) {
  return Boolean(node.modifiers?.some((modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword));
}

function escapeMarkdown(value) {
  return String(value).replaceAll("|", "\\|");
}

function escapeTable(value) {
  return escapeMarkdown(value).replaceAll("\n", "<br>");
}
