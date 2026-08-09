import type { components } from "@/generated/openapi";

export type ApiEnvelopeDto =
  components["schemas"]["httpserver.Envelope"];

export type ApiErrorEnvelopeDto =
  components["schemas"]["httpserver.ErrorEnvelope"];

export type WebSession =
  components["schemas"]["servercore.WebSessionData"];

export type WebLoginRequest =
  components["schemas"]["servercore.webLoginRequest"];
