import type { PineSourceStructureKind } from "./pineSourceStructureIndex";
import { splitCallArgs, trimDetail, unquote } from "./pineSourceStructureText";

export interface IndexedRawMatch {
  kind: PineSourceStructureKind;
  label: string;
  detail: string;
}

export function classifyIndexedRawDefinition(text: string): IndexedRawMatch | null {
  const importMatch = text.match(/^import\s+(.+?)(?:\s+as\s+([A-Za-z_][A-Za-z0-9_]*))?$/);
  if (importMatch !== null) {
    return {
      kind: "library",
      label: importMatch[2] === undefined ? "导入库" : `导入库 ${importMatch[2]}`,
      detail: importMatch[1]?.trim() ?? text,
    };
  }

  const libraryMatch = text.match(/^library\s*\((.*)\)$/);
  if (libraryMatch !== null) {
    const args = splitCallArgs(libraryMatch[1] ?? "");
    return {
      kind: "library",
      label: "库声明",
      detail: unquote(args[0] ?? text),
    };
  }

  const indicatorMatch = text.match(/^indicator\s*\((.*)\)$/);
  if (indicatorMatch !== null) {
    const args = splitCallArgs(indicatorMatch[1] ?? "");
    return {
      kind: "declaration",
      label: "指标声明",
      detail: unquote(args[0] ?? text),
    };
  }

  const typeMatch = text.match(/^(export\s+)?type\s+([A-Za-z_][A-Za-z0-9_]*)\b(.*)$/);
  if (typeMatch !== null) {
    return {
      kind: "type",
      label: `${typeMatch[1] === undefined ? "类型定义" : "导出类型"} ${typeMatch[2] ?? "对象"}`,
      detail: (typeMatch[3] ?? "").trim() || text,
    };
  }

  const methodMatch = text.match(/^(export\s+)?method\s+([A-Za-z_][A-Za-z0-9_]*)\s*\((.*)\)\s*=>\s*(.*)$/);
  if (methodMatch !== null) {
    return {
      kind: "method",
      label: `${methodMatch[1] === undefined ? "方法" : "导出方法"} ${methodMatch[2] ?? "method"}`,
      detail: trimDetail(methodMatch[3] ?? methodMatch[4] ?? "", text),
    };
  }

  return null;
}

