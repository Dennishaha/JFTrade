function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

// Metrics gained payload-size and retry/error dimensions after the original
// settings API shipped. Keep older snapshots readable at the wire boundary.
export function normalizeADKMetricsWire(value: unknown): unknown {
  if (!isRecord(value) || !isRecord(value.tools)) return value;
  const tools = value.tools;
  return {
    ...value,
    tools: {
      ...tools,
      outputBytesTotal: tools.outputBytesTotal === undefined ? 0 : tools.outputBytesTotal,
      outputBytesMax: tools.outputBytesMax === undefined ? 0 : tools.outputBytesMax,
      truncated: tools.truncated === undefined ? 0 : tools.truncated,
      errorCount: tools.errorCount === undefined ? 0 : tools.errorCount,
      retryableErrors: tools.retryableErrors === undefined ? 0 : tools.retryableErrors,
      byErrorCode: tools.byErrorCode === undefined ? {} : tools.byErrorCode,
    },
  };
}
