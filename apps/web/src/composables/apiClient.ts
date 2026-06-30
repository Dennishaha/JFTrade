import {
  type ApiErrorEnvelope,
  type ApiSuccessEnvelope,
} from "@/contracts";
import type { paths } from "@/generated/openapi";

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

export class ApiClientError extends Error {
  readonly code: string;
  readonly status: number;

  constructor(message: string, code: string, status: number) {
    super(message);
    this.name = "ApiClientError";
    this.code = code;
    this.status = status;
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
type JsonRequestBody<
  TPath extends ApiPath,
  TMethod extends HttpMethod,
> = OperationFor<TPath, TMethod> extends {
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
    if (body != null && !body.ok) {
      throw new ApiClientError(body.error.message, body.error.code, response.status);
    }
    throw new Error(`${response.status} ${response.statusText}`);
  }

  if (body == null) {
    throw new Error("API response body is empty");
  }

  if (!body.ok) {
    throw new ApiClientError(body.error.message || "Unknown API error", body.error.code, response.status);
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

export async function apiPost<
  TResponse,
  TPath extends PathWithMethod<"post">,
>(
  path: TPath,
  body: JsonRequestBody<TPath, "post">,
  init?: ApiRequestOptions,
): Promise<TResponse> {
  return fetchEnvelopeWithInit<TResponse>(path, withJsonBody("POST", body, init));
}

export async function apiPut<
  TResponse,
  TPath extends PathWithMethod<"put">,
>(
  path: TPath,
  body: JsonRequestBody<TPath, "put">,
  init?: ApiRequestOptions,
): Promise<TResponse> {
  return fetchEnvelopeWithInit<TResponse>(path, withJsonBody("PUT", body, init));
}

export async function apiDelete<
  TResponse,
  TPath extends PathWithMethod<"delete">,
>(
  path: TPath,
  init?: ApiRequestOptions,
): Promise<TResponse> {
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
  return fetchEnvelopeWithInit<TResponse>(path, withJsonBody("PUT", body, init));
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
