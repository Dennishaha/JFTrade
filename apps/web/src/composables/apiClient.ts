import {
  type ApiErrorEnvelope,
  type ApiSuccessEnvelope,
} from "@jftrade/ui-contracts";

import { buildRuntimeApiUrl } from "../runtimeConfig";

export function buildApiUrl(path: string): string {
  return buildRuntimeApiUrl(path);
}

export async function fetchEnvelope<T>(path: string): Promise<T> {
  const response = await fetch(buildApiUrl(path));
  const body = (await response.json()) as
    | ApiSuccessEnvelope<T>
    | ApiErrorEnvelope;

  if (!response.ok || !body.ok) {
    throw new Error(body.ok ? "未知 API 错误" : body.error.message);
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
    throw new Error(body.ok ? "未知 API 错误" : body.error.message);
  }

  return body.data;
}