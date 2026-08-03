import ts from "typescript";

import { schemaToType } from "../generate-api-types.mjs";

const printer = ts.createPrinter({ newLine: ts.NewLineKind.LineFeed });

// Classification manifests use POSIX separators so their keys stay stable
// across CI and local Windows runs, where path.relative() returns backslashes.
export function normalizeRelativePath(file) {
  return file.replaceAll("\\", "/");
}

export function generatedSchemaViolations(
  spec,
  generatedSource,
  fileName = "openapi.ts",
) {
  const sourceFile = parseSource(generatedSource, fileName);
  const violations = diagnosticsFor(sourceFile);
  const schemasNode = findComponentsSchemas(sourceFile);
  if (schemasNode == null) {
    return [
      ...violations,
      "generated TypeScript does not declare components.schemas",
    ];
  }

  const actual = new Map();
  for (const member of schemasNode.members) {
    if (!ts.isPropertySignature(member) || member.type == null) {
      violations.push("components.schemas may only contain typed properties");
      continue;
    }
    const name = propertyName(member.name);
    if (name == null) {
      violations.push("components.schemas contains an unsupported property name");
      continue;
    }
    actual.set(name, member.type);
  }

  const definitions = spec.definitions ?? {};
  const expectedNames = new Set(Object.keys(definitions));
  for (const name of [...actual.keys()].sort()) {
    if (!expectedNames.has(name)) {
      violations.push(`${name}: generated TypeScript schema is absent from Swagger`);
    }
  }

  for (const [name, schema] of Object.entries(definitions).sort(([a], [b]) =>
    a.localeCompare(b),
  )) {
    const actualType = actual.get(name);
    if (actualType == null) {
      violations.push(`${name}: Swagger schema is absent from generated TypeScript`);
      continue;
    }
    const expected = parsedType(schemaToType(schema, `definitions.${name}`));
    const actualText = canonicalType(actualType, sourceFile);
    const expectedText = canonicalType(expected.node, expected.sourceFile);
    if (actualText !== expectedText) {
      violations.push(
        `${name}: generated fields/types differ from Swagger (actual ${actualText}; expected ${expectedText})`,
      );
    }
  }
  return violations;
}

export function wireContractViolations({
  indexSource,
  wireSources,
  schemaNames,
  pathNames = new Set(),
}) {
  const violations = [];
  const indexFile = parseSource(indexSource, "contracts/index.ts");
  violations.push(...diagnosticsFor(indexFile));
  const expectedModules = new Set(
    [...wireSources.keys()]
      .map((file) => `./wire/${file.replace(/\.ts$/, "")}`)
      .sort(),
  );
  const exportedModules = new Set();

  for (const statement of indexFile.statements) {
    if (
      !ts.isExportDeclaration(statement) ||
      statement.moduleSpecifier == null ||
      !ts.isStringLiteral(statement.moduleSpecifier)
    ) {
      violations.push(
        `contracts/index.ts:${lineFor(indexFile, statement)} must contain wire re-exports only`,
      );
      continue;
    }
    const moduleName = statement.moduleSpecifier.text;
    if (!moduleName.startsWith("./wire/")) {
      violations.push(
        `contracts/index.ts:${lineFor(indexFile, statement)} re-exports non-wire module ${moduleName}`,
      );
    }
    exportedModules.add(moduleName);
  }

  for (const moduleName of expectedModules) {
    if (!exportedModules.has(moduleName)) {
      violations.push(`contracts/index.ts does not export ${moduleName}`);
    }
  }
  for (const moduleName of exportedModules) {
    if (!expectedModules.has(moduleName)) {
      violations.push(`contracts/index.ts exports unknown module ${moduleName}`);
    }
  }

  for (const [file, source] of [...wireSources.entries()].sort()) {
    const sourceFile = parseSource(source, file);
    violations.push(...diagnosticsFor(sourceFile));
    for (const statement of sourceFile.statements) {
      if (ts.isImportDeclaration(statement)) {
        if (
          !statement.importClause?.isTypeOnly ||
          !ts.isStringLiteral(statement.moduleSpecifier) ||
          statement.moduleSpecifier.text !== "@/generated/openapi"
        ) {
          violations.push(
            `${file}:${lineFor(sourceFile, statement)} may only type-import generated OpenAPI symbols`,
          );
        }
        continue;
      }
      if (
        !ts.isTypeAliasDeclaration(statement) ||
        !hasExportModifier(statement)
      ) {
        violations.push(
          `${file}:${lineFor(sourceFile, statement)} may only export direct wire type aliases`,
        );
        continue;
      }
      const target = generatedAliasTarget(statement.type);
      if (target == null) {
        violations.push(
          `${file}:${lineFor(sourceFile, statement)} ${statement.name.text} is not a direct schema or operation alias`,
        );
        continue;
      }
      if (target.kind === "schema" && !schemaNames.has(target.name)) {
        violations.push(
          `${file}:${lineFor(sourceFile, statement)} references unknown schema ${target.name}`,
        );
      }
      if (target.kind === "path" && !pathNames.has(target.name)) {
        violations.push(
          `${file}:${lineFor(sourceFile, statement)} references unknown operation path ${target.name}`,
        );
      }
    }
  }
  return violations;
}

