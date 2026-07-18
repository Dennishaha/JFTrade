import { type ApiErrorEnvelope, type ApiSuccessEnvelope } from "@/contracts";
import type { paths } from "@/generated/openapi";

import { buildRuntimeApiUrl, resolveDesktopApiToken } from "../runtimeConfig";

export function buildApiUrl(path: string): string {
  return buildRuntimeApiUrl(path);
}

export interface WebSession {
  authenticated: boolean;
  csrfToken?: string;
  expiresAt?: string;
}

export const WEB_AUTH_REQUIRED_EVENT = "jftrade:web-auth-required";

let csrfToken = "";

export class ApiClientError extends Error {
  readonly code: string;
  readonly status: number;
  readonly retryAfterMs: number | undefined;

  constructor(
    message: string,
    code: string,
    status: number,
    retryAfterMs?: number,
  ) {
    super(message);
    this.name = "ApiClientError";
    this.code = code;
    this.status = status;
    this.retryAfterMs = retryAfterMs;
  }
}

type HttpMethod = "get" | "post" | "put" | "patch" | "delete";
type ApiPath = keyof paths & string;
type PathWithMethod<TMethod extends HttpMethod> = {
  [TPath in ApiPath]: TMethod extends keyof paths[TPath] ? TPath : never;
}[ApiPath];
type OperationFor<
  TPath extends ApiPath,
  TMethod extends HttpMethod,
> = TMethod extends keyof paths[TPath] ? paths[TPath][TMethod] : never;
type JsonRequestBody<TPath extends ApiPath, TMethod extends HttpMethod> =
  OperationFor<TPath, TMethod> extends {
    requestBody: { content: { "application/json": infer TBody } };
  }
    ? TBody
    : never;

type ApiRequestOptions = Omit<RequestInit, "body" | "credentials" | "method">;

export function setCSRFToken(value: string): void {
  csrfToken = value;
}

export function csrfHeaders(method = "POST"): Record<string, string> {
  return authHeaders(method);
}

function authHeaders(method = "GET"): Record<string, string> {
  const headers: Record<string, string> = {};
  const desktopApiToken = resolveDesktopApiToken();
  if (desktopApiToken) {
    headers.Authorization = `Bearer ${desktopApiToken}`;
  }
  if (csrfToken && !["GET", "HEAD", "OPTIONS"].includes(method.toUpperCase())) {
    headers["X-CSRF-Token"] = csrfToken;
  }
  return headers;
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
    if (body != null && !body.ok) {
      const error = new ApiClientError(
        body.error.message,
        body.error.code,
        response.status,
        responseRetryAfterMs(response),
      );
      notifyWebAuthRequired(error);
      throw error;
    }
    throw new Error(`${response.status} ${response.statusText}`);
  }

  if (body == null) {
    throw new Error("API response body is empty");
  }

  if (!body.ok) {
    const error = new ApiClientError(
      body.error.message || "Unknown API error",
      body.error.code,
      response.status,
      responseRetryAfterMs(response),
    );
    notifyWebAuthRequired(error);
    throw error;
  }

  return body.data;
}

function responseRetryAfterMs(response: Response): number | undefined {
  const raw = response.headers?.get?.("Retry-After")?.trim();
  if (!raw) return undefined;
  const seconds = Number(raw);
  if (Number.isFinite(seconds) && seconds >= 0) {
    return Math.ceil(seconds * 1000);
  }
  const retryAt = Date.parse(raw);
  if (!Number.isFinite(retryAt)) return undefined;
  return Math.max(0, retryAt - Date.now());
}

function notifyWebAuthRequired(error: ApiClientError): void {
  const webAccessBoundary =
    error.code === "WEB_AUTH_REQUIRED" ||
    error.code === "WEB_ACCESS_DISABLED" ||
    error.code === "REMOTE_WEB_ACCESS_DISABLED";
  if (!webAccessBoundary || resolveDesktopApiToken() != null) {
    return;
  }
  csrfToken = "";
  if (typeof window !== "undefined") {
    window.dispatchEvent(new Event(WEB_AUTH_REQUIRED_EVENT));
  }
}