export function classifyIndexedRawCall(text: string): IndexedRawMatch | null {
  const expression = stripAssignmentPrefix(text);
  const call = expression.match(/^([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*)\s*(?:<[^>]+>)?\s*\(/)?.[1] ?? "";
  if (call === "") {
    return null;
  }

  if (isRawCollectionCall(call)) {
    return {
      kind: "collection",
      label: rawCollectionLabel(call),
      detail: text,
    };
  }
  if (isRawVisualCall(call)) {
    return {
      kind: "visual",
      label: rawVisualLabel(call),
      detail: text,
    };
  }
  if (call.startsWith("request.")) {
    return {
      kind: "request",
      label: rawRequestLabel(call),
      detail: text,
    };
  }
  if (call === "alert") {
    return {
      kind: "alert",
      label: "即时提醒",
      detail: text,
    };
  }
  if (call === "runtime.error") {
    return {
      kind: "runtime",
      label: "运行时错误",
      detail: text,
    };
  }
  if (call === "strategy.close_all") {
    return {
      kind: "order",
      label: "全部平仓",
      detail: text,
    };
  }
  if (call === "strategy.cancel" || call === "strategy.cancel_all") {
    return {
      kind: "order",
      label: "撤销订单",
      detail: text,
    };
  }
  if (call.startsWith("strategy.risk.")) {
    return {
      kind: "order",
      label: "风控声明",
      detail: text,
    };
  }
  return null;
}

export function classifyIndexedRawObject(text: string): IndexedRawMatch | null {
  const fieldUpdate = text.match(/^([A-Za-z_][A-Za-z0-9_]*(?:\[[^\]]+\])?(?:\.[A-Za-z_][A-Za-z0-9_]*)+)\s*:=\s*(.*)$/);
  if (fieldUpdate !== null && isObjectPath(fieldUpdate[1] ?? "")) {
    return {
      kind: "object",
      label: `对象字段更新 ${fieldUpdate[1] ?? "字段"}`,
      detail: fieldUpdate[2] ?? "",
    };
  }

  const expression = stripAssignmentPrefix(text);
  const constructor = expression.match(/^([A-Z][A-Za-z0-9_]*)\.new\s*(?:<[^>]+>)?\s*\(/);
  if (constructor !== null) {
    return {
      kind: "object",
      label: `对象构造 ${constructor[1] ?? "对象"}`,
      detail: text,
    };
  }

  const receiver = objectReceiver(expression);
  if (receiver === null || isKnownNamespace(receiver)) {
    return null;
  }
  if (/^[A-Za-z_][A-Za-z0-9_]*\[[^\]]+\]\.[A-Za-z_][A-Za-z0-9_]*(?:\b|$)/.test(expression)) {
    return {
      kind: "object",
      label: `对象历史读取 ${receiver}`,
      detail: text,
    };
  }
  if (/\.[A-Za-z_][A-Za-z0-9_]*\s*(?:<[^>]+>)?\s*\(/.test(expression) || /\)\s*\.[A-Za-z_][A-Za-z0-9_]*\s*\(/.test(expression)) {
    return {
      kind: "object",
      label: `对象方法 ${receiver}`,
      detail: text,
    };
  }
  if (/^[A-Za-z_][A-Za-z0-9_]*(?:\[[^\]]+\])?\.[A-Za-z_][A-Za-z0-9_]*/.test(expression)) {
    return {
      kind: "object",
      label: `对象字段读取 ${receiver}`,
      detail: text,
    };
  }
  return null;
}

export function classifyIndexedRawDeclaration(text: string): IndexedRawMatch | null {
  const tuple = text.match(/^\[([^\]]+)\]\s*(?::=|=)\s*(.*)$/);
  if (tuple !== null) {
    return {
      kind: "declaration",
      label: "Tuple 解构",
      detail: `${tuple[1]?.trim() ?? ""} = ${tuple[2]?.trim() ?? ""}`,
    };
  }

  const reassignment = text.match(/^([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*)\s*:=\s*(.*)$/);
  if (reassignment !== null) {
    return {
      kind: "assignment",
      label: `重赋值 ${reassignment[1] ?? "变量"}`,
      detail: reassignment[2] ?? text,
    };
  }

  const typedDeclaration = text.match(/^(?:(varip|const)\s+|(var)\s+)?([A-Za-z_][A-Za-z0-9_.]*(?:<[^>]+>)?(?:\[\])?)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$/);
  if (typedDeclaration !== null) {
    const modifier = typedDeclaration[1] ?? typedDeclaration[2] ?? "";
    const typeName = typedDeclaration[3] ?? "";
    const name = typedDeclaration[4] ?? "变量";
    const expression = typedDeclaration[5] ?? "";
    if (modifier === "var" && !typeName.includes(".") && typeName[0] === typeName[0]?.toLowerCase()) {
      return {
        kind: "declaration",
        label: `类型状态变量 ${name}`,
        detail: `${typeName} = ${expression}`,
      };
    }
    return {
      kind: "declaration",
      label: declarationLabel(modifier, name),
      detail: `${typeName} = ${expression}`,
    };
  }

  const modifiedDeclaration = text.match(/^(varip|const)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$/);
  if (modifiedDeclaration !== null) {
    return {
      kind: "declaration",
      label: declarationLabel(modifiedDeclaration[1] ?? "", modifiedDeclaration[2] ?? "变量"),
      detail: modifiedDeclaration[3] ?? "",
    };
  }

  const fieldDeclaration = text.match(/^([A-Za-z_][A-Za-z0-9_.]*(?:<[^>]+>)?(?:\[\])?)\s+([A-Za-z_][A-Za-z0-9_]*)$/);
  if (fieldDeclaration !== null) {
    return {
      kind: "declaration",
      label: `字段声明 ${fieldDeclaration[2] ?? "字段"}`,
      detail: fieldDeclaration[1] ?? text,
    };
  }

  return null;
}

function stripAssignmentPrefix(text: string): string {
  return text.replace(
    /^(?:\[[^\]]+\]|(?:var\s+|varip\s+|const\s+)?(?:[A-Za-z_][A-Za-z0-9_.]*(?:<[^>]+>)?(?:\[\])?\s+)?[A-Za-z_][A-Za-z0-9_]*(?:\s*:\s*[A-Za-z_][A-Za-z0-9_.<>\[\], ]+)?)\s*(?::=|=)\s*/,
    "",
  ).trim();
}

