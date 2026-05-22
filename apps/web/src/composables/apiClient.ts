import {
  type ApiErrorEnvelope,
  type ApiSuccessEnvelope,
} from "@jftrade/ui-contracts";

const apiBaseUrl = (
  import.meta.env.VITE_API_BASE_URL as string | undefined
)?.replace(/\/$/, "");

export function buildApiUrl(path: string): string {
  return apiBaseUrl ? `${apiBaseUrl}${path}` : `http://127.0.0.1:3000${path}`;
}

export async function fetchEnvelope<T>(path: string): Promise<T> {
  const response = await fetch(buildApiUrl(path));
  const body = (await response.json()) as
    | ApiSuccessEnvelope<T>
    | ApiErrorEnvelope;

  if (!response.ok || !body.ok) {
    throw new Error(body.ok ? "Unknown API error" : body.error.message);
  }

  return body.data;
}

export async function fetchEnvelopeWithInit<T>(
  path: string,
  init: RequestInit,
): Promise<T> {
  const response = await fetch(buildApiUrl(path), init);
  const body = (await response.json()) as
    | ApiSuccessEnvelope<T>
    | ApiErrorEnvelope;

  if (!response.ok || !body.ok) {
    throw new Error(body.ok ? "Unknown API error" : body.error.message);
  }

  return body.data;
}