export function viewModelClassificationViolations({
  classification,
  sources,
  adapterSources,
  testFiles,
}) {
  const normalizedClassification = new Map(
    Object.entries(classification).map(([file, entry]) => [
      normalizeRelativePath(file),
      entry,
    ]),
  );
  const normalizedSources = new Map(
    [...sources.entries()].map(([file, source]) => [
      normalizeRelativePath(file),
      source,
    ]),
  );
  const normalizedAdapterSources = new Map(
    [...adapterSources.entries()].map(([file, source]) => [
      normalizeRelativePath(file),
      source,
    ]),
  );
  const normalizedTestFiles = new Set(
    [...testFiles].map((file) => normalizeRelativePath(file)),
  );
  const violations = [];
  const classified = new Set(normalizedClassification.keys());
  for (const file of [...normalizedSources.keys()].sort()) {
    if (!classified.has(file)) {
      violations.push(`${file}: missing type classification`);
    }
  }
  for (const file of [...classified].sort()) {
    if (!normalizedSources.has(file)) {
      violations.push(`${file}: classification points to a missing source file`);
    }
  }

  for (const [file, entry] of [...normalizedClassification.entries()].sort()) {
    const source = normalizedSources.get(file);
    if (source == null) continue;
    const sourceFile = parseSource(source, file);
    violations.push(...diagnosticsFor(sourceFile));
    const declarations = exportedDeclarations(sourceFile);
    if (declarations.length === 0) {
      violations.push(`${file}: classified module has no exported declarations`);
    }
    if (!["normalized-api", "ui-view-model", "client-infrastructure"].includes(entry.kind)) {
      violations.push(`${file}: unsupported classification ${String(entry.kind)}`);
      continue;
    }
    if (entry.kind === "normalized-api") {
      if (!Array.isArray(entry.adapters) || entry.adapters.length === 0) {
        violations.push(`${file}: normalized API models require an explicit adapter`);
      }
      if (!Array.isArray(entry.tests) || entry.tests.length === 0) {
        violations.push(`${file}: normalized API models require boundary tests`);
      }
      for (const adapter of entry.adapters ?? []) {
        const adapterSource = normalizedAdapterSources.get(
          normalizeRelativePath(adapter),
        );
        if (adapterSource == null) {
          violations.push(`${file}: adapter ${adapter} does not exist`);
          continue;
        }
        if (!adapterSource.includes("@/types")) {
          violations.push(`${file}: adapter ${adapter} does not consume classified view models`);
        }
        if (!/\b(map|normalize|require|to[A-Z]\w*Wire)\w*\s*\(/.test(adapterSource)) {
          violations.push(`${file}: adapter ${adapter} has no explicit mapping boundary`);
        }
      }
      for (const testFile of entry.tests ?? []) {
        if (!normalizedTestFiles.has(normalizeRelativePath(testFile))) {
          violations.push(`${file}: boundary test ${testFile} does not exist`);
        }
      }
    }
  }
  return violations;
}

export function classifiedDeclarationCounts(classification, sources) {
  const counts = {
    "normalized-api": 0,
    "ui-view-model": 0,
    "client-infrastructure": 0,
  };
  const normalizedSources = new Map(
    [...sources.entries()].map(([file, source]) => [
      normalizeRelativePath(file),
      source,
    ]),
  );
  for (const [file, entry] of Object.entries(classification)) {
    const source = normalizedSources.get(normalizeRelativePath(file));
    if (source == null || !(entry.kind in counts)) continue;
    counts[entry.kind] += exportedDeclarations(parseSource(source, file)).length;
  }
  return counts;
}

function parseSource(source, fileName) {
  return ts.createSourceFile(
    fileName,
    source,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TS,
  );
}

function diagnosticsFor(sourceFile) {
  return sourceFile.parseDiagnostics.map(
    (diagnostic) =>
      `${sourceFile.fileName}:${lineForPosition(sourceFile, diagnostic.start ?? 0)} ${ts.flattenDiagnosticMessageText(diagnostic.messageText, "\n")}`,
  );
}

function findComponentsSchemas(sourceFile) {
  const components = sourceFile.statements.find(
    (statement) =>
      ts.isInterfaceDeclaration(statement) &&
      statement.name.text === "components",
  );
  if (components == null || !ts.isInterfaceDeclaration(components)) return null;
  const schemas = components.members.find(
    (member) =>
      ts.isPropertySignature(member) && propertyName(member.name) === "schemas",
  );
  return schemas != null &&
    ts.isPropertySignature(schemas) &&
    schemas.type != null &&
    ts.isTypeLiteralNode(schemas.type)
    ? schemas.type
    : null;
}

function propertyName(name) {
  if (
    ts.isIdentifier(name) ||
    ts.isStringLiteral(name) ||
    ts.isNumericLiteral(name)
  ) {
    return name.text;
  }
  return null;
}

function parsedType(typeText) {
  const sourceFile = parseSource(`type Expected = ${typeText};`, "expected.ts");
  const declaration = sourceFile.statements[0];
  if (!ts.isTypeAliasDeclaration(declaration)) {
    throw new Error(`failed to parse expected type ${typeText}`);
  }
  return { node: declaration.type, sourceFile };
}

function canonicalType(node, sourceFile) {
  return printer
    .printNode(ts.EmitHint.Unspecified, node, sourceFile)
    .replace(/\s+/g, " ")
    .trim();
}

function hasExportModifier(node) {
  return node.modifiers?.some(
    (modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword,
  ) === true;
}

function generatedAliasTarget(typeNode) {
  const text = typeNode.getText().replace(/\s+/g, "");
  const schema =
    /^components\["schemas"\]\["([^"]+)"\](?:\[[^\]]+\])*$/.exec(text);
  if (schema != null) return { kind: "schema", name: schema[1] };
  const path = /^paths\["([^"]+)"\](?:\[[^\]]+\])+$/.exec(text);
  if (path != null) return { kind: "path", name: path[1] };
  return null;
}

function exportedDeclarations(sourceFile) {
  const declarations = [];
  for (const statement of sourceFile.statements) {
    if (!hasExportModifier(statement)) continue;
    if (ts.isVariableStatement(statement)) {
      declarations.push(
        ...statement.declarationList.declarations.map((declaration) =>
          propertyName(declaration.name) ?? declaration.name.getText(sourceFile),
        ),
      );
      continue;
    }
    if (
      ts.isInterfaceDeclaration(statement) ||
      ts.isTypeAliasDeclaration(statement) ||
      ts.isEnumDeclaration(statement) ||
      ts.isClassDeclaration(statement) ||
      ts.isFunctionDeclaration(statement)
    ) {
      declarations.push(statement.name?.text ?? "<anonymous>");
    }
  }
  return declarations;
}

function lineFor(sourceFile, node) {
  return lineForPosition(sourceFile, node.getStart(sourceFile));
}

function lineForPosition(sourceFile, position) {
  return sourceFile.getLineAndCharacterOfPosition(position).line + 1;
}
