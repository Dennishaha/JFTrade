import {
  type ApiErrorEnvelope,
  type ApiSuccessEnvelope,
} from "@/contracts";

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
  let body: ApiSuccessEnvelope<T> | ApiErrorEnvelope | null = null;
  let rawBody = "";

  if (typeof response.text === "function") {
    rawBody = await response.text();
  } else if (typeof response.json === "function") {
    body = (await response.json()) as ApiSuccessEnvelope<T> | ApiErrorEnvelope;
  }

  if (body == null && rawBody.trim() !== "") {
    try {
      body = JSON.parse(rawBody) as ApiSuccessEnvelope<T> | ApiErrorEnvelope;
    } catch {
      if (!response.ok) {
        throw new Error(
          `${response.status} ${response.statusText}: ${rawBody.trim()}`,
        );
      }
      throw new Error("API response is not valid JSON");
    }
  }

  if (!response.ok) {
    const message =
      body != null && !body.ok
        ? body.error.message
        : `${response.status} ${response.statusText}`;
    throw new Error(message);
  }

  if (body == null) {
    throw new Error("API response body is empty");
  }

  if (!body.ok) {
    throw new Error(body.error.message || "Unknown API error");
  }

  return body.data;
}

export async function administratorSession(): Promise<AdministratorSession> {
  const response = await fetch(buildApiUrl("/api/v1/auth/session"), {
    credentials: "include",
  });
  return parseEnvelope<AdministratorSession>(response);
}

export async function administratorLogin(
  key: string,
): Promise<AdministratorSession> {
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
