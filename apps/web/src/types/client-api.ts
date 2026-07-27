import type { components } from "@/generated/openapi";

type GeneratedEnvelope = components["schemas"]["httpserver.Envelope"];
type GeneratedErrorEnvelope =
  components["schemas"]["httpserver.ErrorEnvelope"];

export type ApiSuccessEnvelope<T> = Omit<
  GeneratedEnvelope,
  "data" | "error" | "ok"
> & {
  ok: true;
  data: T;
  error?: never;
};

export type ApiErrorEnvelope = Omit<GeneratedErrorEnvelope, "ok"> & {
  ok: false;
};
