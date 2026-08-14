export type UnknownRecord = Record<string, unknown>;

export function labelFromKey(key: string): string {
  return key.replace(/([A-Z])/g, " $1").replace(/[_-]+/g, " ").replace(/\b\w/g, (char) => char.toUpperCase()).trim();
}

export function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