function objectReceiver(expression: string): string | null {
  return expression.match(/^([A-Za-z_][A-Za-z0-9_]*)(?:\[[^\]]+\])?\./)?.[1] ?? null;
}

function isObjectPath(value: string): boolean {
  const receiver = objectReceiver(value);
  return receiver !== null && !isKnownNamespace(receiver);
}

function isKnownNamespace(value: string): boolean {
  return [
    "array",
    "barmerge",
    "color",
    "dayofweek",
    "hline",
    "log",
    "map",
    "math",
    "matrix",
    "month",
    "order",
    "plot",
    "position",
    "request",
    "runtime",
    "session",
    "shape",
    "size",
    "str",
    "strategy",
    "syminfo",
    "ta",
    "table",
    "text",
    "timeframe",
    "xloc",
    "yloc",
  ].includes(value);
}

function declarationLabel(modifier: string, name: string): string {
  if (modifier === "varip") return `Bar 内持久变量 ${name}`;
  if (modifier === "const") return `常量声明 ${name}`;
  if (modifier === "var") return `类型状态变量 ${name}`;
  return `类型声明 ${name}`;
}

function isRawVisualCall(call: string): boolean {
  return call === "plotshape" ||
    call === "plotchar" ||
    call === "hline" ||
    call === "fill" ||
    call === "bgcolor" ||
    call === "barcolor" ||
    call.startsWith("label.") ||
    call.startsWith("line.") ||
    call.startsWith("box.") ||
    call.startsWith("table.");
}

function isRawCollectionCall(call: string): boolean {
  return call.startsWith("array.") || call.startsWith("map.") || call.startsWith("matrix.");
}

function rawVisualLabel(call: string): string {
  if (call === "plotshape") return "形状绘图";
  if (call === "plotchar") return "字符绘图";
  if (call === "hline") return "水平线";
  if (call === "fill") return "填充区域";
  if (call === "bgcolor") return "背景着色";
  if (call === "barcolor") return "K 线着色";
  if (call.startsWith("label.")) return "标签绘制";
  if (call.startsWith("line.")) return "线段绘制";
  if (call.startsWith("box.")) return "矩形绘制";
  if (call.startsWith("table.")) return "表格绘制";
  return "视觉输出";
}

function rawCollectionLabel(call: string): string {
  if (call.startsWith("array.")) return "数组操作";
  if (call.startsWith("map.")) return "Map 操作";
  if (call.startsWith("matrix.")) return "矩阵操作";
  return "集合操作";
}

function rawRequestLabel(call: string): string {
  if (call === "request.security") return "跨周期请求";
  if (call === "request.security_lower_tf") return "低周期请求";
  if (call === "request.currency_rate") return "汇率请求";
  if (call === "request.dividends") return "分红请求";
  if (call === "request.splits") return "拆股请求";
  if (call === "request.earnings") return "财报请求";
  return "数据请求";
}
