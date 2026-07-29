import type { ADKSessionContextSnapshot } from "@/types";

import { apiGetPath, apiPostPath } from "@/composables/shared/apiClient";
import { requireADKContextSnapshot } from "@/composables/adk/adkApiMappers";

export async function fetchADKSessionContext(
  sessionId: string,
): Promise<ADKSessionContextSnapshot> {
  return requireADKContextSnapshot(
    await apiGetPath(
      "/api/v1/adk/sessions/{sessionId}/context",
      `/api/v1/adk/sessions/${encodeURIComponent(sessionId)}/context`,
    ),
  );
}

export async function compactADKSessionContext(
  sessionId: string,
  mode: "normal" | "aggressive",
): Promise<ADKSessionContextSnapshot> {
  return requireADKContextSnapshot(
    await apiPostPath(
      "/api/v1/adk/sessions/{sessionId}/context/compact",
      `/api/v1/adk/sessions/${encodeURIComponent(sessionId)}/context/compact`,
      { mode },
    ),
  );
}
