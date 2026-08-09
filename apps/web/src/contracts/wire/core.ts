import type { components } from "@/generated/openapi";

export type ApiEnvelopeDto =
  components["schemas"]["httpserver.Envelope"];

export type ApiErrorEnvelopeDto =
  components["schemas"]["httpserver.ErrorEnvelope"];

export type WebSession =
  components["schemas"]["webaccess.WebSessionData"];

export type WebLoginRequest =
  components["schemas"]["webaccess.webLoginRequest"];
