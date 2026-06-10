import type { ADKSessionContextSnapshot } from "@/contracts";

import { fetchEnvelope, fetchEnvelopeWithInit } from "./apiClient";

export async function fetchADKSessionContext(
  sessionId: string,
): Promise<ADKSessionContextSnapshot> {
  return fetchEnvelope<ADKSessionContextSnapshot>(
    `/api/v1/adk/sessions/${encodeURIComponent(sessionId)}/context`,
  );
}

export async function compactADKSessionContext(
  sessionId: string,
  mode: "normal" | "aggressive",
): Promise<ADKSessionContextSnapshot> {
  return fetchEnvelopeWithInit<ADKSessionContextSnapshot>(
    `/api/v1/adk/sessions/${encodeURIComponent(sessionId)}/context/compact`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ mode }),
    },
  );
}
