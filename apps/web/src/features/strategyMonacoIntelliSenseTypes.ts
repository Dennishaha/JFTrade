export interface MonacoExtraLibDefinition {
  filePath: string;
  content: string;
}

export interface MonacoCompletionDefinition {
  label: string;
  insertText: string;
  detail: string;
  documentation: string;
  kind?: "function" | "snippet" | "interface" | "variable";
  insertTextRule?: "plain" | "snippet";
  sortText?: string;
}

export interface MonacoHoverDefinition {
  target: string;
  signature: string;
  documentation: string;
}