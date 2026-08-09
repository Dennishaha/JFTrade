import { parseStrategyFlowNodeAnnotationLines } from "./strategyVisualBuilderShared";
import type { ParsedPineEntry } from "./strategyVisualBuilderPineParserTypes";

export function tokenizePine(script: string): ParsedPineEntry[] {
  const normalized = script.replace(/\r\n/g, "\n");
  const lines = normalized.split("\n");
  const entries: ParsedPineEntry[] = [];
  let offset = 0;
  let pendingComments: string[] = [];
  let pendingStart: number | null = null;

  for (let index = 0; index < lines.length; index += 1) {
    const raw = lines[index] ?? "";
    const start = offset;
    const end = start + raw.length;
    const trimmed = raw.trim();

    if (trimmed.startsWith("#") || trimmed.startsWith("// @jftradeFlow")) {
      if (pendingComments.length === 0) {
        pendingStart = start;
      }
      pendingComments.push(trimmed);
      offset = end + 1;
      continue;
    }

    if (trimmed.startsWith("//")) {
      offset = end + 1;
      continue;
    }

    if (trimmed !== "") {
      entries.push({
        lineNumber: index + 1,
        raw,
        trimmed,
        indent: raw.length - raw.trimStart().length,
        start,
        end,
        annotation: pendingComments.length > 0
          ? parseStrategyFlowNodeAnnotationLines(pendingComments)
          : null,
        annotationStart: pendingStart,
      });
      pendingComments = [];
      pendingStart = null;
    }

    offset = end + 1;
  }

  return entries;
}
