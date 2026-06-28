import { createResponse } from "./helpers"

export function createStrategyPineAnalyzeResponse(payload: Record<string, unknown>) {
  const script = String(payload.script ?? "")
  const lines = script.split("\n")
  const typedCollectionMatch = (trimmed: string) =>
    trimmed.match(/^(?:(?:var|varip|const)\s+)?(array|map|matrix)(?:<([^>]+)>)?\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$/i)
  const assignmentName = (trimmed: string) =>
    typedCollectionMatch(trimmed)?.[3] ?? trimmed.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*=/)?.[1] ?? ""
  const declarations = lines.flatMap((line, index) => {
    const trimmed = line.trim()
    const typedCollection = typedCollectionMatch(trimmed)
    if (typedCollection !== null || trimmed.includes("array.new")) {
      const name = typedCollection?.[3] ?? assignmentName(trimmed)
      const namespace = typedCollection?.[1]?.toLowerCase() ?? "array"
      const typeArgs = typedCollection?.[2] ?? ""
      const callMatch = trimmed.match(/\b(array|map|matrix)\.([A-Za-z_][A-Za-z0-9_]*)(?:<[^>]+>)?\(/)
      return [{
        line: index + 1,
        kind: "collection",
        name,
        namespace,
        call: callMatch === null ? "" : `${callMatch[1]}.${callMatch[2]}`,
        typeArgs,
        executable: false,
      }]
    }
    if (trimmed.startsWith("type ")) {
      const fields = lines.slice(index + 1).flatMap((candidate) => {
        if (!/^\s+/.test(candidate)) {
          return []
        }
        const field = candidate.trim().match(/^([A-Za-z_][A-Za-z0-9_]*)\s+([A-Za-z_][A-Za-z0-9_]*)(?:\s*=\s*(.+))?$/)
        if (field === null) {
          return []
        }
        return [{
          type: field[1] ?? "",
          name: field[2] ?? "",
          default: field[3] ?? "",
        }]
      })
      return [{
        line: index + 1,
        kind: "type",
        name: trimmed.replace(/^type\s+/, "").trim(),
        fields,
        executable: false,
      }]
    }
    if (trimmed.startsWith("method ")) {
      const match = trimmed.match(/^method\s+([A-Za-z_][A-Za-z0-9_]*)\(([^)]*)\)/)
      const rawParams = match?.[2]?.split(",").map((param) => param.trim()).filter((param) => param !== "") ?? []
      const parameters = rawParams.map((param) => {
        const defaultValue = param.includes("=") ? param.replace(/^.*=\s*/, "").trim() : ""
        const [type = "", name = ""] = param.replace(/\s*=.*$/, "").trim().split(/\s+/)
        return { type, name, default: defaultValue }
      })
      return [{
        line: index + 1,
        kind: "method",
        name: match?.[1] ?? "",
        receiver: parameters[0],
        parameters,
        executable: false,
      }]
    }
    if (trimmed.startsWith("import ")) {
      const [importPath = "", alias = ""] = trimmed
        .replace(/^import\s+/, "")
        .trim()
        .split(/\s+as\s+/i)
      const version = importPath.split("/").at(-1) ?? ""
      return [{
        line: index + 1,
        kind: "import",
        name: importPath,
        alias,
        importPath,
        version,
        executable: false,
      }]
    }
    return []
  })
  const collectionTypes = new Map<string, string>()
  const collectionSignature = (namespace: string, operation: string) => {
    if (namespace === "array" && operation === "push") {
      return "array.push(id, value)"
    }
    if (namespace === "array" && operation === "get") {
      return "array.get(id, index)"
    }
    if (namespace === "map" && operation === "put") {
      return "map.put(id, key, value)"
    }
    if (namespace === "matrix" && operation === "set") {
      return "matrix.set(id, row, column, value)"
    }
    return ""
  }
  const collectionOperations = lines.flatMap((line, index) => {
    const trimmed = line.trim()
    const match = trimmed.match(/\b(array|map|matrix)\.([A-Za-z_][A-Za-z0-9_]*)(?:<[^>]+>)?\((.*)\)/)
    if (match !== null) {
      const namespace = match[1] ?? ""
      const operation = match[2] ?? ""
      const args = (match[3] ?? "")
        .split(",")
        .map((arg) => arg.trim())
        .filter((arg) => arg !== "")
      const target = operation.startsWith("new")
        ? assignmentName(trimmed)
        : args[0] ?? ""
      if (operation.startsWith("new") && target !== "") {
        collectionTypes.set(target.toLowerCase(), namespace)
      }
      return [{
        line: index + 1,
        namespace,
        operation,
        call: `${namespace}.${operation}`,
        signature: collectionSignature(namespace, operation),
        target,
        arguments: args,
        mutates: operation.startsWith("new") || ["push", "put", "set"].includes(operation),
        supported: true,
        executable: false,
      }]
    }
    const methodMatch = trimmed.match(/\b([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)\((.*)\)/)
    if (methodMatch === null) {
      return []
    }
    const target = methodMatch[1] ?? ""
    const namespace = collectionTypes.get(target.toLowerCase())
    if (namespace === undefined) {
      return []
    }
    const operation = methodMatch[2] ?? ""
    const args = [
      target,
      ...(methodMatch[3] ?? "")
        .split(",")
        .map((arg) => arg.trim())
        .filter((arg) => arg !== ""),
    ]
    return [{
      line: index + 1,
      namespace,
      operation,
      call: `${namespace}.${operation}`,
      signature: collectionSignature(namespace, operation),
      target,
      arguments: args,
      mutates: operation.startsWith("new") || ["push", "put", "set"].includes(operation),
      supported: true,
      executable: false,
    }]
  })
  const typeDeclarations = new Map(
    declarations
      .filter((declaration) => declaration.kind === "type" && declaration.name)
      .map((declaration) => [String(declaration.name).toLowerCase(), declaration]),
  )
  const methodDeclarations = new Map(
    declarations
      .filter((declaration) => declaration.kind === "method" && declaration.name && declaration.receiver?.type)
      .map((declaration) => [`${String(declaration.receiver?.type).toLowerCase()}.${String(declaration.name).toLowerCase()}`, declaration]),
  )
  const objectTypes = new Map<string, string>()
  const formatParameter = (parameter: { type?: string; name?: string; default?: string }) =>
    [
      parameter.type ?? "",
      parameter.name ?? "",
    ].filter((value) => value !== "").join(" ") +
    ((parameter.default ?? "") === "" ? "" : ` = ${parameter.default}`)
  const objectOperations = lines.flatMap((line, index) => {
    const trimmed = line.trim()
    const constructorMatch = trimmed.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*([A-Za-z_][A-Za-z0-9_]*)\.new\((.*)\)/)
    if (constructorMatch !== null) {
      const target = constructorMatch[1] ?? ""
      const type = constructorMatch[2] ?? ""
      const declaration = typeDeclarations.get(type.toLowerCase())
      if (declaration === undefined) {
        return []
      }
      objectTypes.set(target.toLowerCase(), type)
      const args = (constructorMatch[3] ?? "").split(",").map((arg) => arg.trim()).filter((arg) => arg !== "")
      return [{
        line: index + 1,
        kind: "constructor",
        type,
        call: `${type}.new`,
        signature: `${type}.new(${(declaration.fields ?? []).map(formatParameter).join(", ")})`,
        target,
        arguments: args,
        supported: true,
        executable: false,
      }]
    }
    const methodMatch = trimmed.match(/^[A-Za-z_][A-Za-z0-9_]*\s*=\s*([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)\((.*)\)/)
    if (methodMatch === null) {
      return []
    }
    const target = methodMatch[1] ?? ""
    const method = methodMatch[2] ?? ""
    const type = objectTypes.get(target.toLowerCase())
    if (type === undefined) {
      return []
    }
    const declaration = methodDeclarations.get(`${type.toLowerCase()}.${method.toLowerCase()}`)
    if (declaration === undefined) {
      return []
    }
    const args = (methodMatch[3] ?? "").split(",").map((arg) => arg.trim()).filter((arg) => arg !== "")
    return [{
      line: index + 1,
      kind: "method",
      type,
      method,
      call: `${target}.${method}`,
      signature: `${method}(${(declaration.parameters ?? []).map(formatParameter).join(", ")})`,
      target,
      arguments: args,
      supported: true,
      executable: false,
    }]
  })
  const visuals = lines.flatMap((line, index) => {
    const trimmed = line.trim()
    if (trimmed.startsWith("plot(")) {
      return [{
        line: index + 1,
        kind: "plot",
        call: "plot",
        target: "close",
        title: trimmed.includes("title=") ? "Close" : "",
        arguments: trimmed.includes("title=") ? ["close", "title=\"Close\""] : ["close"],
        namedArgs: trimmed.includes("title=") ? { title: "\"Close\"" } : {},
        text: trimmed,
      }]
    }
    const assignedVisual = trimmed.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(label|line|box|table)\.new\((.*)\)$/)
    if (assignedVisual === null) {
      return []
    }
    const variable = assignedVisual[1] ?? ""
    const namespace = assignedVisual[2] ?? ""
    const args = (assignedVisual[3] ?? "").split(",").map((arg) => arg.trim()).filter((arg) => arg !== "")
    return [{
      line: index + 1,
      kind: namespace === "table" ? "table" : "drawing",
      call: `${namespace}.new`,
      variable,
      target: namespace === "table" ? args[0] ?? "" : args[2] ?? "",
      title: namespace === "label" && args[2] !== undefined ? args[2].replace(/^"|"$/g, "") : "",
      arguments: args,
      text: trimmed,
    }]
  })
  const diagnostics = collectionOperations.length === 0
    ? []
    : [{
        severity: "error",
        code: "PINE_COLLECTION_UNSUPPORTED",
        message: "Pine collection namespaces array/matrix/map are not executable in this JFTrade Pine v6 version",
        line: collectionOperations[0]?.line ?? 1,
        column: 1,
        endLine: collectionOperations[0]?.line ?? 1,
        endColumn: 2,
      }]
  return createResponse({
    ok: diagnostics.length === 0,
    collectionOperations,
    diagnostics,
    declarations,
    objectOperations,
    visuals,
    features: ["metadata.version6", "metadata.strategy", "tooling.semantic_analyze_payload"],
  })
}
