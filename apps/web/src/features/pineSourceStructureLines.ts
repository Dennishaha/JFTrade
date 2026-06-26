export interface SourceLine {
  number: number;
  endNumber: number;
  startOffset: number;
  endOffset: number;
  indent: number;
  text: string;
  trimmed: string;
}

interface DelimiterState {
  depth: number;
  quote: string | null;
  escaped: boolean;
}

export function splitSourceLines(source: string): SourceLine[] {
  const lines: SourceLine[] = [];
  let offset = 0;
  const parts = source.split(/(\r?\n)/);
  for (let index = 0, lineNumber = 1; index < parts.length; index += 2, lineNumber += 1) {
    const text = parts[index] ?? "";
    const newline = parts[index + 1] ?? "";
    const indentText = text.match(/^\s*/)?.[0] ?? "";
    lines.push({
      number: lineNumber,
      endNumber: lineNumber,
      startOffset: offset,
      endOffset: offset + text.length,
      indent: Math.floor(indentText.replace(/\t/g, "    ").length / 4),
      text,
      trimmed: text.trim(),
    });
    offset += text.length + newline.length;
  }
  return lines;
}

export function readLogicalLine(lines: SourceLine[], startIndex: number): { line: SourceLine; nextIndex: number } {
  const first = lines[startIndex]!;
  let state = advanceDelimiterState(first.trimmed, emptyDelimiterState());
  if (!delimiterIsOpen(state)) {
    return { line: first, nextIndex: startIndex + 1 };
  }

  const rawParts = [first.text];
  const trimmedParts = [first.trimmed];
  let last = first;
  let index = startIndex + 1;
  while (index < lines.length && delimiterIsOpen(state)) {
    const line = lines[index]!;
    rawParts.push(line.text);
    trimmedParts.push(line.trimmed);
    state = advanceDelimiterState(line.trimmed, state);
    last = line;
    index += 1;
  }

  return {
    line: {
      number: first.number,
      endNumber: last.endNumber,
      startOffset: first.startOffset,
      endOffset: last.endOffset,
      indent: first.indent,
      text: rawParts.join("\n"),
      trimmed: normalizeLogicalLine(trimmedParts.join(" ")),
    },
    nextIndex: index,
  };
}

function emptyDelimiterState(): DelimiterState {
  return { depth: 0, quote: null, escaped: false };
}

function delimiterIsOpen(state: DelimiterState): boolean {
  return state.depth > 0 || state.quote !== null;
}

function advanceDelimiterState(text: string, previous: DelimiterState): DelimiterState {
  const state = { ...previous };
  for (let index = 0; index < text.length; index += 1) {
    const char = text[index]!;
    const next = text[index + 1] ?? "";
    if (state.quote !== null) {
      if (state.escaped) {
        state.escaped = false;
        continue;
      }
      if (char === "\\") {
        state.escaped = true;
        continue;
      }
      if (char === state.quote) {
        state.quote = null;
      }
      continue;
    }
    if (char === "/" && next === "/") {
      break;
    }
    if (char === "\"" || char === "'") {
      state.quote = char;
      continue;
    }
    if (char === "(" || char === "[" || char === "{") {
      state.depth += 1;
      continue;
    }
    if (char === ")" || char === "]" || char === "}") {
      state.depth = Math.max(0, state.depth - 1);
    }
  }
  return state;
}

function normalizeLogicalLine(text: string): string {
  return text.replace(/\s+/g, " ").trim();
}
