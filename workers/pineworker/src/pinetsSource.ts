export function normalizePineSourceForPineTS(source: string): string {
  let output = "";
  let index = 0;
  let state: "code" | "line_comment" | "block_comment" | "single_quote" | "double_quote" = "code";
  while (index < source.length) {
    const char = source[index];
    const next = source[index + 1];
    if (state === "code") {
      if (char === "/" && next === "/") {
        output += "//";
        index += 2;
        state = "line_comment";
        continue;
      }
      if (char === "/" && next === "*") {
        output += "/*";
        index += 2;
        state = "block_comment";
        continue;
      }
      if (char === "'") {
        output += char;
        index++;
        state = "single_quote";
        continue;
      }
      if (char === "\"") {
        output += char;
        index++;
        state = "double_quote";
        continue;
      }
      if (source.startsWith("timenow", index) && !isIdentifierChar(source[index - 1]) && !isIdentifierChar(source[index + "timenow".length])) {
        output += "time_close";
        index += "timenow".length;
        continue;
      }
      output += char;
      index++;
      continue;
    }
    output += char;
    index++;
    if (state === "line_comment" && char === "\n") {
      state = "code";
    } else if (state === "block_comment" && char === "*" && next === "/") {
      output += next;
      index++;
      state = "code";
    } else if (state === "single_quote" && char === "'" && source[index - 2] !== "\\") {
      state = "code";
    } else if (state === "double_quote" && char === "\"" && source[index - 2] !== "\\") {
      state = "code";
    }
  }
  return output;
}

function isIdentifierChar(value: string | undefined): boolean {
  return value !== undefined && /[A-Za-z0-9_]/.test(value);
}



