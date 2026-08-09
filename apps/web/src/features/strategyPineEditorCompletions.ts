import type {
  MonacoCompletionDefinition,
  MonacoExtraLibDefinition,
} from "./strategyMonacoIntelliSenseTypes";
import { strategyPineEditorCompletionCatalogPart1 } from "./strategyPineEditorCompletionCatalogPart1";
import { strategyPineEditorCompletionCatalogPart2 } from "./strategyPineEditorCompletionCatalogPart2";
import { strategyPineEditorCompletionCatalogPart3 } from "./strategyPineEditorCompletionCatalogPart3";

export const strategyPineEditorExtraLibs: MonacoExtraLibDefinition[] = [];

export const strategyPineEditorCompletions: MonacoCompletionDefinition[] = [
  ...strategyPineEditorCompletionCatalogPart1,
  ...strategyPineEditorCompletionCatalogPart2,
  ...strategyPineEditorCompletionCatalogPart3,
];

