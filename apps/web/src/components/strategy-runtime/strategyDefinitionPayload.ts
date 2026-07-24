import type {
  PineV6WorkflowDocument,
} from "@/contracts";
import type { components } from "@/generated/openapi";

import { PINE_WORKER_RUNTIME, PINE_V6_SOURCE_FORMAT } from "./strategyRuntimeIdentity";

export interface BuildPineStrategyDefinitionPayloadInput {
  id: string;
  name: string;
  version: string;
  description: string;
  script: string;
  visualModel: PineV6WorkflowDocument | null;
  createdAt: string;
  updatedAt: string;
}

export function buildPineStrategyDefinitionPayload(
  input: BuildPineStrategyDefinitionPayloadInput,
): components["schemas"]["strategy.StrategyDesignDefinition"] {
  return {
    id: input.id,
    name: input.name,
    version: input.version,
    description: input.description,
    runtime: PINE_WORKER_RUNTIME,
    sourceFormat: PINE_V6_SOURCE_FORMAT,
    script: input.script,
    ...(input.visualModel == null ? {} : { visualModel: input.visualModel }),
    createdAt: input.createdAt,
    updatedAt: input.updatedAt,
  };
}
