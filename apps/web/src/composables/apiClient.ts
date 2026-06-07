import {
  type ApiErrorEnvelope,
  type ApiSuccessEnvelope,
} from "@jftrade/ui-contracts";

import { buildRuntimeApiUrl } from "../runtimeConfig";

export function buildApiUrl(path: string): string {
  return buildRuntimeApiUrl(path);
}

export interface AdministratorSession {
  authenticated: boolean;
  csrfToken?: string;
  expiresAt?: string;
}

let csrfToken = "";

export function setCSRFToken(value: string): void {
  csrfToken = value;
}

export function csrfHeaders(method = "POST"): Record<string, string> {
  return authHeaders(method);
}

function authHeaders(method = "GET"): Record<string, string> {
  if (csrfToken && !["GET", "HEAD", "OPTIONS"].includes(method.toUpperCase())) {
    return { "X-CSRF-Token": csrfToken };
  }
  return {};
}

async function parseEnvelope<T>(response: Response): Promise<T> {
  const body = (await response.json()) as ApiSuccessEnvelope<T> | ApiErrorEnvelope;
  if (!response.ok || !body.ok) {
    throw new Error(body.ok ? "未知 API 错误" : body.error.message);
  }
  return body.data;
}

export async function administratorSession(): Promise<AdministratorSession> {
  const response = await fetch(buildApiUrl("/api/v1/auth/session"), {
    credentials: "include",
  });
  return parseEnvelope<AdministratorSession>(response);
}

export async function administratorLogin(key: string): Promise<AdministratorSession> {
  const response = await fetch(buildApiUrl("/api/v1/auth/login"), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ key }),
  });
  return parseEnvelope<AdministratorSession>(response);
}

export async function fetchEnvelope<T>(path: string): Promise<T> {
  const response = await fetch(buildApiUrl(path), {
    credentials: "include",
    headers: authHeaders("GET"),
  });
  return parseEnvelope<T>(response);
}

export async function fetchEnvelopeWithInit<T>(
  path: string,
  init: RequestInit,
): Promise<T> {
  const headers = {
    ...authHeaders(init.method ?? "GET"),
    ...(init.headers as Record<string, string> | undefined),
  };
  const response = await fetch(buildApiUrl(path), {
    ...init,
    credentials: "include",
    headers,
  });
  return parseEnvelope<T>(response);
}
