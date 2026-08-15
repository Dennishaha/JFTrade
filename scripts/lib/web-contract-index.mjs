import ts from "./typescript6.mjs";

export function contractIndexViolations(source, fileName = "index.ts") {
  const sourceFile = ts.createSourceFile(
    fileName,
    source,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TS,
  );
  const violations = sourceFile.parseDiagnostics.map((diagnostic) => ({
    line: lineForPosition(sourceFile, diagnostic.start ?? 0),
    message: ts.flattenDiagnosticMessageText(diagnostic.messageText, "\n"),
  }));

  for (const statement of sourceFile.statements) {
    if (
      ts.isExportDeclaration(statement) &&
      statement.moduleSpecifier != null &&
      ts.isStringLiteral(statement.moduleSpecifier)
    ) {
      continue;
    }
    violations.push({
      line: lineForPosition(sourceFile, statement.getStart(sourceFile)),
      message:
        "contracts/index.ts may only contain re-exports with an explicit module source",
    });
  }
  return violations;
}

function lineForPosition(sourceFile, position) {
  return sourceFile.getLineAndCharacterOfPosition(position).line + 1;
}
