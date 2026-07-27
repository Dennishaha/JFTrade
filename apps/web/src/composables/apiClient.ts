import type { ApiErrorEnvelope, ApiSuccessEnvelope } from "@/types";
import type { WebSession } from "@/contracts";
import type { paths } from "@/generated/openapi";

import { buildRuntimeApiUrl, resolveDesktopApiToken } from "../runtimeConfig";

export function buildApiUrl(path: string): string {
  return buildRuntimeApiUrl(path);
}

export type { WebSession } from "@/contracts";

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
export type RequestBodyFor<
  TPath extends ApiPath,
  TMethod extends HttpMethod,
> =
  OperationFor<TPath, TMethod> extends {
    requestBody?: { content: { "application/json": infer TBody } };
  }
    ? TBody
    : never;

type SuccessResponse<TResponses> = {
  [TStatus in keyof TResponses]: TStatus extends `${2}${number}${number}`
    ? TResponses[TStatus]
    : never;
}[keyof TResponses];

type JsonBodyFromResponse<TResponse> = TResponse extends {
  content: { "application/json": infer TBody };
}
  ? TBody
  : never;

type JsonSuccessBody<TOperation> = TOperation extends {
  responses: infer TResponses;
}
  ? JsonBodyFromResponse<SuccessResponse<TResponses>>
  : never;

// ResponseDataFor 从生成类型中推导某个路径任意 JSON 2xx 响应 envelope 的 data。
// 未 typed 的端点（envelope.data 为 unknown）安全退化为 unknown。
export type ResponseDataFor<TPath extends ApiPath, TMethod extends HttpMethod> =
  JsonSuccessBody<OperationFor<TPath, TMethod>> extends infer TBody
    ? TBody extends { data?: infer TData }
      ? unknown extends TData
        ? unknown
        : TData
      : unknown
    : unknown;

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
  if (!isWebAuthBoundaryCode(error.code) || resolveDesktopApiToken() != null) {
    return;
  }
  csrfToken = "";
  if (typeof window !== "undefined") {
    window.dispatchEvent(new Event(WEB_AUTH_REQUIRED_EVENT));
  }
}

function isWebAuthBoundaryCode(code: string): boolean {
  return (
    code === "WEB_AUTH_REQUIRED" ||
    code === "WEB_ACCESS_DISABLED" ||
    code === "REMOTE_WEB_ACCESS_DISABLED"
  );
}

async function notifyRawWebAuthRequired(response: Response): Promise<void> {
  if (!response.clone || resolveDesktopApiToken() != null) {
    return;
  }

  try {
    const body = (await response.clone().json()) as Partial<ApiErrorEnvelope>;
    const code =
      body != null &&
      typeof body === "object" &&
      body.ok === false &&
      body.error != null &&
      typeof body.error.code === "string"
        ? body.error.code
        : "";
    if (isWebAuthBoundaryCode(code)) {
      notifyWebAuthRequired(
        new ApiClientError(
          body.error?.message || "Web authentication required",
          code,
          response.status,
          responseRetryAfterMs(response),
        ),
      );
    }
  } catch {
    // Non-envelope failures remain available to the protocol-specific caller.
  }
}

async function performApiRequest(
  path: string,
  init: RequestInit = {},
): Promise<Response> {
  const headers = {
    ...authHeaders(init.method ?? "GET"),
    ...(init.headers as Record<string, string> | undefined),
  };
  return fetch(buildApiUrl(path), {
    ...init,
    credentials: "include",
    headers,
  });
}

// apiRawRequest is the single authenticated boundary for non-envelope
// protocols such as SSE. JSON API consumers must use the typed api* helpers.
export async function apiRawRequest(
  path: string,
  init: RequestInit = {},
): Promise<Response> {
  const response = await performApiRequest(path, init);
  if (!response.ok) {
    await notifyRawWebAuthRequired(response);
  }
  return response;
}

export async function webSession(): Promise<WebSession> {
  const response = await performApiRequest("/api/v1/auth/session");
  return parseEnvelope<WebSession>(response);
}

export async function webLogin(password: string): Promise<WebSession> {
  const body: RequestBodyFor<"/api/v1/auth/login", "post"> = { password };
  const response = await performApiRequest("/api/v1/auth/login", {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json", ...authHeaders("POST") },
    body: JSON.stringify(body),
  });
  return parseEnvelope<WebSession>(response);
}