export async function webSession(): Promise<WebSession> {
  const response = await fetch(buildApiUrl("/api/v1/auth/session"), {
    credentials: "include",
    headers: authHeaders("GET"),
  });
  return parseEnvelope<WebSession>(response);
}

export async function webLogin(password: string): Promise<WebSession> {
  const response = await fetch(buildApiUrl("/api/v1/auth/login"), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json", ...authHeaders("POST") },
    body: JSON.stringify({ password }),
  });
  return parseEnvelope<WebSession>(response);
}

export async function webLogout(): Promise<WebSession> {
  const response = await fetch(buildApiUrl("/api/v1/auth/logout"), {
    method: "POST",
    credentials: "include",
    headers: authHeaders("POST"),
  });
  const session = await parseEnvelope<WebSession>(response);
  csrfToken = "";
  if (typeof window !== "undefined") {
    window.dispatchEvent(new Event(WEB_AUTH_REQUIRED_EVENT));
  }
  return session;
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

function withJsonBody<TBody>(
  method: string,
  body: TBody,
  init: ApiRequestOptions = {},
): RequestInit {
  return {
    ...init,
    method,
    headers: {
      "Content-Type": "application/json",
      ...(init.headers as Record<string, string> | undefined),
    },
    body: JSON.stringify(body),
  };
}

export async function apiGet<TResponse, TPath extends PathWithMethod<"get">>(
  path: TPath,
  init?: ApiRequestOptions,
): Promise<TResponse> {
  return fetchEnvelopeWithInit<TResponse>(path, { ...init, method: "GET" });
}

export async function apiPost<TResponse, TPath extends PathWithMethod<"post">>(
  path: TPath,
  body: JsonRequestBody<TPath, "post">,
  init?: ApiRequestOptions,
): Promise<TResponse> {
  return fetchEnvelopeWithInit<TResponse>(
    path,
    withJsonBody("POST", body, init),
  );
}

export async function apiPut<TResponse, TPath extends PathWithMethod<"put">>(
  path: TPath,
  body: JsonRequestBody<TPath, "put">,
  init?: ApiRequestOptions,
): Promise<TResponse> {
  return fetchEnvelopeWithInit<TResponse>(
    path,
    withJsonBody("PUT", body, init),
  );
}

export async function apiDelete<
  TResponse,
  TPath extends PathWithMethod<"delete">,
>(path: TPath, init?: ApiRequestOptions): Promise<TResponse> {
  return fetchEnvelopeWithInit<TResponse>(path, { ...init, method: "DELETE" });
}

export async function apiGetPath<
  TResponse,
  TPath extends PathWithMethod<"get">,
>(
  _template: TPath,
  path: string,
  init?: ApiRequestOptions,
): Promise<TResponse> {
  return fetchEnvelopeWithInit<TResponse>(path, { ...init, method: "GET" });
}

export async function apiPutPath<
  TResponse,
  TPath extends PathWithMethod<"put">,
>(
  _template: TPath,
  path: string,
  body: JsonRequestBody<TPath, "put">,
  init?: ApiRequestOptions,
): Promise<TResponse> {
  return fetchEnvelopeWithInit<TResponse>(
    path,
    withJsonBody("PUT", body, init),
  );
}

export async function apiPostPath<
  TResponse,
  TPath extends PathWithMethod<"post">,
>(
  _template: TPath,
  path: string,
  body: JsonRequestBody<TPath, "post">,
  init?: ApiRequestOptions,
): Promise<TResponse> {
  return fetchEnvelopeWithInit<TResponse>(
    path,
    withJsonBody("POST", body, init),
  );
}

export async function apiPatchPath<
  TResponse,
  TPath extends PathWithMethod<"patch">,
>(
  _template: TPath,
  path: string,
  body: JsonRequestBody<TPath, "patch">,
  init?: ApiRequestOptions,
): Promise<TResponse> {
  return fetchEnvelopeWithInit<TResponse>(
    path,
    withJsonBody("PATCH", body, init),
  );
}

export async function apiDeletePath<
  TResponse,
  TPath extends PathWithMethod<"delete">,
>(
  _template: TPath,
  path: string,
  init?: ApiRequestOptions,
): Promise<TResponse> {
  return fetchEnvelopeWithInit<TResponse>(path, { ...init, method: "DELETE" });
}