export async function webLogout(): Promise<WebSession> {
  const response = await performApiRequest("/api/v1/auth/logout", {
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

async function requestEnvelopeWithInit<T>(
  path: string,
  init: RequestInit,
): Promise<T> {
  const response = await performApiRequest(path, init);
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

export function apiRawPost<TPath extends PathWithMethod<"post">>(
  path: TPath,
  body: RequestBodyFor<TPath, "post">,
  init?: ApiRequestOptions,
): Promise<Response> {
  return apiRawRequest(path, withJsonBody("POST", body, init));
}

export function apiGet<TPath extends PathWithMethod<"get">>(
  path: TPath,
  init?: ApiRequestOptions,
): Promise<ResponseDataFor<TPath, "get">>;
export async function apiGet(
  path: string,
  init?: ApiRequestOptions,
): Promise<unknown> {
  return requestEnvelopeWithInit(path, { ...init, method: "GET" });
}

export function apiPost<TPath extends PathWithMethod<"post">>(
  path: TPath,
  body: RequestBodyFor<TPath, "post">,
  init?: ApiRequestOptions,
): Promise<ResponseDataFor<TPath, "post">>;
export async function apiPost(
  path: string,
  body: unknown,
  init?: ApiRequestOptions,
): Promise<unknown> {
  return requestEnvelopeWithInit(path, withJsonBody("POST", body, init));
}

export function apiPostAction<TPath extends PathWithMethod<"post">>(
  path: TPath,
  init?: ApiRequestOptions,
): Promise<ResponseDataFor<TPath, "post">>;
export async function apiPostAction(
  path: string,
  init?: ApiRequestOptions,
): Promise<unknown> {
  return requestEnvelopeWithInit(path, { ...init, method: "POST" });
}

export function apiPut<TPath extends PathWithMethod<"put">>(
  path: TPath,
  body: RequestBodyFor<TPath, "put">,
  init?: ApiRequestOptions,
): Promise<ResponseDataFor<TPath, "put">>;
export async function apiPut(
  path: string,
  body: unknown,
  init?: ApiRequestOptions,
): Promise<unknown> {
  return requestEnvelopeWithInit(path, withJsonBody("PUT", body, init));
}

export function apiDelete<TPath extends PathWithMethod<"delete">>(
  path: TPath,
  init?: ApiRequestOptions,
): Promise<ResponseDataFor<TPath, "delete">>;
export async function apiDelete(
  path: string,
  init?: ApiRequestOptions,
): Promise<unknown> {
  return requestEnvelopeWithInit(path, { ...init, method: "DELETE" });
}

export function apiDeleteBody<TPath extends PathWithMethod<"delete">>(
  path: TPath,
  body: RequestBodyFor<TPath, "delete">,
  init?: ApiRequestOptions,
): Promise<ResponseDataFor<TPath, "delete">>;
export async function apiDeleteBody(
  path: string,
  body: unknown,
  init?: ApiRequestOptions,
): Promise<unknown> {
  return requestEnvelopeWithInit(path, withJsonBody("DELETE", body, init));
}

export function apiGetPath<TPath extends PathWithMethod<"get">>(
  _template: TPath,
  path: string,
  init?: ApiRequestOptions,
): Promise<ResponseDataFor<TPath, "get">>;
export async function apiGetPath(
  _template: string,
  path: string,
  init?: ApiRequestOptions,
): Promise<unknown> {
  return requestEnvelopeWithInit(path, { ...init, method: "GET" });
}

export function apiPutPath<TPath extends PathWithMethod<"put">>(
  _template: TPath,
  path: string,
  body: RequestBodyFor<TPath, "put">,
  init?: ApiRequestOptions,
): Promise<ResponseDataFor<TPath, "put">>;
export async function apiPutPath(
  _template: string,
  path: string,
  body: unknown,
  init?: ApiRequestOptions,
): Promise<unknown> {
  return requestEnvelopeWithInit(path, withJsonBody("PUT", body, init));
}

export function apiPostPath<TPath extends PathWithMethod<"post">>(
  _template: TPath,
  path: string,
  body: RequestBodyFor<TPath, "post">,
  init?: ApiRequestOptions,
): Promise<ResponseDataFor<TPath, "post">>;
export async function apiPostPath(
  _template: string,
  path: string,
  body: unknown,
  init?: ApiRequestOptions,
): Promise<unknown> {
  return requestEnvelopeWithInit(path, withJsonBody("POST", body, init));
}

export function apiPostPathAction<TPath extends PathWithMethod<"post">>(
  _template: TPath,
  path: string,
  init?: ApiRequestOptions,
): Promise<ResponseDataFor<TPath, "post">>;
export async function apiPostPathAction(
  _template: string,
  path: string,
  init?: ApiRequestOptions,
): Promise<unknown> {
  return requestEnvelopeWithInit(path, { ...init, method: "POST" });
}

export function apiPatchPath<TPath extends PathWithMethod<"patch">>(
  _template: TPath,
  path: string,
  body: RequestBodyFor<TPath, "patch">,
  init?: ApiRequestOptions,
): Promise<ResponseDataFor<TPath, "patch">>;
export async function apiPatchPath(
  _template: string,
  path: string,
  body: unknown,
  init?: ApiRequestOptions,
): Promise<unknown> {
  return requestEnvelopeWithInit(path, withJsonBody("PATCH", body, init));
}

export function apiDeletePath<TPath extends PathWithMethod<"delete">>(
  _template: TPath,
  path: string,
  init?: ApiRequestOptions,
): Promise<ResponseDataFor<TPath, "delete">>;
export async function apiDeletePath(
  _template: string,
  path: string,
  init?: ApiRequestOptions,
): Promise<unknown> {
  return requestEnvelopeWithInit(path, { ...init, method: "DELETE" });
}
